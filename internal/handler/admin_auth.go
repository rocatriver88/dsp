package handler

import (
	"net/http"
	"os"
)

// AdminAuthMiddleware provides a simple token-based authentication for admin endpoints.
// The admin token is set via ADMIN_TOKEN env var. This is separate from the
// advertiser API Key auth — admin operations require a different credential.
//
// In production, this should be replaced with a proper auth system (OAuth, RBAC).
// For now, a static token is sufficient for the internal admin panel.
func AdminAuthMiddleware(next http.Handler) http.Handler {
	token := os.Getenv("ADMIN_TOKEN")
	if token == "" {
		token = "admin-secret" // default for development
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS preflight must bypass auth
		if r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("X-Admin-Token")
		if auth == "" {
			auth = r.URL.Query().Get("admin_token")
		}
		if auth != token {
			WriteError(w, http.StatusUnauthorized, "admin authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
