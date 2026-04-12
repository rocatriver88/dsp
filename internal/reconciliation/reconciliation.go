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

// RunDaily performs end-of-day reconciliation and saves to daily_reconciliation table.
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

		adjustment := chSpent - redisSpent
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
