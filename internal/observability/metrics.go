package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Bidder metrics
	BidRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_bid_requests_total",
		Help: "Total bid requests processed",
	}, []string{"result"}) // result: bid, no_bid, error

	BidLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "dsp_bid_latency_seconds",
		Help:    "Bid request processing latency",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25},
	})

	WinNoticesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_win_notices_total",
		Help: "Total win notices processed",
	}, []string{"status"}) // status: ok, duplicate, error, rejected

	// API metrics
	APIRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_api_requests_total",
		Help: "Total API requests",
	}, []string{"method", "path", "status"})

	RateLimitHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dsp_rate_limit_hits_total",
		Help: "Total rate limit rejections",
	})

	// Campaign metrics
	ActiveCampaigns = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dsp_active_campaigns",
		Help: "Number of active campaigns in bidder cache",
	})

	AutoPauseTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_auto_pause_total",
		Help: "Total auto-pause actions",
	}, []string{"reason"})

	// Consumer metrics
	KafkaConsumeErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dsp_kafka_consume_errors_total",
		Help: "Total Kafka consume errors",
	}, []string{"topic"})

	DLQPublishTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dsp_dlq_publish_total",
		Help: "Total events published to dead-letter queue",
	})
)
