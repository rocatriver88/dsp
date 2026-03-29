package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/heartgryphon/dsp/internal/billing"
)

func (d *Deps) HandleTopUp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AdvertiserID int64  `json:"advertiser_id"`
		AmountCents  int64  `json:"amount_cents"`
		Description  string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.AmountCents <= 0 {
		WriteError(w, http.StatusBadRequest, "amount must be positive")
		return
	}
	txn, err := d.BillingSvc.TopUp(r.Context(), req.AdvertiserID, req.AmountCents, req.Description)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, txn)
}

func (d *Deps) HandleTransactions(w http.ResponseWriter, r *http.Request) {
	advID, _ := strconv.ParseInt(r.URL.Query().Get("advertiser_id"), 10, 64)
	if advID == 0 {
		WriteError(w, http.StatusBadRequest, "advertiser_id required")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	txns, err := d.BillingSvc.GetTransactions(r.Context(), advID, limit, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if txns == nil {
		txns = []billing.Transaction{}
	}
	WriteJSON(w, http.StatusOK, txns)
}

func (d *Deps) HandleBalance(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	balance, billingType, err := d.BillingSvc.GetBalance(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "advertiser not found")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"advertiser_id": id,
		"balance_cents": balance,
		"billing_type":  billingType,
	})
}
