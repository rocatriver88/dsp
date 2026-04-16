package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleRejectRegistration_InvalidJSON verifies that sending malformed
// JSON to the reject endpoint returns 400 instead of silently proceeding
// with an empty reason. Deps.RegSvc is intentionally nil because the
// handler must return before reaching the Reject call.
func TestHandleRejectRegistration_InvalidJSON(t *testing.T) {
	d := &Deps{} // RegSvc intentionally nil

	req := httptest.NewRequest(http.MethodPost, "/admin/registrations/1/reject",
		strings.NewReader(`{not valid json`))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	d.HandleRejectRegistration(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON body, got %d: %s",
			w.Code, w.Body.String())
	}
}

// TestHandleRejectRegistration_InvalidID verifies that a non-numeric
// registration id in the path returns 400 instead of silently defaulting
// to zero.
func TestHandleRejectRegistration_InvalidID(t *testing.T) {
	d := &Deps{} // RegSvc intentionally nil

	req := httptest.NewRequest(http.MethodPost, "/admin/registrations/abc/reject",
		strings.NewReader(`{"reason":"test"}`))
	req.SetPathValue("id", "abc")
	w := httptest.NewRecorder()

	d.HandleRejectRegistration(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-numeric id, got %d: %s",
			w.Code, w.Body.String())
	}
}
