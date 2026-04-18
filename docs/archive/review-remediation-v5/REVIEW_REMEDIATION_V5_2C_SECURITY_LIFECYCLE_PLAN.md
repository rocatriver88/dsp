# V5.2C Security-Adjacent + Lifecycle — Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development`. Per-task two-stage review, Phase 2C boundary loop non-negotiable.

**Goal:** Close the "周边基础设施" P2 findings that the 2026-04-15 independent Claude review flagged outside the core tenant-isolation scope. These are mostly small, isolated hardening fixes, but collectively they remove several real attack surfaces and lifecycle hazards that V5 and V5.1 didn't touch.

**Architecture:** Each finding is a single focused commit. No cross-cutting refactors. The plan is pure hardening — no new features.

**Tech Stack:** Go, Next.js, existing Redis/ratelimit/http.Server plumbing.

**Scope — NOT in this plan:**
- Phase 2A contract unification
- Phase 2B observability + alerts
- Any new features

---

## Findings to fix (from the 2026-04-15 review)

### Security

1. **`cmd/bidder /stats` unauthenticated competitor recon** — `cmd/bidder/main.go:606-622` exposes all active campaigns' id/name/bid_cpm_cents/budget_daily/budget_remaining/creatives_count to any client that can reach the bidder's public URL. Move to `/internal/stats` behind admin auth OR add X-Admin-Token check.

2. **`ApiKeyGate` admin route lockout** (`web/app/_components/ApiKeyGate.tsx:36-82`) — even after the V5 closeout rewrite, the `if (!apiKey)` check fires BEFORE the `if (isAdmin)` bypass, so admin users with only an admin token (no tenant api_key) get stuck on the tenant login screen and can't reach `/admin/*`. Re-order: check `isAdmin` first and return `<>{children}</>` so `AdminTokenGate` in `/admin/layout.tsx` takes over.

3. **`admin/layout.tsx` network-error fallback** (`web/app/admin/layout.tsx:108-132`) — `.catch(() => setToken(stored))` means a server health-check failure causes the frontend to treat the stored token as valid. Attacker DoSing the admin health endpoint (or offline cached page) gets into the admin UI shell, leaking route structure. Remove the fallback — fail closed.

4. **Redis → rate limit fail-open without alert** — `cmd/api/main.go:76-83` sets `rdb = nil` on Redis ping failure, which makes `ratelimit.Middleware` skip via its nil check and allow all requests. Plus there's no alert, so a silent Redis outage turns the API into "no rate limit, forever". Options: (a) fail hard at startup if Redis is required (production env), or (b) add a startup WARN + metrics counter `dsp_ratelimit_degraded` and alert on it.

5. **Upload legacy directory** — `internal/handler/upload.go:17, 38` — the upload file server serves both `var/uploads` (new writes) and `uploads` (legacy reads). Any file left in legacy from before the var/ migration is still publicly readable. Remove the legacy fallback.

6. **Rate-limit Redis key = plaintext API key** — `internal/ratelimit/ratelimit.go:31` uses `"ratelimit:" + "key:" + <raw-api-key>` as the Redis key. A Redis dump/snapshot leaks every tenant key. Hash: `"ratelimit:key:" + sha256(apiKey)[:16]`.

### Lifecycle

7. **`http.Server` missing timeouts (slowloris)** — `cmd/api/main.go:139,143` and `cmd/bidder/main.go:174` all construct `http.Server{Addr, Handler}` with no `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`. One slow client can hold a goroutine forever. Fix: set `ReadHeaderTimeout: 10s`, `ReadTimeout: 30s`, `WriteTimeout: 60s`, `IdleTimeout: 120s` on all three.

8. **`LoadLocation("Asia/Shanghai")` error drop** — `cmd/bidder/main.go:131-151`'s daily budget reset loads the timezone and drops the error. `internal/config/config.go:107` already has `CSTLocation` package-level — replace the bidder's call site with the config-level var.

### Code quality

9. **`advertiserChargeCents` extraction** — `cmd/bidder/main.go:403,442,453` computes `int64(price / 0.90 * 100)` three times with the magic number `0.90` inlined. Extract a helper `func advertiserChargeCents(price float64) int64` with a named constant `PlatformMargin = 0.10` (so `1 - PlatformMargin = 0.90`).

10. **`GetCampaign` nil during warm-up → wrong billing model** — `cmd/bidder/main.go:398` checks `c != nil && c.BillingModel == "cpc"`. During loader warm-up (first few seconds after startup), `GetCampaign` returns nil for campaigns that exist in DB but haven't been loaded yet. First wins during this window silently walk the CPM path even for CPC campaigns → double-charge or missed-charge depending on which branch is wrong. Fix: if `c == nil && campaignID > 0` during warm-up, reject the click with 503 "bidder warming up, retry" instead of silently walking the wrong path.

11. **`admin.go` JSON decode error swallowed** — `internal/handler/admin.go:267` drops the decode error in `HandleRejectRegistration`. A client sending invalid JSON proceeds with an empty `req.Reason`, which then gets written into the audit trail as a blank rejection reason. Return 400 on decode error.

---

## File Structure

**Modified files (Go backend):**
- `cmd/bidder/main.go` — /stats move, http.Server timeouts, LoadLocation replacement, advertiserChargeCents extraction, GetCampaign warm-up guard
- `cmd/bidder/routes.go` — /stats registration
- `cmd/api/main.go` — http.Server timeouts, Redis failure handling
- `internal/handler/admin.go` — JSON decode error
- `internal/handler/upload.go` — legacy dir removal
- `internal/ratelimit/ratelimit.go` — hashed Redis keys
- `internal/config/config.go` — re-exported `CSTLocation` if not already public

**Modified files (frontend):**
- `web/app/_components/ApiKeyGate.tsx` — admin route bypass ordering fix
- `web/app/admin/layout.tsx` — network-error fallback removal

**New test files:**
- `internal/handler/upload_test.go` (if not present) — add a case verifying the legacy dir is not served
- `internal/ratelimit/ratelimit_test.go` — add a case verifying the Redis key is a hash, not plaintext
- `cmd/bidder/main_test.go` — add a case for advertiserChargeCents helper
- `web/app/_components/__tests__/ApiKeyGate.test.tsx` — if the project has a frontend test harness; otherwise verify manually via /qa

---

## Task sequencing

Each finding is an independent commit. Safe to work in parallel in principle, but this plan orders them to minimize merge conflict risk:

- **Task 1**: /stats admin auth move (finding #1)
- **Task 2**: ApiKeyGate admin bypass fix (finding #2)
- **Task 3**: admin/layout network-error fallback removal (finding #3)
- **Task 4**: Redis fail-hard or fail-with-alert (finding #4)
- **Task 5**: Upload legacy dir removal (finding #5)
- **Task 6**: Rate-limit key hashing (finding #6)
- **Task 7**: http.Server timeouts (finding #7) — touches 3 files, biggest blast radius, do after everything else so merge conflicts don't cascade
- **Task 8**: LoadLocation dedup (finding #8)
- **Task 9**: advertiserChargeCents extraction (finding #9)
- **Task 10**: GetCampaign warm-up guard (finding #10)
- **Task 11**: admin.go JSON decode error (finding #11)
- **Task 12**: Phase 2C boundary loop

Each task follows the standard TDD pattern: write failing test → implement → verify → commit.

---

## Normative decisions (locked)

### `cmd/bidder /stats` move

**Decision:** Register under `/internal/stats` on the internal port (matches existing `/internal/active-campaigns` pattern). Admin token required. The public bidder port loses the endpoint entirely.

### Redis rate-limit failure

**Decision:** In production, Redis is REQUIRED. `Config.Validate()` refuses to start if `REDIS_ADDR` is unset in production. `cmd/api/main.go` fails hard on Redis ping failure instead of nil-ing the client. Dev / test envs keep the fail-open behavior via a `cfg.RedisOptional` flag that defaults false.

### Rate-limit key hashing

**Decision:** SHA256, hex-encoded, truncated to first 16 hex chars (64 bits). 64-bit collision resistance is overkill for a fixed-window counter bucket; the real purpose is just "prevent plaintext key dumps from Redis". Helper lives in `internal/ratelimit/ratelimit.go`.

### http.Server timeouts

**Decision:**
- `ReadHeaderTimeout: 10s` — slowloris class
- `ReadTimeout: 30s` — big JSON body upload tolerance
- `WriteTimeout: 60s` — SSE exception below
- `IdleTimeout: 120s` — keep-alive cleanup

**SSE exception:** `/api/v1/analytics/stream` needs a WriteTimeout of 0 (no limit) or the SSE stream gets force-closed every 60 seconds. Use per-route override via a custom `http.Handler` that sets `w.Header().Set("X-Accel-Buffering", "no")` and a flushing Writer... actually simpler: apply the 60s WriteTimeout to the API server, and have the analytics SSE handler reset its own response deadline via `rc := http.NewResponseController(w); rc.SetWriteDeadline(time.Time{})`. Go 1.20+ supports this. Document in the handler.

### Upload legacy dir

**Decision:** Delete the `uploads/` fallback. If any legacy files exist there, they're now unreachable. One-time cleanup migration script (under `scripts/cleanup-legacy-uploads.sh`) moves surviving files to `var/uploads/`.

---

## Task 0 through Task 12

Task 0 is baseline; Task 12 is Phase 2C boundary loop. Tasks 1–11 each follow the standard shape: write failing test → implement → verify → commit. Implementation details for each task live inline here, but the task bodies are identical in structure to Phase 1's Task templates — see `docs/REVIEW_REMEDIATION_V5_1_HOTFIX_PLAN.md` for the shape, and each task's findings paragraph in the section above for the specific code changes.

Given the highly mechanical nature of these hardening fixes, the implementer subagent for Task 1–11 can be the same across multiple tasks (but fresh context per task — dispatch a new Agent call for each, do not reuse an agent session).

---

## Out of scope

- Phase 2A / 2B (separate plans already written)
- Fundamental architectural refactors (bidder handler extraction is `2026-04-14-D1` open debt from V5, not Phase 2C scope)
- Per-advertiser SSE rate-limit bucketing (explicit deferral from Phase 1)
- Full tracing / OpenTelemetry (separate effort)
- PagerDuty integration (Phase 2B Slack + email is the baseline)
