# gstack

Use the /browse skill from gstack for all web browsing. Never use mcp__claude-in-chrome__* tools.

## Available skills

- /office-hours
- /plan-ceo-review
- /plan-eng-review
- /plan-design-review
- /design-consultation
- /design-shotgun
- /review
- /ship
- /land-and-deploy
- /canary
- /benchmark
- /browse
- /connect-chrome
- /qa
- /qa-only
- /design-review
- /setup-browser-cookies
- /setup-deploy
- /retro
- /investigate
- /document-release
- /codex
- /cso
- /autoplan
- /careful
- /freeze
- /guard
- /unfreeze
- /gstack-upgrade

## Subagent Rules

- Always use Opus model for all subagents (set `model: "opus"` on every Agent call)
- When dispatching subagents for frontend work that calls backend APIs:
  - Always include the exact backend route table (grep `HandleFunc` registrations) in the prompt
  - Always include actual JSON response shapes (from Go struct JSON tags or handler code)
  - Never let subagents guess cross-system contracts — guesses across boundaries are always wrong
- Never skip spec compliance review for frontend tasks, even when rushing

## Development Workflow

Standard flow for any feature/phase implementation.
Skills 分工：superpowers 管"做"（生成方案、写代码、纪律约束），gstack 管"查"（挑战方案、验证产品、发布部署）。

```
═══════════════════════════════════════════
 Phase 0: 构思 + 设计
═══════════════════════════════════════════

superpowers:brainstorming              构思：探索意图、需求、设计方向
  ↓
gstack /office-hours                   灵魂拷问（大功能才用）
  ↓
superpowers:writing-plans              出实现计划
  ↓
gstack /plan-ceo-review                挑战范围（该不该扩大/收缩）
gstack /plan-eng-review + /codex       挑战架构（Codex 独立审查技术假设）
gstack /plan-design-review             挑战交互/视觉（有 UI 时）
  ↓
计划定稿

═══════════════════════════════════════════
 Phase 1-N: 实现（每个 Phase 重复）
═══════════════════════════════════════════

superpowers:executing-plans            按计划推进
  │
  │  ┌─── 每个 task ───────────────────────────┐
  │  │  superpowers:TDD                         │ 红→绿→重构
  │  │    遇 bug → superpowers:                 │
  │  │             systematic-debugging         │
  │  │  superpowers:requesting-code-review      │ task 级 review
  │  │  superpowers:verification-               │ 跑测试确认
  │  │             before-completion            │
  │  └─────────────────────────────────────────┘
  │
  │  Phase 完成 → 审查+测试循环（最多 5 轮）:
  │
  │  ┌─── 循环直到零问题 ──────────────────────┐
  │  │                                          │
  │  │  ── 审查（先查代码）──                    │
  │  │  1. superpowers:requesting-code-review   │ Phase 级全量审查
  │  │  2. gstack /review + /codex              │ 多专项深度审查 + Codex 对抗
  │  │  3. 修复 review 发现的问题               │
  │  │                                          │
  │  │  ── 测试（再跑系统）──                    │
  │  │  4. go test ./... -short                 │ 内核逻辑
  │  │  5. bash scripts/qa/run.sh               │ API 链路
  │  │  6. python test/e2e/test_e2e_flow.py     │ 浏览器端到端全链路
  │  │  7. gstack /qa                           │ 浏览器系统性测试
  │  │                                          │
  │  │  有修复 → 回到 1                          │
  │  │  零问题 → Phase 通过                      │
  │  └──────────────────────────────────────────┘

═══════════════════════════════════════════
 Phase Final: 终审 + 发布
═══════════════════════════════════════════

  ┌─── 循环直到零问题（最多 5 轮）──────────┐
  │                                          │
  │  ── 终审（先查代码）──                    │
  │  1. superpowers:requesting-code-review   │ 终审
  │  2. gstack /review + /codex（必须）      │ 多专项 + Codex 对抗 + challenge
  │  3. 修复                                 │
  │                                          │
  │  ── 终测（再跑系统）──                    │
  │  4. go test ./... -short                 │
  │  5. bash scripts/qa/run.sh               │
  │  6. python test/e2e/test_e2e_flow.py     │
  │  7. gstack /qa                           │
  │  8. gstack /browse                       │ 截图验证
  │  9. gstack /design-review                │ 视觉合规（有 UI 时）
  │  10. gstack /cso                         │ 安全审计（涉及敏感时）
  │                                          │
  │  有修复 → 回到 1                          │
  │  零问题 → 终审通过                        │
  └──────────────────────────────────────────┘
  ↓
superpowers:finishing-a-development-branch    决定 merge/PR
  ↓
gstack /ship                                 创建 PR、push
gstack /land-and-deploy                      merge + 部署
gstack /canary                               线上监控
```

**三层测试定位：**
- `go test` — 秒级，验内核逻辑（SQL、租户隔离、业务规则）
- `scripts/qa/run.sh` — 十秒级，验 API 契约（curl 全链路，检查 JSON 响应）
- `test/e2e/test_e2e_flow.py` — 分钟级，验用户体验（浏览器操作+数据库交叉比对）

**Codex 使用规则：**
- Phase 边界 review：需要（gstack /review 内开启 Codex 对抗审查）
- Phase Final 终审：必须（Codex review + challenge 模式都开）
- 安全相关改动：必须（无论改动大小）
- 跨边界改动（前端↔后端、服务↔服务）：需要
- task 级 review / 构思阶段 / 设计审查：不需要
- plan-eng-review：需要（架构决策定了改动成本极高，Codex consult 挑战技术假设）

**用户参与点（仅以下 5 个环节需要用户介入，其余自动推进）：**
1. 提需求 — "我要做 X"
2. 拍板范围 — /plan-ceo-review 后确认做大还是做小
3. 批准计划 — "计划通过"
4. 批准推送 — "推吧"
5. 批准上线 — "上线"

**Key rules:**
- 审查在测试之前 — review 发现结构性问题，修完再跑测试，避免测试白跑
- 验证是完整轮次循环 — 每轮按顺序走完所有步骤，发现问题就地修，走完一轮后从头开始下一轮，直到整轮零问题
- 只要某一轮进行了任何问题修复，就必须跑下一轮，不能修完直接 push
- 三层测试 + /qa runs at EVERY Phase boundary, not just at the end
- Compiling + unit tests ≠ working system — must verify against live services
- /browse screenshots at the end confirm visual compliance with DESIGN.md
- Do NOT skip any step

## /browse Verification Standard

/browse 不是"截图+目视"，是四维度系统验证。每个页面必须完成以下四项：

### 0. 用户直觉检查（最优先）
- 看到截图后，第一件事是问自己：**如果我是第一次打开这个页面，有没有看起来"不对劲"的地方？**
- 检查布局是否合理：多余的元素？缺失的元素？重复的组件？不该出现的东西？
- 这一步比 CSS 数值和数据正确性更重要 — 一个 CSS 值全对但布局明显错误的页面，验证是失败的
- 只有通过了直觉检查，才进入后面的技术验证

### 1. 交互验证（模拟用户点击）
- 每个页面的关键按钮、链接、表单都要点击/提交
- 验证点击后的页面跳转、状态变化、数据更新是否正确
- 用 `snapshot -D` 对比操作前后的 DOM 变化

### 2. 视觉合规（CSS 抽查 vs DESIGN.md）
- 每个页面至少抽查 3 个元素的实际 CSS 值
- 用 `getComputedStyle` 检查：字体族、字号、颜色、间距、背景色
- 对照 DESIGN.md 的具体值（如 Geist 字体、#2563EB 主色、4px 间距倍数）
- 不合规项记录到报告中

### 3. 数据正确性（页面 vs 数据库交叉比对）
- 每个页面显示的关键数字必须和数据库实际值比对
- 用 SQL 查询真实数据，和页面截图中的数字逐一对照
- 不一致项记录到报告中

## Design System
Always read DESIGN.md before making any visual or UI decisions.
All font choices, colors, spacing, and aesthetic direction are defined there.
Do not deviate without explicit user approval.
In QA mode, flag any code that doesn't match DESIGN.md.
