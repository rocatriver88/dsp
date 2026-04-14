# 评审整改任务单 — 2026-04-14 执行基线（V5）

> 本文件是 2026-04-14 项目评审整改任务的**最终执行基线**。V5 在技术决定上完整采纳 codex V4 的结论，本版的作用仅是合并 V1 → V4 的演进轨迹并归档历史版本。后续若有新的评审发现，从本文件继续迭代。
>
> **历史版本**（供审计与溯源）：`docs/archive/REVIEW_REMEDIATION_PLAN_2026-04-14/`
> - `V1.md` — 原独立第三方评审 + Claude Code 第一次注释（commit `97052d3`）
> - `V2.md` — codex 对 V1 的响应，提出 Claude V2 的执行方案调整
> - `V3.md` — Claude Code 对 V2 的响应（commit `989685c`），含 Claude Code 后来被 codex 纠正的 Step 1A 错误
> - `V4.md` — codex 对 V3 的响应，修正 V3 六处问题
> - （本文件，无版本后缀）— V5 执行基线，内容 = V4 原文 + 本审计表

## 演进与决策审计

这份任务单的最终形态经过**两轮评审者往返**（Claude Code ↔ codex）和**一次显著的技术错误修正**。本节完整记录每一个关键议题的演进轨迹、最终决定及其理由，供未来维护者溯源"为什么当初这么定"。

### 1. `win` / `impression` 迁移方案（本轮最严重的一次 bug 修正）

**演进**
- **V1 原评审**：建议直接停写 `impression`、同时校准下游聚合
- **V2 Claude Code**：改成两步迁移，Step 1A 在 bidder 仍双写的情况下把聚合改为 `event_type IN ('win','impression')`
- **V4 codex** 指出 **V2 Step 1A 有双计缺陷**——双写状态下每次赢标在 `bid_log` 已存两行（`win` + `impression`），OR 聚合会把每次赢标算作 2 次曝光、CTR 腰斩、autopause 阈值失效
- **V5 采纳** codex 的 `effective_delivery = countDistinctIf(request_id, event_type IN ('win','impression'))` 方案

**最终决定**
两步迁移，独立 PR 提交：
- Step A：在 reporting / autopause / reconciliation 中引入 `effective_delivery` 共享聚合概念，bidder 仍保持双写
- Step B：删除 `handleWin` 中伪造的 `SendImpression`，bid_log 从此不再新增 `event_type='impression'` 行
- Step C（可随 A 或 B）：click 去重 + click/convert 改用后台 context

**理由**
`countDistinctIf(request_id, ...)` 在双写、停写、未来真实曝光回调三种状态下指标都稳定。V2 的 OR 聚合方案如果直接合并，会在落地当天让线上 CTR 减半。这是本次评审链条中捕获到的最严重 bug。

### 2. `api_key` 隐藏：DTO vs `json:"-"`

**演进**
- **V1 原评审**：建议新增响应 DTO
- **V3 Claude Code**：反驳为"过度工程"，建议直接在 model 上打 `json:"-"`
- **V4 codex**：反驳重回 DTO，指出 `campaign.Advertiser` struct **已同时扛 DB model / OpenAPI schema / handler response 三个角色**
- **V5 采纳** codex 的 DTO 方案

**最终决定**
新增 `AdvertiserResponse` DTO，所有对外读接口返回 DTO；持久化模型 `campaign.Advertiser` 的 `APIKey` 字段保持 `json:"api_key"` 不变。创建路径（`POST /advertisers`、admin 审核通过）继续返回一次明文 key，使用独立响应结构。

**理由**
`json:"-"` 打在持久化模型上有两个具体问题：(a) 内部审计 / 调试路径对 `Advertiser` 的 `json.Marshal` 也会丢失字段，是正确性问题；(b) 未来 admin 路径需要受控展示 key 时被 tag 反向约束。另外，"创建返回 key、读接口不返回"的分叉已经存在（创建路径当前用 `map[string]any{"id":..., "api_key":...}`），DTO 不是假想需求，而是对现状的合理重构。

### 3. `POST /billing/topup` body 携带他人 `advertiser_id` 的处理

**演进**
- **V1 原评审**：忽略 body `advertiser_id`，强制从 auth 取
- **V3 Claude Code**：提出"静默忽略该字段、路由到本人、返回 200"的"降噪"方案，类比 GitHub
- **V4 codex**：反驳，这是账务写入，静默纠偏会让客户端以为给 B 充值实际给 A 扣款，破坏 reconciliation
- **V5 采纳** codex 的 400

**最终决定**
过渡期如果 body 含 `advertiser_id` 且不等于 auth id，返回 **400 Bad Request**，附错误信息 "advertiser_id mismatch"；等于或缺失则 200 正常处理。最终状态应移除该字段。

**理由**
billing 是账务写入的硬边界。参数自相矛盾时必须显式失败，不能让客户端产生"成功但数据偏移"的状态——那会直接破坏对方的对账系统。这既不是攻击面（不是"危险输入"），也不是资源探测（无需 hide existence），语义上就是请求参数自相矛盾 → 400。

### 4. admin token 缺失或错误的响应码

**演进**
- **V1 原评审**：未明确
- **V3 Claude Code**：在规则段写 403，但在测试段写 401，**自相矛盾**
- **V4 codex**：统一为 401，并定义完整三码规则
- **V5 采纳**

**最终决定**
三码规则，全文一致：
- **401 Unauthorized**：认证凭证缺失或无效（API key 错、admin token 错、未登录）
- **404 Not Found**：已认证但访问其他租户的资源（隐藏存在性）
- **400 Bad Request**：请求参数自相矛盾或语义非法（如 topup body 中的 foreign `advertiser_id`）

**理由**
admin token 错误语义上是"未证明身份"，对应 401。403 适用于"身份已知但无权限"的场景（如 RBAC），不适用于未认证。同类越权不能在不同 handler 里返回不同码——测试必须断言精确码而非 4xx 桶。

### 5. `Config.Validate()` 的形态

**演进**
- **V1 原评审**：启动时调用 `Validate()`
- **V2 Claude Code**：写成 `if err := cfg.Validate(); err != nil { log.Fatalf(...) }` 的硬要求
- **V3 Claude Code**：放宽为"任意形态只要启动失败"（`log.Fatal` / `panic` / `return error` 均可）
- **V4 codex**：收紧为 `Validate() error`，`log.Fatal` 由 main 决定
- **V5 采纳** codex

**最终决定**
- `func (*Config) Validate() error` ——纯函数式，只返回错误
- `cmd/api` 与 `cmd/bidder` 在启动时 `if err := cfg.Validate(); err != nil { log.Fatal(err) }`
- config 包的单元测试直接断言 `err != nil`

**理由**
副作用（进程退出）应当和纯函数（校验逻辑）分开。把 `log.Fatal` 写在 config 包里会让单元测试笨重——要么用子进程，要么替换 logger，都比 pure function 贵得多。这是软件工程的基本纪律。

### 6. shutdown 约束形式

**演进**
- **V2 Claude Code**：写成硬性四步序列 + 具体代码形态（`signal.NotifyContext`、一个 rootCtx 管所有）
- **V3 Claude Code**：略松但仍是序列
- **V4 codex**：改为"不变量约束" + **`workerCtx` 与 request ctx 分离**
- **V5 采纳**

**最终决定**
- `processCtx`：进程级，从信号派生
- `workerCtx`：长期后台 loop 用
- **request ctx**：由 `http.Server` 独立管理，**不绑定到 root ctx**
- 关闭不变量（五条）：
  1. 新请求停止进入
  2. worker 能退出
  3. inflight 请求能 drain
  4. producer 在最后一次业务写入后 flush / close
  5. 存储连接在上层不再使用后关闭
- 实现者在满足上述五条的前提下自由选择代码结构

**理由**
把 request 和 worker 绑定到同一 root ctx 会让 root cancel 误杀正在处理的 HTTP 请求，客户端见到假失败、触发重试、级联故障。正确做法是区分"worker 生命周期"（进程启动到退出）和"request 生命周期"（请求到达到 handler 返回），两者由不同的 context 树管理。

### 7. 创意 handler 的 scope 覆盖范围

**演进**
- **V1 原评审**：漏（只列 advertiser / billing / report）
- **V2 Claude Code**：加入 `HandleCreate/Update/DeleteCreative`
- **V4 codex**：补齐 `HandleListCreatives`（V3 Claude 漏）
- **V5 四个 handler 全部纳入**

**最终决定**
四个 creative handler 全部做 owner check：
- `GET /campaigns/{id}/creatives`（list）
- `POST /creatives`（create）
- `PUT /creatives/{id}`（update）
- `DELETE /creatives/{id}`（delete）

写路径（create / update / delete）修复 scope 时**同批**补 `campaign:updates` publish，和 `docs/contracts/biz-engine.md` §1 的契约一致。

**理由**
- `delete` 按 creative id 直接删，任何广告主能删他人创意
- `update` 能篡改他人 `ad_markup` —— **存储型 XSS 注入向量**（创意会被投放到真实流量里）
- `list` 泄露他人 campaign 下的创意列表
- `create` 能把创意塞到他人 campaign 下

四者严重性相当，必须同批修。

### 8. 测试要求的表述

**演进**
- **V1 原评审**：测试放在最后的 P2 批次
- **V2 Claude Code**：改为每一项 P0/P1 子项先写 failing test 再修，TDD 红绿硬门槛
- **V4 codex**：改为"PR 必须携带回归测试"，不把 red-first 流程写进验收
- **V5 采纳**

**最终决定**
每个安全 / 计费 / 租户边界修复 PR 必须包含覆盖该修复的测试；测试应当"修复前失败、修复后通过"，由 reviewer 结合 diff 判断。**不**把 red-first 流程作为文档层面的验收条款。

**理由**
整改任务单是 **spec** 不是 **workflow**。reviewer 从 PR 能看到"测试存在 + 覆盖范围"，看不到"测试是先写的还是后写的"——红绿先后无法从产物强制，写进 spec 反而冗余。项目级 CLAUDE.md 仍规定 TDD 是工作流偏好，本任务单不重复该条款。

---

以下内容 = V4 技术部分逐字采纳。

## 执行原则

- 先封安全边界，再修事件与计费语义，最后补运行时治理与回归防线。
- 能用局部改动闭环的问题，不顺带大改架构。
- 影响 API 形状时，同步更新契约与生成物。
- 每个整改 PR 必须包含对应测试，不接受"后补测试"。

## P0：租户隔离与敏感信息

### 问题范围

- `HandleGetAdvertiser` 按 path id 直读 advertiser。
- billing 三个接口按 path/query/body 中的 advertiser id 取数。
- report 中 `stats/hourly/geo/bids/attribution` 五条路径缺 owner check。
- creative 的 list/create/update/delete 缺 owner check。
- advertiser 读模型会把 `api_key` 序列化出去。
- 前端 billing 页面硬编码 advertiser `1`。

### 返回码规则

- 缺失/错误 API key 或 admin token：`401`
- 租户资源越权：`404`
- 请求参数自相矛盾或非法：`400`

### 必做改动

1. advertiser
   - `GET /api/v1/advertisers/{id}` 必须校验 path id == auth advertiser id，否则 `404`。
   - 新建 `AdvertiserResponse`，对外不返回 `api_key`。
   - 创建 advertiser / 审核通过后首次发 key，使用专门响应结构返回。

2. billing
   - `GET /billing/transactions` 不再按 query `advertiser_id` 取数，统一从 auth context 取。
   - `GET /billing/balance/{id}` 做 owner check，不匹配返回 `404`。
   - `POST /billing/topup` 最终移除 `advertiser_id` 语义；过渡期若 body 提供且与 auth id 不一致，返回 `400`，不要静默改给自己。

3. reports
   - `stats/hourly/geo/bids/attribution` 五条路径，先做 `GetCampaignForAdvertiser`。
   - `simulate/export` 已有检查，保持一致。

4. creatives
   - list：先验 campaign owner。
   - create：按 `campaign_id` 验 owner。
   - update/delete：先取 creative 所属 campaign，再验 owner。
   - create/update/delete 修复 scope 的同时补 `campaign:updates` publish。

5. web
   - billing 页面移除 advertiser `1` 硬编码。
   - `web/lib/api.ts` 的 self-service billing API 去掉 advertiserId 参数。

### 测试要求

- A 的 key 访问 B 的 advertiser / billing / report / creative 全路径均为 `404`。
- topup 在 body 中带错 advertiser id 返回 `400`。
- advertiser 读接口返回体中不含 `api_key`。
- creative 合法写操作会发 `campaign:updates`。

## P0：管理面与启动配置安全

### 问题范围

- `cmd/api`、`cmd/bidder` 未调用配置校验。
- admin middleware 有默认 `admin-secret`。
- admin middleware 接受 query `admin_token`。

### 必做改动

1. 配置校验
   - `Config.Validate() error`
   - `cmd/api`、`cmd/bidder` 启动时强制校验并在失败时退出
   - 生产环境至少校验：
     - `BIDDER_HMAC_SECRET`
     - `ADMIN_TOKEN`
     - `CORS_ALLOWED_ORIGINS` 不能保留本地默认值

2. admin auth
   - 删除 `admin-secret` fallback
   - 删除 query `admin_token`
   - 只接受 `X-Admin-Token`
   - 无 token / token 错误统一 `401`

3. 文档
   - 在 `config.go` 注释和运行时文档中明确生产必须设置的 secret 清单。

### 测试要求

- `Validate()` 在生产缺少关键 secret 时返回 error。
- 正确 admin header 通过。
- query token 无效。
- 未设置 admin token 时服务启动失败。

## P1：事件语义、计费与报表口径

### 问题范围

- `handleWin` 同时发送 `win` 和伪造的 `impression`。
- 下游多个模块把 `event_type='impression'` 当真实曝光。
- click 无去重。
- click/convert 异步发送仍用 request context。

### 处理原则

这里要同时解决两件事：

- 短期内不能让指标跳变
- 长期内不能继续把"赢标"与"真实曝光"混成一个概念

因此本版不把"`impression` 字段继续代表 `win OR impression`"写成最终语义，而是拆成两个层次：

- 存储层保留真实事件类型
- 聚合层在过渡期引入一个共享概念：`effective_delivery`

`effective_delivery` 的临时定义：

```sql
countDistinctIf(request_id, event_type IN ('win', 'impression'))
```

它只用于当前需要"已投出一次、避免双计"的聚合场景，目的是平滑迁移，不是把真实 impression 定义永久改写掉。

### 建议迁移顺序

1. Step A：先引入共享聚合 helper
   - 在 reporting/autopause/reconciliation 中统一通过 helper 或统一 SQL 片段计算 `effective_delivery`
   - 当前 UI 上对外仍可先复用现有 `impressions` 字段，但文档要标明这是过渡口径，不等同于未来真实展示曝光

2. Step B：删除 `handleWin` 中伪造的 `SendImpression`
   - 之后 `bid_log` 不再新增伪造 impression
   - 因 Step A 已落地，指标不跳变

3. Step C：修 click/convert 一致性
   - `click:dedup:{request_id}`
   - click/convert 的 producer 改用后台 context 或带 timeout 的派生 context

4. Step D：后续单独议题，不纳入本轮
   - 若产品真的需要"真实曝光"指标，应补独立曝光上报链路，并把 API/UI 里的字段命名与文案重新收敛

### 结果要求

- 迁移前后投放指标不出现人为跳变。
- click 重试不重复扣费。
- handler 返回后 click/convert 事件不因 request context cancel 丢失。
- 文档明确区分过渡口径与真实 impression 语义。

### 测试要求

- 双写状态下，相同 `request_id` 的 `win + impression` 只计一次 `effective_delivery`。
- 删除伪造 impression 后，同样数据集的聚合结果保持不变。
- 同一 `request_id` 多次 click 只扣一次预算。
- request context 取消后，mock producer 仍能收到 click/convert 发送。

## P1：生命周期与优雅关闭

### 问题范围

- 长期 goroutine 使用永不取消的 `context.Background()`
- shutdown 只关 HTTP server，不显式收敛后台 worker

### 必做改动

1. 上下文分层
   - 主进程有 `processCtx`
   - 长期 worker 用 `workerCtx`
   - 请求仍由 HTTP server/request context 管理，不和 workerCtx 强绑定

2. 关闭不变量
   - 新请求停止进入
   - worker 能退出
   - inflight 请求被 drain
   - producer 在最后一次业务写入后 flush/close
   - 存储连接最后关闭

3. 文档化失败策略
   - Redis、Kafka、ClickHouse 各自 fail-open / fail-closed 行为写入运行时文档

### 测试要求

- cancel workerCtx 后各后台 loop 能退出。
- 进程收到 SIGTERM 后在限定时间内退出。
- shutdown window 内不会出现新的非预期后台写入。

## P2：回归保护

### 目标

建立最少但有效的护栏，阻止相同类型问题再次出现。

### 必做改动

1. handler 集成测试
   - 两广告主 A/B
   - 穷举越权读写路径
   - 精确断言 `401/404/400`

2. 敏感字段扫描
   - advertiser 响应不含 `api_key`

3. bidder 行为测试
   - win/click 去重
   - click/convert 背景发送

4. lifecycle 测试
   - worker 退出
   - 进程 shutdown

5. 前端回归
   - billing 页面不再依赖 advertiser `1`
   - campaign/report/creative 页面在修复后正常工作

### 验收标准

- 本轮已识别问题都有测试保护。
- `make test` 继续通过。
- 新增集成测试可单独运行，不强迫把所有全仓长测都塞进默认短测路径。

## 建议执行顺序

1. P0 租户隔离与 DTO 化响应
2. P0 admin/config 安全
3. P1 事件语义 Step A + Step C
4. P1 删除伪造 impression
5. P1 lifecycle
6. P2 集成与回归保护

## 建议验证命令

```powershell
make test
go test ./internal/handler/... -count=1
go test ./cmd/bidder ./internal/reporting ./internal/reconciliation -count=1
cd web && npm run lint
```

如接口或 schema 变化：

```powershell
make api-gen
```

如需完整链路验证：

```powershell
./scripts/test-env.sh verify
```

## 观察项

- analytics SSE 当前仍通过 query `api_key` 透传，这是安全异味，但因 `EventSource` 限制，建议单独立项评估，不与本轮 P0 混做。
- `dsp.billing` topic 仍 produced but unconsumed，可在事件语义整改时一并判断是否保留。
- `SendLoss` 未接入，先不纳入本轮。
- 广告主侧敏感操作的 audit trail 仍偏弱，建议在边界修复完成后单独立项。

## 交付要求

- 每个批次独立 PR。
- 每个 PR 必须说明：
  - 修了哪些风险
  - 改了哪些接口语义
  - 新增了哪些测试
  - 有哪些延后项
- 事件语义迁移必须分步提交，避免 reviewer 把"聚合修复"和"停写 impression"揉成一个无法观察的数据变更。
