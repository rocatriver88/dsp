package autopause

import "testing"

func TestCheckSpendSpike_Normal(t *testing.T) {
	// Budget ¥24000/day = ¥1000/hour. Spent ¥1500 this hour = 1.5x. No spike.
	if r := CheckSpendSpike(2400000, 150000); r != "" {
		t.Errorf("expected no spike at 1.5x, got: %s", r)
	}
}

func TestCheckSpendSpike_Triggered(t *testing.T) {
	// Budget ¥24000/day = ¥1000/hour. Spent ¥2500 this hour = 2.5x. Spike!
	if r := CheckSpendSpike(2400000, 250000); r == "" {
		t.Error("expected spike at 2.5x, got empty")
	}
}

func TestCheckSpendSpike_ExactThreshold(t *testing.T) {
	// Budget ¥24000/day = ¥1000/hour. Spent ¥2000 = exactly 2x. Not triggered (> not >=).
	if r := CheckSpendSpike(2400000, 200000); r != "" {
		t.Errorf("expected no spike at exactly 2x, got: %s", r)
	}
}

func TestCheckSpendSpike_ZeroBudget(t *testing.T) {
	// Zero budget should not trigger
	if r := CheckSpendSpike(0, 100000); r != "" {
		t.Errorf("expected no spike with zero budget, got: %s", r)
	}
}

func TestCheckCTRAnomaly_CPM_Normal(t *testing.T) {
	// 2% CTR on CPM = normal
	if r := CheckCTRAnomaly("cpm", 10000, 200); r != "" {
		t.Errorf("expected no anomaly at 2%% CTR, got: %s", r)
	}
}

func TestCheckCTRAnomaly_CPM_Triggered(t *testing.T) {
	// 6% CTR on CPM over 1000+ impressions = anomaly
	if r := CheckCTRAnomaly("cpm", 2000, 120); r == "" {
		t.Error("expected anomaly at 6% CTR")
	}
}

func TestCheckCTRAnomaly_CPM_BelowThreshold(t *testing.T) {
	// High CTR but < 1000 impressions = not enough data
	if r := CheckCTRAnomaly("cpm", 500, 50); r != "" {
		t.Errorf("expected no anomaly with <1000 impressions, got: %s", r)
	}
}

func TestCheckCTRAnomaly_CPC_Ignored(t *testing.T) {
	// CPC campaigns should never trigger CTR anomaly (structurally different CTR)
	if r := CheckCTRAnomaly("cpc", 10000, 1000); r != "" {
		t.Errorf("CPC should not trigger CTR anomaly, got: %s", r)
	}
}

func TestCheckCTRAnomaly_OCPM_Ignored(t *testing.T) {
	if r := CheckCTRAnomaly("ocpm", 10000, 1000); r != "" {
		t.Errorf("oCPM should not trigger CTR anomaly, got: %s", r)
	}
}
