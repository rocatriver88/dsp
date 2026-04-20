# Exchange Adapter Onboarding Checklist

Before registering a new `exchange.Adapter` in `cmd/bidder/main.go` (via `exchange.DefaultRegistry.Register(...)`) or wiring a custom registry, complete every item on this checklist. The bidder's `/win` clearing-price cap (see `docs/contracts/campaigns.md` and issue [#29](https://github.com/rocatriver88/dsp/issues/29)) assumes all adapters produce price-normalized OpenRTB, so onboarding discipline is load-bearing for billing correctness.

## Why this checklist exists

The bidder's `handleWin` caps the URL `price` by the HMAC-signed `bid_price_cents` (Phase 2 F2.5 / Codex Finding #2). The cap math at `cmd/bidder/main.go` treats both sides as **per-impression dollars**. If an adapter feeds through a `${AUCTION_PRICE}` substitution in a different unit (raw CPM, yuan, sub-units, etc.), the cap either:

- Under-caps → URL tampering succeeds silently, budget is drained 1000× faster than bid
- Over-caps → every legitimate win gets clamped, billing is 1000× too low, metric `bidder_clearing_price_capped_total{handler="win"}` spikes to 100% of traffic

Neither is acceptable. Every adapter MUST normalize to the contract below.

## The contract

`Adapter.ParseBidRequest` and `Adapter.FormatBidResponse` MUST treat price in the same unit the internal OpenRTB path uses:

| Field | Unit | Where |
|-------|------|-------|
| `imp[i].bidfloor` | per-impression, bid currency (typically USD) | Input to `Engine.Bid` |
| `seatbid[0].bid[0].price` | per-impression, bid currency | Output of `Engine.Bid`, consumed by NURL |
| `${AUCTION_PRICE}` macro substitution | per-impression, bid currency | URL-encoded by exchange when firing NURL |

Exchanges that natively quote CPM must divide by 1000 at `ParseBidRequest` entry and multiply by 1000 at `FormatBidResponse` exit.

Exchanges using non-USD currency must convert — prefer exchange-rate sourced from config, not hardcoded — and document the FX source in the adapter's doc comment.

## Checklist

When preparing a new adapter PR:

- [ ] **Unit convention documented**: adapter's type-level doc comment names its native price unit (CPM vs per-impression, currency) and shows the normalization step
- [ ] **ParseBidRequest normalizes**: if exchange sends CPM → `bidfloor *= 0.001` inside Parse; if non-USD → FX-convert; commits the normalized value to the returned `*openrtb2.BidRequest`
- [ ] **FormatBidResponse de-normalizes**: reverse of above so the exchange sees prices in its expected format
- [ ] **Unit test for price normalization**: `TestExchange_<Name>_PriceUnitNormalization` that:
    - Seeds a request with known non-standard unit (e.g., CPM $2.00 = `bidfloor=2.00`)
    - Calls `ParseBidRequest`
    - Asserts the returned `bidfloor` is in per-impression dollars (`0.002`)
- [ ] **Unit test for response round-trip**: `TestExchange_<Name>_BidResponseRoundTrip` that:
    - Takes a `*openrtb2.BidResponse` with `price=0.00500` (per-impression $0.005 = $5 CPM)
    - Calls `FormatBidResponse`
    - Asserts the emitted JSON contains the exchange's expected unit (e.g., CPM `"price": 5.0`)
- [ ] **Integration test against mock exchange endpoint**: if the adapter hits an HTTP endpoint, mock it and assert the clearing-price cap in /win fires or passes as expected given a full round-trip
- [ ] **Registry registration**: add the adapter in `cmd/bidder/main.go` under the `DefaultRegistry` wiring with a clear comment: `// <Name>: native CPM, normalized to per-impression in adapter.ParseBidRequest`
- [ ] **Monitoring watch**: note in the PR body that after deploy, `bidder_clearing_price_capped_total{handler="win"}` should stay near zero for the new exchange's traffic. Non-zero indicates a normalization bug.
- [ ] **Update `docs/contracts/campaigns.md`** if the cap behavior differs for this exchange (edge cases, exemptions)

## Reference: existing adapters

- `exchange.DefaultSelfAdapter` (`/bid/self`) — native OpenRTB 2.5, no normalization needed. Spec-compliant per-impression pricing.
- `exchange.NewCustomAdapter` — template for non-standard exchanges. Callers supply `parseFn` / `formatFn` closures that MUST perform normalization if required.

## If you're debugging a unit mismatch

Symptoms:
- Reports show clearing prices 1000× high or low
- `bidder_clearing_price_capped_total{handler="win"}` saturates for one exchange
- `[WIN] clearing price ... exceeded signed bid cap` log spam

Debug order:
1. Add a log inside the adapter's `ParseBidRequest` dumping the inbound `bidfloor` alongside what you return
2. Compare to `bid.Price` in decorator's `decorateBidResponse` for the same `req.ID`
3. If they differ by 1000×, the normalization step is missing or doubled

## References

- Price-unit contract: `internal/exchange/adapter.go` type-level doc on `Adapter`
- Cap logic: `cmd/bidder/main.go` (handleWin `bidder_clearing_price_capped_total` branch)
- Activation monitoring contract: `docs/contracts/campaigns.md`
- Related findings: issue [#29](https://github.com/rocatriver88/dsp/issues/29) (F8)
