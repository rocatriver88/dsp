package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// helper: build a test admin middleware with the inner handler returning 200.
func makeAdminAuth(t *testing.T, token string) http.Handler {
	t.Helper()
	t.Setenv("ADMIN_TOKEN", token)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return AdminAuthMiddleware(inner)
}

func TestAdminAuth_HeaderMatchPassesThrough(t *testing.T) {
	h := makeAdminAuth(t, "real-admin-token")

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.Header.Set("X-Admin-Token", "real-admin-token")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for matching X-Admin-Token, got %d", w.Code)
	}
}

func TestAdminAuth_QueryParamRejected(t *testing.T) {
	h := makeAdminAuth(t, "real-admin-token")

	// Legacy behavior allowed this via ?admin_token=. V5 removes it —
	// URL-based tokens leak into logs and referrers.
	req := httptest.NewRequest(http.MethodGet, "/admin/ping?admin_token=real-admin-token", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for query-param token (should no longer be honored), got %d", w.Code)
	}
}

func TestAdminAuth_NoTokenRejected(t *testing.T) {
	h := makeAdminAuth(t, "real-admin-token")

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when no token is sent, got %d", w.Code)
	}
}

func TestAdminAuth_WrongTokenRejected(t *testing.T) {
	h := makeAdminAuth(t, "real-admin-token")

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.Header.Set("X-Admin-Token", "guessed-wrong")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong token, got %d", w.Code)
	}
}

// TestAdminAuth_DefaultFallbackRejected guards the exact regression the V5
// fix targets: previously the middleware silently accepted the literal
// string "admin-secret" when ADMIN_TOKEN was unset. With the fallback
// removed the middleware must refuse the request even if the client
// sends that exact string.
func TestAdminAuth_DefaultFallbackRejected(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "")
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := AdminAuthMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.Header.Set("X-Admin-Token", "admin-secret")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when ADMIN_TOKEN unset — default fallback must be gone — got %d", w.Code)
	}
}

// TestAdminAuth_EmptyTokenEmptyHeaderRejected covers a subtle regression
// where an unset ADMIN_TOKEN plus an empty X-Admin-Token header would make
// `auth != token` evaluate to `"" != ""` (false) and let the request
// through. The "token == empty → reject" guard at the top of the handler
// must short-circuit before that comparison.
func TestAdminAuth_EmptyTokenEmptyHeaderRejected(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "")
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := AdminAuthMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	// no X-Admin-Token header at all
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty-token / empty-header combination, got %d", w.Code)
	}
}

func TestAdminAuth_OptionsBypassesAuth(t *testing.T) {
	h := makeAdminAuth(t, "real-admin-token")

	req := httptest.NewRequest(http.MethodOptions, "/admin/ping", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for OPTIONS preflight (CORS bypass), got %d", w.Code)
	}
}
