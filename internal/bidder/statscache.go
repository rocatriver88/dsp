package bidder

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/redis/go-redis/v9"
)

// StatsCache periodically loads campaign CTR/CVR/WinRate from ClickHouse
// and caches them in Redis for fast access during bidding.
//
// Cache keys:
//   stats:ctr:{campaign_id}   → CTR as float (e.g., "0.023")
//   stats:cvr:{campaign_id}   → CVR as float (e.g., "0.051")
//   stats:winrate:{campaign_id} → win rate as float (e.g., "0.45")
//
// Data is 24h rolling window from ClickHouse, refreshed every 5 minutes.
type StatsCache struct {
	rdb         *redis.Client
	reportStore *reporting.Store
	campaigns   func() []*LoadedCampaign // getter for active campaigns
	stopCh      chan struct{}
	mu          sync.RWMutex
	local       map[int64]CachedStats // in-memory fallback
}

// CachedStats holds cached prediction data for a campaign.
type CachedStats struct {
	CTR     float64 // click-through rate (0.0 - 1.0)
	CVR     float64 // conversion rate (0.0 - 1.0)
	WinRate float64 // win rate (0.0 - 1.0)
}

func NewStatsCache(rdb *redis.Client, reportStore *reporting.Store, getCampaigns func() []*LoadedCampaign) *StatsCache {
	return &StatsCache{
		rdb:         rdb,
		reportStore: reportStore,
		campaigns:   getCampaigns,
		stopCh:      make(chan struct{}),
		local:       make(map[int64]CachedStats),
	}
}

// Start begins the background refresh loop.
func (sc *StatsCache) Start(ctx context.Context) {
	// Initial load
	sc.refresh(ctx)

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-sc.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			sc.refresh(ctx)
		}
	}
}

func (sc *StatsCache) Stop() {
	close(sc.stopCh)
}

// Get returns cached stats for a campaign. Returns zero values if not cached.
func (sc *StatsCache) Get(ctx context.Context, campaignID int64) CachedStats {
	// Try Redis first
	pipe := sc.rdb.Pipeline()
	ctrCmd := pipe.Get(ctx, fmt.Sprintf("stats:ctr:%d", campaignID))
	cvrCmd := pipe.Get(ctx, fmt.Sprintf("stats:cvr:%d", campaignID))
	pipe.Exec(ctx)

	ctr, err1 := ctrCmd.Float64()
	cvr, err2 := cvrCmd.Float64()

	if err1 == nil && err2 == nil {
		return CachedStats{CTR: ctr, CVR: cvr}
	}

	// Fall back to in-memory cache
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.local[campaignID]
}

func (sc *StatsCache) refresh(ctx context.Context) {
	if sc.reportStore == nil {
		return
	}

	campaigns := sc.campaigns()
	if len(campaigns) == 0 {
		return
	}

	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	updated := 0

	newLocal := make(map[int64]CachedStats, len(campaigns))

	for _, c := range campaigns {
		stats, err := sc.reportStore.GetCampaignStats(ctx, uint64(c.ID), from, now)
		if err != nil {
			continue
		}

		// CTR and CVR from ClickHouse are percentages (e.g., 2.3 = 2.3%)
		// Convert to ratios (0.023) for EffectiveBidCPMCents
		cached := CachedStats{
			CTR:     stats.CTR / 100.0,
			CVR:     stats.CVR / 100.0,
			WinRate: stats.WinRate / 100.0,
		}

		// Only cache if we have meaningful data (at least 100 impressions)
		if stats.Impressions < 100 {
			continue
		}

		newLocal[c.ID] = cached

		// Write to Redis with 10-minute TTL (slightly longer than refresh interval)
		pipe := sc.rdb.Pipeline()
		pipe.Set(ctx, fmt.Sprintf("stats:ctr:%d", c.ID), strconv.FormatFloat(cached.CTR, 'f', 6, 64), 10*time.Minute)
		pipe.Set(ctx, fmt.Sprintf("stats:cvr:%d", c.ID), strconv.FormatFloat(cached.CVR, 'f', 6, 64), 10*time.Minute)
		pipe.Set(ctx, fmt.Sprintf("stats:winrate:%d", c.ID), strconv.FormatFloat(cached.WinRate, 'f', 6, 64), 10*time.Minute)
		pipe.Exec(ctx)

		updated++
	}

	sc.mu.Lock()
	sc.local = newLocal
	sc.mu.Unlock()

	if updated > 0 {
		log.Printf("[STATS-CACHE] refreshed %d/%d campaigns with ClickHouse stats", updated, len(campaigns))
	}
}
