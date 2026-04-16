package guardrail

import (
	"context"
	"log"
	"strings"

	"github.com/heartgryphon/dsp/internal/observability"
	"github.com/redis/go-redis/v9"
)

// classifyTripReason maps a free-form trip reason string to one of the
// bounded label values for dsp_guardrail_trips_total.
func classifyTripReason(reason string) string {
	r := strings.ToLower(reason)
	switch {
	case strings.Contains(r, "daily") && strings.Contains(r, "budget"):
		return "daily_budget"
	case strings.Contains(r, "max") && strings.Contains(r, "cpm"):
		return "max_cpm"
	case strings.Contains(r, "manual"):
		return "manual"
	default:
		return "other"
	}
}

const circuitBreakerKey = "guardrail:circuit_breaker"
const circuitBreakerReasonKey = "guardrail:circuit_breaker:reason"

// CircuitBreaker is a Redis-backed kill switch for all bidding.
// "Open" means bidding is allowed. "Tripped" means all bidding stops.
//
// When FailClosed is true (production mode), Redis errors cause bidding
// to be blocked — preventing unlimited spend when the guardrail state
// is unknown. When FailClosed is false (dev mode, default), Redis errors
// are treated as "bidding allowed" to avoid blocking local development.
type CircuitBreaker struct {
	rdb        *redis.Client
	FailClosed bool // true = production: block bids on Redis error
}

func NewCircuitBreaker(rdb *redis.Client) *CircuitBreaker {
	return &CircuitBreaker{rdb: rdb}
}

// NewCircuitBreakerWithMode creates a CircuitBreaker with explicit fail
// mode. Pass failClosed=true for production (block bids on Redis error)
// or false for development (allow bids on Redis error).
func NewCircuitBreakerWithMode(rdb *redis.Client, failClosed bool) *CircuitBreaker {
	return &CircuitBreaker{rdb: rdb, FailClosed: failClosed}
}

// IsOpen returns true if bidding is allowed (circuit is open/normal).
//
// On Redis errors the behaviour depends on FailClosed:
//   - FailClosed=false (dev): returns true  (fail-open, allow bids)
//   - FailClosed=true (prod): returns false (fail-closed, block bids)
func (cb *CircuitBreaker) IsOpen(ctx context.Context) bool {
	val, err := cb.rdb.Get(ctx, circuitBreakerKey).Result()
	if err == redis.Nil {
		return true
	}
	if err != nil {
		observability.RedisErrorsTotal.WithLabelValues("get").Inc()
		if cb.FailClosed {
			log.Printf("[CIRCUIT-BREAKER] Redis error (fail-CLOSED, blocking bids): %v", err)
			return false
		}
		log.Printf("[CIRCUIT-BREAKER] Redis error (fail-open): %v", err)
		return true
	}
	return val != "tripped"
}

// Trip closes the circuit breaker, blocking all bids.
func (cb *CircuitBreaker) Trip(ctx context.Context, reason string) {
	cb.rdb.Set(ctx, circuitBreakerKey, "tripped", 0)
	cb.rdb.Set(ctx, circuitBreakerReasonKey, reason, 0)
	observability.GuardrailTripsTotal.WithLabelValues(classifyTripReason(reason)).Inc()
	log.Printf("[CIRCUIT-BREAKER] TRIPPED: %s", reason)
}

// Reset opens the circuit breaker, allowing bids again.
func (cb *CircuitBreaker) Reset(ctx context.Context) {
	cb.rdb.Del(ctx, circuitBreakerKey)
	cb.rdb.Del(ctx, circuitBreakerReasonKey)
	log.Printf("[CIRCUIT-BREAKER] RESET: bidding resumed")
}

// TripReason returns the reason the circuit was tripped, or empty string.
func (cb *CircuitBreaker) TripReason(ctx context.Context) string {
	val, _ := cb.rdb.Get(ctx, circuitBreakerReasonKey).Result()
	return val
}
