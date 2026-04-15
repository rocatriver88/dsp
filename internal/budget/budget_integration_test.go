//go:build integration

package budget_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/qaharness"
)

// Scenario 8 — simple deduct.
func TestBudget_SimpleDeduct(t *testing.T) {
	h := qaharness.New(t)
	campID := int64(900001)
	svc := budget.New(h.RDB)

	if err := svc.InitDailyBudget(h.Ctx, campID, 10_000); err != nil {
		t.Fatal(err)
	}
	remain, err := svc.CheckAndDeductBudget(h.Ctx, campID, 300)
	if err != nil {
		t.Fatal(err)
	}
	if remain != 9700 {
		t.Errorf("expected 9700, got %d", remain)
	}
	if got := h.GetBudgetRemaining(campID); got != 9700 {
		t.Errorf("redis GET returned %d, expected 9700", got)
	}
}

// Scenario 9 — exhaustion returns -1 without over-deducting.
func TestBudget_Exhaustion(t *testing.T) {
	h := qaharness.New(t)
	campID := int64(900002)
	svc := budget.New(h.RDB)

	if err := svc.InitDailyBudget(h.Ctx, campID, 100); err != nil {
		t.Fatal(err)
	}

	for i, amt := range []int64{50, 50} {
		r, err := svc.CheckAndDeductBudget(h.Ctx, campID, amt)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if r < 0 {
			t.Fatalf("step %d unexpected -1", i)
		}
	}
	r, err := svc.CheckAndDeductBudget(h.Ctx, campID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if r != -1 {
		t.Errorf("expected -1 on third deduct, got %d", r)
	}
	if got := h.GetBudgetRemaining(campID); got != 0 {
		t.Errorf("budget should be 0 after exhaustion, got %d", got)
	}
}

// Scenario 10 — 100 concurrent deducts, exact math.
// Proves the Lua script is atomic: final remaining = 10000 - 50 * successes,
// with no double-deduction regardless of race timing.
func TestBudget_ConcurrentAtomicity(t *testing.T) {
	h := qaharness.New(t)
	campID := int64(900003)
	svc := budget.New(h.RDB)

	if err := svc.InitDailyBudget(h.Ctx, campID, 10_000); err != nil {
		t.Fatal(err)
	}

	var successes, failures atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			r, err := svc.CheckAndDeductBudget(ctx, campID, 50)
			if err != nil {
				failures.Add(1)
				return
			}
			if r < 0 {
				failures.Add(1)
			} else {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()

	total := successes.Load() + failures.Load()
	if total != 100 {
		t.Fatalf("expected 100 total, got %d", total)
	}
	expectedRemaining := int64(10_000) - 50*successes.Load()
	got := h.GetBudgetRemaining(campID)
	if got != expectedRemaining {
		t.Errorf("budget mismatch: expected %d (10000 - 50 * %d successes), got %d",
			expectedRemaining, successes.Load(), got)
	}
	t.Logf("concurrent test: successes=%d failures=%d remaining=%d",
		successes.Load(), failures.Load(), got)
}

// Scenario 11 — PipelineCheck rolls budget back when freq cap is hit.
func TestBudget_PipelineFreqRollback(t *testing.T) {
	h := qaharness.New(t)
	campID := int64(900004)
	svc := budget.New(h.RDB)
	if err := svc.InitDailyBudget(h.Ctx, campID, 10_000); err != nil {
		t.Fatal(err)
	}

	user := "qa-user-1"

	// 1st call: both OK; budget becomes 9950
	bOK, fOK, err := svc.PipelineCheck(h.Ctx, campID, user, 50, 2, 24)
	if err != nil || !bOK || !fOK {
		t.Fatalf("1st: err=%v bOK=%v fOK=%v", err, bOK, fOK)
	}
	// 2nd call: both OK; budget becomes 9900
	bOK, fOK, err = svc.PipelineCheck(h.Ctx, campID, user, 50, 2, 24)
	if err != nil || !bOK || !fOK {
		t.Fatalf("2nd: err=%v bOK=%v fOK=%v", err, bOK, fOK)
	}
	// 3rd call: freq cap hit, budget must roll back.
	bOK, fOK, err = svc.PipelineCheck(h.Ctx, campID, user, 50, 2, 24)
	if err != nil {
		t.Fatal(err)
	}
	if fOK {
		t.Error("expected freq cap to block 3rd call")
	}
	// Budget MUST still be 9900 (not 9850) — proves the rollback in PipelineCheck.
	if got := h.GetBudgetRemaining(campID); got != 9900 {
		t.Errorf("budget should remain at 9900 after freq rollback, got %d", got)
	}
	// Freq counter should have been incremented three times regardless.
	if got := h.GetFreqCount(campID, user); got != 3 {
		t.Errorf("freq count should be 3, got %d", got)
	}
}
