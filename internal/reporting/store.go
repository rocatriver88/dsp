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

func NewStore(addr, user, password string) (*Store, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{Database: "default", Username: user, Password: password},
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
		  exchange_id, request_id, geo_country, device_os, device_id, bid_price_cents, clear_price_cents,
		  charge_cents, event_type, loss_reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		evt.EventTime, evt.EventTime,
		evt.CampaignID, evt.CreativeID, evt.AdvertiserID,
		evt.ExchangeID, evt.RequestID,
		evt.GeoCountry, evt.DeviceOS, evt.DeviceID,
		evt.BidPriceCents, evt.ClearPriceCents, evt.ChargeCents,
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
	DeviceID        string // IDFA/GAID/OAID for attribution
	BidPriceCents   uint32
	ClearPriceCents uint32 // ADX cost (per impression, cents)
	ChargeCents     uint32 // Advertiser charge (per event, cents)
	EventType       string // bid, win, loss, impression, click, conversion
	LossReason      string
}

// CampaignStats holds aggregated stats for a campaign.
type CampaignStats struct {
	CampaignID  uint64  `json:"campaign_id"`
	Impressions    uint64  `json:"impressions"`
	Clicks         uint64  `json:"clicks"`
	Conversions    uint64  `json:"conversions"`
	Wins           uint64  `json:"wins"`
	Bids           uint64  `json:"bids"`
	SpendCents     uint64  `json:"spend_cents"`      // advertiser charge total (cents)
	AdxCostCents   uint64  `json:"adx_cost_cents"`   // ADX settlement cost (cents)
	ProfitCents    int64   `json:"profit_cents"`      // spend - adx_cost (cents)
	CTR            float64 `json:"ctr"`
	WinRate        float64 `json:"win_rate"`
	CVR            float64 `json:"cvr"`
	CPA            float64 `json:"cpa"`
}

// GetCampaignStats returns aggregated stats for a campaign within a date range.
func (s *Store) GetCampaignStats(ctx context.Context, campaignID uint64, from, to time.Time) (*CampaignStats, error) {
	stats := &CampaignStats{CampaignID: campaignID}

	row := s.conn.QueryRow(ctx, `
		SELECT
			countIf(event_type = 'impression') AS impressions,
			countIf(event_type = 'click') AS clicks,
			countIf(event_type = 'conversion') AS conversions,
			countIf(event_type = 'win') AS wins,
			countIf(event_type = 'bid') AS bids,
			sum(charge_cents) AS spend_cents,
			sumIf(clear_price_cents, event_type = 'win') AS adx_cost_cents
		FROM bid_log
		WHERE campaign_id = ? AND event_date >= ? AND event_date <= ?
	`, campaignID, from, to)

	if err := row.Scan(&stats.Impressions, &stats.Clicks, &stats.Conversions, &stats.Wins, &stats.Bids, &stats.SpendCents, &stats.AdxCostCents); err != nil {
		return nil, err
	}

	if stats.Impressions > 0 {
		stats.CTR = float64(stats.Clicks) / float64(stats.Impressions) * 100
	}
	if stats.Bids > 0 {
		stats.WinRate = float64(stats.Wins) / float64(stats.Bids) * 100
	}
	if stats.Clicks > 0 {
		stats.CVR = float64(stats.Conversions) / float64(stats.Clicks) * 100
	}
	if stats.Conversions > 0 {
		stats.CPA = float64(stats.SpendCents) / float64(stats.Conversions) / 100
	}
	stats.ProfitCents = int64(stats.SpendCents) - int64(stats.AdxCostCents)

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

// BidSimulation holds the result of a bid simulation query.
type BidSimulation struct {
	CurrentBidCPMCents    int     `json:"current_bid_cpm_cents"`
	SimulatedBidCPMCents  int     `json:"simulated_bid_cpm_cents"`
	TotalBids             uint64  `json:"total_bids"`
	ActualWins            uint64  `json:"actual_wins"`
	CurrentWinRate        float64 `json:"current_win_rate"`
	SimulatedWins         uint64  `json:"simulated_wins"`
	SimulatedWinRate      float64 `json:"simulated_win_rate"`
	SimulatedSpendCents   uint64  `json:"simulated_spend_cents"`
	MedianClearPriceCents uint32  `json:"median_clear_price_cents"`
	MaxClearPriceCents    uint32  `json:"max_clear_price_cents"`
	DataDays              int     `json:"data_days"`
}

// SimulateBid estimates win rate and spend for a hypothetical bid CPM.
func (s *Store) SimulateBid(ctx context.Context, campaignID uint64, simulatedCPMCents int) (*BidSimulation, error) {
	var result BidSimulation
	result.SimulatedBidCPMCents = simulatedCPMCents
	result.DataDays = 7

	err := s.conn.QueryRow(ctx, `
		SELECT
			count()                                                      AS total_bids,
			countIf(event_type = 'win')                                  AS actual_wins,
			countIf(clear_price_cents > 0 AND clear_price_cents <= ?)    AS simulated_wins,
			sumIf(clear_price_cents, clear_price_cents > 0 AND clear_price_cents <= ?) AS simulated_spend_cents,
			toUInt32(quantileExactIf(0.5)(clear_price_cents, clear_price_cents > 0)) AS median_clear_price,
			max(clear_price_cents)                                       AS max_clear_price
		FROM bid_log
		WHERE campaign_id = ?
		  AND event_date >= today() - 7
	`, simulatedCPMCents, simulatedCPMCents, campaignID).Scan(
		&result.TotalBids,
		&result.ActualWins,
		&result.SimulatedWins,
		&result.SimulatedSpendCents,
		&result.MedianClearPriceCents,
		&result.MaxClearPriceCents,
	)
	if err != nil {
		return nil, fmt.Errorf("simulate bid: %w", err)
	}

	if result.TotalBids > 0 {
		result.CurrentWinRate = float64(result.ActualWins) / float64(result.TotalBids)
		result.SimulatedWinRate = float64(result.SimulatedWins) / float64(result.TotalBids)
	}

	return &result, nil
}

func (s *Store) Close() error {
	return s.conn.Close()
}
