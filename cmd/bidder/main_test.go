package main

import (
	"strings"
	"testing"
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
