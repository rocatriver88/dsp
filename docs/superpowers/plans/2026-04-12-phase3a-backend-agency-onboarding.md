# Phase 3A: Agency Onboarding Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add invite-code registration, admin API extensions (list advertisers, global stats, manual top-up), CSV report export, and audit logging to support agency onboarding.

**Architecture:** Extend the existing registration service with invite codes. New `internal/audit/` package for operation logging. New admin API endpoints for advertiser management. CSV export via a new handler. All changes are backend-only — frontend admin dashboard is Plan 3B.

**Tech Stack:** Go 1.26, PostgreSQL (new migration for invite_codes + audit_log tables), existing billing/campaign/registration packages

---

## File Structure

```
migrations/
└── 010_phase3.sql              # invite_codes + audit_log tables

internal/audit/
├── audit.go                    # Audit logger: Record + Query
└── audit_test.go

internal/registration/
└── service.go                  # Modify: add invite code validation

internal/handler/
├── admin.go                    # Modify: add ListAdvertisers, GlobalStats, AdminTopUp
├── export.go                   # New: CSV export handler
└── handler.go                  # Modify: add AuditLog to Deps

internal/campaign/
└── store.go                    # Modify: add ListAllAdvertisers, CountCampaignsByAdvertiser

cmd/api/
└── main.go                     # Modify: register new routes, init audit
```

---

### Task 1: Database Migration

**Files:**
- Create: `migrations/010_phase3.sql`

- [ ] **Step 1: Write migration**

```sql
-- migrations/010_phase3.sql
-- Phase 3: Invite codes + Audit log

-- Invite codes for controlled registration
CREATE TABLE IF NOT EXISTS invite_codes (
    id          BIGSERIAL PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,
    created_by  TEXT NOT NULL DEFAULT 'system',
    max_uses    INT NOT NULL DEFAULT 1,
    used_count  INT NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add invite_code reference to registration requests
ALTER TABLE registration_requests
    ADD COLUMN IF NOT EXISTS invite_code TEXT;

-- Audit log for operation tracking
CREATE TABLE IF NOT EXISTS audit_log (
    id              BIGSERIAL PRIMARY KEY,
    advertiser_id   BIGINT,
    actor           TEXT NOT NULL,
    action          TEXT NOT NULL,
    resource_type   TEXT NOT NULL,
    resource_id     BIGINT,
    details         JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_advertiser ON audit_log(advertiser_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action, created_at DESC);
```

- [ ] **Step 2: Commit**

```bash
git add migrations/010_phase3.sql
git commit -m "feat: add Phase 3 migration (invite_codes + audit_log)"
```

---

### Task 2: Audit Log Package

**Files:**
- Create: `internal/audit/audit.go`
- Create: `internal/audit/audit_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/audit/audit_test.go
package audit

import (
	"testing"
	"time"
)

func TestEntryJSON(t *testing.T) {
	e := Entry{
		AdvertiserID: 1,
		Actor:        "admin",
		Action:       "campaign.update",
		ResourceType: "campaign",
		ResourceID:   42,
		Details:      map[string]any{"field": "budget_daily_cents", "old": 1000, "new": 2000},
		CreatedAt:    time.Now(),
	}

	if e.Actor != "admin" {
		t.Errorf("expected actor 'admin', got '%s'", e.Actor)
	}
	if e.Action != "campaign.update" {
		t.Errorf("expected action 'campaign.update', got '%s'", e.Action)
	}
}

func TestActions(t *testing.T) {
	actions := []string{
		ActionCampaignCreate, ActionCampaignUpdate, ActionCampaignStart,
		ActionCampaignPause, ActionCreativeCreate, ActionCreativeDelete,
		ActionTopUp, ActionRegistrationApprove,
	}
	for _, a := range actions {
		if a == "" {
			t.Error("action constant should not be empty")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/audit/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write implementation**

```go
// internal/audit/audit.go
package audit

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Action constants
const (
	ActionCampaignCreate      = "campaign.create"
	ActionCampaignUpdate      = "campaign.update"
	ActionCampaignStart       = "campaign.start"
	ActionCampaignPause       = "campaign.pause"
	ActionCreativeCreate      = "creative.create"
	ActionCreativeUpdate      = "creative.update"
	ActionCreativeDelete      = "creative.delete"
	ActionCreativeApprove     = "creative.approve"
	ActionCreativeReject      = "creative.reject"
	ActionTopUp               = "billing.topup"
	ActionRegistrationApprove = "registration.approve"
	ActionRegistrationReject  = "registration.reject"
)

// Entry represents a single audit log record.
type Entry struct {
	ID           int64          `json:"id"`
	AdvertiserID int64          `json:"advertiser_id"`
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   int64          `json:"resource_id"`
	Details      map[string]any `json:"details,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// Logger writes audit entries to PostgreSQL.
type Logger struct {
	db *pgxpool.Pool
}

func NewLogger(db *pgxpool.Pool) *Logger {
	return &Logger{db: db}
}

// Record writes an audit entry. Errors are logged but not returned
// to avoid blocking the main operation.
func (l *Logger) Record(ctx context.Context, e Entry) {
	detailsJSON, _ := json.Marshal(e.Details)
	_, err := l.db.Exec(ctx,
		`INSERT INTO audit_log (advertiser_id, actor, action, resource_type, resource_id, details)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		e.AdvertiserID, e.Actor, e.Action, e.ResourceType, e.ResourceID, detailsJSON,
	)
	if err != nil {
		log.Printf("[AUDIT] Failed to record %s: %v", e.Action, err)
	}
}

// Query returns audit entries for an advertiser, newest first.
func (l *Logger) Query(ctx context.Context, advertiserID int64, limit, offset int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := l.db.Query(ctx,
		`SELECT id, advertiser_id, actor, action, resource_type, resource_id, details, created_at
		 FROM audit_log WHERE advertiser_id = $1
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		advertiserID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var detailsJSON []byte
		if err := rows.Scan(&e.ID, &e.AdvertiserID, &e.Actor, &e.Action,
			&e.ResourceType, &e.ResourceID, &detailsJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		if detailsJSON != nil {
			json.Unmarshal(detailsJSON, &e.Details)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// QueryAll returns all audit entries (for admin), newest first.
func (l *Logger) QueryAll(ctx context.Context, limit, offset int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := l.db.Query(ctx,
		`SELECT id, advertiser_id, actor, action, resource_type, resource_id, details, created_at
		 FROM audit_log ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var detailsJSON []byte
		if err := rows.Scan(&e.ID, &e.AdvertiserID, &e.Actor, &e.Action,
			&e.ResourceType, &e.ResourceID, &detailsJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		if detailsJSON != nil {
			json.Unmarshal(detailsJSON, &e.Details)
		}
		entries = append(entries, e)
	}
	return entries, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/audit/ -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/audit/
git commit -m "feat(audit): add audit log package with Record and Query"
```

---

### Task 3: Invite Code Registration

**Files:**
- Modify: `internal/registration/service.go`
- Create: `internal/registration/invite.go`
- Create: `internal/registration/invite_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/registration/invite_test.go
package registration

import (
	"testing"
)

func TestGenerateInviteCode(t *testing.T) {
	code := GenerateInviteCode()
	if len(code) < 8 {
		t.Errorf("invite code too short: %s", code)
	}
	// Should be unique
	code2 := GenerateInviteCode()
	if code == code2 {
		t.Error("two generated codes should not be equal")
	}
}

func TestInviteCodeFormat(t *testing.T) {
	code := GenerateInviteCode()
	// Should be alphanumeric hex
	for _, c := range code {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("invite code contains invalid char: %c", c)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/registration/ -run TestGenerate -v`
Expected: FAIL — function not defined

- [ ] **Step 3: Write invite.go**

```go
// internal/registration/invite.go
package registration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateInviteCode creates a random 16-character hex invite code.
func GenerateInviteCode() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateInviteCode generates and stores an invite code.
func (s *Service) CreateInviteCode(ctx context.Context, createdBy string, maxUses int, expiresAt *time.Time) (string, error) {
	code := GenerateInviteCode()
	_, err := s.db.Exec(ctx,
		`INSERT INTO invite_codes (code, created_by, max_uses, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		code, createdBy, maxUses, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("create invite code: %w", err)
	}
	return code, nil
}

// ValidateInviteCode checks if an invite code is valid and not expired/exhausted.
func (s *Service) ValidateInviteCode(ctx context.Context, code string) error {
	var maxUses, usedCount int
	var expiresAt *time.Time
	err := s.db.QueryRow(ctx,
		`SELECT max_uses, used_count, expires_at FROM invite_codes WHERE code = $1`,
		code,
	).Scan(&maxUses, &usedCount, &expiresAt)
	if err != nil {
		return fmt.Errorf("invalid invite code")
	}
	if usedCount >= maxUses {
		return fmt.Errorf("invite code has been fully used")
	}
	if expiresAt != nil && time.Now().After(*expiresAt) {
		return fmt.Errorf("invite code has expired")
	}
	return nil
}

// UseInviteCode increments the used_count of an invite code.
func (s *Service) UseInviteCode(ctx context.Context, code string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE invite_codes SET used_count = used_count + 1 WHERE code = $1`,
		code,
	)
	return err
}

// ListInviteCodes returns all invite codes for admin viewing.
func (s *Service) ListInviteCodes(ctx context.Context) ([]InviteCode, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, code, created_by, max_uses, used_count, expires_at, created_at
		 FROM invite_codes ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codes []InviteCode
	for rows.Next() {
		var c InviteCode
		if err := rows.Scan(&c.ID, &c.Code, &c.CreatedBy, &c.MaxUses,
			&c.UsedCount, &c.ExpiresAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		codes = append(codes, c)
	}
	return codes, nil
}

// InviteCode represents a stored invite code.
type InviteCode struct {
	ID        int64      `json:"id"`
	Code      string     `json:"code"`
	CreatedBy string     `json:"created_by"`
	MaxUses   int        `json:"max_uses"`
	UsedCount int        `json:"used_count"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
```

- [ ] **Step 4: Modify Submit to require invite code**

In `internal/registration/service.go`, modify the `Submit` method. Add invite code validation at the beginning, after email validation:

```go
// In Submit(), after validateEmail check:
	// Invite code required
	if req.InviteCode == "" {
		return 0, fmt.Errorf("invite code is required")
	}
	if err := s.ValidateInviteCode(ctx, req.InviteCode); err != nil {
		return 0, err
	}
```

Add `InviteCode` field to `Request` struct:

```go
type Request struct {
	// ... existing fields ...
	InviteCode string `json:"invite_code,omitempty"`
}
```

Update the INSERT to include invite_code:

```go
	err = s.db.QueryRow(ctx,
		`INSERT INTO registration_requests (company_name, contact_email, contact_phone, business_type, website, invite_code)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		req.CompanyName, req.ContactEmail, req.ContactPhone, req.BusinessType, req.Website, req.InviteCode,
	).Scan(&id)
```

After successful insert, use the invite code:

```go
	if err == nil {
		s.UseInviteCode(ctx, req.InviteCode)
	}
```

- [ ] **Step 5: Run tests**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/registration/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/registration/
git commit -m "feat(registration): add invite code system for controlled onboarding"
```

---

### Task 4: Admin API Extensions

**Files:**
- Modify: `internal/campaign/store.go` — add ListAllAdvertisers
- Modify: `internal/handler/admin.go` — add new admin handlers
- Modify: `internal/handler/handler.go` — add AuditLog to Deps
- Modify: `cmd/api/main.go` — register routes, init audit

- [ ] **Step 1: Add ListAllAdvertisers to campaign store**

Append to `internal/campaign/store.go`:

```go
// ListAllAdvertisers returns all advertisers for admin dashboard.
func (s *Store) ListAllAdvertisers(ctx context.Context) ([]*Advertiser, error) {
	rows, err := s.db.Query(ctx,
		`SELECT a.id, a.company_name, a.contact_email, a.api_key, a.balance_cents,
		        a.billing_type, a.created_at, a.updated_at,
		        COUNT(c.id) FILTER (WHERE c.status = 'active') as active_campaigns,
		        COALESCE(SUM(c.spent_cents), 0) as total_spent_cents
		 FROM advertisers a
		 LEFT JOIN campaigns c ON c.advertiser_id = a.id
		 GROUP BY a.id
		 ORDER BY a.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var advs []*Advertiser
	for rows.Next() {
		a := &Advertiser{}
		var activeCampaigns int64
		var totalSpent int64
		if err := rows.Scan(&a.ID, &a.CompanyName, &a.ContactEmail, &a.APIKey,
			&a.BalanceCents, &a.BillingType, &a.CreatedAt, &a.UpdatedAt,
			&activeCampaigns, &totalSpent); err != nil {
			return nil, err
		}
		advs = append(advs, a)
	}
	return advs, nil
}
```

- [ ] **Step 2: Add AuditLog to handler Deps**

In `internal/handler/handler.go`, add:

```go
import "github.com/heartgryphon/dsp/internal/audit"

type Deps struct {
	// ... existing fields ...
	AuditLog  *audit.Logger           // nil if audit disabled
}
```

- [ ] **Step 3: Add admin handlers**

Append to `internal/handler/admin.go`:

```go
// HandleListAdvertisers returns all advertisers for admin dashboard.
func (d *Deps) HandleListAdvertisers(w http.ResponseWriter, r *http.Request) {
	advs, err := d.Store.ListAllAdvertisers(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list advertisers")
		return
	}
	WriteJSON(w, http.StatusOK, advs)
}

// HandleAdminTopUp allows admin to add balance to any advertiser.
func (d *Deps) HandleAdminTopUp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AdvertiserID int64  `json:"advertiser_id"`
		AmountCents  int64  `json:"amount_cents"`
		Description  string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AmountCents <= 0 {
		WriteError(w, http.StatusBadRequest, "amount must be positive")
		return
	}
	if req.Description == "" {
		req.Description = "admin manual top-up"
	}

	tx, err := d.BillingSvc.TopUp(r.Context(), req.AdvertiserID, req.AmountCents, req.Description)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if d.AuditLog != nil {
		d.AuditLog.Record(r.Context(), audit.Entry{
			AdvertiserID: req.AdvertiserID,
			Actor:        "admin",
			Action:       audit.ActionTopUp,
			ResourceType: "advertiser",
			ResourceID:   req.AdvertiserID,
			Details: map[string]any{
				"amount_cents": req.AmountCents,
				"description":  req.Description,
			},
		})
	}

	WriteJSON(w, http.StatusOK, tx)
}

// HandleCreateInviteCode generates a new invite code.
func (d *Deps) HandleCreateInviteCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxUses   int        `json:"max_uses"`
		ExpiresAt *time.Time `json:"expires_at,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}

	code, err := d.RegSvc.CreateInviteCode(r.Context(), "admin", req.MaxUses, req.ExpiresAt)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]string{"code": code})
}

// HandleListInviteCodes returns all invite codes.
func (d *Deps) HandleListInviteCodes(w http.ResponseWriter, r *http.Request) {
	codes, err := d.RegSvc.ListInviteCodes(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, codes)
}

// HandleAuditLog returns audit entries.
func (d *Deps) HandleAuditLog(w http.ResponseWriter, r *http.Request) {
	if d.AuditLog == nil {
		WriteError(w, http.StatusServiceUnavailable, "audit log not available")
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}

	entries, err := d.AuditLog.QueryAll(r.Context(), limit, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, entries)
}
```

Add `"fmt"` and `"time"` to imports in admin.go if not present. Also add `"github.com/heartgryphon/dsp/internal/audit"`.

- [ ] **Step 4: Register routes in cmd/api/main.go**

Add to the adminMux section:

```go
adminMux.HandleFunc("GET /api/v1/admin/advertisers", h.HandleListAdvertisers)
adminMux.HandleFunc("POST /api/v1/admin/topup", h.HandleAdminTopUp)
adminMux.HandleFunc("POST /api/v1/admin/invite-codes", h.HandleCreateInviteCode)
adminMux.HandleFunc("GET /api/v1/admin/invite-codes", h.HandleListInviteCodes)
adminMux.HandleFunc("GET /api/v1/admin/audit-log", h.HandleAuditLog)
```

Initialize audit logger and add to Deps:

```go
import "github.com/heartgryphon/dsp/internal/audit"

auditLogger := audit.NewLogger(db)

h := &handler.Deps{
	// ... existing fields ...
	AuditLog:  auditLogger,
}
```

- [ ] **Step 5: Build and verify**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/api/`

- [ ] **Step 6: Commit**

```bash
git add internal/campaign/store.go internal/handler/admin.go internal/handler/handler.go cmd/api/main.go
git commit -m "feat(admin): add advertiser list, manual top-up, invite codes, audit log endpoints"
```

---

### Task 5: CSV Report Export

**Files:**
- Create: `internal/handler/export.go`
- Modify: `cmd/api/main.go` — register route

- [ ] **Step 1: Write export handler**

```go
// internal/handler/export.go
package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"

	"github.com/heartgryphon/dsp/internal/auth"
)

// HandleExportCampaignCSV exports campaign stats as CSV.
func (d *Deps) HandleExportCampaignCSV(w http.ResponseWriter, r *http.Request) {
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "reports not available")
		return
	}

	campaignID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}

	// Verify ownership
	advID := auth.AdvertiserIDFromContext(r.Context())
	if _, err := d.Store.GetCampaignForAdvertiser(r.Context(), campaignID, advID); err != nil {
		WriteError(w, http.StatusNotFound, "campaign not found")
		return
	}

	from, to := ParseDateRange(r)
	stats, err := d.ReportStore.GetCampaignStats(r.Context(), uint64(campaignID), from, to)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to fetch stats")
		return
	}

	// Set CSV headers
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="campaign_%d_%s_%s.csv"`,
			campaignID, from.Format("20060102"), to.Format("20060102")))

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header row
	writer.Write([]string{
		"Campaign ID", "Period Start", "Period End",
		"Impressions", "Clicks", "Conversions", "Wins", "Bids",
		"Spend (cents)", "ADX Cost (cents)", "CTR (%)", "Win Rate (%)", "CVR (%)", "CPA (cents)",
	})

	// Data row
	writer.Write([]string{
		fmt.Sprintf("%d", campaignID),
		from.Format("2006-01-02"),
		to.Format("2006-01-02"),
		fmt.Sprintf("%d", stats.Impressions),
		fmt.Sprintf("%d", stats.Clicks),
		fmt.Sprintf("%d", stats.Conversions),
		fmt.Sprintf("%d", stats.Wins),
		fmt.Sprintf("%d", stats.Bids),
		fmt.Sprintf("%d", stats.SpendCents),
		fmt.Sprintf("%d", stats.ADXCostCents),
		fmt.Sprintf("%.2f", stats.CTR),
		fmt.Sprintf("%.2f", stats.WinRate),
		fmt.Sprintf("%.2f", stats.CVR),
		fmt.Sprintf("%d", stats.CPA),
	})
}

// HandleExportBidsCSV exports bid-level detail as CSV.
func (d *Deps) HandleExportBidsCSV(w http.ResponseWriter, r *http.Request) {
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "reports not available")
		return
	}

	campaignID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}

	advID := auth.AdvertiserIDFromContext(r.Context())
	if _, err := d.Store.GetCampaignForAdvertiser(r.Context(), campaignID, advID); err != nil {
		WriteError(w, http.StatusNotFound, "campaign not found")
		return
	}

	from, to := ParseDateRange(r)
	bids, err := d.ReportStore.GetBidTransparency(r.Context(), uint64(campaignID), from, to, 10000, 0)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to fetch bids")
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="bids_%d_%s_%s.csv"`,
			campaignID, from.Format("20060102"), to.Format("20060102")))

	writer := csv.NewWriter(w)
	defer writer.Flush()

	writer.Write([]string{
		"Time", "Event Type", "Geo", "Device OS",
		"Bid Price (cents)", "Clear Price (cents)", "Charge (cents)",
	})

	for _, b := range bids {
		writer.Write([]string{
			b.EventTime.Format("2006-01-02 15:04:05"),
			b.EventType,
			b.GeoCountry,
			b.DeviceOS,
			fmt.Sprintf("%d", b.BidPriceCents),
			fmt.Sprintf("%d", b.ClearPriceCents),
			fmt.Sprintf("%d", b.ChargeCents),
		})
	}
}
```

- [ ] **Step 2: Register routes in cmd/api/main.go**

Add to the public API routes (requires advertiser auth):

```go
publicMux.HandleFunc("GET /api/v1/export/campaign/{id}/stats", h.HandleExportCampaignCSV)
publicMux.HandleFunc("GET /api/v1/export/campaign/{id}/bids", h.HandleExportBidsCSV)
```

- [ ] **Step 3: Build and verify**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/api/`

- [ ] **Step 4: Commit**

```bash
git add internal/handler/export.go cmd/api/main.go
git commit -m "feat(export): add CSV export for campaign stats and bid details"
```

---

### Task 6: Advertiser Audit Log Endpoint

**Files:**
- Modify: `cmd/api/main.go` — add advertiser-facing audit endpoint

- [ ] **Step 1: Add advertiser audit handler**

Add to `internal/handler/export.go` (or a new file):

```go
// HandleMyAuditLog returns audit entries for the authenticated advertiser.
func (d *Deps) HandleMyAuditLog(w http.ResponseWriter, r *http.Request) {
	if d.AuditLog == nil {
		WriteError(w, http.StatusServiceUnavailable, "audit log not available")
		return
	}

	advID := auth.AdvertiserIDFromContext(r.Context())
	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}

	entries, err := d.AuditLog.Query(r.Context(), advID, limit, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, entries)
}
```

- [ ] **Step 2: Register route**

Add to public API routes in `cmd/api/main.go`:

```go
publicMux.HandleFunc("GET /api/v1/audit-log", h.HandleMyAuditLog)
```

- [ ] **Step 3: Build**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/api/`

- [ ] **Step 4: Commit**

```bash
git add internal/handler/export.go cmd/api/main.go
git commit -m "feat(api): add advertiser-facing audit log endpoint"
```

---

### Task 7: Integration Smoke Test

**Files:**
- None new

- [ ] **Step 1: Run all tests**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/audit/ ./internal/registration/ ./internal/handler/ ./cmd/autopilot/ -v -count=1`
Expected: All pass

- [ ] **Step 2: Build all binaries**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/api/ && go build ./cmd/bidder/ && go build ./cmd/autopilot/`

- [ ] **Step 3: Run go vet**

Run: `cd /c/Users/Roc/github/dsp && go vet ./...`

- [ ] **Step 4: Commit if needed**

```bash
git add -A && git commit -m "chore: Phase 3A integration smoke test pass"
```

---

## Out of Scope (Phase 3B — Frontend)

- Admin dashboard UI pages (`/admin/*`)
- Advertiser-facing audit log UI
- CSV export UI buttons
- Invite code management UI

## Task Dependency Graph

```
Task 1 (migration) ───────────────────────────┐
Task 2 (audit log) ───────────────────────────┤
Task 3 (invite codes) ────────────────────────┤
                                               ├── Task 4 (admin API) ── Task 7 (smoke)
Task 5 (CSV export) ──────────────────────────┤
Task 6 (advertiser audit endpoint) ── Task 2 ─┘
```

Tasks 1, 2, 3, 5 can be parallelized. Task 4 depends on 1+2+3. Task 6 depends on 2. Task 7 depends on all.
