package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/heartgryphon/dsp/internal/billing"
)

// HandleTopUp godoc
// @Summary Top up advertiser balance
// @Tags billing
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param body body object{advertiser_id=integer,amount_cents=integer,description=string} true "Top-up data"
// @Success 200 {object} billing.Transaction
// @Router /billing/topup [post]
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

// HandleBalance godoc
// @Summary Get advertiser balance
// @Tags billing
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Advertiser ID"
// @Success 200 {object} object{advertiser_id=integer,balance_cents=integer,billing_type=string}
// @Router /billing/balance/{id} [get]
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
