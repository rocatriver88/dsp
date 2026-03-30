package bidder

import (
	"context"
	"fmt"
	"time"

	"github.com/heartgryphon/dsp/internal/antifraud"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/events"
	"github.com/prebid/openrtb/v20/openrtb2"
)

// Engine is the production bidder that uses CampaignLoader + Redis budget/freq.
//
// Bid flow:
//   BidRequest → Device check → Fraud check → GDPR → Campaign match
//   → Pacing check (ShouldBid) → Budget+Freq pipeline → AdjustedBid
//   → BidResponse + async Kafka event
type Engine struct {
	loader     *CampaignLoader
	budget     *budget.Service
	strategy   *BidStrategy             // pacing + win-rate bid adjustment
	statsCache *StatsCache              // ClickHouse CTR/CVR cache, nil if unavailable
	producer   *events.Producer         // nil if Kafka unavailable
	fraud      *antifraud.Filter        // nil to skip fraud checks
}

func NewEngine(loader *CampaignLoader, budgetSvc *budget.Service, strategy *BidStrategy, statsCache *StatsCache, producer *events.Producer, fraud *antifraud.Filter) *Engine {
	return &Engine{
		loader:     loader,
		budget:     budgetSvc,
		strategy:   strategy,
		statsCache: statsCache,
		producer:   producer,
		fraud:      fraud,
	}
}

// Bid evaluates a bid request and returns a response, or nil for no-bid.
func (e *Engine) Bid(ctx context.Context, req *openrtb2.BidRequest) (*openrtb2.BidResponse, error) {
	if req == nil || len(req.Imp) == 0 {
		return nil, nil
	}

	imp := req.Imp[0]
	if imp.Banner == nil && imp.Video == nil && imp.Native == nil {
		return nil, nil
	}

	// No device info = likely bot or malformed request, no-bid
	if req.Device == nil {
		return nil, nil
	}

	// Extract request signals
	geoCountry := ""
	deviceOS := req.Device.OS
	userID := req.Device.IFA // IDFA/GAID
	if req.Device.Geo != nil {
		geoCountry = req.Device.Geo.Country
	}

	// Anti-fraud Layer 1 check
	if e.fraud != nil {
		result := e.fraud.Check(ctx, req.Device.IP, req.Device.UA, userID)
		if !result.Allowed {
			return nil, nil // silently no-bid on fraud
		}
	}

	// GDPR check
	gdprApplies := false
	if req.Regs != nil && req.Regs.GDPR != nil && *req.Regs.GDPR == 1 {
		gdprApplies = true
	}
	if gdprApplies {
		userID = "" // no user-level tracking under GDPR
	}

	// Find matching campaigns from in-memory cache
	candidates := e.loader.GetActiveCampaigns()
	var best *LoadedCampaign
	var bestBidCPM int
	for _, c := range candidates {
		if !matchesTargeting(c, geoCountry, deviceOS) {
			continue
		}
		if len(c.Creatives) == 0 {
			continue
		}

		// Use real CTR/CVR from ClickHouse cache if available, else defaults
		var predictedCTR, predictedCVR float64
		if e.statsCache != nil {
			cached := e.statsCache.Get(ctx, c.ID)
			predictedCTR = cached.CTR
			predictedCVR = cached.CVR
		}
		bidCPM := c.EffectiveBidCPMCents(predictedCTR, predictedCVR)
		if e.strategy != nil {
			bidCPM = e.strategy.AdjustedBid(ctx, c.ID, bidCPM, c.BudgetDailyCents)
		}

		if best == nil || bidCPM > bestBidCPM {
			best = c
			bestBidCPM = bidCPM
		}
	}

	if best == nil {
		return nil, nil
	}

	// Pacing check: should we participate in this auction?
	if e.strategy != nil && !e.strategy.ShouldBid(ctx, best.ID, best.BudgetDailyCents) {
		return nil, nil
	}

	// Redis pipeline: budget + frequency check (single RTT)
	// Budget amounts are in per-impression cents (not CPM cents)
	bidAmountCents := int64(bestBidCPM) / 1000 // CPM cents → per-impression cents
	if bidAmountCents < 1 {
		bidAmountCents = 1 // minimum 1 cent per impression
	}
	freqCap := 0
	freqPeriod := 24
	if best.Targeting.FrequencyCap != nil {
		freqCap = best.Targeting.FrequencyCap.Count
		freqPeriod = best.Targeting.FrequencyCap.PeriodHours
	}

	budgetOK, freqOK, err := e.budget.PipelineCheck(ctx, best.ID, userID, bidAmountCents, freqCap, freqPeriod)
	if err != nil {
		// Redis down: fail-closed, no bid
		return nil, fmt.Errorf("redis check: %w", err)
	}
	if !budgetOK || !freqOK {
		return nil, nil
	}

	// Record bid for strategy tracking
	if e.strategy != nil {
		go e.strategy.RecordBid(ctx, best.ID)
	}

	// Pick first creative
	creative := best.Creatives[0]
	// Markup: bid to ADX at 90% of adjusted bid (platform keeps 10%)
	bidPrice := float64(bestBidCPM) * 0.90 / 100.0 / 1000.0 // CPM cents × 0.90 → dollars per impression
	bidID := fmt.Sprintf("bid-%d-%d", best.ID, time.Now().UnixNano())

	resp := &openrtb2.BidResponse{
		ID: req.ID,
		SeatBid: []openrtb2.SeatBid{{
			Bid: []openrtb2.Bid{{
				ID:      bidID,
				ImpID:   imp.ID,
				Price:   bidPrice,
				AdM:     creative.AdMarkup,
				ADomain: []string{creative.DestinationURL},
				CID:     fmt.Sprintf("%d", best.ID),
				CrID:    fmt.Sprintf("%d", creative.ID),
			}},
			Seat: fmt.Sprintf("campaign-%d", best.ID),
		}},
		Cur: "CNY",
	}

	// Emit bid event to Kafka (async, non-blocking)
	if e.producer != nil {
		go e.producer.SendBid(ctx, events.Event{
			CampaignID:   best.ID,
			CreativeID:   creative.ID,
			AdvertiserID: best.AdvertiserID,
			RequestID:    req.ID,
			BidPrice:     bidPrice,
			GeoCountry:   geoCountry,
			DeviceOS:     deviceOS,
			DeviceID:     userID, // IDFA/GAID for attribution
		})
	}

	return resp, nil
}

func matchesTargeting(c *LoadedCampaign, geo, os string) bool {
	t := c.Targeting

	if len(t.Geo) > 0 && geo != "" {
		if !contains(t.Geo, geo) {
			return false
		}
	}

	if len(t.OS) > 0 && os != "" {
		if !contains(t.OS, os) {
			return false
		}
	}

	if len(t.Device) > 0 {
		// Device targeting checked at impression level if needed
	}

	return true
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
