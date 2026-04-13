# 模块架构说明

## 1. 系统总览
该仓库组织成一个多服务 DSP 平台：

- `cmd/api` 暴露广告主 API、管理 API、上传、报表和账务接口
- `cmd/bidder` 处理竞价请求和回调流量
- `cmd/consumer` 将事件流写入 ClickHouse 用于分析
- `web/` 提供操作侧 UI
- `cmd/autopilot`、`cmd/exchange-sim`、`cmd/simulate`、`cmd/resetbudget` 提供验证与运维辅助

运行时依赖主要包括：PostgreSQL 用于事务状态，Redis 用于热路径控制与协调，Kafka 用于事件传输，ClickHouse 用于报表查询。

## 2. API 服务架构
`cmd/api/main.go` 负责装配控制面主服务。

### 公共 API
公共 mux 在 `/api/v1/` 下提供广告主侧能力，包括：
- advertisers
- campaigns
- creatives
- reports
- analytics
- billing
- uploads
- registration

鉴权由 `internal/auth` 里的 API Key 机制处理。HTTP 栈中同时接入了 CORS、request ID、结构化日志和限流。

### 内部与管理 API
管理路由通过 `handler.AdminAuthMiddleware` 保护，并暴露在独立 internal 端口上。这样可以把注册审批、素材审核、邀请码、审计日志、熔断控制和健康检查等运营接口与广告主公共接口隔离开。

### 领域模块
API 服务主要由以下包组成：
- `internal/campaign`：广告主、Campaign、素材持久化
- `internal/billing`：余额、充值与花费记账
- `internal/registration`：邀请码与接入流程
- `internal/reporting`：基于 ClickHouse 的报表查询
- `internal/audit`：敏感操作审计日志
- `internal/autopause`、`internal/reconciliation`、`internal/guardrail`：后台保护任务

## 3. Bidder 架构
`cmd/bidder/main.go` 是负责请求时决策的数据面服务。

### 请求流
1. 接收 OpenRTB 或交易所归一化后的请求
2. 解析设备与地域上下文
3. 通过 `internal/bidder` 加载活动 Campaign 状态
4. 运行 `internal/antifraud` 反作弊过滤
5. 通过 `internal/budget` 执行预算与频控限制
6. 应用 pacing 和性能信号驱动的出价策略
7. 执行 guardrail 检查
8. 生成竞价响应并发送事件
9. 处理带 HMAC 校验的 win、click、convert 回调

### 支撑组件
- `internal/bidder/loader.go`：维护活动 Campaign 状态
- `internal/bidder/strategy.go`：结合性能和 pacing 信号调整出价
- `internal/bidder/statscache.go`：把 ClickHouse 的 CTR/CVR 信号刷新到 Redis
- `internal/exchange`：抽象交易所协议归一化
- `internal/events`：提供 Kafka 事件生产、重放和 dead-letter 支持

## 4. 分析与报表链路
报表链路拆分为在线事件生产和离线查询服务两部分。

- Bidder 将事件写入 Kafka topic
- `cmd/consumer` 读取分析 topic，并将标准化事件写入 ClickHouse
- `internal/reporting` 提供 Campaign 统计、小时报表、地域分布、竞价透明度、归因和总览查询
- 导出接口和 SSE 分析能力建立在 reporting 包之上

这种拆分让 bidder 的延迟敏感逻辑与较重的报表查询解耦。

## 5. Web 应用架构
`web/` 下的前端使用 Next.js App Router。

- `web/app/`：广告主和管理端路由
- `web/lib/`：共享客户端逻辑，包括生成的 API 类型
- `web/public/`：静态资源

前端依赖后端生成的 OpenAPI 契约。只要 API 形状变化，就必须执行 `make api-gen` 同步刷新 `docs/openapi3.yaml` 和 `web/lib/api-types.ts`。

## 6. 横切能力
- `internal/config`：集中处理基于环境变量的配置
- `internal/observability`：结构化日志与 request ID
- API 与 bidder 服务都暴露 Prometheus 指标
- Docker Compose 文件定义本地平台拓扑
- `scripts/test-env.sh` 负责隔离集成验证环境编排
- `cmd/autopilot` 提供场景化端到端验证

## 7. 架构特征

### 优势
- 控制面、竞价面、分析写入和 UI 分层清晰
- campaign、billing、reporting、fraud、guardrail 等领域边界较清楚
- 已具备本地可观测性与重放机制

### 当前约束
- 交易所覆盖仍然偏少
- 合规与账务强化仍在 roadmap 中
- 部分产品流达到的是运营可用深度，而不是完整企业级深度

## 8. 推荐阅读顺序
新贡献者建议按以下顺序阅读：
1. `cmd/api/main.go`
2. `cmd/bidder/main.go`
3. `cmd/consumer/main.go`
4. `internal/handler/`
5. `internal/bidder/`
6. `internal/reporting/`
7. `web/`
8. `TODOS.md`
