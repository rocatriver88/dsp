# 关键函数链

这份文档把仓库里最常用的两条运行链路展开到函数级别，方便做 4 件事：

- 查 bug 时快速定位入口、状态变更点和跨服务触发点
- 做改动影响分析，判断需要补哪些测试
- 给 Codex/Claude Code 下更精确的任务
- review 时核对一条链是否被改断、漏测或漏掉下游同步

## 1. `POST /api/v1/campaigns/{id}/start` 到 bidder 生效

### 1.1 入口与 handler 链

1. API 进程从 [cmd/api/main.go](../cmd/api/main.go) 的 `main()` 启动，装配 `campaign.Store`、`billing.Service`、`budget.Service`、Redis、Postgres 等依赖。
2. `main()` 调用 [internal/handler/routes.go](../internal/handler/routes.go) 的 `BuildPublicHandler` 构建公共 HTTP handler 链。
3. `BuildPublicHandler` 内部调用 `BuildPublicMux` 注册路由。
4. `BuildPublicMux` 把 `POST /api/v1/campaigns/{id}/start` 绑定到 [internal/handler/campaign.go](../internal/handler/campaign.go) 的 `HandleStartCampaign`。
5. 请求经过 `WithCORS -> RequestIDMiddleware -> LoggingMiddleware -> WithAuthExemption -> RateLimit -> TenantAuthMiddleware` 后进入 `HandleStartCampaign`。

### 1.2 `HandleStartCampaign` 内部调用链

`HandleStartCampaign` 的主路径如下：

1. 从 path 读取 `id`，从 context 读取当前 advertiser ID。
2. 调用 [internal/campaign/store.go](../internal/campaign/store.go) 的 `GetCampaignForAdvertiser`，确认 campaign 属于当前租户。
3. 调用同一个 `campaign.Store` 上的 `GetCreativesByCampaign`，校验至少存在一个素材。
4. 在 handler 内做启动前置校验：
   - `end_date` 不能已过期
   - `budget_total_cents >= budget_daily_cents`
   - 非 sandbox campaign 在余额不足时不能启动
5. 若账务依赖存在，调用 `BillingSvc.GetBalance` 做余额检查。
6. 调用 `campaign.Store.TransitionStatus(..., campaign.StatusActive)`，把 DB 中的 campaign 状态改为 `active`。
7. 若 `BudgetSvc` 存在，调用 `InitDailyBudget` 初始化 Redis 中的日预算计数器。
8. 若 `Redis` 存在，调用 [internal/bidder/loader.go](../internal/bidder/loader.go) 的 `NotifyCampaignUpdate(..., "activated")` 发布 pub/sub 事件。
9. handler 返回 `{"status":"active"}`。

### 1.3 跨服务同步到 bidder 的链

`NotifyCampaignUpdate` 之后，真正让 bidder 生效的是下面这条链：

1. `NotifyCampaignUpdate` 把 `{"campaign_id": <id>, "action": "activated"}` 发布到 Redis channel `campaign:updates`。
2. Bidder 进程从 [cmd/bidder/main.go](../cmd/bidder/main.go) 的 `main()` 启动时，构造 [internal/bidder/loader.go](../internal/bidder/loader.go) 的 `CampaignLoader`。
3. `main()` 调用 `loader.Start(workerCtx)`：
   - 先执行 `fullLoad()` 做一次全量加载
   - 再同步订阅 Redis `campaign:updates`
   - 后台启动 `periodicRefresh()` 和 `listenPubSub()`
4. `listenPubSub()` 收到 `action == "activated"` 的消息后：
   - 调用 `campaign.Store.GetCampaign(ctx, campaignID)` 回查 Postgres
   - 调用 `toCampaignWithCreatives()` 重新解析 targeting 和 creatives
   - 把结果写回 `CampaignLoader.campaigns` 内存 map
   - 若有 `budgetSvc`，补做 `InitTotalBudget`
5. 之后新进来的 `/bid` 请求会通过 `loader.GetActiveCampaigns()` 看到这个 campaign。

### 1.4 “生效”在代码里的准确含义

这里的“bidder 生效”不是指 API 返回 200，而是指：

- campaign 已在 Postgres 中切成 `active`
- Redis 已发出 `campaign:updates`
- bidder 的 `CampaignLoader` 已把该 campaign 刷进内存缓存
- 后续 [cmd/bidder/main.go](../cmd/bidder/main.go) 的 `handleBid` 调用 `Engine.Bid` 时，`loader.GetActiveCampaigns()` 能取到它

### 1.5 这条链最相关的测试

- [internal/handler/e2e_public_campaign_test.go](../internal/handler/e2e_public_campaign_test.go)
  - `TestCampaign_StartPublishesActivated`
  - `TestCampaign_PausePublishesPaused`
  - `TestCampaign_UpdatePublishesUpdated`
- [internal/bidder/loader_integration_test.go](../internal/bidder/loader_integration_test.go)
  - `TestLoader_PubSubActivated`
  - `TestLoader_PubSubUpdatedTargeting`
  - `TestLoader_PubSubRemoveActions`
  - `TestLoader_FallbackReload`
- [internal/bidder/engine_integration_test.go](../internal/bidder/engine_integration_test.go)
  - 启动后 campaign 是否真的参与竞价，最终还是由这些 engine 集成测试兜底验证

## 2. `POST /bid` 到 ClickHouse 入库

### 2.1 入口与 HTTP 链

1. Bidder 进程从 [cmd/bidder/main.go](../cmd/bidder/main.go) 的 `main()` 启动。
2. `main()` 装配：
   - `budget.Service`
   - `BidStrategy`
   - `CampaignLoader`
   - `StatsCache`
   - `events.Producer`
   - `antifraud.Filter`
   - `guardrail.Guardrail`
3. `main()` 创建 `Deps`，调用 [cmd/bidder/routes.go](../cmd/bidder/routes.go) 的 `RegisterRoutes` 注册公共路由。
4. `RegisterRoutes` 把 `POST /bid` 绑定到 [cmd/bidder/main.go](../cmd/bidder/main.go) 的 `handleBid`。
5. 请求经过 bidder 本地的 `withLogging()` 中间层后进入 `handleBid`。

### 2.2 `handleBid` 到 `Engine.Bid`

`handleBid` 的主路径如下：

1. 解析 OpenRTB `BidRequest`。
2. 调用 [internal/bidder/engine.go](../internal/bidder/engine.go) 的 `Engine.Bid(ctx, req)`。
3. `Engine.Bid` 依次执行：
   - 基础请求合法性校验
   - 设备信息提取
   - secure/bidfloor/site/app 类别等信号提取
   - `fraud.Check` 反作弊
   - `guardrail.PreCheck` 护栏前置检查
   - GDPR / CCPA 处理
   - `loader.GetActiveCampaigns()` 获取内存中的活动 campaign
   - `matchesTargeting()` 过滤定向条件
   - `matchCreativeToImp()` 匹配素材与广告位
   - `statsCache.Get()` 读取 CTR/CVR
   - `strategy.AdjustedBid()` 调整 bid
   - `guardrail.CheckBidCeiling()` 校验 bid ceiling
   - `budget.PipelineCheck()` 用 Redis 一次 RTT 做预算/频控检查
4. 选出最佳 campaign 后，组装 `openrtb2.BidResponse`。
5. `Engine.Bid` 通过 `producer.Go(func() { producer.SendBid(context.Background(), evt) })` 异步发送 bid 事件。
6. `handleBid` 回填 tracking 信息并把竞价响应返回给上游。

### 2.3 `SendBid` 到 Kafka

bid 事件离开 bidder 进程的链路是：

1. [internal/events/producer.go](../internal/events/producer.go) 的 `SendBid` 把事件类型设为 `bid`。
2. `SendBid` 调用 `Send(ctx, "dsp.bids", evt)`。
3. `Send`：
   - 补 `Timestamp`
   - JSON 序列化
   - 用 `campaign_id` 作为 Kafka message key
   - 写入 topic `dsp.bids`
4. 若 Kafka 写失败，则 `bufferToDisk()` 把消息落到本地磁盘缓冲目录，等待 `ReplayBuffer()` 重放。

### 2.4 Kafka 到 ClickHouse

写入 ClickHouse 的链路在 consumer 进程里：

1. [cmd/consumer/main.go](../cmd/consumer/main.go) 的 `main()` 创建 `reporting.Store` 和 `DLQProducer`。
2. `main()` 调用 [cmd/consumer/runner.go](../cmd/consumer/runner.go) 的 `RunConsumer(ctx, deps)`。
3. `RunConsumer` 为每个 topic 启一个 Kafka reader；默认消费：
   - `dsp.bids`
   - `dsp.impressions`
4. 每个 reader 根据 store 能力选择：
   - `runBatchLoop()` 批量写入
   - 或 `runSingleLoop()` 单条写入
5. 每条消息先经过 `decodeEvent()`，把 `events.Event` 转成 `reporting.BidEvent`。
6. 然后调用 [internal/reporting/store.go](../internal/reporting/store.go)：
   - `InsertBatch()`，或
   - `InsertEvent()`
7. 两者最终都执行 `INSERT INTO bid_log (...)`，把事件写入 ClickHouse 的 `bid_log` 表。
8. 若写 ClickHouse 失败，consumer 通过 `DLQProducer.SendToDeadLetter(...)` 把原始消息送到 `dsp.dead-letter`。

### 2.5 这条链入库后谁来读

入库完成后，API 服务里的 `reporting.Store` 查询接口会读取同一张 `bid_log` 表，例如：

- `GetCampaignStats`
- `GetHourlyStats`
- `GetGeoBreakdown`
- `GetBidTransparency`
- `SimulateBid`

也就是说，`/bid` 这条链不是“只返回一次竞价响应”，它还会顺着 `Kafka -> consumer -> ClickHouse -> reporting query` 进入后续分析面。

### 2.6 关于 `win/click/convert`

`POST /bid` 本身只会直接发出 `bid` 事件。后续这些回调沿用同一条下游写入通道：

- [cmd/bidder/main.go](../cmd/bidder/main.go) `handleWin` -> `producer.SendWin`
- [cmd/bidder/main.go](../cmd/bidder/main.go) `handleClick` -> `producer.SendClick`
- [cmd/bidder/main.go](../cmd/bidder/main.go) `handleConvert` -> `producer.SendConversion`

它们和 `SendBid` 一样，最终都经过 consumer 写进 `bid_log`。

### 2.7 这条链最相关的测试

- [internal/bidder/engine_test.go](../internal/bidder/engine_test.go)
  - targeting、creative match、bid 计算的单测
- [internal/bidder/engine_integration_test.go](../internal/bidder/engine_integration_test.go)
  - `TestEngine_BidHappyPath`
  - `TestEngine_MultiCandidateHighestBid`
  - `TestEngine_BidFloorFilter`
  - `TestEngine_BudgetExhausted`
- [cmd/bidder/handlers_integration_test.go](../cmd/bidder/handlers_integration_test.go)
  - `handleWin / handleClick / handleConvert` 的真实 handler 行为
- [cmd/consumer/runner_test.go](../cmd/consumer/runner_test.go)
  - `decodeEvent` 和批处理行为
- [cmd/consumer/consumer_integration_test.go](../cmd/consumer/consumer_integration_test.go)
  - `TestConsumer_AllEventTypesLand`
  - `TestConsumer_CHFailureDLQ`
- [internal/reporting/attribution_integration_test.go](../internal/reporting/attribution_integration_test.go)
- [internal/reporting/stats_integration_test.go](../internal/reporting/stats_integration_test.go)

## 3. 用这份文档的推荐方式

拿到一个需求或 bug 后，先判断它落在哪条链上，再反推要看的文件：

- 如果是 campaign 启停后 bidder 没生效，先看第 1 条链
- 如果是报表没数、数不对、Kafka/ClickHouse 延迟异常，先看第 2 条链
- 如果要给 Codex 下任务，直接把“沿着第几条链排查/修改”写进 prompt

示例：

```text
沿着 docs/function-chains.md 里的第 1 条链排查：
为什么 POST /api/v1/campaigns/{id}/start 返回 200 后，bidder 仍然看不到该 campaign。
请逐步检查 handler、DB 状态变更、Redis pub/sub、loader.listenPubSub 和相关测试。
```
