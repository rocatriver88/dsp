//go:build integration

package reconciliation_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/alert"
	"github.com/heartgryphon/dsp/internal/billing"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/qaharness"
	"github.com/heartgryphon/dsp/internal/reconciliation"
	"github.com/heartgryphon/dsp/internal/reporting"
)

// The ClickHouse server in the compose stack runs in UTC, so bid_log.event_date
// is derived from the Go-side time.Time in UTC. reconciliation.GetCampaignStats
// compares the Date column against a DateTime built from time.Local's dayStart —
// if time.Local is CST, dayStart in UTC lands mid-day on the previous day and
// the midnight-UTC event_date fails the `event_date >= from` guard even though
// the row was inserted "today local". Pinning time.Local to UTC for this test
// package keeps Go's dayStart aligned with CH's event_date boundary, so these
// tests verify the reconciliation logic itself rather than the TZ skew.
func init() {
	time.Local = time.UTC
}

// fakeAlerter implements alert.Sender, capturing messages instead of sending.
type fakeAlerter struct {
	sent []string
}

func (f *fakeAlerter) Send(title, content string) error {
	f.sent = append(f.sent, title+" | "+content)
	return nil
}

// Compile-time assertion that fakeAlerter satisfies alert.Sender.
var _ alert.Sender = (*fakeAlerter)(nil)

// setReconcileBudgetKey writes the Redis daily-budget key using the *server-
// local* date, which is what reconciliation.RunHourly reads (reconciliation.go
// uses time.Now().Format, not config.CSTLocation). This avoids a TZ mismatch
// with qaharness.SetBudgetRemaining which writes the CST-formatted key.
func setReconcileBudgetKey(t *testing.T, h *qaharness.TestHarness, campID, remaining int64) {
	t.Helper()
	key := fmt.Sprintf("budget:daily:%d:%s", campID, time.Now().Format("2006-01-02"))
	if err := h.RDB.Set(h.Ctx, key, remaining, 25*time.Hour).Err(); err != nil {
		t.Fatalf("set budget key %s: %v", key, err)
	}
}

// newReconcileSvc wires up a reconciliation.Service with test collaborators.
func newReconcileSvc(t *testing.T, h *qaharness.TestHarness, alerter alert.Sender) *reconciliation.Service {
	t.Helper()
	store := campaign.NewStore(h.PG)
	rs, err := reporting.NewStore(h.Env.ClickHouseAddr, h.Env.ClickHouseUser, h.Env.ClickHousePass)
	if err != nil {
		t.Fatalf("reporting store: %v", err)
	}
	t.Cleanup(func() { _ = rs.Close() })

	billSvc := billing.New(h.PG)
	return reconciliation.New(h.RDB, store, rs, billSvc, alerter)
}

// findResult returns the ReconcileResult for campID, or fatals if missing.
func findResult(t *testing.T, results []reconciliation.ReconcileResult, campID int64) reconciliation.ReconcileResult {
	t.Helper()
	for _, r := range results {
		if r.CampaignID == campID {
			return r
		}
	}
	t.Fatalf("campaign %d missing from RunHourly results (got %d results)", campID, len(results))
	return reconciliation.ReconcileResult{}
}

// Scenario 34 — Redis and CH agree; no alert fired.
func TestReconcile_Consistent(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("recon-ok")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-recon-ok",
		BudgetDailyCents: 10_000,
	})
	// Redis: budget 10000 - 3000 = 7000 remaining (3000 spent per Redis view).
	setReconcileBudgetKey(t, h, campID, 7_000)

	// CH: single win event with charge=3000, timestamped "now" so it falls
	// inside RunHourly's [dayStart, now] query window.
	now := time.Now()
	h.InsertBidLogRow(campID, advID, 1, "win", "qa-recon-ok-1", "dev-A", 0, 0, 3_000, now)

	alerter := &fakeAlerter{}
	svc := newReconcileSvc(t, h, alerter)
	results, err := svc.RunHourly(h.Ctx, 1.0)
	if err != nil {
		t.Fatal(err)
	}

	r := findResult(t, results, campID)
	if r.RedisSpentCents != 3_000 {
		t.Errorf("RedisSpentCents: want 3000 got %d", r.RedisSpentCents)
	}
	if r.CHSpentCents != 3_000 {
		t.Errorf("CHSpentCents: want 3000 got %d", r.CHSpentCents)
	}
	if d := r.DiffPercent(); d != 0 {
		t.Errorf("DiffPercent: want 0 got %v", d)
	}
	if len(alerter.sent) != 0 {
		t.Errorf("expected no alert, got %d: %+v", len(alerter.sent), alerter.sent)
	}
}

// Scenario 35 — CH shows more spend than Redis (~14% drift), alert fired.
func TestReconcile_DriftAlerts(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("recon-drift")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-recon-drift",
		BudgetDailyCents: 10_000,
	})
	// Redis says 3000 spent.
	setReconcileBudgetKey(t, h, campID, 7_000)
	// CH says 3500 spent → drift = (3500-3000)/3500 ≈ 14.3%.
	now := time.Now()
	h.InsertBidLogRow(campID, advID, 1, "win", "qa-recon-drift-1", "dev-B", 0, 0, 3_500, now)

	alerter := &fakeAlerter{}
	svc := newReconcileSvc(t, h, alerter)
	results, err := svc.RunHourly(h.Ctx, 5.0) // 5% threshold
	if err != nil {
		t.Fatal(err)
	}

	r := findResult(t, results, campID)
	if r.RedisSpentCents != 3_000 {
		t.Errorf("RedisSpentCents: want 3000 got %d", r.RedisSpentCents)
	}
	if r.CHSpentCents != 3_500 {
		t.Errorf("CHSpentCents: want 3500 got %d", r.CHSpentCents)
	}
	if !r.NeedsAlert(5.0) {
		t.Errorf("expected NeedsAlert(5.0) true, diff=%v%%", r.DiffPercent())
	}

	if len(alerter.sent) != 1 {
		t.Fatalf("expected 1 alert, got %d: %+v", len(alerter.sent), alerter.sent)
	}
	if !strings.Contains(alerter.sent[0], "Campaign") {
		t.Errorf("alert should mention 'Campaign': %q", alerter.sent[0])
	}
	if !strings.Contains(alerter.sent[0], "Reconciliation Drift Detected") {
		t.Errorf("alert title should be 'Reconciliation Drift Detected': %q", alerter.sent[0])
	}
}

// Scenario 36 — CB5 probe. Documents what reporting.Store.GetCampaignStats
// reports for SpendCents when bid_log has mixed event_types with non-zero
// charge_cents. The reconciliation service uses this value as chSpent, while
// production's bidder only deducts the Redis budget on /win or /click.
//
// If SpendCents == 350 (win+click+all-others summed), CB5 is CONFIRMED:
//
//	the SQL aggregates all event_types and over-reports CH spend vs Redis.
//
// If SpendCents == 300, the query is win-only and CB5 is disproved.
func TestReconcile_SQLAggregationSemantics(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("recon-cb5")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-recon-cb5",
		BudgetDailyCents: 10_000,
	})

	now := time.Now()
	// Mixed row set:
	//   bid        charge=0
	//   win        charge=300  (CPM impression the advertiser was billed for)
	//   click      charge=50   (CPC billing event)
	//   conversion charge=0
	h.InsertBidLogRow(campID, advID, 1, "bid", "qa-cb5-a", "dev-1", 0, 0, 0, now)
	h.InsertBidLogRow(campID, advID, 1, "win", "qa-cb5-b", "dev-1", 0, 300, 300, now)
	h.InsertBidLogRow(campID, advID, 1, "click", "qa-cb5-c", "dev-1", 0, 0, 50, now)
	h.InsertBidLogRow(campID, advID, 1, "conversion", "qa-cb5-d", "dev-1", 0, 0, 0, now)

	// Query reporting.Store directly — we want to see the raw SpendCents value.
	rs, err := reporting.NewStore(h.Env.ClickHouseAddr, h.Env.ClickHouseUser, h.Env.ClickHousePass)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()

	// GetCampaignStats compares `event_date (Date) >= from (DateTime)`, so
	// `from` must land on or before the midnight that CH coerces event_date
	// to. Use today 00:00 UTC as `from` and tomorrow 00:00 UTC as `to`.
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	stats, err := rs.GetCampaignStats(h.Ctx, uint64(campID), dayStart, dayStart.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("CB5 probe: GetCampaignStats.SpendCents = %d", stats.SpendCents)
	t.Logf("CB5 probe: breakdown — bid=0, win=300, click=50, conversion=0")
	t.Logf("CB5 probe: if SpendCents == 350 (sum of all event_types), CB5 is CONFIRMED")
	t.Logf("CB5 probe: if SpendCents == 300 (win-only),              CB5 is disproved")

	// REGRESSION SENTINEL for CB5. The current buggy behavior is SpendCents=350
	// (sum of win.charge=300 + click.charge=50, unfiltered by event_type). When
	// biz fixes CB5 by filtering the SQL to `sumIf(charge_cents, event_type IN
	// ('win','click'))`, this assertion will fail and force a test update — that
	// is the point: a deliberate fix MUST go through this file.
	if stats.SpendCents != 350 {
		t.Errorf("CB5 regression sentinel: SpendCents = %d, expected 350 (bug still present). If biz fixed CB5, update this assertion and the report.", stats.SpendCents)
	}
}

// Scenario 37 — RunHourly handles an unhealthy ClickHouse gracefully.
// Must NOT panic. A closed reporting.Store causes GetCampaignStats to error;
// reconciliation.go currently `continue`s on that error, so we expect an empty
// results slice with no alert fired.
func TestReconcile_CHFailureDoesNotPanic(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("recon-ch-fail")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-recon-ch-fail",
		BudgetDailyCents: 10_000,
	})
	setReconcileBudgetKey(t, h, campID, 7_000)

	alerter := &fakeAlerter{}
	store := campaign.NewStore(h.PG)
	rs, err := reporting.NewStore(h.Env.ClickHouseAddr, h.Env.ClickHouseUser, h.Env.ClickHousePass)
	if err != nil {
		t.Fatal(err)
	}
	// Force subsequent CH queries through this store to fail.
	_ = rs.Close()
	billSvc := billing.New(h.PG)
	svc := reconciliation.New(h.RDB, store, rs, billSvc, alerter)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RunHourly panicked: %v", r)
		}
	}()

	// Give the call a bounded context just in case a closed CH client hangs.
	ctx, cancel := context.WithTimeout(h.Ctx, 30*time.Second)
	defer cancel()

	results, err := svc.RunHourly(ctx, 1.0)
	t.Logf("CHFailure scenario: RunHourly returned err=%v results=%d alerts=%d",
		err, len(results), len(alerter.sent))
	// No hard assertion on err / results shape — contract is "doesn't panic".
}
