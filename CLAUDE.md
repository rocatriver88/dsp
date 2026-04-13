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

Standard flow for any feature/phase implementation:

```
1. Brainstorming                    构思 → 设计文档
2. Writing Plans                    设计 → 实现计划
3. Execution (TDD)                  计划 → 代码
   ├── 每个 task:
   │   ├── implementer subagent     写测试 → 写代码 → 跑测试 → 提交
   │   ├── spec compliance review   对照计划检查（没多做、没少做）
   │   └── code quality review      代码质量审查
   │
   ├── 每个 Phase 完成后:
   │   ├── requesting-code-review   阶段性全量审查 → 修 Critical/Important
   │   ├── verification-before-completion   启动真实服务，跑集成验证
   │   ├── /qa                              无头浏览器系统性测试前端
   │   └── 一轮走完后回到 requesting-code-review，直到整轮三步都零问题（最多 5 轮）
   │
   └── 全部实现完成后:
       ├── final-code-review        全量审查 → 修 Critical/Important
       ├── verification-before-completion   启动真实服务，跑集成验证
       ├── /qa                              无头浏览器系统性测试前端
       ├── /browse                          截图验证关键页面
       └── 一轮走完后回到 final-code-review，直到整轮四步都零问题（最多 5 轮）
4. Finishing Branch                  打 tag / 创建 PR / push
```

**Key rules:**
- 验证是完整轮次循环 — 每轮按顺序走完所有步骤，发现问题就地修，走完一轮后从头开始下一轮，直到整轮零问题
- 只要某一轮进行了任何问题修复，就必须跑下一轮，不能修完直接 push
- verification + /qa runs at EVERY Phase boundary, not just at the end
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
