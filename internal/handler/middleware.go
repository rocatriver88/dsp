package handler

import (
	"net/http"
	"strings"

	"github.com/heartgryphon/dsp/internal/config"
)

// WithAuthExemption routes unauthenticated paths directly to the mux, bypassing auth middleware.
func WithAuthExemption(authed http.Handler, publicMux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SSE endpoints can't set custom headers from EventSource, so accept api_key in query params
		if strings.HasPrefix(r.URL.Path, "/api/v1/analytics/") {
			if apiKey := r.URL.Query().Get("api_key"); apiKey != "" {
				r.Header.Set("X-API-Key", apiKey)
			}
		}
		if r.URL.Path == "/health" || r.URL.Path == "/api/v1/docs" || (r.Method == "POST" && r.URL.Path == "/api/v1/register") {
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
