package bidder

import (
	"context"
	"testing"
	"time"
)

// TestStatsCache_Start_ExitsOnContextCancel verifies the Batch 3 lifecycle
// contract for the stats cache refresh loop: once its context is cancelled,
// the background goroutine must exit promptly rather than leak past the
// process shutdown window.
//
// The refresh tick is 5 minutes, so during this test the loop spends its
// entire time parked in the select — the <-ctx.Done() branch is the only
// thing that lets it leave. reportStore is nil so the initial refresh
// short-circuits; Redis is nil for the same reason (the loop never calls
// it on nil reportStore). We're testing the for-select, not the refresh.
func TestStatsCache_Start_ExitsOnContextCancel(t *testing.T) {
	sc := NewStatsCache(nil, nil, func() []*LoadedCampaign { return nil })

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		sc.Start(ctx)
		close(done)
	}()

	// Give the goroutine a moment to reach the select.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// expected: loop saw ctx.Done and returned
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StatsCache.Start did not exit within 500ms after ctx cancel")
	}
}

// TestStatsCache_Start_ExitsOnStop verifies the legacy stopCh path still
// works, so the belt-and-suspenders call in cmd/bidder/main.go (which
// invokes Stop even though workerCtx is already cancelled) remains safe.
func TestStatsCache_Start_ExitsOnStop(t *testing.T) {
	sc := NewStatsCache(nil, nil, func() []*LoadedCampaign { return nil })

	done := make(chan struct{})
	go func() {
		sc.Start(context.Background())
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	sc.Stop()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StatsCache.Start did not exit within 500ms after Stop()")
	}
}
