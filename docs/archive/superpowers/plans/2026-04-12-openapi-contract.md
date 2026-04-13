# OpenAPI Contract Automation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Set up swaggo + openapi-typescript so that API types and paths are generated from Go handler annotations, eliminating hand-written frontend types.

**Architecture:** Add swaggo `@` annotations to all handler functions → `swag init` generates OpenAPI spec → `openapi-typescript` generates TypeScript types → frontend imports generated types instead of hand-writing them. Makefile target `api-gen` chains both steps.

**Tech Stack:** swaggo/swag (Go), openapi-typescript (npm), Makefile

---

## File Structure

```
docs/
├── docs.go                # swaggo generated package (auto)
├── swagger.json           # generated OpenAPI spec (auto)
└── swagger.yaml           # generated OpenAPI spec (auto)

internal/handler/
├── campaign.go            # Modify: add swag annotations to 15 handlers
├── billing.go             # Modify: add swag annotations to 3 handlers
├── report.go              # Modify: add swag annotations to 6 handlers
├── admin.go               # Modify: add swag annotations to 14 handlers
├── guardrail.go           # Modify: add swag annotations to 3 handlers
├── export.go              # Modify: add swag annotations to 3 handlers
├── analytics.go           # Modify: add swag annotations to 2 handlers
├── upload.go              # Modify: add swag annotation to 1 handler
└── docs.go                # Modify: add swag annotation to 1 handler

cmd/api/main.go            # Modify: add top-level swag annotations
web/lib/api-types.ts       # Create: auto-generated (DO NOT EDIT)
web/lib/api.ts             # Modify: replace hand-written interfaces with imports
web/lib/admin-api.ts       # Modify: replace hand-written interfaces with imports
web/package.json           # Modify: add openapi-typescript + generate script
Makefile                   # Create: api-gen target
```

---

### Task 1: Install swaggo + Top-Level Annotations

**Files:**
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Install swag CLI**

Run: `go install github.com/swaggo/swag/cmd/swag@latest`

Verify: `swag --version`

- [ ] **Step 2: Add top-level swag annotations to cmd/api/main.go**

Add these comment lines immediately before `func main()`:

```go
// @title DSP Platform API
// @version 1.0
// @description Demand-Side Platform — programmatic advertising API
// @host localhost:8181
// @BasePath /api/v1
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
// @securityDefinitions.apikey AdminAuth
// @in header
// @name X-Admin-Token
func main() {
```

- [ ] **Step 3: Test swag init (will have warnings about unannotated handlers, that's OK)**

Run: `cd /c/Users/Roc/github/dsp && swag init -g cmd/api/main.go -o docs/ --parseDependency --parseInternal 2>&1 | head -20`

Expected: Creates `docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml`

- [ ] **Step 4: Commit**

```bash
git add cmd/api/main.go docs/
git commit -m "feat(api): add swaggo top-level annotations and initial spec generation"
```

---

### Task 2: Annotate Campaign Handlers

**Files:**
- Modify: `internal/handler/campaign.go`

- [ ] **Step 1: Add swag annotations to all handlers in campaign.go**

Add `// @` comment blocks before each handler function. The annotations must go directly above the function signature, after any existing doc comment. Here are the annotations for each handler:

**HandleCreateAdvertiser:**
```go
// HandleCreateAdvertiser godoc
// @Summary Create advertiser
// @Tags advertisers
// @Accept json
// @Produce json
// @Param body body object{company_name=string,contact_email=string,balance_cents=integer} true "Advertiser data"
// @Success 201 {object} object{id=integer,api_key=string}
// @Failure 400 {object} object{error=string}
// @Router /advertisers [post]
```

**HandleGetAdvertiser:**
```go
// HandleGetAdvertiser godoc
// @Summary Get advertiser by ID
// @Tags advertisers
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Advertiser ID"
// @Success 200 {object} campaign.Advertiser
// @Router /advertisers/{id} [get]
```

**HandleCreateCampaign:**
```go
// HandleCreateCampaign godoc
// @Summary Create campaign
// @Tags campaigns
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param body body object{advertiser_id=integer,name=string,billing_model=string,budget_total_cents=integer,budget_daily_cents=integer,bid_cpm_cents=integer} true "Campaign data"
// @Success 201 {object} object{id=integer}
// @Failure 400 {object} object{error=string}
// @Router /campaigns [post]
```

**HandleListCampaigns:**
```go
// HandleListCampaigns godoc
// @Summary List campaigns for advertiser
// @Tags campaigns
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {array} campaign.Campaign
// @Router /campaigns [get]
```

**HandleGetCampaign:**
```go
// HandleGetCampaign godoc
// @Summary Get campaign by ID
// @Tags campaigns
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Success 200 {object} campaign.Campaign
// @Router /campaigns/{id} [get]
```

**HandleUpdateCampaign:**
```go
// HandleUpdateCampaign godoc
// @Summary Update campaign
// @Tags campaigns
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path int true "Campaign ID"
// @Param body body object{name=string,bid_cpm_cents=integer,budget_daily_cents=integer,targeting=object} true "Update fields"
// @Success 200 {object} object{status=string}
// @Router /campaigns/{id} [put]
```

**HandleStartCampaign:**
```go
// HandleStartCampaign godoc
// @Summary Start campaign
// @Tags campaigns
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Success 200 {object} object{status=string}
// @Router /campaigns/{id}/start [post]
```

**HandlePauseCampaign:**
```go
// HandlePauseCampaign godoc
// @Summary Pause campaign
// @Tags campaigns
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Success 200 {object} object{status=string}
// @Router /campaigns/{id}/pause [post]
```

**HandleListCreatives:**
```go
// HandleListCreatives godoc
// @Summary List creatives for campaign
// @Tags creatives
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Success 200 {array} campaign.Creative
// @Router /campaigns/{id}/creatives [get]
```

**HandleCreateCreative:**
```go
// HandleCreateCreative godoc
// @Summary Create creative
// @Tags creatives
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param body body object{campaign_id=integer,name=string,ad_type=string,format=string,size=string,ad_markup=string,destination_url=string} true "Creative data"
// @Success 201 {object} object{id=integer}
// @Router /creatives [post]
```

**HandleUpdateCreative:**
```go
// HandleUpdateCreative godoc
// @Summary Update creative
// @Tags creatives
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path int true "Creative ID"
// @Success 200 {object} object{status=string}
// @Router /creatives/{id} [put]
```

**HandleDeleteCreative:**
```go
// HandleDeleteCreative godoc
// @Summary Delete creative
// @Tags creatives
// @Security ApiKeyAuth
// @Param id path int true "Creative ID"
// @Success 200 {object} object{status=string}
// @Router /creatives/{id} [delete]
```

**HandleAdTypes:**
```go
// HandleAdTypes godoc
// @Summary List available ad types
// @Tags reference
// @Produce json
// @Success 200 {object} object
// @Router /ad-types [get]
```

**HandleBillingModels:**
```go
// HandleBillingModels godoc
// @Summary List billing models
// @Tags reference
// @Produce json
// @Success 200 {object} object
// @Router /billing-models [get]
```

- [ ] **Step 2: Regenerate spec**

Run: `cd /c/Users/Roc/github/dsp && swag init -g cmd/api/main.go -o docs/ --parseDependency --parseInternal`

- [ ] **Step 3: Commit**

```bash
git add internal/handler/campaign.go docs/
git commit -m "feat(api): add swag annotations to campaign handlers"
```

---

### Task 3: Annotate Billing + Report + Analytics + Upload + Export Handlers

**Files:**
- Modify: `internal/handler/billing.go`
- Modify: `internal/handler/report.go`
- Modify: `internal/handler/analytics.go`
- Modify: `internal/handler/upload.go`
- Modify: `internal/handler/export.go`

- [ ] **Step 1: Add annotations to billing.go**

**HandleTopUp:**
```go
// HandleTopUp godoc
// @Summary Top up advertiser balance
// @Tags billing
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param body body object{advertiser_id=integer,amount_cents=integer,description=string} true "Top-up data"
// @Success 200 {object} billing.Transaction
// @Router /billing/topup [post]
```

**HandleTransactions:**
```go
// HandleTransactions godoc
// @Summary List billing transactions
// @Tags billing
// @Security ApiKeyAuth
// @Produce json
// @Param advertiser_id query int false "Advertiser ID"
// @Param limit query int false "Limit" default(50)
// @Param offset query int false "Offset" default(0)
// @Success 200 {array} billing.Transaction
// @Router /billing/transactions [get]
```

**HandleBalance:**
```go
// HandleBalance godoc
// @Summary Get advertiser balance
// @Tags billing
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Advertiser ID"
// @Success 200 {object} object{advertiser_id=integer,balance_cents=integer,billing_type=string}
// @Router /billing/balance/{id} [get]
```

- [ ] **Step 2: Add annotations to report.go**

**HandleCampaignStats:**
```go
// HandleCampaignStats godoc
// @Summary Get campaign stats
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Param from query string false "Start date (YYYY-MM-DD)"
// @Param to query string false "End date (YYYY-MM-DD)"
// @Success 200 {object} reporting.CampaignStats
// @Router /reports/campaign/{id}/stats [get]
```

**HandleHourlyStats:**
```go
// HandleHourlyStats godoc
// @Summary Get hourly stats
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Param date query string false "Date (YYYY-MM-DD)"
// @Success 200 {array} reporting.HourlyStats
// @Router /reports/campaign/{id}/hourly [get]
```

**HandleGeoBreakdown:**
```go
// HandleGeoBreakdown godoc
// @Summary Get geo breakdown
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Success 200 {array} reporting.GeoStats
// @Router /reports/campaign/{id}/geo [get]
```

**HandleBidTransparency:**
```go
// HandleBidTransparency godoc
// @Summary Get bid-level details
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Param limit query int false "Limit" default(100)
// @Param offset query int false "Offset" default(0)
// @Success 200 {array} reporting.BidDetail
// @Router /reports/campaign/{id}/bids [get]
```

**HandleAttribution:**
```go
// HandleAttribution godoc
// @Summary Get attribution report
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Success 200 {object} reporting.AttributionReport
// @Router /reports/campaign/{id}/attribution [get]
```

**HandleOverviewStats:**
```go
// HandleOverviewStats godoc
// @Summary Get today's overview stats
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} object{today_spend_cents=integer,today_impressions=integer,today_clicks=integer,ctr=number,balance_cents=integer}
// @Router /reports/overview [get]
```

- [ ] **Step 3: Add annotations to analytics.go, upload.go, export.go**

**HandleAnalyticsStream (analytics.go):**
```go
// HandleAnalyticsStream godoc
// @Summary Real-time analytics SSE stream
// @Tags analytics
// @Security ApiKeyAuth
// @Produce text/event-stream
// @Success 200 {string} string "SSE stream"
// @Router /analytics/stream [get]
```

**HandleAnalyticsSnapshot (analytics.go):**
```go
// HandleAnalyticsSnapshot godoc
// @Summary Get analytics snapshot
// @Tags analytics
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} object
// @Router /analytics/snapshot [get]
```

**HandleUpload (upload.go):**
```go
// HandleUpload godoc
// @Summary Upload creative image
// @Tags creatives
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Image file"
// @Success 200 {object} object{url=string}
// @Router /upload [post]
```

**HandleExportCampaignCSV (export.go):**
```go
// HandleExportCampaignCSV godoc
// @Summary Export campaign stats as CSV
// @Tags export
// @Security ApiKeyAuth
// @Produce text/csv
// @Param id path int true "Campaign ID"
// @Param from query string false "Start date"
// @Param to query string false "End date"
// @Success 200 {file} file
// @Router /export/campaign/{id}/stats [get]
```

**HandleExportBidsCSV (export.go):**
```go
// HandleExportBidsCSV godoc
// @Summary Export bid details as CSV
// @Tags export
// @Security ApiKeyAuth
// @Produce text/csv
// @Param id path int true "Campaign ID"
// @Success 200 {file} file
// @Router /export/campaign/{id}/bids [get]
```

**HandleMyAuditLog (export.go):**
```go
// HandleMyAuditLog godoc
// @Summary Get my audit log
// @Tags audit
// @Security ApiKeyAuth
// @Produce json
// @Param limit query int false "Limit" default(50)
// @Param offset query int false "Offset" default(0)
// @Success 200 {array} audit.Entry
// @Router /audit-log [get]
```

- [ ] **Step 4: Regenerate spec**

Run: `cd /c/Users/Roc/github/dsp && swag init -g cmd/api/main.go -o docs/ --parseDependency --parseInternal`

- [ ] **Step 5: Commit**

```bash
git add internal/handler/billing.go internal/handler/report.go internal/handler/analytics.go internal/handler/upload.go internal/handler/export.go docs/
git commit -m "feat(api): add swag annotations to billing, report, analytics, export handlers"
```

---

### Task 4: Annotate Admin + Guardrail Handlers

**Files:**
- Modify: `internal/handler/admin.go`
- Modify: `internal/handler/guardrail.go`

- [ ] **Step 1: Add annotations to admin.go**

Note: Admin handlers use `AdminAuth` security and the routes are under `/api/v1/admin/` but since swag BasePath is `/api/v1`, use Router paths like `/admin/...`.

**HandleRegister** (public, no auth):
```go
// HandleRegister godoc
// @Summary Submit registration request
// @Tags registration
// @Accept json
// @Produce json
// @Param body body object{company_name=string,contact_email=string,invite_code=string} true "Registration data"
// @Success 201 {object} object{id=integer,status=string,message=string}
// @Router /register [post]
```

**HandleListRegistrations:**
```go
// HandleListRegistrations godoc
// @Summary List pending registrations
// @Tags admin
// @Security AdminAuth
// @Produce json
// @Success 200 {array} registration.Request
// @Router /admin/registrations [get]
```

**HandleApproveRegistration:**
```go
// HandleApproveRegistration godoc
// @Summary Approve registration
// @Tags admin
// @Security AdminAuth
// @Produce json
// @Param id path int true "Registration ID"
// @Success 200 {object} object{advertiser_id=integer,api_key=string,message=string}
// @Router /admin/registrations/{id}/approve [post]
```

**HandleRejectRegistration:**
```go
// HandleRejectRegistration godoc
// @Summary Reject registration
// @Tags admin
// @Security AdminAuth
// @Accept json
// @Param id path int true "Registration ID"
// @Param body body object{reason=string} true "Rejection reason"
// @Success 200 {object} object{status=string}
// @Router /admin/registrations/{id}/reject [post]
```

**HandleSystemHealth:**
```go
// HandleSystemHealth godoc
// @Summary Get system health
// @Tags admin
// @Security AdminAuth
// @Produce json
// @Success 200 {object} object{status=string,active_campaigns=integer,time=string}
// @Router /admin/health [get]
```

**HandleListCreativesForReview:**
```go
// HandleListCreativesForReview godoc
// @Summary List creatives for review
// @Tags admin
// @Security AdminAuth
// @Produce json
// @Success 200 {array} campaign.Creative
// @Router /admin/creatives [get]
```

**HandleApproveCreative:**
```go
// HandleApproveCreative godoc
// @Summary Approve creative
// @Tags admin
// @Security AdminAuth
// @Param id path int true "Creative ID"
// @Success 200 {object} object{status=string}
// @Router /admin/creatives/{id}/approve [post]
```

**HandleRejectCreative:**
```go
// HandleRejectCreative godoc
// @Summary Reject creative
// @Tags admin
// @Security AdminAuth
// @Accept json
// @Param id path int true "Creative ID"
// @Param body body object{reason=string} true "Rejection reason"
// @Success 200 {object} object{status=string}
// @Router /admin/creatives/{id}/reject [post]
```

**HandleListAdvertisers:**
```go
// HandleListAdvertisers godoc
// @Summary List all advertisers
// @Tags admin
// @Security AdminAuth
// @Produce json
// @Success 200 {array} campaign.Advertiser
// @Router /admin/advertisers [get]
```

**HandleAdminTopUp:**
```go
// HandleAdminTopUp godoc
// @Summary Admin top-up advertiser balance
// @Tags admin
// @Security AdminAuth
// @Accept json
// @Produce json
// @Param body body object{advertiser_id=integer,amount_cents=integer,description=string} true "Top-up data"
// @Success 200 {object} billing.Transaction
// @Router /admin/topup [post]
```

**HandleCreateInviteCode:**
```go
// HandleCreateInviteCode godoc
// @Summary Create invite code
// @Tags admin
// @Security AdminAuth
// @Accept json
// @Produce json
// @Param body body object{max_uses=integer,expires_at=string} true "Invite code config"
// @Success 201 {object} object{code=string}
// @Router /admin/invite-codes [post]
```

**HandleListInviteCodes:**
```go
// HandleListInviteCodes godoc
// @Summary List invite codes
// @Tags admin
// @Security AdminAuth
// @Produce json
// @Success 200 {array} registration.InviteCode
// @Router /admin/invite-codes [get]
```

**HandleAuditLog:**
```go
// HandleAuditLog godoc
// @Summary Get audit log
// @Tags admin
// @Security AdminAuth
// @Produce json
// @Param limit query int false "Limit" default(50)
// @Param offset query int false "Offset" default(0)
// @Success 200 {array} audit.Entry
// @Router /admin/audit-log [get]
```

**HandleActiveCampaigns** (internal route, skip swag annotation — it's on a different mux/port)

- [ ] **Step 2: Add annotations to guardrail.go**

**HandleCircuitBreak:**
```go
// HandleCircuitBreak godoc
// @Summary Trip circuit breaker
// @Tags admin
// @Security AdminAuth
// @Accept json
// @Produce json
// @Param body body object{reason=string} true "Trip reason"
// @Success 200 {object} object{status=string,reason=string}
// @Router /admin/circuit-break [post]
```

**HandleCircuitReset:**
```go
// HandleCircuitReset godoc
// @Summary Reset circuit breaker
// @Tags admin
// @Security AdminAuth
// @Produce json
// @Success 200 {object} object{status=string}
// @Router /admin/circuit-reset [post]
```

**HandleCircuitStatus:**
```go
// HandleCircuitStatus godoc
// @Summary Get circuit breaker status
// @Tags admin
// @Security AdminAuth
// @Produce json
// @Success 200 {object} object{circuit_breaker=string,reason=string,global_spend_today_cents=integer}
// @Router /admin/circuit-status [get]
```

- [ ] **Step 3: Regenerate spec**

Run: `cd /c/Users/Roc/github/dsp && swag init -g cmd/api/main.go -o docs/ --parseDependency --parseInternal`

- [ ] **Step 4: Verify spec has all endpoints**

Run: `cd /c/Users/Roc/github/dsp && cat docs/swagger.json | python -c "import sys,json; d=json.load(sys.stdin); print(len(d['paths']), 'endpoints')"` or `grep '"/' docs/swagger.json | wc -l`

Expected: 30+ paths

- [ ] **Step 5: Commit**

```bash
git add internal/handler/admin.go internal/handler/guardrail.go docs/
git commit -m "feat(api): add swag annotations to admin and guardrail handlers"
```

---

### Task 5: Setup openapi-typescript + Generate TS Types

**Files:**
- Modify: `web/package.json`
- Create: `web/lib/api-types.ts` (auto-generated)

- [ ] **Step 1: Install openapi-typescript**

Run: `cd /c/Users/Roc/github/dsp/web && npm install -D openapi-typescript`

- [ ] **Step 2: Add generate script to package.json**

Add to `scripts` in `web/package.json`:

```json
"generate:api": "openapi-typescript ../docs/swagger.yaml -o lib/api-types.ts"
```

- [ ] **Step 3: Generate types**

Run: `cd /c/Users/Roc/github/dsp/web && npm run generate:api`

Expected: Creates `web/lib/api-types.ts` with all TypeScript types from the OpenAPI spec.

- [ ] **Step 4: Verify generated file**

Run: `head -30 /c/Users/Roc/github/dsp/web/lib/api-types.ts`

Should contain `export interface paths`, `export interface components`, etc.

- [ ] **Step 5: Commit**

```bash
git add web/package.json web/package-lock.json web/lib/api-types.ts
git commit -m "feat(web): add openapi-typescript and generate API types from spec"
```

---

### Task 6: Replace Hand-Written Types in Frontend

**Files:**
- Modify: `web/lib/api.ts`
- Modify: `web/lib/admin-api.ts`

- [ ] **Step 1: Update api.ts**

Replace all hand-written interfaces at the top of `web/lib/api.ts` with imports from the generated types. The exact import syntax depends on what `openapi-typescript` generates, but typically:

```typescript
import type { components } from './api-types';

type Advertiser = components['schemas']['campaign.Advertiser'];
type Campaign = components['schemas']['campaign.Campaign'];
type Creative = components['schemas']['campaign.Creative'];
type CampaignStats = components['schemas']['reporting.CampaignStats'];
// etc.
```

Remove the old `export interface Advertiser { ... }` blocks. Keep the `api` object methods and the `request<T>` helper unchanged — only the types change.

**Important:** The exact schema names in the generated file depend on what swaggo produces. Read `web/lib/api-types.ts` first to find the correct schema key names before writing the imports.

Export the types so existing pages can still import them:

```typescript
export type { Advertiser, Campaign, Creative, CampaignStats };
```

- [ ] **Step 2: Update admin-api.ts**

Same approach — replace hand-written interfaces with imports from generated types:

```typescript
import type { components } from './api-types';

type AdminAdvertiser = components['schemas']['campaign.Advertiser'];
type CircuitStatus = ...; // find in generated types
type AuditEntry = components['schemas']['audit.Entry'];
type InviteCode = components['schemas']['registration.InviteCode'];
// etc.
```

Remove old interface blocks.

- [ ] **Step 3: Verify TypeScript compilation**

Run: `cd /c/Users/Roc/github/dsp/web && npx tsc --noEmit 2>&1 | grep -v "app/page.tsx"`

Expected: No new errors from our changes (the pre-existing app/page.tsx error may remain).

- [ ] **Step 4: Commit**

```bash
git add web/lib/api.ts web/lib/admin-api.ts
git commit -m "feat(web): replace hand-written API types with generated imports"
```

---

### Task 7: Makefile + Smoke Test

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Create Makefile**

```makefile
# Makefile

.PHONY: api-gen build test

# Generate OpenAPI spec from Go annotations, then generate TypeScript types
api-gen:
	swag init -g cmd/api/main.go -o docs/ --parseDependency --parseInternal
	cd web && npx openapi-typescript ../docs/swagger.yaml -o lib/api-types.ts
	@echo "API spec and TypeScript types generated."

# Build all Go binaries
build:
	go build ./cmd/api/
	go build ./cmd/bidder/
	go build ./cmd/autopilot/

# Run all tests
test:
	go test ./... -short -count=1
```

- [ ] **Step 2: Run full pipeline**

Run: `cd /c/Users/Roc/github/dsp && make api-gen && make build`

Expected: Both succeed.

- [ ] **Step 3: Run go vet**

Run: `cd /c/Users/Roc/github/dsp && go vet ./...`

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "feat: add Makefile with api-gen, build, test targets"
```

---

## Task Dependency Graph

```
Task 1 (swag setup) ─── Task 2 (campaign annotations) ───┐
                        Task 3 (billing/report/etc) ──────┤
                        Task 4 (admin/guardrail) ─────────┤
                                                           ├── Task 5 (openapi-typescript) ── Task 6 (replace types) ── Task 7 (Makefile)
```

Tasks 2, 3, 4 depend on Task 1 but are independent of each other. Task 5 depends on 2+3+4. Task 6 depends on 5. Task 7 depends on 6.
