# 测试任务拆解清单（2026-04-20）

本文把 [全功能、全链路测试计划](./full-function-full-chain-test-plan-2026-04-20.md) 拆成可直接执行的任务清单，目标是让 AI 可以按任务粒度推进，而不是只对着总计划泛泛实现。

适用方式：

- 作为测试补齐 backlog
- 作为 AI Coding 派工清单
- 作为 PR 验收与发布前 gate 的检查表

## 1. 执行原则

每个任务都必须满足这 5 条：

1. 有明确目标文件
2. 有明确新增/修改的测试类型
3. 有明确断言点
4. 有明确执行命令
5. 有明确完成标准（DoD）

不接受以下交付：

- “补了点测试”但没有说明覆盖了什么风险
- “跑绿了”但没说跑了哪些命令
- 只补 happy path，不补失败路径
- 只补单测，不补跨模块链路

## 2. 建议执行顺序

推荐按下面顺序推进：

1. `P0-1` exchange bid 路径
2. `P0-2` campaign start 失败分支
3. `P0-3` start -> loader -> bid 跨模块链
4. `P0-4` consumer batch DLQ
5. `P1-*` 逐项补强
6. `P2-*` 建浏览器与非功能体系

如果要并行派给多个 AI，优先按“写集不重叠”拆：

- Worker A：`cmd/bidder/handlers_integration_test.go`
- Worker B：`internal/handler/e2e_public_campaign_test.go`
- Worker C：`internal/bidder/*integration_test.go`
- Worker D：`cmd/consumer/consumer_integration_test.go`

## 3. P0 任务（必须先做）

### P0-1 `POST /bid/{exchange_id}` 正向集成测试

**目标**

补上 exchange bid 路径的真实 handler 测试，堵住 direct 路径和 exchange 路径行为漂移。

**目标文件**

- [cmd/bidder/handlers_integration_test.go](../cmd/bidder/handlers_integration_test.go)

**必须覆盖**

- 请求 `POST /bid/{exchange_id}` 可返回有效 bid response
- response 中带 `NURL`
- response 中带 click tracking
- click URL 可达
- response 格式经过 adapter `FormatBidResponse`
- 事件仍写入 `dsp.bids`

**建议新增测试**

- `TestHandlers_ExchangeBidHappyPath`
- `TestHandlers_ExchangeBid_UnknownExchange_400`
- `TestHandlers_ExchangeBid_IncludesClickTracking`

**关键断言**

- `SeatBid[0].Bid[0].NURL` 非空
- 返回 payload 中包含 click URL 或 tracking 标记
- `request_id` 对应的 `dsp.bids` 数量为 1
- 对未知 exchange 返回 400

**执行命令**

```powershell
go test -tags integration ./cmd/bidder/... -run ExchangeBid -count=1
```

**完成标准（DoD）**

- 至少一条 exchange 正向测试落地并通过
- 至少一条 exchange 失败路径测试落地并通过
- 测试能证明 exchange 路径不再漏 click tracking

### P0-2 Campaign start 失败分支测试

**目标**

把 `HandleStartCampaign` 最关键的 fail-closed 行为补齐，避免只测“余额足够”的 happy path。

**目标文件**

- [internal/handler/e2e_public_campaign_test.go](../internal/handler/e2e_public_campaign_test.go)
- 如需要，补辅助 fixture 到 [internal/handler/e2e_support_test.go](../internal/handler/e2e_support_test.go)

**必须覆盖**

- 余额不足时 start 失败
- `BillingSvc.GetBalance` 出错时 start 失败
- 没有 creative 时 start 失败
- 没有 approved creative 时 start 失败
- `budget_total_cents < budget_daily_cents` 时失败
- `end_date` 在过去时失败

**建议新增测试**

- `TestCampaign_Start_InsufficientBalance_Fails`
- `TestCampaign_Start_BalanceLookupError_FailsClosed`
- `TestCampaign_Start_NoCreative_422`
- `TestCampaign_Start_NoApprovedCreative_422`
- `TestCampaign_Start_BudgetTotalBelowDaily_422`
- `TestCampaign_Start_EndDatePast_422`

**关键断言**

- 返回码正确，不接受 200
- 不发布 `activated`
- DB 状态仍不是 `active`

**执行命令**

```powershell
go test -tags e2e ./internal/handler/... -run Campaign_Start -count=1
```

**完成标准（DoD）**

- 至少 4 条失败分支测试落地并通过
- 对应失败场景不会误发布 `activated`
- 能抓住“余额查询错误时仍放行”的缺陷

### P0-3 `start -> Redis -> loader -> /bid` 跨模块链路测试

**目标**

证明 campaign start 后不是“只发布了 Redis 消息”，而是 bidder 确实能立即看到 campaign 并参与竞价。

**目标文件**

- 新增一个 integration 测试文件，建议：
  - [internal/bidder/start_to_bid_integration_test.go](../internal/bidder/)（新文件）
  - 或补到 [internal/bidder/loader_integration_test.go](../internal/bidder/loader_integration_test.go)

**必须覆盖**

- 创建 campaign + creative
- 执行 start
- 收到 `activated`
- loader cache 出现该 campaign
- 紧接着发 bid request
- bidder 返回命中该 campaign 的 bid response

**建议新增测试**

- `TestStartToBid_ActivatedCampaignImmediatelyBiddable`

**关键断言**

- `loader.GetCampaign(id) != nil`
- `resp != nil`
- `resp.SeatBid[0].Bid[0].CID == campaignID`

**执行命令**

```powershell
go test -tags integration ./internal/bidder/... -run StartToBid -count=1
```

**完成标准（DoD）**

- 一条真正跨 API / Redis / loader / engine 的测试落地并通过
- 这条测试不依赖 sleep-only 判定最终结果，关键节点要有状态断言

### P0-4 Consumer batch DLQ 集成测试

**目标**

补齐生产默认路径 `runBatchLoop` 的失败测试，验证批量写 ClickHouse 失败时整批进入 DLQ。

**目标文件**

- [cmd/consumer/consumer_integration_test.go](../cmd/consumer/consumer_integration_test.go)
- 如需辅助类型，可补到 [cmd/consumer/runner_test.go](../cmd/consumer/runner_test.go)

**必须覆盖**

- Store 实现 `BatchBidLogStore`
- `InsertBatch` 返回错误
- consumer 走 `runBatchLoop`
- 同一批消息全部进入 `dsp.dead-letter`

**建议新增测试**

- `TestConsumer_CHBatchFailureDLQ`

**关键断言**

- 至少 2 条同批次消息进入 DLQ
- `original_topic` 正确
- 每条消息都可在 DLQ 中找到原始 `request_id`

**执行命令**

```powershell
go test -tags integration ./cmd/consumer/... -run BatchFailureDLQ -count=1
```

**完成标准（DoD）**

- 至少一条 batch DLQ 集成测试落地并通过
- 明确证明测试走的是 `BatchBidLogStore` 路径，而非 single insert 路径

## 4. P1 任务（本迭代补强）

### P1-1 `/win` 事件 payload 强化断言

**目标文件**

- [cmd/bidder/handlers_integration_test.go](../cmd/bidder/handlers_integration_test.go)

**必须覆盖**

- `campaign_id`
- `request_id`
- `clear_price`
- `creative_id`
- `bid_price`

**建议新增/强化**

- 强化 `TestHandlers_WinNormalCPM`

**执行命令**

```powershell
go test -tags integration ./cmd/bidder/... -run WinNormalCPM -count=1
```

### P1-2 creative 修改/删除后 bidder 实时刷新

**目标文件**

- [internal/handler/e2e_creative_pubsub_test.go](../internal/handler/e2e_creative_pubsub_test.go)
- [internal/bidder/loader_integration_test.go](../internal/bidder/loader_integration_test.go)
- 新增 bidder 命中行为断言测试

**必须覆盖**

- create/update/delete creative 后发 `updated`
- loader 刷新 creative 集合
- `/bid` 行为随 creative 变化而变化

### P1-3 autopause -> no-bid 链路

**目标文件**

- `internal/autopause/*test.go`
- `internal/bidder/engine_integration_test.go`

**必须覆盖**

- autopause 触发后 campaign 被暂停
- loader 移除 campaign
- 后续 `/bid` no-bid

### P1-4 replay buffer -> recovery

**目标文件**

- `internal/events/producer_integration_test.go`

**必须覆盖**

- Kafka 不可用时落磁盘
- Kafka 恢复后 replay
- 最终消息进入 Kafka/ClickHouse

### P1-5 管理与内部接口的强化验证

**目标文件**

- `internal/handler/e2e_admin_system_test.go`
- `cmd/bidder/main_test.go`
- 新增浏览器或 API 测试

**必须覆盖**

- `/internal/stats`
- admin users
- circuit status / reset
- advertiser list redact

## 5. P2 任务（体系建设）

### P2-1 前端浏览器自动化测试框架

**目标**

给 `web/` 建 Playwright 测试骨架。

**目标文件**

- `web/package.json`
- `web/playwright.config.*`
- `web/e2e/*`

**第一批页面**

- login gate
- overview
- campaigns list
- billing
- reports detail
- admin home

**完成标准**

- `cd web && npx playwright test` 可运行

### P2-2 关键用户旅程浏览器测试

建议拆成：

- `tenant-onboarding.spec.ts`
- `campaign-start.spec.ts`
- `billing-topup.spec.ts`
- `reports-analytics.spec.ts`
- `admin-review.spec.ts`

### P2-3 性能 smoke

**目标**

建立 `/bid`、consumer、reports 的轻量压测脚本。

**建议位置**

- `scripts/perf/`

**第一批目标**

- `/bid` QPS smoke
- consumer 批写 smoke
- reports 查询 smoke

### P2-4 可观测性自动验收

**必须覆盖**

- `/health/live`
- `/health/ready`
- `/metrics`
- request ID
- 关键错误日志

**建议位置**

- `test/observability/`

## 6. AI 派工建议

每个任务可以直接用下面格式派给 AI：

```text
处理测试任务 <任务ID>。

目标：
<复制任务目标>

只允许修改：
<目标文件>

要求：
1. 先阅读相关实现和现有测试
2. 补充最小必要测试
3. 不做无关重构
4. 运行对应命令
5. 最后汇报：新增测试、覆盖风险、执行结果
```

## 7. 发布前检查清单

### P0 必须全完成

- [ ] P0-1 exchange bid
- [ ] P0-2 campaign start 失败分支
- [ ] P0-3 start -> bid 跨模块链
- [ ] P0-4 consumer batch DLQ

### P1 至少完成大半

- [ ] P1-1 `/win` payload
- [ ] P1-2 creative 刷新链
- [ ] P1-3 autopause -> no-bid
- [ ] P1-4 replay -> recovery
- [ ] P1-5 admin/internal 强化验证

### P2 至少完成基础设施

- [ ] Playwright 框架
- [ ] 首批关键页面 smoke

## 8. 任务完成的最终定义

一个测试任务只有在下面全部满足时才算完成：

- 测试代码已落地
- 对应命令已实际执行
- 风险点已被断言覆盖
- 没有因为测试而引入新的 flaky 行为
- 结果已记录到 PR 或测试报告

这份清单的作用不是“管理测试工作”，而是把“AI 补测试”变成真正可交付、可验收、可追踪的工程任务。
