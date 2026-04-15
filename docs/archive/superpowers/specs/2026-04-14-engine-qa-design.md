# Engine QA 轮次设计(2026-04-14)

本文档是 engine 分支一轮系统性 QA 的设计 spec。约束来自 `docs/contracts/biz-engine.md`(以下简称"契约")和 `CLAUDE.md` 中的 Development Workflow。

工作目录:`C:/Users/Roc/github/dsp/.worktrees/engine`
基线 commit:本 spec 提交前的 HEAD(见 git log)

---

## 1. 目标与范围

### 1.1 目标

对 engine 分支(`cmd/bidder`、`cmd/consumer`、`internal/bidder`、`internal/events`、`internal/budget`、`internal/reconciliation`、`internal/reporting`)做一轮端到端 QA,同时把测试空洞补齐,使契约 §4 "engine 负责" 列的每一项都有**可回归的集成测试**,并就地修掉发现的 bug。本轮采用 "测 + 修 + 补覆盖" 模式。

### 1.2 In scope

与契约 §4 engine 列对齐:

1. **CampaignLoader**:Redis `campaign:updates` 5 种 action(activated / updated / paused / completed / deleted)订阅 + 30s 全量 reload 兜底 + Postgres JSON `targeting` 解析
2. **Engine.Bid 出价主路径**:OpenRTB 解析、anti-fraud / guardrail / GDPR / 定向匹配(geo + os)/ imp.bidfloor / statsCache CTR-CVR / strategy 调价 / budget+freq PipelineCheck / BidResponse 写入(含 NURL+HMAC token+click tracker)/ 异步 `SendBid`
3. **`/win` `/click` `/convert`**:HMAC token 校验、Redis SetNX 去重、CPM vs CPC 扣费路径、金额单位换算、`strategy.RecordWin/Spend`、事件发布到 Kafka
4. **Kafka `events.Producer`**:`dsp.bids` / `dsp.impressions` schema、Async 模式下 disk buffer 回退与 `ReplayBuffer`、`dsp.dead-letter`
5. **Consumer → ClickHouse `bid_log`**:字段映射、失败路径、DLQ
6. **`reconciliation.RunHourly`**:Redis 日预算 vs ClickHouse 聚合 spend 对比、告警阈值、4 向数据一致性
7. **`reporting.GetAttributionReport`**:`last_click` / `first_click` / `linear` 三种模型、30 天回溯窗口、credit 数学不变式
8. **`reporting.GetCampaignStats`**:engine 写入 schema 与 biz 读 SQL 的一致性(engine 只验证 schema 稳定性,不改 biz SQL)

### 1.3 Out of scope(硬约束)

- **biz 侧代码**:`cmd/api` / `web/` / `internal/handler` / `internal/billing` / `internal/campaign` / `internal/reporting` 的 handler 层 — 只读不改
- **契约 §3.1 `dsp.billing` topic**:produced but unconsumed,本轮不覆盖
- **契约 §3.2 `SendLoss` 埋点**:定义但未调用,本轮不埋点不覆盖
- **契约 §3.3 创意 CRUD pub/sub**:biz 责任,本轮不动
- **`docs/contracts/biz-engine.md` 本身**:冻结。若发现契约问题在本 spec "已知遗留" 小节记录,最后合回 main 时统一处理
- **性能 baseline**(latency / QPS / Kafka lag 数值硬门槛):留给后续 `/benchmark` 轮次
- **`docker-compose.override.yml` / `.env`**:不动,本 worktree 的 compose 栈已预配 +12000 端口偏移
- **新业务功能**:本轮不加功能
- **engine 架构重构**:除本 spec §3.4 列出的"最小必要重构"外,不做架构改动
- **ClickHouse schema 改动**:即使发现不合理,先记录,不改

---

## 2. 架构决策与理由

下面 5 个决策是通过 brainstorming 阶段 5 轮多选题收敛得到的,这里一次性记录理由:

### 2.1 测试真实度:打真实 compose 栈(不 mock)

选 C 档:集成测试直接打本 worktree `docker compose up -d` 起好的栈,通过 env var 读连接串。

- **理由**:本轮要抓的大部分 bug(Kafka Async 吞错误、Redis Lua 原子性、ClickHouse 聚合 SQL 语义、pub/sub 丢消息)在 mock 下看不到。testcontainers 方案真实度相同但启动慢,本 worktree 的 compose 栈已经预配好隔离端口,直接复用。
- **代价**:测试要求 compose 栈先起起来,不能在 CI 干跑。本轮不解决 CI 适配,后续可以在 CI 里先 `docker compose up` 再跑。

### 2.2 场景深度:档 2 "Happy Path + 边界矩阵"(~43 场景)

每个模块按 "正常 / 错误 / 边界" 展开,共 43 个场景(P1 11 + P2 22 + P3 10)。档 1(~15)太浅抓不到未知 bug,档 3(~60+)里的混沌和长跑更适合单独 soak test 轮次。

### 2.3 Phase 切法:按"链路层次"切 3 个 Phase(不按模块)

- **P1 数据 + 配置基础**:CampaignLoader + budget/freq(出价依赖的下层)
- **P2 出价 + 结算链路 e2e**:Engine.Bid + /win /click /convert + Producer + Consumer + bid_log 落盘(完整链路,跨组件 bug 在这里暴露)
- **P3 消费路径**:Reconciliation + Attribution + GetCampaignStats 读方验证

**理由**:engine 的大部分 bug 是跨组件的(金额换算在多处各算一次、Kafka Async 吞错直到 consumer 才暴露、`event.Timestamp` 在 producer 被覆盖到 consumer 才体现)。按模块切会让这些 bug 藏到最后一个 Phase 集中爆发;按链路切则每个 Phase 都是一条"竖直切片",bug 立刻可见。

### 2.4 测试脚手架:新开 `internal/qaharness` 包

不扩展 `cmd/autopilot`(保持其 e2e 长跑定位),也不每个测试文件各写一份 setup,而是开一个只被 `//go:build integration` 测试引用的小型 helper 包。

### 2.5 Phase 退出标准:S2 "测试全绿 + 4 向数据对账"

每个 Phase 退出必须满足:

- `go test -tags=integration` 全绿
- `requesting-code-review` 报告无 Critical / Important
- **数据正确性**:用真实 SQL / Redis / Kafka 查询验证最终状态,不只看 HTTP 200 或函数返回值
- 4 向对账(Kafka 消息数 / ClickHouse 行数 / Redis 计数 / Engine 内部 metric)在关键场景下严格一致
- 整轮验证循环零问题(最多 5 轮,CLAUDE.md 硬约束)

不采用 S1(只看测试退出码,太松)或 S3(加性能门槛,真实栈抖动会吓到测试失败)。

---

## 3. 测试架构与包布局

### 3.1 `internal/qaharness` 新包

只被 `*_integration_test.go` 引用,不进生产 binary。

```
internal/qaharness/
├── env.go              # 从 env var 读 compose 栈连接串,构造 pg/redis/kafka/ch client
├── harness.go          # TestHarness 结构体,聚合所有 client,提供 Reset()
├── campaign.go         # SeedCampaign / SeedCreative / CleanCampaigns
├── openrtb.go          # BuildBidRequest(geo, os, format) 生成测试用 BidRequest
├── kafka.go            # WaitForMessages(topic, count, timeout) / DrainTopic
├── clickhouse.go       # WaitForRowCount(event_type, campaignID, n) / QueryCampaignSpend
├── redis.go            # GetBudgetRemaining / GetFreqCount / PublishUpdate
└── assert.go           # AssertKafkaEqCH / AssertBudgetDelta / AssertSpendConsistency
```

**约定**:
- 每个文件不超过 ~150 行
- `harness.go` 是 facade,测试代码只引用 `qaharness.TestHarness`
- `env.go` 读取的 env var(缺省值指向本 worktree compose 栈):

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `QA_POSTGRES_DSN` | `postgres://dsp:dsp_dev_password@localhost:17432/dsp?sslmode=disable` | bidder 用的主库(用户名/密码来自 `internal/config/config.go` 的默认值) |
| `QA_REDIS_ADDR` | `localhost:18380` | Redis |
| `QA_REDIS_PASSWORD` | `dsp_dev_password` | compose Redis 要求 `requirepass`,与 `docker-compose.yml:28` 的默认一致 |
| `QA_REDIS_DB` | `15` | 测试用隔离 DB 编号 |
| `QA_KAFKA_BROKERS` | `localhost:21094` | Kafka broker |
| `QA_CLICKHOUSE_ADDR` | `localhost:21001` | ClickHouse native TCP(go-clickhouse v2 用 native,不用 HTTP) |
| `QA_CLICKHOUSE_USER` | `default` | |
| `QA_CLICKHOUSE_PASSWORD` | `dsp_dev_password` | compose CH 配置要求密码,与 `docker-compose.yml:46` 默认一致 |

- 所有 client 复用生产代码的构造器(`pgxpool.New` / `redis.NewClient` / `kafka.Reader` / `clickhouse.Open`),不造轮子

### 3.2 集成测试文件布局

```
internal/bidder/
├── engine_integration_test.go        # P2 出价主路径 e2e
├── loader_integration_test.go        # P1 pub/sub + 30s reload
└── budget_integration_test.go        # P1 Lua 原子性 + freq 回滚

cmd/bidder/
└── handlers_integration_test.go      # P2 /win /click /convert + Kafka publish

cmd/consumer/
└── consumer_integration_test.go      # P2 Kafka → CH 落盘

internal/reconciliation/
└── reconciliation_integration_test.go  # P3 对账

internal/reporting/
└── attribution_integration_test.go     # P3 归因 + GetCampaignStats
```

全部用 `//go:build integration` build tag,默认 `go test ./...` 不会跑。显式触发:

```bash
go test -tags=integration -timeout 10m \
  ./internal/bidder/... \
  ./cmd/bidder/... \
  ./cmd/consumer/... \
  ./internal/reconciliation/... \
  ./internal/reporting/...
```

### 3.3 数据隔离约定

本 worktree 的 compose 栈已物理隔离,但测试之间仍需自清:

1. **Postgres**:QA 测试创建的 advertiser / campaign / creative 一律以 `qa-` 为 name 前缀。`TestHarness.Reset()` 只 `DELETE ... WHERE name LIKE 'qa-%'`,绝不碰非前缀数据
2. **Redis**:测试连 DB 15(`QA_REDIS_DB=15`),生产 bidder 用 DB 0。`Reset()` 只对 DB 15 做 `FLUSHDB`
3. **ClickHouse**:QA 测试用的 `advertiser_id` 统一在 9xxxxx 段,`Reset()` 执行 `ALTER TABLE bid_log DELETE WHERE advertiser_id >= 900000`(异步操作,`Reset()` 不等待)
4. **Kafka**:无法选择性删消息。每个测试用 per-test UUID 作 `request_id` 前缀,`WaitForMessages` 只统计匹配前缀的消息。测试自己的 consumer 用独立 group name(`qa-test-<uuid>-<topic>`)并在创建 reader 时 `SetOffset(kafka.LastOffset)`,只读从测试启动后的新消息,避免被历史积压消息干扰

### 3.4 最小必要重构

为了让 handler 和 consumer reader loop 能被 httptest / in-process 复用而非重写,下面两处重构是 scope 内的,会在 Phase 2 的第一个 task 里做:

1. **`cmd/bidder/main.go`**:把 handler 注册抽成 `func RegisterRoutes(mux *http.ServeMux, deps *Deps)` 和 `type Deps struct { engine *bidder.Engine; budgetSvc *budget.Service; ... }`。`main` 函数只负责构造 `Deps` 并调用 `RegisterRoutes`。
2. **`cmd/consumer/main.go`**:把 reader goroutine 里的 `for { ReadMessage; process }` 循环抽成 `func RunConsumer(ctx, cfg, store, dlq) error`,可在测试中用独立 ctx 启停。

两个重构的接口签名变化**不得影响外部调用方**(只改 cmd/ 下的私有结构),并在各自 task 说明里解释理由。

---

## 4. Phase 1:数据 + 配置基础(11 场景)

**目标**:确保 bidder 出价前依赖的 CampaignLoader 和 budget/freq 两层基础是可信的。本 Phase 不发起任何出价。

### 4.1 CampaignLoader 同步(5 场景)

| # | 场景 | 步骤 | S2 断言 |
|---|---|---|---|
| 1 | 启动时全量加载 | seed 3 active + 2 paused + 1 draft → `cl.Start(ctx)` | `len(GetActiveCampaigns)==3`,paused/draft 不在缓存 |
| 2 | pub/sub `activated` | seed 1 paused → DB update status=active → publish `activated` | ≤1s 缓存出现该 campaign |
| 3 | pub/sub `paused` / `completed` / `deleted` | 对 1 个 active campaign 分别发 3 种 action(3 子用例) | ≤1s 缓存移除 |
| 4 | pub/sub `updated`(targeting 变更) | `targeting.geo` 从 `["US"]` → `["CN"]` → publish `updated` | 缓存里新 targeting 生效,creatives 仍在 |
| 5 | pub/sub 丢消息 → 30s 兜底 reload | 直接 DB insert active campaign **不发 pub/sub** → 等 35s | 兜底 reload 后该 campaign 出现在缓存(直击契约 §1 "30s 兜底") |

### 4.2 CampaignLoader 边界(2 场景)

| # | 场景 | 断言 |
|---|---|---|
| 6 | pub/sub payload 非法 JSON | loader 不 panic,后续消息继续处理 |
| 7 | 未知 action 字段(如 `"weird"`) | loader 不 panic,记 warn,缓存不变 |

### 4.3 Budget Lua 原子性(3 场景)

| # | 场景 | 步骤 | S2 断言 |
|---|---|---|---|
| 8 | 单次扣减 | `InitDailyBudget(10000)` → `CheckAndDeductBudget(300)` | 返回 9700,Redis `GET` 9700 |
| 9 | 预算耗尽返回 -1 | `Init(100)` → 扣 50, 50, 1 | 第 3 次返回 -1,Redis 仍是 0(未被多扣) |
| 10 | 100 并发扣减 | `Init(10000)`,100 goroutine 各扣 50 | 成功+失败=100,最终 Redis 值 = 10000 − 50×成功数(严格相等) |

### 4.4 Frequency Pipeline(1 场景)

| # | 场景 | 断言 |
|---|---|---|
| 11 | budget OK + freq 超限 → 预算回滚 | `Init(10000)`,`cap=2`,3 次 `PipelineCheck` 同 userID | 第 3 次 `freqOK=false`,Redis budget = 9900(不是 9850,证明回滚生效) |

### 4.5 Phase 1 退出标准

- [ ] 11 个场景全部 pass(`go test -tags=integration ./internal/bidder/... ./internal/budget/...`)
- [ ] `requesting-code-review` 无 Critical / Important
- [ ] Postgres/Redis/bidder 内存缓存在每个场景结束时一致
- [ ] Redis DB 15 无 `qa-` 前缀残留 key
- [ ] Postgres 无 `qa-` 残留行
- [ ] 如有 fix,额外跑一轮完整验证循环直到整轮零问题(最多 5 轮)

---

## 5. Phase 2:出价 + 结算链路 e2e(22 场景)

**目标**:从 OpenRTB BidRequest 打进 bidder 到事件在 ClickHouse `bid_log` 被读到,完整链路端到端跑通 + 数据逐字对账。**跨组件 bug 主要在这里暴露。**

### 5.1 Engine.Bid 主路径(10 场景)

测试文件 `internal/bidder/engine_integration_test.go`,走真实 Engine + Loader + Budget + Strategy + StatsCache + Producer。

| # | 场景 | 输入 | S2 断言 |
|---|---|---|---|
| 12 | 正常出价(CN+iOS,1 候选) | seed 1 CPM campaign `targeting=[CN]` `os=[iOS]` | HTTP 200,`SeatBid.Price > 0`,Kafka `dsp.bids` 收到 1 条 `type=bid`,CH `bid_log` 落盘 1 条 |
| 13 | 多候选选最高 bid | seed 3 campaign,bid 差异可计算 | 选中的 seat 对应期望的最高 `EffectiveBidCPM` |
| 14 | 定向不匹配返回 204 | request `geo=US`,campaign `targeting=[CN]` | 204,Kafka 无新消息 |
| 15 | 无 Device 返回 204 | `BidRequest.Device=nil` | 204 |
| 16 | 无 Banner/Video/Native 返回 204 | `Imp` 三者皆 nil | 204 |
| 17 | bidfloor 过滤 | `imp.bidfloor` 高于 `EffectiveBidCPM` 换算 per-imp 价 | 204,该 candidate 被过滤 |
| 18 | Guardrail 全局熔断 | `guardrail.PreCheck` 返回拒绝 | 204 |
| 19 | Guardrail bid ceiling | `MaxBidCPMCents` 设低于 campaign bid | 204 |
| 20 | 预算已耗尽(PipelineCheck) | seed budget=0 | 204,Kafka 无新消息 |
| 21 | **CPC 候选 + StatsCache 一致性**(候选 bug 6) | CPC campaign,statsCache 返回 CTR=0.05 | Kafka `dsp.bids` 里的 `bid_price` 字段 == `EffectiveBidCPMCents(0.05, 0) × 0.90 / 100 / 1000`,**不是** 默认 CTR=0.01 版本 |

### 5.2 /win /click /convert handler + HMAC(6 场景)

测试文件 `cmd/bidder/handlers_integration_test.go`,用 httptest 起 mux(依赖 §3.4 的 `RegisterRoutes` 重构)。

| # | 场景 | 输入 | S2 断言 |
|---|---|---|---|
| 22 | /win 正常 CPM | 先 /bid 拿 NURL → 用 token 回调 /win | 200,Redis budget 减了 `int64(price/0.9*100)`,Kafka `dsp.bids type=win` + `dsp.impressions type=impression` 各 1 条 |
| 23 | /win HMAC 非法 | 篡改 token | 403,Redis 和 Kafka 无变化 |
| 24 | /win 重复回调去重 | 同一 request_id 连发 3 次 /win | 第 1 次扣费,第 2-3 次返回 `"duplicate"`,Redis budget 只减一次,Kafka 只 1 条 win |
| 25 | /win 金额换算边界 | `price=0.00123` | Redis 扣减量与 `int64(0.00123/0.9*100)=0` 一致,**记录是否因截断导致永远扣不到钱**(候选 bug 2 探针) |
| 26 | /click CPC 扣费 | seed CPC campaign → /bid → /click | Redis budget 减 `BidCPCCents`,Kafka `dsp.impressions type=click`,`AdvertiserCharge == BidCPCCents/100.0` |
| 27 | /convert HMAC 非法 | 篡改 token | 403,Kafka 无 conversion |

### 5.3 Kafka Producer Async 行为(3 场景)

| # | 场景 | 步骤 | S2 断言 |
|---|---|---|---|
| 28 | 正常发布 | 发 100 个 event | Kafka 读到 100 条,disk buffer 目录为空或无新增 |
| 29 | **Async 模式下 Kafka 不可达 → disk buffer**(候选 bug 4) | 用指向 closed port 的 broker 起临时 Producer → 发 10 个 event | disk buffer 文件包含 10 条 JSON 行。**若 Async 吞错导致 buffer 空,此场景 fail。** |
| 30 | ReplayBuffer 恢复 | 手工往 buffer 写 5 条,启动新 Producer 调 `ReplayBuffer` | Kafka 收到 5 条,`.replayed` 文件生成 |

### 5.4 Consumer → ClickHouse(3 场景)

测试文件 `cmd/consumer/consumer_integration_test.go`,通过 §3.4 的 `RunConsumer` 重构 in-process 启停。

| # | 场景 | 步骤 | S2 断言 |
|---|---|---|---|
| 31 | 5 种 event 全落盘 | 往 Kafka 推 bid/win/impression/click/conversion 各 1 条 | CH `bid_log` 5 行,`event_type` 与源一致,**`event_time` 字段等于 `event.Timestamp`**(候选 bug 3:Timestamp 被 Send 覆盖的问题) |
| 32 | unmarshal 失败不阻塞 | 推 1 条非法 JSON + 1 条正常 | CH 有 1 行(正常那条),consumer 继续运行 |
| 33 | CH 写入失败 → DLQ | 让 consumer 在读到 event 的瞬间 CH 连接不可用(通过临时停 CH 容器,或给 consumer 注入一个总是返回 error 的 `InsertEvent` wrapper 实现)→ 推 1 条正常 event | `dsp.dead-letter` 收到 1 条 DLQ 消息,payload 中 `original_topic` 正确,`attempt=1` |

### 5.5 Phase 2 退出标准

- [ ] 22 个场景全部 pass
- [ ] `requesting-code-review` 无 Critical / Important
- [ ] **4 向对账**:对 N 条 bid,`Kafka dsp.bids 数 == CH bid_log(event_type=bid)数 == Engine 内部 metric == 测试预期`
- [ ] **金额一致性**:对任意 /win 扣减,`Redis delta == CH sum(charge_cents where event_type=win) == 测试预期`(±1 cent 浮点容差)
- [ ] 候选 bug 2/3/4/6 每个**要么证伪,要么有复现测试 + 已修复**(记录进 §9)
- [ ] 整轮验证循环零问题(最多 5 轮)

---

## 6. Phase 3:消费路径 + 契约读方验证(10 场景)

**目标**:验证 engine 写出的 `bid_log` 能被 reconciliation 和 attribution 正确读回,金额对账不漂,归因模型数学上不出错。依赖 P2 留下的真实数据。

### 6.1 Reconciliation(4 场景)

测试文件 `internal/reconciliation/reconciliation_integration_test.go`。

| # | 场景 | 步骤 | S2 断言 |
|---|---|---|---|
| 34 | 一致无告警 | seed campaign budget=10000,CH `sum(charge)=3000`,Redis budget=7000 | `DiffPercent()==0`,`NeedsAlert(1.0)==false`,`alerter.Send` 未调用 |
| 35 | 超阈值告警 | Redis spent=3000,CH spent=3500 | `DiffPercent()≈14.3%`,`NeedsAlert(5.0)==true`,`alerter.Send` 被调 1 次,消息含 campaign_id 和 diff% |
| 36 | **SQL 聚合口径**(候选 bug 5) | 手工插 1 bid(charge=0)+1 win(charge=300)+1 click(charge=50,CPC)+1 conversion(charge=0) | `GetCampaignStats.SpendCents` 应该算 300 还是 350?根据结果决定是证伪还是修 SQL 加 `event_type in ('win','click')` 并对齐 Redis 扣减口径 |
| 37 | CH 查询失败不 panic | 关 CH 或传坏 `dayStart` | `RunHourly` 返回 error 或空 results,goroutine 不 panic |

### 6.2 Attribution 归因模型(4 场景)

测试文件 `internal/reporting/attribution_integration_test.go`。

| # | 场景 | 步骤 | S2 断言 |
|---|---|---|---|
| 38 | last_click | device_id=dA:`impression → click → impression → conversion` | path 的 credit 数组最后一项 `Credit=1.0`,其余 0,总和==1.0 |
| 39 | first_click | 同上 | credit 数组第一项 `Credit=1.0`,其余 0,总和==1.0 |
| 40 | linear | 3 touchpoints(2 imp+1 click) | 每项 `Credit≈0.333`,总和==1.0(±1e-9 容差) |
| 41 | empty touchpoints skip | 插 conversion 但同 device_id 无任何 imp/click | `TotalConversions==0`,无崩溃 |

### 6.3 GetCampaignStats schema 一致性(2 场景)

契约 §4 里 "`bid_log` 读取 → 报表 查询正确性" 归 biz,**schema 稳定性**归 engine。本 Phase 只验证 engine 写入的字段能被 biz SQL 正确读出。

| # | 场景 | 步骤 | S2 断言 |
|---|---|---|---|
| 42 | 混合聚合 | seed 1 campaign,通过 /bid+/win+/click 产生 10 bid+5 win+5 imp+2 click | `GetCampaignStats.Bids==10, Wins==5, Impressions==5, Clicks==2`,`CTR=0.4`,`WinRate=0.5` |
| 43 | 字段类型边界 | 插 `clear_price_cents=0xFFFFFFFF`(UInt32 max),`device_id=""` | 查询不出错,`GetAttributionReport` 中 `device_id==""` 的行被 `AND device_id != ''` 排除 |

### 6.4 Phase 3 退出标准

- [ ] 10 个场景全部 pass
- [ ] `requesting-code-review` 无 Critical / Important
- [ ] 对 P2 留下的数据跑 `RunHourly`,`DiffPercent < 0.1%`
- [ ] 归因数学不变式:场景 38/39/40 credit 总和 ==1.0
- [ ] 候选 bug 5 要么证伪要么修复
- [ ] 整轮验证循环零问题(最多 5 轮)

---

## 7. 整轮出口循环

CLAUDE.md 要求全部 Phase 结束后再跑一次完整验证。engine 没有前端,`/qa` 和 `/browse` 的替代方案:

1. **final-code-review**:全量 review 整个 branch diff,修 Critical / Important
2. **verification-before-completion**:
   - `go test -tags=integration -timeout 15m ./...` 一次性跑完 43 个场景
   - 额外跑一次 `cmd/autopilot` continuous run(约 3 分钟),验证长跑下无泄漏 / 死锁 / 残留
3. **手动 smoke(替代 /qa)**:engine 对外是 HTTP,手动 `curl`(或 httpie)打一遍:
   ```
   curl -X POST http://localhost:20180/bid -d @test-openrtb.json
   curl 'http://localhost:20180/win?campaign_id=...&price=...&request_id=...&token=...'
   curl 'http://localhost:20180/click?campaign_id=...&request_id=...&token=...'
   curl 'http://localhost:20180/convert?campaign_id=...&request_id=...&token=...'
   curl http://localhost:20180/stats
   curl http://localhost:20180/health
   curl http://localhost:20180/metrics
   ```
   所有响应和 HTTP 状态码存入测试报告 `final-smoke-curl.log`
4. **截图验证(替代 /browse)**:见 §8 测试报告小节,用 `/browse` 技能截取 Grafana / Prometheus / bidder /stats /metrics / biz web 报表页的截图

整轮出口循环最多 5 轮(CLAUDE.md 硬约束)。任意一轮有 fix,下一轮必须从 step 1 重新走。

---

## 8. 测试报告产出

### 8.1 报告路径

```
docs/archive/superpowers/reports/2026-04-14-engine-qa-report.md
docs/archive/superpowers/reports/2026-04-14-engine-qa-report/
    ├── screenshots/
    │   ├── grafana-bidder-overview.png
    │   ├── grafana-kafka-lag.png
    │   ├── grafana-clickhouse-insertion-rate.png
    │   ├── prometheus-bidder-metrics.png
    │   ├── bidder-stats-endpoint.png
    │   ├── bidder-metrics-endpoint.png
    │   └── biz-web-campaign-stats.png
    └── data/
        ├── p1-campaignloader-sync.json        # pub/sub 同步延迟样本
        ├── p1-budget-lua-concurrency.txt      # 100 并发 Lua 原子性结果
        ├── p2-4way-reconciliation.csv         # Kafka/CH/Redis/Engine 四向计数
        ├── p2-money-consistency.csv           # /win 扣费 Redis vs CH 逐条
        ├── p2-candidate-bugs-status.md        # 6 个候选 bug 状态
        ├── p3-reconciliation-diff.csv         # RunHourly 对账结果
        ├── p3-attribution-credits.json        # 三种模型数学不变式
        └── final-smoke-curl.log               # curl 全端点实录
```

报告目录 `docs/archive/superpowers/reports/` 和子目录在写作时用 Write 创建。

### 8.2 报告正文章节

```
1. Executive summary(总场景数 / 通过 / 发现 bug / 修复 / 证伪,3-5 行)
2. 环境信息(compose stack 版本、git commit hash、测试开始/结束时间)
3. Phase 1 结果(11 场景 pass/fail 表 + 数据链接 + Grafana 截图)
4. Phase 2 结果(22 场景 pass/fail 表 + 4 向对账表 + 金额一致性表 + 多张 Grafana 截图)
5. Phase 3 结果(10 场景 pass/fail 表 + 归因不变式三张表)
6. 候选 bug 状态表(ID / 描述 / 挂靠场景 / 状态 / 修复 commit)
7. Final smoke 结果(curl 输出 + HTTP 状态 + latency + 多张 UI 截图)
8. 已知遗留 / Out of scope 回顾(引用契约 §3.x)
9. 结论与建议
```

### 8.3 截图来源

compose 栈里已预配端口,用 `/browse` 技能(gstack headless chrome)截图:

- **Grafana** `http://localhost:16100`
- **Prometheus** `http://localhost:22090`
- **bidder /stats** `http://localhost:20180/stats`
- **bidder /metrics** `http://localhost:20180/metrics`
- **biz web campaign stats** `http://localhost:16000/campaigns/<id>/stats`(验证 engine 写的数据能被 biz 前端显示,完成"数据回流到业务系统"的闭环证据)

**Grafana dashboard 预配弹性**:如果 worktree 的 Grafana 尚未预配 dashboard,在 final smoke 前花 ~10 分钟从 Grafana UI 手动加一个最小 dashboard(bidder QPS + Kafka lag + CH insertion rate 三个 panel)并导出 JSON 存入 `docs/archive/superpowers/reports/2026-04-14-engine-qa-report/grafana-dashboard.json`。如果时间允许直接走这条路;如果紧张,可以退化到只用 Prometheus 原始 metrics 页 + bidder `/metrics` 文本页作为视觉证据。**硬要求:报告里至少 5 张截图**。

### 8.4 报告增量生成

报告不是最后一次性写出来,而是 **Phase 结束追加**:

| 时点 | 追加内容 | commit |
|---|---|---|
| Phase 1 结束 | §1 初稿 + §3 Phase 1 小节 | 一次 |
| Phase 2 结束 | §4 Phase 2 小节 | 一次 |
| Phase 3 结束 | §5 + §6 候选 bug 状态 | 一次 |
| Final smoke 结束 | §7 smoke 结果 + 更新 §1 数字 | 一次 |
| QA 全部结束 | §9 结论 | 一次 |

这样 PR commit history 就能反映 QA 过程。

---

## 9. 候选 bug 清单(执行前的假说)

本节列出 spec 编写阶段根据代码 inspection 发现的 6 个**可疑点**。这些不是结论,是 QA 要验证的假说。每一条明确挂到一个场景去证伪或修复。执行过程中会在 `p2-candidate-bugs-status.md` 和本节更新最终状态。

| ID | 描述 | 涉及文件:行 | 挂靠场景 | 初始状态 |
|---|---|---|---|---|
| CB1 | 生产路径 `Engine.Bid` 无端到端 httptest 覆盖(`integration_test.go` 测的是 legacy `bidder.New()`) | `internal/bidder/integration_test.go` | 整个 Phase 2 | 无覆盖(本轮补上即可) |
| CB2 | `handleWin` 里 `int64(price/0.90*100)` 浮点+单位换算,在小额或大额下可能漂移或截断 | `cmd/bidder/main.go:350,381` | 场景 22, 25 | 待验证 |
| CB3 | `events.Send` 里 `evt.Timestamp = time.Now().UTC()` 无条件覆盖,导致 `bid_log.event_time` 是 Kafka send 时间,不是事件发生时间,可能影响归因时序 | `internal/events/producer.go:75` | 场景 31 | 待验证 |
| CB4 | `kafka.Writer.Async = true` + `WriteMessages` 几乎不返回错误,`bufferToDisk` 分支基本不触发,Kafka 掉消息静默丢失 | `internal/events/producer.go:64, 95-97` | 场景 29 | 待验证 |
| CB5 | `reconciliation.RunHourly` 用 `c.BudgetDailyCents - remaining` 算 `redisSpent`,但 ClickHouse 这边 `sum(charge_cents)` 对所有 event_type 累加。若 click/convert 带 charge 会 double-count 或口径不齐 | `internal/reconciliation/reconciliation.go:85-96` + `internal/reporting/store.go:94` | 场景 36 | 待验证 |
| CB6 | `handleWin` 用 `EffectiveBidCPMCents(0, 0)` 回算 `bidPrice` 写进 Kafka,但 `/bid` 时用的是 statsCache 的 CTR/CVR,两处不一致。Kafka `bid_price` 字段 != 实际出价 | `cmd/bidder/main.go:391` + `internal/bidder/engine.go:147-149` | 场景 21 | 待验证 |

如果任一候选证伪为"非 bug",允许在报告 §6 表格标注"已核实非 bug,不修",不强制修复。若确认是 bug,按 TDD 补测试 → 修 → 再验证流程处理。

---

## 10. 已知遗留(不改但记录)

引用契约 §3:

- **§3.1** `dsp.billing` topic produced 但无 consumer — 本轮不动,未来如需独立结算服务从此 topic 重建
- **§3.2** `SendLoss` 定义但未调用 — 本轮不埋点,`bid_log` 里不会出现 `event_type=loss` 行
- **§3.3** 创意 CRUD 不发 pub/sub(`POST/PUT/DELETE /api/v1/creatives`)— biz 责任,engine QA 用例不依赖此路径

若本轮执行过程中发现**契约文档本身**有问题(例如字段语义不清),在报告 §8 "已知遗留" 小节记录,最后合回 main 时统一更新 `docs/contracts/biz-engine.md`。

---

## 11. Finishing 计划

走 `superpowers:finishing-a-development-branch` 技能,采用 **一个大 PR** 合回 main:

**PR 内容**:
- `internal/qaharness/` 新包(8 文件)
- 7 个 `*_integration_test.go` 文件(43 个测试)
- 发现的 bug fix(每个 fix 对应 §9 一条)
- 最小必要重构(`cmd/bidder/main.go` 和 `cmd/consumer/main.go` 的 handler / loop 抽取)
- `docs/archive/superpowers/specs/2026-04-14-engine-qa-design.md`(本 spec)
- `docs/archive/superpowers/plans/2026-04-14-engine-qa-plan.md`(下一步 writing-plans 生成)
- `docs/archive/superpowers/reports/2026-04-14-engine-qa-report.md` + `report/` 目录(增量累积)

**PR 操作**:
- 不 force-push
- `gh pr create` → 等 CI → 默认 squash 合并
- PR 描述引用本 spec 和 plan 文件路径,按 Phase 分小节列交付物
- commit 历史在每个 task 结束时由 TDD 流程创建,finishing 时不改写

**不生成**:
- CHANGELOG 条目(本轮不是 feature 发布,由 final-code-review 阶段的 `document-release` 技能判断)
- README / ARCHITECTURE 更新(同上)

---

## 12. 测试命令速查

**前置**(一次性):
```bash
cd /c/Users/Roc/github/dsp/.worktrees/engine
docker compose up -d
docker compose ps                 # 等所有服务 healthy
```

**单元测试(快速反馈)**:
```bash
go test ./...
```

**集成测试(打真实 compose 栈)**:
```bash
go test -tags=integration -timeout 10m \
  ./internal/bidder/... \
  ./cmd/bidder/... \
  ./cmd/consumer/... \
  ./internal/reconciliation/... \
  ./internal/reporting/...
```

**单个 Phase 的集成测试**:
```bash
# Phase 1
go test -tags=integration -timeout 5m ./internal/bidder/... ./internal/budget/...

# Phase 2
go test -tags=integration -timeout 10m ./internal/bidder/... ./cmd/bidder/... ./cmd/consumer/...

# Phase 3
go test -tags=integration -timeout 5m ./internal/reconciliation/... ./internal/reporting/...
```

**Final smoke**:
```bash
# 从 compose 栈打 bidder HTTP 端点
curl -sS http://localhost:20180/health | jq
curl -sS http://localhost:20180/stats | jq
curl -sS http://localhost:20180/metrics | head -50
# autopilot 长跑 3 分钟
go run ./cmd/autopilot -duration=3m -mode=continuous
```

**检查 QA 残留**:
```bash
# Postgres
psql "$QA_POSTGRES_DSN" -c "SELECT count(*) FROM campaigns WHERE name LIKE 'qa-%';"
# Redis DB 15
redis-cli -p 18380 -n 15 DBSIZE
# ClickHouse
echo "SELECT count() FROM bid_log WHERE advertiser_id >= 900000" | curl 'http://localhost:20124/' --data-binary @-
```
