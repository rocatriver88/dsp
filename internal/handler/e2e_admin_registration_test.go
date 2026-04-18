//go:build e2e
// +build e2e

package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
)

// Admin registration handlers exercised here (internal/handler/admin.go):
//
//   POST /api/v1/admin/invite-codes
//     Request:  {max_uses int, expires_at *time.Time}
//     Success:  201 {code string}
//
//   GET  /api/v1/admin/invite-codes
//     Success:  200 []registration.InviteCode
//
//   GET  /api/v1/admin/registrations
//     Success:  200 []registration.Request (pending only)
//
//   POST /api/v1/admin/registrations/{id}/approve
//     Request:  (none)
//     Success:  200 {advertiser_id int64, api_key string, user_email string, temp_password string, message string}
//
// The public Register handler is used as a setup step for the approve
// flow. P2.3 flagged that the handler returns 409 for every Submit
// error (duplicate pending, rate limit, invalid invite, etc.), so the
// approve-flow test tolerates 409s at the submit step as long as a
// pending row can still be found by contact_email.

// TestAdmin_InviteCodes_CreateAndList covers POST + GET for invite codes
// using the bare admin mux (execAdmin) so we exercise handler logic, not
// the AdminAuthMiddleware.
func TestAdmin_InviteCodes_CreateAndList(t *testing.T) {
	d := mustDeps(t)

	// Create
	body := map[string]any{
		"max_uses": 10,
	}
	createReq := adminReq(t, http.MethodPost, "/api/v1/admin/invite-codes", body)
	createW := execAdmin(t, d, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("POST /admin/invite-codes: expected 201, got %d: %s",
			createW.Code, createW.Body.String())
	}
	var created struct {
		Code string `json:"code"`
	}
	decodeJSON(t, createW, &created)
	if created.Code == "" {
		t.Fatalf("POST /admin/invite-codes: expected non-empty code (body=%s)",
			createW.Body.String())
	}

	// List
	listReq := adminReq(t, http.MethodGet, "/api/v1/admin/invite-codes", nil)
	listW := execAdmin(t, d, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("GET /admin/invite-codes: expected 200, got %d: %s",
			listW.Code, listW.Body.String())
	}
	if !contains(listW.Body.String(), created.Code) {
		t.Fatalf("GET /admin/invite-codes: expected list to contain %q, got body=%s",
			created.Code, listW.Body.String())
	}
}

// TestAdmin_Registrations_ApproveFlow creates an invite code via the
// service, submits a registration as the public /register endpoint,
// and then approves it through the admin endpoint. It asserts on the
// final approve response + verifies an advertiser row landed in pg.
func TestAdmin_Registrations_ApproveFlow(t *testing.T) {
	d := mustDeps(t)
	pool := mustPool(t)
	ctx := context.Background()

	// 1. Create invite via the service (bypasses the admin handler).
	code, err := d.RegSvc.CreateInviteCode(ctx, "qa-"+safeName(t.Name()), 10, nil)
	if err != nil {
		t.Fatalf("CreateInviteCode: %v", err)
	}
	if code == "" {
		t.Fatalf("CreateInviteCode returned empty code")
	}

	// 2. Submit registration via public POST /api/v1/register.
	email := fmt.Sprintf("pending-%d-%d@test.local", nowNano(), fixtureSeq.Add(1))
	submitBody := map[string]any{
		"company_name":  "reg-" + safeName(t.Name()),
		"contact_email": email,
		"invite_code":   code,
	}
	submitReq := authedReq(t, http.MethodPost, "/api/v1/register", submitBody, "")
	submitW := execPublic(t, d, submitReq)
	// P2.3 documented that HandleRegister maps every Submit error to 409.
	// Accept 200/201 as the true success path, tolerate 409 with a log.
	switch submitW.Code {
	case http.StatusOK, http.StatusCreated:
		// ok
	case http.StatusConflict:
		t.Logf("submit returned 409 (documented P2.3 catchall bug) body=%s",
			submitW.Body.String())
	default:
		t.Fatalf("POST /register: unexpected status %d: %s",
			submitW.Code, submitW.Body.String())
	}

	// 3. Look up the pending registration id via direct SQL.
	var pendingID int64
	err = pool.QueryRow(ctx,
		`SELECT id FROM registration_requests WHERE contact_email = $1`,
		email,
	).Scan(&pendingID)
	if err != nil {
		t.Fatalf("lookup pending registration by email %q: %v "+
			"(submit status=%d body=%s)",
			email, err, submitW.Code, submitW.Body.String())
	}
	if pendingID == 0 {
		t.Fatalf("lookup returned zero id for email %q", email)
	}

	// 4. Approve via admin handler (nil body).
	approvePath := "/api/v1/admin/registrations/" +
		strconv.FormatInt(pendingID, 10) + "/approve"
	approveReq := adminReq(t, http.MethodPost, approvePath, nil)
	approveW := execAdmin(t, d, approveReq)
	if approveW.Code != http.StatusOK {
		t.Fatalf("POST /admin/registrations/%d/approve: expected 200, got %d: %s",
			pendingID, approveW.Code, approveW.Body.String())
	}
	var approved struct {
		AdvertiserID int64  `json:"advertiser_id"`
		APIKey       string `json:"api_key"`
		UserEmail    string `json:"user_email"`
		TempPassword string `json:"temp_password"`
	}
	decodeJSON(t, approveW, &approved)
	if approved.AdvertiserID == 0 {
		t.Fatalf("approve response missing advertiser_id (body=%s)",
			approveW.Body.String())
	}
	if approved.APIKey == "" {
		t.Fatalf("approve response missing api_key (body=%s)",
			approveW.Body.String())
	}
	if approved.UserEmail != email {
		t.Fatalf("approve response user_email=%q want %q", approved.UserEmail, email)
	}
	if approved.TempPassword == "" {
		t.Fatalf("approve response missing temp_password (body=%s)",
			approveW.Body.String())
	}

	// 5. Verify the advertiser row actually exists for this email.
	var advID int64
	err = pool.QueryRow(ctx,
		`SELECT id FROM advertisers WHERE contact_email = $1`,
		email,
	).Scan(&advID)
	if err != nil {
		t.Fatalf("lookup advertiser by email %q: %v", email, err)
	}
	if advID != approved.AdvertiserID {
		t.Fatalf("advertiser id mismatch: approve said %d, db has %d",
			approved.AdvertiserID, advID)
	}

	// 6. Verify an advertiser-role user row was seeded for this email with a
	// password hash. Plaintext only lives in the approve response.
	var userRole string
	var userAdvID *int64
	var passwordHash string
	err = pool.QueryRow(ctx,
		`SELECT role, advertiser_id, password_hash FROM users WHERE email = $1`,
		email,
	).Scan(&userRole, &userAdvID, &passwordHash)
	if err != nil {
		t.Fatalf("lookup seeded user by email %q: %v", email, err)
	}
	if userRole != "advertiser" {
		t.Fatalf("seeded user role=%q want advertiser", userRole)
	}
	if userAdvID == nil || *userAdvID != advID {
		t.Fatalf("seeded user advertiser_id mismatch: user=%v adv=%d", userAdvID, advID)
	}
	if passwordHash == "" {
		t.Fatalf("seeded user has empty password_hash")
	}
}

// TestAdmin_Registrations_NoToken_401 verifies AdminAuthMiddleware blocks
// requests that lack X-Admin-Token. This is the one test in this file
// that must go through execAdminWithAuth (not execAdmin).
func TestAdmin_Registrations_NoToken_401(t *testing.T) {
	d := mustDeps(t)

	req := adminReq(t, http.MethodGet, "/api/v1/admin/registrations", nil)
	req.Header.Del("X-Admin-Token")

	w := execAdminWithAuth(t, d, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /admin/registrations without token: expected 401, got %d: %s",
			w.Code, w.Body.String())
	}
}
