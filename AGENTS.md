# 仓库指南

## 项目结构与模块划分
`cmd/` 放可执行服务入口，例如 `api`、`bidder`、`consumer`、`autopilot` 和 `exchange-sim`。共享的 Go 业务代码按领域放在 `internal/` 下，例如 `internal/handler`、`internal/bidder`、`internal/campaign`，测试文件与源码同目录，命名为 `*_test.go`。Next.js 控制台位于 `web/`，主要目录为 `app/`、`lib/`、`public/`；修改前端前先看 `web/AGENTS.md`。数据库变更放在 `migrations/`，叙述性文档放在 `docs/`,生成的 API 契约放在 `docs/generated/`,环境与运维辅助脚本位于 `scripts/`、`deploy/`(监控配置位于 `deploy/monitoring/`)。

## 目录边界规则
仓库内容分为四类：`source`、`generated`、`local`、`runtime`。`source` 指 `cmd/`、`internal/`、`web/`、`migrations/` 和正式文档；`generated` 指 `docs/generated/` 与 `web/lib/api-types.ts` 这类可再生产物；`local` 指 `.claude/`、`.context/`、`.gstack/` 等个人环境状态；`runtime` 指 `var/`(上传、autopilot 输出、临时截图、运行日志等均落在此)。规则是：编译产物只允许出现在 `bin/`，生成契约只允许出现在 `docs/generated/`，`local` 与 `runtime` 内容不得进入版本库。
`bin/` 的规范文件名统一为无扩展名，例如 `bin/api`、`bin/bidder`。如果 Windows 下遗留了旧的 `.exe` 文件，应在构建前清理，不保留双份产物。

## 构建、测试与开发命令
使用 `make build` 将主要 Go 服务统一编译到 `bin/`，使用 `make test` 运行 Go 短测集。修改 API 注解后执行 `make api-gen`，同步刷新 `docs/generated/openapi3.yaml` 和 `web/lib/api-types.ts`。前端本地开发使用 `cd web && npm run dev`，生产构建使用 `cd web && npm run build`，提交前至少运行 `cd web && npm run lint`。如需完整本地环境，执行 `./scripts/test-env.sh all` 启动隔离测试栈，执行 `./scripts/test-env.sh verify` 跑 autopilot 验证。

## 代码风格与命名约定
保持 Go 代码满足 `gofmt`，遵循 Go 标准命名：导出标识符使用 `CamelCase`，包内私有辅助函数使用 `camelCase`，包名尽量短且与目录一致。`cmd/` 只保留装配和入口逻辑，可复用能力下沉到 `internal/`。`web/` 侧遵循 Next.js App Router 约定，例如 `app/.../page.tsx` 和 `lib/...`，风格检查依赖 ESLint。除非生成源发生变化，不要手工修改生成产物。

## 测试要求
修改 Go 代码时，在对应包旁边新增或更新 `*_test.go`。领域逻辑优先写表驱动单测，集成覆盖保留在 `integration_test.go` 或 `cmd/autopilot` 的场景测试中。仓库当前没有统一覆盖率门槛，但每个行为变更都应补足相应测试，并至少运行 `make test`，必要时补跑前端 lint 和 build。

## 提交与 Pull Request 规范
最近提交历史采用 Conventional Commit 风格，例如 `fix(config): ...`、`feat: ...`、`chore: ...`；继续沿用这一格式，并保持 scope 有意义。PR 需要说明行为变化、列出验证步骤、在适用时关联 issue 或任务单；涉及 `web/` 的改动应附截图。若本次改动运行了 `make api-gen` 或刷新了前端 API 类型，请在 PR 描述中明确说明生成文件已同步。
