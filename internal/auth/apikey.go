package auth

import (
	"context"
	"encoding/json"
	"net/http"
)

type contextKey string

const (
	advertiserKey contextKey = "advertiser"
	userKey       contextKey = "user"
)

// User is the authenticated user info stored in request context.
// Set by JWT middleware when a valid JWT is present.
// UserFromContext returns nil for API Key-only requests.
type User struct {
	ID           int64
	Email        string
	Role         string
	AdvertiserID int64 // 0 for platform_admin
}

// WithUser injects a User into the context.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// UserFromContext extracts the authenticated user from the request context.
// Returns nil if no user is set (API Key auth or unauthenticated request).
func UserFromContext(ctx context.Context) *User {
	u, _ := ctx.Value(userKey).(*User)
	return u
}

// Advertiser is the minimal advertiser info stored in request context.
type Advertiser struct {
	ID           int64
	CompanyName  string
	ContactEmail string
}

// AdvertiserFromContext extracts the authenticated advertiser from the request context.
// Returns nil if no advertiser is set (unauthenticated request).
func AdvertiserFromContext(ctx context.Context) *Advertiser {
	adv, _ := ctx.Value(advertiserKey).(*Advertiser)
	return adv
}

// AdvertiserIDFromContext returns the authenticated advertiser ID, or 0 if unauthenticated.
func AdvertiserIDFromContext(ctx context.Context) int64 {
	if adv := AdvertiserFromContext(ctx); adv != nil {
		return adv.ID
	}
	return 0
}

// WithAdvertiser injects an Advertiser into the context.
func WithAdvertiser(ctx context.Context, adv *Advertiser) context.Context {
	return context.WithValue(ctx, advertiserKey, adv)
}

// WithAdvertiserForTest injects a minimal Advertiser into the context for tests.
// Production code must not use this helper — real requests get their advertiser
// via APIKeyMiddleware after a successful key lookup.
func WithAdvertiserForTest(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, advertiserKey, &Advertiser{ID: id})
}

// APIKeyLookup is the function signature for looking up an advertiser by API key.
type APIKeyLookup func(ctx context.Context, key string) (id int64, companyName, email string, err error)

// APIKeyMiddleware returns middleware that validates X-API-Key header.
// Requests without a valid key get 401. The lookup function queries the database.
func APIKeyMiddleware(lookup APIKeyLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				writeAuthError(w, http.StatusUnauthorized, "missing X-API-Key header")
				return
			}

			id, name, email, err := lookup(r.Context(), key)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "invalid API key")
				return
			}

			adv := &Advertiser{ID: id, CompanyName: name, ContactEmail: email}
			ctx := context.WithValue(r.Context(), advertiserKey, adv)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeAuthError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
