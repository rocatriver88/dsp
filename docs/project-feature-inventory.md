# DSP 平台功能清单

## 适用范围
本文汇总仓库中已经实现的功能面，适合作为产品、工程、QA 和评审交接时的能力总览。

## 1. 平台形态
- `cmd/api` 提供广告主侧与管理侧 HTTP API
- `cmd/bidder` 处理 OpenRTB 竞价、win、click、convert 回调
- `cmd/consumer` 负责 Kafka 到 ClickHouse 的分析链路写入
- `web/` 提供广告主与管理端控制台
- `cmd/autopilot`、`cmd/exchange-sim`、`cmd/simulate`、`cmd/resetbudget` 提供运维与验证辅助工具

## 2. 广告主接入与权限控制
- 广告主创建与查询 API
- 基于 `X-API-Key` 的广告主鉴权
- 基于 `X-Admin-Token` 的管理与内部接口鉴权
- 自助注册、审批和拒绝流程
- 邀请码发放、使用次数统计与消费
- API 与存储层面的基础多租户隔离

## 3. Campaign 管理
- Campaign 创建、列表、详情、更新、启动、暂停
- 覆盖 `draft`、`active`、`paused`、`completed`、`deleted` 的状态迁移
- 总预算与日预算控制
- 支持地域、OS、设备、频控、人群包含/排除，以及年龄和性别等定向字段

## 4. 计费与花费逻辑
- 支持 CPM、CPC、oCPM 三种计费模型
- 通过有效 CPM 统一竞价排序逻辑
- CPC 在 click 链路扣费
- CPM 与 oCPM 在 win 或 impression 链路扣费
- 包含余额查询、充值、流水、花费记账、发票基础模型和对账任务

## 5. 素材与资源管理
- 素材创建、更新、删除与查询
- 待审、通过、拒绝三种审核状态
- 上传接口与 `/uploads/` 静态文件服务(落盘位置 `var/uploads/`)
- 支持 banner、native、splash、interstitial 等素材类型
- 原生广告支持 title、description、icon、image、CTA 等字段

## 6. 竞价、护栏与反作弊
- 标准 `/bid` 入口与按交易所区分的 `/bid/{exchange_id}` 适配入口
- win、click、convert 回调接口
- 使用 HMAC token 校验回调合法性
- 基于 Redis 的预算与频控校验
- 支持 pacing、胜率反馈和 CTR/CVR 缓存信号的动态出价策略
- 反作弊覆盖黑名单、机房流量、异常 UA 和请求频率控制
- 护栏能力覆盖全局预算上限、出价 ceiling、熔断、低余额和花费尖峰
- 自动暂停规则覆盖预算耗尽和异常指标

## 7. 分析、报表与审计
- Kafka 事件生产，支持重放缓冲与 dead-letter 处理
- ClickHouse 事件写入与报表查询
- 覆盖 Campaign 统计、小时报表、地域分布、竞价透明度、归因、总览、CSV 导出和 SSE 分析流
- 审计日志覆盖 campaign、creative、billing、registration 等敏感操作
- 管理 API 覆盖注册审核、素材审核、广告主列表、充值、邀请码、熔断控制、审计日志和系统健康检查

## 8. 前端与运维能力
- 广告主端页面覆盖总览、Campaign、账务、报表、分析和素材工作流
- 管理端页面覆盖总览、agencies、creatives、invites 和 audit
- 基于 Docker Compose 的本地栈，包括 PostgreSQL、Redis、ClickHouse、Kafka、Prometheus、Grafana
- 具备结构化日志、request ID、`/metrics` 和内部健康检查接口
- 提供 autopilot 验证与 exchange simulation 工具

## 9. 延后或未来方向
- PIPL 合规工作
- 更多真实交易所接入
- 更深入的管理后台运营流程
- 更强的自动化对账能力
- 出价模拟器
- 支付集成
- 低余额告警产品化
- 对外 API SDK
