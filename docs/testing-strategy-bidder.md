# Bidder 模块测试策略

> 产出自 `engineering:testing-strategy` skill + bidder 代码侦查。
> 首演计划：2026-04-20 选 P0-1（Loader 租户隔离）走一遍完整 TDD 流程，
> 验证 CLAUDE.md 新加的 TDD Evidence Rule。

## 1. 当前基线（事实，不是目标）

| 层 | 文件 | 真依赖？ | 覆盖目标 |
|----|------|---------|---------|
| **Unit / mock** | `bidder_test.go`<br>`engine_test.go`<br>`strategy_test.go`<br>`statscache_lifecycle_test.go` | 否（nil producer / miniredis / mock budgets） | 纯逻辑分支（pacing 公式、早期 return、超时/错误路径） |
| **Integration `//go:build integration`** | `engine_integration_test.go` (409行)<br>`handlers_integration_test.go` (542行)<br>`integration_test.go`<br>`loader_integration_test.go` | 是（testcontainers: 真 Postgres + 真 Redis + 真 Kafka） | 完整 bid/win/click flow、HMAC、dedup、CPC 计费、pub/sub reconcile |
| **回归 sentinel** | `main_test.go`（localhost:7380） | 是（真 Redis） | V5.1 P1-3 open-redirect 回归 |

**关键观察**：集成层已经打了真 store，**不存在 nil-store 反模式**。这是健康的起点。

---

## 2. 按组件分层策略

### 2.1 Engine — `internal/bidder/engine.go`
**核心职责**：竞价主控流（fraud → guardrail → GDPR → targeting → pacing → budget pipeline → dynamic bid → creative match → Kafka emit）

| 什么测 | 层 | 现状 | 建议 |
|--------|-----|------|------|
| 每个 gate 的**单点拒绝**（fraud yes/no, guardrail yes/no, GDPR yes/no, 超投放时间, 超地域, 超 OS） | Unit | 已覆盖 `engine_test.go` early return | 保持。每加一个 gate 必须先加一个 `TestEngineBid_<gate>_<case>` |
| **gate 组合**（例：fraud 过 + guardrail 拒；targeting 过 + budget 拒） | Integration | 部分 | **补**：对关键组合跑集成 |
| **dynamic bid adjustment 数值** | Unit | 已覆盖 strategy 侧 | 保持 |
| **creative 尺寸匹配**（impression slot ≠ creative → no-bid） | Unit | 需确认 | **补** — 之前有过 `secure flag` fix（1fa48b9）说明这块脆 |

**覆盖率目标**：`Engine.Bid` 分支覆盖 90%+（核心函数要求高）

### 2.2 Strategy — `internal/bidder/strategy.go`
**核心职责**：pacing 决策（ShouldBid）+ dynamic bid（AdjustedBid）+ Redis counter 记账

| 什么测 | 层 | 覆盖目标 |
|--------|-----|---------|
| `ShouldBid` 当 spend_rate > 1.1 时拒（已有） | Unit (miniredis) | 保持 |
| `AdjustedBid` win-rate 高时降价、低时升价的方向 | Unit | 保持 |
| Redis INCR 的**原子性**：并发 100 次 `RecordBid` 最后 counter = 100 | Integration | **补** — strategy_test.go 现在是单协程，race 靠 Redis 原子保证，但应该有断言 |
| `context.Background()` vs `r.Context()` 不被 handler cancel 带走 | Integration | 已有 sentinel (M5 Round 3) — 保持并定期手动跑 |

### 2.3 Loader — `internal/bidder/loader.go`
**核心职责**：campaign 加载 + pub/sub 订阅 + 30s 周期全量 reconcile

| 什么测 | 层 | 现状 | 建议 |
|--------|-----|------|------|
| 启动时从 DB 加载 active campaigns | Integration | 已覆盖 `loader_integration_test.go` | 保持 |
| pub/sub 收到 `campaign:updates` 后增量刷新 | Integration | 已覆盖 | 保持 |
| **租户隔离**：advertiser A 的 campaign 不会被加载到 advertiser B 的视图 | Integration | 需确认 | **P0 必补** — CLAUDE.md 点名的盲区 |
| 30s 周期 reconcile 把 DB 删除的 campaign 从内存踢掉 | Integration | 已覆盖 | 保持 |
| RWMutex 并发：边刷新边读不会 deadlock / panic | Unit (go test -race) | 需确认 | **补** — 跑 `-race` 跑 100 并发 GetCampaign + 1 ReplaceMap |

### 2.4 StatsCache — `internal/bidder/statscache.go`
**核心职责**：ClickHouse 24h CTR/CVR/WinRate 滚动窗口 + Redis 缓存 + 内存 fallback

| 什么测 | 层 | 覆盖目标 |
|--------|-----|---------|
| ClickHouse 宕机时内存 fallback 返回默认值不 panic | Unit | 已覆盖 lifecycle test | 保持 |
| 5min 刷新周期的 timer 正确停止 | Unit | 已覆盖 | 保持 |
| Redis cache miss → 回源 ClickHouse → 写回 Redis | Integration | 需确认 | **补** |

### 2.5 HTTP Handlers — `cmd/bidder/main.go`
**核心职责**：`/bid` `/win` `/click` `/convert` 的 HTTP 层

| 什么测 | 层 | 现状 | 建议 |
|--------|-----|------|------|
| HMAC token 验证 | Integration | 已覆盖 `handlers_integration_test.go` | 保持 |
| dedup SetNX（5min 窗口内重放被拒） | Integration | 已覆盖 | 保持；**注意 SetNX deprecated 迁移时必须验证** |
| CPC 计费：win 跳过扣费 + click 按 BidCPCCents 扣费 | Integration | 已覆盖 | 保持 |
| CPM 计费：win 按 clear_price 扣费 | Integration | 需确认 | **补** |
| **warm-up 期间 click 返回 503** 而不是错误计费（v5.2c 修） | Integration | 已有 sentinel | 保持 |
| **guardrail fail-closed**（Redis 挂时拒绝而非放行，a32ac0f 修） | Integration | 需确认 | **P0 必补** — fail-open 是安全事故 |
| `/internal/stats` 只接受 `X-Admin-Token`（68406de） | Integration | 需确认 | **补** — 权限回归 |

---

## 3. Coverage Gap 盘点（按优先级）

### P0（必补，安全/租户隔离相关）

| # | Gap | 建议 test 名 | 类型 |
|---|-----|-------------|------|
| 1 | Loader 租户隔离：advertiser A 加载时不污染 advertiser B 的 campaign 视图 | `TestLoader_TenantIsolation_NoCrossAdvertiserLeak` | Integration + 真 pg |
| 2 | `/internal/stats` 无 token / 错 token → 401 | `TestInternalStatsEndpoint_RequiresAdminToken` | Integration |
| 3 | guardrail Redis 宕机时 `/bid` 返回 no-bid (fail-closed) 而非竞价 | `TestBid_GuardrailFailClosed_OnRedisError` | Integration |
| 4 | Engine hot path: 即使 loader 加载了跨租户 campaign（人为注入），bid 响应里的 seatbid 也不暴露错 advertiser | `TestEngineBid_HotPathTenantIsolation` | Integration（需要故意污染 loader 状态） |

### P1（建议补，业务正确性）

| # | Gap | test 名 |
|---|-----|---------|
| 5 | CPM 计费：win 时按 clear_price 正确扣预算 | `TestHandleWin_CPM_DeductsClearPrice` |
| 6 | StatsCache Redis cache miss 回源 ClickHouse | `TestStatsCache_RedisMiss_FallsBackToClickHouse` |
| 7 | Loader RWMutex 并发压力（`-race`） | `TestLoader_ConcurrentReadWrite_NoRace` |
| 8 | Daily budget reset goroutine 在 DST / 时区变化边界正确（CST 0:00:05） | `TestDailyBudgetReset_TimezoneBoundary` |

### P2（可选，体验/长尾）

| # | Gap | test 名 |
|---|-----|---------|
| 9 | Kafka producer 挂时 replay buffer 持久化到磁盘 | `TestKafkaBuffer_SurvivesProducerDown` |
| 10 | Campaign 过期（end_date < now）不再参与竞价 | `TestCampaignExpiry_ExcludedFromBid` |
| 11 | Creative secure flag 不匹配（HTTP 请求要 HTTPS creative）no-bid（1fa48b9 回归哨兵） | `TestCreativeMatch_SecureFlag_Mismatch` |

---

## 4. Regression Sentinel（回归哨兵）现有 vs 建议

**已有（保持）：**
- M5 Round 3 `context.Background() vs r.Context()` RecordWin 哨兵
- V5.1 P1-3 `handleClick` open-redirect 哨兵
- v5.2c warm-up 期 click 返回 503 哨兵

**建议新增：**
- **P0-3 哨兵**：`guardrail` fail-closed on Redis error — a32ac0f 修过一次，值得守
- **P0-2 哨兵**：`/internal/stats` X-Admin-Token — 68406de 把它从公开端口搬到内部端口，权限退化风险高
- **P1-7 哨兵**：loader RWMutex 并发 `-race`

每个 sentinel 在测试注释里写明：`// REGRESSION SENTINEL: <commit-sha> <one-line summary>`。这样未来 reviewer grep `REGRESSION SENTINEL` 能快速找到所有需要特殊关注的测试。

---

## 5. 测试命令矩阵

| 粒度 | 命令 | 期望耗时 | 用途 |
|------|------|---------|------|
| 秒级 | `go test ./internal/bidder ./cmd/bidder -short` | <10s | 每次 commit pre-push，TDD 红绿循环 |
| 十秒级 | `go test -tags=integration ./internal/bidder ./cmd/bidder` | 30-60s | PR 提交前，Phase 结束 |
| `-race` | `go test -race ./internal/bidder` | 30-60s | PR 提交前，检查并发正确性 |
| 分钟级 | `bash scripts/qa/run.sh`<br>`python test/e2e/test_e2e_flow.py` | 2-5min | Phase Final |

**CI 强制**：PR 必须跑 Integration + `-race`，否则 request changes。

---

## 6. 覆盖率目标

- **Engine.Bid / handleBid / handleWin / handleClick**：分支覆盖 90%+
- **整个 `internal/bidder` 包**：行覆盖 80%+
- **`cmd/bidder` 包**：行覆盖 70%+（启动代码难覆，可接受）
- **新增 PR 不得降低现有覆盖率**（CI 跑 `go test -coverprofile` + diff check）

---

## 7. 和 CLAUDE.md TDD Evidence Rule 的接口

| Rule | 在 bidder 上的落地 |
|------|-------------------|
| Rule 1（bug fix 两 commit） | 所有 P0-3/P0-2/P0-4 的补测都应该先以 `test(bidder):` commit 落地，跑 CI 看它红，再跟 `fix(bidder):` commit 让它绿（即使 fix 已在 main 上也应该这么做以证明回归测试真的会抓） |
| Rule 3（租户/权限测试打真 Store） | 所有 P0 项都必须用 `qaharness.TestHarness` 的真 pg + 真 Redis，禁止 nil-store 或 mock-only |
| Rule 4（前端 TDD） | 不适用（bidder 纯后端） |

---

## 8. 下一步（可执行清单）

按优先级，建议接下来一个 Phase 里落：

1. **补 P0-1 到 P0-4 四个集成测试**（~200 行新 test 代码，跑真 testcontainers，预计 1-2 个 task）
2. **给 P0-3 / P0-2 / P1-7 加上 `// REGRESSION SENTINEL` 注释**
3. **在 `.github/workflows/ci.yml` 加 `-race` + `-coverprofile` 步骤**（如果还没有）
4. **P1 四项按需补**（不阻塞，但下次涉及相关代码时应先补测再改）

---

## 9. 首演计划（2026-04-20）

目标：拿 **P0-1 Loader 租户隔离** 走一遍完整 TDD，同时验证 CLAUDE.md 新加的 TDD Evidence Rule 能跑通。

预期步骤：
1. 新分支 `test/bidder-loader-tenant-isolation`
2. **Commit A**：写 `TestLoader_TenantIsolation_NoCrossAdvertiserLeak`，本地跑 `go test -tags=integration ./internal/bidder -run TestLoader_TenantIsolation` 必须 FAIL（证明有风险 or 证明测试本身能抓到）
3. 根据 Commit A 的红色原因分类：
   - 若代码真的没隔离 → **Commit B**：修 loader 代码让绿
   - 若代码其实隔离了（假阳性）→ 改测试逻辑让 RED 变成合理的"断言功能缺失"而不是"bug 存在"；或宣告此 gap 其实是 false alarm，在策略文档里划掉
4. push 分支，CI 跑（docs-check / test / frontend / docker）
5. 开 PR，**不用 Squash merge**（按 Rule 1）
6. 合入 main
7. 复盘：这次首演暴露了哪些 CLAUDE.md Rule 写得不够清楚的地方，回改 rule 文本
