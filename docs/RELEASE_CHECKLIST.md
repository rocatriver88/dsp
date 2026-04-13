# 发布检查清单

## 1. 范围与分支整洁度
- 确认发布分支只包含本次预期变更
- 移除或排除临时文件、浏览报告和无关实验内容
- 确保提交信息可读且 scope 清晰
- 确认没有提交 `.claude/`、`.context/`、`.gstack/`、`var/` 等本地或运行时内容
- 确认没有根目录二进制产物，编译结果只允许位于 `bin/`

## 2. Codex 评审
- 以 `main` 为基线运行分支评审

```powershell
codex review --base main "Act as a strict production reviewer for this DSP repository. Review for correctness, regressions, edge cases, unsafe config changes, missing tests, API/schema mismatches, and operational risks. Pay special attention to Go services under cmd/ and internal/, docker-compose.test.yml, migrations/, docs/, and web/ API type generation. Report findings ordered by severity with concrete file references. Ignore formatting-only comments."
```

- 将问题分流为：
  - `P0/P1`：合并前必须修复
  - `P2`：低成本则修，否则单独排期
  - `Skip`：误报或纯风格问题

## 3. Claude Code 修复轮次
- 将已验证的 `P0/P1` 问题按小批次交给 Claude Code
- 要求采用最小正确修复，补充测试，且不做无关重构
- 对声称是 false positive 的问题再次人工确认后再忽略

## 4. 核心验证
- 运行后端测试

```powershell
make test
```

- 如果改了前端：

```powershell
cd web
npm run lint
npm run build
```

- 如果改了 API handler、请求/响应结构或 swagger 注解：

```powershell
make api-gen
```

- 如果改了环境变量、端口、Docker 拓扑或服务启动流程：

```powershell
./scripts/test-env.sh verify
```

## 5. 契约与产物同步
- API 契约变化时，确认 `docs/generated/openapi3.yaml` 已更新
- 前端契约变化时，确认 `web/lib/api-types.ts` 已重新生成
- 确认 migration 与运行时配置仍与服务代码一致
- 重大产品或架构变更时，同步更新相关文档
- 确认生成产物仍留在 `docs/generated/`，没有散落到根目录或 `docs/` 其他位置

## 6. 复审
- 修复后再跑一次 Codex 评审

```powershell
codex review --base main "Re-review this DSP branch after fixes. Focus on whether previous high-severity findings are resolved, whether the fixes introduced regressions, and whether any critical risks remain."
```

- 合并前清掉所有剩余 `P0/P1` 问题

## 7. 合并就绪条件
- 所有关键问题都已修复或确认是 false positive
- 必要验证命令全部通过
- 需要生成的产物已同步更新
- 后端与前端之间不存在已知 API 漂移
- 不存在未评审的配置或 migration 风险
- 如变更对外可见，已准备发布说明或变更摘要

## 8. 建议保留的证据
- Codex review 输出
- triage 分流记录
- 实际执行过的验证命令
- `web/` 可见变更的截图
- 延后处理的 `P2` 项记录
