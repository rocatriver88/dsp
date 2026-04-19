package guardrail

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6380", Password: "dsp_dev_password"})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	t.Cleanup(func() {
		rdb.Del(context.Background(), circuitBreakerKey)
		rdb.Del(context.Background(), circuitBreakerReasonKey)
		rdb.Close()
	})
	return rdb
}

func TestCircuitBreaker_DefaultOpen(t *testing.T) {
	rdb := newTestRedis(t)
	cb := NewCircuitBreaker(rdb)
	if !cb.IsOpen(context.Background()) {
		t.Error("circuit breaker should be open (bidding allowed) by default")
	}
}

func TestCircuitBreaker_Trip(t *testing.T) {
	rdb := newTestRedis(t)
	cb := NewCircuitBreaker(rdb)
	ctx := context.Background()

	cb.Trip(ctx, "manual: test trip")

	if cb.IsOpen(ctx) {
		t.Error("circuit breaker should be closed (bidding blocked) after trip")
	}

	reason := cb.TripReason(ctx)
	if reason != "manual: test trip" {
		t.Errorf("expected trip reason 'manual: test trip', got '%s'", reason)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	rdb := newTestRedis(t)
	cb := NewCircuitBreaker(rdb)
	ctx := context.Background()

	cb.Trip(ctx, "test")
	cb.Reset(ctx)

	if !cb.IsOpen(ctx) {
		t.Error("circuit breaker should be open (bidding allowed) after reset")
	}
}

func TestCircuitBreaker_FailOpen_OnRedisError(t *testing.T) {
	// A client pointing at a non-existent Redis will produce errors on Get.
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:1"}) // no Redis here
	defer rdb.Close()
	cb := NewCircuitBreakerWithMode(rdb, false) // dev mode: fail-open

	// Should return true (allow bids) even though Redis is unreachable.
	if !cb.IsOpen(context.Background()) {
		t.Error("fail-open circuit breaker should allow bids when Redis is unreachable")
	}
}

// TestCircuitBreaker_FailClosed_OnRedisError verifies that a fail-closed
// circuit breaker returns !IsOpen (blocking bids) when Redis is
// unreachable. Point of failure would be a refactor that drops the
// FailClosed branch, or inverts the return value.
//
// REGRESSION SENTINEL: P0-3 guardrail fail-closed discipline
// (docs/testing-strategy-bidder.md §3 P0-3). Originally landed as fix
// commit a32ac0f. Break-revert verified 2026-04-19: editing
// circuitbreaker.go:66 from `if cb.FailClosed` to `if false` triggers
// "fail-closed circuit breaker should block bids when Redis is
// unreachable" — exactly the bug this sentinel is designed to catch.
// Revert restores GREEN.
func TestCircuitBreaker_FailClosed_OnRedisError(t *testing.T) {
	// A client pointing at a non-existent Redis will produce errors on Get.
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:1"}) // no Redis here
	defer rdb.Close()
	cb := NewCircuitBreakerWithMode(rdb, true) // production mode: fail-closed

	// Should return false (block bids) because Redis is unreachable.
	if cb.IsOpen(context.Background()) {
		t.Error("fail-closed circuit breaker should block bids when Redis is unreachable")
	}
}

func TestNewCircuitBreakerWithMode_DefaultFalse(t *testing.T) {
	// NewCircuitBreaker (no mode) should default to fail-open.
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:1"})
	defer rdb.Close()
	cb := NewCircuitBreaker(rdb)
	if cb.FailClosed {
		t.Error("default circuit breaker should be fail-open (FailClosed=false)")
	}
}
