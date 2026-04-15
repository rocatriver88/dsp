//go:build e2e
// +build e2e

package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestReports_AllEndpoints seeds a single fixture set (advertiser + campaign +
// creative + 15 bid_log rows spread across event types) and then exercises
// every campaign-scoped report endpoint plus /reports/overview. Each subtest
// asserts a 200 status, a non-empty body, and that the body decodes as JSON.
func TestReports_AllEndpoints(t *testing.T) {
	d := mustDeps(t)
	if d.ReportStore == nil {
		t.Skip("clickhouse not available")
	}

	advID, apiKey := newAdvertiser(t, d)
	campID := newCampaign(t, d, advID)
	crID := newCreative(t, d, campID)

	conn := mustCHConn(t)
	for _, ev := range []string{"bid", "win", "impression", "click", "conversion"} {
		insertBidLog(t, conn, advID, campID, crID, ev, 3)
	}

	cases := []struct {
		name string
		path string
	}{
		{"stats", fmt.Sprintf("/api/v1/reports/campaign/%d/stats", campID)},
		{"hourly", fmt.Sprintf("/api/v1/reports/campaign/%d/hourly", campID)},
		{"geo", fmt.Sprintf("/api/v1/reports/campaign/%d/geo", campID)},
		{"bids", fmt.Sprintf("/api/v1/reports/campaign/%d/bids", campID)},
		{"attribution", fmt.Sprintf("/api/v1/reports/campaign/%d/attribution", campID)},
		{"simulate", fmt.Sprintf("/api/v1/reports/campaign/%d/simulate?bid_cpm_cents=150", campID)},
		{"overview", "/api/v1/reports/overview"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := authedReq(t, "GET", tc.path, nil, apiKey)
			w := execAuthed(t, d, req)
			if w.Code != http.StatusOK {
				t.Fatalf("%s: want 200, got %d body=%s", tc.name, w.Code, w.Body.String())
			}
			if w.Body.Len() == 0 {
				t.Fatalf("%s: empty body", tc.name)
			}
			var discard any
			decodeJSON(t, w, &discard)
		})
	}
}

// TestReports_Export_CSV verifies both CSV export endpoints return a
// text/csv content type and a non-trivial body once fixture impressions
// have been seeded.
func TestReports_Export_CSV(t *testing.T) {
	d := mustDeps(t)
	if d.ReportStore == nil {
		t.Skip("clickhouse not available")
	}

	advID, apiKey := newAdvertiser(t, d)
	campID := newCampaign(t, d, advID)
	crID := newCreative(t, d, campID)

	conn := mustCHConn(t)
	insertBidLog(t, conn, advID, campID, crID, "impression", 5)

	cases := []struct {
		name string
		path string
	}{
		{"stats_csv", fmt.Sprintf("/api/v1/export/campaign/%d/stats", campID)},
		{"bids_csv", fmt.Sprintf("/api/v1/export/campaign/%d/bids", campID)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := authedReq(t, "GET", tc.path, nil, apiKey)
			w := execAuthed(t, d, req)
			if w.Code != http.StatusOK {
				t.Fatalf("%s: want 200, got %d body=%s", tc.name, w.Code, w.Body.String())
			}
			ct := w.Header().Get("Content-Type")
			if !contains(ct, "csv") {
				t.Fatalf("%s: want csv content-type, got %q", tc.name, ct)
			}
			if !contains(w.Body.String(), ",") {
				t.Fatalf("%s: expected csv body with comma separators", tc.name)
			}
		})
	}
}

// TestReports_AllEndpoints_ForbiddenCrossTenant ensures every campaign-scoped
// report endpoint rejects requests from an advertiser who does not own the
// campaign with a 404 (matching HandleBidSimulate / CSV export convention).
// This test is the main RED→GREEN driver for the P2.6b hotfix: it fails with
// 200 on the unfixed tree and passes with 404 after the precheck lands.
func TestReports_AllEndpoints_ForbiddenCrossTenant(t *testing.T) {
	d := mustDeps(t)
	if d.ReportStore == nil {
		t.Skip("clickhouse not available")
	}

	advA, _ := newAdvertiser(t, d)
	campA := newCampaign(t, d, advA)
	_, keyB := newAdvertiser(t, d)

	endpoints := []string{"stats", "hourly", "geo", "bids", "attribution"}
	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			path := fmt.Sprintf("/api/v1/reports/campaign/%d/%s", campA, ep)
			req := authedReq(t, "GET", path, nil, keyB)
			w := execAuthed(t, d, req)
			if w.Code != http.StatusNotFound {
				t.Fatalf("cross-tenant %s: want 404, got %d body=%s",
					ep, w.Code, w.Body.String())
			}
		})
	}
}

// TestAnalytics_Snapshot hits the analytics snapshot endpoint and asserts a
// 200 once an advertiser context is established. The body shape is not
// asserted: when ClickHouse is reachable (mustDeps wired ReportStore) the
// handler always returns JSON, otherwise the test is skipped above.
func TestAnalytics_Snapshot(t *testing.T) {
	d := mustDeps(t)
	if d.ReportStore == nil {
		t.Skip("clickhouse not available")
	}
	_, apiKey := newAdvertiser(t, d)
	req := authedReq(t, "GET", "/api/v1/analytics/snapshot", nil, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot: want 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestAnalytics_Stream_ContentType validates the SSE endpoint returns a
// text/event-stream content type and a 200 status. The handler enters a
// 5s ticker loop that exits on ctx.Done; we attach a cancellable context to
// the request and cancel it after 150ms so the test completes quickly even
// though execAuthed is synchronous.
func TestAnalytics_Stream_ContentType(t *testing.T) {
	d := mustDeps(t)
	if d.ReportStore == nil {
		t.Skip("clickhouse not available")
	}
	_, apiKey := newAdvertiser(t, d)

	ctx, cancel := context.WithCancel(context.Background())
	req := authedReq(t, "GET", "/api/v1/analytics/stream", nil, apiKey).WithContext(ctx)

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- execAuthed(t, d, req)
	}()
	time.AfterFunc(150*time.Millisecond, cancel)

	select {
	case w := <-done:
		if w.Code != http.StatusOK {
			t.Fatalf("stream: want 200, got %d", w.Code)
		}
		if !strings.Contains(w.Header().Get("Content-Type"), "event-stream") {
			t.Fatalf("stream: content-type want event-stream, got %q",
				w.Header().Get("Content-Type"))
		}
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("stream test hung > 3s")
	}
}
