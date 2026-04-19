# Campaigns API Contract

本文档定义 `/api/v1/campaigns/*` 端点的对外语义契约,聚焦 activation / pause / update 这类涉及跨系统一致性的操作。与 `docs/contracts/biz-engine.md` 互补:后者描述 biz↔engine 之间的数据同步机制(pub/sub + 30s periodic reload),本文件描述客户端可以观察到的 HTTP 行为(响应码、失败模式、SLA)。

**适用范围**
- api 侧:`internal/handler/campaign.go`(`HandleStartCampaign` / `HandlePauseCampaign` / `HandleUpdateCampaign`)
- engine 侧:`internal/bidder/loader.go`(`CampaignLoader.listenPubSub` / `periodicRefresh`)
- 观测:`internal/observability` 下的 activation / legacy-token / clearing-price 指标

---

## Campaign Activation Contract

### Activation sequence (post-2026-04)

`POST /api/v1/campaigns/{id}/start` executes in this order:

1. **Validate tenant + preconditions** (campaign exists, creatives approved, end_date valid, budget_total >= budget_daily).
2. **Balance check**: sandbox → skip; else `BillingSvc.GetBalance` → 503 if error, 422 if insufficient.
3. **`BudgetSvc.InitDailyBudgetNX`**: 503 if error (no DB transition yet, so no orphan state). NX semantics preserve any already-spent counter.
4. **`Store.TransitionStatus(active)`**: 409 on conflict.
5. **`NotifyCampaignUpdate`** pub/sub: errors recorded as metric, do NOT block response.
6. Return 200.

**Why InitDailyBudget runs before TransitionStatus:** if it failed and we'd committed active first, the campaign would appear active in Postgres but be no-bid forever in the bidder (missing daily key = 0 budget). Prepare-then-commit ordering eliminates this.

**Why NX semantics:** pause → resume within the same day must NOT refill spent amount (that would be a budget bypass). Daily reset is handled exclusively by the midnight cron job in `cmd/bidder/main.go:145`.

### Bidder visibility after 200

1. **Pub/sub fast path (<1s)**: `campaign:updates action=activated` reaches `CampaignLoader.listenPubSub`; loader re-queries Postgres, updates in-memory map, **reinits DailyBudget with NX** (recovery path if handler-side init needed retry; NX is a no-op when key already exists).
2. **Periodic fallback (<=30s)**: If pub/sub delivery fails, `CampaignLoader.periodicRefresh` picks up the campaign during the next 30s full-load cycle; also reinits DailyBudget with NX.

### SLA

Clients MUST NOT assume a 200 on `/start` means the bidder is instantly serving. The guarantee is **bidder is serving within 30 seconds of the 200 response**, given Postgres + bidder process healthy.

### Failure-mode matrix

| Failure | Response | Recovery |
|---------|----------|----------|
| Postgres tenant lookup error | 404 | N/A (never activated) |
| `BillingSvc.GetBalance` error | 503 | Client retry |
| Balance insufficient | 422 | Top up + retry |
| `BillingSvc` nil at runtime | 503 | Wiring bug — startup assert in `cmd/api/main.go:126` should catch it at boot |
| `BudgetSvc.InitDailyBudgetNX` error | 503 | Client retry (Redis-recovery dependent); no DB transition |
| `Store.TransitionStatus` conflict | 409 | Already active or state-machine transition disallowed |
| `NotifyCampaignUpdate` pub/sub error | 200 + metric | Loader 30s refresh recovers |
| Post-200 Redis outage (loader rebuilding) | 200 | Loader reinit on next periodic refresh; NX keeps counters intact |

### Monitoring

- `campaign_activation_pubsub_failures_total{action="activated"|"paused"|"updated"}` — pub/sub delivery failures. Sustained non-zero rate → users hitting the 30s fallback path.
- `bidder_token_legacy_accepted_total{handler="win"|"click"|"convert"}` — transitional HMAC token validations. Expected to spike briefly after a Phase 2 deploy, then return to zero within 5-min token TTL. Sustained non-zero = stuck legacy path, investigate (deploy-window removal pending — see Phase 2 follow-up F7).
- `bidder_clearing_price_capped_total{handler="win"}` — URL `price` param exceeded signed `bid_price_cents`. Non-zero indicates URL tampering attempt or upstream exchange bug — investigate.
