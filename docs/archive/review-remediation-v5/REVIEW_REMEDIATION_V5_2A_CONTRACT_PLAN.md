# V5.2A Contract Unification — Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` to execute this plan. Each task goes implementer → spec reviewer → code quality reviewer. Reviewer-triggered fixes require re-dispatching the same reviewer. Phase 2A boundary loop at the end is non-negotiable per CLAUDE.md.

**Goal:** Resolve the "multi-source contract drift" that the 2026-04-15 independent Claude + Codex review flagged across the DSP API contract surface. Make the generated OpenAPI spec the single source of truth, eliminate hand-written TypeScript response types that silently drift from the backend, and fix a documented semantic-reversal in the circuit breaker status field.

**Architecture:**
- Each P2 finding is addressed independently with test-driven verification. The plan is NOT a big-bang rewrite — each fix is an atomic commit that leaves the contract more consistent than it found it.
- **Two sources of drift are removed**: the `/api/v1/docs` hand-built route is deleted (it no longer matches anything generated) and the `web/lib/api.ts` / `web/lib/admin-api.ts` hand-written response types are replaced with `components["schemas"][...]` imports from `web/lib/api-types.ts`.
- **CI gate** (`make docs-check` or equivalent) is added so contract drift fails at PR time instead of showing up months later.

**Tech Stack:** Go 1.23, swaggo/swag for OpenAPI gen, `swagger2openapi` for 2.0→3.0 conversion, `openapi-typescript` for TS type gen, Next.js App Router + TypeScript on the frontend.

**Scope — NOT in this plan:**
- New features or new endpoints — this is pure alignment work.
- Runtime handler behavior changes unless the contract documents a different behavior than runtime (then the handler is adjusted to match the documented behavior, with the documented behavior winning on taste).
- Frontend visual changes. Type-level replacements are the scope.
- Phase 2B (observability) and Phase 2C (security-adjacent + lifecycle) findings.

---

## File Structure

**Modified files:**
- `internal/handler/routes.go` — delete the `/api/v1/docs` registration (handler file is deleted below)
- `internal/handler/docs.go` — DELETE entirely (hand-built `HandleAPIDocs` detached from the real router)
- `internal/handler/guardrail.go` — fix circuit-breaker status semantic (`"open"` currently means "normal/closed-for-business-as-usual", violating standard CB convention)
- `web/lib/admin-api.ts` — `CircuitStatus` interface deleted, replaced with `components["schemas"]["handler.CircuitStatusResponse"]` import; all admin response interfaces follow suit
- `web/lib/api.ts` — all hand-written billing/stats/bid response interfaces replaced with generated type imports
- `web/app/admin/**/*.tsx` — update callers of the renamed CB status field (if any)
- `Makefile` or `scripts/docs-check.sh` — new CI gate that re-runs `swag init` + `npm run generate:api` and fails on any diff

**New files:**
- `scripts/docs-check.sh` — idempotent regeneration + `git diff --exit-code` on tracked generated artifacts
- `internal/handler/guardrail_test.go` — add tests locking the new CB status semantic if tests don't exist
- `test/integration/v5_2a_contract_test.go` — end-to-end assertion that the generated OpenAPI spec matches what the handler actually emits on representative endpoints

---

## Normative contract decisions (locked before any task starts)

### Circuit breaker status semantic

**Current (bug):**
- `internal/handler/guardrail.go` emits `"status": "open"` when the breaker is CLOSED (normal operation, traffic flowing)
- Emits `"status": "tripped"` when the breaker is OPEN (fail-fast, traffic blocked)

This reverses the standard circuit-breaker lexicon. An operator reading a Grafana dashboard that says `circuit_breaker: "open"` will 99% of the time interpret that as "broken / something's wrong" not "healthy". Runtime bug is masked because both backend and frontend happen to share the same wrong label.

**New (locked):**
- `"closed"` = normal operation, traffic flowing (industry standard: breaker is closed → circuit is connected)
- `"open"` = tripped, fail-fast, traffic blocked (industry standard: breaker is open → circuit is broken)

**Migration note:** The old values `"open"` (=normal) and `"tripped"` (=failing) are NOT kept as aliases. This is a renaming of externally-visible strings; the Phase 2A commit updates backend, frontend, generated contract, and tests atomically. Any operator dashboard that was hard-coded to the old values needs a one-line fix.

### Hand-written TS type removal

**Current:** `web/lib/api.ts` has ~15 hand-written interfaces (`CampaignStats`, `HourlyStats`, `GeoStats`, `BidDetail`, `Transaction`, `OverviewStats`, etc.) that duplicate types already emitted into `web/lib/api-types.ts` by `openapi-typescript`.

**New:**
- `web/lib/api.ts` keeps its API client functions but imports all response types from `web/lib/api-types.ts`:
  ```ts
  import type { components } from "./api-types";
  type CampaignStats = components["schemas"]["internal_reporting.CampaignStats"];
  ```
- `web/lib/admin-api.ts` does the same for admin types.
- Any hand-written type that has NO generated equivalent is flagged — the backend struct is annotated with `@Success {object} handler.<Name>` so swag emits it.

### `/api/v1/docs` deletion

**Current:** `internal/handler/docs.go:HandleAPIDocs` hand-builds a JSON listing of routes. Codex review found it was already detached from the real router (routes documented there didn't match the mux). It's a third source of truth competing with swag+openapi3.

**New:** Delete the hand-built handler entirely. The canonical docs path is swagger UI served from `docs/generated/swagger.json` (or whatever static host the frontend uses).

### `make docs-check`

**Current:** There's no CI step that re-runs swag+openapi-typescript and fails on drift. `88a1413` (V5 closeout) had to regenerate these files manually after a review found they were stale.

**New:** `make docs-check` (or `scripts/docs-check.sh`) runs:
1. `swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal`
2. `cd web && npm run generate:api`
3. `git diff --exit-code -- docs/generated web/lib/api-types.ts`

If any tracked generated file differs from the regenerated output, the script exits non-zero. CI pipeline runs this as a required check.

---

## Task 0: Baseline + contract audit

**Purpose:** Catalog every drift point before touching code so the fix scope is locked. This prevents mid-task scope creep and lets the reviewer verify each drift was addressed.

- [ ] **Step 1: Confirm branch + git state**

```bash
git checkout -b review-remediation-v5.2a-contract
git log --oneline -5
git status
go test ./... -count=1 -timeout 5m
```

All green baseline. If anything fails, STOP — establish a clean baseline first.

- [ ] **Step 2: Produce the drift catalog**

Create `docs/REVIEW_REMEDIATION_V5_2A_DRIFT_CATALOG.md` with:

1. Every `interface` or `type` declaration in `web/lib/api.ts` and `web/lib/admin-api.ts`, grouped by whether a generated equivalent exists in `web/lib/api-types.ts`.
2. The current value + type of the `circuit_breaker` field in `internal/handler/guardrail.go` and every frontend caller that reads it.
3. Every route registration in `internal/handler/routes.go` that doesn't have a `@Router` annotation on its handler godoc.
4. The `/api/v1/docs` route map from `internal/handler/docs.go` side-by-side with the real `BuildPublicMux` + `BuildAdminMux` route map, flagging every drift entry.

This is a documentation deliverable — no code changes. The reviewer uses it to verify every drift is addressed by a subsequent task.

- [ ] **Step 3: Commit the catalog**

```bash
git add docs/REVIEW_REMEDIATION_V5_2A_DRIFT_CATALOG.md
git commit -m "docs(v5.2a): contract drift catalog — sources identified before fix"
```

---

## Task 1: Delete `/api/v1/docs` hand-built handler

**Files:**
- Delete: `internal/handler/docs.go`
- Modify: `internal/handler/routes.go` (remove the registration)
- Modify: `internal/handler/e2e_authz_table_test.go` (remove `/api/v1/docs` from any exemption list if present)

- [ ] **Step 1: Confirm nothing imports the handler**

```bash
grep -rn "HandleAPIDocs\|internal/handler/docs.go" . --include="*.go" 2>&1 | head
```

Expected: the only production reference is `routes.go`. The file itself can be deleted.

- [ ] **Step 2: Write failing test**

Add to `internal/handler/e2e_authz_table_test.go`:

```go
func TestAPIDocs_NoLongerRegistered(t *testing.T) {
	d := mustDeps(t)
	mux := handler.BuildPublicMux(d)
	req := httptest.NewRequest("GET", "/api/v1/docs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("/api/v1/docs expected 404 after V5.2A deletion, got %d", w.Code)
	}
}
```

Run: `go test ./internal/handler/ -tags e2e -run TestAPIDocs_NoLongerRegistered -v` → expect FAIL (current registration returns 200).

- [ ] **Step 3: Delete the file + registration**

```bash
rm internal/handler/docs.go
```

Edit `internal/handler/routes.go`:
- Delete the line `mux.HandleFunc("GET /api/v1/docs", d.HandleAPIDocs)`.
- If `WithAuthExemption` exempts `/api/v1/docs`, delete that exemption too (the route is gone — no auth decision needed for it).

- [ ] **Step 4: Run tests — expect PASS**

```bash
go build ./...
go test ./internal/handler/ -tags e2e -run TestAPIDocs_NoLongerRegistered -v
go test ./... -count=1
```

- [ ] **Step 5: Regenerate OpenAPI docs**

```bash
swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal
cd web && npm run generate:api && cd ..
```

Expected: `docs/generated/*` no longer references `/api/v1/docs`.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(v5.2a): delete hand-built /api/v1/docs route

The /api/v1/docs handler at internal/handler/docs.go was a hand-built
JSON listing of routes that had drifted from the real mux. Codex
review (2026-04-15) found routes documented there that didn't match
BuildPublicMux/BuildAdminMux. It was a third source of truth
competing with swag+openapi3.

Canonical docs are now served from docs/generated/swagger.json +
openapi3.yaml only. The route is deleted, the exemption is deleted,
and TestAPIDocs_NoLongerRegistered locks in that the old path
returns 404."
```

---

## Task 2: Fix circuit breaker status semantic

**Files:**
- Modify: `internal/handler/guardrail.go` (update the status values and docstrings)
- Modify: `internal/guardrail/guardrail.go` or wherever `IsTripped()` / `TripReason()` live (check for any internal callers that read the stringified status)
- Modify: `web/app/admin/page.tsx` and any other frontend caller that reads `circuit_breaker: string`
- Modify: `web/lib/admin-api.ts` `CircuitStatus` interface (replaced with generated type in Task 5)
- Modify: `web/lib/api.ts` `CircuitStatus` (same)
- Add: `internal/handler/guardrail_test.go` test case locking the new values

- [ ] **Step 1: Grep for callers**

```bash
grep -rn 'circuit_breaker\|CircuitStatus\|"open"\|"tripped"\|"closed"' internal/handler/ web/ --include="*.go" --include="*.ts" --include="*.tsx"
```

Catalog every production reference. Reviewer checks this list against the final commit.

- [ ] **Step 2: Write failing test**

Add to `internal/handler/guardrail_test.go`:

```go
func TestHandleCircuitStatus_UsesStandardCBSemantics(t *testing.T) {
	// With the new V5.2A semantics:
	//   "closed" = normal operation (breaker is closed → circuit connected)
	//   "open"   = tripped (breaker is open → circuit broken)
	d := mustDeps(t)

	// 1. Initially closed
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/admin/circuit-status", nil)
	d.HandleCircuitStatus(w, req)
	var resp struct {
		Status string `json:"circuit_breaker"`
	}
	decodeJSON(t, w, &resp)
	if resp.Status != "closed" {
		t.Fatalf("expected 'closed' (normal), got %q", resp.Status)
	}

	// 2. After Trip()
	d.Guardrail.Trip(context.Background(), "test trip")
	w = httptest.NewRecorder()
	d.HandleCircuitStatus(w, req)
	decodeJSON(t, w, &resp)
	if resp.Status != "open" {
		t.Fatalf("expected 'open' (tripped), got %q", resp.Status)
	}
}
```

Run → expect FAIL.

- [ ] **Step 3: Fix the handler**

Edit `internal/handler/guardrail.go`. Change the two string emissions:
- Where it currently writes `"status": "open"` when normal → write `"closed"`
- Where it currently writes `"status": "tripped"` when tripped → write `"open"`

Update the handler godoc to document the new semantic.

- [ ] **Step 4: Fix frontend callers**

For each caller found in Step 1, update the comparison:
- Was: `circuit.circuit_breaker === "tripped"` (when checking if tripped)
- Now: `circuit.circuit_breaker === "open"`

And:
- Was: `circuit.circuit_breaker === "open"` (when checking if normal) — hopefully none, since the old semantic was confusing
- Now: `circuit.circuit_breaker === "closed"`

- [ ] **Step 5: Run tests**

```bash
go test ./internal/handler/ -run TestHandleCircuitStatus -v
go test ./... -count=1
cd web && npx tsc --noEmit && npm run lint
```

- [ ] **Step 6: Regenerate + commit**

```bash
swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal
cd web && npm run generate:api && cd ..
git add -A
git commit -m "fix(v5.2a): circuit breaker status uses standard CB semantics

Rename circuit_breaker status values to match industry convention:
  closed = normal operation (traffic flowing)
  open   = tripped (fail-fast, traffic blocked)

Previously the handler emitted 'open' for normal and 'tripped' for
tripped, reversing the standard CB lexicon and causing operators
reading dashboards to 99% misinterpret 'circuit_breaker: open' as
'something is broken' when it actually meant 'everything is fine'.

No runtime behavior change — this is purely a string rename across
backend, frontend, and generated contract. Callers in
web/app/admin/page.tsx and web/lib/{api,admin-api}.ts are updated
atomically."
```

---

## Task 3: Annotate un-annotated handlers so swag picks them up

**Files:**
- Modify: various `internal/handler/*.go` files — add `@Router` and related annotations to handlers that lack them

- [ ] **Step 1: Run the drift-catalog step to list handlers without `@Router`**

Read Task 0 Step 2's catalog. For each handler without a swag annotation, add one in this task.

- [ ] **Step 2: For each handler, add the annotations per the existing repo pattern**

Use existing annotated handlers as templates (`HandleCampaignStats`, `HandleTopUp`, etc.). Annotations:
- `@Summary` — one-line description
- `@Tags` — category matching existing convention
- `@Security` — `ApiKeyAuth` for tenant-scoped, `AdminAuth` for admin, `SSETokenAuth` for SSE
- `@Param` — for path/query/body params
- `@Success` — typed response via `{object} handler.<TypeName>`
- `@Failure` — error cases
- `@Router` — path + method

- [ ] **Step 3: For each `@Success` that references an inline `object{...}` schema, extract to a named struct**

This gives `openapi-typescript` a named type to export instead of an anonymous inline type — makes frontend imports cleaner.

- [ ] **Step 4: Regenerate + verify**

```bash
swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal
cd web && npm run generate:api && cd ..
```

Check `docs/generated/openapi3.yaml` — the number of un-annotated `HandleFunc` registrations should be zero.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "docs(v5.2a): add swag annotations to un-annotated handlers

Every production handler now has a @Router annotation so swag picks
it up. Inline object{...} response schemas extracted to named
handler types for cleaner openapi-typescript output."
```

---

## Task 4: Replace hand-written TS types with generated imports (frontend)

**Files:**
- Modify: `web/lib/api.ts` — replace all hand-written interfaces with `components["schemas"][...]` imports
- Modify: `web/lib/admin-api.ts` — same
- Modify: `web/app/**/*.tsx` — update any importers that imported the old hand-written types from `web/lib/api.ts`

- [ ] **Step 1: Catalog every hand-written type in both files**

From Task 0 Step 2.

- [ ] **Step 2: For each, find the generated equivalent in `web/lib/api-types.ts`**

If one exists, replace. If not, backtrack to Task 3 and annotate the handler to emit the type.

- [ ] **Step 3: Replace, file by file**

For each hand-written type, replace the `interface Foo { ... }` declaration with:

```ts
import type { components } from "./api-types";
type Foo = components["schemas"]["internal_handler.FooResponse"];
```

- [ ] **Step 4: Run typecheck + lint**

```bash
cd web && npx tsc --noEmit && npm run lint
```

Any type mismatch means the hand-written type was wrong in a way that will break callers. Fix the caller, not the generated type.

- [ ] **Step 5: Commit**

```bash
git add web/lib/api.ts web/lib/admin-api.ts web/app/
git commit -m "refactor(v5.2a): frontend imports types from api-types.ts

web/lib/api.ts and web/lib/admin-api.ts no longer hand-write
response type interfaces. All response types are now imported from
the generated web/lib/api-types.ts so backend-frontend drift fails
at typecheck time instead of silently diverging.

Callers in web/app/* updated to match the generated type names
where they differ from the old hand-written names."
```

---

## Task 5: Add `make docs-check` CI gate

**Files:**
- Create: `scripts/docs-check.sh`
- Modify: `Makefile` (add `docs-check` target)
- Modify: `.github/workflows/ci.yml` or equivalent (add docs-check step)

- [ ] **Step 1: Write the script**

Create `scripts/docs-check.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

echo "Regenerating OpenAPI + TS types..."
swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal >/dev/null 2>&1
(cd web && npm run generate:api >/dev/null 2>&1)

if ! git diff --quiet -- docs/generated web/lib/api-types.ts; then
    echo "ERROR: generated contract is out of date."
    echo "Run: swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal"
    echo "Then: cd web && npm run generate:api"
    echo ""
    echo "Diff:"
    git diff -- docs/generated web/lib/api-types.ts
    exit 1
fi
echo "OK: generated contract is up to date."
```

- [ ] **Step 2: Add Makefile target**

```makefile
.PHONY: docs-check
docs-check:
	@./scripts/docs-check.sh
```

- [ ] **Step 3: Wire into CI**

Add a step in `.github/workflows/ci.yml` (or whatever CI config the repo uses):

```yaml
- name: Contract drift check
  run: make docs-check
```

- [ ] **Step 4: Verify locally**

```bash
make docs-check
```

Expected: OK output.

Then make a deliberate drift (edit a godoc `@Summary`) and re-run:

```bash
make docs-check
```

Expected: non-zero exit + diff shown.

Revert the deliberate drift.

- [ ] **Step 5: Commit**

```bash
git add scripts/docs-check.sh Makefile .github/workflows/ci.yml
git commit -m "feat(v5.2a): add make docs-check CI gate

Regenerates swag + openapi-typescript outputs and fails if the
tracked files drift from what regeneration produces. CI runs this
on every PR so contract drift becomes a PR-time failure instead of
a months-later 'wait, the spec is lying' surprise."
```

---

## Task 6: Integration test — generated spec matches runtime

**Files:**
- Create: `test/integration/v5_2a_contract_test.go`

- [ ] **Step 1: Write the test**

For a representative set of endpoints (one per category: tenant-scoped, admin, SSE, billing, report), read the generated `docs/generated/openapi3.yaml` and compare against what the handler actually emits.

Minimal version:
1. Parse `openapi3.yaml`
2. For each of ~10 representative endpoints, extract the response schema
3. Hit the endpoint through `shared.Server` with a real authenticated request
4. Decode the response into `map[string]any`
5. Verify every top-level field in the response matches a field in the schema (type + optional/required)

The test doesn't have to be exhaustive — catching drift on 10 representative endpoints proves the regeneration pipeline is wired up correctly and the `make docs-check` gate is doing its job.

- [ ] **Step 2: Run**

```bash
go test -tags integration ./test/integration/ -run TestV5_2A_ContractMatchesRuntime -v
```

- [ ] **Step 3: Commit**

```bash
git add test/integration/v5_2a_contract_test.go
git commit -m "test(v5.2a): integration test verifies generated spec matches runtime"
```

---

## Task 7: Phase 2A boundary loop

Same structure as Phase 1 boundary loop:
- [ ] Round 1 requesting-code-review → fix Critical/Important → re-review
- [ ] Round 1 verification-before-completion (`test-env.sh up`, `make docs-check`, `go test ./... -count=1`, `go test -tags integration ./...`)
- [ ] Round 1 `/qa` (headless browser smoke of circuit-status display on /admin, analytics page still works, campaigns list still works)
- [ ] Loop until a round is zero-issue. Max 5 rounds.

---

## Out of scope (Phase 2B / 2C)

- Observability metrics, health check split, alert pipeline → Phase 2B
- bidder `/stats` unauth, ApiKeyGate admin routes, Redis rate-limit fail-hard, upload legacy dir, http.Server timeouts → Phase 2C
- Per-advertiser SSE rate limit bucket → Phase 2C debt

## Self-review (completed by plan author)

**Spec coverage:** Every P2 contract finding from the 2026-04-15 independent review that the primary reviewer categorized as "Contract 统一" has a task: (1) `/api/v1/docs` hand-built deletion → Task 1, (2) `/billing/balance/{id}` already done via e39a35d, (3) hand-written TS types → Task 4 (with Task 3 as backing work so swag has something to generate from), (4) CB semantic → Task 2, (5) missing `make docs-check` CI gate → Task 5. Runtime-spec drift integration test → Task 6.

**Placeholder scan:** No TBD. Every task has actual commands and actual code. One forward reference (Task 4 depends on Task 3 annotations being in place) is documented.

**Type consistency:** The plan picks `"closed"`/`"open"` for CB status and uses those values consistently in Task 2's test, frontend fix, and commit message. No drift.

**CLAUDE.md alignment:** Per-task two-stage review implied by "subagent-driven-development"; Phase 2A boundary loop explicit at Task 7; integration tests hit real Store; no nil-store stubs for tenant-isolation-adjacent assertions.
