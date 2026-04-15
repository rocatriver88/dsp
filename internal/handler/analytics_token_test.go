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
	secret := []byte("test-secret-long-enough-for-hmac-32chars-min")
	d := &Deps{SSETokenSecret: secret}

	req := httptest.NewRequest("POST", "/api/v1/analytics/token", nil)
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
	// Returned expires_at should be ~5 minutes in the future
	if resp.ExpiresAt < time.Now().Unix()+4*60 || resp.ExpiresAt > time.Now().Unix()+6*60 {
		t.Errorf("expected expires_at ~5min from now, got delta %d", resp.ExpiresAt-time.Now().Unix())
	}
	// Validate the returned token round-trips
	advID, err := auth.ValidateSSEToken(secret, resp.Token, time.Now())
	if err != nil {
		t.Fatalf("returned token fails its own validation: %v", err)
	}
	if advID != 77 {
		t.Fatalf("expected advID 77, got %d", advID)
	}
}

func TestHandleAnalyticsStreamToken_Unauthenticated_Returns401(t *testing.T) {
	d := &Deps{SSETokenSecret: []byte("test-secret-long-enough-for-hmac-32chars-min")}
	req := httptest.NewRequest("POST", "/api/v1/analytics/token", nil)
	rec := httptest.NewRecorder()
	d.HandleAnalyticsStreamToken(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without context advertiser, got %d", rec.Code)
	}
}

func TestHandleAnalyticsStreamToken_MissingSecret_Returns500(t *testing.T) {
	d := &Deps{} // no SSETokenSecret configured
	req := httptest.NewRequest("POST", "/api/v1/analytics/token", nil)
	ctx := auth.WithAdvertiserForTest(req.Context(), 77)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	d.HandleAnalyticsStreamToken(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 without configured secret, got %d", rec.Code)
	}
}
