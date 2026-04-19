# engineering: 插件组

## 完整成员（10 个）

| Skill | 一句话定位 | 典型触发词 |
|-------|-----------|-----------|
| `engineering:architecture` | 产出/评估 ADR（架构决策记录） | "choose between", "document design decision", "trade-offs" |
| `engineering:system-design` | 系统/服务/API 架构设计 | "design a system for", "how should we architect" |
| `engineering:code-review` | PR diff 审查（注入、N+1、安全） | PR URL, "review before merge", "is this safe" |
| `engineering:debug` | 结构化调试（reproduce → isolate → diagnose → fix） | 错误栈、"works in staging not prod"、"broke after deploy" |
| `engineering:incident-response` | 线上事故 triage + postmortem | "we have an incident", "prod is down" |
| `engineering:testing-strategy` | 测试策略 + 测试计划设计 | "how should we test", "test strategy for" |
| `engineering:tech-debt` | 技术债识别、分类、优先级 | "tech debt audit", "what to refactor" |
| `engineering:deploy-checklist` | 发布前核对清单 | "about to ship", "migrations + feature flags" |
| `engineering:documentation` | README / runbook / 技术文档 | "write docs for", "create README" |
| `engineering:standup` | 从 commits/PRs 生成 standup 更新 | "yesterday's work", "blockers" |

## 三方对比

| 维度 | superpowers | gstack | engineering: |
|------|-------------|--------|-------------|
| **核心隐喻** | 纪律（铁律、红绿重构、反合理化） | 产品 + 工程师在"造"（Garry Tan 口吻：shipping、made-up world、用户反馈环） | 方法论 / 模板 / checklist |
| **作者立场** | 教条、反 "just this once" 式自我欺骗 | 实用派、犀利、反 corporate、带幽默 | 中立、教科书味、无人格 |
| **触发场景** | 做事之前的纪律约束（TDD / 调试 / 计划） | 做完后验证 + 发布 + 挑战（/review /ship /qa /browse） | 需要"规范产出物"时（ADR、runbook、postmortem、tech debt 报告） |
| **产出形态** | 过程（红绿重构循环、调试四阶段） | 执行（自动跑脚本、截图、CI、PR、merge） | 文档（ADR markdown、checklist、事故报告） |
| **对抗性** | 高（内置 "red flags" 反鸡汤清单） | 高（codex challenge 模式、CEO review） | 低（更像"请问你有没有想过..."） |
| **落地深度** | 深（硬规则 + 铁律 + 不可跳步骤） | 深（和 git/gh/CI 深度集成） | 浅（更像起点模板） |
| **对项目的侵入性** | 中（只改工作方式） | 高（会写文件、起服务、发 PR） | 低（只产出 md 文档） |

## 相同点

三组都是 skill 生态、通过 `Skill` 工具调用、可被 CLAUDE.md 引用、可被其他 skill 组合调用。都**不是替代品**，是**同一工作流里的不同层**。

## 不同点的本质

```
superpowers    = HOW（怎么做对）— 纪律层
gstack         = DO（替你做完）  — 执行层
engineering:   = WHAT（产出什么）— 文档层
```

- **superpowers 是思维模式**。`test-driven-development` 不会帮你写代码，它规定你写代码的**顺序**和**证据链**。违反铁律会被明确叫停。
- **gstack 是工程师代理**。`/ship` 真的会去 push + 建 PR；`/qa` 真的会起浏览器、点按钮、比对数据库；`/review` 真的会跑 codex。它**替你动手**。
- **engineering: 是文档工厂**。`architecture` 给你一份 ADR 模板（填好 Context / Decision / Consequences）；`incident-response` 产出一份 postmortem；`tech-debt` 产出一份分类清单。它**产出可交付物**。

## 重叠区与互补关系

### 看似重叠但角色不同

| 功能 | superpowers | gstack | engineering: |
|------|-------------|--------|-------------|
| **Code review** | `requesting-code-review`（纪律：每个 task 必须 review + re-dispatch） | `/review`（执行：跑多专项审查 + codex） | `code-review`（模板：审查时看什么 — N+1、注入、边界） |
| **Debug** | `systematic-debugging`（纪律：Iron Law no fix without root cause） | `/investigate`（执行：自动跑复现 + 假设验证 + 修） | `debug`（模板：reproduce → isolate → diagnose → fix 的四阶段框架） |
| **Testing** | `test-driven-development`（纪律：红绿重构、watch it fail） | （无直接对应，靠 `/qa` 做事后系统测试） | `testing-strategy`（模板：测试金字塔、覆盖策略、不同层定位） |

### 没有重叠、engineering: 独有

- **ADR（`architecture`）** — superpowers 和 gstack 都没这个。架构决策记录是公司化工程产物
- **Incident response（`incident-response`）** — 事故响应流程 + postmortem
- **Deploy checklist（`deploy-checklist`）** — 发布前核对清单（迁移、feature flag、回滚计划）
- **Standup（`standup`）** — 从 commits 生成站会更新
- **Tech debt（`tech-debt`）** — 债务盘点 + 分类 + 优先级

这 5 个是典型的"工程团队协作产物"，而不是"写代码的动作"。

## 你 dsp 工作流怎么用它们

你现在的 CLAUDE.md 只显式写了 superpowers + gstack，没引用 engineering:。其实可以在这些节点补：

| 你现有节点 | 可接入的 engineering: skill | 价值 |
|-----------|--------------------------|------|
| `plan-eng-review + /codex` 定架构 | `engineering:architecture` 产 ADR | 把"为什么选 Kafka 不选 SQS"这类决策**留下来**，不只是一次对话 |
| V5 remediation 这类大事件 | `engineering:incident-response` 写 postmortem | 你 memory 里的 `feedback_per_phase_review.md` 其实就是一个非正式 postmortem — 用这个 skill 能产出格式化版本 |
| Phase Final 之前 | `engineering:deploy-checklist` 做发布核对 | 你现在是嵌在工作流里的 review+测试循环，可以补一个显式 checklist 作为最后一扇门 |
| 看到 `TODOS.md` / 债务积累 | `engineering:tech-debt` 做优先级 | 你有 memory `project_v5_completed.md` 提到的"open debt 2026-04-14-D1 (bidder handler extraction)" — 这种债务用这个 skill 规范化 |
| 每周 | `engineering:standup` + gstack `/retro` 并用 | standup 看日度、retro 看周度，互补 |

## 一句话总结

**superpowers 管你动手前先想清楚。gstack 直接替你动手。engineering: 把动手的结果沉淀成团队共享的文档。**

三组并不竞争 — 一个完整成熟的项目三组都需要。你现在的流程是 80% superpowers + 20% gstack，engineering: 几乎为 0。引入它的成本很低（产出 markdown 而已），但能补齐"团队知识资产"这块短板。
