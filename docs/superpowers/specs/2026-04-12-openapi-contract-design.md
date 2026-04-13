# OpenAPI 合约自动化设计

## 概述

接入 swaggo + openapi-typescript，实现 Go 后端 handler 注释 → OpenAPI spec 自动生成 → TypeScript 类型自动生成的全链路合约自动化。前端不再手写 API 路径和类型。

## 工作流

```
Go handler 注释 (@Summary, @Router, @Param, @Success)
  → swag init -g cmd/api/main.go -o docs/
  → docs/swagger.json + docs/swagger.yaml
  → npx openapi-typescript docs/swagger.yaml -o web/lib/api-types.ts
  → web/lib/api.ts 和 web/lib/admin-api.ts import 生成的类型
```

## 变更清单

### 后端

1. **安装 swaggo：** `go install github.com/swaggo/swag/cmd/swag@latest`

2. **cmd/api/main.go 顶层注释：**
   ```go
   // @title DSP Platform API
   // @version 1.0
   // @description Demand-Side Platform API
   // @host localhost:8181
   // @BasePath /api/v1
   // @securityDefinitions.apikey ApiKeyAuth
   // @in header
   // @name X-API-Key
   // @securityDefinitions.apikey AdminAuth
   // @in header
   // @name X-Admin-Token
   ```

3. **每个 handler 加注释：** 覆盖 `internal/handler/` 下所有 handler（campaign.go, billing.go, admin.go, guardrail.go, export.go）。格式示例：
   ```go
   // HandleListCampaigns godoc
   // @Summary List campaigns
   // @Tags campaigns
   // @Security ApiKeyAuth
   // @Success 200 {array} campaign.Campaign
   // @Router /campaigns [get]
   ```

4. **生成 spec：** `swag init -g cmd/api/main.go -o docs/ --parseDependency --parseInternal`

### 前端

5. **安装依赖：** `npm install -D openapi-typescript`

6. **生成脚本：** `package.json` 加 `"generate:api": "openapi-typescript ../docs/swagger.yaml -o lib/api-types.ts"`

7. **替换手写类型：**
   - `web/lib/api.ts` — 删除手写 interface（Advertiser, Campaign, etc.），改为 `import type { components } from './api-types'`
   - `web/lib/admin-api.ts` — 同上，删除 AdminAdvertiser, CircuitStatus 等手写类型

### 自动化

8. **Makefile 目标：**
   ```makefile
   api-gen:
   	swag init -g cmd/api/main.go -o docs/ --parseDependency --parseInternal
   	cd web && npx openapi-typescript ../docs/swagger.yaml -o lib/api-types.ts
   ```

9. **CI 检查（可选，后续接入）：**
   - `make api-gen` 后 `git diff --exit-code docs/ web/lib/api-types.ts`
   - 有差异 = spec 和代码不同步 = CI 失败

## 不做的事

- 不换 HTTP 框架
- 不引入 runtime validation
- 不改现有 API 行为
- 不修改 bidder 服务的注释（bidder 有独立端口和 HMAC auth，不走 swaggo）

## 成功标准

- `make api-gen` 一键生成 spec + TS 类型
- `web/lib/api.ts` 和 `web/lib/admin-api.ts` 中零手写类型
- 后端改 handler 返回结构 → 重新 `make api-gen` → 前端编译报错（如果类型不匹配）
