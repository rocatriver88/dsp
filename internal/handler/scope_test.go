package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heartgryphon/dsp/internal/auth"
)

func withAdvertiser(r *http.Request, id int64) *http.Request {
	ctx := auth.WithAdvertiserForTest(r.Context(), id)
	return r.WithContext(ctx)
}

func TestEnsureSelfAccess_MatchingIDReturnsTrue(t *testing.T) {
	req := withAdvertiser(httptest.NewRequest(http.MethodGet, "/test", nil), 42)
	w := httptest.NewRecorder()

	if !ensureSelfAccess(w, req, 42) {
		t.Fatalf("expected true when auth ID matches path ID")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected no body write on success, got status %d", w.Code)
	}
}

func TestEnsureSelfAccess_MismatchedIDWrites404(t *testing.T) {
	req := withAdvertiser(httptest.NewRequest(http.MethodGet, "/test", nil), 42)
	w := httptest.NewRecorder()

	if ensureSelfAccess(w, req, 99) {
		t.Fatalf("expected false when auth ID differs from path ID")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected error message in body")
	}
}

func TestEnsureSelfAccess_NoAuthContextWrites401(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// Intentionally do not attach any advertiser to context.
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	if ensureSelfAccess(w, req, 1) {
		t.Fatalf("expected false when no advertiser in context")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 (V5 three-code rule), got %d", w.Code)
	}
}

func TestRequireAuth_WithAdvertiserReturnsID(t *testing.T) {
	req := withAdvertiser(httptest.NewRequest(http.MethodGet, "/test", nil), 42)
	w := httptest.NewRecorder()

	id, ok := requireAuth(w, req)
	if !ok {
		t.Fatalf("expected true when advertiser in context")
	}
	if id != 42 {
		t.Errorf("expected id 42, got %d", id)
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected no body write on success, got %d", w.Code)
	}
}

func TestRequireAuth_NoContextWrites401(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	id, ok := requireAuth(w, req)
	if ok {
		t.Fatalf("expected false when no advertiser in context")
	}
	if id != 0 {
		t.Errorf("expected zero id, got %d", id)
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
