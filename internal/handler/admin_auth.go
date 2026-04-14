package handler

import (
	"net/http"
	"os"
)

// AdminAuthMiddleware provides token-based authentication for admin endpoints.
// The admin token is set via the ADMIN_TOKEN env var. Config.Validate()
// ensures ADMIN_TOKEN is set in production; the middleware refuses to
// authorize any request when the token is empty regardless of environment
// (defense in depth against startup misconfiguration or test harnesses
// that forget to set the var).
//
// V5 §P0 three-code rule:
//   - missing or wrong credentials → 401 Unauthorized
//
// The legacy "admin_token" query parameter is deliberately not supported:
// URL parameters leak into access logs, reverse-proxy logs, and browser
// history/referrer. Use the X-Admin-Token header exclusively.
func AdminAuthMiddleware(next http.Handler) http.Handler {
	token := os.Getenv("ADMIN_TOKEN")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS preflight must bypass auth so browsers can discover allowed
		// methods before attaching credentials.
		if r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}

		// Defense in depth: if ADMIN_TOKEN is not configured, fail closed
		// regardless of what the client sends. Config.Validate should have
		// caught this at startup in production, but we never trust a
		// happy path that depends on external ordering.
		if token == "" {
			WriteError(w, http.StatusUnauthorized, "admin authentication not configured")
			return
		}

		auth := r.Header.Get("X-Admin-Token")
		if auth != token {
			WriteError(w, http.StatusUnauthorized, "admin authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
