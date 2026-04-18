# TODOS

## P0 — Completed

> All P0 items resolved in commits 17094d5..8b4d649 (2026-03-28)

- ~~Secure bidder endpoints~~ → HMAC-SHA256 token validation (PR1b)
- ~~Remove hardcoded ADVERTISER_ID~~ → API Key auth + frontend multi-tenancy (PR1b + PR3)
- ~~Fix billing model in bidder engine~~ → EffectiveBidCPMCents() ranking (PR2)

## P0.5 — Completed

- ~~CPC charge on click~~ → handleWin skips CPC, handleClick deducts BidCPCCents
- ~~DLQ attempt count persistence~~ → attempt count in event payload, increments on re-queue

## P1 — Completed (final batch)

- ~~Frontend loading/error states~~ → LoadingSkeleton, ErrorState, EmptyState components + 401 redirect
- ~~Grafana dashboard~~ → Prometheus + Grafana in docker-compose, pre-configured DSP dashboard
- ~~Kafka replay auto-start~~ → producer.ReplayBuffer() called at bidder startup

## P1 — Completed

> Resolved in commits 17094d5..8b4d649 (2026-03-28)

- ~~Fix CORS wildcard~~ → origin allowlist (PR1a)
- ~~Add pause_reason field~~ → migration 007 + model (PR2)
- ~~Kafka event replay~~ → ReplayBuffer method (PR2)
- ~~Dead-letter queue~~ → dsp.dead-letter topic + SendToDeadLetter (PR2)
- ~~Fail /win on Redis budget failure~~ → already existed, confirmed (PR2)
- ~~Batch-load creatives~~ → GetCreativesByCampaigns batch query (PR2)
- ~~Separate internal API port~~ → :8182 (PR1a)
- ~~Add Redis/ClickHouse auth~~ → requirepass + user/password (PR1a)
- ~~Full observability~~ → slog JSON + Prometheus /metrics + request_id (PR3 + final)

## Infra — Completed 2026-04-18

> CI/tooling/workspace cleanup pass. Three PRs merged (#13, #12, #14).

- ~~CI stuck red for 3 days~~ → pinned `golangci-lint@v1.64.8` with `install-mode: goinstall` + `swag@v1.16.4` + Go `1.26.1` (PR #13)
- ~~60+ `golangci-lint` violations masked by broken tool~~ → new `.golangci.yml` (33 lines of per-path / per-function exclusions) + real I/O error logging in `internal/events/producer.go`, targeted `_ =` / `//nolint` annotations elsewhere (PR #13)
- ~~`scripts/docs-check.sh` silently hid `swag` failures~~ → released stderr + stdout, added `go mod download` before `swag init` to avoid cold-cache module race (PR #13)
- ~~`user.UserResponse` swagger annotation fatal on Linux CI~~ → fully-qualified path `github_com_heartgryphon_dsp_internal_user.UserResponse`; `/admin/users` schema now correctly emitted in OpenAPI (PR #13)
- ~~`docs/` sprawl at top level (15 items)~~ → archived V5 remediation (6 files) + QA screenshots (29) + qa-checklist to `docs/archive/`; renamed `PROJECT_OVERVIEW.md` → `OVERVIEW.md`, `project-feature-inventory.md` → `feature-inventory.md`; inlined `docs/templates/` into `docs/README.md` appendices (PR #12, #14)
- ~~Workspace noise: 30 AI-tool skill-mirror dirs + build artifacts + skills bundle~~ → added to `.gitignore` (PR #14)
- ~~Docker WSL2 vhdx bloated to 470 GB on C:~~ → reset vhdx + enabled `sparseVhd` in `.wslconfig` + BuildKit GC in `daemon.json`; released 467 GB back to Windows

## P2 — Tech debt (identified 2026-04-18)

### Redis `SetNX` deprecation (SA1019) migration

**What:** 3 callsites use `//nolint:staticcheck` + TODO pending migration to `SetArgs{Mode:"NX"}`:
- `cmd/bidder/main.go:423` (win dedup)
- `cmd/bidder/main.go:592` (click dedup)
- `internal/budget/budget.go:124` (InitTotalBudget)

**Why deferred:** `SetNX(...).Result()` returns `(bool, error)` whereas `SetArgs(...).Result()` returns `(string, error)`. The `wasNew bool` → `err == nil vs redis.Nil` mapping requires careful semantic review in the bidder hot path — dedup failure → double budget deduction.

**Depends on:** Dedicated eng-review with test coverage for both win/click duplicate-detection paths.

### `HandleMe` DTO drift risk

**What:** `internal/handler/auth_handlers.go:197+` writes `map[string]any{id, email, name, role, advertiser_id, status, last_login_at, created_at}` inline, but the swagger annotation (as of PR #13) declares `user.UserResponse`. A future field rename on the typed DTO won't flag the handler.

**Fix:** Replace inline map with `user.NewUserResponse(dbUser)` (constructor already exists at `internal/user/model.go:36`).

**Depends on:** Nothing. ~30 LOC change.

### `cmd/autopilot/` errcheck exclusion

**What:** `.golangci.yml` currently excludes the whole `cmd/autopilot/` directory from `errcheck`. 5 `alerter.Send(...)` callsites silently ignore the error return (notifier failures go undetected).

**Why deferred:** Autopilot is a dev-only automation harness today. If it ever becomes a production/on-call path, tighten back.

**Depends on:** Productionization decision for autopilot.

### Open GitHub issues for the three tech-debt items above

**What:** The TODO comments in code today don't link to tracked issues; they'll rot. Create GH issues with labels `tech-debt` and `infra`.

**Depends on:** Nothing.

### Review SetNX dedup fail-open policy under Redis outage

**What:** `cmd/bidder/main.go:423,592` and `internal/budget/budget.go:124` currently fail-open on Redis `SetNX` error (log and proceed). Codex flagged this as a real policy question during Phase Final: if the system invariant is "never double-debit under Redis faults," this path should fail-closed (reject the callback) instead. Today's code preserves callback availability at the cost of strict idempotency.

**Why deferred:** Policy decision, not a bug. Needs product/eng alignment on the idempotency vs availability trade-off during Redis outage.

**Depends on:** SLO conversation on billing accuracy under degraded infra.

### `scripts/docs-check.sh` can mask future `go.sum` drift

**What:** Codex observed during Phase Final: the new `go mod download` call (added in PR #13 to avoid cold-cache swag races) will happily fetch missing deps at CI time. If a developer bumps `go.mod` but forgets to commit `go.sum`, `docs-check.sh` will now *succeed* (deps downloaded on the fly) where previously it would have failed earlier. The diff check only covers `docs/generated/` and `web/lib/api-types.ts`, not `go.sum`.

**Fix:** Add `git diff --quiet -- go.sum` check after `go mod download` (or pass `-mod=readonly` where feasible).

**Depends on:** Nothing. ~5 LOC in `scripts/docs-check.sh`.

## P2 — Deferred

### PIPL compliance
**What:** Audit data flows for China PIPL compliance, add consent mechanisms, ensure data localization.
**Depends on:** Legal review of specific PIPL requirements

### Multiple exchange integrations
**What:** Extend exchange adapter to support 3+ real Chinese exchanges.
**Depends on:** Partnership agreements

### Advanced admin panel
**What:** Admin dashboard for managing advertisers, approving registrations, viewing system health.
**Depends on:** Auth system + multi-tenancy (now available)

### Automated billing reconciliation
**What:** Daily reconciliation between Redis spend tracking and ClickHouse event aggregation.
**Depends on:** Production traffic data

### Bid simulator
**What:** "What if I changed my bid?" predictive tool using historical data.
**Depends on:** Sufficient historical bid data in ClickHouse

### Alipay/WeChat Pay integration
**What:** Self-serve payment for advertiser balance topup.
**Depends on:** Billing system hardening

### API 审计追踪
**What:** 数据库审计表 + 中间件记录所有敏感操作（campaign 启停、充值、创意审批）。争议解决和合规基础。
**Depends on:** 无

### 余额低告警
**What:** 广告主余额低于 1 天预算时通过 API/前端提醒。避免 campaign 被突然暂停的惊讶体验。
**Depends on:** 无

## P3 — Future

### API 客户端 SDK (Go/Python)
**What:** 为技术型代理商提供 Go 和 Python 客户端库，降低 API 集成门槛。
**Depends on:** API 接口稳定后
