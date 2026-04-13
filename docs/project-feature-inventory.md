# DSP Platform Feature Inventory

## Scope
This document summarizes the functional surface already implemented in the repository. It is intended for product, engineering, QA, and review handoff.

## 1. Platform Shape
- API service under `cmd/api` for advertiser-facing and admin-facing HTTP APIs
- Bidder service under `cmd/bidder` for OpenRTB bidding, win, click, and convert callbacks
- Consumer service under `cmd/consumer` for Kafka to ClickHouse analytics ingestion
- Next.js dashboard under `web/` for advertiser and admin workflows
- Operational tools under `cmd/autopilot`, `cmd/exchange-sim`, `cmd/simulate`, and `cmd/resetbudget`

## 2. Advertiser Onboarding and Access Control
- Advertiser creation and lookup APIs
- API key based advertiser authentication via `X-API-Key`
- Admin token based internal and admin APIs via `X-Admin-Token`
- Self-registration flow with approval and rejection
- Invite code issuance, usage counting, and consumption
- Basic multi-tenant separation at API and storage access layer

## 3. Campaign Management
- Campaign create, list, detail, update, start, and pause flows
- Campaign state transitions including `draft`, `active`, `paused`, `completed`, and `deleted`
- Budget controls for total and daily limits
- Targeting fields for geo, OS, device, frequency, audience include and exclude, plus age and gender model support

## 4. Billing and Spend Logic
- Billing model support for CPM, CPC, and oCPM
- Unified bid comparison through effective CPM normalization
- CPC charging on click path
- CPM and oCPM charging on win or impression path
- Balance lookup, top-up, transactions, spend logging, invoice primitives, and reconciliation jobs

## 5. Creative and Asset Management
- Creative create, update, delete, and list flows
- Review states for pending, approved, and rejected creatives
- Upload endpoint plus static asset serving from `uploads/`
- Creative support for banner, native, splash, and interstitial formats
- Native asset support for title, description, icon, image, and CTA

## 6. Bidding, Guardrails, and Fraud Controls
- Standard `/bid` endpoint and exchange-specific `/bid/{exchange_id}` adapter entry
- Win, click, and convert callback endpoints
- HMAC token validation for callback authenticity
- Redis-backed budget and frequency enforcement
- Dynamic bid strategy using pacing, win-rate feedback, and cached CTR/CVR signals
- Anti-fraud filters for blacklist, datacenter traffic, abnormal UA, and request frequency
- Guardrails for global budget caps, bid ceiling, circuit breaker, low balance, and spend spikes
- Auto-pause rules for exhausted budgets and abnormal campaign metrics

## 7. Analytics, Reporting, and Audit
- Kafka event production with replay buffer and dead-letter handling
- ClickHouse event ingestion into reporting store
- Reporting endpoints for campaign stats, hourly stats, geo breakdown, bid transparency, attribution, overview, CSV export, and SSE analytics stream
- Audit logging for campaign, creative, billing, and registration actions
- Admin APIs for registrations, creative review, advertiser overview, top-up, invite codes, circuit controls, audit log, and system health

## 8. Frontend and Operations
- Advertiser dashboard pages for overview, campaigns, billing, reports, analytics, and creative workflows
- Admin pages for overview, agencies, creatives, invites, and audit
- Docker Compose based local stack with PostgreSQL, Redis, ClickHouse, Kafka, Prometheus, and Grafana
- Structured logging, request IDs, `/metrics`, and internal health endpoints
- Autopilot verification and exchange simulation tooling for integration checks

## 9. Deferred or Future Areas
- PIPL compliance work
- Additional real exchange integrations
- Deeper admin operations workflows
- Hardened automated reconciliation
- Bid simulator
- Payments integration
- Low-balance notification productization
- Public API SDKs
