package reporting

import (
	"context"
	"fmt"
	"time"
)

// Attribution models for conversion analysis.
const (
	ModelLastClick  = "last_click"
	ModelFirstClick = "first_click"
	ModelLinear     = "linear"
)

// ConversionPath represents the full journey from impression to conversion.
type ConversionPath struct {
	ConversionTime time.Time        `json:"conversion_time"`
	DeviceID       string           `json:"device_id"`
	CampaignID     uint64           `json:"campaign_id"`
	Touchpoints    []Touchpoint     `json:"touchpoints"`
	Model          string           `json:"model"`
	Credit         []CreditedEvent  `json:"credit"`
}

// Touchpoint is a single impression or click in the conversion path.
type Touchpoint struct {
	Time      time.Time `json:"time"`
	EventType string    `json:"event_type"` // impression or click
	CreativeID uint64   `json:"creative_id"`
	RequestID string    `json:"request_id"`
}

// CreditedEvent shows how much credit each touchpoint gets under the model.
type CreditedEvent struct {
	RequestID string  `json:"request_id"`
	EventType string  `json:"event_type"`
	Credit    float64 `json:"credit"` // 0.0 - 1.0
}

// AttributionReport holds aggregated attribution data for a campaign.
type AttributionReport struct {
	CampaignID    uint64           `json:"campaign_id"`
	Model         string           `json:"model"`
	Window        string           `json:"window"`
	TotalConversions uint64        `json:"total_conversions"`
	AttributedPaths  []ConversionPath `json:"paths,omitempty"`
	Summary       AttributionSummary `json:"summary"`
}

// AttributionSummary gives high-level attribution metrics.
type AttributionSummary struct {
	AvgTouchpoints    float64 `json:"avg_touchpoints"`
	AvgTimeToConvert  string  `json:"avg_time_to_convert"`
	ClickConversions  uint64  `json:"click_conversions"`   // conversions with a click in path
	ViewConversions   uint64  `json:"view_conversions"`    // conversions with impression only
}

// GetAttributionReport queries ClickHouse for conversion paths and applies the attribution model.
func (s *Store) GetAttributionReport(ctx context.Context, campaignID uint64, from, to time.Time, model string, limit int) (*AttributionReport, error) {
	if limit <= 0 {
		limit = 100
	}

	// Get conversions with their device_id
	convRows, err := s.conn.Query(ctx, `
		SELECT event_time, device_id, request_id
		FROM bid_log
		WHERE campaign_id = ? AND event_type = 'conversion'
		  AND event_date >= ? AND event_date <= ?
		  AND device_id != ''
		ORDER BY event_time DESC
		LIMIT ?
	`, campaignID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("query conversions: %w", err)
	}
	defer convRows.Close()

	report := &AttributionReport{
		CampaignID: campaignID,
		Model:      model,
		Window:     fmt.Sprintf("%s to %s", from.Format("2006-01-02"), to.Format("2006-01-02")),
	}

	var totalTouchpoints int
	var totalSeconds float64

	for convRows.Next() {
		var convTime time.Time
		var deviceID, requestID string
		if err := convRows.Scan(&convTime, &deviceID, &requestID); err != nil {
			continue
		}

		// Look back 30 days for touchpoints from this device
		lookback := convTime.Add(-30 * 24 * time.Hour)
		touchRows, err := s.conn.Query(ctx, `
			SELECT event_time, event_type, creative_id, request_id
			FROM bid_log
			WHERE campaign_id = ? AND device_id = ?
			  AND event_type IN ('impression', 'click')
			  AND event_date >= ? AND event_time < ?
			ORDER BY event_time ASC
		`, campaignID, deviceID, lookback, convTime)
		if err != nil {
			continue
		}

		var touchpoints []Touchpoint
		for touchRows.Next() {
			var tp Touchpoint
			if err := touchRows.Scan(&tp.Time, &tp.EventType, &tp.CreativeID, &tp.RequestID); err != nil {
				continue
			}
			touchpoints = append(touchpoints, tp)
		}
		touchRows.Close()

		if len(touchpoints) == 0 {
			continue
		}

		// Apply attribution model
		credit := applyModel(model, touchpoints)

		path := ConversionPath{
			ConversionTime: convTime,
			DeviceID:       deviceID,
			CampaignID:     campaignID,
			Touchpoints:    touchpoints,
			Model:          model,
			Credit:         credit,
		}
		report.AttributedPaths = append(report.AttributedPaths, path)
		report.TotalConversions++

		totalTouchpoints += len(touchpoints)
		totalSeconds += convTime.Sub(touchpoints[0].Time).Seconds()

		// Check if path contains a click
		hasClick := false
		for _, tp := range touchpoints {
			if tp.EventType == "click" {
				hasClick = true
				break
			}
		}
		if hasClick {
			report.Summary.ClickConversions++
		} else {
			report.Summary.ViewConversions++
		}
	}

	if report.TotalConversions > 0 {
		report.Summary.AvgTouchpoints = float64(totalTouchpoints) / float64(report.TotalConversions)
		avgDuration := time.Duration(totalSeconds/float64(report.TotalConversions)) * time.Second
		report.Summary.AvgTimeToConvert = formatDuration(avgDuration)
	}

	return report, nil
}

func applyModel(model string, touchpoints []Touchpoint) []CreditedEvent {
	credits := make([]CreditedEvent, len(touchpoints))

	switch model {
	case ModelFirstClick:
		// 100% credit to first touchpoint
		for i, tp := range touchpoints {
			credits[i] = CreditedEvent{RequestID: tp.RequestID, EventType: tp.EventType, Credit: 0}
		}
		credits[0].Credit = 1.0

	case ModelLinear:
		// Equal credit to all touchpoints
		share := 1.0 / float64(len(touchpoints))
		for i, tp := range touchpoints {
			credits[i] = CreditedEvent{RequestID: tp.RequestID, EventType: tp.EventType, Credit: share}
		}

	default: // last_click
		// 100% credit to last touchpoint
		for i, tp := range touchpoints {
			credits[i] = CreditedEvent{RequestID: tp.RequestID, EventType: tp.EventType, Credit: 0}
		}
		credits[len(credits)-1].Credit = 1.0
	}

	return credits
}

func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}
