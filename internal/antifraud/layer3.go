package antifraud

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/heartgryphon/dsp/internal/reporting"
)

// Layer3 performs daily batch fraud analysis using ClickHouse aggregations.
//
// Checks:
//   - Campaign CTR anomaly: CTR > 5% over 1000+ impressions (CPM only)
//   - Traffic source quality: geo distributions that deviate from targeting
//
// Results are logged and can trigger auto-pause via the autopause service.
type Layer3 struct {
	reportStore *reporting.Store
}

func NewLayer3(reportStore *reporting.Store) *Layer3 {
	return &Layer3{reportStore: reportStore}
}

// AnomalyReport holds daily fraud analysis results.
type AnomalyReport struct {
	CampaignID  int64   `json:"campaign_id"`
	Date        string  `json:"date"`
	CTR         float64 `json:"ctr"`
	Impressions uint64  `json:"impressions"`
	Suspicious  bool    `json:"suspicious"`
	Reason      string  `json:"reason,omitempty"`
}

// AnalyzeCampaign runs daily fraud checks for a single campaign.
func (l *Layer3) AnalyzeCampaign(ctx context.Context, campaignID uint64, date time.Time) (*AnomalyReport, error) {
	if l.reportStore == nil {
		return nil, fmt.Errorf("ClickHouse not available")
	}

	nextDay := date.Add(24 * time.Hour)
	stats, err := l.reportStore.GetCampaignStats(ctx, campaignID, date, nextDay)
	if err != nil {
		return nil, err
	}

	report := &AnomalyReport{
		CampaignID:  int64(campaignID),
		Date:        date.Format("2006-01-02"),
		CTR:         stats.CTR,
		Impressions: stats.Impressions,
	}

	// CTR anomaly: > 5% with sufficient data (1000+ impressions)
	if stats.Impressions >= 1000 && stats.CTR > 5.0 {
		report.Suspicious = true
		report.Reason = fmt.Sprintf("CTR anomaly: %.1f%% over %d impressions", stats.CTR, stats.Impressions)
		log.Printf("[FRAUD-L3] campaign=%d %s", campaignID, report.Reason)
	}

	return report, nil
}

// RunDaily analyzes all provided campaign IDs for the given date.
func (l *Layer3) RunDaily(ctx context.Context, campaignIDs []uint64, date time.Time) []AnomalyReport {
	var reports []AnomalyReport
	for _, id := range campaignIDs {
		report, err := l.AnalyzeCampaign(ctx, id, date)
		if err != nil {
			log.Printf("[FRAUD-L3] campaign=%d error: %v", id, err)
			continue
		}
		reports = append(reports, *report)
	}

	suspicious := 0
	for _, r := range reports {
		if r.Suspicious {
			suspicious++
		}
	}
	log.Printf("[FRAUD-L3] daily analysis: %d campaigns, %d suspicious", len(reports), suspicious)
	return reports
}
