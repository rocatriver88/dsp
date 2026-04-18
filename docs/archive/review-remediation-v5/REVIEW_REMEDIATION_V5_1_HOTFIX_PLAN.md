# V5.1 Security Hotfix — Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Each task must go through implementer → spec reviewer → code quality reviewer before marking complete. Inline fixes triggered by a reviewer require re-dispatching the same reviewer to verify. Phase boundary loop (end of this plan) is non-negotiable per CLAUDE.md.

**Goal:** Fix 3 P1 security findings from the 2026-04-15 independent Claude + Codex review, caught in the V5 integration audit blind spot (周边基础设施审查未覆盖 handler 层内部 ownership check 之外的攻击面).

**Architecture:**
- **P1-1** — Analytics SSE tenant API key leak via URL query: replace with short-lived HMAC-signed SSE token issued via a dedicated endpoint, routed through a new SSE token middleware that shares the same `*auth.Advertiser` context model as `APIKeyMiddleware`.
- **P1-2** — `HandleCreateAdvertiser` privilege escalation: move `POST /api/v1/advertisers` from the public mux to the admin mux (behind `AdminAuthMiddleware`). The legitimate tenant bootstrap path is `POST /api/v1/register` → admin approval → api_key delivery, which already exists. Update `cmd/autopilot/client.go` to use the new admin path.
- **P1-3** — `/click?dest=` open redirect: delete the `dest` query parameter handling entirely. Verified dead code: `cmd/bidder/main.go:276-277` constructs legitimate click URLs without `dest`, and `injectClickTracker` at line 633 only appends a 1x1 tracking pixel.

**Tech Stack:** Go 1.23 (backend), Next.js App Router (frontend), pgx/pgxpool (Postgres), HMAC-SHA256, `crypto/subtle.ConstantTimeCompare`

**Scope — NOT in this plan:**
- P2 findings (contract unification, observability, ApiKeyGate admin bug, etc.) — Phase 2A/2B/2C
- Refactoring `HandleCreateAdvertiser` to strip self-settable `balance_cents` — Phase 2C (hotfix only moves the route; admin now controls access)
- Replacing `internal/alert.Noop{}` with a real alert pipeline — Phase 2B
- Rate-limit Redis fail-open hardening — Phase 2C

---

## File Structure

**New files:**
- `internal/auth/sse_token.go` — `IssueSSEToken(secret []byte, advID int64, ttl time.Duration) string` and `ValidateSSEToken(secret []byte, token string, now time.Time) (int64, error)`
- `internal/auth/sse_token_test.go` — unit tests (table-driven; expired, forged, malformed, happy path, constant-time behavior)
- `internal/auth/sse_middleware.go` — `SSETokenMiddleware(secret []byte) func(http.Handler) http.Handler` — reads `?token=` query, validates, injects `*Advertiser{ID}` via same `advertiserKey` context key
- `internal/auth/sse_middleware_test.go` — middleware-level tests using `httptest`
- `test/integration/v5_1_hotfix_test.go` — integration tests for all 3 P1s against real Postgres + real `campaign.Store`

**Modified files (Go):**
- `internal/config/config.go` — add `APIHMACSecret string` field + env var `API_HMAC_SECRET` + validate distinct-from-dev-default in production
- `internal/handler/deps.go` (or wherever `Deps` struct lives; if in handler.go, use that) — add `HMACSecret []byte` field
- `cmd/api/main.go` — wire `cfg.APIHMACSecret` into `handler.Deps.HMACSecret`
- `internal/handler/analytics.go` — add `HandleAnalyticsStreamToken` handler (POST, reads advID from context, returns `{token, expires_at}`)
- `internal/handler/routes.go` — split analytics SSE routes into a separate sub-mux behind `SSETokenMiddleware`, keep `/api/v1/analytics/token` in main publicMux; move `POST /api/v1/advertisers` from `BuildPublicMux` to `BuildAdminMux`
- `internal/handler/middleware.go` — **delete** the `strings.HasPrefix(r.URL.Path, "/api/v1/analytics/")` + `api_key` query block from `WithAuthExemption` (lines 13-18)
- `internal/handler/campaign.go` — update `HandleCreateAdvertiser` godoc annotations to reflect admin-only access
- `cmd/bidder/main.go` — delete `dest` handling: line 499 (read), lines 516-518 (dedup branch redirect), lines 569-571 (happy-path redirect)
- `cmd/autopilot/client.go` — update `CreateAdvertiser` to use new admin path + admin token header + `adminURL` base (like `AdminApproveRegistration` pattern at line 372)

**Modified files (frontend):**
- `web/app/analytics/page.tsx` — replace direct `EventSource(".../analytics/stream?api_key=...")` with two-step flow: fetch token via POST `/api/v1/analytics/token` (X-API-Key header), then `EventSource(".../analytics/stream?token=...)`. Handle token-refresh-on-reconnect (5-min TTL).

**Test files created/modified:**
- `internal/auth/sse_token_test.go` — new
- `internal/auth/sse_middleware_test.go` — new
- `internal/handler/analytics_token_test.go` — new, covers the token issue endpoint
- `test/integration/v5_1_hotfix_test.go` — new, end-to-end against real Store
- `cmd/bidder/main_test.go` — add regression test for `/click?dest=` → NO redirect to arbitrary dest

---

## Token format (normative, for `internal/auth/sse_token.go`)

```
token := base64url(<advID>:<expUnix>) + "." + hex(hmac_sha256(secret, <advID>:<expUnix>))
```

- Payload: decimal `advertiserID`, colon, decimal unix seconds of expiry
- Signature: lowercase hex of HMAC-SHA256 over the payload (pre-base64url bytes)
- Separator: `.` between encoded payload and signature
- Validation: constant-time HMAC comparison via `crypto/subtle.ConstantTimeCompare`
- TTL: 5 minutes (`5 * time.Minute`)
- Clock skew tolerance: none in Phase 1 (keep it simple; 5 min is wide enough)

**Rationale:** Self-contained HMAC token avoids Redis dependency and matches the existing bidder pattern (`internal/auth/click_token.go` or similar — verify during implementation). Base64url the payload so `:` doesn't collide with HTTP parsing.

---

## Task 0: Context baseline + verify prerequisites

**Purpose:** Freeze the "before" state so reviewers can compare against it and confirm no unrelated drift.

- [ ] **Step 1: Run git status + diff to confirm clean working tree**

```bash
git status
git log --oneline -3
git branch --show-current
```

Expected:
- branch: `review-remediation-v5.1-hotfix`
- clean working tree
- HEAD at `e39a35d` (closeout-debt billing/balance/{id}) or a descendant

- [ ] **Step 2: Verify P1-3 dead-code observation (no code change)**

```bash
grep -n "dest" cmd/bidder/main.go
grep -n 'fmt.Sprintf.*click' cmd/bidder/main.go
```

Expected:
- Line 276-277 emits `clickURL := fmt.Sprintf("%s/click?campaign_id=%s&request_id=%s&token=%s", baseURL, bid.CID, req.ID, token)` — **no `dest` parameter**
- Line 499 reads `dest := r.URL.Query().Get("dest")` — this is the attack surface
- Line 633 `injectClickTracker` — only appends 1x1 pixel, never adds `dest`

If any legitimate caller DOES emit `dest`, STOP and escalate — the deletion is no longer safe.

- [ ] **Step 3: Verify `internal/auth` has no existing SSE token helpers**

```bash
grep -rn "SSEToken\|AnalyticsToken\|sse_token" internal/auth/
```

Expected: no matches (we are adding the first implementation).

- [ ] **Step 4: Run existing test suite to establish baseline**

```bash
go test ./... -count=1
```

Expected: PASS (per memory `project_v5_completed.md`, V5 tests pass as of 2026-04-15).

If any test fails before we touch code, STOP — baseline is not green.

- [ ] **Step 5: Commit baseline marker (empty doc commit)**

```bash
# No code change — just marks where the branch started for reviewers.
git log --oneline -1
```

No commit needed; this task is pure verification.

---

## Task 1: Add `APIHMACSecret` config + Deps wiring

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/handler/handler.go` (or `deps.go` — wherever `type Deps struct` is defined; grep to find)
- Modify: `cmd/api/main.go:127-137`
- Test: `internal/config/config_test.go` (if exists) or new file

- [ ] **Step 1: Grep for `type Deps struct`**

```bash
grep -rn "type Deps struct" internal/handler/
```

Record the file and line for Step 3.

- [ ] **Step 2: Write failing config test**

Add to `internal/config/config_test.go` (create file if missing):

```go
package config

import (
	"os"
	"testing"
)

func TestAPIHMACSecretLoadedFromEnv(t *testing.T) {
	os.Setenv("API_HMAC_SECRET", "test-api-hmac-secret-long-enough-12345678")
	defer os.Unsetenv("API_HMAC_SECRET")
	cfg := Load()
	if cfg.APIHMACSecret != "test-api-hmac-secret-long-enough-12345678" {
		t.Fatalf("expected API_HMAC_SECRET to be loaded into cfg.APIHMACSecret, got %q", cfg.APIHMACSecret)
	}
}

func TestAPIHMACSecretRejectedInProduction(t *testing.T) {
	os.Setenv("API_HMAC_SECRET", "dev-api-hmac-secret-change-in-production")
	os.Setenv("ENVIRONMENT", "production")
	defer os.Unsetenv("API_HMAC_SECRET")
	defer os.Unsetenv("ENVIRONMENT")
	cfg := Load()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Validate to reject dev default in production, got nil")
	}
}
```

- [ ] **Step 3: Run test — expect FAIL**

```bash
go test ./internal/config/ -run TestAPIHMACSecret -v
```

Expected: FAIL with compile error or undefined field `APIHMACSecret`.

- [ ] **Step 4: Add `APIHMACSecret` field + validation**

In `internal/config/config.go`:
1. Add constant near line 14 (next to `defaultBidderHMACSecret`):
   ```go
   defaultAPIHMACSecret = "dev-api-hmac-secret-change-in-production"
   ```
2. Add field to the `Config` struct near line 35 (next to `BidderHMACSecret`):
   ```go
   APIHMACSecret string
   ```
3. Add env load near line 64:
   ```go
   APIHMACSecret: getEnv("API_HMAC_SECRET", defaultAPIHMACSecret),
   ```
4. Add to `Validate()` near line 92 (mirror the `BidderHMACSecret` check):
   ```go
   if c.APIHMACSecret == defaultAPIHMACSecret {
       return fmt.Errorf("API_HMAC_SECRET must be set in production; refusing to start with the baked-in dev secret")
   }
   ```
5. Update the production-check doc comment near line 82 to mention `API_HMAC_SECRET` too.

- [ ] **Step 5: Run test — expect PASS**

```bash
go test ./internal/config/ -run TestAPIHMACSecret -v
```

Expected: both subtests PASS.

- [ ] **Step 6: Add `HMACSecret` to `handler.Deps`**

Edit the file from Step 1. Add after the existing field list (order consistency with `BidderHMACSecret` in bidder `Deps`):

```go
HMACSecret []byte
```

- [ ] **Step 7: Wire in `cmd/api/main.go`**

Edit lines 127-137 (the `&handler.Deps{...}` literal). Add:

```go
HMACSecret: []byte(cfg.APIHMACSecret),
```

- [ ] **Step 8: Run full build**

```bash
go build ./...
```

Expected: clean build.

- [ ] **Step 9: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go internal/handler/handler.go cmd/api/main.go
git commit -m "feat(config): add APIHMACSecret for SSE token signing (V5.1 hotfix prep)

Distinct from BidderHMACSecret on cryptographic-hygiene grounds: analytics
SSE tokens and bidder click tokens are different trust domains and must
not share a signing key. Validate() refuses the dev default in production,
same pattern as BIDDER_HMAC_SECRET."
```

---

## Task 2: Implement SSE token helpers (`internal/auth/sse_token.go`)

**Files:**
- Create: `internal/auth/sse_token.go`
- Create: `internal/auth/sse_token_test.go`

- [ ] **Step 1: Write failing tests first**

Create `internal/auth/sse_token_test.go`:

```go
package auth

import (
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("test-secret-long-enough-for-hmac-12345678")

func TestIssueAndValidateSSEToken_HappyPath(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok := IssueSSEToken(testSecret, 42, 5*time.Minute, now)
	if tok == "" {
		t.Fatal("IssueSSEToken returned empty token")
	}
	if !strings.Contains(tok, ".") {
		t.Fatalf("token should contain . separator, got %q", tok)
	}
	advID, err := ValidateSSEToken(testSecret, tok, now.Add(1*time.Minute))
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if advID != 42 {
		t.Fatalf("expected advID 42, got %d", advID)
	}
}

func TestValidateSSEToken_Expired(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok := IssueSSEToken(testSecret, 42, 5*time.Minute, now)
	if _, err := ValidateSSEToken(testSecret, tok, now.Add(6*time.Minute)); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestValidateSSEToken_WrongSecret(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok := IssueSSEToken(testSecret, 42, 5*time.Minute, now)
	other := []byte("other-secret-long-enough-for-hmac-1234567")
	if _, err := ValidateSSEToken(other, tok, now); err == nil {
		t.Fatal("expected wrong-secret token to be rejected")
	}
}

func TestValidateSSEToken_MalformedTokens(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cases := []string{
		"",
		"notoken",
		"bogus.signature.extra",
		"aGVsbG8.",
		".notsigned",
		"!!!.!!!",
	}
	for _, c := range cases {
		if _, err := ValidateSSEToken(testSecret, c, now); err == nil {
			t.Errorf("expected malformed token %q to be rejected", c)
		}
	}
}

func TestValidateSSEToken_TamperedPayload(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok := IssueSSEToken(testSecret, 42, 5*time.Minute, now)
	// Flip a character in the payload half
	idx := strings.Index(tok, ".")
	if idx < 1 {
		t.Fatal("unexpected token format")
	}
	// Replace first char of payload with its next ASCII char
	tampered := string(tok[0]+1) + tok[1:]
	if _, err := ValidateSSEToken(testSecret, tampered, now); err == nil {
		t.Fatal("expected tampered token to be rejected")
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./internal/auth/ -run TestIssueAndValidateSSEToken -v
go test ./internal/auth/ -run TestValidateSSEToken -v
```

Expected: FAIL with undefined `IssueSSEToken` / `ValidateSSEToken`.

- [ ] **Step 3: Implement `sse_token.go`**

Create `internal/auth/sse_token.go`:

```go
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// IssueSSEToken returns a short-lived HMAC-signed token that binds an
// advertiser ID to an expiry time. The token format is:
//
//	base64url(<advID>:<expUnix>) + "." + hex(HMAC-SHA256(secret, <advID>:<expUnix>))
//
// The raw payload (before base64url) is also what gets signed, so validation
// decodes, verifies, and parses back into advID / exp.
func IssueSSEToken(secret []byte, advID int64, ttl time.Duration, now time.Time) string {
	payload := fmt.Sprintf("%d:%d", advID, now.Add(ttl).Unix())
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}

// ValidateSSEToken parses and verifies an SSE token. It returns the
// authenticated advertiser ID on success. Errors cover malformed tokens,
// signature mismatches, and expired tokens. Constant-time comparison is
// used on the HMAC check to prevent timing oracles.
func ValidateSSEToken(secret []byte, token string, now time.Time) (int64, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, errors.New("malformed token")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, errors.New("malformed token payload")
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(payloadBytes)
	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(parts[1])) != 1 {
		return 0, errors.New("invalid signature")
	}
	payload := string(payloadBytes)
	sep := strings.IndexByte(payload, ':')
	if sep < 1 || sep == len(payload)-1 {
		return 0, errors.New("malformed payload")
	}
	advID, err := strconv.ParseInt(payload[:sep], 10, 64)
	if err != nil || advID <= 0 {
		return 0, errors.New("invalid advertiser id")
	}
	expUnix, err := strconv.ParseInt(payload[sep+1:], 10, 64)
	if err != nil {
		return 0, errors.New("invalid expiry")
	}
	if now.Unix() >= expUnix {
		return 0, errors.New("token expired")
	}
	return advID, nil
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/auth/ -run "TestIssueAndValidateSSEToken|TestValidateSSEToken" -v
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/sse_token.go internal/auth/sse_token_test.go
git commit -m "feat(auth): add IssueSSEToken + ValidateSSEToken for analytics SSE auth

Self-contained HMAC-SHA256 token with 5-minute TTL. Format:
base64url(advID:expUnix).hex(hmac). Used by SSETokenMiddleware to
authenticate EventSource requests without leaking X-API-Key into URL
query (V5.1 hotfix P1-1).

Tests cover happy path, expiry, wrong secret, tampered payload,
malformed input, and empty token."
```

---

## Task 3: Implement `SSETokenMiddleware` + tests

**Files:**
- Create: `internal/auth/sse_middleware.go`
- Create: `internal/auth/sse_middleware_test.go`

- [ ] **Step 1: Write failing middleware test**

Create `internal/auth/sse_middleware_test.go`:

```go
package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSSETokenMiddleware_ValidToken_InjectsAdvertiser(t *testing.T) {
	secret := []byte("test-secret-long-enough-for-hmac-12345678")
	now := time.Now()
	tok := IssueSSEToken(secret, 99, 5*time.Minute, now)

	var gotAdvID int64
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAdvID = AdvertiserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := SSETokenMiddleware(secret)(inner)
	req := httptest.NewRequest("GET", "/api/v1/analytics/stream?token="+tok, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotAdvID != 99 {
		t.Fatalf("expected advID 99 in context, got %d", gotAdvID)
	}
}

func TestSSETokenMiddleware_MissingToken_Returns401(t *testing.T) {
	secret := []byte("test-secret-long-enough-for-hmac-12345678")
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called")
	})
	handler := SSETokenMiddleware(secret)(inner)
	req := httptest.NewRequest("GET", "/api/v1/analytics/stream", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSSETokenMiddleware_InvalidToken_Returns401(t *testing.T) {
	secret := []byte("test-secret-long-enough-for-hmac-12345678")
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called")
	})
	handler := SSETokenMiddleware(secret)(inner)
	req := httptest.NewRequest("GET", "/api/v1/analytics/stream?token=garbage.signature", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSSETokenMiddleware_RejectsQueryApiKey(t *testing.T) {
	// Regression test: make sure the middleware does NOT accept X-API-Key
	// via query param, even as a fallback. That was the exact P1 being fixed.
	secret := []byte("test-secret-long-enough-for-hmac-12345678")
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called for api_key query")
	})
	handler := SSETokenMiddleware(secret)(inner)
	req := httptest.NewRequest("GET", "/api/v1/analytics/stream?api_key=dsp_abc123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for api_key query, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./internal/auth/ -run TestSSETokenMiddleware -v
```

Expected: FAIL, undefined `SSETokenMiddleware`.

- [ ] **Step 3: Implement middleware**

Create `internal/auth/sse_middleware.go`:

```go
package auth

import (
	"context"
	"net/http"
	"time"
)

// SSETokenMiddleware returns middleware that authenticates requests using
// a short-lived HMAC token in the ?token= query parameter. It exists for
// EventSource/SSE clients that cannot set custom headers.
//
// Unlike APIKeyMiddleware, the long-lived tenant X-API-Key is never
// exposed via URL query. Clients first POST to /api/v1/analytics/token
// (authenticated via X-API-Key header, normal chain) to mint a 5-minute
// SSE token, then use that token in the stream URL.
func SSETokenMiddleware(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.URL.Query().Get("token")
			if token == "" {
				writeAuthError(w, http.StatusUnauthorized, "missing SSE token")
				return
			}
			advID, err := ValidateSSEToken(secret, token, time.Now())
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "invalid SSE token")
				return
			}
			adv := &Advertiser{ID: advID}
			ctx := context.WithValue(r.Context(), advertiserKey, adv)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/auth/ -run TestSSETokenMiddleware -v
```

Expected: all 4 subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/sse_middleware.go internal/auth/sse_middleware_test.go
git commit -m "feat(auth): add SSETokenMiddleware for analytics EventSource auth

Validates ?token= query via HMAC and injects *Advertiser into context
using the same advertiserKey pattern as APIKeyMiddleware. Does NOT
accept ?api_key= query fallback — that was the P1 being fixed.

Regression test TestSSETokenMiddleware_RejectsQueryApiKey locks in
the constraint."
```

---

## Task 4: Wire token-issue endpoint + route split + remove old exemption

**Files:**
- Modify: `internal/handler/analytics.go` (add `HandleAnalyticsStreamToken`)
- Modify: `internal/handler/routes.go` (split analytics routes, add token endpoint, move `HandleCreateAdvertiser` — but that's Task 5; this task only moves analytics)
- Modify: `internal/handler/middleware.go` (delete the `api_key` query block)
- Create: `internal/handler/analytics_token_test.go`

- [ ] **Step 1: Write failing handler test**

Create `internal/handler/analytics_token_test.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
)

func TestHandleAnalyticsStreamToken_ReturnsValidToken(t *testing.T) {
	secret := []byte("test-secret-long-enough-for-hmac-12345678")
	d := &Deps{HMACSecret: secret}

	req := httptest.NewRequest("POST", "/api/v1/analytics/token", nil)
	// Simulate APIKeyMiddleware having run — inject advertiser into context.
	ctx := auth.WithAdvertiserForTest(req.Context(), 77)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	d.HandleAnalyticsStreamToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("expected non-empty token")
	}
	advID, err := auth.ValidateSSEToken(secret, resp.Token, time.Now())
	if err != nil {
		t.Fatalf("returned token fails its own validation: %v", err)
	}
	if advID != 77 {
		t.Fatalf("expected advID 77, got %d", advID)
	}
}

func TestHandleAnalyticsStreamToken_Unauthenticated_Returns401(t *testing.T) {
	d := &Deps{HMACSecret: []byte("test-secret-long-enough-for-hmac-12345678")}
	req := httptest.NewRequest("POST", "/api/v1/analytics/token", nil)
	rec := httptest.NewRecorder()
	d.HandleAnalyticsStreamToken(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without context advertiser, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test — expect FAIL**

```bash
go test ./internal/handler/ -run TestHandleAnalyticsStreamToken -v
```

Expected: FAIL, undefined `HandleAnalyticsStreamToken`.

- [ ] **Step 3: Implement the handler**

Append to `internal/handler/analytics.go`:

```go
// HandleAnalyticsStreamToken godoc
// @Summary Issue a short-lived SSE auth token for /analytics/stream
// @Description Returns a 5-minute HMAC-signed token bound to the authenticated advertiser.
// @Description The token is used in the ?token= query of /api/v1/analytics/stream to authenticate
// @Description EventSource connections without exposing the long-lived X-API-Key in URL logs.
// @Tags analytics
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} object{token=string,expires_at=integer}
// @Failure 401 {object} object{error=string}
// @Router /analytics/token [post]
func (d *Deps) HandleAnalyticsStreamToken(w http.ResponseWriter, r *http.Request) {
	advID := auth.AdvertiserIDFromContext(r.Context())
	if advID == 0 {
		WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if len(d.HMACSecret) == 0 {
		WriteError(w, http.StatusInternalServerError, "SSE token signing not configured")
		return
	}
	ttl := 5 * time.Minute
	now := time.Now()
	token := auth.IssueSSEToken(d.HMACSecret, advID, ttl, now)
	WriteJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": now.Add(ttl).Unix(),
	})
}
```

- [ ] **Step 4: Run handler test — expect PASS**

```bash
go test ./internal/handler/ -run TestHandleAnalyticsStreamToken -v
```

Expected: both subtests PASS.

- [ ] **Step 5: Update `routes.go` — split analytics routes**

Edit `internal/handler/routes.go`:

1. In `BuildPublicMux` at lines 44-45 (the two `mux.HandleFunc("GET /api/v1/analytics/...")` lines), **delete those two lines**.
2. Add the token issue endpoint to `BuildPublicMux` after the deletion point:
   ```go
   mux.HandleFunc("POST /api/v1/analytics/token", d.HandleAnalyticsStreamToken)
   ```
3. Add a new function below `BuildAdminMux`:
   ```go
   // BuildAnalyticsSSEMux returns a dedicated mux for SSE-authenticated
   // analytics endpoints. These use short-lived HMAC tokens instead of
   // long-lived X-API-Key to avoid leaking credentials into URL logs
   // (V5.1 hotfix P1-1).
   func BuildAnalyticsSSEMux(d *Deps) *http.ServeMux {
       mux := http.NewServeMux()
       mux.HandleFunc("GET /api/v1/analytics/stream", d.HandleAnalyticsStream)
       mux.HandleFunc("GET /api/v1/analytics/snapshot", d.HandleAnalyticsSnapshot)
       return mux
   }
   ```
4. Edit `BuildPublicHandler` (line 95). Replace the body with:
   ```go
   func BuildPublicHandler(cfg *config.Config, d *Deps) http.Handler {
       publicMux := BuildPublicMux(d)
       apiKeyLookup := func(ctx context.Context, key string) (int64, string, string, error) {
           adv, err := d.Store.GetAdvertiserByAPIKey(ctx, key)
           if err != nil {
               return 0, "", "", err
           }
           return adv.ID, adv.CompanyName, adv.ContactEmail, nil
       }
       limiter := ratelimit.New(d.Redis)
       authed := auth.APIKeyMiddleware(apiKeyLookup)(publicMux)
       rateLimited := ratelimit.Middleware(limiter, ratelimit.APIKeyFunc, 100, time.Minute)(authed)
       withExemption := WithAuthExemption(rateLimited, publicMux)

       // Analytics SSE endpoints use HMAC-signed ?token= instead of X-API-Key
       // to avoid leaking tenant credentials into EventSource URL logs.
       // /api/v1/analytics/token stays in publicMux (APIKeyMiddleware-gated)
       // to mint tokens; /api/v1/analytics/stream and /snapshot are routed
       // through SSETokenMiddleware via a dedicated sub-mux.
       analyticsSSEMux := BuildAnalyticsSSEMux(d)
       analyticsSSE := auth.SSETokenMiddleware(d.HMACSecret)(analyticsSSEMux)

       dispatcher := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           p := r.URL.Path
           if p == "/api/v1/analytics/stream" || p == "/api/v1/analytics/snapshot" {
               analyticsSSE.ServeHTTP(w, r)
               return
           }
           withExemption.ServeHTTP(w, r)
       })

       return WithCORS(cfg, observability.RequestIDMiddleware(observability.LoggingMiddleware(dispatcher)))
   }
   ```

- [ ] **Step 6: Delete analytics api_key block from middleware.go**

Edit `internal/handler/middleware.go`. Delete lines 13-18 (the `strings.HasPrefix(r.URL.Path, "/api/v1/analytics/")` block). The remaining `WithAuthExemption` body becomes:

```go
func WithAuthExemption(authed http.Handler, publicMux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/api/v1/docs" || strings.HasPrefix(r.URL.Path, "/uploads/") || (r.Method == "POST" && r.URL.Path == "/api/v1/register") {
			publicMux.ServeHTTP(w, r)
			return
		}
		authed.ServeHTTP(w, r)
	})
}
```

Also remove the now-unused import if `strings` is no longer referenced (it still is, for the `/uploads/` prefix check, so keep it).

- [ ] **Step 7: Build + run full test suite**

```bash
go build ./...
go test ./internal/auth/ ./internal/handler/ ./internal/config/ -count=1 -v
```

Expected: clean build, all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/handler/analytics.go internal/handler/analytics_token_test.go internal/handler/routes.go internal/handler/middleware.go
git commit -m "feat(p1-1): remove analytics SSE X-API-Key query leak

Split /api/v1/analytics/stream and /snapshot into a dedicated sub-mux
behind SSETokenMiddleware. Add POST /api/v1/analytics/token
(APIKeyMiddleware-gated) that mints 5-min HMAC SSE tokens. Delete the
\`?api_key=\` query fallback from WithAuthExemption — it was a
credential-in-URL leak that survived V5 because the audit scope did not
cover middleware path exemptions.

Backend fix only. Frontend migration lands in the same PR (Task 5)."
```

---

## Task 5: Frontend — migrate `web/app/analytics/page.tsx` to token flow

**Files:**
- Modify: `web/app/analytics/page.tsx`

**IMPORTANT — read `web/AGENTS.md` before editing**: the file warns that this is NOT standard Next.js; read `node_modules/next/dist/docs/` for current conventions before writing code.

- [ ] **Step 1: Read web/AGENTS.md**

```bash
cat web/AGENTS.md
```

Note the warning. If the Next.js docs directory exists, skim the relevant guide (App Router / data fetching / client components) before editing.

- [ ] **Step 2: Read the current analytics page**

```bash
cat web/app/analytics/page.tsx
```

Note the current EventSource setup at line 34-38.

- [ ] **Step 3: Implement new token-fetch flow**

Replace the current EventSource initialization (around lines 30-60 of `web/app/analytics/page.tsx`) with a two-step flow:

```tsx
"use client";
// ... keep existing imports and state ...

useEffect(() => {
  if (typeof window === "undefined") return;
  const apiKey = localStorage.getItem("dsp_api_key") || "";
  if (!apiKey) return;

  let cancelled = false;
  let es: EventSource | null = null;
  let refreshTimer: ReturnType<typeof setTimeout> | null = null;

  async function fetchToken(): Promise<string | null> {
    const r = await fetch(`${API_BASE}/api/v1/analytics/token`, {
      method: "POST",
      headers: { "X-API-Key": apiKey },
    });
    if (!r.ok) return null;
    const body = await r.json() as { token: string; expires_at: number };
    return body.token;
  }

  async function connect() {
    const token = await fetchToken();
    if (cancelled || !token) return;
    const url = `${API_BASE}/api/v1/analytics/stream?token=${encodeURIComponent(token)}`;
    es = new EventSource(url);
    es.onmessage = (ev) => {
      // ... existing onmessage handler ...
    };
    es.onerror = () => {
      // EventSource auto-reconnects; re-mint on next reconnect attempt by
      // closing and scheduling a full re-connect in 2s so the server sees
      // a fresh, non-expired token.
      if (es) es.close();
      es = null;
      if (!cancelled) refreshTimer = setTimeout(() => { connect(); }, 2000);
    };
    // Pre-emptive refresh at 4 minutes (token is 5-min TTL, 1-min buffer)
    refreshTimer = setTimeout(() => {
      if (es) es.close();
      es = null;
      if (!cancelled) connect();
    }, 4 * 60 * 1000);
  }

  connect();

  return () => {
    cancelled = true;
    if (refreshTimer) clearTimeout(refreshTimer);
    if (es) es.close();
  };
}, []);
```

Also ensure the `onmessage` body is preserved from the original (copy the state-update logic verbatim).

- [ ] **Step 4: Commit**

```bash
git add web/app/analytics/page.tsx
git commit -m "feat(p1-1): frontend — use SSE token flow instead of api_key query

Fetch short-lived token via POST /api/v1/analytics/token (X-API-Key
header), then use token in EventSource ?token= query. Pre-emptive
refresh at 4 min (1 min before 5-min TTL expiry). Close + reconnect
on error with a fresh token.

Closes the tenant credential leak into proxy/browser history/referrer
logs that was P1-1."
```

---

## Task 6: P1-2 — Move `HandleCreateAdvertiser` to admin mux

**Files:**
- Modify: `internal/handler/routes.go`
- Modify: `internal/handler/campaign.go` (godoc update only)
- Modify: `cmd/autopilot/client.go`
- Test: new integration test in `test/integration/v5_1_hotfix_test.go`

- [ ] **Step 1: Write failing integration test first**

Create `test/integration/v5_1_hotfix_test.go` if it doesn't exist yet (if it was already created in a later task, append to it):

```go
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestP1_2_CreateAdvertiser_BlockedOnPublicPath verifies that the
// privilege-escalation path is closed: any tenant-authenticated POST to
// the old public /api/v1/advertisers path must now return 404 (route no
// longer registered). Admin path with X-Admin-Token must still work.
func TestP1_2_CreateAdvertiser_BlockedOnPublicPath(t *testing.T) {
	h := newTestHarness(t)  // uses real Postgres + real campaign.Store
	defer h.Close()

	// Create a legitimate tenant via the proper register/approve flow.
	tenant := h.RegisterAndApprove(t, "tenantco", "tc@example.com")

	// Attempt: tenant POSTs to old public path.
	body, _ := json.Marshal(map[string]any{
		"company_name":  "evilco",
		"contact_email": "evil@example.com",
		"balance_cents": 1_000_000_00,
	})
	req, _ := http.NewRequest("POST", h.PublicURL+"/api/v1/advertisers", bytes.NewReader(body))
	req.Header.Set("X-API-Key", tenant.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 404 or 405 from public /advertisers, got %d", resp.StatusCode)
	}

	// Admin path must still work.
	req2, _ := http.NewRequest("POST", h.InternalURL+"/api/v1/admin/advertisers", bytes.NewReader(body))
	req2.Header.Set("X-Admin-Token", h.AdminToken)
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("admin request: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from admin /advertisers, got %d", resp2.StatusCode)
	}
}
```

**Note:** `newTestHarness`, `RegisterAndApprove`, etc. may or may not exist. If they do (per memory, V5 has `test/integration/harness_test.go`), reuse. If not, STOP and escalate — integration test infra needs to be built, which is Phase 2 scope, not hotfix.

- [ ] **Step 2: Run test — expect FAIL (compile error if harness missing, or wrong status)**

```bash
go test ./test/integration/ -run TestP1_2_CreateAdvertiser_BlockedOnPublicPath -v -count=1
```

Expected: FAIL. If FAIL is due to missing harness types, stop and escalate.

- [ ] **Step 3: Move route in `routes.go`**

Edit `internal/handler/routes.go`:
1. In `BuildPublicMux` at line 20, **delete**: `mux.HandleFunc("POST /api/v1/advertisers", d.HandleCreateAdvertiser)`
2. In `BuildAdminMux` near line 83 (next to `HandleListAdvertisers`), **add**:
   ```go
   mux.HandleFunc("POST /api/v1/admin/advertisers", d.HandleCreateAdvertiser)
   ```

- [ ] **Step 4: Update godoc on the handler**

Edit `internal/handler/campaign.go` lines 15-23. Change the godoc block:

```go
// HandleCreateAdvertiser godoc
// @Summary Create advertiser (admin only)
// @Description Admin-only shortcut for bootstrapping an advertiser. Regular
// @Description tenants must use POST /api/v1/register → admin approval instead.
// @Description This endpoint was moved behind admin auth in V5.1 to close a
// @Description privilege-escalation path where any authenticated tenant could
// @Description self-credit a new advertiser via the balance_cents field.
// @Tags admin
// @Security AdminAuth
// @Accept json
// @Produce json
// @Param body body object{company_name=string,contact_email=string,balance_cents=integer} true "Advertiser data"
// @Success 201 {object} object{id=integer,api_key=string,message=string}
// @Failure 400 {object} object{error=string}
// @Failure 401 {object} object{error=string}
// @Router /admin/advertisers [post]
```

- [ ] **Step 5: Update autopilot client**

Edit `cmd/autopilot/client.go`. Replace `CreateAdvertiser` (lines 108-124) with an admin-path version:

```go
// CreateAdvertiser creates an advertiser via the admin API. Requires
// AdminToken to be set. adminURL is the internal API base
// (e.g., http://localhost:8182). This changed in V5.1 — the old public
// POST /api/v1/advertisers path is no longer registered.
func (c *DSPClient) CreateAdvertiser(adminURL, companyName, email string) (*AdvertiserResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"company_name":  companyName,
		"contact_email": email,
	})
	req, _ := http.NewRequest("POST", adminURL+"/api/v1/admin/advertisers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Token", c.AdminToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create advertiser: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create advertiser: status %d, body: %s", resp.StatusCode, data)
	}
	var result AdvertiserResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("create advertiser: decode: %w", err)
	}
	return &result, nil
}
```

Then grep the repo for callers of `CreateAdvertiser` and update their call sites to pass `adminURL`:

```bash
grep -rn "CreateAdvertiser(" cmd/autopilot/
```

Update each caller to pass the admin URL (same base as used by `AdminCreateInviteCode`, etc.). If a caller has no access to adminURL, add it to that caller's config.

- [ ] **Step 6: Run integration test — expect PASS**

```bash
go test ./test/integration/ -run TestP1_2_CreateAdvertiser_BlockedOnPublicPath -v -count=1
```

Expected: PASS.

- [ ] **Step 7: Run full Go build + unit suite**

```bash
go build ./...
go test ./... -count=1
```

Expected: clean build, all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/handler/routes.go internal/handler/campaign.go cmd/autopilot/client.go test/integration/v5_1_hotfix_test.go
git commit -m "fix(p1-2): move HandleCreateAdvertiser to admin mux

The public POST /api/v1/advertisers path was a tenant→advertiser
privilege escalation: any authenticated tenant could POST with a
body containing balance_cents and create a new advertiser with any
initial balance, whose api_key was returned in the response.

Fix: moved the route from BuildPublicMux to BuildAdminMux behind
AdminAuthMiddleware. The legitimate tenant bootstrap path
(POST /api/v1/register → admin approval) already existed.

Updated cmd/autopilot/client.go CreateAdvertiser to hit the admin
path via adminURL + X-Admin-Token. Integration regression test in
test/integration/v5_1_hotfix_test.go locks in the 404 on the old
public path and 201 on the new admin path.

The hotfix does NOT further restrict balance_cents self-setting
under admin; that tightening is Phase 2C scope."
```

---

## Task 7: P1-3 — Delete `/click?dest=` handling from bidder

**Files:**
- Modify: `cmd/bidder/main.go` (lines 499, 516-518, 569-571)
- Test: `cmd/bidder/main_test.go`

- [ ] **Step 1: Read the current handleClick + injectClickTracker**

```bash
sed -n '495,575p' cmd/bidder/main.go
sed -n '630,640p' cmd/bidder/main.go
```

Confirm:
- Line 499: `dest := r.URL.Query().Get("dest")` — the read
- Line 516-518: `if dest != "" { http.Redirect(w, r, dest, http.StatusFound); return }` — dedup branch
- Line 569-571: `if dest != "" { http.Redirect(w, r, dest, http.StatusFound); return }` — happy path
- Line 276-277: `clickURL := fmt.Sprintf("%s/click?campaign_id=%s&request_id=%s&token=%s", ...)` — NO dest in legitimate emission
- Line 633 `injectClickTracker` — only appends 1x1 pixel, no dest reference

- [ ] **Step 2: Write failing regression test**

Append to `cmd/bidder/main_test.go`:

```go
func TestHandleClick_RejectsArbitraryDest_NoRedirect(t *testing.T) {
	// Regression: /click?dest=https://evil.example must NOT 302 to evil.
	// The dest query parameter was an open redirect — kept as dead code
	// through V5. V5.1 hotfix P1-3 deleted the entire dest branch.
	d := newTestBidderDeps(t) // build minimal Deps with HMACSecret + in-memory deps
	token := auth.GenerateToken(d.HMACSecret, "1", "req-P1-3")
	url := fmt.Sprintf(
		"/click?campaign_id=1&request_id=req-P1-3&token=%s&dest=https://evil.example",
		token,
	)
	req := httptest.NewRequest("GET", url, nil)
	rec := httptest.NewRecorder()
	d.handleClick(rec, req)

	// Acceptable outcomes: 200 JSON status=clicked, or 409 budget issues.
	// NOT acceptable: 302 Location: https://evil.example
	if rec.Code == http.StatusFound {
		loc := rec.Header().Get("Location")
		if strings.Contains(loc, "evil.example") {
			t.Fatalf("open redirect regression: Location=%q", loc)
		}
	}
	// Also confirm Location header is not set at all in the happy path
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("unexpected Location header in click response: %q", loc)
	}
}
```

**Note:** `newTestBidderDeps` may need to be added if bidder tests use a different helper. Grep first to find the existing pattern:

```bash
grep -n "Deps{" cmd/bidder/main_test.go cmd/bidder/handlers_integration_test.go
```

Use whichever helper is already in use. If none of them give you a `Deps` with a real budget service, stub budget to something benign for this test (the assertion is about Location header only, not budget semantics).

- [ ] **Step 3: Run test — expect FAIL**

```bash
go test ./cmd/bidder/ -run TestHandleClick_RejectsArbitraryDest -v
```

Expected: FAIL — the current code WILL redirect to `https://evil.example`.

- [ ] **Step 4: Delete the dest handling**

Edit `cmd/bidder/main.go`:

1. **Line 499** — delete: `dest := r.URL.Query().Get("dest")`
2. **Lines 516-518** (inside the dedup branch) — delete:
   ```go
   if dest != "" {
       http.Redirect(w, r, dest, http.StatusFound)
       return
   }
   ```
3. **Lines 569-571** (end of handleClick) — delete:
   ```go
   if dest != "" {
       http.Redirect(w, r, dest, http.StatusFound)
       return
   }
   ```
4. Add a godoc note at the top of `handleClick` near line 495:
   ```go
   // handleClick validates a click callback's HMAC token, deduplicates by
   // request_id, and (for CPC campaigns) deducts the per-click budget.
   //
   // V5.1 hotfix P1-3: the legacy ?dest= redirect parameter was deleted.
   // It was dead code in the legitimate flow (injectClickTracker never
   // emits it — see line ~276) and an open-redirect attack surface for
   // anyone who could observe a valid click URL.
   func (d *Deps) handleClick(w http.ResponseWriter, r *http.Request) {
   ```

- [ ] **Step 5: Run test — expect PASS**

```bash
go test ./cmd/bidder/ -run TestHandleClick_RejectsArbitraryDest -v
```

Expected: PASS. Location header is not set.

- [ ] **Step 6: Run full bidder test suite**

```bash
go test ./cmd/bidder/ -count=1 -v
```

Expected: all bidder tests PASS. None of the existing tests should depend on the `dest` redirect — if any does, grep to find it and decide: test was wrong (update test), or test documents legitimate behavior (STOP and escalate).

- [ ] **Step 7: Commit**

```bash
git add cmd/bidder/main.go cmd/bidder/main_test.go
git commit -m "fix(p1-3): delete /click?dest= open redirect dead code

The dest query parameter was a 302-to-arbitrary-URL open redirect
protected only by HMAC on (campaign_id, request_id) — not on dest
itself. Legitimate click URLs never set dest (injectClickTracker at
line ~633 only appends a 1x1 tracking pixel and cmd/bidder/main.go:276
constructs the click URL without it), so the whole dest branch was
100% dead code + attack surface.

Anyone who observed a valid click URL (in ad exchange logs, browser
history, referrer headers, or by replaying a token within its 5-min
TTL) could construct /click?campaign_id=...&request_id=...&token=...&dest=https://phish.example
and use bidder's public domain to bounce users anywhere.

Regression test TestHandleClick_RejectsArbitraryDest_NoRedirect in
cmd/bidder/main_test.go locks in the constraint."
```

---

## Task 8: Integration — full Phase 1 smoke

**Files:**
- Test: `test/integration/v5_1_hotfix_test.go` (expanded if not already done in Task 6)

- [ ] **Step 1: Add P1-1 end-to-end integration test**

Append to `test/integration/v5_1_hotfix_test.go`:

```go
// TestP1_1_AnalyticsSSE_RejectsApiKeyQuery verifies the V5.1 hotfix:
// /api/v1/analytics/stream must no longer accept ?api_key=. Only the
// short-lived ?token= (issued via POST /api/v1/analytics/token) works.
func TestP1_1_AnalyticsSSE_RejectsApiKeyQuery(t *testing.T) {
	h := newTestHarness(t)
	defer h.Close()

	tenant := h.RegisterAndApprove(t, "tenantco", "tc@example.com")

	// OLD attack path: ?api_key= in URL. Must now return 401.
	req, _ := http.NewRequest("GET",
		h.PublicURL+"/api/v1/analytics/stream?api_key="+tenant.APIKey, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("api_key query: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for ?api_key= query, got %d", resp.StatusCode)
	}

	// NEW flow: POST /analytics/token with X-API-Key header, then use token.
	tokReq, _ := http.NewRequest("POST", h.PublicURL+"/api/v1/analytics/token", nil)
	tokReq.Header.Set("X-API-Key", tenant.APIKey)
	tokResp, err := http.DefaultClient.Do(tokReq)
	if err != nil {
		t.Fatalf("token request: %v", err)
	}
	defer tokResp.Body.Close()
	if tokResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /analytics/token, got %d", tokResp.StatusCode)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(tokResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if body.Token == "" {
		t.Fatal("empty token")
	}

	// Now the stream should accept the signed token.
	streamReq, _ := http.NewRequest("GET",
		h.PublicURL+"/api/v1/analytics/stream?token="+body.Token, nil)
	streamReq.Header.Set("Accept", "text/event-stream")
	streamResp, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from signed token stream, got %d", streamResp.StatusCode)
	}
	// Close the SSE stream — the test only verifies auth acceptance.
}
```

- [ ] **Step 2: Run the full hotfix integration suite**

```bash
go test ./test/integration/ -run "TestP1_" -v -count=1
```

Expected: P1_1 and P1_2 tests PASS.

- [ ] **Step 3: Commit**

```bash
git add test/integration/v5_1_hotfix_test.go
git commit -m "test(v5.1): integration tests for P1-1 analytics SSE token flow"
```

---

## Task 9: Phase 1 boundary loop

**Purpose:** Per CLAUDE.md "Three Hard Rules", no hotfix lands without a full per-phase loop of `requesting-code-review` + `verification-before-completion` + `/qa`. Any fix in any round triggers a new round. Max 5 rounds. Stop only on a zero-issue round.

- [ ] **Step 1: Round 1 — requesting-code-review**

Invoke the `superpowers:requesting-code-review` skill with:
- Scope: this branch (`review-remediation-v5.1-hotfix`) vs main
- Review dimensions: spec compliance against this plan + code quality + security + test adequacy (special attention: tenant-isolation tests must hit real Store per CLAUDE.md)
- Reviewer model: Opus
- Must independently re-verify all 3 P1 fix correctness

Fix every Critical and Important issue inline. Re-dispatch the reviewer to confirm fixes.

- [ ] **Step 2: Round 1 — verification-before-completion**

Invoke the `superpowers:verification-before-completion` skill:
- Start real Postgres + Redis (the project's standard dev stack)
- Start `cmd/api` and `cmd/bidder` with `API_HMAC_SECRET=...` and `BIDDER_HMAC_SECRET=...` set to non-default values
- Run `go test ./... -count=1`
- Run `test/integration/v5_1_hotfix_test.go` against the live services
- Additionally curl-test:
  - `curl -v "http://localhost:8181/api/v1/analytics/stream?api_key=dsp_xxx"` → must 401
  - `curl -v -X POST -H "X-API-Key: dsp_xxx" http://localhost:8181/api/v1/analytics/token` → must 200 JSON
  - `curl -v "http://localhost:8181/api/v1/analytics/stream?token=<token>"` → must 200 SSE
  - `curl -v -X POST -H "X-API-Key: dsp_xxx" http://localhost:8181/api/v1/advertisers -d '{...}'` → must 404
  - `curl -v "http://localhost:8282/click?campaign_id=1&request_id=r1&token=<valid>&dest=https://evil.example"` → must NOT have `Location: https://evil.example` in response headers

- [ ] **Step 3: Round 1 — /qa**

Invoke `/qa` (gstack) for headless-browser testing:
- Focus pages: `/analytics` (verify SSE works end-to-end with new token flow), `/` (tenant login), `/campaigns`, `/reports` (smoke)
- Verify no regressions in ApiKeyGate behavior (for tenant routes — admin routes' ApiKeyGate bug stays open for Phase 2C)
- Console error watch: no 401s on analytics page after login

- [ ] **Step 4: Decide on Round 2**

If Round 1 had ANY fix (review finding fixed, verification issue fixed, QA issue fixed):
- **Round 2 is MANDATORY**. Go back to Step 1 and repeat the full loop.

If Round 1 was clean (zero issues across all three steps):
- Phase 1 boundary loop is COMPLETE. Proceed to Task 10.

Cap: 5 rounds. If still finding issues after 5 rounds, STOP and escalate to user — indicates deeper design problem.

- [ ] **Step 5: Commit any fixes from the loop**

Each round's fixes get their own commits tagged `roundN-fix(v5.1): ...`. Do NOT squash until Task 10.

---

## Task 10: Land Phase 1 hotfix

- [ ] **Step 1: Confirm all 9 preceding tasks complete + boundary loop clean**

```bash
git log --oneline main..HEAD
```

Expected: a small set of commits covering config, auth helpers, route surgery, frontend migration, test, round-fixes.

- [ ] **Step 2: Ask user — merge to main or push + PR?**

Use AskUserQuestion. Present both options. Wait for decision. Default recommendation: push + create PR via `gh pr create` for audit trail visibility, since this is a security hotfix touching auth.

- [ ] **Step 3: Execute chosen landing strategy**

If merge: `git checkout main && git merge --no-ff review-remediation-v5.1-hotfix -m "merge(V5.1): P1 security hotfix — 3 findings from 2026-04-15 independent review"` then push.

If PR: `git push origin review-remediation-v5.1-hotfix && gh pr create --title "V5.1 security hotfix: 3 P1 findings from 2026-04-15 review" --body ...`

- [ ] **Step 4: Update memory**

Update `~/.claude/projects/C--Users-Roc-github-dsp/memory/project_v5_completed.md` (or add a new `project_v5_1_hotfix.md`) recording:
- V5.1 hotfix landed, date
- 3 P1s fixed, with one-line each
- Phase 2A/2B/2C still open
- Link to this plan doc

---

## Out of scope — defer to Phase 2

These P2 findings are explicitly NOT touched by this hotfix. Phase 2A/2B/2C plans will address them after V5.1 lands:

- **P2-C**: Contract unification (hand-written TS types in `web/lib/api.ts` / `admin-api.ts`, `/api/v1/docs` hand-built detachment, `make docs-check` CI, circuit-status `open`/`tripped` semantic confusion)
- **P2-S**: Other security — bidder `GET /stats` unauth, upload legacy dir, Redis→ratelimit fail-hard, ApiKeyGate admin route UX lock-out, admin/layout.tsx network-error fallback
- **P2-L**: Lifecycle — `http.Server` missing timeouts (slowloris) for both cmd/api and cmd/bidder, `LoadLocation("Asia/Shanghai")` dedup with `config.CSTLocation`
- **P2-O**: Observability — no prometheus business metrics, `/health` is liveness not readiness, `internal/alert.Noop{}` still active
- **P2-Q**: Code quality — `advertiserChargeCents` extraction, `0.90` magic number, `GetCampaign` nil during warm-up CPC→CPM silent fallback, admin.go JSON decode error swallowed

---

## Self-review (completed by plan author)

**1. Spec coverage:** All 3 P1 findings from the 2026-04-15 independent review have a dedicated task (P1-1 → Tasks 1-5, P1-2 → Task 6, P1-3 → Task 7). Task 8 cross-verifies P1-1 end-to-end. Task 9 is the mandatory CLAUDE.md boundary loop.

**2. Placeholder scan:** No TBD, no "add appropriate", no "similar to above" references. Every code step has actual code. Every command has an exact expected outcome. One caveat: Task 6 Step 1 references `newTestHarness` / `RegisterAndApprove` which may not exist in the project yet; the task explicitly flags this and says STOP if missing.

**3. Type consistency:** `IssueSSEToken` / `ValidateSSEToken` / `SSETokenMiddleware` signatures are consistent across Tasks 2-4. `HMACSecret []byte` field is used identically in Task 1 Step 6, Task 4 Step 3, and Task 8. Context injection uses `advertiserKey` constant + `*Advertiser{ID}` which matches `internal/auth/apikey.go:11,22-25`.

**4. CLAUDE.md alignment:** Per-task two-stage review is implicit in "subagent-driven-development" — each task goes implementer → spec reviewer → code quality reviewer. Task 9 boundary loop is explicitly per-phase. Integration tests hit real Store (CLAUDE.md: "Nil-store test stubs are specifically insufficient for tenant-isolation coverage").
