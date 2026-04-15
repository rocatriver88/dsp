# biz QA 轮次设计 — 2026-04-14

本文档是 biz worktree 本轮系统性 QA 的设计(spec)。产物是 Phase 化的实现计划所依据的权威文档。

---

## 1. 范围 + 成功标准

### 1.1 本轮覆盖

| 块 | 范围 | 产物 |
|---|---|---|
| F · Creative pub/sub 修复 | 3 条 handler + 对应 Go 测试 | 代码 + 测试 |
| B1 · 后端 Go e2e 测试基建 | test helpers、build tag、per-test 唯一前缀 fixture、mux 抽取 | `internal/handler/e2e_*_test.go` |
| B2 · 后端 handler e2e 覆盖 | 全部 `publicMux` + `adminMux` 路由 happy-path + 写类路由 pub/sub 断言 + 401/403/404 table-driven | Go 测试文件 |
| B3 · 一次性脚本 harness | 完整业务流(注册→审核→充值→建广告→启停→创意→报表)一遍,产报告 | `scripts/qa/*.sh` + `docs/qa/2026-04-14-biz-qa-report.md` |
| F1 · 前端 /qa | `web/app/**` 所有页面按 CLAUDE.md 四维度扫 | 修复 commit + 报告章节 |
| F2 · DESIGN.md 合规 | 每页 3+ 元素 CSS 抽查、字体/颜色/间距对齐 | 修复 commit + 报告章节 |
| F3 · 契约一致性 | `docs/generated/openapi3.yaml` ↔ 后端路由 & 响应体 ↔ `web/lib/api-types.ts` 三方对齐 | 漂移点写报告,阻塞性现场修 |

### 1.2 本轮不覆盖(显式排除)

- engine 代码(`cmd/bidder`, `cmd/consumer`, `internal/bidder`, `internal/events`)
- `dsp.billing` topic、`SendLoss`(契约 §3.1/3.2)
- 性能 / 压测(本轮只验正确性)
- 鉴权加固(只验现有 `X-API-Key` / `X-Admin-Token` 行为,不引入新方案)

### 1.3 成功标准(本轮 done 的定义)

1. Creative 3 条路由都发 `campaign:updates` pub/sub,且有 Go 测试订阅到消息
2. 所有 handler 路由至少 1 条 e2e 用例(happy-path),写入状态变更的路由额外有 pub/sub 断言
3. 脚本 harness 从零跑到"广告主能看到自己投放产生的报表"一路不报错
4. `web/app/**` 每个页面通过 /qa 四维度 + DESIGN.md 合规 + 契约漂移扫描(§6.2 定义的 0/1/2/3 四维度,加本轮新增的契约一致性维度)
5. `docs/qa/2026-04-14-biz-qa-report.md` 产出,列出所有发现的 bug / 已修 / 已记未修
6. 整轮 verification 循环最后一轮零问题(按 CLAUDE.md 每 Phase 5 轮上限)

---

## 2. Phase 结构

Phase 串行执行,禁止并行。

### P1 · Creative pub/sub 修复

改 3 个 handler:
- `HandleCreateCreative`:从请求 body 的 `campaign_id` 拿 id
- `HandleUpdateCreative`:先 `GetCreative` 查出 `CampaignID`,再调发布
- `HandleDeleteCreative`:同上(先查再发,因为删除后就查不到了)

调用:`bidder.NotifyCampaignUpdate(ctx, d.Redis, campaignID, "updated")`。

参照 commit `becdc67` 的 campaign handler 改法。

**TDD**:先写 3 条测试(subscribe `campaign:updates` → 调 handler → 断收到消息)→ 测试红 → 改代码 → 测试绿。

**P1 测试的时序妥协**:P1 执行时 §3 的 e2e 基建(mux 抽取 + helpers)还没建。P1 的 3 条测试直接调 `h.HandleCreateCreative(w, r)` 等 handler 方法,bypass mux 和中间件,因为本 Phase 只关心"handler 内部是否 publish",不关心路由/鉴权。pub/sub 订阅逻辑写在 `internal/handler/e2e_creative_pubsub_test.go` 局部,build tag `e2e`,连真实 pg/redis(env 同 §3.2)。advertiser/campaign/creative fixture 用内联代码调 `campaign.Store` 直接建。P2 把这些局部代码抽到 `e2e_support_test.go`,P1 的测试文件随后改成调公共 helper。

**Exit**:测试全绿,verification loop 零问题一轮,commit 可独立 revert。

### P2 · 后端 Go e2e 测试基建 + 全 handler 覆盖

P2 开工时,P1 的创意测试已经存在但用的是内联代码。第一个 task 是**把 `cmd/api/main.go` 里的路由注册 + 中间件链抽到 `internal/handler/` 下**:

```go
func BuildPublicMux(d *Deps) *http.ServeMux
func BuildAdminMux(d *Deps) *http.ServeMux
func BuildPublicHandler(cfg *config.Config, d *Deps) http.Handler  // 含中间件链
```

`cmd/api/main.go` 改成调这三个函数。纯重构,不改行为。抽完验证 `go build ./cmd/api` + `docker compose up -d` + 健康检查通过,再进下一 task。

**rate-limit 中间件在 e2e 测试构造时跳过**,原因是 infra 层不属于 handler 语义,且并发测试会撞限流。production `BuildPublicHandler` 保留 rate-limit。

第二个 task 是 `e2e_support_test.go` helpers(见 §3),同时把 P1 的内联代码迁移到公共 helper,P1 的测试文件改成调 helper(不改测试语义)。

第三个 task 起按 §4 覆盖矩阵逐个写测试文件。

**Exit**:所有路由至少 1 绿测试,verification loop 零问题一轮。

### P3 · 脚本 harness + 后端 bug triage

实现 `scripts/qa/run.sh` + 分步脚本(见 §5)。从零跑一遍,发现的 bug 按阻塞/非阻塞分档:

- **阻塞**:现场修,流程严格为 "补 Go 测试(红)→ 改代码(绿)→ 跑 harness 再走一遍"
- **非阻塞**:记入报告

**Exit**:harness 从零到底一次通过,所有 fix 都有对应 Go 测试兜底,verification loop 零问题一轮。

### P4 · 前端 /qa 四维度扫 + DESIGN.md 合规

范围:`web/app/**` 全部页面(见 §6)。每页按 CLAUDE.md 四维度 + 本设计新增的"契约漂移扫描"维度。

**第一步**:读 `web/AGENTS.md` + `web/CLAUDE.md` + `DESIGN.md`。DESIGN.md 的权威值抄入报告"合规基准"章节,后续抽查以此为准。

**第二步**:查 OpenAPI regenerate 流程(Makefile / scripts),不存在就记报告,本轮不强行引入新工具。

前端 bug 修复不走 TDD(那是后端的路),走:改 `.tsx` / CSS → 热重载 → 再跑对应页面 /qa → 回归通过。后端根因切换到后端 TDD 流程。

**Exit**:所有页面四维度通过,DESIGN.md 合规差异为 0(或有理由的记录),契约三方无阻塞性漂移,verification loop 零问题一轮。

### P5 · 最终清扫 + /browse 截图

- final-code-review 全量
- /browse 截图:每个关键页面三档响应式(1440 / 768 / 375)存 `docs/qa/screenshots/2026-04-14/`
- 关键交互 before/after 截图
- 生成 `docs/qa/2026-04-14-biz-qa-report.md` 最终版
- 在报告里记录"下次合回 main 时要改契约文档的哪一段"(本轮禁止改契约)
- 走 `finishing-a-development-branch` 合回 main

**Exit**:PR 可合并,verification loop 零问题一轮。

### Phase 间依赖

- P1 独立,作为最快落地的契约合规修复
- P2 依赖 P1(e2e helpers 会复用 pub/sub 订阅能力)
- P3 依赖 P2(harness 发现 bug 后用 P2 基建补兜底测试)
- P4 理论上可以与 P2 并行,但决策为**严格串行**(避免 verification 循环打乱)
- P5 依赖全部

---

## 3. Go e2e 测试基建

### 3.1 文件布局

```
internal/handler/
  e2e_support_test.go               // build tag e2e,所有 helper
  e2e_public_advertiser_test.go     // advertiser + register + billing
  e2e_public_campaign_test.go       // campaign + start/pause
  e2e_public_creative_test.go       // creative CRUD + pub/sub
  e2e_public_report_test.go         // reports + export + analytics
  e2e_public_meta_test.go           // ad-types + billing-models + audit-log + upload
  e2e_admin_registration_test.go    // registrations + invite-codes
  e2e_admin_creative_test.go        // creatives review
  e2e_admin_system_test.go          // health + circuit + advertisers + topup + active-campaigns
  e2e_authz_table_test.go           // 通用 401/403/404 table-driven
```

所有文件头:

```go
//go:build e2e
// +build e2e
```

默认 `go test ./...` 跳过。跑 e2e:

```
go test -tags=e2e -count=1 ./internal/handler/...
```

### 3.2 连接参数(env + 默认值)

对齐 biz worktree 的 +11000 端口偏移:

| var | default |
|---|---|
| `DSP_E2E_PG_DSN` | `postgres://dsp:dsp@localhost:16432/dsp?sslmode=disable` |
| `DSP_E2E_REDIS_ADDR` | `localhost:17380` |
| `DSP_E2E_CH_ADDR` | `localhost:19124` |
| `DSP_E2E_ADMIN_TOKEN` | `admin-secret` |

`scripts/qa/e2e-env.sh` 一条 `source` 把以上都设好。

### 3.3 核心 helpers

```go
func mustDeps(t *testing.T) *handler.Deps
func newAdvertiser(t *testing.T, d *handler.Deps) (int64, string)
func authedReq(t *testing.T, method, path string, body any, apiKey string) *http.Request
func adminReq(t *testing.T, method, path string, body any) *http.Request
func exec(t *testing.T, mux http.Handler, req *http.Request) *httptest.ResponseRecorder

// subscribe 先返 wait 闭包,确保 subscription 在 handler 触发前建立
func subscribeUpdates(t *testing.T, rdb *redis.Client, campaignID int64) func(time.Duration) bool
```

`subscribeUpdates` 实现关键:`pubsub.Receive` 等 confirm 消息后再返 wait 闭包。按 `campaign_id` filter,支持并发测试不串场。

### 3.4 Fixture 清理策略

**不清理**,每个测试唯一邮箱前缀(`qa-{t.Name()}-{random}@test`),数据沉淀。

重置方式:`docker compose down -v && docker compose up -d`(migrate 幂等)。

`ListAll` 类 admin 接口的断言只验"我刚建的那条在列表里",不检查 total。

### 3.5 跳过 rate-limit 中间件

e2e 构造 handler 时不挂 `ratelimit.Middleware`。理由:
- rate-limit 是 infra,不属于 handler 语义
- 快速建广告主 + 连发 5 个请求会被限流,让测试不可重复
- rate-limit 可以单独针对性测试(本轮不做)

production `BuildPublicHandler` 保留 rate-limit。

---

## 4. 后端 handler 覆盖矩阵

目标:每条路由至少一条 happy-path,写类路由加 pub/sub 断言,权限/错误分支 each case 1 条。

### 4.1 Public 路由

| 路由 | happy | pub/sub | 错误分支 |
|---|---|---|---|
| `POST /api/v1/advertisers` | ✅ | — | 400 缺字段 |
| `GET /api/v1/advertisers/{id}` | ✅ | — | 404 |
| `POST /api/v1/campaigns` | ✅ | `updated` | 400 targeting 非法 JSON |
| `GET /api/v1/campaigns` | ✅ | — | 401 |
| `GET /api/v1/campaigns/{id}` | ✅ | — | 404 / 403 |
| `PUT /api/v1/campaigns/{id}` | ✅ | `updated` | 404 |
| `POST /api/v1/campaigns/{id}/start` | ✅ draft→active | `activated` | 余额不足 400 |
| `POST /api/v1/campaigns/{id}/pause` | ✅ active→paused | `paused` | 状态非 active 400 |
| `GET /api/v1/campaigns/{id}/creatives` | ✅ | — | — |
| `POST /api/v1/creatives` | ✅ | `updated` | 400 ad_markup 缺失 |
| `PUT /api/v1/creatives/{id}` | ✅ | `updated` | 404 |
| `DELETE /api/v1/creatives/{id}` | ✅ | `updated` | 404 |
| `GET /api/v1/ad-types` | ✅ | — | — |
| `GET /api/v1/billing-models` | ✅ | — | — |
| `GET /api/v1/reports/campaign/{id}/stats` | ✅(CH fixture) | — | 403 |
| `GET /api/v1/reports/campaign/{id}/hourly` | ✅ | — | — |
| `GET /api/v1/reports/campaign/{id}/geo` | ✅ | — | — |
| `GET /api/v1/reports/campaign/{id}/bids` | ✅ | — | — |
| `GET /api/v1/reports/campaign/{id}/attribution` | ✅ | — | — |
| `GET /api/v1/reports/campaign/{id}/simulate` | ✅ | — | — |
| `GET /api/v1/reports/overview` | ✅ | — | — |
| `GET /api/v1/export/campaign/{id}/stats` | ✅ CSV header | — | — |
| `GET /api/v1/export/campaign/{id}/bids` | ✅ CSV header | — | — |
| `GET /api/v1/audit-log` | ✅ | — | — |
| `GET /api/v1/analytics/stream` | SSE 只验 200 + content-type | — | — |
| `GET /api/v1/analytics/snapshot` | ✅ | — | — |
| `POST /api/v1/billing/topup` | ✅ 余额增加 | — | 400 金额非正 |
| `GET /api/v1/billing/transactions` | ✅ | — | — |
| `GET /api/v1/billing/balance/{id}` | ✅ | — | 403 |
| `POST /api/v1/upload` | ✅ 小 PNG 返 url | — | 400 类型非法 |
| `POST /api/v1/register` | ✅ invite-code 有效 | — | 400 invite-code 无效 |

### 4.2 Admin 路由

| 路由 | happy | 错误 |
|---|---|---|
| `GET /internal/active-campaigns` | ✅ | 401 |
| `GET /api/v1/admin/registrations` | ✅ | 401 |
| `POST /api/v1/admin/registrations/{id}/approve` | ✅ pending→approved | 404 |
| `POST /api/v1/admin/registrations/{id}/reject` | ✅ pending→rejected | 400 状态非 pending |
| `GET /api/v1/admin/health` | ✅ | 401 |
| `GET /api/v1/admin/creatives` | ✅ pending 列表 | 401 |
| `POST /api/v1/admin/creatives/{id}/approve` | ✅ | 404 |
| `POST /api/v1/admin/creatives/{id}/reject` | ✅ | — |
| `POST /api/v1/admin/circuit-break` | ✅ | — |
| `POST /api/v1/admin/circuit-reset` | ✅ | — |
| `GET /api/v1/admin/circuit-status` | ✅ | — |
| `GET /api/v1/admin/advertisers` | ✅ 见我建的那条 | — |
| `POST /api/v1/admin/topup` | ✅ 余额增加 | — |
| `POST /api/v1/admin/invite-codes` | ✅ 拿到 code | — |
| `GET /api/v1/admin/invite-codes` | ✅ 见刚建的 | — |
| `GET /api/v1/admin/audit-log` | ✅ | — |

### 4.3 通用 table-driven 断言

- 所有 public 路由:`401 无 X-API-Key`
- 所有 admin 路由:`401 无 X-Admin-Token`
- 所有 `{id}` 路由:`404 不存在` 或 `403 别人的`

写在 `e2e_authz_table_test.go`。

### 4.4 ClickHouse fixture

Reports 类测试需要 `bid_log` 数据。不走真实 bidder,直接写 ClickHouse:
- 优先用 `reporting.Store` 公开的写接口(P2 第一 task 确认是否存在)
- 否则 raw `clickhouse-go/v2` client 打 SQL
- 所有 fixture 行绑定当前测试建的独立 advertiser + campaign,查询按 `advertiser_id` filter

---

## 5. 脚本 harness(P3)

### 5.1 技术选型

bash + curl + jq + redis-cli + clickhouse-client。轻依赖(docker 容器里都有),可读性高。宿主机若缺 jq / redis-cli / clickhouse-client,harness 可以 `docker compose exec` 打到容器里跑,00-bootstrap.sh 负责探测并给出提示。

`set -euo pipefail` + `trap ERR` 打印完整上下文。

### 5.2 文件布局

```
scripts/qa/
  run.sh               # 主入口
  lib.sh               # helper
  e2e-env.sh           # env 导出
  00-bootstrap.sh      # 等 docker 栈 ready
  10-invite-admin.sh   # admin 建 invite code
  20-register.sh       # 注册 + 审核 + 拿 API key
  30-topup.sh          # 充值 + 查余额
  40-campaign.sh       # 建广告 + 更新 + 启动
  50-creative.sh       # 创意 CRUD + pub/sub 订阅验证
  60-reports.sh        # 查 reports(预埋 bid_log fixture)
  70-admin-review.sh   # 创意审核
  99-teardown.sh       # 不清数据,只打印摘要
```

### 5.3 报告格式

`docs/qa/2026-04-14-biz-qa-report.md`:

```markdown
# biz QA 报告 — 2026-04-14

## 栈版本
commit: <git rev>
docker compose services: up/healthy

## Harness 流程执行
[✅] 00-bootstrap    (2.1s)
[✅] 10-invite-admin (0.3s)
...

## 发现的 bug
### B001 · [fixed] handler 500 when ...
- 路由:...
- 复现:...
- 根因:...
- 修复 commit:...
- 回归测试:...

### B002 · [recorded, not fixed]
- 原因:本轮不修 ...
- 跟踪:TODO

## 覆盖
- handler e2e tests: N tests, N passed
- harness steps: N steps, N passed

## 未覆盖(明确)
- engine 侧
- dsp.billing / SendLoss
- 性能 / 压测
```

### 5.4 bug 分档规则

**阻塞(就地修)**
- HTTP 500 / panic
- 契约违反:状态机错乱、余额扣成负数、pub/sub 不发
- 权限绕过
- 数据丢失或错误覆盖

**非阻塞(记报告)**
- 响应码语义细节(400 vs 422)
- 错误消息文案
- 前端纯 UI 问题(归 P4)
- 性能(除非慢到测试超时)

**阻塞修复流程硬性要求**:补 Go 测试(红) → 改代码(绿) → 跑 harness 再走一遍。

### 5.5 pub/sub 订阅验证(harness 侧)

```bash
redis-cli -p 17380 SUBSCRIBE campaign:updates > "$TMP/updates.log" &
SUB_PID=$!
sleep 0.3
curl ... /api/v1/creatives ...
sleep 0.5
kill $SUB_PID
grep -q "campaign_id.*$CAMPAIGN_ID.*updated" "$TMP/updates.log" \
  || fail "creative POST did not publish campaign:updates"
```

### 5.6 数据预埋

harness 用 `clickhouse-client` 注入 10 条 `bid_log` fixture(单个 campaign,近 1 小时内),保证 reports 步骤能看到数据,同时不耦合 engine。

---

## 6. 前端 QA(P4)

### 6.1 范围

| 路径 | 文件 | 备注 |
|---|---|---|
| `/` | `page.tsx` | 首页 / 登录落地(未登录态也要测) |
| `/campaigns` | `campaigns/page.tsx` | 广告列表 |
| `/campaigns/new` | `campaigns/new/page.tsx` | 新建广告 |
| `/campaigns/[id]` | `campaigns/[id]/page.tsx` | 广告详情 / 编辑 / 启停 / 创意 |
| `/reports` | `reports/page.tsx` | 报表入口 |
| `/reports/[id]` | `reports/[id]/page.tsx` | 单广告报表 |
| `/analytics` | `analytics/page.tsx` | 实时分析 |
| `/billing` | `billing/page.tsx` | 余额/充值/交易 |
| `/admin` | `admin/page.tsx` | 管理首页 |
| `/admin/invites` | `admin/invites/page.tsx` | 邀请码 |
| `/admin/agencies` | `admin/agencies/page.tsx` | 广告主审核 |
| `/admin/creatives` | `admin/creatives/page.tsx` | 创意审核 |
| `/admin/audit` | `admin/audit/page.tsx` | 审计日志 |

**必测未登录态**:首页 `/` 无 API key 必须能渲染登录 UI,不能白屏 401(上一轮漏过)。

### 6.2 每页四维度 + 契约漂移

按 CLAUDE.md 的 /browse Verification Standard:

**0. 用户直觉检查**(第一关卡,过不了不进下面)

**1. 交互验证**
- 表单字段:空 / 合法 / 非法 各一次
- 按钮:每个 action 点击,验证请求 + 页面变化
- 链接:列表页进详情再返回,验证状态/滚动
- 权限跨页面:未登录→被拦、登录后持久化、刷新保留
- `snapshot -D` 对比 DOM before/after

**2. 视觉合规**
每页至少 3 个元素 `getComputedStyle` 抽查:字体、字号、颜色、间距、断点。抽查对象优先:主要按钮、卡片容器、表单输入框、导航。

三档响应式:1440 / 768 / 375。

**3. 数据正确性**
页面数字对 DB 实际值:campaigns 列表 vs pg COUNT、billing 余额 vs `advertiser_balances`、reports 数字 vs ClickHouse 聚合、admin 广告主列表 vs pg COUNT、audit 日志 vs `audit_log`。

**4. 契约漂移扫描**(本轮新增)
对比三方:
1. `cmd/api/main.go` 路由注册(权威后端)
2. `docs/generated/openapi3.yaml`(swagger)
3. `web/lib/api-types.ts`(从 yaml 生成)

漂移处理:
- 阻塞性(前端实际在用漂移字段) → 现场 regenerate openapi + ts
- 非阻塞(文档陈旧) → 记报告

具体 regenerate 命令走现有 Makefile/scripts。P4 第一步查清楚,不存在就记报告。

### 6.3 修复流程

前端 bug 不走 Go TDD:
1. /qa 浏览器复现
2. 改 `web/app/**/*.tsx` 或 `globals.css`
3. 热重载验证
4. 再跑对应页面 /qa 确认无回归
5. commit

后端根因切换到后端 TDD 流程。

### 6.4 成功标准

- 所有页面四维度通过
- DESIGN.md 合规差异为 0(或有理由的记录)
- 契约三方无阻塞性漂移
- /qa 报告章节合入 `docs/qa/2026-04-14-biz-qa-report.md`

---

## 7. 验证循环 + 风险

### 7.1 每 Phase verification loop

```
loop (max 5 rounds):
  1. requesting-code-review     → 修 Critical/Important
  2. verification-before-completion
       - docker 栈起来
       - P1/P2: go test -tags=e2e -count=1 ./internal/handler/...
       - P3:     bash scripts/qa/run.sh
       - P4:     /qa 扫本 Phase 涉及的页面
  3. /qa(前端涉及时)
  if 有任何修复: 回步骤 1
  if 整轮零问题:退出,进下一 Phase
```

"整轮零问题"必须是**修完后再跑的那一轮**全绿才算。

### 7.2 Phase 间环境

不重置。docker 栈持续运行,跨 Phase 共享数据(唯一前缀保证隔离)。应急路径:`docker compose down -v && docker compose up -d`。

### 7.3 P5 final 循环

1~3 步 + final-code-review(全量)+ /browse 截图(三档响应式 + 关键交互 before/after)+ 报告最终版 + 契约修改清单(待合回 main 时应用)。

### 7.4 风险清单

- **R1 docker 启动时序**:bootstrap 脚本 health check 循环 60s;`mustDeps` 失败带人类可读提示
- **R2 mux 抽取顺序错**:作为 P2 第一 task 单独 commit,抽完先验 `cmd/api` 启动,出问题回退
- **R3 ClickHouse fixture 污染**:所有 fixture 绑独立 advertiser+campaign,查询按 `advertiser_id` filter
- **R4 Next.js 非标准版本**:改前端前读 `web/AGENTS.md` + `web/CLAUDE.md` + 相关 next 文档
- **R5 DESIGN.md 权威值未读**:P4 第一步读 DESIGN.md 把权威值抄入报告基准章节;不完整则记"按现有平均兜底"
- **R6 OpenAPI regenerate 未知**:P4 第一步查 Makefile;不存在则记报告,本轮不引入新工具

### 7.5 已知未知(P2 开工时现场确认)

1. `reporting.Store` 是否有公开的写接口,否则 raw clickhouse-go client
2. `campaign.Store` 现存 fixture 方法
3. `handler.UploadFileServer()` 底下的存储路径,测试 upload 不污染宿主
4. OpenAPI regenerate 流程是否在 Makefile 里

以上 1-4 项在 P2 第一个 task 结束后明确落地,不在设计阶段猜。

---

## 8. 禁止事项(本轮硬性)

- 不碰 engine 代码(`cmd/bidder`, `cmd/consumer`, `internal/bidder`, `internal/events`)
- 不改 `docs/contracts/biz-engine.md`(发现契约问题记录下来,P5 报告里列清单,最后合回 main 时在 main 上统一改)
- 不改 `docker-compose.override.yml` / `.env`
- 不碰 `dsp.billing` / `SendLoss`(契约 §3.1/3.2)
- 不给出"未修 pub/sub 但声称修好了"的报告(必须有订阅到消息的证据)
