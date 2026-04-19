package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ──────────────────────────────────────────────────────────────────────
// Business metrics — V5.2B observability plan
//
// Naming convention: <subsystem>_<metric>_<unit>  (Prometheus best practice)
// Every label set is bounded to prevent cardinality explosion.
// ──────────────────────────────────────────────────────────────────────

var (
	// ── Bidder metrics ────────────────────────────────────────────────

	// BidRequestsTotal counts every bid request processed by the bidder.
	// Labels:
	//   exchange — bounded by ExchangeRegistry entries
	//   result   — {won, lost, passed, rejected}
	BidRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_bid_requests_total",
		Help: "Total bid requests processed by the bidder.",
	}, []string{"exchange", "result"})

	// BidLatency measures per-exchange bid-request processing latency.
	BidLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dsp_bid_latency_seconds",
		Help:    "Bid request processing latency in seconds.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25},
	}, []string{"exchange"})

	// WinsTotal counts win notices by billing model.
	// Labels: billing_model — {cpm, cpc, ocpm}
	WinsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_wins_total",
		Help: "Total win notices by billing model.",
	}, []string{"billing_model"})

	// ClicksTotal counts attributed clicks by billing model.
	// Labels: billing_model — {cpm, cpc, ocpm}
	ClicksTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_clicks_total",
		Help: "Total attributed clicks by billing model.",
	}, []string{"billing_model"})

	// BudgetDeductedCentsTotal tracks real spend (in cents) by billing model.
	// Labels: billing_model — {cpm, cpc, ocpm}
	BudgetDeductedCentsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_budget_deducted_cents_total",
		Help: "Total budget deducted in cents by billing model.",
	}, []string{"billing_model"})

	// ── Guardrail / auction outcome metrics ──────────────────────────

	// GuardrailTripsTotal counts circuit-breaker activations.
	// Labels: reason — {daily_budget, max_cpm, manual, other}
	GuardrailTripsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_guardrail_trips_total",
		Help: "Total guardrail (circuit-breaker) activations.",
	}, []string{"reason"})

	// AuctionOutcome records why auctions ended as they did.
	// Labels: outcome — {no_campaigns, under_bid, fraud_rejected, ok}
	AuctionOutcome = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_auction_outcome",
		Help: "Auction outcome distribution.",
	}, []string{"outcome"})

	// ── Campaign metrics ─────────────────────────────────────────────

	// CampaignActiveTotal is the current number of active campaigns.
	CampaignActiveTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dsp_campaign_active_total",
		Help: "Number of currently active campaigns.",
	})

	// ── Infrastructure metrics ───────────────────────────────────────

	// ProducerInflight tracks Kafka producer in-flight depth per topic.
	// Labels: topic
	ProducerInflight = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dsp_producer_inflight",
		Help: "Kafka producer in-flight message count per topic.",
	}, []string{"topic"})

	// RedisErrorsTotal counts Redis operation errors by operation type.
	// Labels: op — {get, set, incr, setnx, pubsub}
	RedisErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_redis_errors_total",
		Help: "Total Redis errors by operation type.",
	}, []string{"op"})

	// ── Existing operational metrics (kept from V5) ──────────────────

	// APIRequestsTotal counts all API requests.
	APIRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_api_requests_total",
		Help: "Total API requests.",
	}, []string{"method", "path", "status"})

	// RateLimitHits counts rate-limiter rejections.
	RateLimitHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dsp_rate_limit_hits_total",
		Help: "Total rate limit rejections.",
	})

	// AutoPauseTotal counts auto-pause actions by reason.
	AutoPauseTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_auto_pause_total",
		Help: "Total auto-pause actions.",
	}, []string{"reason"})

	// KafkaConsumeErrors counts Kafka consume errors by topic.
	KafkaConsumeErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_kafka_consume_errors_total",
		Help: "Total Kafka consume errors.",
	}, []string{"topic"})

	// DLQPublishTotal counts events published to the dead-letter queue.
	DLQPublishTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dsp_dlq_publish_total",
		Help: "Total events published to dead-letter queue.",
	})

	// ── Phase 2 (HMAC rollover / clearing-price) metrics ─────────────

	// BidderTokenLegacyAccepted counts win/click/convert token
	// validations that fell back to the legacy 4-param HMAC signature
	// during a deploy transition. Expected to spike briefly right after
	// a Phase 2 deploy, then return to zero within the 5-minute token
	// TTL. Sustained non-zero reading >15 min after deploy indicates
	// stuck legacy-token traffic — investigate before removing the
	// fallback branch.
	// Labels: handler — {win, click, convert}
	BidderTokenLegacyAccepted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bidder_token_legacy_accepted_total",
		Help: "Count of HMAC token validations accepted via legacy 4-param signature during deploy transition.",
	}, []string{"handler"})

	// BidderClearingPriceCapped counts /win requests whose unsigned URL
	// `price` exceeded the HMAC-signed bid_price_cents bound. Non-zero
	// indicates either (a) a URL-tamper attempt or (b) an upstream
	// exchange bug sending a clearing price above our bid. Either case
	// is suspicious and the metric is the primary signal for a
	// security runbook.
	// Labels: handler — currently {win}; may extend if click/convert
	// acquire their own pricing branches.
	BidderClearingPriceCapped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bidder_clearing_price_capped_total",
		Help: "Count of /win requests where clearing price exceeded the signed bid price and was capped.",
	}, []string{"handler"})

	// CampaignActivationPubSubFailures counts pub/sub delivery failures
	// during campaign start/pause/update. Pub/sub delivery is best-effort;
	// on failure the bidder's periodic 30s loader refresh catches up as
	// an eventual-consistency fallback. High rates here correlate with
	// longer user-visible activation lag.
	// Labels: action — {activated, paused, updated, removed}
	CampaignActivationPubSubFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "campaign_activation_pubsub_failures_total",
		Help: "Count of Redis pub/sub publish failures during campaign activation/pause/update notifications.",
	}, []string{"action"})
)
