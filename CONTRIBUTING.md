# 贡献指南

## 开始前先看
- 阅读 [README.md](./README.md) 了解项目形态和验证命令
- 阅读 [AGENTS.md](./AGENTS.md) 了解仓库级贡献规则
- 阅读 [docs/REVIEW_WORKFLOW.md](./docs/REVIEW_WORKFLOW.md) 了解 Codex 与 Claude Code 的协作方式

## 本地开发
- 后端构建：`make build`
- 后端测试：`make test`
- API 契约生成：`make api-gen`
- 前端 lint 与 build：
  - `cd web`
  - `npm run lint`
  - `npm run build`
- 完整隔离环境：`./scripts/test-env.sh all`
- 集成验证：`./scripts/test-env.sh verify`

## 变更范围要求
- 保持 `cmd/` 足够薄，可复用逻辑放进 `internal/`
- Go 测试与源码同目录，命名为 `*_test.go`
- 除非源契约发生变化，不要手工修改 OpenAPI 产物或前端 API 类型
- 新增子系统、工作流或操作面时，同步更新相关文档
- 编译产物只允许进入 `bin/`，生成契约只允许进入 `docs/generated/`
- `bin/` 内的规范产物名统一使用无扩展名，不保留历史 `.exe` 副本
- `.claude/`、`.context/`、`.gstack/`、`var/` 等本地状态或运行时内容不得提交

## 评审与发布
- 日常评审流程使用 [docs/REVIEW_WORKFLOW.md](./docs/REVIEW_WORKFLOW.md)
- 合并或发版前使用 [docs/RELEASE_CHECKLIST.md](./docs/RELEASE_CHECKLIST.md)
- 评审重点放在行为正确性、配置风险、API 漂移和缺失测试，不纠结纯样式问题

## 文档地图
- 功能与能力总览：[docs/project-feature-inventory.md](./docs/project-feature-inventory.md)
- 历史完成度快照:[docs/archive/](./docs/archive/)
- 架构说明：[docs/module-architecture.md](./docs/module-architecture.md)
- 文档索引：[docs/README.md](./docs/README.md)
- 生成契约目录：[docs/generated/](./docs/generated)
