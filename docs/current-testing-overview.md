目前这个项目的测试是“分层做”的，不是一套命令包打天下。最核心的点是：默认跑的是快的单测，涉及真实依赖的测试用 `build tag` 单独跑，前端目前主要靠 `lint + build + 全链路验证`。

**测试分层**
1. 默认单测  
平时最常跑的是 `make test`，实际就是 [Makefile](</C:/Users/Roc/github/dsp/Makefile:31>) 里的 `go test ./... -short -count=1`。这层主要测包内逻辑、边界条件、纯函数和轻量行为，不要求外部服务。

2. 后端 integration  
这一层不是默认跑的，要显式带 tag。项目里有两类：
- 安全/租户隔离这类回归测试在 `test/integration/`，跑法是 `go test -tags integration ./test/integration/...`
- bidder、consumer、reporting、budget、reconciliation 这些真实链路测试也走 `integration`，比如 `cmd/bidder/*_integration_test.go`、`cmd/consumer/consumer_integration_test.go`、`internal/bidder/*_integration_test.go`。这类测试会接真实的 Postgres、Redis、Kafka、ClickHouse，很多是通过 `internal/qaharness` 统一装配的。整体说明在 [docs/module-architecture.md](</C:/Users/Roc/github/dsp/docs/module-architecture.md:102>)。

3. handler e2e  
API/handler 层还有一套单独的 `e2e` tag，文件主要是 `internal/handler/e2e_*_test.go`。跑法是 `go test -tags e2e ./internal/handler/...`。这层更像“接口契约测试”，会把生产用的 mux 和 middleware 直接装到 `httptest` 里，重点验证 public/admin 路由、auth、pubsub、billing、reporting 这些行为。

4. 前端验证  
前端目前没有仓库自带的单元测试或组件测试文件；我核对了 `web/`，只有 [web/package.json](</C:/Users/Roc/github/dsp/web/package.json:5>) 里的 `lint`、`build`、`dev`、`start`。所以前端当前主要靠：
- `cd web && npm run lint`
- `cd web && npm run build`

5. 全链路验证  
仓库还提供了一套隔离测试环境脚本 [scripts/test-env.sh](</C:/Users/Roc/github/dsp/scripts/test-env.sh:1>)：
- `./scripts/test-env.sh all`：拉起测试基础设施、迁移、服务和前端
- `./scripts/test-env.sh verify`：执行 `cmd/autopilot` 的全链路验证

**平时怎么用**
如果是日常开发，我建议这样跑：

- 改了 Go 逻辑：先 `make test`
- 改了 handler / 鉴权 / 租户隔离：再跑 `go test -tags e2e ./internal/handler/...`，必要时加 `go test -tags integration ./test/integration/...`
- 改了 bidder / consumer / reporting / budget 这种跨服务链路：跑对应的 `integration` 测试
- 改了前端：跑 `cd web && npm run lint` 和 `cd web && npm run build`
- 改动影响整条业务流：最后用 `./scripts/test-env.sh verify` 做一次全链路确认

一句话概括：这个项目现在的测试策略是“默认快测保反馈，tag 分层保真实性，autopilot 保全链路”，而前端自动化测试还没有像后端这样成体系。  

如果你愿意，我下一条可以直接给你整理一份“这个项目最实用的测试命令清单”，按 `改 API`、`改 bidder`、`改前端` 三种场景来列。
