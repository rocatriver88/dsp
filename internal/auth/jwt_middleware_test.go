package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// testJWTSecret is a 32+ byte secret used by all middleware tests.
var testJWTSecret = []byte("test-jwt-secret-at-least-32bytes!!")

// stubLookup returns a fixed advertiser for the given API key.
func stubLookup(validKey string, advID int64) APIKeyLookup {
	return func(ctx context.Context, key string) (int64, string, string, error) {
		if key == validKey {
			return advID, "Test Corp", "test@example.com", nil
		}
		return 0, "", "", fmt.Errorf("not found")
	}
}

// okHandler is a simple handler that returns 200.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// issueTestJWT is a test helper that issues a JWT with the given claims.
func issueTestJWT(t *testing.T, userID int64, email, role string, aid int64) string {
	t.Helper()
	token, err := issueJWT(testJWTSecret, userID, email, role, aid, 15*time.Minute)
	if err != nil {
		t.Fatalf("issueJWT: %v", err)
	}
	return token
}

// issueExpiredJWT is a test helper that issues an already-expired JWT.
func issueExpiredJWT(t *testing.T, userID int64, email, role string, aid int64) string {
	t.Helper()
	token, err := issueJWT(testJWTSecret, userID, email, role, aid, -1*time.Hour)
	if err != nil {
		t.Fatalf("issueJWT: %v", err)
	}
	return token
}

// --- TenantAuthMiddleware tests ---

func TestTenantAuth_JWTAdvertiser_SetsContexts(t *testing.T) {
	// Test case 1: JWT advertiser -> AdvertiserFromContext + UserFromContext set
	token := issueTestJWT(t, 1, "user@test.com", RoleAdvertiser, 42)

	var gotAdv *Advertiser
	var gotUser *User
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAdv = AdvertiserFromContext(r.Context())
		gotUser = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	h := TenantAuthMiddleware(testJWTSecret, stubLookup("key", 99))(inner)
	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotAdv == nil || gotAdv.ID != 42 {
		t.Errorf("expected advertiser ID 42, got %v", gotAdv)
	}
	if gotUser == nil {
		t.Fatal("expected user in context")
	}
	if gotUser.ID != 1 {
		t.Errorf("expected user ID 1, got %d", gotUser.ID)
	}
	if gotUser.Email != "user@test.com" {
		t.Errorf("expected email user@test.com, got %s", gotUser.Email)
	}
	if gotUser.Role != RoleAdvertiser {
		t.Errorf("expected role advertiser, got %s", gotUser.Role)
	}
	if gotUser.AdvertiserID != 42 {
		t.Errorf("expected user advertiser ID 42, got %d", gotUser.AdvertiserID)
	}
}

func TestTenantAuth_JWTAdmin_403(t *testing.T) {
	// Test case 8: JWT admin on tenant route -> 403 "advertiser access required"
	token := issueTestJWT(t, 1, "admin@test.com", RolePlatformAdmin, 0)

	h := TenantAuthMiddleware(testJWTSecret, stubLookup("key", 99))(okHandler)
	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestTenantAuth_APIKeyOnly_SetsAdvertiser_NoUser(t *testing.T) {
	// Test case 3: API Key only -> AdvertiserFromContext set, UserFromContext nil
	var gotAdv *Advertiser
	var gotUser *User
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAdv = AdvertiserFromContext(r.Context())
		gotUser = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	h := TenantAuthMiddleware(testJWTSecret, stubLookup("dsp_valid_key", 42))(inner)
	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	req.Header.Set("X-API-Key", "dsp_valid_key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotAdv == nil || gotAdv.ID != 42 {
		t.Errorf("expected advertiser ID 42, got %v", gotAdv)
	}
	if gotUser != nil {
		t.Errorf("expected nil user for API key auth, got %v", gotUser)
	}
}

func TestTenantAuth_ExpiredJWT_FallbackToAPIKey(t *testing.T) {
	// Test case 4: Expired JWT + valid API Key -> fallback to API Key
	expiredToken := issueExpiredJWT(t, 1, "user@test.com", RoleAdvertiser, 42)

	var gotAdv *Advertiser
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAdv = AdvertiserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	h := TenantAuthMiddleware(testJWTSecret, stubLookup("dsp_valid_key", 99))(inner)
	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	req.Header.Set("X-API-Key", "dsp_valid_key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotAdv == nil || gotAdv.ID != 99 {
		t.Errorf("expected advertiser ID 99 (from API key fallback), got %v", gotAdv)
	}
}

func TestTenantAuth_JWTAdvertiser_APIKey_SameTenant_JWTWins(t *testing.T) {
	// Test case 5: JWT advertiser + API Key same tenant -> JWT wins
	token := issueTestJWT(t, 1, "user@test.com", RoleAdvertiser, 42)

	var gotAdv *Advertiser
	var gotUser *User
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAdv = AdvertiserFromContext(r.Context())
		gotUser = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	h := TenantAuthMiddleware(testJWTSecret, stubLookup("dsp_valid_key", 42))(inner)
	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-API-Key", "dsp_valid_key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotAdv == nil || gotAdv.ID != 42 {
		t.Errorf("expected advertiser ID 42 (from JWT), got %v", gotAdv)
	}
	if gotUser == nil || gotUser.ID != 1 {
		t.Errorf("expected user from JWT, got %v", gotUser)
	}
}

func TestTenantAuth_JWTAdvertiser_APIKey_DifferentTenant_400(t *testing.T) {
	// Test case 6: JWT advertiser + API Key different tenant -> 400 "credential conflict"
	token := issueTestJWT(t, 1, "user@test.com", RoleAdvertiser, 42)

	h := TenantAuthMiddleware(testJWTSecret, stubLookup("dsp_valid_key", 99))(okHandler)
	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-API-Key", "dsp_valid_key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestTenantAuth_JWTAdmin_APIKey_400(t *testing.T) {
	// Test case 7: JWT admin + API Key -> 400 "credential conflict"
	token := issueTestJWT(t, 1, "admin@test.com", RolePlatformAdmin, 0)

	h := TenantAuthMiddleware(testJWTSecret, stubLookup("dsp_valid_key", 99))(okHandler)
	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-API-Key", "dsp_valid_key")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestTenantAuth_MalformedJWT_NoAPIKey_401(t *testing.T) {
	// Test case 10: Malformed JWT + no API Key -> 401
	h := TenantAuthMiddleware(testJWTSecret, stubLookup("key", 99))(okHandler)
	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	req.Header.Set("Authorization", "Bearer not.a.real.jwt")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestTenantAuth_NoCredentials_401(t *testing.T) {
	// No JWT, no API Key -> 401
	h := TenantAuthMiddleware(testJWTSecret, stubLookup("key", 99))(okHandler)
	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// --- HumanAdminAuthMiddleware tests ---

func TestHumanAdminAuth_JWTAdmin_SetsUserContext(t *testing.T) {
	// Test case 2: JWT admin -> UserFromContext set, no AdvertiserFromContext
	token := issueTestJWT(t, 1, "admin@test.com", RolePlatformAdmin, 0)

	var gotUser *User
	var gotAdv *Advertiser
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = UserFromContext(r.Context())
		gotAdv = AdvertiserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	h := HumanAdminAuthMiddleware(testJWTSecret, "test-admin-token")(inner)
	req := httptest.NewRequest("GET", "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotUser == nil {
		t.Fatal("expected user in context")
	}
	if gotUser.ID != 1 {
		t.Errorf("expected user ID 1, got %d", gotUser.ID)
	}
	if gotUser.Role != RolePlatformAdmin {
		t.Errorf("expected role platform_admin, got %s", gotUser.Role)
	}
	if gotAdv != nil {
		t.Errorf("expected no advertiser in context for admin, got %v", gotAdv)
	}
}

func TestHumanAdminAuth_JWTAdvertiser_403(t *testing.T) {
	// Test case 9: JWT advertiser on admin route -> 403 "platform admin required"
	token := issueTestJWT(t, 1, "user@test.com", RoleAdvertiser, 42)

	h := HumanAdminAuthMiddleware(testJWTSecret, "test-admin-token")(okHandler)
	req := httptest.NewRequest("GET", "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestHumanAdminAuth_AdminToken_OK(t *testing.T) {
	// X-Admin-Token backward compat
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	h := HumanAdminAuthMiddleware(testJWTSecret, "test-admin-token")(inner)
	req := httptest.NewRequest("GET", "/api/v1/admin/users", nil)
	req.Header.Set("X-Admin-Token", "test-admin-token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestHumanAdminAuth_NoCredentials_401(t *testing.T) {
	h := HumanAdminAuthMiddleware(testJWTSecret, "test-admin-token")(okHandler)
	req := httptest.NewRequest("GET", "/api/v1/admin/users", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHumanAdminAuth_OPTIONS_Bypass(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	h := HumanAdminAuthMiddleware(testJWTSecret, "test-admin-token")(inner)
	req := httptest.NewRequest("OPTIONS", "/api/v1/admin/users", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called {
		t.Error("OPTIONS should bypass auth")
	}
}

// --- ServiceAuthMiddleware tests ---

func TestServiceAuth_AdminToken_OK(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	h := ServiceAuthMiddleware("test-admin-token")(inner)
	req := httptest.NewRequest("GET", "/internal/active-campaigns", nil)
	req.Header.Set("X-Admin-Token", "test-admin-token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestServiceAuth_JWTNotAccepted(t *testing.T) {
	// JWT is explicitly NOT accepted on service routes
	token := issueTestJWT(t, 1, "admin@test.com", RolePlatformAdmin, 0)

	h := ServiceAuthMiddleware("test-admin-token")(okHandler)
	req := httptest.NewRequest("GET", "/internal/active-campaigns", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 (JWT not accepted for service routes), got %d", rr.Code)
	}
}

func TestServiceAuth_NoCredentials_401(t *testing.T) {
	h := ServiceAuthMiddleware("test-admin-token")(okHandler)
	req := httptest.NewRequest("GET", "/internal/active-campaigns", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestServiceAuth_EmptyToken_FailClosed(t *testing.T) {
	// If admin token is not configured, fail closed
	h := ServiceAuthMiddleware("")(okHandler)
	req := httptest.NewRequest("GET", "/internal/active-campaigns", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty admin token config, got %d", rr.Code)
	}
}

func TestServiceAuth_OPTIONS_Bypass(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	h := ServiceAuthMiddleware("test-admin-token")(inner)
	req := httptest.NewRequest("OPTIONS", "/internal/active-campaigns", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called {
		t.Error("OPTIONS should bypass auth")
	}
}

// --- extractBearer tests ---

func TestExtractBearer_Valid(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer mytoken123")
	got := extractBearer(req)
	if got != "mytoken123" {
		t.Errorf("expected mytoken123, got %q", got)
	}
}

func TestExtractBearer_Missing(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	got := extractBearer(req)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractBearer_CaseInsensitive(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "bearer mytoken123")
	got := extractBearer(req)
	if got != "mytoken123" {
		t.Errorf("expected mytoken123, got %q", got)
	}
}

func TestExtractBearer_WrongScheme(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	got := extractBearer(req)
	if got != "" {
		t.Errorf("expected empty for Basic scheme, got %q", got)
	}
}
