package reporting

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type Store struct {
	conn driver.Conn
}

func NewStore(addr string) (*Store, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{Database: "default"},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
	})
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("clickhouse ping: %w", err)
	}
	return &Store{conn: conn}, nil
}

// InsertEvent inserts a single event into bid_log.
func (s *Store) InsertEvent(ctx context.Context, evt BidEvent) error {
	return s.conn.Exec(ctx,
		`INSERT INTO bid_log (event_date, event_time, campaign_id, creative_id, advertiser_id,
		  exchange_id, request_id, geo_country, device_os, bid_price_cents, clear_price_cents,
		  event_type, loss_reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		evt.EventTime, evt.EventTime,
		evt.CampaignID, evt.CreativeID, evt.AdvertiserID,
		evt.ExchangeID, evt.RequestID,
		evt.GeoCountry, evt.DeviceOS,
		evt.BidPriceCents, evt.ClearPriceCents,
		evt.EventType, evt.LossReason,
	)
}

// BidEvent represents a row in bid_log.
type BidEvent struct {
	EventTime       time.Time
	CampaignID      uint64
	CreativeID      uint64
	AdvertiserID    uint64
	ExchangeID      string
	RequestID       string
	GeoCountry      string
	DeviceOS        string
	BidPriceCents   uint32
	ClearPriceCents uint32
	EventType       string // bid, win, loss, impression, click
	LossReason      string
}

// CampaignStats holds aggregated stats for a campaign.
type CampaignStats struct {
	CampaignID  uint64  `json:"campaign_id"`
	Impressions uint64  `json:"impressions"`
	Clicks      uint64  `json:"clicks"`
	Wins        uint64  `json:"wins"`
	Bids        uint64  `json:"bids"`
	SpendCents  uint64  `json:"spend_cents"`
	CTR         float64 `json:"ctr"`
	WinRate     float64 `json:"win_rate"`
}

// GetCampaignStats returns aggregated stats for a campaign within a date range.
func (s *Store) GetCampaignStats(ctx context.Context, campaignID uint64, from, to time.Time) (*CampaignStats, error) {
	stats := &CampaignStats{CampaignID: campaignID}

	row := s.conn.QueryRow(ctx, `
		SELECT
			countIf(event_type = 'impression') AS impressions,
			countIf(event_type = 'click') AS clicks,
			countIf(event_type = 'win') AS wins,
			countIf(event_type = 'bid') AS bids,
			sumIf(clear_price_cents, event_type = 'win') AS spend_cents
		FROM bid_log
		WHERE campaign_id = ? AND event_date >= ? AND event_date <= ?
	`, campaignID, from, to)

	if err := row.Scan(&stats.Impressions, &stats.Clicks, &stats.Wins, &stats.Bids, &stats.SpendCents); err != nil {
		return nil, err
	}

	if stats.Impressions > 0 {
		stats.CTR = float64(stats.Clicks) / float64(stats.Impressions) * 100
	}
	if stats.Bids > 0 {
		stats.WinRate = float64(stats.Wins) / float64(stats.Bids) * 100
	}

	return stats, nil
}

// HourlyStats holds per-hour stats.
type HourlyStats struct {
	Hour        int    `json:"hour"`
	Impressions uint64 `json:"impressions"`
	Clicks      uint64 `json:"clicks"`
	SpendCents  uint64 `json:"spend_cents"`
}

// GetHourlyStats returns hourly breakdown for a campaign on a given date.
func (s *Store) GetHourlyStats(ctx context.Context, campaignID uint64, date time.Time) ([]HourlyStats, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT
			toHour(event_time) AS hour,
			countIf(event_type = 'impression') AS impressions,
			countIf(event_type = 'click') AS clicks,
			sumIf(clear_price_cents, event_type = 'win') AS spend_cents
		FROM bid_log
		WHERE campaign_id = ? AND event_date = ?
		GROUP BY hour ORDER BY hour
	`, campaignID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []HourlyStats
	for rows.Next() {
		var h HourlyStats
		if err := rows.Scan(&h.Hour, &h.Impressions, &h.Clicks, &h.SpendCents); err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, nil
}

// BidDetail holds a single bid record for transparency reporting.
type BidDetail struct {
	Time            time.Time `json:"time"`
	RequestID       string    `json:"request_id"`
	EventType       string    `json:"event_type"`
	BidPriceCents   uint32    `json:"bid_price_cents"`
	ClearPriceCents uint32    `json:"clear_price_cents"`
	GeoCountry      string    `json:"geo_country"`
	DeviceOS        string    `json:"device_os"`
	LossReason      string    `json:"loss_reason,omitempty"`
}

// GetBidTransparency returns individual bid records for transparency reporting.
func (s *Store) GetBidTransparency(ctx context.Context, campaignID uint64, from, to time.Time, limit, offset int) ([]BidDetail, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.conn.Query(ctx, `
		SELECT event_time, request_id, event_type, bid_price_cents, clear_price_cents,
		       geo_country, device_os, loss_reason
		FROM bid_log
		WHERE campaign_id = ? AND event_date >= ? AND event_date <= ?
		ORDER BY event_time DESC
		LIMIT ? OFFSET ?
	`, campaignID, from, to, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []BidDetail
	for rows.Next() {
		var d BidDetail
		if err := rows.Scan(&d.Time, &d.RequestID, &d.EventType, &d.BidPriceCents,
			&d.ClearPriceCents, &d.GeoCountry, &d.DeviceOS, &d.LossReason); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, nil
}

// GeoStats holds per-country stats.
type GeoStats struct {
	Country     string `json:"country"`
	Impressions uint64 `json:"impressions"`
	Clicks      uint64 `json:"clicks"`
	SpendCents  uint64 `json:"spend_cents"`
}

// GetGeoBreakdown returns stats broken down by country.
func (s *Store) GetGeoBreakdown(ctx context.Context, campaignID uint64, from, to time.Time) ([]GeoStats, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT geo_country,
			countIf(event_type = 'impression') AS impressions,
			countIf(event_type = 'click') AS clicks,
			sumIf(clear_price_cents, event_type = 'win') AS spend_cents
		FROM bid_log
		WHERE campaign_id = ? AND event_date >= ? AND event_date <= ? AND geo_country != ''
		GROUP BY geo_country ORDER BY impressions DESC
	`, campaignID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []GeoStats
	for rows.Next() {
		var g GeoStats
		if err := rows.Scan(&g.Country, &g.Impressions, &g.Clicks, &g.SpendCents); err != nil {
			return nil, err
		}
		result = append(result, g)
	}
	return result, nil
}

func (s *Store) Close() error {
	return s.conn.Close()
}
