package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/billing"
)

// HandleTopUp godoc
// @Summary Top up the authenticated advertiser's balance
// @Tags billing
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param body body object{amount_cents=integer,description=string} true "Top-up data"
// @Success 200 {object} billing.Transaction
// @Failure 400 {object} object{error=string}
// @Router /billing/topup [post]
//
// advertiser_id is deliberately not part of the accepted request. If the body
// includes it and it does not match the authenticated advertiser, we refuse
// the request with 400 rather than silently routing the charge — a billing
// mismatch is a client bug that must surface, not a data-shift vulnerability
// to paper over.
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
	authID := auth.AdvertiserIDFromContext(r.Context())
	if authID == 0 {
		WriteError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if req.AdvertiserID != 0 && req.AdvertiserID != authID {
		WriteError(w, http.StatusBadRequest, "advertiser_id mismatch")
		return
	}
	txn, err := d.BillingSvc.TopUp(r.Context(), authID, req.AmountCents, req.Description)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, txn)
}

// HandleTransactions godoc
// @Summary List billing transactions for the authenticated advertiser
// @Tags billing
// @Security ApiKeyAuth
// @Produce json
// @Param limit query int false "Limit" default(50)
// @Param offset query int false "Offset" default(0)
// @Success 200 {array} billing.Transaction
// @Router /billing/transactions [get]
//
// The advertiser is always taken from the auth context. Any advertiser_id
// query parameter a caller sends is ignored — this silently routes the
// caller to their own transactions, which is the safe read-path behavior.
func (d *Deps) HandleTransactions(w http.ResponseWriter, r *http.Request) {
	authID := auth.AdvertiserIDFromContext(r.Context())
	if authID == 0 {
		WriteError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	txns, err := d.BillingSvc.GetTransactions(r.Context(), authID, limit, offset)
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
// @Summary Get the authenticated advertiser's balance
// @Tags billing
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} object{advertiser_id=integer,balance_cents=integer,billing_type=string}
// @Failure 401 {object} object{error=string}
// @Router /billing/balance [get]
//
// The advertiser is resolved from the auth context, not from a path
// parameter. This removes the tenant-isolation surface entirely — there is
// no path id to compare against, so cross-tenant access is structurally
// impossible.
func (d *Deps) HandleBalance(w http.ResponseWriter, r *http.Request) {
	authID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	balance, billingType, err := d.BillingSvc.GetBalance(r.Context(), authID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"advertiser_id": authID,
		"balance_cents": balance,
		"billing_type":  billingType,
	})
}
