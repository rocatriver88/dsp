# Module Architecture Guide

## 1. System Overview
The repository is organized as a multi-service DSP platform:

- `cmd/api` exposes advertiser APIs, admin APIs, uploads, reporting, and billing endpoints
- `cmd/bidder` handles bid requests and callback traffic
- `cmd/consumer` persists event streams to ClickHouse for analytics
- `web/` provides the operator-facing UI
- `cmd/autopilot`, `cmd/exchange-sim`, `cmd/simulate`, and `cmd/resetbudget` support verification and operations

Runtime dependencies are PostgreSQL for transactional state, Redis for hot-path controls and coordination, Kafka for event transport, and ClickHouse for reporting queries.

## 2. API Service Architecture
`cmd/api/main.go` wires the main control-plane surface.

### Public API
The public mux serves advertiser-scoped endpoints under `/api/v1/` for:
- advertisers
- campaigns
- creatives
- reports
- analytics
- billing
- uploads
- registration

Authentication is handled with API keys through `internal/auth`. CORS, request IDs, structured logging, and rate limiting are applied in the HTTP stack.

### Internal and Admin API
Admin routes are mounted behind `handler.AdminAuthMiddleware` and exposed on the separate internal port. This isolates operational APIs such as registration approval, creative review, invite codes, audit log access, circuit controls, and health views from the public advertiser surface.

### Domain Modules
The API service composes these main packages:
- `internal/campaign` for advertiser, campaign, and creative persistence
- `internal/billing` for balance, top-up, and spend bookkeeping
- `internal/registration` for invite and onboarding flow
- `internal/reporting` for ClickHouse-backed reporting queries
- `internal/audit` for sensitive action logging
- `internal/autopause`, `internal/reconciliation`, and `internal/guardrail` for background protection tasks

## 3. Bidder Architecture
`cmd/bidder/main.go` is the data-plane service responsible for request-time decisioning.

### Request Flow
1. Receive OpenRTB or exchange-normalized request
2. Parse device and geo context
3. Load active campaign state through `internal/bidder`
4. Run anti-fraud filters in `internal/antifraud`
5. Enforce budget and frequency limits via `internal/budget`
6. Apply bid strategy with pacing and performance signals
7. Enforce guardrail checks
8. Emit bid response and event stream
9. Process win, click, and convert callbacks with HMAC verification

### Supporting Components
- `internal/bidder/loader.go` maintains active campaign state
- `internal/bidder/strategy.go` adjusts bids using performance and pacing signals
- `internal/bidder/statscache.go` refreshes CTR or CVR inputs from ClickHouse to Redis
- `internal/exchange` abstracts exchange-specific protocol normalization
- `internal/events` produces Kafka events with replay and dead-letter support

## 4. Analytics and Reporting Pipeline
The reporting pipeline is split between online event production and offline query serving.

- Bidder publishes events to Kafka topics
- `cmd/consumer` reads analytics topics and persists normalized events to ClickHouse
- `internal/reporting` provides query APIs for campaign stats, hourly views, geo distribution, bid transparency, attribution, and overview metrics
- Export endpoints and SSE analytics surfaces are layered on top of the reporting package

This split keeps bidder latency-sensitive logic separate from heavier reporting queries.

## 5. Web Application Architecture
The frontend under `web/` uses Next.js App Router.

- `web/app/` contains route segments for advertiser and admin pages
- `web/lib/` contains shared client logic, including generated API types
- `web/public/` holds static assets

The frontend depends on the backend contract generated from OpenAPI. When API shapes change, `make api-gen` must refresh both `docs/openapi3.yaml` and `web/lib/api-types.ts`.

## 6. Cross-Cutting Concerns
- `internal/config` centralizes environment-driven configuration
- `internal/observability` provides structured logging and request IDs
- Prometheus metrics are exposed by API and bidder services
- Docker Compose files define the local platform topology
- `scripts/test-env.sh` orchestrates isolated integration validation
- `cmd/autopilot` provides scenario-based end-to-end verification

## 7. Architecture Characteristics

### Strengths
- clear split between control plane, bidding plane, analytics ingestion, and UI
- good package separation across campaign, billing, reporting, fraud, and guardrail concerns
- local observability and replay mechanisms already included

### Current Constraints
- exchange coverage is still narrow
- compliance and finance hardening remain roadmap items
- some product flows are implemented at operational depth, not full enterprise depth

## 8. Recommended Reading Order
For a new contributor, read in this order:
1. `cmd/api/main.go`
2. `cmd/bidder/main.go`
3. `cmd/consumer/main.go`
4. `internal/handler/`
5. `internal/bidder/`
6. `internal/reporting/`
7. `web/`
8. `TODOS.md`
