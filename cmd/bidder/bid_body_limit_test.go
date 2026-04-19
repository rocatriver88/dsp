package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heartgryphon/dsp/internal/exchange"
	"github.com/prebid/openrtb/v20/openrtb2"
)

// TestHandleBid_RejectsOversizedBody verifies direct /bid enforces a 1MB
// body cap via http.MaxBytesReader, matching the exchange path's limit.
// Pre-fix: no limit — a 2MB body decoded fully, handler forwarded to
// Engine.Bid which happened not to panic only because the imp had no
// media type. Post-fix: 413 Request Entity Too Large at MaxBytesReader
// boundary, before any Engine access.
//
// Deps{} leaves Engine as a nil *Engine. If MaxBytesReader fails to
// intercept (regression), the handler will reach d.Engine.Bid and
// nil-deref, producing a panic-in-test — which is itself a valid
// regression signal ("413 branch was skipped"). Any future change that
// makes Engine.Bid dereference its receiver before the nil-guard line
// is reached will surface the regression here.
func TestHandleBid_RejectsOversizedBody(t *testing.T) {
	d := &Deps{}
	padding := strings.Repeat("x", 2<<20) // 2MB of 'x'
	bodyJSON := `{"id":"oversized","imp":[{"id":"1"}],"site":{"id":"s","domain":"` + padding + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/bid", bytes.NewReader([]byte(bodyJSON)))
	w := httptest.NewRecorder()

	d.handleBid(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		body, _ := io.ReadAll(w.Body)
		t.Fatalf("expected 413 for 2MB body, got %d: %s", w.Code, string(body))
	}
}

// fakeExchangeAdapter satisfies exchange.Adapter for body-limit tests.
// Both Parse/Format return errors because the test never reaches them
// (the MaxBytesReader error fires first in handleExchangeBid).
type fakeExchangeAdapter struct{}

func (fakeExchangeAdapter) ID() string       { return "test-fake" }
func (fakeExchangeAdapter) Name() string     { return "Test Fake" }
func (fakeExchangeAdapter) Endpoint() string { return "http://test-fake.local/bid" }
func (fakeExchangeAdapter) Enabled() bool    { return true }
func (fakeExchangeAdapter) ParseBidRequest(raw []byte) (*openrtb2.BidRequest, error) {
	return nil, errors.New("test should never reach ParseBidRequest")
}
func (fakeExchangeAdapter) FormatBidResponse(resp *openrtb2.BidResponse) ([]byte, error) {
	return nil, errors.New("test should never reach FormatBidResponse")
}

// TestHandleExchangeBid_RejectsOversizedBody verifies the exchange-bid
// path enforces a 1MB body cap with MaxBytesReader. Pre-fix: io.LimitReader
// silently truncated oversized bodies and the adapter's parse step failed
// with 400, so clients could not distinguish "oversized" from "malformed".
// Post-fix: 413 returned before adapter.ParseBidRequest runs.
//
// The fakeExchangeAdapter's Parse/Format methods return errors — if
// MaxBytesReader fails to intercept (regression), the handler will call
// ParseBidRequest and return 400, which the test explicitly distinguishes
// from the expected 413.
func TestHandleExchangeBid_RejectsOversizedBody(t *testing.T) {
	registry := exchange.NewRegistry()
	if err := registry.Register(fakeExchangeAdapter{}); err != nil {
		t.Fatalf("register fake adapter: %v", err)
	}
	d := &Deps{ExchangeRegistry: registry}

	padding := strings.Repeat("x", 2<<20) // 2MB of 'x'
	bodyJSON := `{"id":"oversized","imp":[{"id":"1"}],"site":{"id":"s","domain":"` + padding + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/bid/test-fake", bytes.NewReader([]byte(bodyJSON)))
	req.SetPathValue("exchange_id", "test-fake")
	w := httptest.NewRecorder()

	d.handleExchangeBid(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		respBody, _ := io.ReadAll(w.Body)
		t.Fatalf("expected 413 for 2MB body, got %d: %s", w.Code, string(respBody))
	}
}

// TestHandleBid_RejectsTrailingJunk verifies direct /bid drains the full
// body through MaxBytesReader before attempting to parse. Pre-fix:
// json.Decoder.Decode stops at the end of the first complete JSON object
// and does NOT read trailing bytes, so a 1KB valid bid followed by 2MB
// of junk passed the cap silently and reached Engine.Bid. Post-fix:
// io.ReadAll forces MaxBytesReader to see the full body → 413.
func TestHandleBid_RejectsTrailingJunk(t *testing.T) {
	d := &Deps{}
	// Small valid bid body (< 1KB) + 2MB of trailing junk after the closing `}`.
	// A one-shot json.Decoder.Decode would stop at the `}` and never read the tail.
	valid := `{"id":"small","imp":[{"id":"1","banner":{"w":300,"h":250}}]}`
	junk := strings.Repeat("x", 2<<20) // 2MB
	body := valid + junk
	req := httptest.NewRequest(http.MethodPost, "/bid", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()

	d.handleBid(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		respBody, _ := io.ReadAll(w.Body)
		t.Fatalf("expected 413 for valid-JSON-plus-junk body, got %d: %s", w.Code, string(respBody))
	}
}
