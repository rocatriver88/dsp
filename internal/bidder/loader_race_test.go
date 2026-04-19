//go:build integration

package bidder_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/qaharness"
)

// TestLoader_ConcurrentReadWrite_NoRace asserts that CampaignLoader's
// RWMutex-protected in-memory map survives aggressive concurrent reads
// while the pub/sub reload path is simultaneously swapping the map
// pointer. Run with -race to catch unsynchronised reads/writes.
//
// The bidder hot path fans out GetCampaign() across every incoming bid
// request (thousands per second in production), while pub/sub
// notifications trigger full-map reloads under the write lock. Any
// future refactor that drops the lock, replaces it with a channel
// without proper synchronisation, or adds a new field accessed without
// the mutex would be caught here.
//
// REGRESSION SENTINEL: P1-7 Loader concurrent RWMutex safety
// (docs/testing-strategy-bidder.md §3 P1). Break-revert verified
// 2026-04-19: removing the Lock() / Unlock() calls in loader.go's
// fullLoad or pub/sub handler produces a "DATA RACE" report under -race.
func TestLoader_ConcurrentReadWrite_NoRace(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("race-loader")

	// Seed a handful of campaigns so GetCampaign() has real hits and
	// misses during the concurrent read loop.
	const seedCount = 5
	campIDs := make([]int64, 0, seedCount)
	for i := 0; i < seedCount; i++ {
		id := h.SeedCampaign(qaharness.CampaignSpec{
			AdvertiserID: advID,
			Name:         fmt.Sprintf("qa-race-%d", i),
			BidCPMCents:  1000,
		})
		h.SeedCreative(id, "", "")
		campIDs = append(campIDs, id)
	}

	cl := startLoader(t, h)

	// 200ms test: hammer GetCampaign from 50 goroutines while the pub/sub
	// reload handler swaps the map via PublishCampaignUpdate. A race
	// detector failure shows up as a fatal test panic with "DATA RACE".
	var (
		stop    atomic.Bool
		wg      sync.WaitGroup
		reads   atomic.Int64
		notnils atomic.Int64
	)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for !stop.Load() {
				// Cycle through the seeded IDs so every worker hits a
				// mix of present and absent (modular) lookups.
				target := campIDs[seed%len(campIDs)]
				if c := cl.GetCampaign(target); c != nil {
					notnils.Add(1)
				}
				reads.Add(1)
			}
		}(i)
	}

	// Concurrent reloads: publish a flurry of campaign update events to
	// force the loader to re-read from pg and swap the map while the
	// readers above are spinning.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for !stop.Load() {
			for _, id := range campIDs {
				h.PublishCampaignUpdate(id, "updated")
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Run the hammering for 200ms — short enough to keep CI fast, long
	// enough to trigger -race if the synchronisation is wrong.
	time.Sleep(200 * time.Millisecond)
	stop.Store(true)
	wg.Wait()
	<-writerDone

	if reads.Load() == 0 {
		t.Fatalf("no reads completed — scheduler didn't run the readers?")
	}
	t.Logf("race test: %d reads (%d non-nil) against concurrent reloads", reads.Load(), notnils.Load())
}
