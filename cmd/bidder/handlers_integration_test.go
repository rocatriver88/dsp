//go:build integration

package main

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/antifraud"
	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/config"
	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/exchange"
	"github.com/heartgryphon/dsp/internal/guardrail"
	"github.com/heartgryphon/dsp/internal/qaharness"
)

const (
	qaHMACSecret = "qa-test-secret"
	qaPublicURL  = "http://test-bidder"
)

// handlerFixture wires a TestHarness to a real *Deps + httptest.Server running
// the production mux built by RegisterRoutes. Tests construct the fixture,
// seed their rows, then call Start() so the CampaignLoader's synchronous
// initial full-load picks up the seeded rows — the production handlers then
// see the same *LoadedCampaign that a live bidder would.
type handlerFixture struct {
	*qaharness.TestHarness
	deps   *Deps
	loader *bidder.CampaignLoader
	srv    *httptest.Server
}

// newHandlerFixture builds every collaborator and wires them into a *Deps and
// an httptest.NewServer. The loader is NOT started yet — callers must seed
// advertisers/campaigns/creatives first, then call f.Start(t).
func newHandlerFixture(t *testing.T) *handlerFixture {
	t.Helper()
	h := qaharness.New(t)

	loader := bidder.NewCampaignLoader(h.PG, h.RDB)
	producer := events.NewProducer(h.Env.KafkaBrokers, t.TempDir())
	// Mirror the production shutdown ordering (cmd/bidder/main.go:255-260):
	// drain inflight goroutines via WaitInflight before closing Kafka writers.
	// Without this, a handler that spawns `producer.Go(SendClick)` can still
	// be writing when Close() runs, which either drops the message (Async=true
	// + unsynchronised close) or hangs the test on shutdown. See the long
	// comment at handlers_integration_test.go:444 for the original P1-3
	// fallout that motivated this ordering fix.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = producer.WaitInflight(ctx)
		producer.Close()
	})

	budgetSvc := budget.New(h.RDB)
	strategySvc := bidder.NewBidStrategy(h.RDB)
	statsCache := bidder.NewStatsCache(h.RDB, nil, loader.GetActiveCampaigns)
	fraudFilter := antifraud.NewFilter(h.RDB)
	guard := guardrail.New(h.RDB, guardrail.Config{})
	eng := bidder.NewEngine(loader, budgetSvc, strategySvc, statsCache, producer, fraudFilter, guard)

	deps := &Deps{
		Engine:           eng,
		BudgetSvc:        budgetSvc,
		StrategySvc:      strategySvc,
		Loader:           loader,
		Producer:         producer,
		RDB:              h.RDB,
		ExchangeRegistry: exchange.DefaultRegistry(qaPublicURL),
		Guard:            guard,
		HMACSecret:       qaHMACSecret,
		PublicURL:        qaPublicURL,
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, deps)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &handlerFixture{
		TestHarness: h,
		deps:        deps,
		loader:      loader,
		srv:         srv,
	}
}

// Start begins the loader with a context cancelled on test cleanup. Must be
// called after all SeedCampaign/SeedCreative calls so the loader's initial
// fullLoad sees them — the loader is how handleWin/handleClick determine
// billing model, so CPC tests MUST Start() after seeding.
func (f *handlerFixture) Start(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(f.Ctx)
	if err := f.loader.Start(ctx); err != nil {
		cancel()
		t.Fatalf("loader start: %v", err)
	}
	t.Cleanup(func() {
		f.loader.Stop()
		cancel()
	})
}

// buildWinURL constructs the /win query string the way an exchange would,
// with a valid HMAC token. Post-decorator refactor the signature is
// (campaignID, requestID, creativeID, bidPriceCents); existing tests that
// don't carry creative metadata sign with empty strings and the handler's
// ValidateToken naturally reads "" back from URL for the same params.
func (f *handlerFixture) buildWinURL(campaignID int64, requestID string, price float64, geo, osName string) string {
	campIDStr := fmt.Sprintf("%d", campaignID)
	token := auth.GenerateToken(qaHMACSecret, campIDStr, requestID, "", "")
	q := url.Values{}
	q.Set("campaign_id", campIDStr)
	q.Set("price", fmt.Sprintf("%f", price))
	q.Set("request_id", requestID)
	q.Set("creative_id", "")
	q.Set("bid_price_cents", "")
	q.Set("geo", geo)
	q.Set("os", osName)
	q.Set("token", token)
	return f.srv.URL + "/win?" + q.Encode()
}

func (f *handlerFixture) buildClickURL(campaignID int64, requestID string) string {
	campIDStr := fmt.Sprintf("%d", campaignID)
	// Token signature matches handleClick's 4-param ValidateToken
	// (campaignID, requestID, creativeID, bidPriceCents). creative_id
	// is placed in the URL as "" so the handler validates against the
	// same empty string signed here. bid_price_cents is NOT in the URL
	// (handleClick reads "" unconditionally), and the generator above
	// uses "" so the HMACs line up.
	token := auth.GenerateToken(qaHMACSecret, campIDStr, requestID, "", "")
	q := url.Values{}
	q.Set("campaign_id", campIDStr)
	q.Set("request_id", requestID)
	q.Set("creative_id", "")
	q.Set("token", token)
	return f.srv.URL + "/click?" + q.Encode()
}

func (f *handlerFixture) buildConvertURL(campaignID int64, requestID string) string {
	campIDStr := fmt.Sprintf("%d", campaignID)
	// Convert URLs are not produced by the decorator. handleConvert
	// validates with (campaignID, requestID, creativeID, "") — read
	// creative_id from the URL (absent = ""), bid_price_cents hardcoded
	// to "". Sign with empty strings on both sides.
	token := auth.GenerateToken(qaHMACSecret, campIDStr, requestID, "", "")
	q := url.Values{}
	q.Set("campaign_id", campIDStr)
	q.Set("request_id", requestID)
	q.Set("token", token)
	return f.srv.URL + "/convert?" + q.Encode()
}

// readBody drains and returns the response body as a string. The caller keeps
// responsibility for checking resp.StatusCode before calling.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

// ------------------------------------------------------------
// Scenario 22 — /win CPM happy path deducts the clear price and
// emits exactly one win + impression event.
// ------------------------------------------------------------
//
// REGRESSION SENTINEL: P1-5 CPM win billing discipline
// (docs/testing-strategy-bidder.md §3 P1). Guards three invariants
// at once: (a) advertiser billing math (priceCents = int64(price/0.9*100)),
// (b) the warm-up 503 fallback when the loader hasn't materialised
// the campaign yet (cmd/bidder/main.go:449-453), and (c) the C1
// strategy:wins INCR via bare goroutine at cmd/bidder/main.go:505.
//
// Currently -skipped in CI (handlers_integration_test.go line 250 poll
// fails in ubuntu-latest but passes locally). Annotation stays so future
// maintainers have the context when the skip is lifted or the test is
// re-structured.
func TestHandlers_WinNormalCPM(t *testing.T) {
	f := newHandlerFixture(t)
	advID := f.SeedAdvertiser("win-cpm")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-win-cpm",
		BidCPMCents:      2000,
		BudgetDailyCents: 1_000_000,
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	beforeBudget := f.GetBudgetRemaining(campID)
	reqID := fmt.Sprintf("qa-win-%d", time.Now().UnixNano())
	price := 0.05 // clear price in CNY per impression

	winURL := f.buildWinURL(campID, reqID, price, "CN", "iOS")
	resp, err := http.Get(winURL)
	if err != nil {
		t.Fatalf("GET /win: %v", err)
	}
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/win status: want 200, got %d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"ok"`) {
		t.Errorf("/win body: want contains \"ok\", got %s", body)
	}

	// Budget delta: int64(price/0.9*100). For price=0.05 this is int64(5.555...) = 5.
	expectedDelta := int64(price / 0.90 * 100)
	afterBudget := f.GetBudgetRemaining(campID)
	actualDelta := beforeBudget - afterBudget
	if actualDelta != expectedDelta {
		t.Errorf("budget delta: want %d got %d (before=%d after=%d)",
			expectedDelta, actualDelta, beforeBudget, afterBudget)
	}

	// Kafka: dsp.bids should see 1 win event.
	//
	// Note: pre-V5, /win also emitted a duplicate 'impression' event, and
	// this test asserted dsp.impressions >= 1 here. V5 §P1 Step B removed
	// the spurious dsp.impressions write from /win — the real-impression
	// signal now comes from a separate path that this test doesn't
	// exercise (see docs/contracts/biz-engine.md). Reporting aggregation
	// is unaffected because V5 Step A switched the query to
	// countDistinctIf(request_id, event_type IN ('win','impression')).
	//
	// 60s accommodates first-message auto-create handshake (~15-20s per
	// fresh topic) plus the bidder's async Kafka writer batch flush.
	winCount := f.CountMessages("dsp.bids", reqID, 60*time.Second)
	if winCount != 1 {
		t.Errorf("dsp.bids win count: want 1, got %d", winCount)
	}

	// Regression sentinel for C1 (strategy goroutines racing r.Context()):
	// RecordWin should have incremented strategy:wins:{campID}:{today}. If the
	// fix ever regresses to r.Context(), the Redis INCR will silently drop under
	// race and this assertion will fail.
	//
	// The strategy package uses Asia/Shanghai time (see internal/bidder/strategy.go:144).
	winKey := fmt.Sprintf("strategy:wins:%d:%s", campID,
		time.Now().In(config.CSTLocation).Format("2006-01-02"))
	// Poll — the bare `go d.StrategySvc.RecordWin(...)` in cmd/bidder/main.go:505
	// is not wrapped in producer.inflight, so Go-runtime scheduling is the only
	// delay bound. On a local developer box a few ms is enough, but on a shared
	// CI runner (ubuntu-latest, ~2 cores under load from postgres+redis+kafka+
	// clickhouse containers + integration test itself) the goroutine can get
	// starved for hundreds of ms. 10 seconds is generous but still fails fast
	// if the regression actually triggers.
	var strategyWinCount int64
	for i := 0; i < 500; i++ {
		v, err := f.RDB.Get(f.Ctx, winKey).Int64()
		if err == nil && v >= 1 {
			strategyWinCount = v
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if strategyWinCount < 1 {
		t.Errorf("C1 regression sentinel: strategy:wins counter not incremented after /win (want >=1, got %d, key=%s)", strategyWinCount, winKey)
	}
}

// ------------------------------------------------------------
// Scenario 23 — /win with a mangled HMAC token is rejected 403,
// the budget is untouched, and no win event is emitted.
// ------------------------------------------------------------
func TestHandlers_WinHMACInvalid(t *testing.T) {
	f := newHandlerFixture(t)
	advID := f.SeedAdvertiser("win-hmac")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-win-hmac",
		BidCPMCents:      2000,
		BudgetDailyCents: 1_000_000,
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	beforeBudget := f.GetBudgetRemaining(campID)
	reqID := fmt.Sprintf("qa-winhmac-%d", time.Now().UnixNano())

	// Build a normally-valid URL, then corrupt the token param by appending "X".
	good := f.buildWinURL(campID, reqID, 0.05, "CN", "iOS")
	u, err := url.Parse(good)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	q := u.Query()
	q.Set("token", q.Get("token")+"X")
	u.RawQuery = q.Encode()
	badURL := u.String()

	resp, err := http.Get(badURL)
	if err != nil {
		t.Fatalf("GET /win: %v", err)
	}
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("/win status: want 403, got %d body=%s", resp.StatusCode, body)
	}

	afterBudget := f.GetBudgetRemaining(campID)
	if beforeBudget != afterBudget {
		t.Errorf("budget changed on HMAC failure: before=%d after=%d", beforeBudget, afterBudget)
	}

	winCount := f.CountMessages("dsp.bids", reqID, 5*time.Second)
	if winCount != 0 {
		t.Errorf("dsp.bids win count on HMAC failure: want 0, got %d", winCount)
	}
}

// ------------------------------------------------------------
// Scenario 24 — /win dedupes on request_id: three identical calls
// debit the budget exactly once and emit exactly one win event.
// ------------------------------------------------------------
func TestHandlers_WinDedup(t *testing.T) {
	f := newHandlerFixture(t)
	advID := f.SeedAdvertiser("win-dedup")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-win-dedup",
		BidCPMCents:      2000,
		BudgetDailyCents: 1_000_000,
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	beforeBudget := f.GetBudgetRemaining(campID)
	reqID := fmt.Sprintf("qa-windedup-%d", time.Now().UnixNano())
	price := 0.05
	winURL := f.buildWinURL(campID, reqID, price, "CN", "iOS")

	// First call: should succeed normally.
	resp, err := http.Get(winURL)
	if err != nil {
		t.Fatalf("GET /win #1: %v", err)
	}
	body1 := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/win #1 status: want 200, got %d body=%s", resp.StatusCode, body1)
	}
	if !strings.Contains(body1, `"ok"`) {
		t.Errorf("/win #1 body: want contains \"ok\", got %s", body1)
	}

	// Calls 2 and 3: dedup path, should return "duplicate" and NOT re-debit.
	for i := 2; i <= 3; i++ {
		resp, err = http.Get(winURL)
		if err != nil {
			t.Fatalf("GET /win #%d: %v", i, err)
		}
		body := readBody(t, resp)
		if !strings.Contains(body, "duplicate") {
			t.Errorf("/win #%d body: want contains \"duplicate\", got %s", i, body)
		}
	}

	// Budget: only ONE deduction, equal to int64(price/0.9*100).
	expectedDelta := int64(price / 0.90 * 100)
	afterBudget := f.GetBudgetRemaining(campID)
	actualDelta := beforeBudget - afterBudget
	if actualDelta != expectedDelta {
		t.Errorf("budget delta after 3 dedup calls: want %d got %d (before=%d after=%d)",
			expectedDelta, actualDelta, beforeBudget, afterBudget)
	}

	// Kafka: dsp.bids should see exactly 1 win event for this request_id.
	winCount := f.CountMessages("dsp.bids", reqID, 60*time.Second)
	if winCount != 1 {
		t.Errorf("dsp.bids win count under dedup: want 1, got %d", winCount)
	}
}

// ------------------------------------------------------------
// Scenario 25 — CB2 probe: a sub-cent clear price truncates to a
// zero-cent budget delta. Test PASSES as long as /win returns 200
// and the delta matches the int64-truncated math, regardless of
// whether the delta is 0.
// ------------------------------------------------------------
func TestHandlers_WinMoneyEdge(t *testing.T) {
	f := newHandlerFixture(t)
	advID := f.SeedAdvertiser("win-money")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-win-money",
		BidCPMCents:      1000,
		BudgetDailyCents: 1_000_000,
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	beforeBudget := f.GetBudgetRemaining(campID)
	reqID := fmt.Sprintf("qa-winmoney-%d", time.Now().UnixNano())
	price := 0.00123

	winURL := f.buildWinURL(campID, reqID, price, "CN", "iOS")
	resp, err := http.Get(winURL)
	if err != nil {
		t.Fatalf("GET /win: %v", err)
	}
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/win status: want 200, got %d body=%s", resp.StatusCode, body)
	}

	// Production math: int64(price/0.9*100). For price=0.00123 this is
	// int64(0.136666...) = 0 — a CB2 probe: money truncates at sub-cent.
	expectedDelta := int64(price / 0.90 * 100)
	afterBudget := f.GetBudgetRemaining(campID)
	actualDelta := beforeBudget - afterBudget
	if actualDelta != expectedDelta {
		t.Errorf("budget delta: want %d got %d (before=%d after=%d)",
			expectedDelta, actualDelta, beforeBudget, afterBudget)
	}
	t.Logf("CB2 probe: price=%.5f, expected_delta=%d, actual_delta=%d (delta=0 confirms sub-cent truncation)",
		price, expectedDelta, actualDelta)
}

// ------------------------------------------------------------
// Scenario 26 — /click on a CPC campaign deducts BidCPCCents from
// the daily budget and emits a click event with AdvertiserCharge ≈
// BidCPCCents / 100 dollars.
// ------------------------------------------------------------
func TestHandlers_ClickCPCBilling(t *testing.T) {
	f := newHandlerFixture(t)
	advID := f.SeedAdvertiser("click-cpc")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-click-cpc",
		BillingModel:     campaign.BillingCPC,
		BidCPCCents:      100,
		BudgetDailyCents: 1_000_000,
	})
	f.Start(t)

	beforeBudget := f.GetBudgetRemaining(campID)
	reqID := fmt.Sprintf("qa-clickcpc-%d", time.Now().UnixNano())

	clickURL := f.buildClickURL(campID, reqID)
	resp, err := http.Get(clickURL)
	if err != nil {
		t.Fatalf("GET /click: %v", err)
	}
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/click status: want 200, got %d body=%s", resp.StatusCode, body)
	}

	// Budget should drop by exactly BidCPCCents.
	afterBudget := f.GetBudgetRemaining(campID)
	delta := beforeBudget - afterBudget
	if delta != 100 {
		t.Errorf("click budget delta: want 100 got %d (before=%d after=%d)",
			delta, beforeBudget, afterBudget)
	}

	// Inspect the click event payload — AdvertiserCharge should be ≈ 1.0.
	// 60s accommodates first-message auto-create handshake (~15-20s) on a
	// freshly-created topic plus the async Kafka writer batch flush.
	evts := f.ReadMessagesFrom("dsp.impressions", reqID, 1, 60*time.Second)
	if len(evts) != 1 {
		t.Fatalf("dsp.impressions: want 1 event, got %d", len(evts))
	}
	if math.Abs(evts[0].AdvertiserCharge-1.0) > 0.001 {
		t.Errorf("click AdvertiserCharge: want ≈1.0, got %f", evts[0].AdvertiserCharge)
	}
}

// V5.1 P1-3 regression guard is in `cmd/bidder/main_test.go` as
// TestHandleClick_RejectsArbitraryDest_NoRedirect (unit-level). The
// integration-level version was attempted here but kept hanging on
// Kafka producer cleanup — handleClick fires a background SendClick
// via prod.Go(...) which the integration producer's Close() waits
// on through V5 inflight tracking, and the Kafka first-message
// handshake (~15-20s) can race test timeouts. The unit test uses a
// minimal Deps with Producer=nil and campaignID=0 so both the
// CPC-budget-deduction branch and the Kafka-send branch are skipped,
// leaving only the HMAC+dedup+redirect-deletion path under test —
// which is exactly the P1-3 scope.

// ------------------------------------------------------------
// Scenario 27 — /convert with a mangled HMAC token is rejected 403
// and emits no conversion event.
// ------------------------------------------------------------
func TestHandlers_ConvertHMACInvalid(t *testing.T) {
	f := newHandlerFixture(t)
	advID := f.SeedAdvertiser("convert-hmac")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-convert-hmac",
		BidCPMCents:      2000,
		BudgetDailyCents: 1_000_000,
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	reqID := fmt.Sprintf("qa-convhmac-%d", time.Now().UnixNano())

	good := f.buildConvertURL(campID, reqID)
	u, err := url.Parse(good)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	q := u.Query()
	q.Set("token", q.Get("token")+"X")
	u.RawQuery = q.Encode()
	badURL := u.String()

	resp, err := http.Get(badURL)
	if err != nil {
		t.Fatalf("GET /convert: %v", err)
	}
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("/convert status: want 403, got %d body=%s", resp.StatusCode, body)
	}

	// No conversion event should be emitted for this request_id.
	convCount := f.CountMessages("dsp.impressions", reqID, 5*time.Second)
	if convCount != 0 {
		t.Errorf("dsp.impressions count on HMAC failure: want 0, got %d", convCount)
	}
}

// ------------------------------------------------------------
// Positive-path /convert: a valid HMAC token yields 200, a
// "converted" body, and exactly 1 conversion event on
// dsp.impressions. Regression sentinel for the NB11-class bug —
// without a positive-path test, a ctx-race regression on
// handleConvert would only surface via the pre-existing negative
// HMAC test (which can't detect dropped Kafka writes).
// ------------------------------------------------------------
func TestHandlers_ConvertSucceeds(t *testing.T) {
	f := newHandlerFixture(t)
	advID := f.SeedAdvertiser("convert-ok")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-convert-ok",
		BidCPMCents:      2000,
		BudgetDailyCents: 1_000_000,
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	reqID := fmt.Sprintf("qa-convert-%d", time.Now().UnixNano())
	convertURL := f.buildConvertURL(campID, reqID)

	resp, err := f.srv.Client().Get(convertURL)
	if err != nil {
		t.Fatalf("GET /convert: %v", err)
	}
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/convert status: want 200, got %d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "converted") {
		t.Errorf("/convert body: want contains \"converted\", got %s", body)
	}

	// 60s accommodates first-message auto-create handshake on dsp.impressions.
	evts := f.ReadMessagesFrom("dsp.impressions", reqID, 1, 60*time.Second)
	if len(evts) != 1 {
		t.Fatalf("dsp.impressions: want 1 event, got %d", len(evts))
	}
	if evts[0].Type != "conversion" {
		t.Errorf("event type: want \"conversion\", got %q", evts[0].Type)
	}
}
