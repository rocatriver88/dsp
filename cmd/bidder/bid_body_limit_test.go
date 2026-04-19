package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleBid_RejectsOversizedBody verifies direct /bid enforces a 1MB
// body cap via http.MaxBytesReader, matching the exchange path's limit.
// Pre-fix: no limit — a 2MB body decoded fully, risking memory blow-up
// on public endpoints. Post-fix: 413 Request Entity Too Large.
func TestHandleBid_RejectsOversizedBody(t *testing.T) {
	d := &Deps{} // Engine nil — handler short-circuits before Engine.Bid
	// Build a 2MB JSON body: valid OpenRTB skeleton + padding to blow past 1MB.
	padding := strings.Repeat("x", 2<<20) // 2MB of 'x'
	bodyJSON := `{"id":"oversized","imp":[{"id":"1"}],"site":{"id":"s","domain":"` + padding + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/bid", bytes.NewReader([]byte(bodyJSON)))
	w := httptest.NewRecorder()

	d.handleBid(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		// Drain body for clearer failure msg
		body, _ := io.ReadAll(w.Body)
		t.Fatalf("expected 413 for 2MB body, got %d: %s", w.Code, string(body))
	}
}
