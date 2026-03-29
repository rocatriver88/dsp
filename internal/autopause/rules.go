package autopause

// CheckSpendSpike returns a pause reason if hourly spend exceeds 2x expected hourly budget.
// Returns empty string if no anomaly.
func CheckSpendSpike(budgetDailyCents int64, hourlySpendCents uint64) string {
	expectedHourly := float64(budgetDailyCents) / 24.0
	if expectedHourly > 0 && float64(hourlySpendCents) > expectedHourly*2 {
		return "spend_spike: hourly spend exceeded 2x expected rate"
	}
	return ""
}

// CheckCTRAnomaly returns a pause reason if CTR exceeds 5% over 1000+ impressions.
// Only applies to CPM campaigns. CPC campaigns have structurally different CTR.
// Returns empty string if no anomaly.
func CheckCTRAnomaly(billingModel string, impressions, clicks uint64) string {
	if billingModel != "cpm" {
		return ""
	}
	if impressions < 1000 {
		return ""
	}
	ctr := float64(clicks) / float64(impressions)
	if ctr > 0.05 {
		return "ctr_anomaly: CTR exceeded 5% threshold"
	}
	return ""
}
