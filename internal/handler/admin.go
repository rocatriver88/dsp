package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/heartgryphon/dsp/internal/audit"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/registration"
)

func (d *Deps) HandleActiveCampaigns(w http.ResponseWriter, r *http.Request) {
	campaigns, err := d.Store.ListActiveCampaigns(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if campaigns == nil {
		campaigns = []*campaign.Campaign{}
	}
	WriteJSON(w, http.StatusOK, campaigns)
}

func (d *Deps) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req registration.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.CompanyName == "" || req.ContactEmail == "" {
		WriteError(w, http.StatusBadRequest, "company_name and contact_email required")
		return
	}
	id, err := d.RegSvc.Submit(r.Context(), &req)
	if err != nil {
		WriteError(w, http.StatusConflict, err.Error())
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{
		"id":      id,
		"status":  "pending",
		"message": "Registration submitted. We will review within 7 business days.",
	})
}

func (d *Deps) HandleListRegistrations(w http.ResponseWriter, r *http.Request) {
	reqs, err := d.RegSvc.ListPending(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if reqs == nil {
		reqs = []registration.Request{}
	}
	WriteJSON(w, http.StatusOK, reqs)
}

func (d *Deps) HandleApproveRegistration(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	advID, apiKey, err := d.RegSvc.Approve(r.Context(), id, "admin")
	if err != nil {
		WriteError(w, http.StatusConflict, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"advertiser_id": advID,
		"api_key":       apiKey,
		"message":       "Registration approved. Advertiser account created.",
	})
}

// HandleSystemHealth returns system-wide health metrics for admin monitoring.
func (d *Deps) HandleSystemHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	health := map[string]any{
		"status": "ok",
	}

	// Count advertisers
	campaigns, _ := d.Store.ListActiveCampaigns(ctx)
	health["active_campaigns"] = len(campaigns)

	// Count pending registrations
	pending, _ := d.RegSvc.ListPending(ctx)
	health["pending_registrations"] = len(pending)

	// Redis status
	if d.Redis != nil {
		if err := d.Redis.Ping(ctx).Err(); err != nil {
			health["redis"] = "error"
		} else {
			health["redis"] = "ok"
		}
	} else {
		health["redis"] = "unavailable"
	}

	// ClickHouse status
	if d.ReportStore != nil {
		health["clickhouse"] = "ok"
	} else {
		health["clickhouse"] = "unavailable"
	}

	WriteJSON(w, http.StatusOK, health)
}

// HandleListCreativesForReview returns all creatives pending review.
func (d *Deps) HandleListCreativesForReview(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	creatives, err := d.Store.ListCreativesByStatus(r.Context(), status)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if creatives == nil {
		creatives = []*campaign.Creative{}
	}
	WriteJSON(w, http.StatusOK, creatives)
}

// HandleApproveCreative approves a creative for serving.
func (d *Deps) HandleApproveCreative(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := d.Store.UpdateCreativeStatus(r.Context(), id, "approved"); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// HandleRejectCreative rejects a creative with a reason.
func (d *Deps) HandleRejectCreative(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := d.Store.UpdateCreativeStatus(r.Context(), id, "rejected"); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

func (d *Deps) HandleRejectRegistration(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := d.RegSvc.Reject(r.Context(), id, "admin", req.Reason); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

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

// HandleAuditLog returns audit entries for admin.
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
