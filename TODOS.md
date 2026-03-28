# TODOS

## P0 — Completed

> All P0 items resolved in commits 17094d5..8b4d649 (2026-03-28)

- ~~Secure bidder endpoints~~ → HMAC-SHA256 token validation (PR1b)
- ~~Remove hardcoded ADVERTISER_ID~~ → API Key auth + frontend multi-tenancy (PR1b + PR3)
- ~~Fix billing model in bidder engine~~ → EffectiveBidCPMCents() ranking (PR2)

## P0.5 — Remaining

### CPC charge on click (not impression)
**What:** handleWin should skip budget deduction for CPC campaigns. handleClick should deduct budget for CPC campaigns instead. Currently all campaigns are charged on win notice regardless of billing model.
**Why:** CPC advertisers pay per click, not per impression. Charging on impression is a real money bug.
**Effort:** S (human: ~2 hours / CC: ~10 min)
**Depends on:** Nothing
**Context:** `cmd/bidder/main.go` handleWin always deducts. Need to check `loader.GetCampaign(id).BillingModel` and skip deduction if CPC. handleClick needs budget deduction added.

### DLQ rate limiting + attempt count persistence
**What:** (1) DLQ retry goroutine should rate-limit to 10 events/sec. (2) Attempt count stored in event payload, not in-memory.
**Why:** Outside voice flagged unbounded replay and lost attempt counts on restart.
**Effort:** S (CC: ~10 min)
**Depends on:** Nothing

## P1 — Remaining

### Frontend loading/error states + 401 redirect
**What:** Skeleton loaders for data tables, error boundaries with retry. Global 401 interceptor in api.ts that clears localStorage key and redirects to login.
**Why:** Pages jump from blank to populated. 401 with expired/revoked key shows raw error instead of redirecting.
**Effort:** M (human: ~2 days / CC: ~20 min)
**Depends on:** Nothing

### Grafana dashboard in docker-compose
**What:** Add Grafana service to docker-compose.yml with pre-configured Prometheus datasource and DSP dashboard (bid latency, win rate, active campaigns, auto-pause events).
**Why:** Prometheus metrics exist but no visualization. Grafana completes the observability stack.
**Effort:** S (human: ~2 hours / CC: ~15 min)
**Depends on:** Nothing

### Kafka replay goroutine auto-start
**What:** Call `producer.ReplayBuffer(ctx)` at bidder startup before starting HTTP server.
**Why:** ReplayBuffer method exists but is never called. Buffered events from prior Kafka outages stay on disk forever.
**Effort:** S (CC: ~5 min)
**Depends on:** Nothing

## P1 — Completed

> Resolved in commits 17094d5..8b4d649 (2026-03-28)

- ~~Fix CORS wildcard~~ → origin allowlist (PR1a)
- ~~Add pause_reason field~~ → migration 007 + model (PR2)
- ~~Kafka event replay~~ → ReplayBuffer method (PR2)
- ~~Dead-letter queue~~ → dsp.dead-letter topic + SendToDeadLetter (PR2)
- ~~Fail /win on Redis budget failure~~ → already existed, confirmed (PR2)
- ~~Batch-load creatives~~ → GetCreativesByCampaigns batch query (PR2)
- ~~Separate internal API port~~ → :8182 (PR1a)
- ~~Add Redis/ClickHouse auth~~ → requirepass + user/password (PR1a)
- ~~Full observability~~ → slog JSON + Prometheus /metrics + request_id (PR3 + final)

## P2 — Deferred

### PIPL compliance
**What:** Audit data flows for China PIPL compliance, add consent mechanisms, ensure data localization.
**Depends on:** Legal review of specific PIPL requirements

### Multiple exchange integrations
**What:** Extend exchange adapter to support 3+ real Chinese exchanges.
**Depends on:** Partnership agreements

### Advanced admin panel
**What:** Admin dashboard for managing advertisers, approving registrations, viewing system health.
**Depends on:** Auth system + multi-tenancy (now available)

### Automated billing reconciliation
**What:** Daily reconciliation between Redis spend tracking and ClickHouse event aggregation.
**Depends on:** Production traffic data

### Bid simulator
**What:** "What if I changed my bid?" predictive tool using historical data.
**Depends on:** Sufficient historical bid data in ClickHouse

### Alipay/WeChat Pay integration
**What:** Self-serve payment for advertiser balance topup.
**Depends on:** Billing system hardening
