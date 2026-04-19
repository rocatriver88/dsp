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

## TDD Evidence Rule

`superpowers:test-driven-development` 的铁律是 **"没有先失败的测试就不写生产代码"**。
这条规矩口头喊没用 — 必须留下**可审计的证据**证明你先看过红。
本项目落地如下硬规则（reviewer 和 CI 都会卡）：

### 规则 1：Bug fix 必须先提交 failing test commit（硬性）

所有 `fix(...)` 类改动（不含纯 docs/ci/scripts）必须两个 commit：

```
commit A: test(<scope>): add failing regression test for <bug>
commit B: fix(<scope>): <actual fix>
```

- **Commit A 单独可 push / 单独可跑**：在 A 上跑 `go test ./<pkg> -run <TestName>` 必须 FAIL，且失败原因是"功能缺失/行为错误"不是"编译错误/拼写错误"
- **Commit B 让它绿**：在 B 上跑同一条命令必须 PASS
- **不允许把 A 和 B squash**：审查要看到红的那一刻。PR merge 时用 "Rebase and merge" 或 "Create a merge commit"，**禁止 "Squash and merge"** — 此规则对 bug fix PR 是硬性的，对任何显式保留两 commit evidence chain 的 PR（包括 `test(...)` sentinel、`feat(...)` 带 TDD 两段结构）同样适用
- CI 可加一个可选 job：对 PR 的每个 commit 独立跑测试，commit A 失败是期望结果（用 label / commit trailer `Expect-Fail: <TestName>` 声明）

**豁免**：
- 纯文档 / CI / 构建脚本修复（无 `.go` / `.ts` / `.tsx` 变更）
- 明确声明"无法写回归测试"的修复（例：外部 API 行为变更）— 需在 PR body 写清为什么不能测，reviewer 要认可

### 规则 1b：Regression Sentinel 的 Break-Revert Dance

当你给**已经正确的代码**加 regression sentinel（预防未来改坏），测试写完会立刻 PASS，
规则 1 的 "watched it fail" 无法直接满足。这种情况下**必须**走 break-revert dance 证明 sentinel 非空壳：

1. 在 clean main 上写测试 → 跑 → 期望 PASS（前置条件：代码本来就对）
2. **临时**改生产代码，触发 sentinel 应该抓到的那类 bug（例：把 `AdvertiserID: c.AdvertiserID` 改成 `AdvertiserID: 999999`）
3. 再跑测试 → 期望 FAIL，且失败原因对齐你写 sentinel 时设想的情景
4. **立刻**还原生产代码，`git diff` 必须为空
5. 再跑一次测试 → 期望 PASS

上述 5 步必须**完整写进 commit message 或 PR body**，作为审查可复现的证据。
参考模板：见本文件「TDD Evidence 标准格式」一节。

不做 break-revert dance 的 sentinel 一律 reject — 因为你无法证明它真的抓得到那类 bug。

### 规则 2：Feature 的 TDD 证据由开发者自证 + reviewer 抽查

Feature commit（`feat(...)`）不强制两 commit（太重），但：

- PR 描述里加一段 **"TDD Evidence"**：
  - 至少一个新增 `_test.go` 的 Test 名
  - 该 Test 在本地哪次 commit 前确实红过（开发者自述 / reflog 截图 / git stash 过程）
- Reviewer 随机抽查：`git log --follow <test_file>` 看提交顺序是否合理（测试不能总是和实现同一 commit，除非 PR 非常小）

### 规则 3：测试必须打真实依赖（租户隔离 / 权限 / 边界）

这条是 V5 教训的延伸（见 memory `feedback_per_phase_review.md`）：

- **租户隔离测试**：必须打真 Store 或集成环境。用 nil Store / mock DB 无法覆盖 "WHERE clause 返回 0 行 → handler 错误地返回 500/409 而不是 404" 这类 bug — 这恰恰是 tenant-leak 的典型形态
- **权限 / RBAC 测试**：至少一条 case 走真实中间件 + 真实 JWT
- **跨切面审计**（例 "每个 `producer.Send(...)` 是否都包 inflight helper"）：靠 grep 审查，不靠实现者的心智模型

Reviewer 看到新增 `_test.go` 只用 `nil` / `&fakeStore{}` 覆盖关键路径，**直接 block**，要求改打真 Store（`pgtest.NewDB` 或 docker-compose.test.yml 起的 pg）。

### 规则 4：前端 TDD 空白的最小补救

前端目前 `feat(web)` / `fix(web)` 几乎 0 单测，依赖 `test/e2e/test_e2e_flow.py` 兜底。
E2E 循环太慢（分钟级），不能做红绿重构。短期补救：

- **纯函数优先**（formatter / schema validator / API 客户端 / 权限判断 / 金额计算）必须单测，用 vitest + `web/__tests__/`
- 组件级暂不强制 TDD，但涉及业务逻辑的 hook（`useXxx` 里做数据转换 / 状态机 / 缓存）落单测
- 纯视觉 / 布局改动豁免单测，但必须走 `/browse` 四维验证 + `/qa`

### TDD Evidence 标准格式

所有带测试的 PR（包括 `fix`/`feat`/`test` 三类）必须在 PR body 里放一段 "TDD Evidence"，
采用下列表格模板之一。审查时 reviewer 会照这个格式核对。

**格式 A：Bug fix 两 commit 型（Rule 1）**

```markdown
## TDD Evidence

| Step | Action | Result |
|------|--------|--------|
| 1 | Write failing test in commit A | FAIL: <one-line failure message> |
| 2 | Implement fix in commit B | PASS |
| 3 | Other tests unaffected | PASS |
```

**格式 B：Regression Sentinel 型（Rule 1b）**

```markdown
## TDD Evidence (break-revert dance)

| Step | Action | Result |
|------|--------|--------|
| 1 | Wrote test, ran on clean main | PASS (code correct today) |
| 2 | Temporarily edited `path/to/file.go:N` from `X` to `Y` | (prod broken) |
| 3 | Re-ran test | FAIL — <paste exact error> |
| 4 | Reverted file; `git diff <file>` empty | (prod restored) |
| 5 | Re-ran test | PASS |
```

**格式 C：Feature 型（Rule 2）**

```markdown
## TDD Evidence

- New test: `TestXxxYyyZzz` (path/to/file_test.go)
- RED evidence: <describe the moment it was red, e.g. "before commit abc1234, running this test locally reported 'expected X, got <nil>'">
- Reviewer can spot-check via: `git log --follow path/to/file_test.go`
```

这三种格式覆盖了 Rule 1 / 1b / 2 的所有场景。PR body 里**至少**放其中一种，缺失直接 request changes。

### Reviewer Checklist（接入 `superpowers:requesting-code-review`）

每个 task / PR 审查时额外核对：

- [ ] 若是 bug fix：是否有 test commit 在 fix commit 之前？PR body 是否放了格式 A 的 TDD Evidence？
- [ ] 若是 regression sentinel（test-only PR）：PR body 是否放了格式 B（break-revert dance）？5 步是否完整、错误消息是否粘贴？
- [ ] 若是 feature：PR body 是否放了格式 C？随机抽一个 Test 名，问开发者当时 RED 长什么样
- [ ] 新增 `_test.go` 是否避开了 nil-store / mock-only 反模式？涉及租户/权限/边界的，是否打了真 Store？
- [ ] 前端改动：是否对应有单测（纯函数 / 业务 hook），还是能豁免（纯视觉）？
- [ ] Merge 方式：两 commit 及以上的 PR 是否禁用了 Squash merge？

以上任一项不达标 → reviewer 在 PR 上 request changes，不走"小事一桩"放水。

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

## Health Stack

- typecheck: go vet ./...
- lint: golangci-lint run
- test: go test ./... -short
- deadcode: skip (no tool installed)
- shell: skip (shellcheck not installed)
