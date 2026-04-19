# Function Chain Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 5 codex-reported issues on the `POST /campaigns/{id}/start → bidder 生效` and `POST /bid → ClickHouse 入库` chains: exchange bid missing click tracker, fail-open on balance error, non-atomic activation, no body size limit on direct /bid, win events using recalculated values.

**Architecture:** 3 sequential phases, each shipping as its own PR. Phase 1 = `BillingService` interface refactor + small bug fixes (F2 covering err + nil, F4 body limit). Phase 2 = shared bid response decorator + win URL metadata with transitional HMAC validation (F1 + F5). Phase 3 = activation pub/sub metric + contract docs (F3). CEO review added Task 0 (BillingService interface), nil-path fail-closed, transitional token validation, and `math.Round` for bid price cents. Every bug fix follows the TDD Evidence Rule — failing test commit BEFORE fix commit, no squash.

**Tech Stack:** Go 1.22+, PostgreSQL (via pgx), Redis (go-redis/v9), Kafka, ClickHouse, OpenRTB 2.5, HMAC-SHA256.

**Spec:** [docs/superpowers/specs/2026-04-19-function-chain-fixes-design.md](../specs/2026-04-19-function-chain-fixes-design.md)

---

## File Structure

**Created:**
- `internal/handler/billing_iface.go` — `BillingService` interface (Phase 1, Task 0 — CEO Finding #1)
- `cmd/bidder/decorator.go` — shared `decorateBidResponse()` (Phase 2, Task 3)
- `cmd/bidder/bid_body_limit_test.go` — F4 test (Phase 1, Task 2)

**Modified:**
- `internal/handler/handler.go` — `Deps.BillingSvc` type `*billing.Service` → `BillingService` (Phase 1, Task 0)
- `cmd/api/main.go` — non-nil startup assert on `BillingSvc` (Phase 1, Task 0 — CEO Finding #2 defense-in-depth)
- `internal/handler/campaign.go` — F2 fail-closed incl. nil path (Phase 1), F3 metric record (Phase 3)
- `internal/handler/e2e_public_campaign_test.go` — F2 tests (stub + nil) + F3 metric test
- `cmd/bidder/main.go` — F4 body limit (Phase 1), F1/F5 decorator wiring (Phase 2), F5 win handler URL parse + transitional token validation (Phase 2)
- `internal/auth/hmac.go` — no signature change (variadic already); `FormatTokenParams` extended (Phase 2)
- `cmd/bidder/handlers_integration_test.go` — F1 exchange test + F5 win metadata test + token generation (Phase 2)
- `cmd/bidder/main_test.go` — update 4 token calls (Phase 2)
- `internal/auth/hmac_test.go` — update assertions for new params (Phase 2)
- `internal/bidder/loader.go` — `NotifyCampaignUpdate` returns error (Phase 3)
- `internal/observability/metrics.go` — new counters `campaign_activation_pubsub_failures_total` (Phase 3) + `bidder_token_legacy_accepted_total` (Phase 2 — CEO Finding #3)
- `internal/handler/campaign.go` + `HandlePauseCampaign` + `HandleUpdateCampaign` — record metric on pub/sub error (Phase 3)
- `docs/contracts/campaigns.md` (or OpenAPI spec) — 30s eventual-consistency contract (Phase 3)

**Test file tags:** handler e2e tests use `//go:build e2e`; bidder unit tests are untagged; bidder handlers integration tests live in `cmd/bidder/handlers_integration_test.go`.

---

## Phase 1 — Small bug fixes (F2 + F4)

One PR, 5 commits (includes 1 enabling refactor per CEO Finding #1). Task 0 is the enabler; Tasks 1-2 are the actual bug fixes.

### Task 0 — Refactor `BillingSvc` to interface (CEO Finding #1)

Non-functional refactor. Enables F2 testability: FK `campaigns.advertiser_id REFERENCES advertisers(id)` has no `ON DELETE CASCADE`, so `DELETE FROM advertisers` fails with `23503` when campaigns exist. Without interface we can't inject a failing stub.

**Files:**
- Create: `internal/handler/billing_iface.go`
- Modify: `internal/handler/handler.go` (change `BillingSvc` field type)
- Modify: `cmd/api/main.go` (non-nil startup assert per CEO Finding #2 defense-in-depth)

- [ ] **Step 0.1: Create the interface file**

Create `internal/handler/billing_iface.go`:

```go
package handler

import "context"

// BillingService is the narrow handler-facing view of billing.Service.
// Defining it consumer-side (here) lets tests inject minimal stubs
// without pulling the full service + its DB. billing.Service satisfies
// this interface automatically — no changes needed to the concrete type.
type BillingService interface {
	GetBalance(ctx context.Context, advertiserID int64) (balanceCents int64, billingType string, err error)
}
```

- [ ] **Step 0.2: Change `Deps.BillingSvc` field type**

Edit `internal/handler/handler.go`. Find the `Deps` struct (line ~23) and change:
```go
BillingSvc  *billing.Service
```
to:
```go
BillingSvc  BillingService
```

If the `"github.com/heartgryphon/dsp/internal/billing"` import becomes unused in handler.go after this change, remove it. Otherwise leave it.

- [ ] **Step 0.3: Add startup non-nil assert in cmd/api/main.go**

In `cmd/api/main.go`, locate the `handler.Deps{...}` literal. Immediately after the Deps is assembled and before the server starts, add:

```go
if deps.BillingSvc == nil {
	log.Fatal("BillingSvc required at startup; check wiring")
}
```

This is defense-in-depth for CEO Finding #2 — handler layer will also 503 on nil, but startup fail-fast keeps the deploy from silently launching a non-billing server.

- [ ] **Step 0.4: Verify build + existing tests unchanged**

```bash
go build ./...
go test ./... -short
go test -tags=e2e ./internal/handler/ -v
```
Expected: all PASS. Interface is structurally compatible with `*billing.Service`.

- [ ] **Step 0.5: Commit refactor**

```bash
git add internal/handler/billing_iface.go internal/handler/handler.go cmd/api/main.go
git commit -m "refactor(handler): extract BillingService interface for testability

Pure non-functional refactor (no behavior change). billing.Service
already satisfies the new interface shape. Enables handler tests to
inject failing stubs for the F2 fail-closed regression test in the
next commit — FK constraints prevent the alternative (deleting the
advertiser row) in integration tests.

Adds a startup assert in cmd/api/main.go: if wiring produces a nil
BillingSvc, fail fast rather than silently launch a server that will
skip balance checks on /start (fail-open). Handler layer still 503s
on nil as nested defense."
```

### Task 1 — F2: `HandleStartCampaign` fail-closed on balance error + nil BillingSvc

**Files:**
- Test: `internal/handler/e2e_public_campaign_test.go` (append new tests + stub)
- Modify: `internal/handler/campaign.go:326-333`

**Approach:** With the `BillingService` interface from Task 0, inject a `failingBillingStub{}` to force `GetBalance` to return an error. Separate test case mutates `d.BillingSvc = nil` to cover the nil-path fail-closed (CEO Finding #2).

- [ ] **Step 1.1: Write the failing tests (two cases: errored BillingSvc + nil BillingSvc)**

Append to `internal/handler/e2e_public_campaign_test.go`:

```go
// failingBillingStub satisfies handler.BillingService. GetBalance always
// returns a forced error. Used to test the F2 fail-closed path without
// tampering with the advertisers table (FK constraints forbid that).
type failingBillingStub struct{}

func (failingBillingStub) GetBalance(_ context.Context, _ int64) (int64, string, error) {
	return 0, "", errors.New("forced billing error for test")
}

// TestCampaign_StartReturns503WhenBalanceErrors covers the first F2
// fail-closed path: BillingSvc.GetBalance returns a non-nil error.
// Pre-fix: the handler's `if err == nil && balance < ...` swallowed
// the error and fell through to TransitionStatus(active) — a fail-open.
// Post-fix: 503 Service Unavailable.
func TestCampaign_StartReturns503WhenBalanceErrors(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	_ = newCreative(t, d, campaignID)

	// Swap in the failing stub for this test only.
	originalSvc := d.BillingSvc
	d.BillingSvc = failingBillingStub{}
	t.Cleanup(func() { d.BillingSvc = originalSvc })

	req := authedReq(t, http.MethodPost,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignID, 10)+"/start", nil, apiKey)
	w := execAuthed(t, d, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on GetBalance error, got %d: %s",
			w.Code, w.Body.String())
	}
}

// TestCampaign_StartReturns503WhenBillingSvcNil covers the second F2
// fail-closed path (CEO Finding #2): non-sandbox campaign + BillingSvc
// is nil. Pre-fix: the handler's `if d.BillingSvc != nil` guard silently
// bypassed balance checking — a second fail-open surface. Post-fix: 503.
func TestCampaign_StartReturns503WhenBillingSvcNil(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	_ = newCreative(t, d, campaignID)

	originalSvc := d.BillingSvc
	d.BillingSvc = nil
	t.Cleanup(func() { d.BillingSvc = originalSvc })

	req := authedReq(t, http.MethodPost,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignID, 10)+"/start", nil, apiKey)
	w := execAuthed(t, d, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when BillingSvc is nil (non-sandbox), got %d: %s",
			w.Code, w.Body.String())
	}
}
```

Add imports at the top of the file if not already present: `"context"`, `"errors"`.

- [ ] **Step 1.2: Run tests to verify both fail (RED evidence)**

```bash
go test -tags=e2e ./internal/handler/ -run "TestCampaign_StartReturns503WhenBalanceErrors|TestCampaign_StartReturns503WhenBillingSvcNil" -v
```
Expected: both FAIL. `...BalanceErrors` returns 200 (pre-fix: err swallowed → fell through to TransitionStatus → 200 active). `...BillingSvcNil` returns 200 (pre-fix: nil guard silently skipped check → fell through).

- [ ] **Step 1.3: Commit failing tests**

```bash
git add internal/handler/e2e_public_campaign_test.go
git commit -m "test(handler): add failing regression tests for fail-open on balance + nil

Two cases cover F2 (and CEO Finding #2):
- GetBalance returns err → currently 200, should be 503
- BillingSvc is nil on non-sandbox campaign → currently 200, should be 503

Expect-Fail: TestCampaign_StartReturns503WhenBalanceErrors
Expect-Fail: TestCampaign_StartReturns503WhenBillingSvcNil"
```

- [ ] **Step 1.4: Implement the fix**

Edit `internal/handler/campaign.go:326-333`. Replace:
```go
	// Check advertiser balance before starting (skip for sandbox campaigns)
	if !c.Sandbox && d.BillingSvc != nil {
		balance, _, err := d.BillingSvc.GetBalance(r.Context(), advID)
		if err == nil && balance < c.BudgetDailyCents {
			WriteError(w, http.StatusUnprocessableEntity, "insufficient balance: please top up before starting campaign")
			return
		}
	}
```

With:
```go
	// Check advertiser balance before starting. Fail-closed on both
	// surfaces: a query error and a missing BillingSvc. Sandbox
	// campaigns are exempt from balance verification.
	if !c.Sandbox {
		if d.BillingSvc == nil {
			log.Printf("[CAMPAIGN] BillingSvc nil at runtime campaign=%d adv=%d", id, advID)
			WriteError(w, http.StatusServiceUnavailable, "unable to verify balance, please retry")
			return
		}
		balance, _, err := d.BillingSvc.GetBalance(r.Context(), advID)
		if err != nil {
			log.Printf("[CAMPAIGN] balance check failed campaign=%d adv=%d: %v", id, advID, err)
			WriteError(w, http.StatusServiceUnavailable, "unable to verify balance, please retry")
			return
		}
		if balance < c.BudgetDailyCents {
			WriteError(w, http.StatusUnprocessableEntity, "insufficient balance: please top up before starting campaign")
			return
		}
	}
```

Confirm `log` is imported at the top of `internal/handler/campaign.go` (it should be — existing handlers in the file log via the same package).

- [ ] **Step 1.5: Run tests to verify both pass (GREEN)**

```bash
go test -tags=e2e ./internal/handler/ -run "TestCampaign_StartReturns503WhenBalanceErrors|TestCampaign_StartReturns503WhenBillingSvcNil" -v
```
Expected: both PASS.

- [ ] **Step 1.6: Run full handler test suite to check no regression**

```bash
go test -tags=e2e ./internal/handler/ -v
go test ./... -short
```
Expected: all PASS. `TestCampaign_StartPublishesActivated` still passes because `mustDeps(t)` wires a real `BillingSvc` with the seeded 1e6 cent balance.

- [ ] **Step 1.7: Commit fix**

```bash
git add internal/handler/campaign.go
git commit -m "fix(handler): return 503 when BillingSvc errors or is nil [closes F2]

Previously the handler used \`if d.BillingSvc != nil { ... if err == nil &&
balance < budget_daily ... }\` which had two fail-open surfaces:

1. GetBalance returning err → err was silently dropped, fell through to
   TransitionStatus(active). Any billing DB hiccup allowed insufficient-
   balance campaigns to start.
2. BillingSvc being nil at runtime → the outer guard silently skipped
   balance checks entirely, so a non-sandbox campaign could activate
   without any balance verification at all.

Now both paths 503 with a retryable message. Sandbox campaigns remain
exempt. Defense-in-depth: cmd/api/main.go asserts BillingSvc non-nil
on startup (Task 0)."
```

### Task 2 — F4: Direct `/bid` body size limit

**Files:**
- Test: `cmd/bidder/bid_body_limit_test.go` (new)
- Modify: `cmd/bidder/main.go:270-275`

- [ ] **Step 2.1: Write the failing test**

Create `cmd/bidder/bid_body_limit_test.go`:

```go
package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleBid_RejectsOversizedBody verifies direct /bid enforces a 1MB
// body cap via http.MaxBytesReader, matching the exchange path's limit.
// Pre-fix: no limit — a 2MB body decoded fully, risking memory blow-up
// on public endpoints. Post-fix: 413 Request Entity Too Large.
func TestHandleBid_RejectsOversizedBody(t *testing.T) {
	d := &Deps{} // Engine nil — handler short-circuits before Engine.Bid
	// Build a 2MB JSON body: valid OpenRTB skeleton + padding to blow past 1MB.
	padding := strings.Repeat("x", 2<<20) // 2MB of 'x'
	bodyJSON := `{"id":"oversized","imp":[{"id":"1"}],"site":{"id":"s","domain":"` + padding + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/bid", bytes.NewReader([]byte(bodyJSON)))
	w := httptest.NewRecorder()

	d.handleBid(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		// Drain body for clearer failure msg
		body, _ := io.ReadAll(w.Body)
		t.Fatalf("expected 413 for 2MB body, got %d: %s", w.Code, string(body))
	}
}
```

- [ ] **Step 2.2: Run test to verify it fails**

```bash
go test ./cmd/bidder/ -run TestHandleBid_RejectsOversizedBody -v
```
Expected: FAIL — pre-fix the handler decodes the 2MB body (possibly returns 400 from JSON decode error on the engine path, or 500 if Engine.Bid panics on nil — either way NOT 413).

- [ ] **Step 2.3: Commit failing test**

```bash
git add cmd/bidder/bid_body_limit_test.go
git commit -m "test(bidder): add failing regression test for oversized /bid body rejection

Expect-Fail: TestHandleBid_RejectsOversizedBody"
```

- [ ] **Step 2.4: Implement the fix**

Edit `cmd/bidder/main.go:269-275`:

Replace:
```go
	var req openrtb2.BidRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		observability.BidRequestsTotal.WithLabelValues("direct", "rejected").Inc()
		http.Error(w, `{"error":"invalid bid request"}`, http.StatusBadRequest)
		return
	}
```

With:
```go
	// Enforce 1MB body cap to match exchange path and protect against
	// OOM on public /bid endpoint. MaxBytesReader returns *http.MaxBytesError
	// when exceeded; the handler maps that to 413 and everything else to 400.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req openrtb2.BidRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		observability.BidRequestsTotal.WithLabelValues("direct", "rejected").Inc()
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, `{"error":"request body too large"}`, http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, `{"error":"invalid bid request"}`, http.StatusBadRequest)
		return
	}
```

Add import: `"errors"` to the import block at the top of `cmd/bidder/main.go`.

- [ ] **Step 2.5: Run test to verify it passes**

```bash
go test ./cmd/bidder/ -run TestHandleBid_RejectsOversizedBody -v
```
Expected: PASS

- [ ] **Step 2.6: Run full bidder unit tests to check no regression**

```bash
go test ./cmd/bidder/ -v -short
```
Expected: all existing tests PASS (adding `MaxBytesReader` only changes oversized behavior; normal-size bodies decode identically).

- [ ] **Step 2.7: Commit fix**

```bash
git add cmd/bidder/main.go
git commit -m "fix(bidder): enforce 1MB body limit on direct /bid [closes F4]

Exchange path at handleExchangeBid already wraps r.Body in
io.LimitReader(..., 1<<20). Direct /bid had no limit, so public
clients could POST arbitrary-size bodies and stream them through
json.Decode into memory.

Using http.MaxBytesReader (not io.LimitReader) so oversized requests
return 413 Request Entity Too Large rather than silently decoding a
truncated prefix."
```

### Phase 1 Boundary Gate

Before opening the Phase 1 PR, run the full per-Phase review+test cycle until a zero-issue round:

- [ ] **Step P1.G.1:** `superpowers:requesting-code-review` on the 4 Phase 1 commits
- [ ] **Step P1.G.2:** `gstack /review` (Codex review enabled per Phase-boundary policy)
- [ ] **Step P1.G.3:** Fix any findings; if any changes made, re-dispatch review
- [ ] **Step P1.G.4:** `go test ./... -short` — all green
- [ ] **Step P1.G.5:** `go test -tags=e2e ./internal/handler/ -v` — all green
- [ ] **Step P1.G.6:** `bash scripts/qa/run.sh` — all green
- [ ] **Step P1.G.7:** No frontend changes → skip `/qa`, `/browse`
- [ ] **Step P1.G.8:** If any fix was applied at any step, restart from P1.G.1. Only proceed when a full loop had zero issues.

---

## Phase 2 — Bid response decorator + win URL metadata (F1 + F5)

One PR, 4 commits. Introduces `decorateBidResponse()` shared between direct and exchange paths, extends HMAC token to cover `(campaign_id, request_id, creative_id, bid_price_cents)`, updates `handleWin` to read real values from URL.

**Blast radius:** Every `auth.GenerateToken` and `auth.ValidateToken` call site needs updating in lockstep. 10 locations total:
- `cmd/bidder/main.go:307` (direct bid) — handled by decorator
- `cmd/bidder/main.go:385` (exchange bid) — handled by decorator
- `cmd/bidder/main.go` (handleWin validation — find with grep)
- `cmd/bidder/main.go` (handleClick validation — find with grep)
- `cmd/bidder/main_test.go:111, 392, 424, 459` (4 test token generators)
- `cmd/bidder/handlers_integration_test.go:123, 136, 146` (3 buildXxxURL helpers)
- `internal/auth/hmac_test.go` (token unit tests)

### Task 3 — Extract shared `decorateBidResponse`

This is a REFACTOR commit (no behavior change), paired with the next task's failing tests. Because the refactor itself has no user-visible behavior change, there's no RED test for this commit; the Task 4 RED test covers the combined behavior change.

**Files:**
- Create: `cmd/bidder/decorator.go`
- Modify: `cmd/bidder/main.go:293-319` (direct path) and `cmd/bidder/main.go:373-392` (exchange path)

- [ ] **Step 3.1: Create `cmd/bidder/decorator.go`**

```go
package main

import (
	"fmt"
	"math"
	"strconv"

	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/prebid/openrtb/v20/openrtb2"
)

// decorateBidResponse populates NURL (win notice with HMAC) and injects the
// click tracker on the bid's AdM. Shared by direct /bid and /bid/{exchange_id}
// so both paths produce identical win/click wiring.
//
// The NURL and click URL carry creative_id and bid_price_cents as signed
// params so handleWin/handleClick can record the true bid-time values
// without recomputing from current campaign state (which may have shifted
// between bid and win).
//
// Pre-condition: resp.SeatBid[0].Bid[0].CrID is set to the real selected
// creative ID by Engine.Bid (see internal/bidder/engine.go CrID assignment).
func decorateBidResponse(resp *openrtb2.BidResponse, req *openrtb2.BidRequest, baseURL, hmacSecret string) {
	if resp == nil || len(resp.SeatBid) == 0 || len(resp.SeatBid[0].Bid) == 0 {
		return
	}
	bid := &resp.SeatBid[0].Bid[0]

	var geo, osName string
	if req.Device != nil {
		osName = req.Device.OS
		if req.Device.Geo != nil {
			geo = req.Device.Geo.Country
		}
	}

	creativeID := bid.CrID
	// Use math.Round, not int64() truncation (CEO Finding #4).
	// bid.Price is in CPM dollars; e.g. $0.00495 * 100 = 0.495
	// truncated to 0 cents, rounded to 0 cents. But $0.01495 * 100
	// = 1.495 → truncation=1, round=1; $0.0205 * 100 = 2.05
	// → truncation=2 (wrong, should be 2); $0.00995 * 100 = 0.995
	// → truncation=0 (wrong), round=1. Truncation systematically
	// under-counts pennies.
	bidPriceCents := strconv.FormatInt(int64(math.Round(bid.Price*100)), 10)

	token := auth.GenerateToken(hmacSecret, bid.CID, req.ID, creativeID, bidPriceCents)

	bid.NURL = fmt.Sprintf(
		"%s/win?campaign_id=%s&price=${AUCTION_PRICE}&request_id=%s&creative_id=%s&bid_price_cents=%s&geo=%s&os=%s&token=%s",
		baseURL, bid.CID, req.ID, creativeID, bidPriceCents, geo, osName, token,
	)
	clickURL := fmt.Sprintf(
		"%s/click?campaign_id=%s&request_id=%s&creative_id=%s&token=%s",
		baseURL, bid.CID, req.ID, creativeID, token,
	)
	bid.AdM = injectClickTracker(bid.AdM, clickURL)
}
```

- [ ] **Step 3.2: Replace direct /bid decoration site**

Edit `cmd/bidder/main.go:293-319`, replacing the block starting at `if len(resp.SeatBid) > 0 && len(resp.SeatBid[0].Bid) > 0 {` with:

```go
	decorateBidResponse(resp, &req, d.PublicURL, d.HMACSecret)
	if len(resp.SeatBid) > 0 && len(resp.SeatBid[0].Bid) > 0 {
		bid := resp.SeatBid[0].Bid[0]
		log.Printf("[BID] request_id=%s campaign=%s bid=%.6f latency=%s",
			req.ID, bid.CID, bid.Price, latency)
	}
```

- [ ] **Step 3.3: Replace exchange /bid decoration site**

Edit `cmd/bidder/main.go:373-392`, replacing the block with:

```go
	decorateBidResponse(resp, req, d.PublicURL, d.HMACSecret)
	if len(resp.SeatBid) > 0 && len(resp.SeatBid[0].Bid) > 0 {
		bid := resp.SeatBid[0].Bid[0]
		log.Printf("[BID] exchange=%s request_id=%s campaign=%s bid=%.6f latency=%s",
			exchangeID, req.ID, bid.CID, bid.Price, latency)
	}
```

- [ ] **Step 3.4: Fix all GenerateToken test callers (signature now takes 5 params)**

Update every caller to pass `campIDStr, reqID, creativeID, bidPriceCents` (use empty string "" for creativeID and "0" for bidPriceCents in existing tests that don't care about those fields — they should still produce valid tokens for /click and /win tests that don't set those URL params).

In `cmd/bidder/handlers_integration_test.go`, update the 3 helpers:

`buildWinURL` (line 121-132):
```go
func (f *handlerFixture) buildWinURL(campaignID int64, requestID string, price float64, geo, osName string) string {
	campIDStr := fmt.Sprintf("%d", campaignID)
	// Match the decorator's token signing: (campaignID, requestID, creativeID, bidPriceCents).
	// For existing tests that don't carry creative metadata, sign with empty strings
	// so ValidateToken can be called with the same params from handleWin's test URL.
	token := auth.GenerateToken(qaHMACSecret, campIDStr, requestID, "", "")
	q := url.Values{}
	q.Set("campaign_id", campIDStr)
	q.Set("price", fmt.Sprintf("%f", price))
	q.Set("request_id", requestID)
	q.Set("creative_id", "")
	q.Set("bid_price_cents", "")
	q.Set("geo", geo)
	q.Set("os", osName)
	q.Set("token", token)
	return f.srv.URL + "/win?" + q.Encode()
}
```

Apply same pattern to `buildClickURL` and `buildConvertURL`.

In `cmd/bidder/main_test.go` at lines 111, 392, 424, 459: change each
`token := auth.GenerateToken(hmacSecret, campIDStr, reqID)` to
`token := auth.GenerateToken(hmacSecret, campIDStr, reqID, "", "")`.

In `internal/auth/hmac_test.go`: update token assertions if they compare exact strings (they may not — grep to confirm).

In `docs/archive/review-remediation-v5/REVIEW_REMEDIATION_V5_1_HOTFIX_PLAN.md:1169` — this is archive documentation, leave alone.

- [ ] **Step 3.5: Run full build + tests to verify refactor is behavior-preserving**

```bash
go build ./...
go test ./cmd/bidder/ -v -short
go test ./internal/auth/ -v
```
Expected: all PASS (no behavior change yet; token includes two extra empty-string params but URL shape identical for unchanged callers).

- [ ] **Step 3.6: Commit refactor**

```bash
git add cmd/bidder/decorator.go cmd/bidder/main.go cmd/bidder/main_test.go cmd/bidder/handlers_integration_test.go internal/auth/hmac_test.go
git commit -m "refactor(bidder): extract decorateBidResponse shared by direct+exchange bid

Both handleBid and handleExchangeBid now delegate win/click URL
construction to decorateBidResponse. Token signature extended to
(campaignID, requestID, creativeID, bidPriceCents) — for legacy
call sites, creativeID and bidPriceCents are signed as empty strings,
preserving URL shape. Behavior unchanged; F1 + F5 bug fixes land in
the next commit."
```

### Task 4 — F1: Exchange /bid click tracker + F5: handleWin reads real bid-time values

This is the combined fix commit. Two failing tests land BEFORE this fix as per TDD Evidence Rule.

**Files:**
- Test: `cmd/bidder/handlers_integration_test.go` (append 2 new tests)
- Modify: `cmd/bidder/main.go` `handleWin` (line ~514-540)

- [ ] **Step 4.1: Write failing test for F1 — exchange bid click tracker**

Append to `cmd/bidder/handlers_integration_test.go`:

```go
// TestExchangeBid_InjectsClickTracker verifies the /bid/{exchange_id} path
// decorates the bid response with a click tracker URL in AdM, matching the
// direct /bid path. Pre-fix: exchange path only set NURL; AdM was untouched
// and /click was never triggered on exchange traffic, breaking CPC billing.
// Post-fix (Task 4): decorateBidResponse runs on both paths; AdM contains
// a click URL with campaign_id, request_id, creative_id, and token.
func TestExchangeBid_InjectsClickTracker(t *testing.T) {
	f := newHandlerFixture(t)
	f.SeedCampaign(t, f.campaignID, f.creativeID, billingModelCPC, "<div>ad</div>")
	f.Start(t)

	// Minimal OpenRTB BidRequest as a generic exchange would pass through
	// the adapter. Use the "generic" adapter that's registered by default.
	body := []byte(`{"id":"test-req-1","imp":[{"id":"1","banner":{"w":300,"h":250}}],"site":{"id":"s","domain":"example.com"},"device":{"os":"iOS","geo":{"country":"US"}}}`)
	resp, err := http.Post(f.srv.URL+"/bid/generic", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /bid/generic: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, readBody(t, resp))
	}

	var bidResp openrtb2.BidResponse
	if err := json.NewDecoder(resp.Body).Decode(&bidResp); err != nil {
		t.Fatalf("decode bid response: %v", err)
	}
	if len(bidResp.SeatBid) == 0 || len(bidResp.SeatBid[0].Bid) == 0 {
		t.Fatalf("expected at least one bid in response")
	}
	adm := bidResp.SeatBid[0].Bid[0].AdM
	if !strings.Contains(adm, "/click?") {
		t.Errorf("expected AdM to contain /click? tracker URL; got: %s", adm)
	}
	if !strings.Contains(adm, "campaign_id=") {
		t.Errorf("expected AdM click URL to carry campaign_id; got: %s", adm)
	}
	if !strings.Contains(adm, "token=") {
		t.Errorf("expected AdM click URL to carry HMAC token; got: %s", adm)
	}
}
```

Add imports if missing: `"bytes"`, `"encoding/json"`, `"strings"`, and `openrtb2 "github.com/prebid/openrtb/v20/openrtb2"`.

Note: `SeedCampaign` signature and `billingModelCPC` constant — confirm by reading the test fixture file; if they differ, use the same conventions as existing `handlers_integration_test.go` scenarios (e.g., scenario 22 win CPM happy path uses `f.SeedCampaign(t, id, crID, ...)`).

- [ ] **Step 4.2: Write failing test for F5 — handleWin reads real creative_id and bid_price_cents from URL**

Append to `cmd/bidder/handlers_integration_test.go`:

```go
// TestHandleWin_UsesCreativeIDAndBidPriceFromURL verifies /win emits a Kafka
// event carrying the creative_id and bid_price_cents from the URL, NOT
// recomputed from current campaign state. Pre-fix: handleWin used
// c.Creatives[0].ID (which skips non-zero-index creatives) and
// EffectiveBidCPMCents(0,0) (which ignores runtime CTR/CVR adjustments)
// so multi-creative campaigns or strategy shifts produced wrong bid_log rows.
// Post-fix: handleWin parses creative_id and bid_price_cents from URL query
// and passes them through to the event unmodified.
func TestHandleWin_UsesCreativeIDAndBidPriceFromURL(t *testing.T) {
	f := newHandlerFixture(t)
	f.SeedCampaign(t, f.campaignID, f.creativeID, billingModelCPM, "<div>ad</div>")
	f.Start(t)

	reqID := "win-url-metadata-test"
	truthfulCreativeID := "42"         // NOT c.Creatives[0].ID
	truthfulBidPriceCents := int64(250) // NOT EffectiveBidCPMCents(0,0)

	campIDStr := fmt.Sprintf("%d", f.campaignID)
	token := auth.GenerateToken(qaHMACSecret, campIDStr, reqID, truthfulCreativeID, fmt.Sprintf("%d", truthfulBidPriceCents))

	q := url.Values{}
	q.Set("campaign_id", campIDStr)
	q.Set("price", "0.00150")
	q.Set("request_id", reqID)
	q.Set("creative_id", truthfulCreativeID)
	q.Set("bid_price_cents", fmt.Sprintf("%d", truthfulBidPriceCents))
	q.Set("geo", "US")
	q.Set("os", "iOS")
	q.Set("token", token)

	resp, err := http.Get(f.srv.URL + "/win?" + q.Encode())
	if err != nil {
		t.Fatalf("GET /win: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/win: expected 200, got %d: %s", resp.StatusCode, readBody(t, resp))
	}

	// Drain the producer's inflight goroutines so the event is observable.
	f.producer.Flush()

	// Assert the captured win event has the truthful values.
	evt, ok := f.producer.LastEventOfType(t, "win")
	if !ok {
		t.Fatalf("no win event produced")
	}
	if fmt.Sprintf("%d", evt.CreativeID) != truthfulCreativeID {
		t.Errorf("expected creative_id=%s, got %d", truthfulCreativeID, evt.CreativeID)
	}
	expectedBidPrice := float64(truthfulBidPriceCents) / 100.0 / 1000.0 // cents → CPM dollars
	if evt.BidPrice != expectedBidPrice {
		t.Errorf("expected bid_price=%.6f, got %.6f", expectedBidPrice, evt.BidPrice)
	}
}
```

Note: `f.producer.LastEventOfType` and `Flush()` may not exist in the fixture — if not, inspect how scenario 22 asserts on captured events and adapt. The pattern is "fixture wraps a fake producer that buffers events for assertion".

- [ ] **Step 4.3: Run tests to verify both fail**

```bash
go test ./cmd/bidder/ -run "TestExchangeBid_InjectsClickTracker|TestHandleWin_UsesCreativeIDAndBidPriceFromURL" -v
```
Expected: both FAIL — Exchange test fails on "expected AdM to contain /click?" (pre-fix exchange path doesn't decorate AdM); handleWin test fails on creative_id mismatch (pre-fix recomputes from campaign state, returning "1" not "42").

- [ ] **Step 4.4: Commit failing tests**

```bash
git add cmd/bidder/handlers_integration_test.go
git commit -m "test(bidder): add failing tests for exchange click tracker + win URL metadata

Expect-Fail: TestExchangeBid_InjectsClickTracker
Expect-Fail: TestHandleWin_UsesCreativeIDAndBidPriceFromURL"
```

- [ ] **Step 4.5: Implement F5 — parse URL params in handleWin**

Edit `cmd/bidder/main.go` `handleWin`. Locate the block (around line 500-540 based on current state) that constructs the `evt` and recomputes `creativeID` and `bidPrice`. Replace:

```go
		var bidPrice float64
		var advertiserCharge float64
		if c != nil {
			bidPrice = float64(c.EffectiveBidCPMCents(0, 0)) * 0.90 / 100.0 / 1000.0
			if isCPC {
				advertiserCharge = 0
			} else {
				advertiserCharge = price / (1 - PlatformMargin)
			}
		}
		var creativeID, advertiserID int64
		if c != nil {
			advertiserID = c.AdvertiserID
			if len(c.Creatives) > 0 {
				creativeID = c.Creatives[0].ID
			}
		}
```

With:
```go
		// Read truthful bid-time values from URL (signed by decorateBidResponse).
		// Falls back to recomputed values if URL did not carry them (pre-extension
		// tokens from old flights; emit warn log when this happens).
		urlCreativeID, _ := strconv.ParseInt(r.URL.Query().Get("creative_id"), 10, 64)
		urlBidPriceCents, _ := strconv.ParseInt(r.URL.Query().Get("bid_price_cents"), 10, 64)

		var bidPrice float64
		var advertiserCharge float64
		if urlBidPriceCents > 0 {
			bidPrice = float64(urlBidPriceCents) / 100.0 / 1000.0
		} else if c != nil {
			log.Printf("[WIN] fallback bid_price recompute campaign=%d request_id=%s (URL missing bid_price_cents)", campaignID, requestID)
			bidPrice = float64(c.EffectiveBidCPMCents(0, 0)) * 0.90 / 100.0 / 1000.0
		}
		if c != nil {
			if isCPC {
				advertiserCharge = 0
			} else {
				advertiserCharge = price / (1 - PlatformMargin)
			}
		}

		var creativeID, advertiserID int64
		if c != nil {
			advertiserID = c.AdvertiserID
		}
		if urlCreativeID > 0 {
			creativeID = urlCreativeID
		} else if c != nil && len(c.Creatives) > 0 {
			log.Printf("[WIN] fallback creative_id lookup campaign=%d request_id=%s (URL missing creative_id)", campaignID, requestID)
			creativeID = c.Creatives[0].ID
		}
```

Extend the `auth.ValidateToken` call in `handleWin` with **transitional validation** (CEO Finding #3) — try 6-param first, fall back to 4-param legacy during deploy window so old tokens issued by the pre-deploy binary still validate. Search for `ValidateToken` in `cmd/bidder/main.go` handleWin block; the current call should be:

```go
if !auth.ValidateToken(d.HMACSecret, token, campaignIDStr, requestID) {
    // reject
}
```

Change to:
```go
urlCrIDStr := r.URL.Query().Get("creative_id")
urlPriceCentsStr := r.URL.Query().Get("bid_price_cents")

// New-format token: signed over (campID, requestID, creativeID, bidPriceCents).
// Legacy-format token (pre-deploy): signed over (campID, requestID) only.
// Accept either to ride through the deploy window without 403 spikes.
// After one deploy window (~10min past token TTL), a follow-up PR should
// remove the legacy branch.
validNew := auth.ValidateToken(d.HMACSecret, token, campaignIDStr, requestID, urlCrIDStr, urlPriceCentsStr)
validLegacy := !validNew && auth.ValidateToken(d.HMACSecret, token, campaignIDStr, requestID)

if !validNew && !validLegacy {
    // reject with 403
}
if validLegacy {
    observability.BidderTokenLegacyAccepted.WithLabelValues("win").Inc()
    log.Printf("[WIN] legacy token accepted during deploy transition: request_id=%s", requestID)
    // Force the fallback recompute path: legacy tokens cannot have trusted
    // URL metadata (they were issued before creative_id/bid_price_cents
    // were added to NURL). Clear the URL params so the `urlCreativeID > 0`
    // and `urlBidPriceCents > 0` branches fall to the recompute path.
    urlCrIDStr = ""
    urlPriceCentsStr = ""
}
```

Then in the block that parses URL metadata, use the (possibly zeroed) `urlCrIDStr` and `urlPriceCentsStr` strings:

```go
urlCreativeID, _ := strconv.ParseInt(urlCrIDStr, 10, 64)
urlBidPriceCents, _ := strconv.ParseInt(urlPriceCentsStr, 10, 64)
```

Replace the `r.URL.Query().Get("creative_id")` / `...("bid_price_cents")` parse lines in the block above with these variable reads instead.

Apply the same transitional validation block to `handleClick` — `click_url` carries `creative_id` per decorator. Copy the same pattern with `"click"` as the metric label.

Register the new counter in `internal/observability/metrics.go`:
```go
// BidderTokenLegacyAccepted counts win/click token validations that fell
// back to the legacy 4-param HMAC signature during a deploy transition.
// Expected to spike briefly after a Phase 2 deploy, then return to zero
// within the 5-minute token TTL. Sustained non-zero = stuck legacy path,
// remove the legacy branch or investigate.
var BidderTokenLegacyAccepted = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "bidder_token_legacy_accepted_total",
		Help: "Count of HMAC token validations accepted via legacy 4-param signature during deploy transition.",
	},
	[]string{"handler"}, // "win" | "click"
)
```

- [ ] **Step 4.6: Run F5 test to verify it passes**

```bash
go test ./cmd/bidder/ -run TestHandleWin_UsesCreativeIDAndBidPriceFromURL -v
```
Expected: PASS

- [ ] **Step 4.7: Verify F1 test now also passes**

The F1 fix was accomplished by Task 3 (refactor wired decorator into exchange path). Re-run:
```bash
go test ./cmd/bidder/ -run TestExchangeBid_InjectsClickTracker -v
```
Expected: PASS.

If FAIL, debug: inspect that `handleExchangeBid` actually calls `decorateBidResponse`. (This is a good safety net — the refactor didn't include a failing test, so this is where we catch if Task 3 missed a site.)

- [ ] **Step 4.8: Run full bidder suite**

```bash
go test ./cmd/bidder/ -v
go test ./internal/auth/ -v
```
Expected: all PASS.

- [ ] **Step 4.9: Commit fix**

```bash
git add cmd/bidder/main.go
git commit -m "fix(bidder): inject click tracker on exchange + use URL metadata in handleWin [closes F1, F5]

F1: handleExchangeBid now runs decorateBidResponse, which injects the
click tracker URL into AdM — matching direct /bid behavior. CPC
billing on exchange traffic was previously broken because /click was
never triggered without a tracker URL in AdM.

F5: handleWin parses creative_id and bid_price_cents from the URL
query (signed by HMAC token) instead of recomputing from current
campaign state. Fixes bid_log rows that reported the wrong creative
for multi-creative campaigns, or wrong bid_price after bid strategy
shifts between bid and win. Falls back to the old recompute path
with a warn log if the URL lacks the metadata, for in-flight tokens
issued before this deploy."
```

### Phase 2 Boundary Gate

- [ ] **Step P2.G.1-8:** Same structure as Phase 1 boundary (see Step P1.G.1-P1.G.8). Phase 2 has an HMAC token signature change — pay specific attention in `/review` and Codex to whether all 10 token caller sites were updated and whether the fallback path in `handleWin` is safe during deploy.

---

## Phase 3 — Activation contract + pub/sub metric (F3)

One PR, 3 commits. This IS Phase Final for the branch — `/cso` runs here.

### Task 5 — F3: `NotifyCampaignUpdate` returns error + metric counter

**Files:**
- Modify: `internal/bidder/loader.go:343-351` (`NotifyCampaignUpdate` signature)
- Modify: `internal/observability/metrics.go` (new counter)
- Modify: `internal/handler/campaign.go` (3 sites: HandleStartCampaign, HandlePauseCampaign, HandleUpdateCampaign)
- Test: append to `internal/handler/e2e_public_campaign_test.go`

- [ ] **Step 5.1: Write failing test**

Append to `e2e_public_campaign_test.go`:

```go
// TestCampaign_StartRecordsMetricOnPubSubFailure verifies that when Redis
// pub/sub fails during activation, the metric
// campaign_activation_pubsub_failures_total{action="activated"} increments
// while /start still returns 200 (eventual-consistency contract per F3-B).
//
// Force failure by pointing Deps.Redis at a closed client. The DB
// TransitionStatus still succeeds (Postgres is healthy), so the handler
// should return 200 with a metric bump.
func TestCampaign_StartRecordsMetricOnPubSubFailure(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	_ = newCreative(t, d, campaignID)

	// Capture pre-call metric value
	before := observability.CampaignActivationPubSubFailures.WithLabelValues("activated")
	beforeVal := testutil.ToFloat64(before)

	// Replace Redis with a closed client to force Publish() errors.
	d.Redis.Close()

	req := authedReq(t, http.MethodPost,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignID, 10)+"/start", nil, apiKey)
	w := execAuthed(t, d, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (eventual-consistency contract), got %d: %s",
			w.Code, w.Body.String())
	}

	afterVal := testutil.ToFloat64(before)
	if afterVal != beforeVal+1 {
		t.Fatalf("expected pubsub_failures{action=activated} to increment from %v to %v, got %v",
			beforeVal, beforeVal+1, afterVal)
	}
}
```

Add imports: `"github.com/prometheus/client_golang/prometheus/testutil"` and `"github.com/heartgryphon/dsp/internal/observability"`.

- [ ] **Step 5.2: Run test — verify it fails**

```bash
go test -tags=e2e ./internal/handler/ -run TestCampaign_StartRecordsMetricOnPubSubFailure -v
```
Expected: FAIL — `observability.CampaignActivationPubSubFailures` symbol does not exist (compile error), OR if someone has added it already, the counter doesn't increment.

Compile error is acceptable as RED for THIS specific test because the symbol is the feature being added. (Per TDD Evidence Rule: failure reason must be "功能缺失" not "拼写错误" — a missing symbol that maps exactly to the feature is "功能缺失", legitimate RED.)

- [ ] **Step 5.3: Commit failing test**

```bash
git add internal/handler/e2e_public_campaign_test.go
git commit -m "test(handler): add failing test for campaign_activation_pubsub_failures metric

Expect-Fail: TestCampaign_StartRecordsMetricOnPubSubFailure (symbol missing)"
```

- [ ] **Step 5.4: Add the counter to observability/metrics.go**

In `internal/observability/metrics.go`, near the other campaign-related counters, add:

```go
// CampaignActivationPubSubFailures counts pub/sub delivery failures during
// campaign start/pause/update. Pub/sub delivery is best-effort; on failure
// the bidder's periodic 30s loader refresh catches up. High rates here
// correlate with longer user-visible eventual-consistency lag.
var CampaignActivationPubSubFailures = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "campaign_activation_pubsub_failures_total",
		Help: "Count of Redis pub/sub publish failures during campaign activation/pause/update notifications.",
	},
	[]string{"action"}, // "activated" | "paused" | "updated" | "removed"
)
```

Confirm imports (`promauto`, `prometheus`) are present in the file.

- [ ] **Step 5.5: Change `NotifyCampaignUpdate` to return error**

Edit `internal/bidder/loader.go:343-351`:

```go
// NotifyCampaignUpdate publishes a campaign-change message to the
// campaign:updates Redis channel. Returns the Publish error so callers
// can record a metric / logs on failure. Callers should NOT fail the
// overall request on pub/sub error — the loader's periodic refresh
// (~30s) catches up as an eventual-consistency fallback.
func NotifyCampaignUpdate(ctx context.Context, rdb *redis.Client, campaignID int64, action string) error {
	payload, _ := json.Marshal(map[string]any{
		"campaign_id": campaignID,
		"action":      action,
	})
	if err := rdb.Publish(ctx, "campaign:updates", payload).Err(); err != nil {
		log.Printf("[NOTIFY] pub/sub error: %v", err)
		return err
	}
	return nil
}
```

- [ ] **Step 5.6: Update all NotifyCampaignUpdate callers to record the metric**

Find callers: `grep -rn NotifyCampaignUpdate internal/ cmd/`. Expected sites: `HandleStartCampaign`, `HandlePauseCampaign`, `HandleUpdateCampaign`, plus any remove/delete paths.

Pattern for each caller (example for Start):
```go
	if d.Redis != nil {
		if err := bidder.NotifyCampaignUpdate(r.Context(), d.Redis, id, "activated"); err != nil {
			observability.CampaignActivationPubSubFailures.WithLabelValues("activated").Inc()
			// Do NOT return — per F3 contract, eventual-consistency applies.
		}
	}
```

Apply the same wrap at each call site with the corresponding action label.

- [ ] **Step 5.7: Run test — verify GREEN**

```bash
go test -tags=e2e ./internal/handler/ -run TestCampaign_StartRecordsMetricOnPubSubFailure -v
```
Expected: PASS.

- [ ] **Step 5.8: Run broader tests to check no regression**

```bash
go test ./... -short
go test -tags=e2e ./internal/handler/ -v
```
Expected: all PASS.

- [ ] **Step 5.9: Commit fix**

```bash
git add internal/observability/metrics.go internal/bidder/loader.go internal/handler/campaign.go
git commit -m "feat(bidder,handler): record campaign_activation_pubsub_failures_total metric [F3]

NotifyCampaignUpdate now returns the Redis Publish error; handler
sites record campaign_activation_pubsub_failures_total{action=...}
on failure without aborting activation. Per F3 decision (option B),
the /start contract remains eventually-consistent — the loader's
30s refresh catches up if pub/sub is down. The new metric surfaces
activation-delivery degradation to dashboards."
```

### Task 6 — F3: Document 30s activation eventual-consistency contract

**Files:**
- Modify: `docs/contracts/*.md` (find the campaigns contract file) OR `docs/OVERVIEW.md`

- [ ] **Step 6.1: Find the existing contract doc**

```bash
ls docs/contracts/ 2>/dev/null || echo "(no contracts dir — put note in OVERVIEW)"
grep -rn "campaigns/{id}/start\|POST /campaigns" docs/ --include="*.md" | head
```

Decide which file owns the `/start` contract: `docs/contracts/campaigns.md` if exists, otherwise append to `docs/OVERVIEW.md` under a "Campaign activation contract" section.

- [ ] **Step 6.2: Add the contract section**

Append:

```markdown
### Campaign activation eventual-consistency contract

`POST /api/v1/campaigns/{id}/start` returns 200 as soon as the campaign
row in Postgres is transitioned to `active`. The bidder sees the campaign
via two paths:

1. **Pub/sub fast path:** `campaign:updates` message reaches the bidder's
   `CampaignLoader.listenPubSub`, the loader re-queries Postgres and
   updates its in-memory map. Typical latency: sub-second.
2. **Periodic fallback:** If pub/sub delivery fails (Redis outage, network
   blip), the bidder's `CampaignLoader.periodicRefresh` re-reads the full
   active set every 30s and catches up.

**SLA:** Clients MUST NOT assume that a 200 on `/start` means the bidder
is immediately delivering bids for the campaign. The contract is that
the bidder WILL be serving the campaign within **30 seconds** of the 200
response, assuming Postgres and the bidder process are healthy.

**Monitoring:** `campaign_activation_pubsub_failures_total{action}`
increments on pub/sub delivery failure. A sustained non-zero rate means
users are hitting the 30s fallback path. Same applies to pause and update.
```

- [ ] **Step 6.3: Commit**

```bash
git add docs/contracts/campaigns.md  # or docs/OVERVIEW.md
git commit -m "docs(api): document 30s activation eventual-consistency contract [F3]

Clarifies that POST /campaigns/{id}/start returns 200 on DB commit,
with the bidder catching up via pub/sub (fast path) or 30s periodic
refresh (fallback). Establishes the 30s SLA and points to the
campaign_activation_pubsub_failures_total metric for monitoring."
```

### Phase Final Gate (applies to this PR since it's the last)

- [ ] **Step P3.G.1:** `superpowers:requesting-code-review` — full batch (Phase 1 + 2 + 3 together, since this is the branch terminus)
- [ ] **Step P3.G.2:** `gstack /review` + `gstack /codex` with BOTH review and challenge modes
- [ ] **Step P3.G.3:** Fix all findings; if any fix, restart from P3.G.1
- [ ] **Step P3.G.4:** `go test ./... -short`
- [ ] **Step P3.G.5:** `go test -tags=e2e ./internal/handler/ -v`
- [ ] **Step P3.G.6:** `bash scripts/qa/run.sh`
- [ ] **Step P3.G.7:** `gstack /cso` — **mandatory** for this batch because:
  - F2 touches the money path (balance check gate)
  - F5 changes HMAC token semantics (new param positions — audit for token reuse / replay windows / param ordering attacks)
- [ ] **Step P3.G.8:** No frontend → skip `/browse`, `/design-review`, `/qa`
- [ ] **Step P3.G.9:** Full round must be zero-issue. If any fix → restart from P3.G.1.

### Task 7 — Ship

- [ ] **Step 7.1:** `superpowers:finishing-a-development-branch` to decide merge strategy (expect: PR with Rebase-and-merge or Create-merge-commit, NOT Squash — TDD Evidence Rule requires test commits to stay visible)
- [ ] **Step 7.2:** `gstack /ship` — creates PR
- [ ] **Step 7.3:** User approval on PR
- [ ] **Step 7.4:** `gstack /land-and-deploy` — merge + deploy + health check
- [ ] **Step 7.5:** `gstack /canary` — post-deploy monitoring for bid error rate, /start latency/error, new metric emission

---

## Risk Register

- **HMAC token rollover at deploy (Phase 2) — MITIGATED**: The transitional validation in Step 4.5 accepts both new (6-param) and legacy (4-param) tokens for one deploy window. `bidder_token_legacy_accepted_total{handler}` metric monitors the transition. Sustained non-zero reading >15 min after deploy = stuck, investigate. A follow-up PR should remove the legacy branch ≥10 min after token TTL expires (token TTL = 5 min per `auth.hmac.go`).
- **F2 test strategy — RESOLVED via Task 0**: Task 0 introduces `BillingService` interface so tests inject stubs. No schema dependency, no FK cascade risk.
- **ValidateToken backward compat**: `auth.ValidateToken` is variadic on params so adding params at call sites is safe as long as BOTH sides (generator and validator) change together. Any missed site = silent 403 on /win or /click. Phase 2 boundary gate MUST grep `GenerateToken\|ValidateToken` to verify every site updated.
- **`BillingSvc` prod wiring (CEO Finding #2 residual)**: The handler layer now 503s on nil BillingSvc, but prod should never hit this branch. Task 0 adds a startup assert in `cmd/api/main.go` — if that fails, the server crashes on boot instead of launching a bypass server. Handler 503 is belt-and-braces.
- **bid_price_cents precision (CEO Finding #4 residual)**: `math.Round` in the decorator rounds half-away-from-zero. For values ≥ $0.005 it's accurate to a cent. Sub-cent bids (< $0.005) round to 0 cents — acceptable because OpenRTB prices are CPM and sub-cent CPM is vanishingly rare in practice. If this changes, revisit.

## Self-Review (performed, incl. CEO review updates)

- **Spec coverage:** F1, F2, F3, F4, F5 + CEO Findings #1-#4 — each has at least one task.
  - CEO #1 → Task 0 (BillingService interface)
  - CEO #2 → Task 0 (startup assert) + Task 1 (handler 503 on nil) + 2nd test in Step 1.1
  - CEO #3 → Task 4 (transitional ValidateToken) + new metric
  - CEO #4 → Task 3 decorator (math.Round)
- **Placeholder scan:** no "TBD/TODO/implement later"; every step has concrete code or exact commands. ✓
- **Type consistency:** `decorateBidResponse` signature used identically in Task 3.1, 3.2, 3.3. `BillingService` interface matches `billing.Service.GetBalance` return tuple `(int64, string, error)`. `CampaignActivationPubSubFailures` / `BidderTokenLegacyAccepted` names consistent wherever referenced. `NotifyCampaignUpdate` returns `error` in Task 5.5 / 5.6 / test.
- **Gap flagged:** `TestHandleWin_UsesCreativeIDAndBidPriceFromURL` references `f.producer.LastEventOfType` which may not exist — explicitly noted in Step 4.2 with fallback guidance. Acceptable since the exact event-inspection API is a fixture detail.
- **CEO Finding #3 legacy-path F5 interaction:** The transitional validation explicitly clears `urlCrIDStr`/`urlPriceCentsStr` when legacy token is accepted, forcing the recompute fallback. This is correct: legacy tokens were issued by a binary that didn't put creative_id/bid_price_cents into the NURL in the first place, so those URL params (if present at all) are untrusted user input, not signed data. Without the clear, a tampered legacy-token request could inject arbitrary `creative_id` into bid_log.

## CEO Plan Review — Decisions Applied

| Finding | Severity | Decision (user picked) | Plan change |
|--------|----------|----------------------|-------------|
| #1 FK-test broken | CRITICAL | **1A**: abstract `BillingService` interface | New Task 0 in Phase 1 + rewritten Task 1 tests |
| #2 `nil BillingSvc` bypass | CONCERN | **2B**: nil → 503 in handler | Extra test case in Step 1.1 + code change in Step 1.4 + startup assert in Task 0.3 |
| #3 HMAC deploy rollover | CONCERN | **3A**: transitional validation | Step 4.5 try-6-then-4 block + `bidder_token_legacy_accepted_total` metric |
| #4 float→int cents truncation | MINOR | **4A**: `math.Round` | Task 3.1 `decorator.go` uses `math.Round(bid.Price*100)` |

## Next Step

Ready for `/plan-eng-review + /codex` (architecture challenge), then user approval, then implementation.
