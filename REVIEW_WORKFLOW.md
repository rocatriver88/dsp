# 评审工作流

## 目标
使用 Codex 负责评审，Claude Code 负责实现已验证的修复。这个流程的目标是在合并前尽量发现真实 bug、行为回归、配置风险和缺失测试。

## 1. 准备评审范围
- 优先在独立 review 分支上评审，而不是直接评审噪声很多的工作区
- 只保留本次预期变更
- 临时报告、无关实验和生成噪声默认排除，除非它们本身就是本次改动的一部分

推荐评审方式：

```powershell
codex review --base main "<repo-specific review prompt>"
```

只有在当前工作区本身已经被整理为评审范围时，才使用 `--uncommitted`。

## 2. 运行 Codex 评审
该仓库推荐的默认提示词：

```powershell
codex review --base main "Act as a strict production reviewer for this DSP repository. Review for correctness, regressions, edge cases, unsafe config changes, missing tests, API/schema mismatches, and operational risks. Pay special attention to Go services under cmd/ and internal/, docker-compose.test.yml, migrations/, docs/, and web/ API type generation. Report findings ordered by severity with concrete file references. Ignore formatting-only comments."
```

## 3. 问题分流
将每条 finding 分类为：
- `P0/P1`：明确 bug、行为回归、鉴权问题、配置风险、API 破坏或 migration 风险
- `P2`：缺失测试、边界情况、可观测性或可维护性缺口
- `Skip`：误报、符合设计的行为或纯样式问题

建议使用 [docs/templates/review-triage-template.md](./docs/templates/review-triage-template.md) 显式记录分流结果。

## 4. 把修复任务交给 Claude Code
只把已经验证过的 `P0/P1` 问题交给 Claude Code，建议每次 1 到 3 条同类问题。

默认使用 [docs/templates/claude-fix-task-template.md](./docs/templates/claude-fix-task-template.md) 作为交接模板。

## 5. 验证修复结果
根据改动范围执行最小但充分的验证。

核心验证：

```powershell
make test
```

前端改动：

```powershell
cd web
npm run lint
npm run build
```

API 契约变动：

```powershell
make api-gen
```

环境或编排变动：

```powershell
./scripts/test-env.sh verify
```

## 6. 复审
修复后再次运行 Codex：

```powershell
codex review --base main "Re-review this DSP branch after fixes. Focus on whether previous high-severity findings are resolved, whether the fixes introduced regressions, and whether any critical risks remain."
```

只要还有未关闭的 `P0/P1` 问题，就不要合并。

## 7. 该仓库的重点关注区域
审这个仓库时，重点看：
- `internal/auth` 和 `internal/handler` 的鉴权与 API 行为
- `internal/bidder`、`internal/budget`、`internal/guardrail`、`internal/antifraud` 的竞价正确性
- `internal/reporting`、`internal/billing`、`internal/reconciliation` 的数据正确性
- `docker-compose.test.yml` 和 `scripts/test-env.sh` 的测试环境稳定性
- `docs/generated/openapi3.yaml` 和 `web/lib/api-types.ts` 的前后端契约同步

## 8. 相关文档
- 合并与发版门槛：[RELEASE_CHECKLIST.md](./RELEASE_CHECKLIST.md)
- 仓库规则：[AGENTS.md](./AGENTS.md)
- 贡献入口：[CONTRIBUTING.md](./CONTRIBUTING.md)
