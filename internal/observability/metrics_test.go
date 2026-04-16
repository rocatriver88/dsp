package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)


func TestMetrics_AllDeclarationsRegistered(t *testing.T) {
	// allCollectors maps expected metric names to their Collector.
	// promauto registers each collector in init(), so a duplicate
	// Register call should return AlreadyRegisteredError — proving
	// the metric is in the default registry.
	allCollectors := map[string]prometheus.Collector{
		"dsp_bid_requests_total":          BidRequestsTotal,
		"dsp_bid_latency_seconds":         BidLatency,
		"dsp_wins_total":                  WinsTotal,
		"dsp_clicks_total":                ClicksTotal,
		"dsp_budget_deducted_cents_total":  BudgetDeductedCentsTotal,
		"dsp_guardrail_trips_total":       GuardrailTripsTotal,
		"dsp_auction_outcome":             AuctionOutcome,
		"dsp_campaign_active_total":       CampaignActiveTotal,
		"dsp_producer_inflight":           ProducerInflight,
		"dsp_redis_errors_total":          RedisErrorsTotal,
		"dsp_api_requests_total":          APIRequestsTotal,
		"dsp_rate_limit_hits_total":       RateLimitHits,
		"dsp_auto_pause_total":            AutoPauseTotal,
		"dsp_kafka_consume_errors_total":  KafkaConsumeErrors,
		"dsp_dlq_publish_total":           DLQPublishTotal,
	}

	for name, collector := range allCollectors {
		err := prometheus.DefaultRegisterer.Register(collector)
		if err == nil {
			// It wasn't registered — that's a bug. Unregister to clean up.
			prometheus.DefaultRegisterer.Unregister(collector)
			t.Errorf("metric %q was NOT already registered with the default registry", name)
		} else {
			// AlreadyRegisteredError is the expected outcome.
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				t.Errorf("metric %q: unexpected registration error: %v", name, err)
			}
		}
	}
}

func TestMetrics_CounterVecIncrements(t *testing.T) {
	tests := []struct {
		name   string
		inc    func()
		metric string
	}{
		{
			name:   "BidRequestsTotal",
			inc:    func() { BidRequestsTotal.WithLabelValues("adx", "won").Inc() },
			metric: "dsp_bid_requests_total",
		},
		{
			name:   "WinsTotal",
			inc:    func() { WinsTotal.WithLabelValues("cpm").Inc() },
			metric: "dsp_wins_total",
		},
		{
			name:   "ClicksTotal",
			inc:    func() { ClicksTotal.WithLabelValues("cpc").Inc() },
			metric: "dsp_clicks_total",
		},
		{
			name:   "BudgetDeductedCentsTotal",
			inc:    func() { BudgetDeductedCentsTotal.WithLabelValues("ocpm").Inc() },
			metric: "dsp_budget_deducted_cents_total",
		},
		{
			name:   "GuardrailTripsTotal",
			inc:    func() { GuardrailTripsTotal.WithLabelValues("daily_budget").Inc() },
			metric: "dsp_guardrail_trips_total",
		},
		{
			name:   "AuctionOutcome",
			inc:    func() { AuctionOutcome.WithLabelValues("ok").Inc() },
			metric: "dsp_auction_outcome",
		},
		{
			name:   "RedisErrorsTotal",
			inc:    func() { RedisErrorsTotal.WithLabelValues("get").Inc() },
			metric: "dsp_redis_errors_total",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.inc()
			// testutil.CollectAndCount verifies the metric is present and
			// returns the number of time-series. At least 1 means it was
			// incremented successfully.
			count := testutil.CollectAndCount(
				findCollector(t, tc.metric),
			)
			if count < 1 {
				t.Errorf("expected at least 1 time series for %s, got %d", tc.metric, count)
			}
		})
	}
}

func TestMetrics_HistogramObservation(t *testing.T) {
	BidLatency.WithLabelValues("adx").Observe(0.015)
	count := testutil.CollectAndCount(BidLatency)
	if count < 1 {
		t.Error("expected at least 1 time series for dsp_bid_latency_seconds")
	}
}

func TestMetrics_GaugeSetAndRead(t *testing.T) {
	CampaignActiveTotal.Set(42)
	val := testutil.ToFloat64(CampaignActiveTotal)
	if val != 42 {
		t.Errorf("CampaignActiveTotal: expected 42, got %f", val)
	}

	ProducerInflight.WithLabelValues("wins").Set(7)
	val = testutil.ToFloat64(ProducerInflight.WithLabelValues("wins"))
	if val != 7 {
		t.Errorf("ProducerInflight[wins]: expected 7, got %f", val)
	}
}

func TestMetrics_LabelCardinalityBounded(t *testing.T) {
	// The plan specifies bounded label sets. We verify the declared label
	// names match the expected cardinality by checking the Desc. We cannot
	// prevent arbitrary label values at the Prometheus client level (it
	// creates a new series for any value), but we verify the label *names*
	// are exactly what the plan specifies.

	assertLabels := func(t *testing.T, name string, cv *prometheus.CounterVec, expected []string) {
		t.Helper()
		// Create a metric with dummy label values and inspect its Desc.
		vals := make([]string, len(expected))
		for i := range vals {
			vals[i] = "test"
		}
		m, err := cv.GetMetricWithLabelValues(vals...)
		if err != nil {
			t.Fatalf("%s: failed to get metric with %d labels: %v", name, len(expected), err)
		}
		desc := m.Desc().String()
		// Desc().String() includes label names. Verify each expected label
		// appears in the description.
		for _, lbl := range expected {
			if !containsSubstring(desc, lbl) {
				t.Errorf("%s: expected label %q in Desc %s", name, lbl, desc)
			}
		}
	}

	assertLabels(t, "BidRequestsTotal", BidRequestsTotal, []string{"exchange", "result"})
	assertLabels(t, "WinsTotal", WinsTotal, []string{"billing_model"})
	assertLabels(t, "ClicksTotal", ClicksTotal, []string{"billing_model"})
	assertLabels(t, "BudgetDeductedCentsTotal", BudgetDeductedCentsTotal, []string{"billing_model"})
	assertLabels(t, "GuardrailTripsTotal", GuardrailTripsTotal, []string{"reason"})
	assertLabels(t, "AuctionOutcome", AuctionOutcome, []string{"outcome"})
	assertLabels(t, "RedisErrorsTotal", RedisErrorsTotal, []string{"op"})
}

// ── helpers ──────────────────────────────────────────────────────────

// findCollector looks up a prometheus.Collector from the package-level
// variables by metric name. This avoids coupling to the default registry
// for individual metric assertions.
func findCollector(t *testing.T, name string) prometheus.Collector {
	t.Helper()
	switch name {
	case "dsp_bid_requests_total":
		return BidRequestsTotal
	case "dsp_wins_total":
		return WinsTotal
	case "dsp_clicks_total":
		return ClicksTotal
	case "dsp_budget_deducted_cents_total":
		return BudgetDeductedCentsTotal
	case "dsp_guardrail_trips_total":
		return GuardrailTripsTotal
	case "dsp_auction_outcome":
		return AuctionOutcome
	case "dsp_redis_errors_total":
		return RedisErrorsTotal
	default:
		t.Fatalf("unknown metric name: %s", name)
		return nil
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && searchSubstring(s, sub)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
