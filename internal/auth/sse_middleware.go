package auth

import (
	"context"
	"net/http"
	"time"
)

// SSETokenMiddleware returns middleware that authenticates requests using
// a short-lived HMAC token in the ?token= query parameter. It exists for
// EventSource/SSE clients that cannot set custom headers.
//
// Unlike APIKeyMiddleware, the long-lived tenant X-API-Key is never
// exposed via URL query. Clients first POST to /api/v1/analytics/token
// (authenticated via X-API-Key header, normal chain) to mint a 5-minute
// SSE token, then use that token in the stream URL.
//
// V5.1 hotfix P1-1: this middleware deliberately does NOT accept
// ?api_key= as a fallback. See TestSSETokenMiddleware_RejectsQueryApiKey.
//
// Uses time.Now() as the validation clock. For deterministic tests
// (e.g. exactly-at-expiry boundaries), use SSETokenMiddlewareWithClock.
func SSETokenMiddleware(secret []byte) func(http.Handler) http.Handler {
	return SSETokenMiddlewareWithClock(secret, time.Now)
}

// SSETokenMiddlewareWithClock is the testable variant of SSETokenMiddleware
// that accepts an injectable clock function. Production code should use
// SSETokenMiddleware; tests that need deterministic expiry boundaries can
// pass a fixed clock.
//
// Error messages are deliberately uniform ("invalid SSE token") for both
// the missing-token and invalid-signature cases — an attacker must not be
// able to distinguish "no token was sent" from "bad signature" via the
// response body. Both are unauthorized and the user-visible message is
// identical.
func SSETokenMiddlewareWithClock(secret []byte, clock func() time.Time) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.URL.Query().Get("token")
			if token == "" {
				writeAuthError(w, http.StatusUnauthorized, "invalid SSE token")
				return
			}
			advID, err := ValidateSSEToken(secret, token, clock())
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "invalid SSE token")
				return
			}
			adv := &Advertiser{ID: advID}
			ctx := context.WithValue(r.Context(), advertiserKey, adv)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
