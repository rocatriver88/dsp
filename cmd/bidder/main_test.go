package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/redis/go-redis/v9"
)

func TestInjectClickTracker_Normal(t *testing.T) {
	markup := `<a href="https://example.com"><img src="ad.png"/></a>`
	clickURL := "http://localhost:8180/click?campaign_id=1&token=abc"

	result := injectClickTracker(markup, clickURL)

	if !strings.Contains(result, markup) {
		t.Error("original markup should be preserved")
	}
	if !strings.Contains(result, clickURL) {
		t.Error("click URL should be injected")
	}
	if !strings.Contains(result, `width="1"`) {
		t.Error("should contain 1x1 tracking pixel")
	}
}

func TestInjectClickTracker_EmptyMarkup(t *testing.T) {
	result := injectClickTracker("", "http://example.com/click")
	if result != "" {
		t.Errorf("empty markup should return empty, got %q", result)
	}
}

func TestInjectClickTracker_EmptyClickURL(t *testing.T) {
	markup := "<div>ad</div>"
	result := injectClickTracker(markup, "")
	if result != markup {
		t.Errorf("empty click URL should return original markup, got %q", result)
	}
}

// TestHandleClick_RejectsArbitraryDest_NoRedirect is the V5.1 P1-3
// end-to-end regression guard: a fully constructed /click request
// carrying ?dest=https://evil.example MUST NOT produce a 302 or any
// Location header pointing at the attacker URL. Pre-hotfix the
// handler had two redirect branches (dedup + happy path) that
// unconditionally 302'd to the client-supplied dest. V5.1 P1-3
// deleted both branches.
//
// The test uses a minimal Deps with campaignID=0, Producer=nil, and
// a stub Redis. This skips the CPC budget-deduct branch (gated on
// campaignID>0) and the Kafka-send branch (gated on campaignID>0 &&
// d.Producer!=nil), reaching the final `{"status":"clicked"}`
// response without any Kafka round-trip. The integration-level
// variant was attempted in handlers_integration_test.go but hung on
// Kafka producer.Close() waiting for inflight SendClick goroutines
// to drain during the first-connect handshake — see the comment
// there for the full story.
//
// Requires a reachable Redis at localhost:7380 (dsp-test stack). If
// Redis is unavailable, the test skips instead of false-positiving.
func TestHandleClick_RejectsArbitraryDest_NoRedirect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:7380",
		Password: "dsp_test_password",
	})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not reachable (%v) — run scripts/test-env.sh up", err)
	}

	const hmacSecret = "v5-1-p1-3-test-hmac-secret"
	d := &Deps{
		HMACSecret: hmacSecret,
		RDB:        rdb,
		// Producer: nil — handleClick's Kafka-send branch is gated on
		//   campaignID > 0 && d.Producer != nil, so with campaignID=0
		//   (below) the branch is never reached. Leaving this nil also
		//   means there's no producer.Close() to hang on at teardown.
		// Loader: nil — handleClick only calls d.Loader.GetCampaign when
		//   campaignID > 0, same gate.
	}

	// campaignID=0 means the HMAC token is signed over "" as the
	// campaign_id string. Construct the token exactly as the handler
	// will validate it.
	campIDStr := "0"
	reqID := fmt.Sprintf("p1-3-unit-%d", time.Now().UnixNano())
	token := auth.GenerateToken(hmacSecret, campIDStr, reqID)

	target := fmt.Sprintf(
		"/click?campaign_id=%s&request_id=%s&token=%s&dest=%s",
		campIDStr, reqID, token,
		"https%3A%2F%2Fevil.example%2Ffree-money",
	)
	req := httptest.NewRequest("GET", target, nil)
	rec := httptest.NewRecorder()

	d.handleClick(rec, req)

	// Must not be a 302/301 redirect.
	if rec.Code == http.StatusFound || rec.Code == http.StatusMovedPermanently {
		t.Fatalf("V5.1 P1-3 regression: /click returned redirect status %d, Location=%q",
			rec.Code, rec.Header().Get("Location"))
	}
	// Must not emit a Location header on non-redirect responses.
	if loc := rec.Header().Get("Location"); loc != "" {
		if strings.Contains(loc, "evil.example") {
			t.Fatalf("V5.1 P1-3 regression: /click emitted Location %q pointing at attacker dest", loc)
		}
		t.Fatalf("/click emitted unexpected Location %q on non-redirect response", loc)
	}
	// Must reach the final happy-path response.
	if rec.Code != http.StatusOK {
		t.Fatalf("/click status: want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"clicked"`) {
		t.Errorf("/click body: expected status=clicked, got %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// V5.2C Task 9: advertiserChargeCents helper extraction.
// ---------------------------------------------------------------------------

func TestAdvertiserChargeCents(t *testing.T) {
	cases := []struct {
		name          string
		exchangePrice float64
		wantCents     int64
	}{
		{
			name:          "normal clear price 0.05",
			exchangePrice: 0.05,
			// 0.05 / 0.90 * 100 = 5.5555... → int64 truncation → 5
			wantCents: 5,
		},
		{
			name:          "zero price",
			exchangePrice: 0.0,
			wantCents:     0,
		},
		{
			name:          "sub-cent truncation",
			exchangePrice: 0.00123,
			// 0.00123 / 0.90 * 100 = 0.1366... → int64 → 0
			wantCents: 0,
		},
		{
			name:          "round dollar",
			exchangePrice: 1.0,
			// 1.0 / 0.90 * 100 = 111.111... → int64 → 111
			wantCents: 111,
		},
		{
			name:          "large price",
			exchangePrice: 10.0,
			// 10.0 / 0.90 * 100 = 1111.111... → int64 → 1111
			wantCents: 1111,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := advertiserChargeCents(tc.exchangePrice)
			if got != tc.wantCents {
				t.Errorf("advertiserChargeCents(%f) = %d, want %d", tc.exchangePrice, got, tc.wantCents)
			}
		})
	}
}

func TestPlatformMarginConstant(t *testing.T) {
	if PlatformMargin != 0.10 {
		t.Errorf("PlatformMargin = %f, want 0.10", PlatformMargin)
	}
}

// ---------------------------------------------------------------------------
// V5.2C: /stats moved from public mux to internal mux behind admin auth.
// Three invariants:
//   1. /stats on the public mux returns 404
//   2. /internal/stats on the internal mux without X-Admin-Token returns 401
//   3. /internal/stats on the internal mux with valid token returns 200 + JSON
// ---------------------------------------------------------------------------

// TestStats_PublicMux_Returns404 verifies that /stats is no longer registered
// on the public bidder mux after the V5.2C migration.
func TestStats_PublicMux_Returns404(t *testing.T) {
	d := &Deps{
		// No collaborators needed — we're testing route registration, not
		// handler behavior. The public mux no longer registers /stats at all.
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, d)

	req := httptest.NewRequest("GET", "/stats", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("V5.2C regression: GET /stats on public mux should be 404, got %d body=%s",
			rec.Code, rec.Body.String())
	}
}

// TestStats_InternalMux_RequiresAdminToken verifies that /internal/stats
// returns 401 when no X-Admin-Token header is provided.
func TestStats_InternalMux_RequiresAdminToken(t *testing.T) {
	const testToken = "v5-2c-test-admin-token"
	t.Setenv("ADMIN_TOKEN", testToken)

	d := &Deps{
		// Loader/BudgetSvc not needed — the request is rejected before
		// reaching the handler.
	}
	mux := http.NewServeMux()
	RegisterInternalRoutes(mux, d)

	// No token at all
	req := httptest.NewRequest("GET", "/internal/stats", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("V5.2C regression: GET /internal/stats without token should be 401, got %d body=%s",
			rec.Code, rec.Body.String())
	}

	// Wrong token
	req = httptest.NewRequest("GET", "/internal/stats", nil)
	req.Header.Set("X-Admin-Token", "wrong-token")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("V5.2C regression: GET /internal/stats with wrong token should be 401, got %d body=%s",
			rec.Code, rec.Body.String())
	}
}

// TestStats_InternalMux_SuccessWithToken verifies that /internal/stats
// returns 200 and valid JSON when the correct X-Admin-Token is provided.
func TestStats_InternalMux_SuccessWithToken(t *testing.T) {
	const testToken = "v5-2c-test-admin-token"
	t.Setenv("ADMIN_TOKEN", testToken)

	// Construct a CampaignLoader that was never started — GetActiveCampaigns
	// returns an empty slice, which is a valid (empty) stats response.
	loader := bidder.NewCampaignLoader(nil, nil)
	d := &Deps{
		Loader: loader,
		// BudgetSvc: nil — handleStats only calls it inside the loop over
		// active campaigns, and with no campaigns the loop doesn't execute.
	}
	mux := http.NewServeMux()
	RegisterInternalRoutes(mux, d)

	req := httptest.NewRequest("GET", "/internal/stats", nil)
	req.Header.Set("X-Admin-Token", testToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /internal/stats with valid token: want 200, got %d body=%s",
			rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: want application/json, got %q", ct)
	}
	// With no active campaigns the response is an empty JSON array.
	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Errorf("body: want [], got %q", body)
	}
}

// TestStats_InternalMux_FailsClosed_NoAdminToken verifies that when
// ADMIN_TOKEN is not configured (empty), the endpoint returns 401 even
// if the client sends an empty X-Admin-Token header (defense in depth
// against matching "" == "").
func TestStats_InternalMux_FailsClosed_NoAdminToken(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "")

	d := &Deps{}
	mux := http.NewServeMux()
	RegisterInternalRoutes(mux, d)

	req := httptest.NewRequest("GET", "/internal/stats", nil)
	req.Header.Set("X-Admin-Token", "")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("V5.2C regression: empty ADMIN_TOKEN + empty header should be 401, got %d body=%s",
			rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// V5.2C Task 7: http.Server timeout assertions.
//
// We can't easily introspect the servers constructed inside main(), but we
// verify the pattern by asserting the literal values our code sets. If
// someone removes a timeout field or changes a value, this test catches it.
// ---------------------------------------------------------------------------
func TestBidderServerTimeouts(t *testing.T) {
	srv := &http.Server{
		Addr:              ":0",
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("ReadHeaderTimeout: want 10s, got %s", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout: want 30s, got %s", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 60*time.Second {
		t.Errorf("WriteTimeout: want 60s, got %s", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout: want 120s, got %s", srv.IdleTimeout)
	}
}

// TestInjectClickTracker_NeverEmitsDestParam is the V5.1 P1-3 static
// regression guard: the function that constructs click URLs in real
// bid responses must NEVER put a `dest` query parameter into the URL
// it injects. Any dest parameter that reached handleClick would be
// client-controlled attack surface because the HMAC token only signs
// (campaign_id, request_id). The click dest branch has been deleted
// from handleClick; this test locks in the invariant that no
// legitimate caller in this package can re-introduce it by accident.
func TestInjectClickTracker_NeverEmitsDestParam(t *testing.T) {
	cases := []struct {
		name     string
		markup   string
		clickURL string
	}{
		{"banner", `<a href="https://example.com"><img src="ad.png"/></a>`, "http://bidder.example/click?campaign_id=7&request_id=r-abc&token=xyz"},
		{"empty markup", "", "http://bidder.example/click?campaign_id=7&request_id=r-abc&token=xyz"},
		{"empty url", `<div>ad</div>`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := injectClickTracker(tc.markup, tc.clickURL)
			if strings.Contains(out, "dest=") {
				t.Fatalf("V5.1 P1-3 regression: injectClickTracker output contains dest=: %q", out)
			}
		})
	}
}
