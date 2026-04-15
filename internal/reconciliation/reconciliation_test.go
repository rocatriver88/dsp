package reconciliation

// NOTE: reconciliation_integration_test.go pins time.Local = time.UTC in its
// init() to work around a known TZ bug in RunHourly (NB9). That init() runs
// for every test binary built from this package, including this one. Unit
// tests in this file MUST NOT depend on the host's local timezone — if you
// need local-time assertions, use an explicit time.LoadLocation call.

import (
	"testing"
	"time"
)

func TestReconcileResult_NoDifference(t *testing.T) {
	r := ReconcileResult{
		CampaignID:      1,
		Date:            time.Now(),
		RedisSpentCents: 10000,
		CHSpentCents:    10000,
	}
	if r.DiffPercent() != 0 {
		t.Errorf("expected 0%% diff, got %.2f%%", r.DiffPercent())
	}
	if r.NeedsAlert(1.0) {
		t.Error("should not alert when no difference")
	}
}

func TestReconcileResult_SmallDifference(t *testing.T) {
	r := ReconcileResult{
		CampaignID:      1,
		Date:            time.Now(),
		RedisSpentCents: 10000,
		CHSpentCents:    10050,
	}
	diff := r.DiffPercent()
	if diff < 0.4 || diff > 0.6 {
		t.Errorf("expected ~0.5%% diff, got %.2f%%", diff)
	}
	if r.NeedsAlert(1.0) {
		t.Error("0.5%% diff should not trigger 1%% alert threshold")
	}
}

func TestReconcileResult_LargeDifference(t *testing.T) {
	r := ReconcileResult{
		CampaignID:      1,
		Date:            time.Now(),
		RedisSpentCents: 10000,
		CHSpentCents:    10200,
	}
	diff := r.DiffPercent()
	if diff < 1.9 || diff > 2.1 {
		t.Errorf("expected ~2%% diff, got %.2f%%", diff)
	}
	if !r.NeedsAlert(1.0) {
		t.Error("2%% diff should trigger 1%% alert threshold")
	}
}

func TestReconcileResult_ZeroSpend(t *testing.T) {
	r := ReconcileResult{
		CampaignID:      1,
		Date:            time.Now(),
		RedisSpentCents: 0,
		CHSpentCents:    0,
	}
	if r.DiffPercent() != 0 {
		t.Errorf("expected 0%% diff for zero spend, got %.2f%%", r.DiffPercent())
	}
	if r.NeedsAlert(1.0) {
		t.Error("should not alert on zero spend")
	}
}
