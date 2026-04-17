package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// LoginGuard implements brute-force protection for the login endpoint.
//
// Primary: Redis-based per-email rate limiting (5 attempts → 15 min lock).
// Fallback: in-process sliding window per-IP (100/min) when Redis is down.
//
// Fail-closed: if Redis is unavailable AND in-process limit is reached,
// login is denied. This protects platform_admin high-value targets.
type LoginGuard struct {
	redis *redis.Client // nil if Redis unavailable

	mu       sync.Mutex
	counters map[string]*ipCounter // IP → counter (in-process fallback)
}

// ipCounter tracks per-IP login attempts in the in-process fallback.
type ipCounter struct {
	count  int
	window time.Time // start of current window
}

const (
	maxLoginAttempts = 5
	lockoutDuration  = 15 * time.Minute
	ipWindowDuration = time.Minute
	ipMaxPerWindow   = 100
)

// NewLoginGuard creates a new LoginGuard. rdb may be nil (dev mode).
func NewLoginGuard(rdb *redis.Client) *LoginGuard {
	return &LoginGuard{
		redis:    rdb,
		counters: make(map[string]*ipCounter),
	}
}

// Check verifies that a login attempt is allowed for the given email and IP.
// Returns a non-nil error if the attempt should be blocked.
func (g *LoginGuard) Check(ctx context.Context, email, ip string) error {
	// Try Redis first
	if g.redis != nil {
		key := "login_lockout:" + email
		count, err := g.redis.Get(ctx, key).Int()
		if err == nil && count >= maxLoginAttempts {
			return fmt.Errorf("account temporarily locked due to too many failed attempts")
		}
		if err != nil && err != redis.Nil {
			// Redis error — fall through to in-process guard
			return g.checkInProcess(ip)
		}
		// Redis is working, no lockout — allow
		return nil
	}

	// No Redis — use in-process fallback
	return g.checkInProcess(ip)
}

// RecordFailure records a failed login attempt for rate limiting.
func (g *LoginGuard) RecordFailure(ctx context.Context, email, ip string) {
	if g.redis != nil {
		key := "login_lockout:" + email
		pipe := g.redis.Pipeline()
		pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, lockoutDuration)
		_, _ = pipe.Exec(ctx)
		return
	}

	// In-process fallback: just count per IP
	g.mu.Lock()
	defer g.mu.Unlock()
	c, ok := g.counters[ip]
	if !ok || time.Since(c.window) > ipWindowDuration {
		g.counters[ip] = &ipCounter{count: 1, window: time.Now()}
		return
	}
	c.count++
}

// RecordSuccess clears the failure count on successful login.
func (g *LoginGuard) RecordSuccess(ctx context.Context, email string) {
	if g.redis != nil {
		g.redis.Del(ctx, "login_lockout:"+email)
	}
}

// checkInProcess enforces the per-IP in-process fallback (100/min).
func (g *LoginGuard) checkInProcess(ip string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	c, ok := g.counters[ip]
	if !ok || time.Since(c.window) > ipWindowDuration {
		// No counter or window expired — allow
		return nil
	}
	if c.count >= ipMaxPerWindow {
		return fmt.Errorf("too many login attempts, try again later")
	}
	return nil
}
