package autopause

import (
	"context"
	"log/slog"
	"time"

	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/redis/go-redis/v9"
)

// Service checks active campaigns for anomalies and auto-pauses them.
type Service struct {
	store       *campaign.Store
	reportStore *reporting.Store
	rdb         *redis.Client
	interval    time.Duration
}

func New(store *campaign.Store, reportStore *reporting.Store, rdb *redis.Client) *Service {
	return &Service{
		store:       store,
		reportStore: reportStore,
		rdb:         rdb,
		interval:    5 * time.Minute,
	}
}

// Start begins the polling loop. Blocks until ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	if s.reportStore == nil {
		slog.Warn("auto-pause disabled: ClickHouse not connected")
		return
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	slog.Info("auto-pause service started", "interval", s.interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAll(ctx)
		}
	}
}

func (s *Service) checkAll(ctx context.Context) {
	campaigns, err := s.store.ListActiveCampaigns(ctx)
	if err != nil {
		slog.Error("auto-pause: list active campaigns", "error", err)
		return
	}

	now := time.Now().UTC()
	hourStart := now.Truncate(time.Hour)
	hourEnd := hourStart.Add(time.Hour)

	for _, c := range campaigns {
		stats, err := s.reportStore.GetCampaignStats(ctx, uint64(c.ID), hourStart, hourEnd)
		if err != nil || stats == nil {
			continue
		}

		// Check anomaly rules (extracted for testability)
		if reason := CheckSpendSpike(c.BudgetDailyCents, stats.SpendCents); reason != "" {
			s.pause(ctx, c.ID, reason)
			continue
		}
		if reason := CheckCTRAnomaly(c.BillingModel, stats.Impressions, stats.Clicks); reason != "" {
			s.pause(ctx, c.ID, reason)
			continue
		}
	}
}

func (s *Service) pause(ctx context.Context, campaignID int64, reason string) {
	if err := s.store.AutoPause(ctx, campaignID, reason); err != nil {
		slog.Error("auto-pause: failed to pause", "campaign_id", campaignID, "reason", reason, "error", err)
		return
	}
	slog.Warn("auto-pause: campaign paused", "campaign_id", campaignID, "reason", reason)

	// Notify bidder to remove campaign from cache
	if s.rdb != nil {
		import_bidder_notify(ctx, s.rdb, campaignID)
	}
}

// import_bidder_notify uses Redis pub/sub to notify bidder. Avoids import cycle.
func import_bidder_notify(ctx context.Context, rdb *redis.Client, campaignID int64) {
	// Direct Redis publish to avoid importing bidder package
	payload := []byte(`{"campaign_id":` + itoa(campaignID) + `,"action":"paused"}`)
	rdb.Publish(ctx, "campaign:updates", payload)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
