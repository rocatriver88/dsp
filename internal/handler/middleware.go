package handler

import (
	"net/http"
	"strings"

	"github.com/heartgryphon/dsp/internal/config"
)

// WithAuthExemption routes unauthenticated paths directly to the mux, bypassing auth middleware.
//
// V5.1 P1-1: the legacy analytics `?api_key=` query promotion was deleted
// here. SSE authentication now goes through SSETokenMiddleware, wired in
// BuildPublicHandler's dispatcher. Tenant X-API-Key must never appear in
// URL query — putting credentials in URLs leaks them into proxy logs,
// browser history, and referrer headers.
func WithAuthExemption(authed http.Handler, publicMux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/health/") || strings.HasPrefix(r.URL.Path, "/uploads/") || (r.Method == "POST" && r.URL.Path == "/api/v1/register") || (r.Method == "POST" && r.URL.Path == "/api/v1/auth/login") || (r.Method == "POST" && r.URL.Path == "/api/v1/auth/refresh") {
			publicMux.ServeHTTP(w, r)
			return
		}
		authed.ServeHTTP(w, r)
	})
}

// WithCORS adds CORS headers based on configured allowed origins.
func WithCORS(cfg *config.Config, next http.Handler) http.Handler {
	allowed := make(map[string]bool)
	for _, origin := range strings.Split(cfg.CORSAllowedOrigins, ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Admin-Token")
		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Max-Age", "3600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
