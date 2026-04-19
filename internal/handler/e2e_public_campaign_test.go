//go:build e2e
// +build e2e

package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"

	"github.com/heartgryphon/dsp/internal/billing"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/observability"
)

// TestCampaign_Create verifies POST /api/v1/campaigns happy path against
// HandleCreateCampaign. The handler enforces a typed request body:
//
//	{advertiser_id, name, billing_model, budget_total_cents,
//	 budget_daily_cents, bid_cpm_cents, bid_cpc_cents, ...}
//
// The response is {id, status} on 201. We intentionally skip the pub/sub
// assertion here — create publishes "updated" (see becdc67) but we only
// learn the id after POST, which races the subscribe. Update/Start/Pause
// tests below cover the pub/sub path.
func TestCampaign_Create(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)

	body := map[string]any{
		"advertiser_id":      advID,
		"name":               "c-" + safeName(t.Name()),
		"billing_model":      "cpm",
		"budget_total_cents": 100000,
		"budget_daily_cents": 10000,
		"bid_cpm_cents":      100,
		"targeting":          map[string]any{},
	}
	req := authedReq(t, http.MethodPost, "/api/v1/campaigns", body, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /campaigns: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	decodeJSON(t, w, &created)
	if created.ID == 0 {
		t.Fatalf("POST /campaigns: expected non-zero id, got 0 (body=%s)", w.Body.String())
	}
	if created.Status != "draft" {
		t.Fatalf("POST /campaigns: expected status=draft, got %q (body=%s)", created.Status, w.Body.String())
	}
}

// TestCampaign_UpdatePublishesUpdated verifies PUT /api/v1/campaigns/{id}
// returns 200 and publishes a campaign:updates {action:"updated"} message.
// HandleUpdateCampaign request body: {name, bid_cpm_cents, budget_daily_cents, targeting}.
func TestCampaign_UpdatePublishesUpdated(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)

	wait := subscribeUpdatesAction(t, d.Redis, campaignID, "updated")

	body := map[string]any{
		"name":               "renamed-" + safeName(t.Name()),
		"bid_cpm_cents":      200,
		"budget_daily_cents": 20000,
		"targeting":          map[string]any{},
	}
	req := authedReq(t, http.MethodPut,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignID, 10), body, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /campaigns/%d: expected 200, got %d: %s", campaignID, w.Code, w.Body.String())
	}
	if !wait(2 * time.Second) {
		t.Fatalf("did not receive campaign:updates action=updated for campaign_id=%d within 2s", campaignID)
	}
}

// TestCampaign_StartPublishesActivated verifies POST /api/v1/campaigns/{id}/start
// returns 200 and publishes action="activated". HandleStartCampaign requires
// an approved creative, budget_total >= budget_daily, and balance >=
// budget_daily — all satisfied by the shared harness (newCampaign seeds
// budget_total, newCreative approves on create, newAdvertiser seeds 1e6 cents).
func TestCampaign_StartPublishesActivated(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	_ = newCreative(t, d, campaignID)

	wait := subscribeUpdatesAction(t, d.Redis, campaignID, "activated")

	req := authedReq(t, http.MethodPost,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignID, 10)+"/start", nil, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /campaigns/%d/start: expected 200, got %d: %s",
			campaignID, w.Code, w.Body.String())
	}
	if !wait(2 * time.Second) {
		t.Fatalf("did not receive campaign:updates action=activated for campaign_id=%d within 2s", campaignID)
	}
}

// TestCampaign_PausePublishesPaused starts a campaign, then pauses it. We
// subscribe for the "paused" action AFTER the start call so we don't race
// the earlier "activated" message. subscribeUpdatesAction filters by action
// so even if a stale "activated" arrived on the channel we'd ignore it.
func TestCampaign_PausePublishesPaused(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	_ = newCreative(t, d, campaignID)

	startReq := authedReq(t, http.MethodPost,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignID, 10)+"/start", nil, apiKey)
	startW := execAuthed(t, d, startReq)
	if startW.Code != http.StatusOK {
		t.Fatalf("precondition start failed: %d: %s", startW.Code, startW.Body.String())
	}

	wait := subscribeUpdatesAction(t, d.Redis, campaignID, "paused")

	pauseReq := authedReq(t, http.MethodPost,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignID, 10)+"/pause", nil, apiKey)
	pauseW := execAuthed(t, d, pauseReq)
	if pauseW.Code != http.StatusOK {
		t.Fatalf("POST /campaigns/%d/pause: expected 200, got %d: %s",
			campaignID, pauseW.Code, pauseW.Body.String())
	}
	if !wait(2 * time.Second) {
		t.Fatalf("did not receive campaign:updates action=paused for campaign_id=%d within 2s", campaignID)
	}
}

// TestCampaign_Pause_NotActive_400 verifies that pausing a draft campaign
// is rejected. HandlePauseCampaign calls TransitionStatus(..., StatusPaused)
// which validates the transition and returns an error; the handler maps it
// to 409 Conflict. We accept 400 or 409 — both mean "you can't pause what
// isn't active". If the transition is ever widened to allow draft→paused
// the handler will 200 and this test will fail, which is the right signal.
func TestCampaign_Pause_NotActive_400(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)

	req := authedReq(t, http.MethodPost,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignID, 10)+"/pause", nil, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusConflict {
		t.Fatalf("POST /campaigns/%d/pause (draft): expected 400 or 409, got %d: %s",
			campaignID, w.Code, w.Body.String())
	}
}

// TestCampaign_Get_NotFound_404 verifies a GET for a non-existent id
// returns 404. The advertiser context is required (HandleGetCampaign calls
// GetCampaignForAdvertiser which scopes by advertiser_id), so we wrap in
// the real APIKey middleware via execAuthed.
func TestCampaign_Get_NotFound_404(t *testing.T) {
	d := mustDeps(t)
	_, apiKey := newAdvertiser(t, d)
	req := authedReq(t, http.MethodGet, "/api/v1/campaigns/99999999", nil, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /campaigns/99999999: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCampaign_List_IncludesMine verifies that GET /api/v1/campaigns returns
// a list containing the caller's newly-created campaign. HandleListCampaigns
// derives advertiser_id from the authenticated context (no query param) and
// returns 401 if unauthenticated, so this test must run through the APIKey
// middleware chain.
func TestCampaign_List_IncludesMine(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)

	req := authedReq(t, http.MethodGet, "/api/v1/campaigns", nil, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /campaigns: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var list []struct {
		ID           int64 `json:"id"`
		AdvertiserID int64 `json:"advertiser_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("decode list (body=%s): %v", w.Body.String(), err)
	}
	found := false
	for _, c := range list {
		if c.ID == campaignID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("GET /campaigns: expected list to include campaign_id=%d, got %s",
			campaignID, w.Body.String())
	}
}

// failingBillingStub satisfies handler.BillingService. GetBalance always
// returns a forced error. Used to test the F2 fail-closed path without
// tampering with the advertisers table (FK constraints forbid that).
type failingBillingStub struct{}

func (failingBillingStub) GetBalance(_ context.Context, _ int64) (int64, string, error) {
	return 0, "", errors.New("forced billing error for test")
}

func (failingBillingStub) TopUp(_ context.Context, _ int64, _ int64, _ string) (*billing.Transaction, error) {
	return nil, errors.New("not used in this test")
}

func (failingBillingStub) GetTransactions(_ context.Context, _ int64, _, _ int) ([]billing.Transaction, error) {
	return nil, errors.New("not used in this test")
}

// TestCampaign_StartReturns503WhenBalanceErrors covers the first F2
// fail-closed path: BillingSvc.GetBalance returns a non-nil error.
// Pre-fix: the handler's `if err == nil && balance < ...` swallowed
// the error and fell through to TransitionStatus(active) — a fail-open.
// Post-fix: 503 Service Unavailable.
func TestCampaign_StartReturns503WhenBalanceErrors(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	_ = newCreative(t, d, campaignID)

	// Swap in the failing stub for this test only.
	originalSvc := d.BillingSvc
	d.BillingSvc = failingBillingStub{}
	t.Cleanup(func() { d.BillingSvc = originalSvc })

	req := authedReq(t, http.MethodPost,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignID, 10)+"/start", nil, apiKey)
	w := execAuthed(t, d, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on GetBalance error, got %d: %s",
			w.Code, w.Body.String())
	}
}

// TestCampaign_StartReturns503WhenBillingSvcNil covers the second F2
// fail-closed path (CEO Finding #2): non-sandbox campaign + BillingSvc
// is nil. Pre-fix: the handler's `if d.BillingSvc != nil` guard silently
// bypassed balance checking — a second fail-open surface. Post-fix: 503.
func TestCampaign_StartReturns503WhenBillingSvcNil(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	_ = newCreative(t, d, campaignID)

	originalSvc := d.BillingSvc
	d.BillingSvc = nil
	t.Cleanup(func() { d.BillingSvc = originalSvc })

	req := authedReq(t, http.MethodPost,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignID, 10)+"/start", nil, apiKey)
	w := execAuthed(t, d, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when BillingSvc is nil (non-sandbox), got %d: %s",
			w.Code, w.Body.String())
	}
}

// TestCampaign_StartRecordsMetricOnPubSubFailure verifies that when Redis
// pub/sub fails during activation, the metric
// campaign_activation_pubsub_failures_total{action="activated"} increments
// while /start still returns 200 (eventual-consistency contract per F3-B).
//
// We inject the failure by swapping d.Redis (pub/sub surface) for a client
// pointing at a dead address while leaving BudgetSvc's Redis intact. This
// is necessary because after the Phase 3 reorder, InitDailyBudget runs
// BEFORE NotifyCampaignUpdate — using a single closed Redis would trip
// InitDailyBudget first and return 503, not exercise the pub/sub failure
// path we want to observe.
func TestCampaign_StartRecordsMetricOnPubSubFailure(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	_ = newCreative(t, d, campaignID)

	// Capture pre-call metric value for the "activated" label.
	counter := observability.CampaignActivationPubSubFailures.WithLabelValues("activated")
	beforeVal := testutil.ToFloat64(counter)

	// Swap d.Redis for a client pointing at a dead address. BudgetSvc still
	// holds the original live Redis client so InitDailyBudget succeeds. Only
	// NotifyCampaignUpdate's Publish call hits the dead client.
	deadRedis := redis.NewClient(&redis.Options{
		Addr:     "localhost:1", // nothing listens here
		Password: "irrelevant",
	})
	defer deadRedis.Close()
	originalRedis := d.Redis
	d.Redis = deadRedis
	t.Cleanup(func() { d.Redis = originalRedis })

	req := authedReq(t, http.MethodPost,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignID, 10)+"/start", nil, apiKey)
	w := execAuthed(t, d, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (eventual-consistency contract), got %d: %s",
			w.Code, w.Body.String())
	}

	afterVal := testutil.ToFloat64(counter)
	if afterVal != beforeVal+1 {
		t.Fatalf("expected pubsub_failures{action=activated} to increment from %v to %v, got %v",
			beforeVal, beforeVal+1, afterVal)
	}
}

// TestCampaign_StartReturns503WhenInitDailyBudgetFails verifies the
// InitDailyBudget fail-closed path (Codex Finding #3). Pre-fix:
// InitDailyBudget error was log-and-continue, campaign committed active
// with no daily key → bidder saw 0 budget → no-bids forever. Post-fix:
// InitDailyBudget err returns 503 BEFORE TransitionStatus, no active row.
//
// We inject the failure by swapping BudgetSvc with one backed by a dead
// Redis client. BillingSvc uses Postgres not Redis, so balance check still
// works. Handler Redis (for pub/sub) is left live — though it never runs
// because the 503 short-circuits before NotifyCampaignUpdate.
func TestCampaign_StartReturns503WhenInitDailyBudgetFails(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	_ = newCreative(t, d, campaignID)

	// Swap BudgetSvc for one backed by a dead Redis client.
	deadRedis := redis.NewClient(&redis.Options{
		Addr:     "localhost:1", // nothing listens here
		Password: "irrelevant",
	})
	defer deadRedis.Close()
	originalBudget := d.BudgetSvc
	d.BudgetSvc = budget.New(deadRedis)
	t.Cleanup(func() { d.BudgetSvc = originalBudget })

	req := authedReq(t, http.MethodPost,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignID, 10)+"/start", nil, apiKey)
	w := execAuthed(t, d, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on InitDailyBudget failure, got %d: %s",
			w.Code, w.Body.String())
	}

	// Verify the campaign was NOT transitioned to active — no orphan
	// "active but 0 daily budget" row should exist.
	c, err := d.Store.GetCampaign(context.Background(), campaignID)
	if err != nil {
		t.Fatalf("get campaign: %v", err)
	}
	if c.Status == campaign.StatusActive {
		t.Fatalf("CONTRACT VIOLATION: campaign committed active despite 503 response")
	}
}
