package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// hashKey returns a truncated SHA-256 hex digest of key (first 16 hex chars = 64 bits).
// Used to avoid storing plaintext API keys in Redis key names.
func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])[:16]
}

// Limiter implements Redis fixed-window rate limiting with fail-open behavior.
type Limiter struct {
	rdb *redis.Client
}

// New creates a rate limiter. If rdb is nil, all requests are allowed.
func New(rdb *redis.Client) *Limiter {
	return &Limiter{rdb: rdb}
}

// Allow checks if a request identified by key is within the rate limit.
// Returns true if allowed. Fails open on Redis errors (allows the request).
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) bool {
	if l.rdb == nil {
		return true
	}

	redisKey := "ratelimit:" + key
	count, err := l.rdb.Incr(ctx, redisKey).Result()
	if err != nil {
		log.Printf("[RATELIMIT] Redis error (fail-open): %v", err)
		return true
	}

	if count == 1 {
		l.rdb.Expire(ctx, redisKey, window)
	}

	return count <= int64(limit)
}

// Middleware returns HTTP middleware that rate-limits by the given key function.
// keyFunc extracts the rate limit key from the request (e.g., API key or IP).
func Middleware(limiter *Limiter, keyFunc func(*http.Request) string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if !limiter.Allow(r.Context(), key, limit, window) {
				w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IPKeyFunc extracts the client IP for rate limiting unauthenticated requests.
func IPKeyFunc(r *http.Request) string {
	return "ip:" + r.RemoteAddr
}

// APIKeyFunc extracts the API key for rate limiting authenticated requests.
// The key is hashed so that Redis dumps never expose plaintext API keys.
func APIKeyFunc(r *http.Request) string {
	key := r.Header.Get("X-API-Key")
	if key == "" {
		return "ip:" + r.RemoteAddr
	}
	return "key:" + hashKey(key)
}
