package alert

import (
	"strings"
	"testing"
)

func TestEmailBuildMessage(t *testing.T) {
	e := NewEmail("smtp.example.com", "587", "alerts@dsp.io", "ops@dsp.io")
	msg := string(e.BuildMessage("spend spike", "Campaign 7 at 3x normal rate"))

	// Check required headers
	if !strings.Contains(msg, "From: alerts@dsp.io\r\n") {
		t.Error("missing or incorrect From header")
	}
	if !strings.Contains(msg, "To: ops@dsp.io\r\n") {
		t.Error("missing or incorrect To header")
	}
	if !strings.Contains(msg, "Subject: [DSP Alert] spend spike\r\n") {
		t.Error("missing or incorrect Subject header")
	}
	if !strings.Contains(msg, "Content-Type: text/plain; charset=UTF-8\r\n") {
		t.Error("missing Content-Type header")
	}

	// Check body after blank line separator
	parts := strings.SplitN(msg, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatal("expected header/body separator (blank line)")
	}
	body := parts[1]
	if !strings.Contains(body, "Campaign 7 at 3x normal rate") {
		t.Errorf("body missing content: %q", body)
	}
}

func TestEmailBuildMessage_MultipleRecipients(t *testing.T) {
	e := NewEmail("smtp.example.com", "587", "alerts@dsp.io", "ops@dsp.io, oncall@dsp.io")
	msg := string(e.BuildMessage("test", "body"))
	if !strings.Contains(msg, "To: ops@dsp.io, oncall@dsp.io\r\n") {
		t.Error("To header should contain all recipients")
	}
}

// Compile-time assertion that Email satisfies Sender.
var _ Sender = (*Email)(nil)
