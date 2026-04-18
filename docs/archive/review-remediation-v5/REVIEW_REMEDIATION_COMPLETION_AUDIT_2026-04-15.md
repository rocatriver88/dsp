# REVIEW_REMEDIATION 完成度复核（2026-04-15）

## 结论

截至 2026-04-15,本仓库**尚不能**判定为“已按 [REVIEW_REMEDIATION_PLAN_2026-04-14.md](./REVIEW_REMEDIATION_PLAN_2026-04-14.md) 全部完成整改”。

核心服务端整改基本已落地：

- 租户隔离主线已实现
- advertiser 读接口已切到 DTO 脱敏
- `Config.Validate() error` 已落地
- `workerCtx` 生命周期拆分已落地
- `effective_delivery` 聚合口径已落地
- click 去重与 `producer.WaitInflight` 已落地
- `go test ./... -count=1` 当前通过

但仍有 3 类收口项未完成：

1. 生成契约未同步
2. 前端 lint 未通过
3. 文档与代码行为仍有漂移

因此本轮状态应定义为：**主要代码整改完成，发布前收口未完成**。

---

## 已确认完成的部分

### P0 安全边界

- advertiser 读接口使用 `AdvertiserResponse`
  - `internal/handler/campaign.go`
- admin advertiser list 使用 `AdvertiserResponse`
  - `internal/handler/admin.go`
- self-service billing 已按 auth context 收口
  - `internal/handler/billing.go`
- report 五条路径已统一做 owner check
  - `internal/handler/report.go`
- creative `list/create/update/delete` 已做 owner check
  - `internal/handler/campaign.go`

### P0 配置与 admin 安全

- `Config.Validate() error` 已实现
  - `internal/config/config.go`
- `cmd/api` 和 `cmd/bidder` 启动时都调用了 `cfg.Validate()`
  - `cmd/api/main.go`
  - `cmd/bidder/main.go`
- `admin-secret` fallback 已删除
  - `internal/handler/admin_auth.go`
- `admin_token` query 已删除
  - `internal/handler/admin_auth.go`

### P1 事件语义与生命周期

- `handleWin` 不再发送重复 impression
  - `cmd/bidder/main.go`
- reporting 已改为 `countDistinctIf(request_id, event_type IN ('win', 'impression'))`
  - `internal/reporting/store.go`
- click 去重已实现
  - `cmd/bidder/main.go`
- producer inflight drain 已实现
  - `internal/events/producer.go`
  - `cmd/bidder/main.go`
- `workerCtx` 已用于长期后台 loop
  - `cmd/api/main.go`
  - `cmd/bidder/main.go`

### 测试现状

- `go test ./... -count=1` 通过
- 已新增大量 e2e / integration / lifecycle 测试文件

---

## 未完成项

## 1. OpenAPI 与前端生成类型未同步

这是当前最明确的未收口项。

### 现象

运行时代码已经把 advertiser 读接口声明为 `handler.AdvertiserResponse`：

- `internal/handler/campaign.go`
- `internal/handler/admin.go`

但生成物仍把 advertiser 读接口绑定到旧 schema `github_com_heartgryphon_dsp_internal_campaign.Advertiser`，并继续暴露 `api_key`：

- `docs/generated/openapi3.yaml`
- `docs/generated/swagger.yaml`
- `web/lib/api-types.ts`
- `web/lib/api.ts`

### 直接影响

- 契约层仍声称 advertiser 读接口可能返回 `api_key`
- 前端类型仍基于旧 advertiser schema
- 这与 V5 中“DTO 脱敏 + 接口形状变化同步生成物”的要求不一致

### 必须补做

1. 运行 `make api-gen`
2. 检查以下结果是否更新正确：
   - `docs/generated/swagger.yaml`
   - `docs/generated/openapi3.yaml`
   - `web/lib/api-types.ts`
3. 修正 `web/lib/api.ts` 中 `Advertiser` 类型引用，确保不再依赖旧 schema
4. 复核以下接口在生成契约中的响应模型：
   - `GET /api/v1/advertisers/{id}`
   - `GET /api/v1/admin/advertisers`
   - `POST /api/v1/advertisers`
   - `POST /api/v1/admin/registrations/{id}/approve`

### 验收标准

- 读接口 advertiser schema 不再含 `api_key`
- 仅“创建/审批通过”这两条一次性披露路径仍含 `api_key`
- `web/lib/api.ts` 不再把读接口 advertiser 绑定到旧 persistence schema

---

## 2. 前端 lint 未通过

这说明“建议验证命令”还没有闭环。

### 实际验证结果

执行：

```powershell
cd web
npm run lint
```

结果失败，至少包括以下错误：

- `web/app/_components/ApiKeyGate.tsx`
- `web/app/admin/audit/page.tsx`
- `web/app/admin/page.tsx`
- `web/app/campaigns/page.tsx`
- `web/app/reports/page.tsx`

主要错误类型：

- `react-hooks/set-state-in-effect`

另有若干 warning：

- `react-hooks/exhaustive-deps`
- `@next/next/no-img-element`

### 必须补做

1. 修复所有 ESLint error
2. 至少把当前阻断 lint 的 5 个 error 清零
3. 重新执行：

```powershell
cd web
npm run lint
```

### 验收标准

- `npm run lint` exit code = 0
- 如果保留 warning，需要确认项目允许 warning；否则一并清理

---

## 3. 文档与代码行为仍有漂移

当前至少有两处文档没有同步到代码现状。

### 3.1 `docs/contracts/biz-engine.md`

文档仍写着：

- `handleWin` 在成交时同时写 `dsp.bids` 和 `dsp.impressions`

但代码现状已经不是这样：

- `cmd/bidder/main.go` 中已删除重复 impression 发送

### 需要修改

把 `biz-engine.md` 中关于 `handleWin` 双写的现状描述，改为：

- 历史背景：曾经双写
- 当前状态：Step B 已完成，`handleWin` 只发 `win`
- `effective_delivery` 继续保留作为聚合口径

### 3.2 `docs/runtime.md`

文档当前写的是：

- buffer 满时“丢最老(file truncate)”

但代码实际行为是：

- 直接丢当前新事件，并记录 `dropping event`

对应位置：

- `internal/events/producer.go`

### 需要修改

把 `runtime.md` 中 buffer 满时行为改为和代码一致：

- 当前实现：达到上限后拒绝写入当前新事件
- 不要再描述成 “truncate 最老文件”

### 验收标准

- `docs/contracts/biz-engine.md` 与当前 `handleWin` 行为一致
- `docs/runtime.md` 与 `producer.bufferToDisk` 行为一致

---

## 建议 Claude 继续修复的顺序

1. 先修生成契约
   - 因为这是对外接口面
2. 再修前端 lint
   - 因为这是发布前阻断项
3. 最后修文档漂移
   - 因为这不阻断运行，但会误导后续维护

---

## 建议执行命令

### 契约同步

```powershell
make api-gen
```

### 后端验证

```powershell
go test ./... -count=1
```

### 前端验证

```powershell
cd web
npm run lint
```

### 如需完整链路验证

```powershell
./scripts/test-env.sh verify
```

---

## 最终验收判定标准

只有同时满足以下条件，才可以把整改任务标记为“已按要求完成”：

1. `go test ./... -count=1` 通过
2. `cd web && npm run lint` 通过
3. `make api-gen` 生成物已同步
4. advertiser 读接口生成契约不再暴露 `api_key`
5. `docs/contracts/biz-engine.md` 与 `docs/runtime.md` 已同步到现状代码行为

在以上 5 条全部满足之前，本轮整改状态应保持为：

**整改主线已完成，但收口未完成。**
