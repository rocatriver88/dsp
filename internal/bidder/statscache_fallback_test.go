package bidder

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
)

// TestStatsCache_RedisMiss_FallsBackToLocal asserts that when the Redis
// pipeline GET for a campaign's stats:ctr/cvr keys fails (either because
// Redis has no entry yet or the TTL expired), Get() returns the in-memory
// `local` map value instead of a zero CachedStats. The fallback layer is
// how the bidder stays useful when refresh() is still warming up or
// Redis has evicted the data but refresh() hasn't re-populated.
//
// REGRESSION SENTINEL: P1-6 StatsCache Redis-miss fallback
// (docs/testing-strategy-bidder.md §3 P1). Note — the original gap
// description framed this as "miss → on-demand ClickHouse requery", but
// the actual implementation (statscache.go:90-93) falls back to the
// in-memory map populated by the periodic refresh. This test matches
// the implementation, not the original spec. Break-revert verified
// 2026-04-19: removing the `sc.local[campaignID]` fallback line causes
// this test to FAIL with CachedStats{0,0,0}, confirming the fallback is
// the only path protecting against a cold Redis.
func TestStatsCache_RedisMiss_FallsBackToLocal(t *testing.T) {
	// Point at an unreachable Redis — the pipeline Get inside
	// StatsCache.Get will return errors (connection refused), which triggers
	// the local-map fallback path we want to exercise. We don't need a real
	// Redis miss semantic — we just need Redis lookups to fail.
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}) // nothing listens here
	t.Cleanup(func() { _ = rdb.Close() })

	// Construct the StatsCache with a nil report store (refresh() would
	// noop, which is fine — we directly poke the local map).
	sc := NewStatsCache(rdb, nil, func() []*LoadedCampaign { return nil })

	const campaignID int64 = 42
	wanted := CachedStats{CTR: 0.023, CVR: 0.051, WinRate: 0.45}

	// Simulate the state refresh() would leave behind: local map populated,
	// Redis optionally populated. Here we leave Redis empty on purpose to
	// exercise the miss → local fallback path.
	sc.mu.Lock()
	sc.local[campaignID] = wanted
	sc.mu.Unlock()

	// Redis is unreachable — sanity check that the pipeline GET does
	// indeed error out (not return redis.Nil, which would be a real
	// miss). Either way, Get() should fall back to the local map.
	if _, err := rdb.Get(context.Background(), "stats:ctr:42").Result(); err == nil {
		t.Fatal("setup precondition: expected Redis error (unreachable), got nil")
	}

	got := sc.Get(context.Background(), campaignID)
	if got.CTR != wanted.CTR {
		t.Errorf("CTR: want %v, got %v (Redis miss should fall back to local map)", wanted.CTR, got.CTR)
	}
	if got.CVR != wanted.CVR {
		t.Errorf("CVR: want %v, got %v", wanted.CVR, got.CVR)
	}
}

// TestStatsCache_UnknownCampaign_ReturnsZero asserts that when neither
// Redis nor the local map has any entry for a campaign, Get returns the
// zero CachedStats rather than panicking or returning a different
// campaign's values.
//
// REGRESSION SENTINEL: P1-6 companion — defense against map-key
// confusion in the fallback path.
func TestStatsCache_UnknownCampaign_ReturnsZero(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = rdb.Close() })

	sc := NewStatsCache(rdb, nil, func() []*LoadedCampaign { return nil })

	sc.mu.Lock()
	sc.local[42] = CachedStats{CTR: 0.1, CVR: 0.2, WinRate: 0.3}
	sc.mu.Unlock()

	// Ask for a different campaign ID that nobody has cached anywhere.
	got := sc.Get(context.Background(), 99)
	if got.CTR != 0 || got.CVR != 0 || got.WinRate != 0 {
		t.Errorf("unknown campaign: want zero CachedStats, got %+v", got)
	}
}
