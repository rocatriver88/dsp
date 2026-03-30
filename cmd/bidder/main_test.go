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
