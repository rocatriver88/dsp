package bidder

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6380", Password: "dsp_redis_password"})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	t.Cleanup(func() {
		// Clean up test keys
		ctx := context.Background()
		keys, _ := rdb.Keys(ctx, "strategy:*:test-*").Result()
		if len(keys) > 0 {
			rdb.Del(ctx, keys...)
		}
		rdb.Close()
	})
	return rdb
}

func TestAdjustedBid_NoData(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewBidStrategy(rdb)
	ctx := context.Background()

	// No spend/win data yet: should return base bid unchanged
	result := s.AdjustedBid(ctx, 999999, 500, 10000)
	if result != 500 {
		t.Errorf("expected base bid 500 with no data, got %d", result)
	}
}

func TestAdjustedBid_Clamping(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewBidStrategy(rdb)
	ctx := context.Background()

	campaignID := int64(100001)

	// Set extreme win rate (>80%): should lower bid but not below 50% of base
	date := time.Now().UTC().Format("2006-01-02")
	rdb.Set(ctx, fmt.Sprintf("strategy:bids:%d:%s", campaignID, date), 200, 25*time.Hour)
	rdb.Set(ctx, fmt.Sprintf("strategy:wins:%d:%s", campaignID, date), 190, 25*time.Hour) // 95% win rate

	result := s.AdjustedBid(ctx, campaignID, 1000, 10000)
	minBid := 500  // 50% of 1000
	maxBid := 1500 // 150% of 1000
	if result < minBid || result > maxBid {
		t.Errorf("adjusted bid %d outside clamped range [%d, %d]", result, minBid, maxBid)
	}
}

func TestAdjustedBid_HighWinRate(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewBidStrategy(rdb)
	ctx := context.Background()

	campaignID := int64(100002)
	date := time.Now().UTC().Format("2006-01-02")

	// 90% win rate with 200 bids: overpaying, should lower
	rdb.Set(ctx, fmt.Sprintf("strategy:bids:%d:%s", campaignID, date), 200, 25*time.Hour)
	rdb.Set(ctx, fmt.Sprintf("strategy:wins:%d:%s", campaignID, date), 180, 25*time.Hour)

	result := s.AdjustedBid(ctx, campaignID, 1000, 10000)
	if result >= 1000 {
		t.Errorf("expected lower bid with 90%% win rate, got %d", result)
	}
}

func TestAdjustedBid_LowWinRate(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewBidStrategy(rdb)
	ctx := context.Background()

	campaignID := int64(100003)
	date := time.Now().UTC().Format("2006-01-02")

	// 10% win rate with 200 bids: losing, should raise
	rdb.Set(ctx, fmt.Sprintf("strategy:bids:%d:%s", campaignID, date), 200, 25*time.Hour)
	rdb.Set(ctx, fmt.Sprintf("strategy:wins:%d:%s", campaignID, date), 20, 25*time.Hour)

	result := s.AdjustedBid(ctx, campaignID, 1000, 10000)
	if result <= 1000 {
		t.Errorf("expected higher bid with 10%% win rate, got %d", result)
	}
}

func TestShouldBid_BehindSchedule(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewBidStrategy(rdb)
	ctx := context.Background()

	// No spend data = behind schedule, should always bid
	for i := 0; i < 100; i++ {
		if !s.ShouldBid(ctx, 999998, 10000) {
			t.Fatal("should always bid when behind schedule (no spend data)")
		}
	}
}

func TestShouldBid_AheadOfSchedule(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewBidStrategy(rdb)
	ctx := context.Background()

	campaignID := int64(100004)
	date := time.Now().UTC().Format("2006-01-02")

	// Spend 3x expected: very ahead of schedule, should skip many
	hour := time.Now().UTC().Hour()
	if hour == 0 {
		hour = 1
	}
	dailyBudget := int64(24000)
	expectedSpend := dailyBudget * int64(hour) / 24
	actualSpend := expectedSpend * 3 // 3x ahead

	rdb.Set(ctx, fmt.Sprintf("strategy:spend:%d:%s", campaignID, date), actualSpend, 25*time.Hour)

	// Run 100 trials — some should be skipped
	skipped := 0
	for i := 0; i < 100; i++ {
		if !s.ShouldBid(ctx, campaignID, dailyBudget) {
			skipped++
		}
	}
	if skipped == 0 {
		t.Error("expected some bids to be skipped when 3x ahead of schedule")
	}
}

func TestRecordBid(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewBidStrategy(rdb)
	ctx := context.Background()

	campaignID := int64(100005)
	date := time.Now().UTC().Format("2006-01-02")
	rdb.Del(ctx, fmt.Sprintf("strategy:bids:%d:%s", campaignID, date))

	s.RecordBid(ctx, campaignID)
	s.RecordBid(ctx, campaignID)

	count, err := rdb.Get(ctx, fmt.Sprintf("strategy:bids:%d:%s", campaignID, date)).Int64()
	if err != nil {
		t.Fatalf("get bid count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 bids recorded, got %d", count)
	}
}

func TestRecordWin(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewBidStrategy(rdb)
	ctx := context.Background()

	campaignID := int64(100006)
	date := time.Now().UTC().Format("2006-01-02")
	rdb.Del(ctx, fmt.Sprintf("strategy:wins:%d:%s", campaignID, date))

	s.RecordWin(ctx, campaignID)

	count, err := rdb.Get(ctx, fmt.Sprintf("strategy:wins:%d:%s", campaignID, date)).Int64()
	if err != nil {
		t.Fatalf("get win count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 win recorded, got %d", count)
	}
}

func TestRecordSpend(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewBidStrategy(rdb)
	ctx := context.Background()

	campaignID := int64(100007)
	date := time.Now().UTC().Format("2006-01-02")
	rdb.Del(ctx, fmt.Sprintf("strategy:spend:%d:%s", campaignID, date))

	s.RecordSpend(ctx, campaignID, 500)
	s.RecordSpend(ctx, campaignID, 300)

	total, err := rdb.Get(ctx, fmt.Sprintf("strategy:spend:%d:%s", campaignID, date)).Int64()
	if err != nil {
		t.Fatalf("get spend: %v", err)
	}
	if total != 800 {
		t.Errorf("expected 800 total spend, got %d", total)
	}
}

func TestSpendRatio_ZeroBudget(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewBidStrategy(rdb)
	ctx := context.Background()

	ratio := s.spendRatio(ctx, 999997, 0)
	if ratio != 0 {
		t.Errorf("expected 0 ratio with zero budget, got %f", ratio)
	}
}

func TestWinRateFactor_InsufficientData(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewBidStrategy(rdb)
	ctx := context.Background()

	// < 100 bids: should return 1.0 (no adjustment)
	factor := s.winRateFactor(ctx, 999996)
	if factor != 1.0 {
		t.Errorf("expected 1.0 with insufficient data, got %f", factor)
	}
}

func TestWinRateFactor_SweetSpot(t *testing.T) {
	rdb := newTestRedis(t)
	s := NewBidStrategy(rdb)
	ctx := context.Background()

	campaignID := int64(100008)
	date := time.Now().UTC().Format("2006-01-02")

	// 50% win rate = sweet spot, factor should be 1.0
	rdb.Set(ctx, fmt.Sprintf("strategy:bids:%d:%s", campaignID, date), 200, 25*time.Hour)
	rdb.Set(ctx, fmt.Sprintf("strategy:wins:%d:%s", campaignID, date), 100, 25*time.Hour)

	factor := s.winRateFactor(ctx, campaignID)
	if factor != 1.0 {
		t.Errorf("expected 1.0 at 50%% win rate, got %f", factor)
	}
}
