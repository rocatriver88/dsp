# 评审整改任务单（2026-04-14）

## 目的
本任务单基于 2026-04-14 对当前仓库的设计与实现评审结果整理，目标是把高风险问题先收口，再推进数据语义和运行时一致性修正。本文档面向执行修改的工程代理，要求按优先级落地代码、补测试、给出验证结果。

## 复核说明（2026-04-14 二次）
原评审的 14 项事实断言经代码逐条核实，**13 项 TRUE，1 项 PARTIAL**（P0 报表接口一节：`HandleBidSimulate` 已做 ownership 检查，其余 5 条未做）；无任何一项 STALE——近期 commit 未触及其中任一问题。评审事实层面可信。

本次复核新增或修订的条款（已合入下文，以便后续执行无需在两份文档间切换）：

- **P0 新增"创意 handler scope"子项**：`HandleCreateCreative` / `HandleUpdateCreative` / `HandleDeleteCreative` 三个 handler 完全无 ownership 校验。`HandleDeleteCreative` 接收 `creativeID` 直接删除，任意广告主可删他人创意；`HandleUpdateCreative` 可篡改他人 `ad_markup`（存储型 XSS 注入向量）；`HandleCreateCreative` 可把创意塞到他人的 campaign 下。严重性等同原 P0 各项，必须同批修复。
- **P1 win/impression 改为两步迁移**：直接停写 `impression` 会使依赖 `event_type='impression'` 聚合的 `internal/reporting` / `internal/autopause` / `internal/reconciliation` 立刻失效（CTR 分母归零、autopause 阈值失效、对账误差跳变），必须先放宽聚合条件再停写。
- **P1 lifecycle 补出 shutdown 四步排序**：仅 cancel 根 ctx 不够，需要明确 HTTP 排空、Kafka flush、存储关闭的先后，否则会丢事件或产生无主后台写入。
- **测试同步化**：原 P2 的"handler 授权测试""回调测试"条款吸收回各 P0/P1 子项的必做改动，遵循 TDD 先写测试再修代码。P2 只保留"端到端租户隔离集成测试"这种加强型的交叉覆盖。
- **`admin-secret` 默认值直接删除**：不用 ENV guard。保留 fallback 是新人把默认值复制到生产环境的单点隐患。
- **`api_key` 采用 `json:"-"`**：不引入 DTO。只有在"管理员回显 / 广告主屏蔽"两种形态同时存在时 DTO 才值得，现在没有这种分叉。
- **删除原执行原则第 2 条**（本轮不涉及生成产物），新增"观察项"段落列出次级但值得跟踪的债务。

## 执行原则
- 先处理安全边界，再处理数据语义，再处理运行稳定性。
- 每一项整改遵循 TDD：先写一个能复现问题的测试（越权应被拒、重复应被去重、ctx 取消后仍写入等），验证当前 red，再落地修复，验证 green。
- 优先做最小可验证修复，不把本轮整改扩展成大规模重构。当"最小修复"和"破坏现有语义"冲突时（如 P1 win/impression），必须拆成可独立验证的多步迁移，每步都能独立上线。

## P0：租户越权与密钥泄露

### 问题
- 广告主侧部分接口没有按当前认证广告主做 scope。
- `Advertiser` 模型直接序列化 `api_key`，存在敏感信息泄露（`internal/campaign/model.go:187-198` 的 `APIKey string \`json:”api_key”\``）。
- 账务接口信任调用方传入的 `advertiser_id`（`internal/handler/billing.go:20-91`）。
- 报表接口大多直接按 `campaign_id` 查 ClickHouse，没有先校验 campaign 归属（`internal/handler/report.go` 5/6 handler；只有 `HandleBidSimulate` 做了 owner check）。
- **创意 handler 完全无 ownership 校验**：`HandleCreateCreative` / `HandleUpdateCreative` / `HandleDeleteCreative` 按 creativeID 直接操作；特别是 delete 可被任意广告主删他人创意，update 可篡改他人 `ad_markup`（存储型 XSS 注入向量）。
- 前端有硬编码 advertiser ID 的调用（`web/app/billing/page.tsx:33,34,125` 对 advertiser `1` 的三处调用），当前租户边界依赖前端自觉。

### 必做改动
1. 收紧 advertiser 查询
- 将 `GET /api/v1/advertisers/{id}` 改为只允许访问当前认证广告主本人：对比 path id 与 `auth.AdvertiserIDFromContext(r.Context())`，不一致返回 403。
- 在 `Advertiser` struct 的 `APIKey` 字段上改为 `json:”-”`，任何读接口都不再回传。不引入新 DTO（避免无收益的代码扩散）。
- **先写测试**：两广告主 A/B，A 的 key 调 `GET /advertisers/B.id` 必须 403；任何返回 advertiser 对象的接口响应体都不得包含 `api_key` 字段（可用 JSON 反序列化断言）。

2. 收紧 billing 接口
- `POST /api/v1/billing/topup`：忽略 body 中的 `advertiser_id`，强制使用认证上下文中的 advertiser。如果 body 仍要保留字段，服务端直接覆盖。
- `GET /api/v1/billing/transactions`：不再接受 query 参数 `advertiser_id`，从认证上下文取。
- `GET /api/v1/billing/balance/{id}`：将路由改为 `GET /api/v1/billing/balance`，从认证上下文取 ID；或保留现有路由但强制 path id 必须等于 auth id。推荐前者，更符合 REST 语义。
- **先写测试**：A 的 key 充值 body 里写 B.id，期望最终入账到 A；A 的 key 查 B 的 transactions / balance，期望 403。

3. 收紧 reports 接口
- 以下 5 个接口都要在查询报表前先调用 `d.Store.GetCampaignForAdvertiser(r.Context(), campaignID, advID)`，参照 `HandleBidSimulate` 的既有写法（`report.go:207`）：
  - `/api/v1/reports/campaign/{id}/stats`
  - `/api/v1/reports/campaign/{id}/hourly`
  - `/api/v1/reports/campaign/{id}/geo`
  - `/api/v1/reports/campaign/{id}/bids`
  - `/api/v1/reports/campaign/{id}/attribution`
- 保持 `simulate`、`export` 现有的 ownership 校验风格一致。
- **先写测试**：A 的 key 查 B 的 campaign 报表，期望 403（所有 5 个路径）。

4. 收紧 creative handler
- `HandleCreateCreative`：接收到 `campaign_id` 后先 `GetCampaignForAdvertiser(ctx, campaignID, advID)` 校验所有权；校验失败返回 403。
- `HandleUpdateCreative`：先 `GetCreative(ctx, creativeID)` 取 `CampaignID`，再 `GetCampaignForAdvertiser` 校验；校验失败返回 403。`HandleDeleteCreative` 同理。
- 这一轮修完后，契约 `docs/contracts/biz-engine.md` §1 里列出的 creative CRUD pub/sub 发布也必须同时落地（都在这三个 handler 里完成）。
- **先写测试**：A 的 key 尝试创建/修改/删除 B 的 campaign 下的 creative，期望 403；合法的 A 操作自己的 creative，期望 200 并触发 `campaign:updates` 通知。

5. 修正前端错误假设
- 去掉 `web/app/billing/page.tsx` 对 advertiser `1` 的硬编码（3 处）。
- 前端对 advertiser 和 campaign 数据的读取逻辑要建立在”后端强制 scope”上，调用 `/billing/balance` 和 `/billing/transactions` 不再传 id，由后端从 auth context 取。
- 相应更新 `web/lib/api.ts` 的函数签名（删掉 `advertiserId` 参数）。
- **先写测试**：`web/` 目前无 test 脚本（`web/package.json` 无 test 命令），本轮不强求前端单测；但前端改完必须启动 web 在浏览器里跑一遍登录 → 查看余额 → 充值 → 查 transactions 全链路（算入 P2 的端到端 QA 范围）。

### 重点文件
- `internal/handler/campaign.go`（advertiser + creative handler）
- `internal/handler/billing.go`
- `internal/handler/report.go`
- `internal/campaign/model.go`
- `internal/campaign/store.go`
- `web/lib/api.ts`
- `web/app/billing/page.tsx`
- `web/app/campaigns/[id]/page.tsx`
- `web/app/reports/[id]/page.tsx`

### 验收标准
- 任意广告主 API Key 只能访问自己的 advertiser、campaign、billing、report、creative 数据。
- 任意广告主 API Key 无法读取其他 advertiser 的余额、流水、报表；无法创建/修改/删除他人的 creative。
- 任意广告主 API Key 无法从任何读接口拿到 `api_key` 字段。
- 前端不再依赖硬编码 advertiser ID。
- 所有上述”先写测试”条款对应的 handler 单测已落盘并通过。

## P0：管理面和生产配置安全

### 问题
- `Config.Validate()` 已实现（`internal/config/config.go:68-76`），但 `cmd/api/main.go:46` 和 `cmd/bidder/main.go:45` 都只调 `config.Load()`，从未调 `Validate()`。
- admin 鉴权（`internal/handler/admin_auth.go:15-18`）在 `ADMIN_TOKEN` 未设置时**无条件回退到 `"admin-secret"`**，且该回退不依赖 `ENV` 判断。
- admin 鉴权（`internal/handler/admin_auth.go:26-29`）接受 query 参数 `admin_token`，容易泄露到 access log、反向代理日志、浏览器 referrer。

### 必做改动
1. 启动时强制配置校验
- 在 `cmd/api/main.go` 和 `cmd/bidder/main.go` 的 `cfg := config.Load()` 后立刻调用 `if err := cfg.Validate(); err != nil { log.Fatalf(...) }`。
- **先写测试**：`internal/config/config_test.go` 覆盖"生产环境缺失 `BIDDER_HMAC_SECRET` / `ADMIN_TOKEN` 时 `Validate()` 返回错误"。

2. 收紧 admin 鉴权
- 删除 `admin_token` query 参数支持，只接受 `X-Admin-Token` header。
- **删掉整个 `"admin-secret"` 默认值 fallback**：`ADMIN_TOKEN` 未设置时直接 panic 或由 `config.Validate()` 拦截。**不使用 ENV guard 保留开发默认值**——保留 fallback 是一处新人把默认值复制到生产环境的单点隐患，且依赖 ENV 被正确设置本身就是一个脆弱假设。开发者在本地 `.env` 里加一行 `ADMIN_TOKEN=dev-<some-string>` 的成本是零。
- **先写测试**：`internal/handler/admin_auth_test.go` 覆盖：(a) header 带正确 token → 200；(b) query 带正确 token → 401（即使 header 未设）；(c) 无 token → 401；(d) `ADMIN_TOKEN` 未设置时启动阶段被 `Validate()` 拦截（和 #1 同测试）。

3. 复核默认值策略
- 检查 `BIDDER_HMAC_SECRET`、`ADMIN_TOKEN`、`CORS_ALLOWED_ORIGINS` 的开发/生产行为，用 `Validate()` 统一把关。
- 明确哪些默认值只能用于本地开发，并在 `config.go` 对应字段上加注释。

### 重点文件
- `internal/config/config.go`
- `internal/handler/admin_auth.go`
- `cmd/api/main.go`
- `cmd/bidder/main.go`
- `internal/config/config_test.go`（新建或补充）
- `internal/handler/admin_auth_test.go`（新建或补充）

### 验收标准
- 生产环境缺少关键 secret 时服务启动失败（log.Fatalf 或 panic）。
- admin token 不再能通过 URL 传递。
- 默认 token 在任何环境都不生效，包括开发环境。
- 上述测试全部落盘并通过。

## P1：事件语义与计费一致性

### 问题
- `handleWin` (`cmd/bidder/main.go:419-420`) 当前同时发送 `win` 和 `impression` 事件，把赢标等同于真实曝光。
- `internal/reporting`、`internal/autopause`、`internal/reconciliation` 当前都以 `event_type='impression'` 聚合 CTR、spend、autopause 阈值——这意味着"停写 impression"会立刻让这些指标归零。
- CPC 点击链路没有去重（`cmd/bidder/main.go:426-483` 的 `handleClick` 内零 `SetNX` 逻辑），可能重复扣费。`handleWin` 在 L320-329 已有去重模式可直接复用。
- `click`/`convert` 的 Kafka 发送使用 `r.Context()`（`cmd/bidder/main.go:467`、`:500`），handler 返回后 context 取消，可能导致事件丢失。注意：`handleWin` 的 L418-420 已用 `bgCtx := context.Background()` 并注释说明原因——修复方案就是把这个模式复制到 click/convert。

### 必做改动
1. 修正 win/impression 语义（**两步迁移，不能单 commit 完成**）

   **Step 1A：放宽下游聚合口径**（先合，单独 commit）
   - 在 `internal/reporting/store.go` 中所有按 `event_type='impression'` 聚合的 SQL 改为 `event_type IN ('win','impression')`。
   - `internal/autopause/service.go` 的 impression 阈值计算同上。
   - `internal/reconciliation/reconciliation.go` 的 spend / 花费对账同上。
   - 同时保留 bidder 的双写（`SendWin` + `SendImpression`），这一步对线上数据语义零侵入，仅是"两种事件都算曝光"。
   - **先写测试**：注入 10 条 `win` + 0 条 `impression`，reporting 查询返回曝光=10。
   - 在 bid_log 有一个完整小时的新数据之后，进入 Step 1B。

   **Step 1B：停止伪造曝光**（下一个 commit，不和 1A 合成一批）
   - 删除 `handleWin` 里的 `SendImpression` 调用；`win` 只表示赢标成交。
   - 如果仍需要区分"真实曝光"，需要引入独立的曝光回调路径（像素或 SDK 事件），**本轮不做**。
   - 把 Step 1A 放宽的聚合条件收回 `event_type='impression'`——**或者**（更干净）保留 `IN ('win','impression')` 的 OR 条件，语义变成"赢标即记曝光"。推荐后者，少一次 SQL 迁移。
   - **先写测试**：win 事件后，bid_log 里 `event_type='win'` 一条，`event_type='impression'` 零条；reporting 查询结果与 Step 1A 语义一致。

   > **不要把 Step 1A 和 Step 1B 合成一批**。如果先停写 impression 再改聚合，线上 CTR / spend / autopause 会在两个 commit 之间的窗口里全部失真。

2. 补齐点击去重
- 参照 `handleWin` 的 `dedupKey = fmt.Sprintf("win:dedup:%s", requestID)` + `SetNX` TTL 5 分钟的模式，为 `handleClick` 加 `click:dedup:{request_id}` 去重键。CPC 计费依赖 click 次数，重复回调会重复扣费。
- 明确去重维度：同一 `request_id` 的 click 回调，5 分钟内只计一次。
- **先写测试**：对同一 `request_id` 连续发 3 次 click 回调，预算只扣一次，bid_log 只写一条 `event_type='click'`。

3. 修正异步发送上下文
- `cmd/bidder/main.go:467` 的 `producer.SendClick(r.Context(), ...)` 改为 `producer.SendClick(context.Background(), ...)`，原因见 handleWin L418-420 的注释。
- `cmd/bidder/main.go:500` 的 `producer.SendConversion` 同样修正。
- 如果需要超时保护，用 `ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)` 包一层。
- **先写测试**：handler 返回后立刻 cancel r.Context()，验证 Kafka producer 仍能写入缓冲（可用 mock producer 计数）。

4. 校准下游聚合
- 和 Step 1A 一起做。除了 reporting / autopause / reconciliation，还要检查 `internal/billing` 对 click 计费、attribution 查询的事件依赖。
- 在 `docs/contracts/biz-engine.md` §2 的"ClickHouse `bid_log` 表"小节下加一行说明聚合惯例："曝光统计口径 = win OR impression"（或最终敲定的方案）。

### 重点文件
- `cmd/bidder/main.go`
- `internal/events/producer.go`
- `internal/reporting/store.go`
- `internal/autopause/service.go`
- `internal/reconciliation/reconciliation.go`
- `docs/contracts/biz-engine.md`（聚合口径文档）

### 验收标准
- Step 1A 合入后，reporting 指标未出现跳变。
- Step 1B 合入后，`win` 不再产生 `impression` 事件行；`event_type='impression'` 在 bid_log 新增行数归零。
- 重复 click 回调不会重复扣费。
- handler 返回后 click/convert 事件仍能稳定写入 Kafka 或本地缓冲。
- 报表中的曝光、点击、赢标、花费口径一致且可解释。

## P1：运行时生命周期与关闭行为

### 问题
- 长生命周期 goroutine 由 `context.Background()` 驱动（`cmd/api/main.go:47`、`cmd/bidder/main.go:46`），关闭服务时不会统一 cancel。
- `autopause.Start` (`cmd/api/main.go:102`)、`statsCache.Start` (`cmd/bidder/main.go:90`)、`loader.Start` (`cmd/bidder/main.go:112`)、`reconciliation.StartHourlySchedule` (`cmd/api/main.go:107`) 都接收这个永不取消的 ctx。
- 当前 shutdown 使用独立 channel `signal.Notify(quit, ...)`（`main.go:219` 和 `:167`），但只关 HTTP server，不 cancel 根 ctx——后台 goroutine 在 HTTP 停服期间仍继续读写 Redis / Kafka / ClickHouse。

### 必做改动
1. 统一根 context
- `cmd/api/main.go` 和 `cmd/bidder/main.go` 的入口改为：
  ```go
  rootCtx, rootCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
  defer rootCancel()
  ```
- 所有后台服务的 `Start(ctx)` 调用位点从 `context.Background()` 改为 `rootCtx`。
- 检查每个被启动的后台 loop 确实在 for-select 里处理了 `<-ctx.Done()` 并正确退出——**这一步需要逐个 verify**，不能假设：
  - `internal/autopause/service.go`
  - `internal/bidder/statscache.go`
  - `internal/bidder/loader.go`（包括 pub/sub subscribe 的 goroutine）
  - `internal/reconciliation/reconciliation.go`
- **先写测试**：用 `context.WithCancel` 启动一个 loop，cancel 后 100ms 内 goroutine 应退出（可用 `runtime.NumGoroutine()` 对比或 channel 信号）。

2. 统一关闭流程（**严格按以下顺序**）

   ```
   signal 到达 → rootCtx 自动 cancel
     └─ Step A: 后台 goroutine 收到 Done，开始退出（并行，不 block）
     └─ Step B: srv.Shutdown(shutdownCtx)  排空 HTTP inflight 请求
     └─ Step C: producer.Close()           flush Kafka 本地缓冲到 broker 或 disk buffer
     └─ Step D: redis.Close() / clickhouse.Close() / db.Close()
   ```

   关键约束：
   - Step A 必须先于 Step B 开始，否则 HTTP drain 期间后台 worker 还可能写数据。
   - Step B 必须先于 Step C——HTTP handler 可能在排空时仍产生 Kafka 事件，producer 关闭早了会丢。
   - Step C 必须先于 Step D——Kafka producer 关闭期间若依赖 disk buffer，buffer 路径涉及本地文件系统（见 `events/producer.go:23-27`），不依赖 Redis/CH/DB，安全。
   - 每个 Step 给一个独立的 `context.WithTimeout(...)` 作为超时上限。

3. 明确 fail-open/fail-closed
- 对 Redis、Kafka、ClickHouse 的失败策略形成一致规则，并补 `docs/contracts/biz-engine.md` 或单独的 `docs/runtime.md`。
- 至少覆盖：出价热路径 Redis 不可用时（fail-open 还是 fail-closed？），Kafka 全量不可用时（走 disk buffer，但 buffer 满时行为？），ClickHouse 不可用时（reporting 降级到什么？）。

### 重点文件
- `cmd/api/main.go`
- `cmd/bidder/main.go`
- `internal/autopause/service.go`
- `internal/bidder/statscache.go`
- `internal/bidder/loader.go`
- `internal/reconciliation/reconciliation.go`

### 验收标准
- 服务收到退出信号后，所有后台 worker 在 5 秒内停止。
- 不再依赖 `context.Background()` 驱动关键长期任务。
- 关闭期间不会继续产生不受控写操作（可用集成测试观察 Redis / bid_log 在 shutdown 后无新写入）。
- 上述测试全部落盘并通过。

## P2：端到端测试加强（交叉覆盖型）

### 方针
P0 / P1 的所有单元测试已吸收到各自的"必做改动"里按 TDD 同步写，本节只保留"跨 handler / 跨进程"的交叉覆盖测试——用来防范未来新增 handler 再漏 scope 校验的回归。

### 必做改动
1. **端到端租户隔离集成测试**（新建 `internal/handler/tenant_isolation_test.go` 或独立的 `test/integration/tenant_test.go`）
   - 在 `httptest.NewServer` 里启动完整 api handler tree
   - 创建两个 advertiser A / B，分别获取 API key
   - 穷尽地用 A 的 key 访问 B 名下的所有资源：
     - `GET /advertisers/{B.id}` 期望 403
     - `GET /billing/balance/{B.id}` / `/billing/balance`（auth=A）期望只看到 A 的余额
     - `GET /billing/transactions?advertiser_id=B.id` 期望 403 或只看到 A
     - `POST /billing/topup` body 里写 `advertiser_id: B.id`，期望钱进 A
     - `GET /campaigns/{B.campaignID}` 期望 404 或 403
     - `PUT /campaigns/{B.campaignID}` 期望 403
     - `POST /campaigns/{B.campaignID}/start` / `/pause` 期望 403
     - `GET /reports/campaign/{B.campaignID}/stats` 和其他 4 个 report 路径期望 403
     - `POST /creatives` 里写 `campaign_id: B.campaignID` 期望 403
     - `PUT /creatives/{B.creativeID}` / `DELETE` 期望 403
   - 循环所有路由、所有越权组合；这是防回归的核心测试

2. **响应体 `api_key` 不泄露扫描**
   - 对每个返回 advertiser 对象的路径，断言 response body JSON 反序列化后不包含 `api_key` key
   - 可用反射或字符串 grep 的简单实现

3. **shutdown 行为集成测试**
   - 启动完整 bidder/api，触发 SIGTERM，验证 5 秒内进程退出
   - shutdown 期间观察 bid_log / Redis 无新写入

### 重点文件
- `internal/handler/tenant_isolation_test.go`（新建）
- `cmd/bidder/main_test.go`（扩充 shutdown 测试）
- `cmd/api/main_test.go`（扩充 shutdown 测试）

### 验收标准
- 新增集成测试能稳定复现并阻断本次评审发现的所有 P0 问题，且一旦新 handler 漏 scope 检查会立刻 red。
- `go test ./... -count=1` 继续通过（注意去掉 `-short` 让集成测试跑起来，或单独起 integration tag）。

## 建议执行顺序
1. **P0 批次 1**：tenant scope（advertiser / billing / report / creative handler + `api_key` 隐藏 + 前端硬编码清理）。每项 TDD：先写越权测试 → 看红 → 修 → 看绿。
2. **P0 批次 2**：配置校验 + admin 鉴权收紧（删默认值、删 query 参数、启动期 Validate）。每项 TDD。
3. **P1 批次 3**：生命周期治理（signal.NotifyContext + shutdown 四步排序 + 各后台 loop 的 ctx.Done 处理）。先写 shutdown 集成测试复现问题，再逐项修复。
4. **P1 批次 4**：事件语义 Step 1A（放宽聚合口径 + click 去重 + click/convert ctx 修正）。此时 bidder 仍双写 win+impression，但聚合已能正确处理新语义。
5. **P1 批次 5**：事件语义 Step 1B（停写 impression 伪造）。在 4 合入并观察至少一个完整报表周期无异常后才启动。
6. **P2 批次 6**：补端到端租户隔离集成测试 + shutdown 集成测试 + api_key 扫描测试。

每个批次独立提交 PR、独立过 review / verification / QA 循环，不要跨批次累积未验证改动。

## 建议验证命令
```powershell
go test ./... -short -count=1
go test ./... -count=1     # 去掉 -short 以跑集成测试
cd web && npm run lint
```

如果 API 形状有变化（P0 批次 1 会让 billing 路由形变），执行：

```powershell
make api-gen
```

如果前端行为或回调语义发生变化，补跑：

```powershell
./scripts/test-env.sh verify
```

## 观察项（本轮不修，但需跟踪）
- **API key 级 rate limiting**：`internal/ratelimit` 的配额目前是全局还是 per-key？若全局，单个泄露的 key 没有额外防护，scope 检查是唯一防线。需单独评估一轮。
- **bid_log 6 个月 TTL**：`migrations/002_clickhouse.sql:21`。reporting 和 reconciliation 脚本必须处理超过 TTL 的数据被清理的情况，当前是否健壮需 verify。
- **广告主侧敏感操作审计**：topup / pause / delete creative 当前无 audit trail（admin 侧有）。事故溯源时会缺关键证据。
- **`dsp.billing` topic 未被消费**：`internal/events/producer.go:57` 定义但无 consumer（详见 `docs/contracts/biz-engine.md` §3.1）。本轮不处理，但 P1 事件语义修正时要一并考虑是否删除该 produce 调用。

## 交付要求
- 每个批次分批提交 PR，避免把所有问题揉成一个超大补丁。
- 每批 PR 描述必须说明：
  - 修了哪些风险
  - 改了哪些接口或行为
  - 增加了哪些测试（P2 之前的批次也要含各自的 TDD 测试）
  - 还剩哪些未处理项和为什么延后
