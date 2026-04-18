# DSP 平台概览

## 项目定位
该仓库实现了一个多服务的 Demand-Side Platform（DSP），重点覆盖广告主接入、Campaign 运营、实时竞价、报表分析和管理后台。整体形态为 Go 后端加 Next.js 控制台，并配套本地环境、验证脚本和可观测性设施。

## 当前产品能力
- 广告主接入与 API Key 范围隔离
- Campaign 创建、生命周期管理与定向控制
- 素材管理、上传与审核流程
- OpenRTB 实时竞价，以及预算、频控、反作弊和护栏检查
- 余额、流水、充值、对账任务等账务基础能力
- Campaign 统计、小时报表、地域分布、归因、透明度与导出
- 注册审核、素材审核、广告主管理、熔断控制、邀请码、审计与健康检查

## 系统形态
- `cmd/api`：广告主 API、管理 API、报表、账务、上传
- `cmd/bidder`：竞价服务与回调处理
- `cmd/consumer`：Kafka 到 ClickHouse 的分析写入
- `web/`：广告主与管理端控制台
- `scripts/`、`deploy/`：本地环境、部署与可观测性资产（监控配置位于 `deploy/monitoring/`）

核心运行依赖是 PostgreSQL、Redis、Kafka、ClickHouse、Prometheus 和 Grafana。

## 交付状态
该项目已经超出原型阶段。核心 DSP 工作流已经打通，仓库中也具备了较接近生产形态的能力，例如结构化日志、指标暴露、重放机制、隔离测试环境脚本以及浏览器验证报告。

当前主要剩余工作集中在扩展和强化，而不是系统骨架缺失：
- 合规与数据治理
- 更多真实交易所接入
- 更深入的账务自动化和支付能力
- SDK 分发与更完整的生态工具

## 仓库入口文档
- 功能与能力总览:[`feature-inventory.md`](./feature-inventory.md)
- 架构说明:[`module-architecture.md`](./module-architecture.md)
- OpenAPI 契约:[`generated/openapi3.yaml`](./generated/openapi3.yaml)
- 历史完成度快照:[`archive/`](./archive/)

## 常用本地验证
```powershell
make test
cd web; npm run lint; npm run build
make api-gen
./scripts/test-env.sh verify
```
