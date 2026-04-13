# 文档索引

## 核心文档
- [功能清单](./project-feature-inventory.md)：已实现的产品与系统能力
- [模块架构说明](./module-architecture.md):服务划分、模块职责与数据流
- [历史完成度快照](./archive/):版本发布时的交付状态记录(最新 2026-04-13)
- [项目概览](./PROJECT_OVERVIEW.md):顶层定位、形态与能力总览
- [评审工作流](./REVIEW_WORKFLOW.md):Codex 与 Claude Code 的评审闭环
- [发布检查清单](./RELEASE_CHECKLIST.md):合并与发版检查门槛
- [../CONTRIBUTING.md](../CONTRIBUTING.md):贡献入口文档

## API 契约
- `generated/swagger.yaml` / `generated/swagger.json`：生成的 OpenAPI 2 产物
- `generated/openapi3.yaml`：前端类型生成流程使用的 OpenAPI 3 契约
- `generated/docs.go`：生成的 Go swagger 元数据

如果修改了 API handler 或 swagger 注解，请执行 `make api-gen` 重新生成这些文件。

## 验证资料
- `archive/browse-reports/`：归档的浏览器验证报告和截图
- `archive/superpowers/plans/`：归档的实现计划
- `archive/superpowers/specs/`：归档的设计和范围文档

## 模板
- [templates/review-triage-template.md](./templates/review-triage-template.md)：评审问题分流与跟踪模板
- [templates/claude-fix-task-template.md](./templates/claude-fix-task-template.md)：Claude Code 修复任务模板

## 推荐阅读顺序
1. [项目概览](./PROJECT_OVERVIEW.md)
2. [功能清单](./project-feature-inventory.md)
3. [模块架构说明](./module-architecture.md)
4. `generated/openapi3.yaml`
5. `../TODOS.md`

## 维护说明
- 叙述性文档要与 `cmd/`、`internal/`、`web/` 和 `TODOS.md` 保持一致
- 除非生成源发生变化，不要手工修改 OpenAPI 生成产物
- 浏览报告、一次性计划和临时过程文档统一放在 `archive/`
- 新增重大工作流或子系统时，请同步更新本索引和架构说明
