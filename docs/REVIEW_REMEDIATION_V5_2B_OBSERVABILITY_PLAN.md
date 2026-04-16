# V5.2B Observability + Alerts — Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development`. Per-task two-stage review, reviewer-triggered fixes re-dispatched, Phase 2B boundary loop non-negotiable.

**Goal:** Close the observability-and-alerts hole the 2026-04-15 independent review flagged. The DSP spends real ADX dollars but has no business-event metrics (bid rate, win rate, budget drain rate), no readiness-aware health endpoints, and a `Noop` alert pipeline that silently drops every configured alert. Operators cannot distinguish "bidder is idle" from "bidder is wedged" from "bidder is bleeding money."

**Architecture:**
- **Three subsystems**, worked independently: Prometheus business metrics, `/health/live` vs `/health/ready` split, real alert pipeline replacing `alert.Noop{}`.
- Instrumentation is surgical — each metric is added next to the business event it measures, with a single ~5-line `prometheus.NewCounterVec(...)` declaration and a single `.WithLabelValues(...).Inc()` at the event site.
- Health endpoint split is API-server-internal — no client contract change. Existing `/health` keeps behaving as `/health/live` for backward compat.
- Alert pipeline uses the existing `internal/alert` package's interface — only the `Noop{}` implementation is replaced with a real one (Slack webhook + email SMTP).

**Tech Stack:** Go, `prometheus/client_golang`, standard-library `net/http` for health split, `net/smtp` + Slack incoming webhooks for alerts.

**Scope — NOT in this plan:**
- Grafana dashboards (the user has them already per the docker-compose.yml grafana service; this plan only exposes the metrics)
- PagerDuty / on-call integration — Slack + email is the initial production baseline
- OpenTelemetry tracing (distinct from metrics; separate future effort)
- The bidder admin observability panel (Phase 2C has a note about this, but a UI panel is out of Phase 2B scope)

---

## File Structure

**New files:**
- `internal/observability/metrics.go` — centralizes `prometheus.NewCounterVec` / `NewHistogramVec` declarations and registers them with the default registry
- `internal/observability/metrics_test.go` — unit tests for the metric helpers (label cardinality bounds, counter increments)
- `internal/alert/slack.go` — real Slack implementation (POSTs to configured webhook URL)
- `internal/alert/email.go` — real email implementation (`net/smtp`)
- `internal/alert/slack_test.go`, `internal/alert/email_test.go` — unit tests

**Modified files:**
- `cmd/api/main.go` — wire the non-Noop alert impl into `reconSvc` and any other alert consumer
- `cmd/bidder/main.go` — instrument bid-request / win / click / budget-deduction events with prometheus counters
- `internal/handler/routes.go` — add `/health/live` and `/health/ready` routes; `/health/ready` queries Postgres + Redis + ClickHouse
- `internal/bidder/*.go` — instrument internal bidder events (bid strategy decisions, budget check results, guardrail trips)
- `internal/config/config.go` — add `SlackWebhookURL`, `AlertEmailSMTP`, etc. with prod validation
- `docker-compose.yml` (optional) — document that dev Prometheus scrapes the new metrics (existing `prometheus-engine` service should pick up automatically)

---

## Normative metrics decisions

Names follow `<subsystem>_<metric>_<unit>` convention (Prometheus best practice). Labels are bounded to prevent cardinality explosion.

| Metric | Type | Labels | Unit | Purpose |
|---|---|---|---|---|
| `dsp_bid_requests_total` | Counter | `exchange`, `result` (won/lost/passed/rejected) | count | bidder QPS + decision mix |
| `dsp_bid_latency_seconds` | Histogram | `exchange` | seconds | bidder P50/P95/P99 latency |
| `dsp_wins_total` | Counter | `billing_model` (cpm/cpc/ocpm) | count | win rate per billing model |
| `dsp_clicks_total` | Counter | `billing_model` | count | click attribution volume |
| `dsp_budget_deducted_cents_total` | Counter | `billing_model` | cents | real spending rate |
| `dsp_guardrail_trips_total` | Counter | `reason` (daily_budget/max_cpm/manual/other) | count | how often circuit breaker fires |
| `dsp_auction_outcome` | Counter | `outcome` (no_campaigns/under_bid/fraud_rejected/ok) | count | why we lost auctions |
| `dsp_campaign_active_total` | Gauge | — | count | active campaigns right now |
| `dsp_producer_inflight` | Gauge | `topic` | count | Kafka producer inflight depth |
| `dsp_redis_errors_total` | Counter | `op` (get/set/incr/setnx/pubsub) | count | Redis health indicator |

**Cardinality budget:** each counter is at most 10 label-value combinations. The `exchange` label is bounded by `ExchangeRegistry` entries. The `billing_model` label is bounded to `{cpm, cpc, ocpm}`. Nothing in the plan introduces per-campaign or per-advertiser metrics (those are report store concerns, not metric store concerns).

---

## Task 0: Baseline + instrumentation audit

- [ ] **Step 1: Branch + baseline**

```bash
git checkout -b review-remediation-v5.2b-observability
go test ./... -count=1 -timeout 5m
```

- [ ] **Step 2: Catalog existing metrics**

```bash
grep -rn 'prometheus\.\|promhttp\.' internal/ cmd/ --include='*.go'
```

Confirm (as Claude's 2026-04-15 review found) that the only existing Prometheus reference is `/metrics` endpoint serving Go runtime metrics — no business-event counters.

- [ ] **Step 3: Inventory instrumentation sites**

List every location in `cmd/bidder/main.go` and `internal/bidder/` where a business event fires (bid received, win, click, budget deduction, guardrail check, etc.). Task 2 adds a counter increment at each of these.

- [ ] **Step 4: Commit baseline doc**

```bash
git add docs/REVIEW_REMEDIATION_V5_2B_METRICS_INVENTORY.md  # from Step 3
git commit -m "docs(v5.2b): observability instrumentation baseline"
```

---

## Task 1: `internal/observability/metrics.go` — counter declarations

- [ ] **Step 1: Write failing test**

Create `internal/observability/metrics_test.go`:

```go
func TestMetrics_AllDeclarationsRegistered(t *testing.T) {
	// Verify every counter is registered with the default registry.
	registered := prometheus.DefaultGatherer.Gather()
	// ... assert each expected metric name is present
}

func TestMetrics_LabelCardinalityBounded(t *testing.T) {
	// Each metric's label values come from a closed set. Inc with an
	// unknown label value returns an error path (or no-op), not a
	// new time series.
}
```

- [ ] **Step 2: Implement**

```go
package observability

import "github.com/prometheus/client_golang/prometheus"

var (
	BidRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dsp_bid_requests_total",
			Help: "Total bid requests received by the bidder.",
		},
		[]string{"exchange", "result"},
	)
	// ... all metrics from the normative table ...
)

func init() {
	prometheus.MustRegister(
		BidRequestsTotal,
		// ...
	)
}
```

- [ ] **Step 3: Run tests, commit**

---

## Task 2: Instrument `cmd/bidder/main.go` + `internal/bidder/`

- [ ] For each site in Task 0 Step 3, add `.WithLabelValues(...).Inc()` at the event.
- [ ] For latency: wrap the handler in a timed block and `.Observe(time.Since(start).Seconds())`.
- [ ] Unit test each instrumented site: fire the event in a test, read `prometheus.DefaultGatherer.Gather()`, assert the counter increased.
- [ ] Commit

---

## Task 3: `/health/live` + `/health/ready` split

- [ ] **Step 1: Write failing test**

```go
func TestHealth_Live_AlwaysOK(t *testing.T) {
	// /health/live always returns 200 as long as the process is running,
	// regardless of backend health.
}

func TestHealth_Ready_FailsOnBackendOutage(t *testing.T) {
	// /health/ready probes Postgres+Redis+ClickHouse. If any is down,
	// returns 503 with a JSON body listing the failed probes.
}
```

- [ ] **Step 2: Implement**

Add to `internal/handler/routes.go`:
- `/health/live` — returns 200 immediately
- `/health/ready` — queries `db.Ping`, `redis.Ping`, `clickhouse.Ping` (if wired), returns 200 or 503 + failure detail
- Existing `/health` stays as an alias for `/health/live` so existing dashboards don't break

- [ ] **Step 3: Run tests, commit**

---

## Task 4: Real alert pipeline

- [ ] **Step 1: Add config fields**

`internal/config/config.go`:
- `SlackWebhookURL string` — env `SLACK_WEBHOOK_URL`
- `AlertEmailSMTPHost`, `AlertEmailSMTPPort`, `AlertEmailFrom`, `AlertEmailTo` — env `ALERT_EMAIL_*`
- Validate: in production, at least one of Slack or email must be configured (not both required, but Noop forbidden).

- [ ] **Step 2: Implement `internal/alert/slack.go`**

```go
type Slack struct { webhookURL string }
func (s *Slack) Send(ctx context.Context, severity, title, body string) error {
	payload := map[string]any{"text": fmt.Sprintf("*%s* — %s\n%s", severity, title, body)}
	// POST to s.webhookURL, 5s timeout, return error on non-2xx
}
```

- [ ] **Step 3: Implement `internal/alert/email.go`**

Simple `net/smtp.SendMail` wrapper.

- [ ] **Step 4: Wire in `cmd/api/main.go`**

Replace `reconSvc := reconciliation.New(rdb, store, reportStore, billingSvc, alert.Noop{})` with:

```go
var alertImpl alert.Alerter
if cfg.SlackWebhookURL != "" {
	alertImpl = alert.NewSlack(cfg.SlackWebhookURL)
} else if cfg.AlertEmailSMTPHost != "" {
	alertImpl = alert.NewEmail(cfg.AlertEmail...)
} else {
	alertImpl = alert.Noop{}
}
reconSvc := reconciliation.New(rdb, store, reportStore, billingSvc, alertImpl)
```

- [ ] **Step 5: Unit tests + integration test (httptest server that echoes the Slack payload)**

- [ ] **Step 6: Commit**

---

## Task 5: Phase 2B boundary loop

Same shape as Phase 1 / Phase 2A — requesting-code-review, verification-before-completion, /qa. Verify:
- `/metrics` now exposes `dsp_*` business metrics (grep for them in live scrape output)
- `/health/ready` correctly fails when a backend is down (stop Redis, hit the endpoint, assert 503)
- Alert pipeline fires end-to-end to a test Slack webhook (or staging email)

---

## Out of scope

- Phase 2A contract unification (separate plan)
- Phase 2C security-adjacent + lifecycle (separate plan)
- Per-advertiser dashboards (report store concern, not metrics concern)
- Grafana dashboard JSON (user maintains those in `docker-compose.yml`'s grafana service)
