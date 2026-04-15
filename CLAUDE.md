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

### 上面的循环是 NON-NEGOTIABLE

两次事故证明了这一点,每次都付出了真金白银的代价:

- **V5 remediation 2026-04-14**:6 批次安全修复按 batch 跑了单元测试,但没跑 per-phase code review / 集成 / /qa。最后的 retroactive final review 捞出 **5 个真 bug**,其中一个是 Critical "单次 admin 调用泄露所有广告主 api_key"(`HandleListAdvertisers`)—— "一次泄露等于全系统 takeover"级别。净代价:2 轮补救性 review + 修复。见 `~/.claude/projects/C--Users-Roc-github-dsp/memory/feedback_per_phase_review.md` 详细取证
- **biz QA 2026-04-14**:4 个 Critical 安全热修(P2.7b / P2.8b / P4.2b + P2.9 补丁)inline 执行零 review。最后需要回溯加厚的 final review + 对 Critical commits 的独立二次审查才能安全合入。见本 repo `docs/qa/2026-04-14-biz-qa-report.md` 的 P5.1 章节

**必须当场拒绝的内心独白**(这些正是两起事故当时说服自己的理由):

- "这个 fix 很小 / 很明显,重新 review 是 overkill"
- "我内联修完 reviewer 提的问题了,重新 dispatch reviewer 是浪费"
- "Scripted work(bash / shell / 文档)不需要正规 review"
- "我自己跑过测试了,那就算 review 过了"
- "时间紧 / token 预算紧"
- "Subagent 速率限制用完了,自审等价"

**捕捉到任何一个这样的念头 → 立即停下来,该跑的 review 照跑。** 如果 subagent 无法 dispatch(速率限制 / 其他 blocker)→ **立即停下来,上报用户**。不要把自审当作独立 review 的等价物 —— 自审对"写代码时无意识 rationalize 掉的东西"有系统性盲区,独立 review 没有。

**三条铁律(无一例外)**:

1. 每个 task → implementer → spec reviewer → code quality reviewer,**按顺序跑完**才能 mark task done
2. 每次 reviewer 找到问题触发的 inline fix → 必须 **re-dispatch** reviewer 验证 fix 真正解决了问题,才能进下一步
3. 每个 Phase 边界 → `superpowers:requesting-code-review` + `superpowers:verification-before-completion` + `/qa`,**完整循环**到**零问题一轮**才能 mark Phase done

对于 tenant-isolation 覆盖,nil-store test stub **不够** —— 它漏掉的正是"WHERE 子句返 0 行 → handler 把 DB 错误映射为 500/409 而非 404"这一类 bug。tenant-isolation 回归测试**必须**打真 Store。对于 cross-cutting 关注点("是否每个 `producer.Send(...)` 都被 inflight helper 包裹"这种),**用 grep-based 审计**,不要相信 implementer 的心智模型。

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
