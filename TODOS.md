# TODOS

## P0 — Security Critical

### Secure bidder endpoints
**What:** Add auth token validation on bidder /win and /click endpoints. Exchanges include a signed token in callback URLs; bidder validates before processing.
**Why:** Without this, any attacker can POST fake win notices and drain campaign budgets. /bid, /win, /click have zero auth today.
**Effort:** S (human: ~4 hours / CC: ~10 min)
**Depends on:** Nothing
**Context:** Outside voice from CEO review (2026-03-28) flagged this. The bidder is the SSP-facing endpoint. Win notices and click callbacks need signed tokens embedded in the notification URLs generated during bid response.

### Remove hardcoded ADVERTISER_ID = 1 from frontend
**What:** Add auth context provider, token storage (HttpOnly cookie for refresh, memory for access), login redirect on 401, protected route wrapper to Next.js frontend. Remove all hardcoded `ADVERTISER_ID = 1` references.
**Why:** Without this, multi-tenancy doesn't work in the UI. Every page assumes a single advertiser.
**Effort:** L (human: ~1 week / CC: ~30 min)
**Depends on:** Auth system implementation (JWT + API server middleware)
**Context:** `web/app/page.tsx` line 8 hardcodes `const ADVERTISER_ID = 1`. `web/lib/api.ts` has no concept of auth tokens. Frontend effort is closer to L than originally estimated M.

### Fix billing model in bidder engine
**What:** Use `EffectiveBidCPMCents()` for campaign ranking instead of hardcoded `BidCPMCents`. Charge CPC campaigns on click event, not impression.
**Why:** CPC and oCPM campaigns are accepted by the API but the bidder always ranks by CPM and charges on impression. Real money bug.
**Effort:** S (human: ~4 hours / CC: ~10 min)
**Depends on:** Nothing
**Context:** `internal/bidder/engine.go` line ~85 hardcodes `c.BidCPMCents`. `EffectiveBidCPMCents()` method exists in `internal/campaign/model.go` but is never called.

## P0.5 — Implementation Guidance (from CEO Review Session 2, 2026-03-28)

### PR split strategy
**What:** Split PR1 (security) into PR1a (infra: Redis/CH auth, CORS, internal port + admin route move) and PR1b (auth: HMAC, API Key middleware, rate limiting). Total: PR1a, PR1b, PR2 (correctness), PR3 (quality).
**Why:** Outside voice flagged PR1 as a 6-change monolith. Smaller PRs are easier to review and any one breaking CI doesn't block the others.
**Effort:** N/A (organizational, not code)
**Depends on:** Nothing

### BIDDER_PUBLIC_URL env var
**What:** Replace hardcoded `http://localhost:PORT` in `cmd/bidder/main.go:126` with env var `BIDDER_PUBLIC_URL` for constructing nurl and click callback URLs.
**Why:** Outside voice flagged this. Current system sends localhost URLs to exchanges, making win/click callbacks unreachable in any real deployment. HMAC tokens on unreachable URLs are useless.
**Effort:** S (human: ~30 min / CC: ~5 min)
**Depends on:** Nothing

### Move admin endpoints to internal port
**What:** Move `/api/v1/admin/*` routes to internal port 8182 alongside `/internal/*`. Also fix billing topup to validate advertiser_id from auth context (IDOR).
**Why:** Outside voice flagged this. Admin endpoints (approve/reject registrations) and billing topup are currently unauthenticated on the public port.
**Effort:** S (human: ~1 hour / CC: ~10 min)
**Depends on:** Internal port separation

### Win notice deduplication
**What:** Redis SETNX `win:{request_id}` + 5-min TTL before budget deduction. Duplicate wins return 200 but skip deduction and Kafka events.
**Why:** Exchange may retry win notices. Current code deducts budget on every call. Real money double-charge risk.
**Effort:** S (human: ~30 min / CC: ~5 min)
**Depends on:** Nothing

### Campaign validation on start
**What:** `handleStartCampaign` should reject: (1) campaigns with 0 creatives, (2) campaigns with end_date in the past, (3) campaigns with budget_total < budget_daily.
**Why:** CEO review Section 4 flagged these interaction edge cases.
**Effort:** S (human: ~30 min / CC: ~5 min)
**Depends on:** Nothing

### DLQ implementation details
**What:** (1) DLQ retry goroutine should rate-limit to 10 events/sec to avoid overwhelming ClickHouse during recovery. (2) Attempt count should be stored in the event payload (not in-memory) so it survives process restarts.
**Why:** Outside voice flagged unbounded replay and lost attempt counts.
**Effort:** S (included in DLQ implementation)
**Depends on:** Nothing

### CPC auto-pause interaction
**What:** Auto-pause CTR anomaly rule should only trigger for CPM campaigns. CPC campaigns have a structurally different impression-to-click ratio because budget deduction happens on click, not impression. A 5% CTR on a CPC campaign is normal, not anomalous.
**Why:** Outside voice flagged that CPC impressions still land in ClickHouse but budget deduction moves to click, making CTR-based anomaly detection produce false positives on CPC campaigns.
**Effort:** S (add billing_model check to anomaly rule)
**Depends on:** Auto-pause implementation

### LoadedCampaign type alignment
**What:** `bidder.LoadedCampaign` struct needs `BillingModel`, `BidCPCCents`, `OCPMTargetCPACents` fields and an `EffectiveBidCPMCents()` method (or delegate to `campaign.Campaign`).
**Why:** Outside voice flagged that `EffectiveBidCPMCents()` exists on `campaign.Campaign` but bidder uses `LoadedCampaign` which lacks these fields.
**Effort:** S (human: ~1 hour / CC: ~10 min)
**Depends on:** Nothing

## P1 — Production Hardening

### Fix CORS wildcard
**What:** Replace `Access-Control-Allow-Origin: *` with explicit origin allowlist in `cmd/api/main.go`.
**Why:** Wildcard CORS breaks `SameSite=Strict` cookies. Auth refresh token flow will fail in browsers.
**Effort:** S (human: ~2 hours / CC: ~5 min)
**Depends on:** Nothing
**Context:** `cmd/api/main.go` sets wildcard CORS. Must be fixed before JWT auth ships.

### Add pause_reason field to campaigns
**What:** Migration to add `pause_reason TEXT` column to campaigns table. Update Campaign model in Go. Auto-pause feature writes the reason (spend spike, CTR collapse, etc.).
**Why:** Auto-pause is meaningless without telling the advertiser why their campaign was paused.
**Effort:** S (human: ~2 hours / CC: ~5 min)
**Depends on:** Nothing. Include in migration batch (007/008/009).

### Kafka event replay for disk buffer
**What:** Add background goroutine that watches `/tmp/dsp-kafka-buffer/{topic}.jsonl` and replays events to Kafka when connectivity is restored.
**Why:** Events buffered during Kafka outage stay on disk forever. No replay mechanism exists.
**Effort:** S (human: ~4 hours / CC: ~15 min)
**Depends on:** Nothing
**Context:** `internal/events/producer.go` has the disk buffer write path but no read-back/replay path.

### Dead-letter queue for ClickHouse insert failures
**What:** Failed ClickHouse inserts go to a dead-letter Kafka topic (`dsp.dead-letter`) for retry/investigation. Consumer auto-reconnects to ClickHouse with exponential backoff instead of crashing.
**Why:** Current consumer skips failed inserts (events lost forever) and crashes on connection loss.
**Effort:** S (human: ~4 hours / CC: ~15 min)
**Depends on:** Nothing
**Context:** `cmd/consumer/main.go` has no error recovery. CEO review Section 2 flagged this.

### Fail /win on Redis budget failure
**What:** Return error status to the exchange when Redis budget deduction fails. Don't count the impression. Exchange may retry.
**Why:** Current behavior logs the error but continues, causing budget overrun with real money.
**Effort:** S (human: ~2 hours / CC: ~5 min)
**Depends on:** Nothing
**Context:** CEO review Section 2 flagged this.

### Batch-load creatives in campaign loader
**What:** Replace N+1 creative queries with single `SELECT * FROM creatives WHERE campaign_id IN (...)` and map results by campaign_id.
**Why:** 251 queries every 30 seconds with 250 campaigns. Reduces to 2 queries.
**Effort:** S (human: ~2 hours / CC: ~5 min)
**Depends on:** Nothing
**Context:** `internal/bidder/loader.go` loads creatives per-campaign in a loop.

### Separate internal API port
**What:** Move `/internal/*` routes to a separate HTTP listener on port 8182. Only accessible within Docker network.
**Why:** `/internal/active-campaigns` on the public port exposes all campaign data including targeting, budgets, and bid prices.
**Effort:** S (human: ~2 hours / CC: ~5 min)
**Depends on:** Nothing

### Add Redis/ClickHouse auth
**What:** Configure `requirepass` for Redis and user/password for ClickHouse in docker-compose.yml. Update connection strings in Go services.
**Why:** Both services are currently unauthenticated. Exposed to any process on the network.
**Effort:** S (human: ~2 hours / CC: ~10 min)
**Depends on:** Nothing

### Add loading + error states to all frontend pages
**What:** Skeleton loaders for data tables, spinners for stats cards, error boundaries with retry buttons. Global 401 interceptor that redirects to /login.
**Why:** Every page currently jumps from blank to populated with no feedback. Production dashboard managing real money needs loading/error states.
**Effort:** M (human: ~2 days / CC: ~20 min)
**Depends on:** Frontend auth implementation

### Full observability stack
**What:** JSON structured logging (Go slog), Prometheus /metrics endpoints on each service, request_id propagation via headers, Grafana dashboard in docker-compose.
**Why:** Zero structured logging, no metrics, no request tracing. Can't debug production issues.
**Effort:** M (human: ~2 days / CC: ~30 min)
**Depends on:** Nothing

## P2 — Deferred

### PIPL compliance
**What:** Audit data flows for China PIPL compliance, add consent mechanisms, ensure data localization.
**Why:** Legal requirement for operating a DSP in China.
**Effort:** XL (human) -> L (CC: ~2 hours)
**Depends on:** Legal review of specific PIPL requirements
**Context:** System handles GDPR but not PIPL. PIPL has stricter requirements around data localization, consent, cross-border transfer.

### Multiple exchange integrations
**What:** Extend exchange adapter to support 3+ real Chinese exchanges.
**Depends on:** Exchange adapter refactor + partnership agreements

### Advanced admin panel
**What:** Admin dashboard for managing advertisers, approving registrations, viewing system health.
**Depends on:** Auth system + multi-tenancy

### Automated billing reconciliation
**What:** Daily reconciliation between Redis spend tracking and ClickHouse event aggregation.
**Depends on:** Production traffic data

### Bid simulator
**What:** "What if I changed my bid?" predictive tool using historical data.
**Depends on:** Sufficient historical bid data in ClickHouse

### Alipay/WeChat Pay integration
**What:** Self-serve payment for advertiser balance topup.
**Depends on:** Auth system + billing system hardening
