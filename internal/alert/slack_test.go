package alert

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSlackSend(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewSlack(srv.URL)
	err := s.Send("budget exceeded", "Campaign 42 hit $500 daily cap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text, ok := received["text"].(string)
	if !ok {
		t.Fatal("expected 'text' field in Slack payload")
	}
	if text != "*budget exceeded*\nCampaign 42 hit $500 daily cap" {
		t.Errorf("unexpected text payload: %q", text)
	}
}

func TestSlackSend_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := NewSlack(srv.URL)
	err := s.Send("title", "body")
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestSlackSend_ConnectionError(t *testing.T) {
	s := NewSlack("http://127.0.0.1:1") // nothing listens here
	err := s.Send("title", "body")
	if err == nil {
		t.Fatal("expected error on connection failure")
	}
}

// Compile-time assertion that Slack satisfies Sender.
var _ Sender = (*Slack)(nil)
