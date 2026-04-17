package auth

import (
	"context"
	"net/http"
	"strings"
)

// extractBearer extracts the token from an "Authorization: Bearer <token>" header.
// Returns empty string if the header is missing or malformed.
func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// TenantAuthMiddleware authenticates requests for public tenant routes.
//
// Authentication order:
//  1. Try JWT Bearer token first
//     - Valid JWT with role=advertiser: inject WithAdvertiser + WithUser
//     - Valid JWT with role=platform_admin: 403 "advertiser access required"
//     - Valid JWT + API Key with different advertiser_id: 400 "credential conflict"
//     - Valid JWT admin + API Key: 400 "credential conflict"
//  2. Fall back to API Key (existing APIKeyMiddleware logic)
//  3. Both missing: 401
//
// When JWT validation fails (expired, malformed, wrong key), the middleware
// falls back to API Key auth silently. This ensures existing API Key clients
// continue working even if a stale JWT is accidentally sent.
func TenantAuthMiddleware(jwtSecret []byte, apiKeyLookup APIKeyLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bearer := extractBearer(r)
			apiKey := r.Header.Get("X-API-Key")

			// Path 1: Try JWT
			if bearer != "" {
				claims, err := ValidateJWT(bearer, jwtSecret)
				if err == nil {
					// Valid JWT — check for credential conflicts with API Key
					if apiKey != "" {
						if claims.Role == RolePlatformAdmin {
							// Admin + API Key = conflict (admin has no tenant)
							writeAuthError(w, http.StatusBadRequest, "credential conflict: admin JWT and API Key cannot be combined")
							return
						}
						// Advertiser JWT + API Key: check same tenant
						id, _, _, lookupErr := apiKeyLookup(r.Context(), apiKey)
						if lookupErr == nil && id != claims.AdvertiserID {
							writeAuthError(w, http.StatusBadRequest, "credential conflict: JWT and API Key belong to different advertisers")
							return
						}
						// Same tenant or invalid API Key — JWT wins
					}

					// Role gate: platform_admin cannot access tenant routes,
					// EXCEPT /api/v1/auth/* (me, change-password) which are
					// role-agnostic endpoints that both admin and advertiser need.
					if claims.Role == RolePlatformAdmin && !strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
						writeAuthError(w, http.StatusForbidden, "advertiser access required")
						return
					}

					// Valid advertiser JWT — inject both Advertiser and User context
					adv := &Advertiser{ID: claims.AdvertiserID}
					ctx := WithAdvertiser(r.Context(), adv)
					ctx = WithUser(ctx, &User{
						ID:           claims.UserID,
						Email:        claims.Email,
						Role:         claims.Role,
						AdvertiserID: claims.AdvertiserID,
					})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				// JWT validation failed — fall through to API Key
			}

			// Path 2: Try API Key
			if apiKey != "" {
				id, name, email, err := apiKeyLookup(r.Context(), apiKey)
				if err != nil {
					writeAuthError(w, http.StatusUnauthorized, "invalid API key")
					return
				}
				adv := &Advertiser{ID: id, CompanyName: name, ContactEmail: email}
				ctx := context.WithValue(r.Context(), advertiserKey, adv)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Path 3: No credentials
			writeAuthError(w, http.StatusUnauthorized, "authentication required")
		})
	}
}

// HumanAdminAuthMiddleware authenticates requests for /api/v1/admin/* routes.
//
// Authentication order:
//  1. Try JWT Bearer token: must be role=platform_admin
//     - role=advertiser: 403 "platform admin required"
//  2. Fall back to X-Admin-Token (service compatibility during migration)
//  3. Both missing: 401
func HumanAdminAuthMiddleware(jwtSecret []byte, adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// CORS preflight must bypass auth
			if r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			// Path 1: Try JWT
			if bearer := extractBearer(r); bearer != "" {
				claims, err := ValidateJWT(bearer, jwtSecret)
				if err == nil {
					if claims.Role != RolePlatformAdmin {
						writeAuthError(w, http.StatusForbidden, "platform admin required")
						return
					}
					ctx := WithUser(r.Context(), &User{
						ID:    claims.UserID,
						Email: claims.Email,
						Role:  claims.Role,
					})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				// Invalid JWT — fall through to X-Admin-Token
			}

			// Path 2: Try X-Admin-Token (backward compat)
			if adminToken != "" && r.Header.Get("X-Admin-Token") == adminToken {
				next.ServeHTTP(w, r)
				return
			}

			writeAuthError(w, http.StatusUnauthorized, "admin authentication required")
		})
	}
}

// ServiceAuthMiddleware authenticates requests for /internal/* routes.
//
// X-Admin-Token ONLY. JWT is explicitly NOT accepted to prevent browser
// sessions from reaching service-to-service endpoints.
func ServiceAuthMiddleware(adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// CORS preflight must bypass auth
			if r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			// Defense in depth: if adminToken is not configured, fail closed
			if adminToken == "" {
				writeAuthError(w, http.StatusUnauthorized, "service authentication not configured")
				return
			}

			if r.Header.Get("X-Admin-Token") != adminToken {
				writeAuthError(w, http.StatusUnauthorized, "service authentication required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
