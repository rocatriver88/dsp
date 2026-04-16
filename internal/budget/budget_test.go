package budget

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6380", Password: "dsp_redis_password"})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		keys, _ := rdb.Keys(ctx, "budget:daily:test-*").Result()
		freqKeys, _ := rdb.Keys(ctx, "freq:test-*").Result()
		keys = append(keys, freqKeys...)
		if len(keys) > 0 {
			rdb.Del(ctx, keys...)
		}
		rdb.Close()
	})
	return rdb
}

func TestCheckAndDeductBudget_Happy(t *testing.T) {
	rdb := newTestRedis(t)
	svc := New(rdb)
	ctx := context.Background()

	// Init budget with a test-prefixed key to avoid collision
	campaignID := int64(-1) // negative ID won't collide with real data
	svc.InitDailyBudget(ctx, campaignID, 10000)

	remaining, err := svc.CheckAndDeductBudget(ctx, campaignID, 100)
	if err != nil {
		t.Fatalf("deduct: %v", err)
	}
	if remaining != 9900 {
		t.Errorf("expected 9900 remaining, got %d", remaining)
	}
}

func TestCheckAndDeductBudget_Exhausted(t *testing.T) {
	rdb := newTestRedis(t)
	svc := New(rdb)
	ctx := context.Background()

	campaignID := int64(-2)
	svc.InitDailyBudget(ctx, campaignID, 100)

	// Try to deduct more than available
	remaining, err := svc.CheckAndDeductBudget(ctx, campaignID, 200)
	if err != nil {
		t.Fatalf("deduct: %v", err)
	}
	if remaining != -1 {
		t.Errorf("expected -1 (exhausted), got %d", remaining)
	}

	// Verify budget was refunded (should still be 100)
	rem, _ := svc.GetDailyBudgetRemaining(ctx, campaignID)
	if rem != 100 {
		t.Errorf("expected budget to be refunded to 100, got %d", rem)
	}
}

func TestCheckFrequency_UnderCap(t *testing.T) {
	rdb := newTestRedis(t)
	svc := New(rdb)
	ctx := context.Background()

	rdb.Del(ctx, "freq:-3:user-test-1")

	ok, err := svc.CheckFrequency(ctx, -3, "user-test-1", 5, 24)
	if err != nil {
		t.Fatalf("freq check: %v", err)
	}
	if !ok {
		t.Error("expected under cap")
	}
}

func TestCheckFrequency_OverCap(t *testing.T) {
	rdb := newTestRedis(t)
	svc := New(rdb)
	ctx := context.Background()

	// Hit 3 times with cap of 2
	svc.CheckFrequency(ctx, -4, "user-test-2", 2, 24)
	svc.CheckFrequency(ctx, -4, "user-test-2", 2, 24)
	ok, err := svc.CheckFrequency(ctx, -4, "user-test-2", 2, 24)
	if err != nil {
		t.Fatalf("freq check: %v", err)
	}
	if ok {
		t.Error("expected over cap after 3 hits with cap=2")
	}
}

func TestCheckFrequency_EmptyUserID(t *testing.T) {
	rdb := newTestRedis(t)
	svc := New(rdb)
	ctx := context.Background()

	// Empty user ID (GDPR): should always return true
	ok, err := svc.CheckFrequency(ctx, -5, "", 1, 24)
	if err != nil {
		t.Fatalf("freq check: %v", err)
	}
	if !ok {
		t.Error("expected true for empty user ID (GDPR skip)")
	}
}

func TestPipelineCheck_Happy(t *testing.T) {
	rdb := newTestRedis(t)
	svc := New(rdb)
	ctx := context.Background()

	campaignID := int64(-6)
	rdb.Del(ctx, "freq:-6:user-test-3")
	svc.InitDailyBudget(ctx, campaignID, 10000)

	budgetOK, freqOK, err := svc.PipelineCheck(ctx, campaignID, "user-test-3", 100, 5, 24)
	if err != nil {
		t.Fatalf("pipeline check: %v", err)
	}
	if !budgetOK {
		t.Error("expected budget OK")
	}
	if !freqOK {
		t.Error("expected freq OK")
	}
}

func TestPipelineCheck_BudgetExhausted(t *testing.T) {
	rdb := newTestRedis(t)
	svc := New(rdb)
	ctx := context.Background()

	campaignID := int64(-7)
	svc.InitDailyBudget(ctx, campaignID, 50)

	budgetOK, _, err := svc.PipelineCheck(ctx, campaignID, "user-test-4", 100, 5, 24)
	if err != nil {
		t.Fatalf("pipeline check: %v", err)
	}
	if budgetOK {
		t.Error("expected budget NOT OK when deducting 100 from 50")
	}
}

func TestPipelineCheck_FreqCapDoesNotDeductBudget(t *testing.T) {
	rdb := newTestRedis(t)
	svc := New(rdb)
	ctx := context.Background()

	campaignID := int64(-8)
	svc.InitDailyBudget(ctx, campaignID, 10000)

	// First two calls: both OK, budget NOT deducted (check-only)
	for i := 0; i < 2; i++ {
		budgetOK, freqOK, err := svc.PipelineCheck(ctx, campaignID, "user-test-5", 10, 2, 24)
		if err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
		if !budgetOK || !freqOK {
			t.Fatalf("call %d: expected both OK", i+1)
		}
	}

	// Third call: freq cap hit (cap=2), but budget should still be full
	// because PipelineCheck is now check-only (no deduction).
	budgetOK, freqOK, err := svc.PipelineCheck(ctx, campaignID, "user-test-5", 10, 2, 24)
	if err != nil {
		t.Fatalf("pipeline check: %v", err)
	}
	if freqOK {
		t.Error("expected freq NOT OK after exceeding cap")
	}
	// Budget should still be OK because PipelineCheck doesn't deduct
	if !budgetOK {
		t.Error("expected budget still OK (PipelineCheck is check-only, no deduction)")
	}

	// Verify budget is untouched — PipelineCheck never deducts
	rem, _ := svc.GetDailyBudgetRemaining(ctx, campaignID)
	if rem != 10000 {
		t.Errorf("expected budget to remain 10000 (check-only), got %d", rem)
	}
}

func TestPipelineCheck_CheckOnlyDoesNotDeduct(t *testing.T) {
	rdb := newTestRedis(t)
	svc := New(rdb)
	ctx := context.Background()

	campaignID := int64(-9)
	svc.InitDailyBudget(ctx, campaignID, 10000)

	// Multiple PipelineCheck calls should NOT deduct budget
	for i := 0; i < 5; i++ {
		budgetOK, _, err := svc.PipelineCheck(ctx, campaignID, "", 100, 0, 0)
		if err != nil {
			t.Fatalf("check %d: %v", i, err)
		}
		if !budgetOK {
			t.Fatalf("check %d: budget should be OK", i)
		}
	}

	// Budget should still be 10000 (no deduction at bid time)
	rem, _ := svc.GetDailyBudgetRemaining(ctx, campaignID)
	if rem != 10000 {
		t.Errorf("expected budget to remain 10000 after check-only calls, got %d", rem)
	}

	// Real deduction only happens at win time via CheckAndDeductBudget
	remaining, err := svc.CheckAndDeductBudget(ctx, campaignID, 100)
	if err != nil {
		t.Fatalf("deduct: %v", err)
	}
	if remaining != 9900 {
		t.Errorf("expected 9900 after actual deduction, got %d", remaining)
	}
}
