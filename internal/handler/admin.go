package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

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
