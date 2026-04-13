# DSP 平台

## 项目概览
该仓库实现了一个多服务的 Demand-Side Platform（DSP），覆盖广告主接入、Campaign 管理、实时竞价、报表分析、账务基础能力以及管理后台操作。后端使用 Go，控制台使用 Next.js，同时仓库内包含本地环境、可观测性和验证工具链。

## 系统组成
- `cmd/api`：广告主 API、管理 API、报表、账务、上传
- `cmd/bidder`：OpenRTB 竞价与 win、click、convert 回调处理
- `cmd/consumer`：Kafka 到 ClickHouse 的分析链路写入
- `web/`：广告主与管理端控制台
- `scripts/`、`deploy/`、`monitoring/`：本地环境与运维支持

核心运行依赖包括 PostgreSQL、Redis、Kafka、ClickHouse、Prometheus 和 Grafana。

## 已实现能力
- 广告主接入、API Key 鉴权、Admin Token 保护的管理操作
- Campaign 生命周期管理，包括定向、预算、启动与暂停控制
- 素材管理、上传流程与审核状态
- OpenRTB 实时竞价，以及预算、频控、反作弊和护栏检查
- Campaign 统计、小时报表、地域分布、归因、透明度、导出与分析流
- 充值、余额、流水、花费跟踪与对账任务等账务基础能力
- 注册审核、素材审核、广告主管理、邀请码、熔断控制、审计与健康检查

## 本地验证
```powershell
make test
cd web; npm run lint; npm run build
make api-gen
./scripts/test-env.sh verify
```

如需启动完整隔离本地环境：

```powershell
./scripts/test-env.sh all
```

## 文档入口
- [项目概览](./PROJECT_OVERVIEW.md)
- [贡献指南](./CONTRIBUTING.md)
- [评审工作流](./REVIEW_WORKFLOW.md)
- [发布检查清单](./RELEASE_CHECKLIST.md)
- [文档索引](./docs/README.md)
- [功能清单](./docs/project-feature-inventory.md)
- [完成度报告](./docs/current-version-completion-report.md)
- [模块架构说明](./docs/module-architecture.md)
- [OpenAPI 契约](./docs/generated/openapi3.yaml)

## 当前状态
该项目已经超出原型阶段，核心 DSP 流程和主要操作面都已实现。当前剩余工作主要集中在合规、更多交易所接入、账务强化和生态扩展，而不是平台基础能力缺失。
