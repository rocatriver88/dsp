package bidder

import (
	"context"
	"fmt"
	"time"

	"github.com/heartgryphon/dsp/internal/antifraud"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/events"
	"github.com/prebid/openrtb/v20/openrtb2"
)

// Engine is the production bidder that uses CampaignLoader + Redis budget/freq.
type Engine struct {
	loader   *CampaignLoader
	budget   *budget.Service
	producer *events.Producer       // nil if Kafka unavailable
	fraud    *antifraud.Filter      // nil to skip fraud checks
}

func NewEngine(loader *CampaignLoader, budgetSvc *budget.Service, producer *events.Producer, fraud *antifraud.Filter) *Engine {
	return &Engine{
		loader:   loader,
		budget:   budgetSvc,
		producer: producer,
		fraud:    fraud,
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

	// Extract request signals
	var geoCountry, deviceOS, userID string
	if req.Device != nil {
		deviceOS = req.Device.OS
		if req.Device.Geo != nil {
			geoCountry = req.Device.Geo.Country
		}
		userID = req.Device.IFA // use IDFA/GAID as user ID
	}

	// Anti-fraud Layer 1 check
	if e.fraud != nil {
		var ip, ua string
		if req.Device != nil {
			ip = req.Device.IP
			ua = req.Device.UA
		}
		result := e.fraud.Check(ctx, ip, ua, userID)
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
	for _, c := range candidates {
		if !matchesTargeting(c, geoCountry, deviceOS) {
			continue
		}
		if len(c.Creatives) == 0 {
			continue
		}
		if best == nil || c.EffectiveBidCPMCents(0, 0) > best.EffectiveBidCPMCents(0, 0) {
			best = c
		}
	}

	if best == nil {
		return nil, nil
	}

	// Redis pipeline: budget + frequency check (single RTT)
	// Budget amounts are in per-impression cents (not CPM cents)
	bidAmountCents := int64(best.EffectiveBidCPMCents(0, 0)) / 1000 // CPM cents → per-impression cents
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

	// Pick first creative
	creative := best.Creatives[0]
	bidPrice := float64(best.EffectiveBidCPMCents(0, 0)) / 100.0 / 1000.0 // CPM cents → dollars per impression
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
		Cur: "USD",
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

// matchesTargetingForCampaign is used by the original Bidder (Phase 0 compat).
func matchesCampaignTargeting(c *campaign.Targeting, geo, os string) bool {
	if len(c.Geo) > 0 && geo != "" {
		if !contains(c.Geo, geo) {
			return false
		}
	}
	if len(c.OS) > 0 && os != "" {
		if !contains(c.OS, os) {
			return false
		}
	}
	return true
}
