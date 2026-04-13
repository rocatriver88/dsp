package guardrail

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	GlobalDailyBudgetCents int64
	MaxBidCPMCents         int
	LowBalanceAlertCents   int64
	MinBalanceCents        int64
	SpendRateWindowSec     int
	SpendRateMultiplier    float64
}

type CheckResult struct {
	Allowed bool
	Reason  string
}

type Guardrail struct {
	rdb    *redis.Client
	config Config
	CB     *CircuitBreaker
}

func New(rdb *redis.Client, cfg Config) *Guardrail {
	return &Guardrail{
		rdb:    rdb,
		config: cfg,
		CB:     NewCircuitBreaker(rdb),
	}
}

// PreCheck validates circuit breaker and global budget. Call once per bid request.
func (g *Guardrail) PreCheck(ctx context.Context) CheckResult {
	if !g.CB.IsOpen(ctx) {
		return CheckResult{Allowed: false, Reason: "circuit_breaker_tripped"}
	}
	if g.config.GlobalDailyBudgetCents > 0 {
		result := g.CheckGlobalBudget(ctx)
		if !result.Allowed {
			return result
		}
	}
	return CheckResult{Allowed: true}
}

// CheckBidCeiling validates a specific bid against the max CPM ceiling.
func (g *Guardrail) CheckBidCeiling(ctx context.Context, bidCPMCents int) CheckResult {
	if g.config.MaxBidCPMCents > 0 && bidCPMCents > g.config.MaxBidCPMCents {
		return CheckResult{Allowed: false, Reason: "bid_ceiling_exceeded"}
	}
	return CheckResult{Allowed: true}
}

// CheckGlobalBudget checks if total platform spend is under the daily ceiling.
func (g *Guardrail) CheckGlobalBudget(ctx context.Context) CheckResult {
	if g.config.GlobalDailyBudgetCents <= 0 {
		return CheckResult{Allowed: true}
	}

	date := time.Now().Format("2006-01-02")
	key := "guardrail:global_spend:" + date

	spent, err := g.rdb.Get(ctx, key).Int64()
	if err == redis.Nil {
		spent = 0
	} else if err != nil {
		log.Printf("[GUARDRAIL] Redis error (fail-open): %v", err)
		return CheckResult{Allowed: true}
	}

	if spent >= g.config.GlobalDailyBudgetCents {
		return CheckResult{Allowed: false, Reason: "global_daily_budget_exceeded"}
	}
	return CheckResult{Allowed: true}
}

// RecordGlobalSpend atomically adds spend to the global daily counter.
func (g *Guardrail) RecordGlobalSpend(ctx context.Context, amountCents int64) {
	date := time.Now().Format("2006-01-02")
	key := "guardrail:global_spend:" + date

	pipe := g.rdb.Pipeline()
	pipe.IncrBy(ctx, key, amountCents)
	pipe.Expire(ctx, key, 25*time.Hour)
	_, err := pipe.Exec(ctx)
	if err != nil {
		log.Printf("[GUARDRAIL] Record spend error: %v", err)
	}
}

// GetGlobalSpend returns today's total platform spend.
func (g *Guardrail) GetGlobalSpend(ctx context.Context) int64 {
	date := time.Now().Format("2006-01-02")
	key := "guardrail:global_spend:" + date
	val, _ := g.rdb.Get(ctx, key).Int64()
	return val
}
