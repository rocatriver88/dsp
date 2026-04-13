# Fix 7 Important Issues Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 7 Important-level issues found during code review: admin auth validation, reconciliation scope, guardrail API clarity, pagination, export limits, audit metrics, and advertiser stats.

**Architecture:** Each fix is independent and touches 1-3 files. No new packages. Follows existing patterns.

**Tech Stack:** Go 1.26, Next.js 16 (TypeScript), Prometheus, PostgreSQL

---

## File Structure

**Modifications only (no new files):**

```
internal/guardrail/guardrail.go        # I3: split CheckBid → PreCheck + CheckBidCeiling
internal/guardrail/guardrail_test.go    # I3: update tests
internal/bidder/engine.go              # I3: use new method names
internal/audit/audit.go                # I6: add Prometheus counter
internal/campaign/store.go             # I2: add ListCampaignsActiveOnDate; I4: add pagination to ListAllAdvertisers; I7: JOIN with campaign stats
internal/campaign/model.go             # I7: add ActiveCampaigns + TotalSpentCents fields to Advertiser
internal/reconciliation/reconciliation.go  # I2: use ListCampaignsActiveOnDate
internal/handler/admin.go              # I4: add limit/offset to list handlers
internal/handler/export.go             # I5: configurable limit
web/app/admin/layout.tsx               # I1: validate token on login
web/app/admin/page.tsx                 # I7: use real advertiser stats
```

---

### Task 1: I3 — Split Guardrail CheckBid into PreCheck + CheckBidCeiling

**Files:**
- Modify: `internal/guardrail/guardrail.go`
- Modify: `internal/guardrail/guardrail_test.go`
- Modify: `internal/bidder/engine.go`

- [ ] **Step 1: Update guardrail.go — replace CheckBid with two methods**

Replace the `CheckBid` method with:

```go
// PreCheck validates circuit breaker and global budget. Call once per bid request.
func (g *Guardrail) PreCheck(ctx context.Context) CheckResult {
	if !g.CB.IsOpen(ctx) {
		return CheckResult{Allowed: false, Reason: "circuit_breaker_tripped"}
	}
	if g.config.GlobalDailyBudgetCents > 0 {
		result := g.CheckGlobalBudget(ctx)
		if !result.Allowed {
			return result
		}
	}
	return CheckResult{Allowed: true}
}

// CheckBidCeiling validates a specific bid against the max CPM ceiling. Call per candidate.
func (g *Guardrail) CheckBidCeiling(ctx context.Context, bidCPMCents int) CheckResult {
	if g.config.MaxBidCPMCents > 0 && bidCPMCents > g.config.MaxBidCPMCents {
		return CheckResult{Allowed: false, Reason: "bid_ceiling_exceeded"}
	}
	return CheckResult{Allowed: true}
}
```

- [ ] **Step 2: Update guardrail_test.go — rename test functions**

Change `TestBidCeiling_Blocks/Allows/ZeroMeansNoLimit` to use `CheckBidCeiling`:
```go
result := g.CheckBidCeiling(context.Background(), 6000)
```

Change `TestCircuitBreakerCheck` to use `PreCheck`:
```go
result := g.PreCheck(ctx)
```

Change `TestGlobalDailyBudget_Blocks/Allows` to use `PreCheck`:
```go
result := g.PreCheck(ctx)
```

- [ ] **Step 3: Update engine.go — use new method names**

Line 107: change `e.guardrail.CheckBid(ctx, 0)` to `e.guardrail.PreCheck(ctx)`:
```go
	if e.guardrail != nil {
		preCheck := e.guardrail.PreCheck(ctx)
		if !preCheck.Allowed {
			return nil, nil
		}
	}
```

Line 153-154: change `e.guardrail.CheckBid(ctx, bidCPM)` to `e.guardrail.CheckBidCeiling(ctx, bidCPM)`:
```go
		if e.guardrail != nil {
			capCheck := e.guardrail.CheckBidCeiling(ctx, bidCPM)
			if !capCheck.Allowed {
				continue
			}
		}
```

- [ ] **Step 4: Run tests**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/guardrail/ ./internal/bidder/ -short -v -count=1 2>&1 | tail -10`

- [ ] **Step 5: Commit**

```bash
git add internal/guardrail/ internal/bidder/engine.go
git commit -m "fix(guardrail): split CheckBid into PreCheck + CheckBidCeiling (I3)"
```

---

### Task 2: I2 — Reconciliation Scope: Include Paused Campaigns

**Files:**
- Modify: `internal/campaign/store.go`
- Modify: `internal/reconciliation/reconciliation.go`

- [ ] **Step 1: Add ListCampaignsActiveOnDate to store.go**

Append to `internal/campaign/store.go`:

```go
// ListCampaignsActiveOnDate returns campaigns that were active at any point during the given date.
// Includes currently-paused campaigns that were active earlier in the day.
func (s *Store) ListCampaignsActiveOnDate(ctx context.Context, date time.Time) ([]*Campaign, error) {
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	dayEnd := dayStart.Add(24 * time.Hour)
	rows, err := s.db.Query(ctx,
		`SELECT id, advertiser_id, name, status, billing_model,
		        budget_total_cents, budget_daily_cents, spent_cents,
		        bid_cpm_cents, bid_cpc_cents, ocpm_target_cpa_cents,
		        start_date, end_date, targeting, sandbox,
		        pause_reason, paused_at, created_at, updated_at
		 FROM campaigns
		 WHERE status IN ('active', 'paused')
		   AND created_at < $2
		   AND (updated_at >= $1 OR status = 'active')
		 ORDER BY id`,
		dayStart, dayEnd,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var campaigns []*Campaign
	for rows.Next() {
		c := &Campaign{}
		if err := rows.Scan(&c.ID, &c.AdvertiserID, &c.Name, &c.Status, &c.BillingModel,
			&c.BudgetTotalCents, &c.BudgetDailyCents, &c.SpentCents,
			&c.BidCPMCents, &c.BidCPCCents, &c.OCPMTargetCPACents,
			&c.StartDate, &c.EndDate, &c.Targeting, &c.Sandbox,
			&c.PauseReason, &c.PausedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, nil
}
```

Add `"time"` to imports in store.go if not present.

- [ ] **Step 2: Update reconciliation.go to use new method**

In `RunHourly`, replace `s.store.ListActiveCampaigns(ctx)` with:
```go
	campaigns, err := s.store.ListCampaignsActiveOnDate(ctx, time.Now())
```

In `RunDaily`, replace `s.store.ListActiveCampaigns(ctx)` with:
```go
	campaigns, err := s.store.ListCampaignsActiveOnDate(ctx, date)
```

- [ ] **Step 3: Verify compilation**

Run: `cd /c/Users/Roc/github/dsp && go vet ./internal/reconciliation/ ./internal/campaign/`

- [ ] **Step 4: Commit**

```bash
git add internal/campaign/store.go internal/reconciliation/reconciliation.go
git commit -m "fix(reconciliation): include paused campaigns in daily reconciliation (I2)"
```

---

### Task 3: I4 — Add Pagination to Admin List Endpoints

**Files:**
- Modify: `internal/handler/admin.go`
- Modify: `internal/campaign/store.go`

- [ ] **Step 1: Update ListAllAdvertisers to accept limit/offset**

In `internal/campaign/store.go`, change `ListAllAdvertisers` signature and query:

```go
func (s *Store) ListAllAdvertisers(ctx context.Context, limit, offset int) ([]*Advertiser, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(ctx,
		`SELECT id, company_name, contact_email, api_key, balance_cents,
		        billing_type, created_at, updated_at
		 FROM advertisers ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
```

- [ ] **Step 2: Update admin handlers to parse limit/offset**

In `internal/handler/admin.go`, update `HandleListAdvertisers`:

```go
func (d *Deps) HandleListAdvertisers(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)
	advs, err := d.Store.ListAllAdvertisers(r.Context(), limit, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list advertisers")
		return
	}
	WriteJSON(w, http.StatusOK, advs)
}
```

Add a shared `parsePagination` helper at the top of admin.go (or in handler.go):

```go
func parsePagination(r *http.Request) (limit, offset int) {
	limit = 100
	offset = 0
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}
	if limit > 500 {
		limit = 500
	}
	return
}
```

Update `HandleListInviteCodes` similarly — pass limit/offset to `RegSvc.ListInviteCodes`. Update the `ListInviteCodes` method in `internal/registration/invite.go` to accept `limit, offset int` and add `LIMIT $1 OFFSET $2` to the query.

Update `HandleListCreativesForReview` — pass limit/offset to `Store.ListCreativesByStatus`. Update the method to accept `limit, offset int`.

- [ ] **Step 3: Verify compilation**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/api/`

- [ ] **Step 4: Commit**

```bash
git add internal/handler/admin.go internal/campaign/store.go internal/registration/invite.go
git commit -m "fix(admin): add pagination to list endpoints (I4)"
```

---

### Task 4: I5 — Configurable CSV Export Limit

**Files:**
- Modify: `internal/handler/export.go`

- [ ] **Step 1: Update HandleExportBidsCSV**

Replace the hardcoded `10000` with query param parsing:

```go
	// Parse limit from query param, default 10000, max 50000
	exportLimit := 10000
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &exportLimit)
	}
	if exportLimit > 50000 {
		exportLimit = 50000
	}

	bids, err := d.ReportStore.GetBidTransparency(r.Context(), uint64(campaignID), from, to, exportLimit, 0)
```

Add `"fmt"` to imports if not present.

- [ ] **Step 2: Verify**

Run: `cd /c/Users/Roc/github/dsp && go vet ./internal/handler/`

- [ ] **Step 3: Commit**

```bash
git add internal/handler/export.go
git commit -m "fix(export): make CSV bid export limit configurable, max 50000 (I5)"
```

---

### Task 5: I6 — Audit Log Prometheus Counter

**Files:**
- Modify: `internal/audit/audit.go`

- [ ] **Step 1: Add Prometheus counter and increment on failure**

Add import `"github.com/prometheus/client_golang/prometheus"` and register counter:

```go
var auditErrors = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "dsp_audit_errors_total",
	Help: "Total number of failed audit log writes",
})

func init() {
	prometheus.MustRegister(auditErrors)
}
```

In the `Record` method, after the error log line, add:
```go
	if err != nil {
		log.Printf("[AUDIT] Failed to record %s: %v", e.Action, err)
		auditErrors.Inc()
	}
```

- [ ] **Step 2: Run test**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/audit/ -v`

- [ ] **Step 3: Commit**

```bash
git add internal/audit/audit.go
git commit -m "fix(audit): add Prometheus counter for audit write failures (I6)"
```

---

### Task 6: I7 — ListAllAdvertisers with Campaign Stats

**Files:**
- Modify: `internal/campaign/model.go`
- Modify: `internal/campaign/store.go`
- Modify: `web/app/admin/page.tsx`

- [ ] **Step 1: Add fields to Advertiser struct**

In `internal/campaign/model.go`, add two fields to `Advertiser`:

```go
type Advertiser struct {
	ID               int64     `json:"id" db:"id"`
	CompanyName      string    `json:"company_name" db:"company_name"`
	ContactEmail     string    `json:"contact_email" db:"contact_email"`
	APIKey           string    `json:"api_key" db:"api_key"`
	BalanceCents     int64     `json:"balance_cents" db:"balance_cents"`
	BillingType      string    `json:"billing_type" db:"billing_type"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
	ActiveCampaigns  int       `json:"active_campaigns,omitempty"`
	TotalSpentCents  int64     `json:"total_spent_cents,omitempty"`
}
```

- [ ] **Step 2: Update ListAllAdvertisers query with JOIN**

In `internal/campaign/store.go`, update the `ListAllAdvertisers` query (already modified in Task 3 for pagination):

```go
func (s *Store) ListAllAdvertisers(ctx context.Context, limit, offset int) ([]*Advertiser, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(ctx,
		`SELECT a.id, a.company_name, a.contact_email, a.api_key, a.balance_cents,
		        a.billing_type, a.created_at, a.updated_at,
		        COUNT(c.id) FILTER (WHERE c.status = 'active') AS active_campaigns,
		        COALESCE(SUM(c.spent_cents), 0) AS total_spent_cents
		 FROM advertisers a
		 LEFT JOIN campaigns c ON c.advertiser_id = a.id
		 GROUP BY a.id
		 ORDER BY a.created_at DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var advs []*Advertiser
	for rows.Next() {
		a := &Advertiser{}
		if err := rows.Scan(&a.ID, &a.CompanyName, &a.ContactEmail, &a.APIKey,
			&a.BalanceCents, &a.BillingType, &a.CreatedAt, &a.UpdatedAt,
			&a.ActiveCampaigns, &a.TotalSpentCents); err != nil {
			return nil, err
		}
		advs = append(advs, a)
	}
	return advs, nil
}
```

- [ ] **Step 3: Update admin overview page to use real stats**

In `web/app/admin/page.tsx`, wherever the page computes advertiser stats, use the actual `active_campaigns` and `total_spent_cents` fields from the API response instead of hardcoded zeros. The exact changes depend on what the current page code looks like — read it first, then:
- Sum `a.active_campaigns` across all advertisers for the "活跃 Campaign" stat card
- Sum `a.total_spent_cents` for total spent stat (or use `circuit.global_spend_today_cents` for today's spend)

- [ ] **Step 4: Verify**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/api/`

- [ ] **Step 5: Commit**

```bash
git add internal/campaign/model.go internal/campaign/store.go web/app/admin/page.tsx
git commit -m "fix(admin): add campaign stats to advertiser list via JOIN (I7)"
```

---

### Task 7: I1 — Admin Token Server-Side Validation

**Files:**
- Modify: `web/app/admin/layout.tsx`

- [ ] **Step 1: Add token validation on login**

In `web/app/admin/layout.tsx`, in the `AdminTokenGate` component, change the login button handler to validate against the server before storing the token:

```tsx
const handleLogin = async () => {
  if (!input) return;
  setError(null);
  setValidating(true);
  try {
    const res = await fetch(
      `${process.env.NEXT_PUBLIC_ADMIN_API_URL || "http://localhost:8182"}/api/v1/admin/health`,
      { headers: { "X-Admin-Token": input.trim() } }
    );
    if (!res.ok) {
      setError("Token 无效或服务不可用");
      return;
    }
    localStorage.setItem("dsp_admin_token", input.trim());
    setToken(input.trim());
  } catch {
    setError("无法连接到管理服务");
  } finally {
    setValidating(false);
  }
};
```

Add state variables:
```tsx
const [error, setError] = useState<string | null>(null);
const [validating, setValidating] = useState(false);
```

Update the login button to show loading state and error:
```tsx
<button onClick={handleLogin} disabled={!input || validating}
  className="w-full px-4 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700 disabled:bg-gray-300">
  {validating ? "验证中..." : "登录"}
</button>
{error && <p className="text-xs text-red-500 mt-2">{error}</p>}
```

Also update the `onKeyDown` handler on the input to call `handleLogin` instead of directly storing.

- [ ] **Step 2: Verify**

Run: `cd /c/Users/Roc/github/dsp/web && npx tsc --noEmit 2>&1 | grep -v "app/page.tsx"`

- [ ] **Step 3: Commit**

```bash
git add web/app/admin/layout.tsx
git commit -m "fix(web): validate admin token against server before storing (I1)"
```

---

### Task 8: Smoke Test

- [ ] **Step 1: Run all tests**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/guardrail/ ./internal/audit/ ./internal/reconciliation/ ./internal/handler/ ./internal/bidder/ ./cmd/autopilot/ -short -v -count=1 2>&1 | tail -15`

- [ ] **Step 2: Build all binaries**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/api/ && go build ./cmd/bidder/ && go build ./cmd/autopilot/`

- [ ] **Step 3: Go vet**

Run: `cd /c/Users/Roc/github/dsp && go vet ./...`

- [ ] **Step 4: Regenerate OpenAPI spec** (handlers changed)

Run: `cd /c/Users/Roc/github/dsp && swag init -g cmd/api/main.go -o docs/ --parseDependency --parseInternal`

- [ ] **Step 5: Commit**

```bash
git add docs/ && git commit -m "chore: regenerate OpenAPI spec after I1-I7 fixes"
```

---

## Task Dependency Graph

```
Task 1 (I3 guardrail) ──────── independent
Task 2 (I2 reconciliation) ─── independent
Task 3 (I4 pagination) ──────── independent
Task 4 (I5 export limit) ───── independent
Task 5 (I6 audit metrics) ──── independent
Task 6 (I7 advertiser stats) ─ depends on Task 3 (shares ListAllAdvertisers)
Task 7 (I1 admin token) ────── independent
Task 8 (smoke test) ─────────── depends on all
```

All tasks except 6 and 8 are independent and can run in any order. Task 6 must follow Task 3 (both modify ListAllAdvertisers).
