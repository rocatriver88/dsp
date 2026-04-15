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

// Regression test: the V5.1 P1-1 hotfix is explicitly about removing
// credential-in-URL-query support. The middleware must NOT accept
// X-API-Key via query param as a fallback — even a "convenient" fallback
// would defeat the purpose of the fix.
func TestSSETokenMiddleware_RejectsQueryApiKey(t *testing.T) {
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
