# Function Chain Fixes — Design

**Date**: 2026-04-19
**Author**: rocatriver + Claude (Opus 4.7)
**Status**: Brainstorming → Design approved, ready for `writing-plans`
**Source**: Codex review in `docs/function-chain-review.md`（未入库，留在 main 工作区），against `docs/function-chains.md` 的第 1 + 第 2 条链

## 1. Context

Codex 对两条核心链路（`POST /campaigns/{id}/start → bidder 生效`、`POST /bid → ClickHouse 入库`）做了 review，找出 5 条真 bug/契约问题，按严重度分 High/Medium。本 spec 是这 5 条的修复设计。

核验：
- 两条 High（F1、F2）已人工读源码确认存在，非误报
- 三条 Medium（F3、F4、F5）由 codex 指出代码位置，逻辑一致

## 2. Findings & Decisions

### F1 — `/bid/{exchange_id}` 缺 click tracker（High）

**问题**：直接 `/bid` 在 [cmd/bidder/main.go:311-314](../../../cmd/bidder/main.go) 生成 click URL 并调用 `injectClickTracker`；exchange 路径 [cmd/bidder/main.go:373-388](../../../cmd/bidder/main.go) 只补 `NURL`，没装 click tracker。结果：exchange 流量下 `/click` 永远不会被触发，CPC 计费 + click/conversion 链路断掉。

**决策**：抽共享 `decorateBidResponse` 函数，direct + exchange 都走；补 exchange integration test。

### F2 — `HandleStartCampaign` 余额查询 fail-open（High）

**问题**：[internal/handler/campaign.go:327-332](../../../internal/handler/campaign.go)
```go
balance, _, err := d.BillingSvc.GetBalance(r.Context(), advID)
if err == nil && balance < c.BudgetDailyCents { // err 被静默吞掉
    WriteError(...)
    return
}
```
账务库抖动 → `err != nil` → 条件为 false → 直接放行启动。资金/合规 blocker。

**决策**：fail-closed 覆盖两种失败模式：
1. `err != nil` → 返回 `503 Service Unavailable`
2. 非 sandbox campaign 且 `d.BillingSvc == nil` → 返回 `503`（CEO Finding #2 — 堵 fail-open 的另一面：配置遗漏导致 BillingSvc 为 nil 时当前是静默跳过余额检查）

**可测性前置条件**（CEO Finding #1）：必须先把 `Deps.BillingSvc` 从 `*billing.Service` 抽为 `BillingService` interface，否则 F2 测试无法注入 failing stub（FK 无 `ON DELETE CASCADE`，DELETE advertisers 行会因 campaign 引用而失败）。作为 Phase 1 第 0 个 task 落地，`billing.Service` 自动实现 interface，prod 零行为变化。

### F3 — Activation 链不原子（升级后：Critical，Codex Finding #3）

**原始问题**：`TransitionStatus(active)` → `InitDailyBudget` → `NotifyCampaignUpdate` 三步，pub/sub 失败时 bidder 要等 `loader.periodicRefresh()` 的 30s 周期。

**Codex 追加问题**：原分析只看了 pub/sub 失败这一条路径。活的活化链实际有**三个**"200 OK 但静默坏掉"的失败面：
1. pub/sub 失败（原分析 — 30s loader 兜底）
2. **`InitDailyBudget` 失败**（log-and-continue → Redis daily key 永不存在 → `budget.go` 当作 0 预算 → 永久 no-bid。loader 的 fallback 只 init total 不 init daily，**不能自恢复**）
3. **顺序竞态**：`TransitionStatus(active)` 提交 DB 后、`InitDailyBudget` 之前崩溃 → DB 是 active 但无 daily key

**契约对齐（用户：II-C = A+B 组合）**：
1. **A - handler fail-closed**：`HandleStartCampaign` 重排顺序——`InitDailyBudget` 在 `TransitionStatus(active)` 之前。失败返 503，DB 从未 transition，无 orphan state。"prepare-then-commit" 标准做法，消除顺序竞态
2. **B - loader 恢复路径**：`CampaignLoader.listenPubSub`（action=activated）和 `periodicRefresh` 的 fullLoad 都新增 `InitDailyBudget` 调用。如果 handler 层 transient Redis 抖动失败后 Redis 恢复了，loader 会兜底重试
3. **metric**：`campaign_activation_pubsub_failures_total{action}` 覆盖所有 **7 处** `NotifyCampaignUpdate` 调用点（不止 start/pause/update — 还有 creative approve / budget adjust 等 5 处 `action=updated`）
4. **API doc**：30s 契约变成**完整失败矩阵**，明确 503/422/409/200 各自的含义和恢复路径

### F2.5 — clearing price 未签 HMAC，可被 URL 篡改（Codex Finding #2，Decision I-B 新增 scope）

**问题**：exchange 回调 `/win?price=${AUCTION_PRICE}` 的 `price` 是 clearing price，参与 budget deduct 但未在 HMAC token 中签名。理论攻击面：有流量 MITM 能力时可以改大 price，让 win handler 多扣预算。

**决策**：F5 引入 `bid_price_cents` 到签名 token 后，`handleWin` 对比 URL `price` 和签过的 `bid_price_cents`。若 `price > bid_price_cents` → 封顶到 `bid_price_cents`，记 `bidder_clearing_price_capped_total` metric + warn log。攻击者即便改了 URL 的 price 也改不出 bid 上限。

### F4 — 直接 `/bid` 无 body size 限制（Medium）

**问题**：[cmd/bidder/main.go:270](../../../cmd/bidder/main.go) `json.NewDecoder(r.Body).Decode(&req)` 无大小限制；exchange 路径有 `io.LimitReader(r.Body, 1<<20)`。DoS 面不一致。

**决策**：直接 `/bid` 加 `http.MaxBytesReader(w, r.Body, 1<<20)`，两路径对齐。

### F5 — Win 事件使用"重算后的当前值"（Medium）

**问题**：[cmd/bidder/main.go:514-529](../../../cmd/bidder/main.go) 的 `handleWin` 用 `c.Creatives[0].ID` 和 `EffectiveBidCPMCents(0, 0)` 重算 `creativeID` 和 `bidPrice`。多 creative 或 bid 策略/CTR-CVR 在 bid→win 之间变化时，`bid_log` win 行和真实竞价响应不符；`GetBidTransparency` 会读到错的值。

**契约对齐（用户选 A）**：URL 携带真实值，HMAC 覆盖防篡改。

**决策**：
- NURL 扩成 `...&creative_id=<CrID>&bid_price_cents=<math.Round(price*100)>&...`（CEO Finding #4 — 用 `math.Round` 不 `int64()` 截断，避免 $0.00495 → 0 cents 的精度 bug）
- `auth.GenerateToken` 的签名参数从 `(campaignID, requestID)` 扩成 `(campaignID, requestID, creativeID, bidPriceCents)`
- `handleWin`/`handleClick` 从 URL 解析，不再重算
- `bid.CrID` 已由 [internal/bidder/engine.go:250](../../../internal/bidder/engine.go) 正确填入真实 `creative.ID`，无需额外改动

**HMAC token deploy 过渡**（CEO Finding #3）：`handleWin`/`handleClick` 的 token 验证走 try-6-then-4 过渡模式：
1. 先用 6 参数签名（`campID, reqID, creativeID, bidPriceCents`）验证——新 token 命中
2. 失败则退回 4 参数签名（`campID, reqID`）验证——老 token 命中，走重算 fallback，记 metric `bidder_token_legacy_accepted_total{handler}` + warn log
3. 两者都失败 → 403

一个 deploy 窗口稳定后（token TTL = 5min，保守等 ≥ 10min），开 follow-up PR 删掉 legacy 分支。这样消除"旧 binary 发 token、新 binary 验 token"的 403 spike 风险。

## 3. Phase Breakdown

分 3 个 Phase / 3 个 PR。

### Phase 1 — 小修 bug fix（F2 + F4）

- **T0**：refactor `Deps.BillingSvc` 从 `*billing.Service` 抽为 `BillingService` interface（F2 可测性前置 + 未来 mock 基础设施）
- **F2**：`HandleStartCampaign` 余额查询 fail-closed → 503（覆盖 `err != nil` 和 `BillingSvc == nil` 两种 fail-open）
- **F4**：直接 `/bid` 加 `http.MaxBytesReader`

**规模**：±60 行 prod + 2 个 failing test commit + 2 个 fix commit + 1 个 refactor commit（共 5 commit）

**TDD commit 序列**：
```
c1: refactor(handler): extract BillingService interface for testability (non-functional)
c2: test(handler): add failing test for start campaign fail-open (err + nil paths)
c3: fix(handler): return 503 when BillingSvc errors or is nil  [closes F2]
c4: test(bidder): add failing test for oversized /bid body rejection
c5: fix(bidder): enforce 1MB body limit on direct /bid  [closes F4]
```

`c1` 是纯 refactor 无 TDD（行为不变），但 `c2` 的 test 依赖 `c1` 抽出的 interface 注入 `&failingBillingStub{}`。

### Phase 2 — bid 响应装饰统一 + win 元数据（F1 + F5）

- 抽 `decorateBidResponse(bid, req, baseURL, secret)` 到 `cmd/bidder/decorator.go`
- `handleBid` 和 `handleExchangeBid` 都调
- NURL 扩展 + HMAC 签名扩展
- `handleWin` 解析新参数取代重算
- 同步改 10 处 `GenerateToken` caller（2 生产 + 8 测试）
- 旧 token 直接失效（token 短时效，不做兼容）

**规模**：±150 行 prod + ±80 行 test

**TDD commit 序列**：
```
c1: test(bidder): add failing test for exchange bid click tracker injection
c2: test(bidder): add failing test for win handler using real creative_id/bid_price from URL
c3: refactor(bidder): extract decorateBidResponse shared by direct+exchange bid
c4: fix(bidder): inject click tracker on exchange path + carry creative_id/bid_price_cents on NURL  [closes F1+F5]
```

### Phase 3 — Activation 契约 + 可观测性（F3）

- 注册 metric `campaign_activation_pubsub_failures_total{action}`
- `NotifyCampaignUpdate` 返回 error
- `HandleStartCampaign`/`HandlePauseCampaign`/`HandleUpdateCampaign` 记 metric（不阻塞响应）
- API doc 更新：在 `docs/contracts/*.md` 或 OpenAPI 的 start/pause/update 描述里加"最长 30s 最终一致"

**规模**：±40 行 prod + contract doc 更新

**TDD commit 序列**：
```
c1: test(bidder,handler): add failing test asserting pubsub_failures metric increments on Redis error
c2: feat(bidder,handler): NotifyCampaignUpdate returns error, handler records pubsub_failures metric
c3: docs(api): document 30s activation eventual consistency contract
```

## 4. TDD Evidence Strategy

按 CLAUDE.md 的 TDD Evidence Rule：

### 规则 1（硬性）
每条 `fix(...)` 必须两 commit：`test` 在前、`fix` 在后。不得 squash。PR merge 用 "Rebase and merge" 或 "Create a merge commit"。

每条 test commit 独立可 push、独立可跑：
- 在 test commit 上跑 `go test ./<pkg> -run <TestName>` 必须 FAIL
- 失败原因必须是"功能缺失/行为错误"，不是编译错误
- 在 fix commit 上跑同一命令必须 PASS

### 规则 3（硬性）
租户隔离 / 权限 / 边界测试必须打真 Store。F2 的 `HandleStartCampaign` 就是典型租户路径（`GetCampaignForAdvertiser`），**不得用 nil Store 或 fake mock**，必须 `pgtest.NewDB()` 起真 PG。

### PR Body TDD Evidence（规则 2）
每个 PR body 写：
- 每条 fix 对应的 test 名
- "RED 证据" — 在本地 test commit 上跑测试确实红了的描述

## 5. Test Coverage Matrix

| Finding | 单元测试 | 集成测试（真 Store） | API 链路 QA | E2E |
|---------|---------|---------------------|------------|-----|
| F1 exchange click | — | `handleExchangeBid` 打真 PG+Redis 断言 `AdM` 含 click URL | `scripts/qa/run.sh` 补一条 exchange bid 路径 | — |
| F2 fail-closed | `HandleStartCampaign` 真 PG + mock billing 返 error | — | `scripts/qa/run.sh` 补 503 case | — |
| F3 metric | `NotifyCampaignUpdate` 返 error 时 metric +1 | `HandleStartCampaign` 打真 PG + fake Redis failure 断言 metric | Prometheus `/metrics` 端点断言 | — |
| F4 body 限流 | `handleBid` 2MB body → 413 | — | QA 补超大 body case | — |
| F5 win 元数据 | `handleWin` URL 参数解析 + HMAC 验证新参数 | `bid → win` 全链路，`bid_log` 落真实 `bid_price_cents` + `creative_id` | QA 补 win 请求断言 | — |

**前端改动**：无。本批纯 backend。不需要 `test/e2e/test_e2e_flow.py`，不需要 `/qa` 前端检查。

## 6. Phase 边界验证（按 CLAUDE.md 的"审查→测试"循环）

每个 Phase 结束走完整循环，直到零问题才算该 Phase 通过：

```
1. superpowers:requesting-code-review    Phase 级 review
2. gstack /review + /codex               多专项 + Codex 对抗
3. 修复 review 发现的问题
4. go test ./... -short                  单元/集成
5. bash scripts/qa/run.sh                API 链路
6. （无前端，跳过 E2E + /qa）
```

**Phase 3 = Phase Final**（整个 batch 的终审），额外加：
- `gstack /cso` 安全审计 — F2 是资金路径、F5 是 HMAC token 语义变更
- `gstack /browse` — 跳过（无前端）
- `gstack /design-review` — 跳过（无 UI）

## 7. Risk & Open Questions

- **HMAC token deploy 过渡**（已在 CEO Finding #3 消化）：Phase 2 采用 try-6-then-4 过渡验证，旧 token 在 5min TTL 内仍能被新 binary 验通并走重算 fallback。deploy 窗口稳定 ≥10min 后开 follow-up PR 删 legacy 分支。`bidder_token_legacy_accepted_total` metric 监控过渡期。
- **Finding 3 的"30s 契约"是否被前端依赖**：如前端有"创建 campaign → 立即启动 → 立即看到投放"的工作流，30s 延迟可能 surface 成 UX 问题。Phase 3 前需要 grep 前端对 campaign status 的轮询策略，必要时加 `bidder_ready` 字段（留到后续 Phase）。
- **MaxBytesReader 的 error 路径**：Go 的 `http.MaxBytesReader` 超限时 `Decode` 返回 `*http.MaxBytesError`，handler 用 `errors.As` 判类型后返回 413（Payload Too Large）；其他 decode 错误仍返回 400。
- **`BillingSvc` prod 初始化**（CEO Finding #2 衍生）：本 Phase 1 T0 把 `BillingSvc` 抽 interface 后，prod 初始化代码应在 `cmd/api/main.go` 的 wiring 层 assert 非 nil（`if deps.BillingSvc == nil { log.Fatal("BillingSvc required in non-sandbox mode") }`）。handler 层的 `nil → 503` 是纵深防御，不能替代启动校验。Phase 1 T0 commit 建议顺带加这个 assert。

## 8. Not In Scope

- 重构 `handleBid`/`handleExchangeBid`/`handleWin` 到单独文件（`cmd/bidder/main.go` 已较大，但拆分是独立 refactor 任务，本 batch 不做）
- 前端对 campaign status 的轮询 / `bidder_ready` 字段（F3 契约的延伸，本 batch 只写契约 doc）
- Outbox 模式 / 事务性消息（F3 契约选了 B，强一致不做）
- 扩展 `decorateBidResponse` 到 click handler 也用（本 batch 只解 F1+F5）

## 9. Next Step

brainstorming 完成 → 用户 review 此 spec → `superpowers:writing-plans` 出实现计划（按 Phase 拆 task，每个 task 标注 TDD 证据约束）。
