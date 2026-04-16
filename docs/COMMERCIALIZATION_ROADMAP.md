# DSP 商用化路线图

> 基于 2026-04-16 Claude + Codex 双模型独立审查共识。
> 两个 AI 独立审查 agreement rate ~70%，以下优先级为双方一致结论。

## 当前状态

安全基础扎实（V5/V5.1/V5.2 全套审计修复已落地），架构合理（API/Bidder/Consumer 三服务分离），
测试覆盖 68 个文件，Prometheus 20+ 指标，Kafka at-least-once 管道完整。
**定位：功能面较全的原型/内测平台，约 70-80% 商用就绪。**

---

## Phase 1: 计费正确性（BLOCKER — 上线前必须修）

不修这个就上线 = 真金白银打水漂。

| Task | 问题 | 文件 | 修复方案 |
|------|------|------|----------|
| 1.1 总预算实时约束 | PipelineCheck 只扣日预算，BudgetTotalCents 无实时检查 | `internal/bidder/engine.go:195-213`, `internal/budget/budget.go` | Redis Lua 脚本中加 total budget 原子扣减 |
| 1.2 预付费余额实时约束 | 只在 campaign 启动时查余额，运行中无检查 | `internal/handler/campaign.go:326-332` | Redis 维护 advertiser 级余额，win/click 时原子扣减 |
| 1.3 Reconciliation 落账 | RunDaily() 存在但未调度，从不 debit advertiser balance | `internal/reconciliation/reconciliation.go:126-184`, `cmd/api/main.go` | 接入定时调度 + 调用 billingSvc.RecordSpend |
| 1.4 Bid-time 预扣修正 | engine.go 在出价阶段调 PipelineCheck 扣预算 | `internal/bidder/engine.go` | 改为 bid 时只检查不扣减，win 时扣减 |
| 1.5 频控竞态修复 | INCR-before-check 导致频控计数膨胀 | `internal/budget/budget.go:111-128` | 改为 Lua 原子 check-then-increment |

**预估工作量：** 人工 1-2 周 / AI 辅助 1-2 天

---

## Phase 2: 投放约束热路径（BLOCKER）

没有这些，campaign 会在错误的时间、给错误的人、展示错误的素材。

| Task | 问题 | 文件 | 修复方案 |
|------|------|------|----------|
| 2.1 航期控制 | start_date/end_date 未进 bidder | `internal/campaign/store.go:209-235`, `internal/bidder/engine.go` | ListActiveCampaigns 加日期过滤 + engine 二次校验 |
| 2.2 创意尺寸匹配 | 总是取 Creatives[0]，不匹配 imp 尺寸 | `internal/bidder/engine.go:224` | 按 imp.Banner.W/H 过滤候选创意 |
| 2.3 定向维度接入 | matchesTargeting 只用 geo/OS，browser/time_schedule/audience 未接入 | `internal/bidder/engine.go:279-298` | 逐维度接入，audience 调 segment.go |
| 2.4 安全标记匹配 | requireSecure/siteCategories 解析后丢弃 | `internal/bidder/engine.go` | 按 OpenRTB 规范过滤 |

**预估工作量：** 人工 1 周 / AI 辅助 1 天

---

## Phase 3: 身份与接入（BLOCKER/IMPORTANT）

API key 不是用户系统。没有支付网关不是真实充值。

| Task | 问题 | 修复方案 |
|------|------|----------|
| 3.1 用户账号系统 | 当前只有 API key，无登录/注册/密码 | 加 users 表，支持邮箱+密码登录，JWT session |
| 3.2 组织与 RBAC | 单 API key = 单用户，无多人协作 | 加 organizations 表，角色（admin/operator/viewer） |
| 3.3 支付网关集成 | HandleTopUp 直接加余额，无真实支付 | 集成 Stripe/Alipay，充值走支付回调确认 |
| 3.4 TLS 终止 | 全链路 HTTP 明文 | 加 Caddy/nginx 反向代理，HTTPS 证书自动续期 |
| 3.5 数据库连接加密 | DSN 强制 sslmode=disable | 按环境配置 sslmode=require |

**预估工作量：** 人工 2-3 周 / AI 辅助 3-5 天

---

## Phase 4: Exchange 接入（IMPORTANT）

只有 self exchange 不是真正的 DSP。

| Task | 问题 | 修复方案 |
|------|------|----------|
| 4.1 真实 ADX 接入 | DefaultRegistry 只注册 self | 接入 1-2 个真实交易所（Google ADX / 国内主流 ADX） |
| 4.2 Impression 回调 | 无真实展示回调，delivery=win | 实现真正的 impression tracking pixel |
| 4.3 供应路径验证 | ads.txt/sellers.json 未验证 | 验证供应链合规性 |
| 4.4 多币种支持 | 硬编码 CNY | 支持 bidfloor 币种转换 |

**预估工作量：** 人工 2-4 周 / AI 辅助 3-5 天（per exchange）

---

## Phase 5: 生产基础设施（BLOCKER for scale）

不修这些可以小流量测试，但无法承受真实流量。

| Task | 问题 | 修复方案 |
|------|------|----------|
| 5.1 K8s 部署 | 只有 docker-compose | Helm chart + 滚动更新 + 回滚 |
| 5.2 状态组件 HA | PG/Redis/Kafka/CH 全单点 | 主从/集群/多 AZ |
| 5.3 备份与恢复 | 无任何备份策略 | PG PITR + Redis AOF + CH 备份 + 演练 |
| 5.4 ClickHouse 批量写入 | consumer 单条插入 | 改为批量 insert（每 1s 或 1000 条） |
| 5.5 告警规则 | 无 Alertmanager/告警条件 | 预算超支/高错误率/reconciliation 偏差告警 |
| 5.6 Guardrail fail-closed | Redis 故障时 fail-open 可能无限花费 | 生产环境改为 fail-closed |

**预估工作量：** 人工 3-4 周 / AI 辅助 1-2 周

---

## Phase 6: 产品增强（NICE-TO-HAVE）

| Task | 描述 |
|------|------|
| 6.1 Video/CTV 广告类型 | VAST 支持 |
| 6.2 转化归因增强 | S2S postback, MMP 集成 |
| 6.3 报表增强 | 跨 campaign 视图, 定时报送, PDF 导出 |
| 6.4 批量操作 | campaign 克隆, 批量创建, 批量暂停 |
| 6.5 审批流 | 素材审核工作流, campaign 上线审批 |
| 6.6 多语言 | i18n 支持 |

---

## 上线里程碑

```
Phase 1 (计费) ──→ Phase 2 (约束) ──→ 内测上线（小流量真实预算）
                                         │
                                    Phase 3 (身份) ──→ Phase 4 (ADX) ──→ 公测
                                                                           │
                                                                      Phase 5 (基础设施) ──→ 正式商用
```

**最快路径：** Phase 1 + Phase 2 完成后即可小流量内测（约 2-3 天 AI 辅助）。
Phase 3 + 4 完成后可公测。Phase 5 完成后可正式商用。

---

## 已具备的优势（双模型一致认可）

- ✅ 安全基础扎实（V5 全套审计修复）
- ✅ 原子 Redis 日预算扣减（Lua 脚本）
- ✅ HMAC 认证的 win/click 回调
- ✅ 优雅关停（五不变量）
- ✅ Kafka at-least-once + DLQ + 磁盘缓冲
- ✅ Campaign 同步架构（pub/sub + 30s 全量 reconciliation + warm-up guard）
- ✅ 反欺诈层（IP 黑名单 + 数据中心过滤 + UA 异常检测）
- ✅ 动态出价策略（win rate pacing + spend pacing）
- ✅ 20+ Prometheus 指标 + Grafana 仪表盘
- ✅ 68 个测试文件 + CI lint/test/build/contract-check
