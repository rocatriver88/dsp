package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIKeyMiddleware_ValidKey(t *testing.T) {
	lookup := func(ctx context.Context, key string) (int64, string, string, error) {
		if key == "dsp_valid_key" {
			return 42, "Test Corp", "test@example.com", nil
		}
		return 0, "", "", fmt.Errorf("not found")
	}

	handler := APIKeyMiddleware(lookup)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adv := AdvertiserFromContext(r.Context())
		if adv == nil {
			t.Fatal("expected advertiser in context")
		}
		if adv.ID != 42 {
			t.Errorf("expected advertiser ID 42, got %d", adv.ID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	req.Header.Set("X-API-Key", "dsp_valid_key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAPIKeyMiddleware_MissingKey(t *testing.T) {
	lookup := func(ctx context.Context, key string) (int64, string, string, error) {
		return 0, "", "", fmt.Errorf("not found")
	}

	handler := APIKeyMiddleware(lookup)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAPIKeyMiddleware_InvalidKey(t *testing.T) {
	lookup := func(ctx context.Context, key string) (int64, string, string, error) {
		return 0, "", "", fmt.Errorf("not found")
	}

	handler := APIKeyMiddleware(lookup)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	req.Header.Set("X-API-Key", "dsp_invalid")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAdvertiserIDFromContext_NoAuth(t *testing.T) {
	id := AdvertiserIDFromContext(context.Background())
	if id != 0 {
		t.Errorf("expected 0 for unauthenticated context, got %d", id)
	}
}

func TestAdvertiserFromContext_WithAuth(t *testing.T) {
	adv := &Advertiser{ID: 99, CompanyName: "Test"}
	ctx := context.WithValue(context.Background(), advertiserKey, adv)
	got := AdvertiserFromContext(ctx)
	if got == nil || got.ID != 99 {
		t.Errorf("expected advertiser ID 99, got %v", got)
	}
}
