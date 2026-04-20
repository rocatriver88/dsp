# 全功能、全链路测试计划（2026-04-20）

本文不是模板，而是基于当前 DSP 仓库真实能力、真实链路、真实测试现状给出的可执行测试计划。目标只有两个：

- 保证需求被覆盖，不靠“AI 说已经做完了”
- 保证系统对真实人工可用，不靠“单测全绿就算完成”

## 1. 测试总目标

本计划覆盖以下范围：

- `cmd/api` 控制面 API
- `cmd/bidder` 竞价与回调
- `cmd/consumer` Kafka -> ClickHouse 分析链路
- `web/` 广告主与管理端控制台
- PostgreSQL、Redis、Kafka、ClickHouse 之间的跨系统契约
- 关键运维能力：健康检查、指标、回放缓冲、DLQ、对账、自动暂停

本计划的验收标准不是“代码 review 通过”，而是下面 5 类证据全部具备：

1. 需求和测试一一对应
2. 关键功能有自动化测试
3. 关键业务链路有跨服务测试
4. 关键用户旅程有浏览器级验收
5. 运行时故障有观测和恢复验证

## 2. 测试环境与分层

### 2.1 本地快速反馈层

用途：开发中高频运行，快速拦截纯逻辑回归。

- `make test`
- `cd web && npm run lint`
- `cd web && npm run build`

说明：

- `make test` 对应 [Makefile](../Makefile) 的 `go test ./... -short -count=1`
- 这一层不要求真实基础设施
- 目标是让 AI 每次提交前都先过“快闸门”

### 2.2 Handler e2e 层

用途：验证 API/handler 契约、鉴权、租户隔离、pub/sub 发布行为。

- 命令：`go test -tags e2e ./internal/handler/...`
- 依赖：真实 Postgres + Redis + ClickHouse
- 现有基线：`internal/handler/e2e_*_test.go`

这层重点验证：

- public/admin/internal 路由是否可达
- API key / admin token / JWT / SSE token 是否正确
- handler 是否写对 DB
- handler 是否发对 Redis `campaign:updates`
- 报表 handler / billing handler / creative handler / admin handler 是否正确 scope 到当前租户

### 2.3 Engine/Consumer Integration 层

用途：验证 bidder、consumer、reporting 的真实依赖链，不靠 mock。

- 命令：`go test -tags integration ./internal/bidder/... ./cmd/bidder/... ./cmd/consumer/... ./internal/reporting/... ./internal/budget/... ./internal/reconciliation/...`
- 依赖：真实 Postgres + Redis + Kafka + ClickHouse
- 装配基线：`internal/qaharness`

这层重点验证：

- loader 从 DB + Redis pub/sub 同步 campaign
- `Engine.Bid` 的真实过滤、定向、预算、频控、guardrail、stats cache
- `/win` `/click` `/convert` 的真实扣费、去重、Kafka 写入
- consumer 单条/批量写入 ClickHouse
- reporting 从 ClickHouse 读回聚合结果
- DLQ、replay buffer、对账、自动暂停等后台机制

### 2.4 全栈系统层

用途：验证真实用户路径和跨服务完整链路。

- 启动：`./scripts/test-env.sh all`
- 全链路验证：`./scripts/test-env.sh verify`
- 参考脚本：`cmd/autopilot`
- 参考工具：`cmd/exchange-sim`

这层必须回答：

- 从业务侧创建的 campaign，是否真的进入 bidder 可投状态
- 真正的 bid/win/click/convert 数据是否能进入报表和账务视图
- 人在浏览器里是否真的能完成主要操作

### 2.5 浏览器验收层

用途：验证“系统真实人工可使用”。

当前现状：

- 仓库内前端没有成体系的浏览器自动化测试
- 现阶段主要靠 `lint + build + QA 报告 + autopilot`

本计划要求新增一套浏览器级自动化：

- 推荐工具：Playwright
- 建议目录：`web/e2e/`
- 命令建议：`cd web && npx playwright test`

这层重点不在 API 是否 200，而在：

- 页面是否能加载
- 主流程是否能走通
- 错误提示是否可理解
- 权限和导航是否符合真实用户预期
- 移动端、平板、桌面是否可用

## 3. 需求到测试的追踪规则

从今天开始，每个功能都必须能映射到下面 4 个东西：

- 需求说明
- 自动化测试
- 全链路验证
- 人工可用性证据

建议维护一份需求覆盖矩阵：

| 功能 | 场景 | 自动化测试 | 全链路验证 | 浏览器/人工证据 |
|---|---|---|---|---|
| Campaign 启动 | 成功启动 | handler e2e + loader integration | autopilot + exchange-sim | campaigns 页面截图/录屏 |

没有映射关系的需求，不算完成。

## 4. 功能测试矩阵

### 4.1 广告主接入与权限控制

必须覆盖：

- 广告主创建成功
- 广告主仅能读取自己的 advertiser 记录
- `X-API-Key` 缺失、错误、越权时正确返回
- `X-Admin-Token` 保护的接口未授权时返回 401
- 注册流程：有效邀请码、无效邀请码、次数耗尽
- 管理端审批：approve / reject 正确改变状态
- 邀请码创建、列表、消费次数统计正确
- API 与 DB 双层租户隔离：所有带 advertiser/campaign/creative 的读写都要验证 cross-tenant 404/401
- auth login / refresh / me / change-password
- analytics token 签发、SSE token 鉴权、过期 token 拒绝

自动化落点：

- `internal/handler/e2e_public_advertiser_test.go`
- `internal/handler/e2e_admin_registration_test.go`
- `internal/handler/e2e_authz_table_test.go`
- `test/integration/tenant_isolation_test.go`
- `internal/handler/auth_handlers_test.go`
- `internal/handler/analytics_token_test.go`

### 4.2 Campaign 管理

必须覆盖：

- create / list / get / update / start / pause
- 合法状态迁移：`draft -> active -> paused`
- 非法状态迁移被拒绝
- 定向字段持久化：geo、os、browser、time schedule、frequency cap、audience、gender、age
- `budget_total_cents >= budget_daily_cents`
- start 成功后 Redis 发布 `activated`
- pause 成功后 Redis 发布 `paused`
- update 成功后 Redis 发布 `updated`
- bidder loader 真正收到更新并刷新缓存
- start 成功后立即可以被 `/bid` 命中

必须新增的失败场景：

- 启动时余额不足
- 启动时 `BillingSvc.GetBalance` 失败
- 启动时没有 creative
- 启动时没有 approved creative
- 启动时 `end_date` 已过期
- 启动时 `budget_total_cents < budget_daily_cents`

自动化落点：

- `internal/handler/e2e_public_campaign_test.go`
- `internal/bidder/loader_integration_test.go`
- 新增一条跨模块链路测试：`POST /campaigns/{id}/start -> pub/sub -> loader -> /bid`

### 4.3 素材与上传

必须覆盖：

- creative create / update / delete / list
- creative 跨租户访问全部 404
- creative pending / approved / rejected 状态切换
- creative create/update/delete 成功后发布 `campaign:updates`
- banner/native/splash/interstitial 四类素材字段正确存储
- upload 成功上传合法图片并返回 `/uploads/...`
- upload 拒绝伪装文件、可执行文件、错误 MIME、超大文件
- `/uploads/` 静态文件服务可读
- admin creative review 流程能影响投放前置条件

自动化落点：

- `internal/handler/e2e_public_creative_test.go`
- `internal/handler/e2e_creative_pubsub_test.go`
- `internal/handler/e2e_admin_creative_test.go`
- `internal/handler/e2e_public_meta_test.go`

### 4.4 计费与余额

必须覆盖：

- top-up 成功更新余额和流水
- top-up 只能给自己充值；admin top-up 可给任意 advertiser 充值
- transactions / balance 只能看自己的
- CPM 在 `/win` 路径扣费
- CPC 在 `/click` 路径扣费
- oCPM 参与竞价排序和 win 计费
- 预算扣减金额、四舍五入/截断规则明确且被测试
- daily budget 和 total budget 同时生效
- Redis 预算键初始化和重置正确
- reconciliation 正确比较 Redis 与 ClickHouse 聚合

自动化落点：

- `internal/handler/billing_test.go`
- `internal/handler/e2e_public_advertiser_test.go`
- `internal/budget/budget_test.go`
- `internal/budget/budget_integration_test.go`
- `cmd/bidder/handlers_integration_test.go`
- `internal/reconciliation/reconciliation_integration_test.go`

### 4.5 竞价、护栏与反作弊

必须覆盖：

- `POST /bid` 直接入口
- `POST /bid/{exchange_id}` 交易所入口
- exchange adapter parse/format 行为
- direct bid 和 exchange bid 返回字段一致性
- direct bid 和 exchange bid 都带 `NURL`
- direct bid 和 exchange bid 都注入 click tracking
- HMAC token 校验成功/失败
- `/win` `/click` `/convert` dedup
- warm-up 期间 `/win` `/click` 正确返回 503 而不是错误扣费
- targeting：geo、os、browser、time schedule、secure creative、creative size
- fraud filter 拒绝恶意流量
- guardrail precheck / circuit break / bid ceiling / 全局预算
- low balance / spend spike / autopause 规则
- pacing + win-rate + CTR/CVR stats cache 影响 bid 结果
- CPC/oCPM effective CPM 排序正确

必须新增的当前缺口：

- `/bid/{exchange_id}` 正向测试
- exchange 路径 click tracking 断言
- win event payload 正确性断言：`campaign_id`、`request_id`、`clear_price`、`creative_id`、`bid_price`

自动化落点：

- `internal/exchange/adapter_test.go`
- `internal/bidder/engine_test.go`
- `internal/bidder/engine_integration_test.go`
- `cmd/bidder/main_test.go`
- `cmd/bidder/handlers_integration_test.go`

### 4.6 事件、Consumer、ClickHouse 与报表

必须覆盖：

- producer 发送 `bid` / `win` / `click` / `conversion`
- Kafka 不可用时 bufferToDisk
- 启动后 ReplayBuffer 重放
- consumer 单条写入 ClickHouse
- consumer 批量写入 ClickHouse
- malformed JSON 被跳过但消费者继续工作
- ClickHouse 写入失败时进入 `dsp.dead-letter`
- 批量写入失败时整批 fan-out 到 DLQ
- `bid_log` 中 `event_time` 保留原始时间戳
- reporting 聚合：campaign stats、hourly、geo、transparency、attribution、simulate、overview
- CSV export
- analytics snapshot / stream

必须新增的当前缺口：

- `runBatchLoop` 的 DLQ 集成测试
- win event payload 与 reporting 读取字段的一致性验证

自动化落点：

- `internal/events/producer_integration_test.go`
- `cmd/consumer/runner_test.go`
- `cmd/consumer/consumer_integration_test.go`
- `internal/reporting/stats_integration_test.go`
- `internal/reporting/attribution_integration_test.go`
- `internal/handler/e2e_public_report_test.go`

### 4.7 管理端能力

必须覆盖：

- admin overview 页面数据可加载
- registration list / approve / reject
- creative review list / approve / reject
- advertiser list 返回时不泄露 API key
- admin top-up
- invite code create / list
- audit log 查询
- circuit break / reset / status
- internal active campaigns
- admin users create / list / update
- internal/stats 仅允许 admin token

自动化落点：

- `internal/handler/e2e_admin_registration_test.go`
- `internal/handler/e2e_admin_creative_test.go`
- `internal/handler/e2e_admin_system_test.go`
- `internal/handler/user_handlers_test.go`
- `cmd/bidder/main_test.go`

### 4.8 Web 控制台

必须覆盖广告主侧页面：

- 登录门
- 总览
- campaigns 列表
- campaign 新建页
- campaign 详情页
- billing
- reports 列表和详情
- analytics

必须覆盖管理侧页面：

- admin home
- agencies
- creatives
- invites
- audit
- users

每个页面至少验证：

- 页面可加载
- 核心数据正确渲染
- loading / empty / error 状态
- 登录态与权限控制
- 主要表单提交
- API 错误时用户能理解
- mobile / tablet / desktop 三档响应式

这层的落地要求：

- 新增浏览器自动化测试
- 保留关键页面截图
- 每个发布批次至少做一次主路径录屏

## 5. 全链路业务旅程测试

下面这些不是单个功能点，而是必须被证明“整条链真的通”的旅程。

### Journey A：自助接入 -> 审批 -> 登录

链路：

- admin 创建邀请码
- advertiser 使用邀请码注册
- admin 审批
- advertiser 登录
- `me`/dashboard 可见自己的数据

证据：

- handler e2e
- 浏览器录屏
- DB 中 registration/user/advertiser 状态正确

### Journey B：充值 -> 建 campaign -> 建 creative -> 启动 -> bidder 可投

链路：

- advertiser top-up
- create campaign
- create/upload creative
- creative approved
- start campaign
- Redis 发布 `activated`
- bidder loader 收到并刷新缓存
- 紧接着发 `POST /bid`，返回命中该 campaign

证据：

- 新增跨模块自动化测试
- exchange-sim 或 autopilot
- Redis 通道日志
- bidder 缓存/metrics

### Journey C：投放 -> 赢标 -> 点击 -> 转化 -> 报表与账务可见

链路：

- `/bid` 返回 bid response
- `/win` 扣费并发 Kafka 事件
- `/click` 扣 CPC 并发 Kafka 事件
- `/convert` 发 conversion 事件
- consumer 写入 ClickHouse
- reports / analytics / balance / transactions 页面反映结果

证据：

- bidder integration
- consumer integration
- reporting integration
- 浏览器验证 reports/billing/analytics
- ClickHouse `bid_log` 查询结果

### Journey D：创意修改/删除 -> bidder 实时刷新

链路：

- update/delete creative
- API 发布 `updated`
- loader 收到 pub/sub
- 下一次 `/bid` 使用新的 creative 集合

证据：

- creative pubsub e2e
- loader integration
- `/bid` 验证 creative 生效

### Journey E：暂停 / 自动暂停 -> 停止投放

链路：

- 手动 pause 或 autopause 触发
- API/后台任务发布 `paused`
- loader 移除 campaign
- 后续 `/bid` 不再出价

证据：

- pause e2e
- autopause integration
- `/bid` no-bid 断言

### Journey F：Kafka/ClickHouse 故障恢复

链路：

- Kafka down 时 producer bufferToDisk
- Kafka 恢复后 ReplayBuffer
- ClickHouse down 时 consumer 写入 DLQ
- ClickHouse 恢复后可以手动/自动重放

证据：

- producer integration
- consumer DLQ integration
- 运维演练记录

### Journey G：安全回归

链路：

- 跨租户读取 advertiser/campaign/creative/report/billing
- admin token 缺失/错误
- JWT 过期
- SSE token 错误/过期
- 上传伪装文件
- callback HMAC 篡改

证据：

- `test/integration/tenant_isolation_test.go`
- handler e2e
- bidder handler integration

## 6. 非功能测试计划

### 6.1 安全

必须覆盖：

- 所有租户隔离场景
- 所有敏感接口的鉴权
- 上传 MIME sniff
- open redirect 回归
- callback token 重放与篡改
- admin advertiser list 不泄露 API key

### 6.2 性能与容量

第一阶段要求做 smoke，不要求先上完整压测平台：

- `/bid` 单接口压测 smoke
- consumer 批量写入 smoke
- reports 查询延迟 smoke
- 高频 `/bid` 下 Redis budget/freq 正常

建议产物：

- `k6` 或 `vegeta` 脚本
- 基础 QPS / P95 / error rate 报告

### 6.3 稳定性与恢复

必须覆盖：

- 进程重启后服务可恢复
- graceful shutdown 不丢尾部事件
- producer inflight flush 生效
- replay buffer 可恢复消息
- reconciliation 能发现异常差值

### 6.4 可观测性

必须覆盖：

- `/health` / `/health/live` / `/health/ready`
- `/metrics`
- request ID 出现在日志
- 关键失败有结构化日志
- guardrail / reconciliation / autopause 有可见告警或日志

## 7. CI 与发布门槛

### 7.1 PR 门槛

所有 PR 至少通过：

- `make test`
- `cd web && npm run lint`
- `cd web && npm run build`
- 如果改 API 契约：`make api-gen` + `docs-check`

按改动范围追加：

- 改 `internal/handler` 或公共 API：`go test -tags e2e ./internal/handler/...`
- 改 `cmd/bidder` / `internal/bidder`：对应 integration
- 改 `cmd/consumer` / `internal/reporting` / `internal/events`：对应 integration
- 改租户/权限逻辑：`go test -tags integration ./test/integration/...`

### 7.2 Nightly 门槛

每天至少跑一次：

- handler e2e 全量
- bidder/consumer/reporting integration 全量
- `./scripts/test-env.sh verify`
- 浏览器 smoke

### 7.3 Release 门槛

发布前必须全部具备：

- 上述所有自动化测试 green
- 关键业务旅程 A-G 全部通过
- 关键页面截图或录屏
- 需求覆盖矩阵完整
- 剩余风险清单明确
- 可回滚方案验证

## 8. 当前必须补齐的测试缺口

基于仓库现状，优先级如下。

### P0：本周必须补

- `cmd/bidder/handlers_integration_test.go` 增加 `POST /bid/{exchange_id}` 正向测试
- `internal/handler/e2e_public_campaign_test.go` 增加 4 条 start 失败测试：余额不足、余额查询错误、无 approved creative、`budget_total < budget_daily`
- 新增跨模块链路测试：`POST /campaigns/{id}/start -> Redis -> loader cache -> /bid`
- `cmd/consumer/consumer_integration_test.go` 增加 batch-store failing fixture，验证 `runBatchLoop` DLQ

### P1：本迭代补

- 强化 `/win` payload 断言：`campaign_id`、`request_id`、`clear_price`、`creative_id`、`bid_price`
- creative 修改/删除后 bidder 实时刷新并影响 `/bid`
- autopause -> no-bid 链路
- replay buffer -> recovery 链路
- `internal/stats` / admin 用户管理 / circuit status 浏览器与 API 双验证

### P2：下一迭代补

- Web 浏览器自动化测试框架（Playwright）
- 性能 smoke
- 发布前录屏脚本化
- 可观测性断言自动化

## 9. AI Coding 时代的执行原则

这个计划默认你不做人工 code review，所以执行上必须坚持下面 5 条：

1. 先写测试计划，再让 AI 写实现
2. 每个功能先补需求-测试映射，再接受代码
3. 不接受“单测过了但链路没证据”的交付
4. 不接受“接口通了但浏览器没人能用”的交付
5. 每次发布都产出测试报告、截图/录屏、残余风险

如果按这份计划执行，你看的不再是源码，而是：

- 需求覆盖表
- 自动化测试结果
- 全链路验证结果
- 浏览器可用性证据
- 运行时可观测性证据

这才是 AI Coding 场景下真正可依赖的研发质量体系。
