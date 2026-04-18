# 文档索引

## 核心文档
- [功能清单](./feature-inventory.md):已实现的产品与系统能力
- [模块架构说明](./module-architecture.md):服务划分、模块职责与数据流
- [biz↔engine 跨系统契约](./contracts/biz-engine.md):业务系统和投放引擎之间的数据流、effective_delivery 聚合规则、三码返回码规范
- [运行时失败策略 & shutdown 不变量](./runtime.md):每个依赖的 fail-open / fail-closed 判断、Producer.Go / WaitInflight 使用规范、五步 shutdown 顺序
- [手工 QA checklist](./qa-checklist.md):前端回归验证清单(web/ 无自动化测试框架时的兜底)
- [项目概览](./OVERVIEW.md):顶层定位、形态与能力总览
- [评审工作流](./REVIEW_WORKFLOW.md):Codex 与 Claude Code 的评审闭环
- [发布检查清单](./RELEASE_CHECKLIST.md):合并与发版检查门槛
- [商业化路线图](./COMMERCIALIZATION_ROADMAP.md):商业化推进与里程碑
- [../CONTRIBUTING.md](../CONTRIBUTING.md):贡献入口文档

## API 契约
- `generated/swagger.yaml` / `generated/swagger.json`:生成的 OpenAPI 2 产物
- `generated/openapi3.yaml`:前端类型生成流程使用的 OpenAPI 3 契约
- `generated/docs.go`:生成的 Go swagger 元数据

如果修改了 API handler 或 swagger 注解,请执行 `make api-gen` 重新生成这些文件。

## 归档资料 (`archive/`)
- [`archive/review-remediation-v5/`](./archive/review-remediation-v5/):V5 评审整改全套(2026-04-14 基线 + V5.1 热修 + V5.2A/B/C + 完成度审计)
- [`archive/REVIEW_REMEDIATION_PLAN_2026-04-14/`](./archive/REVIEW_REMEDIATION_PLAN_2026-04-14/):V5 基线之前的 V1-V4 历史版本
- [`archive/qa-screenshots/`](./archive/qa-screenshots/):归档的 QA 截图批次
- [`archive/browse-reports/`](./archive/browse-reports/):归档的浏览器验证报告
- [`archive/superpowers/`](./archive/superpowers/):归档的 plans / specs / reports,约定见该目录下 README
- [`archive/2026-04-13-completion-report.md`](./archive/2026-04-13-completion-report.md):历史完成度快照

## 当期规划 (`superpowers/`)
- [`superpowers/plans/`](./superpowers/):当期实现计划(完成后会移入 archive)
- [`superpowers/specs/`](./superpowers/):当期设计规范

## 设计资产 (`design/`)
- [`design/figma-screens/`](./design/figma-screens/):Figma 设计稿截图,配合根目录 `DESIGN.md`

## 推荐阅读顺序
1. [项目概览](./OVERVIEW.md)
2. [功能清单](./feature-inventory.md)
3. [模块架构说明](./module-architecture.md)
4. `generated/openapi3.yaml`
5. `../TODOS.md`

## 维护说明
- 叙述性文档要与 `cmd/`、`internal/`、`web/` 和 `TODOS.md` 保持一致
- 除非生成源发生变化,不要手工修改 OpenAPI 生成产物
- 浏览报告、一次性计划和临时过程文档统一放在 `archive/`
- 新增重大工作流或子系统时,请同步更新本索引和架构说明

---

## 附录 A:Codex 评审分流模板

用于 Codex / Claude Code 评审后结构化记录问题与修复。

```markdown
# Codex 评审分流模板

## 摘要
- 分支:
- 评审命令:
- 评审日期:
- 整体风险:
- 是否可合并:是 / 否 / 修复后可合并

## P0 / P1 问题
| ID | 严重级别 | 文件 / 区域 | 问题描述 | 影响原因 | 处理动作 | 负责人 | 状态 |
|---|---|---|---|---|---|---|---|
| R1 | P1 | `path/to/file` | 简短问题摘要 | 行为回归、配置风险或功能 bug | 立即修复 | Claude Code | 待处理 |

## P2 问题
| ID | 严重级别 | 文件 / 区域 | 问题描述 | 影响原因 | 处理动作 | 负责人 | 状态 |
|---|---|---|---|---|---|---|---|
| R2 | P2 | `path/to/file` | 简短问题摘要 | 缺测试或边界情况 | 低成本则修 | Claude Code | 待处理 |

## False Positive / Skip
| ID | 文件 / 区域 | 问题描述 | 跳过原因 |
|---|---|---|---|
| R3 | `path/to/file` | 简短问题摘要 | 设计如此、证据不足或纯样式问题 |

## 验证计划
- `make test`
- `cd web && npm run lint`
- `cd web && npm run build`
- `make api-gen`
- `./scripts/test-env.sh verify`

## 修复记录
| ID | 修复内容 | 执行的测试 / 检查 | 结果 |
|---|---|---|---|
| R1 | 简短修复说明 | `make test` | 通过 |

## 复审
- 复审命令:
- 剩余风险:
- 合并结论:
```

---

## 附录 B:Claude Code 修复任务模板

把已验证的 Codex finding 交给 Claude Code 时,默认使用下面这个模板。

```text
请在当前 DSP 仓库中处理以下评审问题。

要求:
- 在改代码前先验证每条 finding 是否成立。
- 对有效问题使用最小正确修复。
- 需要时补充或更新测试。
- 不要做无关重构。
- 最后总结:
  1. 哪些 finding 已修复
  2. 哪些 finding 是 false positive
  3. 实际执行了哪些验证

仓库约束:
- 该仓库以 Go 为主,服务入口在 cmd/,共享逻辑在 internal/。
- 前端位于 web/,可能依赖生成的 API 类型。
- 如果改了 API handler、请求结构或响应结构,需要确认是否应重新生成 docs/generated/openapi3.yaml 和 web/lib/api-types.ts。
- 如果改了测试环境配置,要一起检查 docker-compose.test.yml 和 scripts/test-env.sh。
- 优先修真实行为问题和测试,不要把时间花在样式清理上。

问题列表:
1. ...
2. ...
3. ...
```

### 单条问题版本

```text
请在当前 DSP 仓库中验证并处理下面这条评审问题。

要求:
- 先判断该 finding 是否成立。
- 如果成立,用最小正确改动修复。
- 需要时补充或更新测试。
- 不要做无关重构。
- 如果是 false positive,用文件级证据说明原因。

问题:
...
```
