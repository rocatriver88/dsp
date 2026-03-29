# TODOS

## P0 — Completed

> All P0 items resolved in commits 17094d5..8b4d649 (2026-03-28)

- ~~Secure bidder endpoints~~ → HMAC-SHA256 token validation (PR1b)
- ~~Remove hardcoded ADVERTISER_ID~~ → API Key auth + frontend multi-tenancy (PR1b + PR3)
- ~~Fix billing model in bidder engine~~ → EffectiveBidCPMCents() ranking (PR2)

## P0.5 — Completed

- ~~CPC charge on click~~ → handleWin skips CPC, handleClick deducts BidCPCCents
- ~~DLQ attempt count persistence~~ → attempt count in event payload, increments on re-queue

## P1 — Completed (final batch)

- ~~Frontend loading/error states~~ → LoadingSkeleton, ErrorState, EmptyState components + 401 redirect
- ~~Grafana dashboard~~ → Prometheus + Grafana in docker-compose, pre-configured DSP dashboard
- ~~Kafka replay auto-start~~ → producer.ReplayBuffer() called at bidder startup

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
