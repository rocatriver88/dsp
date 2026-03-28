package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiter_NilRedis(t *testing.T) {
	// Nil Redis = fail-open, always allow
	l := New(nil)
	for i := 0; i < 200; i++ {
		if !l.Allow(context.Background(), "test", 10, time.Minute) {
			t.Fatal("nil Redis should always allow")
		}
	}
}

func TestMiddleware_AllowsNormalTraffic(t *testing.T) {
	// With nil Redis (fail-open), middleware should pass through
	l := New(nil)
	handler := Middleware(l, IPKeyFunc, 100, time.Minute)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAPIKeyFunc_WithKey(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "dsp_abc123")
	key := APIKeyFunc(req)
	if key != "key:dsp_abc123" {
		t.Errorf("expected key:dsp_abc123, got %s", key)
	}
}

func TestAPIKeyFunc_WithoutKey(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	key := APIKeyFunc(req)
	if key != "ip:1.2.3.4:5678" {
		t.Errorf("expected ip:1.2.3.4:5678, got %s", key)
	}
}
