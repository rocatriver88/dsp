package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

	// Must start with "key:" prefix
	if !strings.HasPrefix(key, "key:") {
		t.Fatalf("expected key: prefix, got %s", key)
	}

	// Must NOT contain the plaintext API key
	if strings.Contains(key, "dsp_abc123") {
		t.Fatal("Redis key must not contain plaintext API key")
	}

	// The hash portion must be exactly 16 hex chars
	hash := strings.TrimPrefix(key, "key:")
	if len(hash) != 16 {
		t.Errorf("expected 16-char hash, got %d chars: %s", len(hash), hash)
	}
}

func TestHashKey_Deterministic(t *testing.T) {
	// Same input must always produce the same hash
	a := hashKey("dsp_abc123")
	b := hashKey("dsp_abc123")
	if a != b {
		t.Errorf("hashKey not deterministic: %s != %s", a, b)
	}
}

func TestHashKey_DifferentKeysProduceDifferentHashes(t *testing.T) {
	a := hashKey("dsp_abc123")
	b := hashKey("dsp_xyz789")
	if a == b {
		t.Errorf("different API keys produced same hash: %s", a)
	}
}

func TestHashKey_Length(t *testing.T) {
	h := hashKey("anything")
	if len(h) != 16 {
		t.Errorf("expected 16 hex chars, got %d: %s", len(h), h)
	}
	// Verify all chars are valid hex
	for _, c := range h {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("non-hex char in hash: %c", c)
		}
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
