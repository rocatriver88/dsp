# Claude Code 修复任务模板

把已验证的 Codex finding 交给 Claude Code 时，默认使用下面这个模板。

```text
请在当前 DSP 仓库中处理以下评审问题。

要求：
- 在改代码前先验证每条 finding 是否成立。
- 对有效问题使用最小正确修复。
- 需要时补充或更新测试。
- 不要做无关重构。
- 最后总结：
  1. 哪些 finding 已修复
  2. 哪些 finding 是 false positive
  3. 实际执行了哪些验证

仓库约束：
- 该仓库以 Go 为主，服务入口在 cmd/，共享逻辑在 internal/。
- 前端位于 web/，可能依赖生成的 API 类型。
- 如果改了 API handler、请求结构或响应结构，需要确认是否应重新生成 docs/openapi3.yaml 和 web/lib/api-types.ts。
- 如果改了测试环境配置，要一起检查 docker-compose.test.yml 和 scripts/test-env.sh。
- 优先修真实行为问题和测试，不要把时间花在样式清理上。

问题列表：
1. ...
2. ...
3. ...
```

## 单条问题版本

```text
请在当前 DSP 仓库中验证并处理下面这条评审问题。

要求：
- 先判断该 finding 是否成立。
- 如果成立，用最小正确改动修复。
- 需要时补充或更新测试。
- 不要做无关重构。
- 如果是 false positive，用文件级证据说明原因。

问题：
...
```
