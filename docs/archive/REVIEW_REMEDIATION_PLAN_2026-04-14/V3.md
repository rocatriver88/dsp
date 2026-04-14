# 评审整改任务单 V3（2026-04-14）

## 说明

本版基于 V1 原始评审、Claude Code 的复核意见（V2 前半）、以及 codex 对 V2 的二次评审合并整理，达成以下收敛状态：

- V1 的 14 项事实断言经代码核实，13 TRUE / 1 PARTIAL（`HandleBidSimulate` 已做 ownership 检查，另 5 条未做），无任何一项 STALE。
- Claude Code 在 V2 中提出的补充修正（creative 越权、TDD 先行、删除 `admin-secret`、`json:"-"` 取代 DTO、P2 聚焦回归）**全部保留**。
- codex 在 V2 → V3 阶段指出 Claude Code V2 的 `event_type IN ('win','impression')` 迁移方案**存在双计缺陷**，该结论正确，**V3 撤销该方案**，采纳 codex 提出的基于 `request_id` 去重的"有效曝光口径"。
- codex 要求对 shutdown 只约束依赖顺序、不规定具体代码结构，对 `Config.Validate()` 只约束启动期失败、不规定 error 返回形态——**采纳**。
- codex 新指出 `HandleListCreatives` 也存在越权读取、越权错误码需一致（403 / 404 不能混用）、analytics SSE 通过 query 传 `api_key`——**全部采纳**，并进一步明确 404/403 选择规则（见 §P0）。

本文件是目前的执行基线。若后续评审发现仍有遗漏或错误，继续下个版本迭代；但 V3 之前的分歧已经收敛。

## 演进与修正日志

| 版本 | 提出方 | 内容 | 收敛状态 |
|---|---|---|---|
| V1 | 原评审 | P0 advertiser/billing/report scope + api_key 泄露 + config/admin + 前端硬编码；P1 win/impression 双写 + click 去重 + context；P1 lifecycle；P2 补测 | 事实全部经代码核实 |
| V2 | Claude Code | +creative handler scope（P0）；+强制 TDD；删 admin-secret fallback；json:"-" 代替 DTO；win/impression 两步迁移（**Step 1A 使用 `IN ('win','impression')` OR 聚合**）；shutdown 四步排序带具体代码形态；P2 改为集成测试 | creative/TDD/admin/json 部分保留；OR 聚合方案被撤销；shutdown 代码形态要求被软化 |
| V3 | codex + Claude Code | 修正 Step 1A 为 `countDistinct by request_id`；shutdown 只规定依赖而不绑定代码结构；Validate 只规定启动期失败而不绑定形态；+`HandleListCreatives` 越权；+404/403 一致性规则；+SSE `api_key` 观察项 | 本版 |

### 关于 V2 Step 1A 的错误记录（供未来审计用）

Claude Code 在 V2 中提出的 Step 1A 方案是：在 bidder 保持双写 `win + impression` 的情况下，把下游聚合条件改为 `event_type IN ('win','impression')`。**该方案会让每一次赢标被计为两次曝光**（因为 bid_log 已经存在两条行），导致迁移那天 CTR 指标被腰斩、autopause 阈值失效。codex 在 V2 评审中指出该缺陷，V3 完全采纳修正——用 `request_id` 去重的方式定义"有效曝光"，使指标在双写、停写、未来单写三种状态下都稳定。

## 执行原则

- 先安全边界，后数据语义，最后运行时治理。
- 每个整改项遵循 TDD：先写一个能复现问题的测试（越权应被拒、重复应被去重、ctx 取消后仍写入等），验证 red，再落地修复，验证 green。
- 能用局部修复解决的问题，不顺手做架构翻新；但当"最小修复"会破坏现有语义时（如 win/impression），必须拆成可独立上线的多步迁移。
- 只对必要接口做 API 形状修改；如果路由或 schema 有变化，同步 `make api-gen`。

## P0：租户隔离与敏感信息泄露

### 问题范围（事实）
- 广告主侧接口未按认证 scope：`HandleGetAdvertiser` / billing 多个接口 / 5 条 report 接口 / 5 条 creative 接口（含 list）均未做 owner check。
- `Advertiser.APIKey` 在 `internal/campaign/model.go:187-198` 被 `json:"api_key"` 序列化。
- `HandleCreateCreative` / `HandleUpdateCreative` / `HandleDeleteCreative` 按 creative id 直接操作；尤其 delete 可被任意广告主删他人创意，update 可写入他人 `ad_markup`（存储型 XSS 注入向量）。
- `HandleListCreatives`（`GET /api/v1/campaigns/{id}/creatives`）按 campaign id 直查，未校验 campaign 归属——**codex 在 V3 阶段指出的新增项**。
- `web/app/billing/page.tsx:33,34,125` 三处硬编码 `advertiser = 1`。

### 错误码一致性规则（本 section 及全文适用）

租户隔离相关的所有越权行为返回码遵循同一规则：

- **租户边界违规（A 的 key 访问 B 的资源）一律返回 `404`**。目的是不暴露目标资源是否存在。
- **admin token 缺失或错误返回 `403`**。该路径的资源位置语义上是公开的（谁都知道 `/admin/*` 存在），无需隐藏。
- **`POST /api/v1/billing/topup` body 中带 `advertiser_id` 字段**：服务端**静默忽略**，用 auth context 路由到认证用户本人账户，返回 200。不返回 400——"安静地拒绝危险输入"是主流产品的一致性做法。
- 同一路由在不同越权组合下的返回码必须**完全一致**（例如所有 report 路径都用 404，不允许 stats 用 403 而 hourly 用 404）。

所有单测和集成测试都必须对返回码精确断言，而不是"4xx 即可"。

### 必做改动

1. advertiser 接口
   - `GET /api/v1/advertisers/{id}`：path id 必须等于 `auth.AdvertiserIDFromContext(r.Context())`；不等则 404。
   - `Advertiser.APIKey` 字段 json tag 改为 `json:"-"`，任何读接口响应体不再包含 `api_key`。创建接口（`POST /api/v1/advertisers`）和 admin 审核通过接口（`internal/handler/admin.go:104-115`）是例外，创建时明文返回一次即可，类似 GitHub PAT 创建流程。
   - **先写测试**：A 的 key 调 `GET /advertisers/B.id` 期望 404；任何返回 advertiser 对象的读接口响应体反序列化后不得含 `api_key` 字段。

2. billing 接口
   - `POST /api/v1/billing/topup`：忽略 body 中的 `advertiser_id`（字段如存在按错误输入丢弃），用 auth context 路由。
   - `GET /api/v1/billing/transactions`：不接受 query 参数 `advertiser_id`，从 auth context 取。
   - `GET /api/v1/billing/balance/{id}`：owner check；path id ≠ auth id 返回 404。路由是否收敛成无参数的 `/balance` 由实现者自由选择，非必须。
   - **先写测试**：A 的 key 调 topup body 写 B.id，期望 200 且钱进 A；A 的 key 调 B 的 transactions / balance，期望 404。

3. reports 接口
   - 以下 5 条统一在进入 reporting 查询前先 `d.Store.GetCampaignForAdvertiser(r.Context(), campaignID, advID)`（参照 `HandleBidSimulate` 在 `report.go:207` 的写法），失败返回 404：
     - `/api/v1/reports/campaign/{id}/stats`
     - `/api/v1/reports/campaign/{id}/hourly`
     - `/api/v1/reports/campaign/{id}/geo`
     - `/api/v1/reports/campaign/{id}/bids`
     - `/api/v1/reports/campaign/{id}/attribution`
   - `simulate` 和 `export` 已有 owner check，保持一致。
   - **先写测试**：A 的 key 查 B 的 campaign 5 条报表，期望 5 × 404。

4. creative 接口（含 V3 新增的 list）
   - `GET /api/v1/campaigns/{id}/creatives`（`HandleListCreatives`）：先 `GetCampaignForAdvertiser(ctx, campaignID, advID)`，失败 404。
   - `POST /api/v1/creatives`（`HandleCreateCreative`）：按 body 中的 `campaign_id` 做 owner check，失败 404。
   - `PUT /api/v1/creatives/{id}`（`HandleUpdateCreative`）：先 `GetCreative(ctx, creativeID)` 取 `CampaignID`，再 owner check。
   - `DELETE /api/v1/creatives/{id}`（`HandleDeleteCreative`）：同上。
   - 这 4 个修复必须与 `docs/contracts/biz-engine.md` §1 规定的 creative CRUD `campaign:updates` 发布同批完成——create/update/delete 在修完 scope 后立即调 `bidder.NotifyCampaignUpdate(ctx, d.Redis, campaignID, "updated")`；list 是只读不需要 publish。
   - **先写测试**：A 的 key 调 B 的 list / create / update / delete，期望 4 × 404；合法的 A 操作触发 Redis pub/sub 通知。

5. 前端调用
   - 移除 `web/app/billing/page.tsx` 对 advertiser `1` 的 3 处硬编码。
   - `web/lib/api.ts` 的 billing 相关函数不再传 `advertiserId` 参数，由后端从 auth context 路由。
   - 本轮不对 `web/` 加单元测试（当前 `web/package.json` 无 test 脚本），前端改动由 P2 的手工 QA 和 `/browse` 截图验证。

### 测试要求（汇总）
- 两广告主 A/B；A 的 key 访问 B 的 advertiser / balance / transactions / report 全路径 / creative 全路径（含 list），全部 404。
- A 的 key 给 B 充值，钱进 A 的账户（body 中的 `advertiser_id` 被忽略）。
- 所有返回 advertiser 对象的接口响应体都不含 `api_key`。
- creative 合法 CRUD 触发 `campaign:updates`。

### 重点文件
- `internal/handler/campaign.go`（advertiser + creative handlers）
- `internal/handler/billing.go`
- `internal/handler/report.go`
- `internal/campaign/model.go`
- `internal/campaign/store.go`
- `web/lib/api.ts`
- `web/app/billing/page.tsx`

## P0：管理面与启动配置安全

### 问题范围（事实）
- `Config.Validate()` 在 `internal/config/config.go:68-76` 已定义，但 `cmd/api/main.go:46` 和 `cmd/bidder/main.go:45` 从未调用。
- `internal/handler/admin_auth.go:15-18` 无条件 fallback 到 `"admin-secret"`（不受 ENV 判断影响）。
- `internal/handler/admin_auth.go:26-29` 在 header 为空时接受 `?admin_token=` query 参数。

### 必做改动

1. 启动校验
   - `cmd/api/main.go` 与 `cmd/bidder/main.go` 在 `cfg := config.Load()` 后**必须执行** `cfg.Validate()`，且校验失败时进程必须退出（`log.Fatal` / `os.Exit` / panic 均可，目标是启动失败而非实现形态）。
   - `Validate()` 在生产环境必须对 `BIDDER_HMAC_SECRET` / `ADMIN_TOKEN` 缺失失败；对 `CORS_ALLOWED_ORIGINS` 的校验强度由实现者定，但至少在生产环境不得允许通配默认。
   - **先写测试**：`internal/config/config_test.go` 覆盖"ENV=production 缺 `BIDDER_HMAC_SECRET` / `ADMIN_TOKEN` 时 `Validate()` 报错"。

2. admin 鉴权
   - **删除整个 `"admin-secret"` fallback**。不使用 ENV guard。开发者通过 `.env` 或 shell 显式设置 `ADMIN_TOKEN`，没有"偷懒默认值"选项。
   - 删除 `admin_token` query 参数支持，只保留 `X-Admin-Token` header。
   - **先写测试**：(a) 正确 header → 200；(b) 正确 query 无 header → 401；(c) 无 token → 401；(d) `ADMIN_TOKEN` 未设置时启动期被 `Validate()` 拦截。

3. 默认值策略
   - `BIDDER_HMAC_SECRET`、`ADMIN_TOKEN`、`CORS_ALLOWED_ORIGINS` 哪些仅用于本地开发，在 `config.go` 对应字段上加注释。
   - `docs/contracts/biz-engine.md` 或一个独立的 `docs/runtime.md` 中同步记录"生产必须设置的 secret 清单"。

### 测试要求
- 生产环境缺失关键 secret → 启动失败
- admin header 通过 / query 失败 / 无 token 失败
- 默认 token 在任何环境都不生效

### 重点文件
- `internal/config/config.go`
- `internal/handler/admin_auth.go`
- `cmd/api/main.go`
- `cmd/bidder/main.go`
- `internal/config/config_test.go`（新建或扩充）
- `internal/handler/admin_auth_test.go`（新建或扩充）

## P1：事件语义、计费与报表口径

### 问题范围（事实）
- `cmd/bidder/main.go:419-420` `handleWin` 同时 `go producer.SendWin(bgCtx, evt)` 和 `go producer.SendImpression(bgCtx, evt)`，赢标被等同于真实曝光。
- `internal/reporting` / `internal/autopause` / `internal/reconciliation` 当前以 `event_type='impression'` 为曝光口径。
- `cmd/bidder/main.go:426-483` `handleClick` 无 `SetNX` 去重；`handleWin` 在 L320-329 已有 `win:dedup:{request_id}` 去重模式可复制。
- `cmd/bidder/main.go:467` `SendClick(r.Context(), ...)` 和 `:500` `SendConversion(r.Context(), ...)` 用会被取消的 request context；`handleWin` L418-420 已使用 `bgCtx := context.Background()` 并注释原因。

### 迁移原则（**关键约束**）

**不能单 commit 完成**。**尤其不能在 bidder 仍双写 `win+impression` 的情况下，把下游聚合简单改为 `event_type IN ('win','impression')`**——这是一个典型的双计陷阱，每个赢标在 bid_log 已有两条行（win + impression），OR 聚合会让曝光数量瞬间翻倍，CTR 腰斩，autopause 阈值失效。迁移必须保证"当前双写""停写""未来单写"三种状态下指标都稳定。

### 建议迁移顺序

**Step A：先引入去重后的"有效曝光"口径**（独立 commit）

在 reporting / autopause / reconciliation 所有曝光聚合处改为按 `request_id` 去重的方式计数：

```sql
-- ClickHouse
countDistinctIf(request_id, event_type IN ('win', 'impression'))
```

或其他等价方案（`uniqExactIf`、在 Go 层去重后再聚合等）。此方案的不变量：

- 当前双写状态：同一次赢标虽然写了两行，但 `request_id` 相同 → 只计一次。
- 未来停止伪造 impression 之后：只有 `win` 一行，`request_id` 仍只计一次。
- 未来如果引入独立曝光回调：真实 impression 和 win 可以共存，`request_id` 仍只计一次（按"每次赢标记一次曝光"的语义）。

Step A **不动 bidder 回调**，只改下游聚合查询。Step A 合入后观察一个完整报表周期（至少一小时，最好一天），确认无指标跳变。

**Step B：停止伪造 impression**（独立 commit，必须在 Step A 合入并稳定后才启动）

- 删除 `handleWin` 中的 `SendImpression` 调用。
- `bid_log` 从此不再新增 `event_type='impression'` 行。历史数据不动。
- 由于 Step A 已经用 `request_id` 去重，Step B 落地当天指标平滑过渡，无跳变。
- 如未来需要区分"真正的曝光"，引入独立的曝光回调路径（像素、SDK 事件等）写 `impression`，由 Step A 的口径天然兼容。**本轮不做**该扩展。

**Step C：修 click/convert 的一致性**（可以和 Step A 或 Step B 合批，也可以独立）

- `handleClick` 增加 `click:dedup:{request_id}` + `SetNX` 去重，TTL 与 `win:dedup` 一致（当前 5 分钟）。CPC 计费依赖 click 次数，去重不到位会重复扣费。
- `SendClick` 和 `SendConversion` 的 context 从 `r.Context()` 改为 `context.Background()`（或带 timeout 的后台 context），参照 handleWin L418-420 的既有写法。

### 结果要求
- 迁移前、Step A 合入后、Step B 合入后三个时点的 CTR、曝光、花费指标不得出现人为跳变。
- Step B 合入后 `bid_log` 不再产生新的 `event_type='impression'` 行。
- 同一 `request_id` 的 click 多次回调只扣一次费。
- handler 返回后 click/convert 事件仍稳定写入 Kafka 或本地 disk buffer。

### 测试要求
- 双写状态下，注入 10 个不同 `request_id` 的 win 事件（bidder 会各写一个 `win` + 一个 `impression`），reporting 有效曝光返回 10。
- Step B 后注入 10 个 win 事件，有效曝光返回 10（和 Step A 等值）。
- 同一 `request_id` 连续 3 次 click，预算只扣一次、`bid_log` 只写一条 click 行。
- handler 返回后立即 cancel r.Context()，producer 仍能写入 mock buffer。

### 重点文件
- `cmd/bidder/main.go`
- `internal/events/producer.go`
- `internal/reporting/store.go`
- `internal/autopause/service.go`
- `internal/reconciliation/reconciliation.go`
- `docs/contracts/biz-engine.md`（聚合口径文档）

## P1：生命周期与优雅关闭

### 问题范围（事实）
- `cmd/api/main.go:47` `ctx := context.Background()`；`cmd/bidder/main.go:46` 同。
- `autopause.Start` / `statsCache.Start` / `loader.Start` / `reconciliation.StartHourlySchedule` 都接收这个永不取消的 ctx。
- shutdown 用独立 channel（`main.go:219` / `:167` `signal.Notify(quit, ...)`），只停 HTTP server，不 cancel 根 ctx——后台 goroutine 在 HTTP 停服期间仍继续读写 Redis / Kafka / ClickHouse。

### 必做改动（只规定行为，不规定代码结构）

1. 根 context 可取消
   - API 和 bidder 主进程使用可 cancel 的根 context，能在收到 SIGINT / SIGTERM 时自动传播取消信号。具体实现（`signal.NotifyContext`、手写 `context.WithCancel` + `signal.Notify`、或其他）由实现者定。
   - 所有长期后台任务绑定到这一根 context，loop 内必须正确处理 `<-ctx.Done()` 并退出。需要逐个 verify：
     - `internal/autopause/service.go`
     - `internal/bidder/statscache.go`
     - `internal/bidder/loader.go`（含 pub/sub subscribe 的 goroutine）
     - `internal/reconciliation/reconciliation.go`

2. 关闭依赖顺序（**行为约束，不限定代码形态**）

   收到退出信号后的行为必须满足以下**先后关系**：

   ```
   Step 1: 根 ctx cancel  →  后台 worker 开始退出
   Step 2: HTTP server 停止接收新请求、drain inflight
   Step 3: Kafka producer 关闭并 flush 本地 buffer
   Step 4: Redis / ClickHouse / Postgres 等底层资源关闭
   ```

   每一步的理由（why，供实现者判断边界情况）：

   - **Step 1 必须先于 Step 2**：HTTP drain 期间可能还有请求在跑，如果后台 worker（autopause、reconciliation）仍在运行，它们可能和 inflight 请求竞争相同的 DB 行或 Redis 计数器，产生不受控的写。先取消后台是为了让 HTTP drain 在一个"单写者"状态下完成。
   - **Step 2 必须先于 Step 3**：HTTP drain 期间还可能产生 Kafka 事件（click/convert 回调等），如果 producer 先关闭，这些事件会丢。先排空 HTTP 再关 producer 保证没有正在飞的事件。
   - **Step 3 必须先于 Step 4**：producer 关闭时可能需要本地 disk buffer 或临时文件落盘（见 `events/producer.go:23-27`）。这些操作不依赖 Redis / CH / DB，但依赖本地文件系统；所以 producer 先关没问题。反过来如果 DB 先关、producer 还在 flush 失败事件，可能写入失败。
   - 每个 Step 都需要各自的 timeout 保护（例如 shutdown 总超时 30 秒，分配到 4 步）。

   任何满足上述先后关系的代码实现都可接受，不规定必须用 context-chain、defer-stack 还是别的结构。

3. fail-open / fail-closed 显式化
   - Redis、Kafka、ClickHouse 的失败策略在 `docs/runtime.md`（新建或补入契约文档）写明：
     - 出价热路径 Redis 不可用：fail-open 还是 fail-closed？
     - Kafka 全量不可用：disk buffer 兜底行为；buffer 满时策略？
     - ClickHouse 不可用时 reporting 降级到？

### 测试要求
- `cmd/api/main_test.go` / `cmd/bidder/main_test.go`：启动进程 → 发 SIGTERM → 5 秒内退出 → 期间无新的 bid_log / Redis 写入（可用 counter mock 或查 ClickHouse 行数差值）。
- 每个后台 loop 单测：`ctx, cancel := context.WithCancel(background)` → 启动 → `cancel()` → 100ms 内 goroutine 退出。

### 重点文件
- `cmd/api/main.go`
- `cmd/bidder/main.go`
- `internal/autopause/service.go`
- `internal/bidder/statscache.go`
- `internal/bidder/loader.go`
- `internal/reconciliation/reconciliation.go`

## P2：端到端回归保护

### 目标
只保留跨 handler / 跨进程的回归保护，防止未来新增接口再次漏掉 scope 或 shutdown 约束。P0/P1 的所有单点单测已并入各自的 TDD 步骤。

### 必做改动

1. **租户隔离集成测试**（`internal/handler/tenant_isolation_test.go` 或 `test/integration/tenant_test.go`）
   - 在 `httptest.NewServer` 里起完整 api handler tree。
   - 创建两个 advertiser A / B，获取各自 API key。
   - 穷举 A 访问 B 的所有路径（按 P0 §4 错误码规则全部期望 404）：
     - advertiser GET
     - billing balance / transactions / topup（后者忽略 body id）
     - campaign GET / PUT / start / pause / delete
     - creative list / create / update / delete
     - report 5 条路径
   - 穷举特别关注 body-based 越权（topup body 里写 B.id、creative POST body 里写 B 的 campaign_id）。
   - 循环覆盖所有路由 × 所有越权组合——这是防"以后新增 handler 漏 scope"的主要护栏。

2. **响应体敏感字段扫描**
   - 对所有返回 advertiser 对象的路径，断言 response body JSON 反序列化后不含 `api_key` key。
   - 可用反射遍历或简单字符串包含检查实现。

3. **shutdown 集成测试**
   - 启动完整 bidder / api → 发 SIGTERM → 验证 5 秒内进程退出。
   - shutdown 期间观察 bid_log / Redis 无新写入（整个 shutdown window 的 COUNT 差值应为 0 或只包含 inflight 排空的合理值）。

4. **前端手工 QA**（因为 `web/` 暂无单测框架）
   - billing 页面登录后查看余额、充值、查交易记录（配合 P0 的前端 scope 修复）。
   - campaign / report / creatives 页面验证 scope 正常，确认前端无任何硬编码 advertiser 假设残留。
   - 全程对照 `DESIGN.md` 做视觉合规检查。

### 验收标准
- 新增集成测试能稳定阻断本次评审的所有 P0 问题，且一旦新 handler 漏 scope 检查会立刻 red。
- `go test ./... -count=1`（去掉 `-short` 让集成测试跑）通过。
- 前端手工 QA 的所有路径均正常，无 advertiser 越权或 `api_key` 回显。

## 建议执行顺序

1. **P0 批次 1**：P0 租户隔离与 sensitive data（含 `HandleListCreatives`、creative CRUD 的 scope + pub/sub 同批）。
2. **P0 批次 2**：P0 admin/config 安全。
3. **P1 批次 3**：P1 lifecycle（根 context + shutdown 4 步 + fail-open/closed 文档）。
4. **P1 批次 4**：P1 event semantics Step A（有效曝光口径，不动 bidder 回调）。
5. **P1 批次 5**：P1 event semantics Step B + Step C（停止伪造 impression + click 去重 + context 修正）。Step B 必须在 Step A 观察稳定之后才启动。
6. **P2 批次 6**：P2 端到端回归保护。

每批次独立提交 PR、独立过 code review / verification / QA 循环。不要跨批次累积未验证改动。

## 建议验证命令

```powershell
go test ./... -short -count=1
go test ./... -count=1     # 包含集成测试
cd web && npm run lint
```

路由或 schema 变化：

```powershell
make api-gen
```

涉及完整链路或前端行为：

```powershell
./scripts/test-env.sh verify
```

## 观察项（本轮不修，但需跟踪）

- **analytics SSE 通过 `?api_key=` query 参数认证**（`internal/handler/middleware.go:13-16`）：和 admin_token 同类 URL 泄露面，会进 access log / 反向代理日志 / 浏览器 history。替代方案可选：短期 session token、WebSocket、服务端生成的一次性 stream key。在 P0 admin 安全整改完成后单独开一轮评估，因为前端 `EventSource` API 确实无法设自定义 header，方案选择有权衡。
- **`dsp.billing` topic 仍是 produced but unconsumed**（`internal/events/producer.go:57`、详见 `docs/contracts/biz-engine.md` §3.1）：P1 事件语义整改时一并确认是否保留 `SendBilling` 调用。
- **`SendLoss` 定义但未调用**（`internal/events/producer.go:113-117`）：如果最终决定不埋 loss 事件，可删除该方法；如果要埋，从 `handleBid` 的过滤分支注入。
- **API key 级 rate limiting**：`internal/ratelimit` 当前配额维度需单独 verify。若全局限流，单个泄露 key 没有额外防护，scope 检查是唯一防线。
- **bid_log 6 个月 TTL**：`migrations/002_clickhouse.sql:21`。reporting 和 reconciliation 脚本必须处理超过 TTL 的数据被清理的情况，边界健壮性需 verify。
- **广告主侧敏感操作审计**：topup / pause / delete creative 当前无 audit trail（admin 侧有）。事故溯源时缺证据。

## 交付要求

- 每批次独立 PR，避免把所有问题揉成一个超大补丁。
- 每批 PR 描述说明：
  - 修了哪些风险
  - 改了哪些接口或行为（含返回码变化）
  - 新增的测试列表
  - 延后项及理由
- Step A 和 Step B（事件语义）在 PR 描述里必须明确标注"不得与下一步合并"的约束，防止 reviewer 误合。
