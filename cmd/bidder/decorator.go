package main

import (
	"fmt"
	"math"
	"strconv"

	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/prebid/openrtb/v20/openrtb2"
)

// decorateBidResponse populates NURL (win notice with HMAC) and injects the
// click tracker on the bid's AdM. Shared by direct /bid and /bid/{exchange_id}
// so both paths produce identical win/click wiring.
//
// The NURL and click URL carry creative_id and bid_price_cents as signed
// params so handleWin/handleClick can record the true bid-time values
// without recomputing from current campaign state (which may have shifted
// between bid and win).
//
// Pre-condition: resp.SeatBid[0].Bid[0].CrID is set to the real selected
// creative ID by Engine.Bid (see internal/bidder/engine.go CrID assignment).
func decorateBidResponse(resp *openrtb2.BidResponse, req *openrtb2.BidRequest, baseURL, hmacSecret string) {
	if resp == nil || len(resp.SeatBid) == 0 || len(resp.SeatBid[0].Bid) == 0 {
		return
	}
	bid := &resp.SeatBid[0].Bid[0]

	var geo, osName string
	if req.Device != nil {
		osName = req.Device.OS
		if req.Device.Geo != nil {
			geo = req.Device.Geo.Country
		}
	}

	creativeID := bid.CrID
	// Use math.Round, not int64() truncation (CEO Finding #4).
	// bid.Price is in CPM dollars; e.g. $0.00495 * 100 = 0.495 → rounds to 0 cents
	// (acceptable for sub-cent), but $0.01495 → 1.495 → rounds to 1 (not 0).
	// Truncation systematically under-counts pennies at the cent boundary.
	bidPriceCents := strconv.FormatInt(int64(math.Round(bid.Price*100)), 10)

	// F6 (#27): sign a handler-type discriminator as the FIRST variadic
	// param so a token captured from /win can't be replayed on /click or
	// /convert (budget drain + attribution poisoning). The bid-time params
	// (campID, reqID, creativeID, bidPriceCents) are otherwise identical
	// across win/click, and pre-F6 a single token validated on all three
	// endpoints — an attacker with URL visibility (ADX logs, proxy)
	// could charge CPC and emit fraudulent conversions within the 5-min
	// TTL. Post-F6, each handler validates with its own type string.
	winToken := auth.GenerateToken(hmacSecret, "win", bid.CID, req.ID, creativeID, bidPriceCents)
	clickToken := auth.GenerateToken(hmacSecret, "click", bid.CID, req.ID, creativeID, bidPriceCents)

	bid.NURL = fmt.Sprintf(
		"%s/win?campaign_id=%s&price=${AUCTION_PRICE}&request_id=%s&creative_id=%s&bid_price_cents=%s&geo=%s&os=%s&token=%s",
		baseURL, bid.CID, req.ID, creativeID, bidPriceCents, geo, osName, winToken,
	)
	// Click URL carries bid_price_cents so handleClick can round-trip the
	// HMAC-signed value for validation. Without it the token (which is
	// signed over creative_id + bid_price_cents) cannot be verified against
	// the click URL's parameters — every click would 403. See Task 4 plan
	// correction note.
	clickURL := fmt.Sprintf(
		"%s/click?campaign_id=%s&request_id=%s&creative_id=%s&bid_price_cents=%s&token=%s",
		baseURL, bid.CID, req.ID, creativeID, bidPriceCents, clickToken,
	)
	bid.AdM = injectClickTracker(bid.AdM, clickURL)
}
