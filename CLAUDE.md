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
   │   └── /qa                              无头浏览器系统性测试前端
   │
   └── 全部实现完成后:
       ├── requesting-code-review   全量审查 → 修 Critical/Important
       ├── verification-before-completion   启动真实服务，跑集成验证
       ├── /qa                              无头浏览器系统性测试前端
       └── /browse                          截图验证关键页面
4. Final Code Review                最终全量代码审查
5. Finishing Branch                  打 tag / 创建 PR / push
```

**Key rules:**
- verification + /qa runs at EVERY Phase boundary, not just at the end
- Compiling + unit tests ≠ working system — must verify against live services
- /browse screenshots at the end confirm visual compliance with DESIGN.md
- Do NOT skip any step

## Design System
Always read DESIGN.md before making any visual or UI decisions.
All font choices, colors, spacing, and aesthetic direction are defined there.
Do not deviate without explicit user approval.
In QA mode, flag any code that doesn't match DESIGN.md.
