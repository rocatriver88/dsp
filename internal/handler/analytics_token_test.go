package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
)

func TestHandleAnalyticsToken_ReturnsValidToken(t *testing.T) {
	secret := []byte("test-secret-long-enough-for-hmac-32chars-min")
	d := &Deps{SSETokenSecret: secret}

	req := httptest.NewRequest("POST", "/api/v1/analytics/token", nil)
	ctx := auth.WithAdvertiserForTest(req.Context(), 77)
	req = req.WithContext(ctx)

	// Sample now ONCE before the handler runs, and use it for all bounds
	// checks below. Sampling multiple times across a second-boundary can
	// produce a 1s skew that the ±60s window would absorb, but it's still
	// sloppy — hoist it per V5.1 Task 4 code review Important #7.
	before := time.Now()
	rec := httptest.NewRecorder()
	d.HandleAnalyticsToken(rec, req)
	after := time.Now()

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp AnalyticsTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("expected non-empty token")
	}
	// expires_at must be within [now+5m-1s, now+5m+1s] relative to the
	// sample window around the handler call. The handler mints a token
	// with exactly 5-minute TTL from the now it samples internally.
	const ttl = 5 * time.Minute
	if resp.ExpiresAt.Before(before.Add(ttl).Add(-1 * time.Second)) {
		t.Errorf("expires_at too early: %s (expected ≥ %s)", resp.ExpiresAt, before.Add(ttl))
	}
	if resp.ExpiresAt.After(after.Add(ttl).Add(1 * time.Second)) {
		t.Errorf("expires_at too late: %s (expected ≤ %s)", resp.ExpiresAt, after.Add(ttl))
	}
	// Validate the returned token round-trips through ValidateSSEToken
	// and decodes back to the same advertiser id.
	advID, err := auth.ValidateSSEToken(secret, resp.Token, time.Now())
	if err != nil {
		t.Fatalf("returned token fails its own validation: %v", err)
	}
	if advID != 77 {
		t.Fatalf("expected advID 77, got %d", advID)
	}
}

func TestHandleAnalyticsToken_Unauthenticated_Returns401(t *testing.T) {
	d := &Deps{SSETokenSecret: []byte("test-secret-long-enough-for-hmac-32chars-min")}
	req := httptest.NewRequest("POST", "/api/v1/analytics/token", nil)
	rec := httptest.NewRecorder()
	d.HandleAnalyticsToken(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without context advertiser, got %d", rec.Code)
	}
}

func TestHandleAnalyticsToken_MissingSecret_Returns500(t *testing.T) {
	d := &Deps{} // no SSETokenSecret configured
	req := httptest.NewRequest("POST", "/api/v1/analytics/token", nil)
	ctx := auth.WithAdvertiserForTest(req.Context(), 77)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	d.HandleAnalyticsToken(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 without configured secret, got %d", rec.Code)
	}
}
