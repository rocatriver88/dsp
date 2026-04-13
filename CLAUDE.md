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

## Completion Checklist

After all implementation tasks are done, before final code review and finishing branch, MUST run these three steps in order:

1. **verification-before-completion** — Start all services (`docker-compose up`, Go binaries, frontend dev server), run `autopilot verify` or equivalent integration test, confirm real output matches expectations. Evidence before assertions.
2. **/qa** — Use the /qa skill to systematically test all frontend pages with a headless browser. Catches rendering errors, broken API calls, missing data, and interaction bugs that `tsc` cannot detect.
3. **/browse** — Take screenshots of key pages to visually verify they match DESIGN.md. At minimum: login page, main dashboard, and any new pages added in this session.

Do NOT skip these steps. Compiling + unit tests ≠ working system.

## Design System
Always read DESIGN.md before making any visual or UI decisions.
All font choices, colors, spacing, and aesthetic direction are defined there.
Do not deviate without explicit user approval.
In QA mode, flag any code that doesn't match DESIGN.md.
