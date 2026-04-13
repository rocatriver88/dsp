package guardrail

import (
	"context"
	"testing"
	"time"
)

func TestBidCeiling_Blocks(t *testing.T) {
	rdb := newTestRedis(t)
	g := New(rdb, Config{MaxBidCPMCents: 5000})

	result := g.CheckBidCeiling(context.Background(), 6000)
	if result.Allowed {
		t.Error("bid above ceiling should be blocked")
	}
	if result.Reason != "bid_ceiling_exceeded" {
		t.Errorf("expected reason 'bid_ceiling_exceeded', got '%s'", result.Reason)
	}
}

func TestBidCeiling_Allows(t *testing.T) {
	rdb := newTestRedis(t)
	g := New(rdb, Config{MaxBidCPMCents: 5000})

	result := g.CheckBidCeiling(context.Background(), 3000)
	if !result.Allowed {
		t.Errorf("bid below ceiling should be allowed, reason: %s", result.Reason)
	}
}

func TestBidCeiling_ZeroMeansNoLimit(t *testing.T) {
	rdb := newTestRedis(t)
	g := New(rdb, Config{MaxBidCPMCents: 0})

	result := g.CheckBidCeiling(context.Background(), 999999)
	if !result.Allowed {
		t.Error("zero ceiling should mean no limit")
	}
}

func TestGlobalDailyBudget_Blocks(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()

	date := time.Now().Format("2006-01-02")
	rdb.Del(ctx, "guardrail:global_spend:"+date)

	g := New(rdb, Config{GlobalDailyBudgetCents: 1000})
	rdb.Set(ctx, "guardrail:global_spend:"+date, 1100, 25*time.Hour)

	result := g.CheckGlobalBudget(ctx)
	if result.Allowed {
		t.Error("should block when global spend exceeds daily ceiling")
	}
}

func TestGlobalDailyBudget_Allows(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()

	date := time.Now().Format("2006-01-02")
	rdb.Del(ctx, "guardrail:global_spend:"+date)

	g := New(rdb, Config{GlobalDailyBudgetCents: 100000})

	result := g.CheckGlobalBudget(ctx)
	if !result.Allowed {
		t.Error("should allow when under budget")
	}
}

func TestRecordGlobalSpend(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()

	date := time.Now().Format("2006-01-02")
	rdb.Del(ctx, "guardrail:global_spend:"+date)

	g := New(rdb, Config{})

	g.RecordGlobalSpend(ctx, 500)
	g.RecordGlobalSpend(ctx, 300)

	val, _ := rdb.Get(ctx, "guardrail:global_spend:"+date).Int64()
	if val != 800 {
		t.Errorf("expected global spend 800, got %d", val)
	}
}

func TestCircuitBreakerCheck(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()
	g := New(rdb, Config{})

	g.CB.Trip(ctx, "test")
	defer g.CB.Reset(ctx)

	result := g.PreCheck(ctx)
	if result.Allowed {
		t.Error("should block when circuit breaker is tripped")
	}
	if result.Reason != "circuit_breaker_tripped" {
		t.Errorf("expected reason 'circuit_breaker_tripped', got '%s'", result.Reason)
	}
}
