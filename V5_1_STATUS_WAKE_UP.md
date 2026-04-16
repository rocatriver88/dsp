# V5.1 Hotfix — Wake-Up Status

**Branch:** `review-remediation-v5.1-hotfix` (13 commits ahead of main `e39a35d`)
**Date:** 2026-04-16
**State:** All 3 P1s fixed, all automated checks green, manual `/qa` pending before merge to main.

---

## What happened while you were asleep

You authorized autonomous execution at the end of the P1-1 primitives review. Here's what I completed:

### ✅ Done — Phase 1 implementation (10 commits)

**P1-1 — analytics SSE tenant api_key URL leak.** Closed end-to-end. New `APIHMACSecret` config field (distinct from `BidderHMACSecret`, validated ≥32 bytes in prod), new `internal/auth/sse_token.go` + `sse_middleware.go` primitives, new `POST /api/v1/analytics/token` handler, new `BuildAnalyticsSSEMux` routed through `SSETokenMiddleware`, exact-match dispatcher in `BuildPublicHandler`, the legacy `?api_key=` query promotion deleted from `WithAuthExemption`, frontend `web/app/analytics/page.tsx` migrated to the two-step fetch-token-then-EventSource flow. Two rounds of per-task code review + one round of Phase-level review caught a stale `@Security ApiKeyAuth` godoc issue on the SSE endpoints; fixed in `c754ac7` with a new `SSETokenAuth` security scheme + regenerated OpenAPI spec + regenerated `web/lib/api-types.ts`.

**P1-2 — `HandleCreateAdvertiser` tenant → advertiser privilege escalation.** Closed. Route moved from `BuildPublicMux` to `BuildAdminMux` at `POST /api/v1/admin/advertisers` behind `AdminAuthMiddleware`. `cmd/autopilot/client.go:CreateAdvertiser` signature changed to `(adminURL, companyName, email)` and uses `X-Admin-Token` header. Integration regression test `TestP1_2_CreateAdvertiser_BlockedOnPublicPath` in `test/integration/v5_1_hotfix_test.go`.

**P1-3 — `/click?dest=` open redirect.** Closed. The `dest` read + both redirect branches (dedup path + happy path) deleted from `cmd/bidder/main.go:handleClick`. `TestInjectClickTracker_NeverEmitsDestParam` is a static unit-level regression. `TestHandleClick_RejectsArbitraryDest_NoRedirect` is an end-to-end unit-level regression (uses `campaignID=0` + `Producer: nil` to skip the Kafka branch that was hanging the integration-level variant).

### ✅ Done — Phase 1 cascade: 4 pre-existing e2e test failures

The Phase 1 code reviewer surfaced 4 e2e tests failing on parent `a967513` that weren't caused by V5.1 work. I triaged and fixed all 4 in commit `e8991f3`:

- `TestBilling_TopUp_IgnoresBodyAdvertiserID` → renamed to `_RejectsMismatchedBodyAdvertiserID`, assertion flipped from 200 to 400 `"mismatch"` to match the actual handler policy (which was hardened some time after the test was written — handler is correct, test was stale). Added a new `_NoBodyAdvertiserID_CreditsCaller` case for the backward-compat path.
- `TestCreative_Create_BadAdType_400` / `_Update_NotFound_404` / `_Delete_NotFound_404` → these all used `execPublic` with an empty api key, which bypasses auth. V5 hardened the creative handlers to auth-before-validate. Switched to `execAuthed` with a real api key.

Plus 2 tests that broke as a cascade of P1-2's route move:
- `TestAdvertiser_CreateAndGet` and `_Create_MissingFields_400` — they POSTed to `/api/v1/advertisers` via the tenant mux which is no longer registered. Rewrote to invoke `HandleCreateAdvertiser` directly via `httptest.NewRecorder`, matching the pattern `test/integration/create_paths_test.go:43` already uses.

All 7 affected tests now PASS.

### ✅ Done — Phase 1 code review (Phase-level, full branch)

Dispatched `superpowers:code-reviewer` for the full 10-commit branch. Returned **APPROVED with 1 Important + 6 Minor**. Important was fixed in `c754ac7`, re-reviewed, approved clean. Minor items are mostly cleanup debt (comment tightening, test-robustness notes) — none block land.

### ✅ Done — Phase 1 verification-before-completion

Brought up the `dsp-test` docker stack (Postgres 6432, Redis 7380, ClickHouse 9124/10001, Kafka 10094). Ran migrations manually via `docker exec` because the `test-env.sh migrate` script silently swallowed some Postgres migration output via `2>/dev/null`. All schemas landed correctly.

**Test results (all green):**
- `go build ./...` — clean
- `go test ./... -count=1 -timeout 5m` — every package PASS
- `go test -tags e2e ./internal/handler/... -count=1` — full e2e suite PASS (5.3s)
- `go test -tags integration ./test/integration/... -count=1` — full integration suite PASS against live Postgres+Redis+Kafka+ClickHouse (6.0s)
- `go vet -tags integration ./test/integration/...` — clean
- `cd web && npx tsc --noEmit` — clean
- `cd web && npm run lint` — 0 errors (4 pre-existing warnings in `campaigns/[id]/page.tsx` unchanged)
- `grep 'api_key' internal/handler/middleware.go` — zero hits outside the deletion comment
- `grep 'dest' cmd/bidder/main.go` — zero hits in `handleClick` outside the deletion comment

### ✅ Done — Phase 2 plans drafted (committed as docs, not yet implemented)

Three plan docs landed on the branch in commit `c3ac201`:

1. **`docs/REVIEW_REMEDIATION_V5_2A_CONTRACT_PLAN.md`** — contract unification. Delete `/api/v1/docs` hand-built handler, fix circuit-breaker status semantic (currently `"open"` means normal, reversing standard CB lexicon), replace hand-written TS types in `web/lib/api.ts` and `web/lib/admin-api.ts` with `components["schemas"][...]` imports, add `make docs-check` CI gate.
2. **`docs/REVIEW_REMEDIATION_V5_2B_OBSERVABILITY_PLAN.md`** — prometheus business metrics (`dsp_bid_requests_total`, `dsp_wins_total`, `dsp_budget_deducted_cents_total`, etc.), `/health/live` vs `/health/ready` split, real Slack+email alert pipeline replacing `internal/alert.Noop{}`.
3. **`docs/REVIEW_REMEDIATION_V5_2C_SECURITY_LIFECYCLE_PLAN.md`** — bidder `/stats` admin auth, `ApiKeyGate` admin route lockout fix, `admin/layout.tsx` network-error fallback removal, Redis fail-hard with alert, upload legacy dir removal, rate-limit Redis key hashing, `http.Server` slowloris timeouts, `LoadLocation` dedup, `advertiserChargeCents` extraction, `GetCampaign` warm-up guard, `admin.go` JSON decode error.

Each plan is independently executable via `subagent-driven-development` and has its own boundary-loop step.

---

## ❌ NOT done — Phase 1 `/qa` (needs you)

This is the only step that cannot run autonomously. `/qa` needs:
1. The `dsp-test` backend services running (done — I left them up)
2. A running frontend dev server: `cd web && npm run dev`
3. A headless browser session (`/qa` or `/browse` skill)

**What to verify in `/qa`:**

- **Analytics page (the P1-1 end-to-end proof)**: navigate to `/analytics`, ensure you see the "实时连接" indicator go green, live campaign data appears, no console errors, no `?api_key=` in the network tab's EventSource URL, and `?token=...` DOES appear. If you kill Redis and come back, the page should recover after a 2-second backoff with a fresh token.
- **Admin dashboard**: `/admin/*` — confirm ApiKeyGate is still broken for admin-only users (stores the open P2-F1 bug in Phase 2C scope).
- **Campaigns, reports, audit-log**: smoke check no cascading break from the route changes.

**Run when you wake:**

```bash
# Backend should still be up from overnight
docker ps | grep dsp-test

# Start frontend dev server (in a separate terminal or background)
cd web && npm run dev

# Then trigger /qa — it will headless-browser the pages above
# (the /qa skill handles the actual browser driving)
/qa
```

If `/qa` flags anything new, iterate. Per CLAUDE.md's boundary-loop rule, any fix triggers another full Round. But given the code-review + verification-before-completion were both clean, I expect `/qa` to be green too.

---

## What to do next (decision tree)

**Option A — Land Phase 1 immediately**

Skip `/qa`, push the branch, open a PR, merge to main. Acceptable if you trust the automated tests + code review to catch any regression. Carries a residual risk of frontend-only regressions in the analytics page.

```bash
git push -u origin review-remediation-v5.1-hotfix
gh pr create --title "V5.1 security hotfix: 3 P1 findings from 2026-04-15 review" --body "$(cat <<'EOF'
## Summary
- P1-1: close analytics SSE tenant api_key URL leak via short-lived HMAC token
- P1-2: move HandleCreateAdvertiser behind admin auth (privilege escalation)
- P1-3: delete /click?dest= open redirect dead code

## Test plan
- [x] go test ./... -count=1 — all green
- [x] go test -tags e2e ./internal/handler/... — all green
- [x] go test -tags integration ./test/integration/... — all green against live services
- [x] Phase 1 boundary-loop code review (2 rounds + 1 re-review on Important fix) — approved
- [ ] /qa headless browser smoke on /analytics (token flow in real browser)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

**Option B — Run `/qa` first, then land** ← **recommended**

Per the non-negotiable boundary-loop rule in CLAUDE.md, this is the correct path. The full loop has to have a zero-issue round before land. Since `/qa` hasn't run, Round 1 is technically incomplete.

**Option C — Skip land, start Phase 2A**

The three Phase 2 plans are ready. V5.1 can land any time within the next few days; starting 2A now parallelizes progress. But this accumulates unpushed work on an unmerged branch, which violates the "don't let branches diverge too far" rule of thumb.

My recommendation: **Option B**. Run `/qa`, confirm analytics token flow works in a real browser, merge, tag a release, then start Phase 2A on a fresh branch off main.

---

## Where the plan file is

`docs/REVIEW_REMEDIATION_V5_1_HOTFIX_PLAN.md` (committed in `ad0d96e`'s ancestor tree — actually committed later but on this branch). It's the authoritative spec for what Phase 1 delivered. Each task matches one or more commits; the boundary loop is Task 9 and is the one-remaining-step (`/qa`).

## Memory updated

New entry: `project_v5_1_hotfix.md` in `~/.claude/projects/C--Users-Roc-github-dsp/memory/`, indexed in `MEMORY.md`. Next session will pick up context automatically.
