package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/redis/go-redis/v9"
)

func TestInjectClickTracker_Normal(t *testing.T) {
	markup := `<a href="https://example.com"><img src="ad.png"/></a>`
	clickURL := "http://localhost:8180/click?campaign_id=1&token=abc"

	result := injectClickTracker(markup, clickURL)

	if !strings.Contains(result, markup) {
		t.Error("original markup should be preserved")
	}
	if !strings.Contains(result, clickURL) {
		t.Error("click URL should be injected")
	}
	if !strings.Contains(result, `width="1"`) {
		t.Error("should contain 1x1 tracking pixel")
	}
}

func TestInjectClickTracker_EmptyMarkup(t *testing.T) {
	result := injectClickTracker("", "http://example.com/click")
	if result != "" {
		t.Errorf("empty markup should return empty, got %q", result)
	}
}

func TestInjectClickTracker_EmptyClickURL(t *testing.T) {
	markup := "<div>ad</div>"
	result := injectClickTracker(markup, "")
	if result != markup {
		t.Errorf("empty click URL should return original markup, got %q", result)
	}
}

// TestHandleClick_RejectsArbitraryDest_NoRedirect is the V5.1 P1-3
// end-to-end regression guard: a fully constructed /click request
// carrying ?dest=https://evil.example MUST NOT produce a 302 or any
// Location header pointing at the attacker URL. Pre-hotfix the
// handler had two redirect branches (dedup + happy path) that
// unconditionally 302'd to the client-supplied dest. V5.1 P1-3
// deleted both branches.
//
// The test uses a minimal Deps with campaignID=0, Producer=nil, and
// a stub Redis. This skips the CPC budget-deduct branch (gated on
// campaignID>0) and the Kafka-send branch (gated on campaignID>0 &&
// d.Producer!=nil), reaching the final `{"status":"clicked"}`
// response without any Kafka round-trip. The integration-level
// variant was attempted in handlers_integration_test.go but hung on
// Kafka producer.Close() waiting for inflight SendClick goroutines
// to drain during the first-connect handshake — see the comment
// there for the full story.
//
// Requires a reachable Redis at localhost:7380 (dsp-test stack). If
// Redis is unavailable, the test skips instead of false-positiving.
func TestHandleClick_RejectsArbitraryDest_NoRedirect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:7380",
		Password: "dsp_test_password",
	})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not reachable (%v) — run scripts/test-env.sh up", err)
	}

	const hmacSecret = "v5-1-p1-3-test-hmac-secret"
	d := &Deps{
		HMACSecret: hmacSecret,
		RDB:        rdb,
		// Producer: nil — handleClick's Kafka-send branch is gated on
		//   campaignID > 0 && d.Producer != nil, so with campaignID=0
		//   (below) the branch is never reached. Leaving this nil also
		//   means there's no producer.Close() to hang on at teardown.
		// Loader: nil — handleClick only calls d.Loader.GetCampaign when
		//   campaignID > 0, same gate.
	}

	// campaignID=0 means the HMAC token is signed over "" as the
	// campaign_id string. Construct the token exactly as the handler
	// will validate it.
	campIDStr := "0"
	reqID := fmt.Sprintf("p1-3-unit-%d", time.Now().UnixNano())
	token := auth.GenerateToken(hmacSecret, campIDStr, reqID)

	target := fmt.Sprintf(
		"/click?campaign_id=%s&request_id=%s&token=%s&dest=%s",
		campIDStr, reqID, token,
		"https%3A%2F%2Fevil.example%2Ffree-money",
	)
	req := httptest.NewRequest("GET", target, nil)
	rec := httptest.NewRecorder()

	d.handleClick(rec, req)

	// Must not be a 302/301 redirect.
	if rec.Code == http.StatusFound || rec.Code == http.StatusMovedPermanently {
		t.Fatalf("V5.1 P1-3 regression: /click returned redirect status %d, Location=%q",
			rec.Code, rec.Header().Get("Location"))
	}
	// Must not emit a Location header on non-redirect responses.
	if loc := rec.Header().Get("Location"); loc != "" {
		if strings.Contains(loc, "evil.example") {
			t.Fatalf("V5.1 P1-3 regression: /click emitted Location %q pointing at attacker dest", loc)
		}
		t.Fatalf("/click emitted unexpected Location %q on non-redirect response", loc)
	}
	// Must reach the final happy-path response.
	if rec.Code != http.StatusOK {
		t.Fatalf("/click status: want 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"clicked"`) {
		t.Errorf("/click body: expected status=clicked, got %s", rec.Body.String())
	}
}

// TestInjectClickTracker_NeverEmitsDestParam is the V5.1 P1-3 static
// regression guard: the function that constructs click URLs in real
// bid responses must NEVER put a `dest` query parameter into the URL
// it injects. Any dest parameter that reached handleClick would be
// client-controlled attack surface because the HMAC token only signs
// (campaign_id, request_id). The click dest branch has been deleted
// from handleClick; this test locks in the invariant that no
// legitimate caller in this package can re-introduce it by accident.
func TestInjectClickTracker_NeverEmitsDestParam(t *testing.T) {
	cases := []struct {
		name     string
		markup   string
		clickURL string
	}{
		{"banner", `<a href="https://example.com"><img src="ad.png"/></a>`, "http://bidder.example/click?campaign_id=7&request_id=r-abc&token=xyz"},
		{"empty markup", "", "http://bidder.example/click?campaign_id=7&request_id=r-abc&token=xyz"},
		{"empty url", `<div>ad</div>`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := injectClickTracker(tc.markup, tc.clickURL)
			if strings.Contains(out, "dest=") {
				t.Fatalf("V5.1 P1-3 regression: injectClickTracker output contains dest=: %q", out)
			}
		})
	}
}
