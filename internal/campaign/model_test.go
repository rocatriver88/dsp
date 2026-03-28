package campaign

import "testing"

func TestValidTransitions(t *testing.T) {
	valid := []struct{ from, to Status }{
		{StatusDraft, StatusActive},
		{StatusDraft, StatusDeleted},
		{StatusActive, StatusPaused},
		{StatusActive, StatusCompleted},
		{StatusPaused, StatusActive},
		{StatusPaused, StatusCompleted},
	}
	for _, tt := range valid {
		if err := ValidateTransition(tt.from, tt.to); err != nil {
			t.Errorf("expected %s → %s to be valid, got error: %v", tt.from, tt.to, err)
		}
	}
}

func TestInvalidTransitions(t *testing.T) {
	invalid := []struct{ from, to Status }{
		{StatusDraft, StatusPaused},
		{StatusDraft, StatusCompleted},
		{StatusActive, StatusDraft},
		{StatusActive, StatusDeleted},
		{StatusPaused, StatusDraft},
		{StatusPaused, StatusDeleted},
		{StatusCompleted, StatusActive},
		{StatusCompleted, StatusDraft},
		{StatusDeleted, StatusActive},
	}
	for _, tt := range invalid {
		if err := ValidateTransition(tt.from, tt.to); err == nil {
			t.Errorf("expected %s → %s to be invalid, but got no error", tt.from, tt.to)
		}
	}
}
