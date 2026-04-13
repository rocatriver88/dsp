# Phase 2: Spend Guardrails + Reconciliation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add spend safety guardrails to the bidder (global budget ceiling, bid cap, balance alerts, circuit breaker) and implement automated hourly/daily reconciliation between Redis and ClickHouse spend data.

**Architecture:** New `internal/guardrail/` package for spend limits and alerts, integrated into the bidder engine. New `internal/reconciliation/` package with scheduled jobs. Circuit breaker as a Redis-backed atomic flag checked on every bid. Reconciliation runs as a goroutine in the API server (hourly) and a cron-triggered daily job.

**Tech Stack:** Go 1.26, Redis (atomic flags + spend tracking), ClickHouse (actual spend queries), existing alert webhook package, existing billing.SaveReconciliation

---

## File Structure

```
internal/guardrail/
├── guardrail.go          # GlobalGuardrail: budget ceiling, bid cap, balance check
├── guardrail_test.go     # Unit tests with Redis mock
├── circuitbreaker.go     # Redis-backed circuit breaker (atomic flag)
└── circuitbreaker_test.go

internal/reconciliation/
├── reconciliation.go      # Hourly + daily reconciliation logic
└── reconciliation_test.go
```

**Modifications:**
- `internal/config/config.go` — add guardrail config fields
- `internal/bidder/engine.go` — inject guardrail check before bid
- `cmd/bidder/main.go` — initialize guardrail, pass to engine
- `cmd/api/main.go` — add circuit-break admin endpoint
- `internal/handler/admin.go` (or new file) — circuit-break handler
- `cmd/autopilot/continuous.go` — add reconciliation status to daily reports

---

### Task 1: Guardrail Config

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Add guardrail fields to Config struct**

Add to the `Config` struct in `internal/config/config.go` after `BidderHMACSecret`:

```go
// Guardrails
GlobalDailyBudgetCents int64  // all campaigns combined, 0 = no limit
MaxBidCPMCents         int    // single bid ceiling, 0 = no limit
LowBalanceAlertCents   int64  // alert when below this, 0 = disabled
MinBalanceCents        int64  // auto-pause all when below this, 0 = disabled
SpendRateWindowSec     int    // window for spend rate check (default 300 = 5min)
SpendRateMultiplier    float64 // alert if rate > expected * this (default 3.0)
```

Add to `Load()`:

```go
GlobalDailyBudgetCents: parseInt64("GLOBAL_DAILY_BUDGET_CENTS", 0),
MaxBidCPMCents:         parseInt("MAX_BID_CPM_CENTS", 0),
LowBalanceAlertCents:   parseInt64("LOW_BALANCE_ALERT_CENTS", 0),
MinBalanceCents:        parseInt64("MIN_BALANCE_CENTS", 0),
SpendRateWindowSec:     parseInt("SPEND_RATE_WINDOW_SEC", 300),
SpendRateMultiplier:    parseFloat("SPEND_RATE_MULTIPLIER", 3.0),
```

Add helper functions `parseInt64` and `parseFloat` following the existing `getEnv` pattern:

```go
func parseInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func parseInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func parseFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
```

Add `"strconv"` to the imports in config.go.

- [ ] **Step 2: Verify compilation**

Run: `cd /c/Users/Roc/github/dsp && go vet ./internal/config/`

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): add guardrail configuration fields"
```

---

### Task 2: Circuit Breaker

**Files:**
- Create: `internal/guardrail/circuitbreaker.go`
- Create: `internal/guardrail/circuitbreaker_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/guardrail/circuitbreaker_test.go
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
		rdb.Close()
	})
	return rdb
}

func TestCircuitBreaker_DefaultOpen(t *testing.T) {
	rdb := newTestRedis(t)
	cb := NewCircuitBreaker(rdb)

	// Default state: bidding allowed
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/guardrail/ -run TestCircuit -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write implementation**

```go
// internal/guardrail/circuitbreaker.go
package guardrail

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
)

const circuitBreakerKey = "guardrail:circuit_breaker"
const circuitBreakerReasonKey = "guardrail:circuit_breaker:reason"

// CircuitBreaker is a Redis-backed kill switch for all bidding.
// "Open" means bidding is allowed. "Tripped" means all bidding stops.
type CircuitBreaker struct {
	rdb *redis.Client
}

func NewCircuitBreaker(rdb *redis.Client) *CircuitBreaker {
	return &CircuitBreaker{rdb: rdb}
}

// IsOpen returns true if bidding is allowed (circuit is open/normal).
// Returns true (allow) on Redis errors — fail-open to avoid blocking bids
// due to transient Redis issues.
func (cb *CircuitBreaker) IsOpen(ctx context.Context) bool {
	val, err := cb.rdb.Get(ctx, circuitBreakerKey).Result()
	if err == redis.Nil {
		return true // key not set = open = bidding allowed
	}
	if err != nil {
		log.Printf("[CIRCUIT-BREAKER] Redis error (fail-open): %v", err)
		return true
	}
	return val != "tripped"
}

// Trip closes the circuit breaker, blocking all bids.
func (cb *CircuitBreaker) Trip(ctx context.Context, reason string) {
	cb.rdb.Set(ctx, circuitBreakerKey, "tripped", 0)
	cb.rdb.Set(ctx, circuitBreakerReasonKey, reason, 0)
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/guardrail/ -run TestCircuit -v`
Expected: PASS (3 tests) or SKIP if Redis unavailable

- [ ] **Step 5: Commit**

```bash
git add internal/guardrail/
git commit -m "feat(guardrail): add Redis-backed circuit breaker"
```

---

### Task 3: Global Guardrail Service

**Files:**
- Create: `internal/guardrail/guardrail.go`
- Create: `internal/guardrail/guardrail_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/guardrail/guardrail_test.go
package guardrail

import (
	"context"
	"testing"
	"time"
)

func TestBidCeiling_Blocks(t *testing.T) {
	rdb := newTestRedis(t)
	g := New(rdb, Config{
		MaxBidCPMCents: 5000, // 50 CNY CPM ceiling
	})

	result := g.CheckBid(context.Background(), 6000) // 60 CNY CPM
	if result.Allowed {
		t.Error("bid above ceiling should be blocked")
	}
	if result.Reason != "bid_ceiling_exceeded" {
		t.Errorf("expected reason 'bid_ceiling_exceeded', got '%s'", result.Reason)
	}
}

func TestBidCeiling_Allows(t *testing.T) {
	rdb := newTestRedis(t)
	g := New(rdb, Config{
		MaxBidCPMCents: 5000,
	})

	result := g.CheckBid(context.Background(), 3000)
	if !result.Allowed {
		t.Errorf("bid below ceiling should be allowed, reason: %s", result.Reason)
	}
}

func TestBidCeiling_ZeroMeansNoLimit(t *testing.T) {
	rdb := newTestRedis(t)
	g := New(rdb, Config{
		MaxBidCPMCents: 0,
	})

	result := g.CheckBid(context.Background(), 999999)
	if !result.Allowed {
		t.Error("zero ceiling should mean no limit")
	}
}

func TestGlobalDailyBudget_Blocks(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()

	// Clean up
	date := time.Now().Format("2006-01-02")
	rdb.Del(ctx, "guardrail:global_spend:"+date)

	g := New(rdb, Config{
		GlobalDailyBudgetCents: 1000, // 10 CNY ceiling
	})

	// Simulate spending 1100 cents already
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

	g := New(rdb, Config{
		GlobalDailyBudgetCents: 100000,
	})

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

	// Trip circuit breaker
	g.CB.Trip(ctx, "test")

	result := g.CheckBid(ctx, 100)
	if result.Allowed {
		t.Error("should block when circuit breaker is tripped")
	}
	if result.Reason != "circuit_breaker_tripped" {
		t.Errorf("expected reason 'circuit_breaker_tripped', got '%s'", result.Reason)
	}

	g.CB.Reset(ctx)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/guardrail/ -run "TestBid|TestGlobal|TestRecord|TestCircuitBreakerCheck" -v`
Expected: FAIL — types not defined

- [ ] **Step 3: Write implementation**

```go
// internal/guardrail/guardrail.go
package guardrail

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config holds guardrail thresholds.
type Config struct {
	GlobalDailyBudgetCents int64   // 0 = no limit
	MaxBidCPMCents         int     // 0 = no limit
	LowBalanceAlertCents   int64   // 0 = disabled
	MinBalanceCents        int64   // 0 = disabled
	SpendRateWindowSec     int     // default 300
	SpendRateMultiplier    float64 // default 3.0
}

// CheckResult is the outcome of a guardrail check.
type CheckResult struct {
	Allowed bool
	Reason  string
}

// Guardrail enforces global spend safety limits.
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

// CheckBid validates a bid against all guardrails before placing it.
// Call this in the bidder engine before returning a bid response.
func (g *Guardrail) CheckBid(ctx context.Context, bidCPMCents int) CheckResult {
	// 1. Circuit breaker
	if !g.CB.IsOpen(ctx) {
		return CheckResult{Allowed: false, Reason: "circuit_breaker_tripped"}
	}

	// 2. Bid ceiling
	if g.config.MaxBidCPMCents > 0 && bidCPMCents > g.config.MaxBidCPMCents {
		return CheckResult{Allowed: false, Reason: "bid_ceiling_exceeded"}
	}

	// 3. Global daily budget
	if g.config.GlobalDailyBudgetCents > 0 {
		result := g.CheckGlobalBudget(ctx)
		if !result.Allowed {
			return result
		}
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
		return CheckResult{
			Allowed: false,
			Reason:  "global_daily_budget_exceeded",
		}
	}
	return CheckResult{Allowed: true}
}

// RecordGlobalSpend atomically adds spend to the global daily counter.
// Call this after each win notice.
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/guardrail/ -v`
Expected: PASS (all tests) or SKIP if Redis unavailable

- [ ] **Step 5: Commit**

```bash
git add internal/guardrail/
git commit -m "feat(guardrail): add global spend limits, bid ceiling, circuit breaker integration"
```

---

### Task 4: Integrate Guardrail into Bidder Engine

**Files:**
- Modify: `internal/bidder/engine.go`
- Modify: `cmd/bidder/main.go`

- [ ] **Step 1: Add guardrail field to Engine**

In `internal/bidder/engine.go`, add the guardrail import and field:

```go
import (
	// ... existing imports ...
	"github.com/heartgryphon/dsp/internal/guardrail"
)

type Engine struct {
	loader     *CampaignLoader
	budget     *budget.Service
	strategy   *BidStrategy
	statsCache *StatsCache
	producer   *events.Producer
	fraud      *antifraud.Filter
	guardrail  *guardrail.Guardrail // nil to skip guardrail checks
}
```

Update `NewEngine` to accept guardrail:

```go
func NewEngine(loader *CampaignLoader, budgetSvc *budget.Service, strategy *BidStrategy, statsCache *StatsCache, producer *events.Producer, fraud *antifraud.Filter, guard *guardrail.Guardrail) *Engine {
	return &Engine{
		loader:     loader,
		budget:     budgetSvc,
		strategy:   strategy,
		statsCache: statsCache,
		producer:   producer,
		fraud:      fraud,
		guardrail:  guard,
	}
}
```

- [ ] **Step 2: Add guardrail check in Bid() method**

In `engine.go`, after the fraud check (line ~100) and before campaign matching (line ~117), insert:

```go
	// Guardrail: circuit breaker + global budget check
	if e.guardrail != nil {
		preCheck := e.guardrail.CheckBid(ctx, 0) // check circuit breaker + global budget (bid ceiling checked per-candidate below)
		if !preCheck.Allowed {
			return nil, nil
		}
	}
```

Then in the campaign loop (line ~152), after computing `bidCPM` and before selecting the best candidate, add the bid ceiling check:

```go
		// Guardrail: bid ceiling
		if e.guardrail != nil {
			capCheck := e.guardrail.CheckBid(ctx, bidCPM)
			if !capCheck.Allowed {
				continue // skip this candidate, bid too high
			}
		}
```

- [ ] **Step 3: Initialize guardrail in cmd/bidder/main.go**

Add imports and initialization after `engine = bidder.NewEngine(...)`:

```go
import "github.com/heartgryphon/dsp/internal/guardrail"

// After existing service init, before engine creation:
guard := guardrail.New(rdb, guardrail.Config{
	GlobalDailyBudgetCents: cfg.GlobalDailyBudgetCents,
	MaxBidCPMCents:         cfg.MaxBidCPMCents,
	LowBalanceAlertCents:   cfg.LowBalanceAlertCents,
	MinBalanceCents:        cfg.MinBalanceCents,
	SpendRateWindowSec:     cfg.SpendRateWindowSec,
	SpendRateMultiplier:    cfg.SpendRateMultiplier,
})

engine = bidder.NewEngine(loader, budgetSvc, strategySvc, statsCache, producer, fraudFilter, guard)
```

Also add `guard.RecordGlobalSpend(r.Context(), spendCents)` in the `handleWin` function after the budget deduction succeeds (around line 350), so global spend is tracked:

```go
// After successful budget deduction in handleWin:
if guard != nil && !isCPC {
	spendCents := int64(price / 0.90 * 100)
	guard.RecordGlobalSpend(r.Context(), spendCents)
}
```

- [ ] **Step 4: Fix all existing tests**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/bidder/ -v -short`

Existing tests calling `NewEngine` will need the extra `nil` parameter for guardrail. Update all call sites in test files to pass `nil` as the last argument.

- [ ] **Step 5: Verify full build**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/bidder/ && go build ./cmd/api/`

- [ ] **Step 6: Commit**

```bash
git add internal/bidder/engine.go cmd/bidder/main.go internal/bidder/*_test.go
git commit -m "feat(bidder): integrate guardrail checks into bid pipeline"
```

---

### Task 5: Circuit Breaker Admin Endpoint

**Files:**
- Modify: `cmd/api/main.go` — add route
- Modify: `internal/handler/handler.go` — add guardrail to Deps
- Create or modify: admin handler file for circuit-break endpoints

- [ ] **Step 1: Add guardrail to handler Deps**

In `internal/handler/handler.go`, add:

```go
import "github.com/heartgryphon/dsp/internal/guardrail"

type Deps struct {
	Store       *campaign.Store
	ReportStore *reporting.Store
	BillingSvc  *billing.Service
	RegSvc      *registration.Service
	BudgetSvc   *budget.Service
	Redis       *redis.Client
	Guardrail   *guardrail.Guardrail // nil if guardrails disabled
}
```

- [ ] **Step 2: Add circuit-break handlers**

Add to the handler package (in a new section of an existing handler file, or a new `internal/handler/guardrail.go`):

```go
// internal/handler/guardrail.go
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// HandleCircuitBreak trips the circuit breaker, stopping all bidding.
func (d *Deps) HandleCircuitBreak(w http.ResponseWriter, r *http.Request) {
	if d.Guardrail == nil {
		WriteError(w, http.StatusServiceUnavailable, "guardrails not configured")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Reason == "" {
		req.Reason = "manual: admin triggered at " + time.Now().Format(time.RFC3339)
	}

	d.Guardrail.CB.Trip(r.Context(), req.Reason)
	WriteJSON(w, http.StatusOK, map[string]string{
		"status": "tripped",
		"reason": req.Reason,
	})
}

// HandleCircuitReset resets the circuit breaker, resuming bidding.
func (d *Deps) HandleCircuitReset(w http.ResponseWriter, r *http.Request) {
	if d.Guardrail == nil {
		WriteError(w, http.StatusServiceUnavailable, "guardrails not configured")
		return
	}

	d.Guardrail.CB.Reset(r.Context())
	WriteJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

// HandleCircuitStatus returns the current circuit breaker state.
func (d *Deps) HandleCircuitStatus(w http.ResponseWriter, r *http.Request) {
	if d.Guardrail == nil {
		WriteError(w, http.StatusServiceUnavailable, "guardrails not configured")
		return
	}

	ctx := r.Context()
	open := d.Guardrail.CB.IsOpen(ctx)
	reason := d.Guardrail.CB.TripReason(ctx)
	globalSpend := d.Guardrail.GetGlobalSpend(ctx)

	status := "open"
	if !open {
		status = "tripped"
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"circuit_breaker": status,
		"reason":          reason,
		"global_spend_today_cents": globalSpend,
	})
}
```

- [ ] **Step 3: Register routes in cmd/api/main.go**

Add to the admin routes section (around line 132):

```go
adminMux.HandleFunc("POST /api/v1/admin/circuit-break", h.HandleCircuitBreak)
adminMux.HandleFunc("POST /api/v1/admin/circuit-reset", h.HandleCircuitReset)
adminMux.HandleFunc("GET /api/v1/admin/circuit-status", h.HandleCircuitStatus)
```

Initialize guardrail in `cmd/api/main.go` and pass to Deps:

```go
import "github.com/heartgryphon/dsp/internal/guardrail"

// After Redis connection, before handler deps:
var guard *guardrail.Guardrail
if rdb != nil {
	guard = guardrail.New(rdb, guardrail.Config{
		GlobalDailyBudgetCents: cfg.GlobalDailyBudgetCents,
		MaxBidCPMCents:         cfg.MaxBidCPMCents,
	})
}

h := &handler.Deps{
	// ... existing fields ...
	Guardrail: guard,
}
```

- [ ] **Step 4: Build and verify**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/api/ && go build ./cmd/bidder/`

- [ ] **Step 5: Commit**

```bash
git add internal/handler/handler.go internal/handler/guardrail.go cmd/api/main.go
git commit -m "feat(admin): add circuit-break/reset/status endpoints"
```

---

### Task 6: Reconciliation Service

**Files:**
- Create: `internal/reconciliation/reconciliation.go`
- Create: `internal/reconciliation/reconciliation_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/reconciliation/reconciliation_test.go
package reconciliation

import (
	"testing"
	"time"
)

func TestReconcileResult_NoDifference(t *testing.T) {
	r := ReconcileResult{
		CampaignID:      1,
		Date:            time.Now(),
		RedisSpentCents: 10000,
		CHSpentCents:    10000,
	}
	if r.DiffPercent() != 0 {
		t.Errorf("expected 0%% diff, got %.2f%%", r.DiffPercent())
	}
	if r.NeedsAlert(1.0) {
		t.Error("should not alert when no difference")
	}
}

func TestReconcileResult_SmallDifference(t *testing.T) {
	r := ReconcileResult{
		CampaignID:      1,
		Date:            time.Now(),
		RedisSpentCents: 10000,
		CHSpentCents:    10050,
	}
	diff := r.DiffPercent()
	if diff < 0.4 || diff > 0.6 {
		t.Errorf("expected ~0.5%% diff, got %.2f%%", diff)
	}
	if r.NeedsAlert(1.0) {
		t.Error("0.5% diff should not trigger 1% alert threshold")
	}
}

func TestReconcileResult_LargeDifference(t *testing.T) {
	r := ReconcileResult{
		CampaignID:      1,
		Date:            time.Now(),
		RedisSpentCents: 10000,
		CHSpentCents:    10200,
	}
	diff := r.DiffPercent()
	if diff < 1.9 || diff > 2.1 {
		t.Errorf("expected ~2%% diff, got %.2f%%", diff)
	}
	if !r.NeedsAlert(1.0) {
		t.Error("2% diff should trigger 1% alert threshold")
	}
}

func TestReconcileResult_ZeroSpend(t *testing.T) {
	r := ReconcileResult{
		CampaignID:      1,
		Date:            time.Now(),
		RedisSpentCents: 0,
		CHSpentCents:    0,
	}
	if r.DiffPercent() != 0 {
		t.Errorf("expected 0%% diff for zero spend, got %.2f%%", r.DiffPercent())
	}
	if r.NeedsAlert(1.0) {
		t.Error("should not alert on zero spend")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/reconciliation/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write implementation**

```go
// internal/reconciliation/reconciliation.go
package reconciliation

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/heartgryphon/dsp/internal/alert"
	"github.com/heartgryphon/dsp/internal/billing"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/redis/go-redis/v9"
)

// ReconcileResult holds the outcome of comparing Redis vs ClickHouse spend.
type ReconcileResult struct {
	CampaignID      int64
	Date            time.Time
	RedisSpentCents int64
	CHSpentCents    int64
}

// DiffPercent returns the absolute percentage difference between Redis and ClickHouse.
func (r ReconcileResult) DiffPercent() float64 {
	max := math.Max(float64(r.RedisSpentCents), float64(r.CHSpentCents))
	if max == 0 {
		return 0
	}
	diff := math.Abs(float64(r.RedisSpentCents) - float64(r.CHSpentCents))
	return (diff / max) * 100
}

// NeedsAlert returns true if the difference exceeds the threshold percentage.
func (r ReconcileResult) NeedsAlert(thresholdPercent float64) bool {
	if r.RedisSpentCents == 0 && r.CHSpentCents == 0 {
		return false
	}
	return r.DiffPercent() > thresholdPercent
}

// Service runs reconciliation checks.
type Service struct {
	rdb         *redis.Client
	store       *campaign.Store
	reportStore *reporting.Store
	billingSvc  *billing.Service
	alerter     alert.Sender
}

func New(rdb *redis.Client, store *campaign.Store, reportStore *reporting.Store, billingSvc *billing.Service, alerter alert.Sender) *Service {
	return &Service{
		rdb:         rdb,
		store:       store,
		reportStore: reportStore,
		billingSvc:  billingSvc,
		alerter:     alerter,
	}
}

// RunHourly compares Redis spend vs ClickHouse spend for active campaigns.
// Alerts if any campaign has >thresholdPercent difference.
func (s *Service) RunHourly(ctx context.Context, thresholdPercent float64) ([]ReconcileResult, error) {
	if s.reportStore == nil {
		return nil, fmt.Errorf("ClickHouse not available")
	}

	campaigns, err := s.store.ListActiveCampaigns(ctx)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}

	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var results []ReconcileResult
	var alerts []string

	for _, c := range campaigns {
		// Redis: read daily budget remaining, compute spent
		date := now.Format("2006-01-02")
		key := fmt.Sprintf("budget:daily:%d:%s", c.ID, date)
		remaining, err := s.rdb.Get(ctx, key).Int64()
		if err == redis.Nil {
			remaining = 0
		} else if err != nil {
			continue
		}
		redisSpent := c.BudgetDailyCents - remaining
		if redisSpent < 0 {
			redisSpent = 0
		}

		// ClickHouse: query actual spend
		stats, err := s.reportStore.GetCampaignStats(ctx, uint64(c.ID), dayStart, now)
		if err != nil {
			continue
		}
		chSpent := int64(stats.SpendCents)

		result := ReconcileResult{
			CampaignID:      c.ID,
			Date:            now,
			RedisSpentCents: redisSpent,
			CHSpentCents:    chSpent,
		}
		results = append(results, result)

		if result.NeedsAlert(thresholdPercent) {
			alerts = append(alerts, fmt.Sprintf(
				"Campaign %d: Redis=%d CH=%d diff=%.1f%%",
				c.ID, redisSpent, chSpent, result.DiffPercent()))
		}
	}

	if len(alerts) > 0 {
		msg := fmt.Sprintf("Reconciliation alert (%d campaigns):\n", len(alerts))
		for _, a := range alerts {
			msg += "- " + a + "\n"
		}
		s.alerter.Send("Reconciliation Drift Detected", msg)
		log.Printf("[RECONCILIATION] %s", msg)
	}

	return results, nil
}

// RunDaily performs end-of-day reconciliation: compares Redis vs ClickHouse,
// saves results to the daily_reconciliation table, and records billing adjustments.
func (s *Service) RunDaily(ctx context.Context, date time.Time) error {
	if s.reportStore == nil {
		return fmt.Errorf("ClickHouse not available")
	}

	campaigns, err := s.store.ListActiveCampaigns(ctx)
	if err != nil {
		return fmt.Errorf("list campaigns: %w", err)
	}

	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	dayEnd := dayStart.Add(24 * time.Hour)
	var totalAdjustment int64

	for _, c := range campaigns {
		dateStr := date.Format("2006-01-02")
		key := fmt.Sprintf("budget:daily:%d:%s", c.ID, dateStr)
		remaining, _ := s.rdb.Get(ctx, key).Int64()
		redisSpent := c.BudgetDailyCents - remaining
		if redisSpent < 0 {
			redisSpent = 0
		}

		stats, err := s.reportStore.GetCampaignStats(ctx, uint64(c.ID), dayStart, dayEnd)
		if err != nil {
			continue
		}
		chSpent := int64(stats.SpendCents)

		adjustment := chSpent - redisSpent // positive = underbilled, negative = overbilled
		status := "reconciled"
		if adjustment != 0 {
			status = "adjusted"
		}

		rec := &billing.Reconciliation{
			CampaignID:      c.ID,
			Date:            dayStart,
			RedisSpent:      redisSpent,
			ClickhouseSpent: chSpent,
			Adjustment:      adjustment,
			Status:          status,
		}
		if err := s.billingSvc.SaveReconciliation(ctx, rec); err != nil {
			log.Printf("[RECONCILIATION] Save failed for campaign %d: %v", c.ID, err)
			continue
		}

		totalAdjustment += adjustment
		log.Printf("[RECONCILIATION] Campaign %d: redis=%d ch=%d adj=%d status=%s",
			c.ID, redisSpent, chSpent, adjustment, status)
	}

	s.alerter.Send("Daily Reconciliation Complete",
		fmt.Sprintf("Date: %s\nCampaigns: %d\nTotal adjustment: %d cents",
			date.Format("2006-01-02"), len(campaigns), totalAdjustment))

	return nil
}

// StartHourlySchedule runs hourly reconciliation in a goroutine.
func (s *Service) StartHourlySchedule(ctx context.Context, thresholdPercent float64) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				results, err := s.RunHourly(ctx, thresholdPercent)
				if err != nil {
					log.Printf("[RECONCILIATION] Hourly error: %v", err)
				} else {
					log.Printf("[RECONCILIATION] Hourly check: %d campaigns checked", len(results))
				}
			}
		}
	}()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/reconciliation/ -v`
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/reconciliation/
git commit -m "feat(reconciliation): add hourly + daily reconciliation service"
```

---

### Task 7: Integrate Reconciliation into API Server

**Files:**
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Start hourly reconciliation in API server**

In `cmd/api/main.go`, after starting the auto-pause service, add:

```go
import (
	"github.com/heartgryphon/dsp/internal/reconciliation"
)

// After autoPauseSvc.Start(ctx):
if reportStore != nil && rdb != nil {
	alerter := newAlertSender(cfg) // need to add this helper or use alert.Noop{}
	reconSvc := reconciliation.New(rdb, store, reportStore, billingSvc, alert.Noop{})
	reconSvc.StartHourlySchedule(ctx, 1.0) // 1% threshold
	log.Println("Hourly reconciliation started")
}
```

Note: For alerting, either add a helper function to create an alert sender from config (similar to autopilot's `newAlertSender`), or use `alert.Noop{}` for now and configure it later.

- [ ] **Step 2: Build and verify**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/api/`

- [ ] **Step 3: Commit**

```bash
git add cmd/api/main.go
git commit -m "feat(api): start hourly reconciliation schedule"
```

---

### Task 8: Autopilot Reconciliation Monitoring

**Files:**
- Modify: `cmd/autopilot/client.go` — add circuit-status API call
- Modify: `cmd/autopilot/continuous.go` — add reconciliation to daily report

- [ ] **Step 1: Add circuit-status client method**

Append to `cmd/autopilot/client.go`:

```go
type CircuitStatus struct {
	Status     string `json:"circuit_breaker"`
	Reason     string `json:"reason"`
	GlobalSpend int64 `json:"global_spend_today_cents"`
}

func (c *DSPClient) GetCircuitStatus() (*CircuitStatus, error) {
	data, status, err := c.do("GET", "/api/v1/admin/circuit-status", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("circuit status: status %d", status)
	}
	var resp CircuitStatus
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("circuit status: decode: %w", err)
	}
	return &resp, nil
}
```

- [ ] **Step 2: Add to daily report in continuous.go**

In the `generateDailyReport()` method, after the overview step, add:

```go
	// Circuit breaker status
	if circuitStatus, err := s.client.GetCircuitStatus(); err == nil {
		steps = append(steps, StepResult{
			Name:   "Circuit Breaker Status",
			Passed: circuitStatus.Status == "open",
			Detail: fmt.Sprintf("Status: %s, Global spend: %d cents",
				circuitStatus.Status, circuitStatus.GlobalSpend),
			Error: circuitStatus.Reason,
		})
	}
```

- [ ] **Step 3: Build and test**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/autopilot/`

- [ ] **Step 4: Commit**

```bash
git add cmd/autopilot/client.go cmd/autopilot/continuous.go
git commit -m "feat(autopilot): add circuit breaker and reconciliation monitoring to daily reports"
```

---

### Task 9: Integration Smoke Test

**Files:**
- None new

- [ ] **Step 1: Run all unit tests**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/guardrail/ ./internal/reconciliation/ ./cmd/autopilot/ -v -count=1`
Expected: All tests pass

- [ ] **Step 2: Run full project build**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/api/ && go build ./cmd/bidder/ && go build ./cmd/autopilot/`
Expected: All binaries compile

- [ ] **Step 3: Run go vet**

Run: `cd /c/Users/Roc/github/dsp && go vet ./...`
Expected: No issues

- [ ] **Step 4: Commit final state if needed**

```bash
git add -A && git commit -m "chore: Phase 2 integration smoke test pass"
```

---

## Out of Scope

- **Exchange reconciliation** (comparing with Exchange-side reports): Depends on having real Exchange API access and report formats. Deferred until actual Exchange partnerships are signed.
- **Balance alert webhook**: The infrastructure exists (alert package + config fields). Implementing the periodic balance check is trivial once there are real advertisers. Deferred to Phase 3.
- **Spend rate alerting**: Similar — config fields exist, but the actual monitoring goroutine adds complexity for a scenario that only matters with real traffic. Deferred to Phase 3.

## Task Dependency Graph

```
Task 1 (config) ──────────────────────────────────┐
Task 2 (circuit breaker) ─────────────────────────┤
Task 3 (guardrail service) ── depends on Task 2 ──┤
                                                    ├── Task 4 (bidder integration) ─── Task 9 (smoke test)
Task 5 (admin endpoints) ── depends on Task 3 ────┤
Task 6 (reconciliation) ──────────────────────────┤
Task 7 (API reconciliation schedule) ─ Task 6 ────┤
Task 8 (autopilot upgrade) ── depends on Task 5 ──┘
```

Tasks 1, 2, 6 can be parallelized. Task 3 depends on 2. Task 4 depends on 1+3. Task 5 depends on 3. Task 7 depends on 6. Task 8 depends on 5. Task 9 depends on all.
