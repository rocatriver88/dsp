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

// CheckBudgetExhausted returns a pause reason if total spend exceeds total budget.
// Returns empty string if within budget.
func CheckBudgetExhausted(budgetTotalCents int64, totalSpendCents uint64) string {
	if budgetTotalCents > 0 && totalSpendCents >= uint64(budgetTotalCents) {
		return "budget_exhausted: total spend reached campaign budget limit"
	}
	return ""
}

// CheckDailyBudgetExhausted returns a pause reason if daily spend exceeds daily budget.
func CheckDailyBudgetExhausted(budgetDailyCents int64, dailySpendCents uint64) string {
	if budgetDailyCents > 0 && dailySpendCents >= uint64(budgetDailyCents) {
		return "daily_budget_exhausted: daily spend reached daily budget limit"
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
