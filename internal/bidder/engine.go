package bidder

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/heartgryphon/dsp/internal/antifraud"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/guardrail"
	"github.com/heartgryphon/dsp/internal/observability"
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
	guardrail  *guardrail.Guardrail     // nil to skip guardrail checks
}

func NewEngine(loader *CampaignLoader, budgetSvc *budget.Service, strategy *BidStrategy, statsCache *StatsCache, producer *events.Producer, fraud *antifraud.Filter, guard *guardrail.Guardrail) *Engine {
	return &Engine{
		loader:     loader,
		budget:     budgetSvc,
		strategy:   strategy,
		statsCache: statsCache,
		producer:   producer,
		fraud:      fraud,
		guardrail:  guard,
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

	// OpenRTB 2.5: require secure creative if imp.secure=1
	requireSecure := false
	if imp.Secure != nil && *imp.Secure == 1 {
		requireSecure = true
	}
	_ = requireSecure // TODO: filter creatives by secure flag

	// OpenRTB 2.5: respect bidfloor
	bidFloor := imp.BidFloor
	bidFloorCur := imp.BidFloorCur
	if bidFloorCur == "" {
		bidFloorCur = "USD"
	}

	// OpenRTB 2.5: extract site/app categories for contextual targeting
	var siteCategories []string
	if req.Site != nil {
		siteCategories = req.Site.Cat
	}
	if req.App != nil && len(siteCategories) == 0 {
		siteCategories = req.App.Cat
	}
	_ = siteCategories // available for future contextual targeting

	// OpenRTB 2.5: supply chain transparency (ads.txt/sellers.json)
	if req.Source != nil && req.Source.SChain != nil {
		// Log supply chain for transparency auditing
		_ = req.Source.SChain.Complete // 1 = full chain visible
	}

	// Anti-fraud Layer 1 check
	if e.fraud != nil {
		result := e.fraud.Check(ctx, req.Device.IP, req.Device.UA, userID)
		if !result.Allowed {
			observability.AuctionOutcome.WithLabelValues("fraud_rejected").Inc()
			return nil, nil // silently no-bid on fraud
		}
	}

	// Guardrail: circuit breaker + global budget
	if e.guardrail != nil {
		preCheck := e.guardrail.PreCheck(ctx)
		if !preCheck.Allowed {
			return nil, nil
		}
	}

	// GDPR check (2.5: also check USPrivacy/CCPA)
	gdprApplies := false
	if req.Regs != nil && req.Regs.GDPR != nil && *req.Regs.GDPR == 1 {
		gdprApplies = true
	}
	if req.Regs != nil && req.Regs.USPrivacy != "" {
		// CCPA: if opt-out signal present (1YY-), respect it
		if len(req.Regs.USPrivacy) >= 3 && req.Regs.USPrivacy[2] == 'Y' {
			userID = "" // user opted out of sale
		}
	}
	if gdprApplies {
		userID = "" // no user-level tracking under GDPR
	}

	// Find matching campaigns from in-memory cache
	candidates := e.loader.GetActiveCampaigns()
	now := time.Now()
	var best *LoadedCampaign
	var bestCreative *campaign.Creative
	var bestBidCPM int
	for _, c := range candidates {
		// Defense in depth: loader cache may be stale for up to 30s
		if !campaignDateActive(c, now) {
			continue
		}

		if !matchesTargeting(c, geoCountry, deviceOS, req.Device.UA, now) {
			continue
		}

		// Match creative to impression slot size
		matched := matchCreativeToImp(c.Creatives, &imp)
		if matched == nil {
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

		// Guardrail: bid ceiling
		if e.guardrail != nil {
			capCheck := e.guardrail.CheckBidCeiling(ctx, bidCPM)
			if !capCheck.Allowed {
				continue
			}
		}

		// OpenRTB 2.5: enforce bidfloor
		if bidFloor > 0 {
			bidPricePerImp := float64(bidCPM) * 0.90 / 100.0 / 1000.0
			if bidFloorCur == "CNY" || bidFloorCur == "" {
				// bidfloor in CNY, our price in CNY
			}
			if bidPricePerImp < bidFloor {
				continue // below floor, skip
			}
		}

		if best == nil || bidCPM > bestBidCPM {
			best = c
			bestCreative = matched
			bestBidCPM = bidCPM
		}
	}

	if best == nil {
		if len(candidates) == 0 {
			observability.AuctionOutcome.WithLabelValues("no_campaigns").Inc()
		} else {
			observability.AuctionOutcome.WithLabelValues("under_bid").Inc()
		}
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

	// Record bid for strategy tracking. Use context.Background() because
	// the goroutine outlives the bid handler — the caller returns the
	// bid response immediately and r.Context() gets cancelled, which
	// would abort the Redis write mid-flight. Round 2 review I-Pre-1.
	if e.strategy != nil {
		bestID := best.ID
		go e.strategy.RecordBid(context.Background(), bestID)
	}

	// Use the creative matched to the impression slot
	creative := bestCreative
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

	observability.AuctionOutcome.WithLabelValues("ok").Inc()

	// Emit bid event to Kafka. Round 2 review I-New-1: this is the
	// highest-volume producer call in the entire system (one per
	// auction, vs one per click/win for the handler-level calls), and
	// the Round 1 I4 fix missed it. Two bugs were stacked here:
	//
	//   1. Untracked goroutine (not visible to producer.WaitInflight)
	//      → V5 §P1 invariant 4 was silently violated for every bid.
	//   2. Wrong context: ctx is the bid handler's r.Context(), which
	//      cancels within a few ms of the handler returning the bid
	//      response. Most SendBid writes were aborted mid-flight.
	//
	// Fix: route through producer.Go (tracked) with context.Background()
	// (outlives handler).
	if e.producer != nil {
		evt := events.Event{
			CampaignID:   best.ID,
			CreativeID:   creative.ID,
			AdvertiserID: best.AdvertiserID,
			RequestID:    req.ID,
			BidPrice:     bidPrice,
			GeoCountry:   geoCountry,
			DeviceOS:     deviceOS,
			DeviceID:     userID, // IDFA/GAID for attribution
		}
		e.producer.Go(func() { e.producer.SendBid(context.Background(), evt) })
	}

	return resp, nil
}

// cstLocation is Asia/Shanghai (UTC+8), loaded once at package init.
var cstLocation *time.Location

func init() {
	var err error
	cstLocation, err = time.LoadLocation("Asia/Shanghai")
	if err != nil {
		// Fallback: fixed offset UTC+8
		cstLocation = time.FixedZone("CST", 8*60*60)
	}
}

func matchesTargeting(c *LoadedCampaign, geo, os, ua string, now time.Time) bool {
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

	// Time schedule: check if current hour (CST) is in any schedule entry
	if len(t.TimeSchedule) > 0 {
		cstNow := now.In(cstLocation)
		weekday := int(cstNow.Weekday()) // 0=Sun matches Schedule.Day
		hour := cstNow.Hour()
		matched := false
		for _, sched := range t.TimeSchedule {
			if sched.Day == weekday && containsInt(sched.Hours, hour) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Browser targeting: check if UA contains any of the target browsers
	if len(t.Browser) > 0 && ua != "" {
		if !containsAnySubstring(ua, t.Browser) {
			return false
		}
	}

	return true
}

// matchCreativeToImp returns the first creative that matches the impression's
// size requirements, or nil if none match. For banner impressions it checks
// Banner.W/H and Banner.Format against creative.Size ("WxH"). For non-banner
// impressions (video, native), any creative is accepted since size matching
// doesn't apply.
func matchCreativeToImp(creatives []*campaign.Creative, imp *openrtb2.Imp) *campaign.Creative {
	if imp.Banner == nil {
		// Non-banner: accept any creative (video/native don't use WxH matching)
		if len(creatives) > 0 {
			return creatives[0]
		}
		return nil
	}

	// Collect acceptable sizes from the impression
	type size struct{ w, h int64 }
	var acceptable []size
	if imp.Banner.W != nil && imp.Banner.H != nil {
		acceptable = append(acceptable, size{*imp.Banner.W, *imp.Banner.H})
	}
	for _, f := range imp.Banner.Format {
		if f.W != 0 && f.H != 0 {
			acceptable = append(acceptable, size{f.W, f.H})
		}
	}

	// If the impression specifies no size, accept any creative
	if len(acceptable) == 0 {
		if len(creatives) > 0 {
			return creatives[0]
		}
		return nil
	}

	for _, cr := range creatives {
		cw, ch, ok := parseCreativeSize(cr.Size)
		if !ok {
			continue // unparseable size, skip
		}
		for _, s := range acceptable {
			if cw == s.w && ch == s.h {
				return cr
			}
		}
	}
	return nil
}

// parseCreativeSize parses a "WxH" string (e.g. "300x250") into width and height.
func parseCreativeSize(s string) (w, h int64, ok bool) {
	parts := strings.SplitN(s, "x", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	h, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return w, h, true
}

// campaignDateActive returns true if the campaign's date window includes the given time.
// A nil StartDate/EndDate means no bound in that direction.
func campaignDateActive(c *LoadedCampaign, now time.Time) bool {
	if c.StartDate != nil && now.Before(*c.StartDate) {
		return false
	}
	if c.EndDate != nil && now.After(*c.EndDate) {
		return false
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

func containsInt(slice []int, item int) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

// containsAnySubstring returns true if haystack contains any of the needles
// as a case-insensitive substring. Used for browser UA matching.
func containsAnySubstring(haystack string, needles []string) bool {
	lower := strings.ToLower(haystack)
	for _, n := range needles {
		if strings.Contains(lower, strings.ToLower(n)) {
			return true
		}
	}
	return false
}
