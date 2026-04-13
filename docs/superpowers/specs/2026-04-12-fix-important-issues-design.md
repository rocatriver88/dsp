# Fix 7 Important Issues from Code Review

## 概述

修复全量 code review 中发现的 7 个 Important 级别问题。这些问题不影响编译和单元测试，但在生产环境中会导致数据不完整、安全隐患或 UX 问题。

---

## I1: Admin Token Gate 服务端校验

**问题：** 前端 Admin 登录接受任何非空字符串，不验证 token 是否有效。用户会短暂看到 admin 界面后才被 401 踢出。

**修复：** 登录时先调 `GET /api/v1/admin/health`，token 正确才存储到 localStorage。失败则显示错误提示。

**文件：** `web/app/admin/layout.tsx`

---

## I2: 对账遗漏已暂停 Campaign

**问题：** `RunHourly` 和 `RunDaily` 调用 `ListActiveCampaigns()` 只返回当前 `status='active'` 的 campaign。白天活跃但晚上暂停的 campaign 会被跳过。

**修复：** 新增 `ListCampaignsActiveOnDate(ctx, date)` 方法，查询 `WHERE status IN ('active', 'paused') AND updated_at >= dayStart`，确保当天有过活动的 campaign 都参与对账。

**文件：** `internal/campaign/store.go`、`internal/reconciliation/reconciliation.go`

---

## I3: 护栏 PreCheck 语义不清

**问题：** `engine.go` 调用 `guardrail.CheckBid(ctx, 0)` 做 pre-check，传入 `bidCPMCents=0` 让出价上限检查变成 no-op。代码意图不明确。

**修复：** 拆分为两个方法：
- `PreCheck(ctx) CheckResult` — 只检查熔断器 + 全局预算
- `CheckBidCeiling(ctx, bidCPMCents) CheckResult` — 只检查出价上限

Engine 中 pre-check 调 `PreCheck`，每个 candidate 调 `CheckBidCeiling`。

**文件：** `internal/guardrail/guardrail.go`、`internal/guardrail/guardrail_test.go`、`internal/bidder/engine.go`

---

## I4: Admin 列表端点无分页

**问题：** `HandleListAdvertisers`、`HandleListInviteCodes`、`HandleListCreativesForReview` 返回无限制的全量结果。

**修复：** 三个端点都加 `?limit=N&offset=N` query 参数支持，默认 `limit=100`。Store 方法相应加 `LIMIT $1 OFFSET $2`。

**文件：** `internal/handler/admin.go`、`internal/campaign/store.go`

---

## I5: CSV 导出行数硬编码

**问题：** `HandleExportBidsCSV` 硬编码 `limit=10000`，不可配置。

**修复：** 从 `?limit=N` query 参数读取，默认 10000，上限 50000。

**文件：** `internal/handler/export.go`

---

## I6: 审计 Record() 静默丢失

**问题：** `audit.Logger.Record()` 失败只打日志，无监控指标。持续失败时无人知道。

**修复：** 加 Prometheus counter `dsp_audit_errors_total`，每次 Record 失败递增。运维可通过 Grafana 告警发现。

**文件：** `internal/audit/audit.go`

---

## I7: ListAllAdvertisers 缺 Campaign 统计

**问题：** Admin 概览页的代理商列表没有活跃 campaign 数和总花费，显示硬编码 0。

**修复：** `ListAllAdvertisers` 加 `LEFT JOIN campaigns` 返回 `active_campaigns` 和 `total_spent_cents`。`Advertiser` struct 加对应字段。前端 admin overview 页使用真实数据。

**文件：** `internal/campaign/store.go`、`internal/campaign/model.go`、`web/app/admin/page.tsx`
