package exchange

import (
	"encoding/json"
	"testing"

	"github.com/prebid/openrtb/v20/openrtb2"
)

func TestStandardAdapter_ParseBidRequest(t *testing.T) {
	adapter := NewStandardAdapter(ExchangeConfig{ID: "test", Name: "Test", Enabled: true})

	raw := `{"id":"req-1","imp":[{"id":"imp-1","banner":{"w":300,"h":250}}]}`
	req, err := adapter.ParseBidRequest([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if req.ID != "req-1" {
		t.Errorf("expected req-1, got %s", req.ID)
	}
	if len(req.Imp) != 1 {
		t.Fatalf("expected 1 imp, got %d", len(req.Imp))
	}
}

func TestStandardAdapter_FormatBidResponse(t *testing.T) {
	adapter := NewStandardAdapter(ExchangeConfig{ID: "test", Name: "Test", Enabled: true})

	resp := &openrtb2.BidResponse{ID: "resp-1", Cur: "USD"}
	out, err := adapter.FormatBidResponse(resp)
	if err != nil {
		t.Fatalf("format: %v", err)
	}

	var parsed openrtb2.BidResponse
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.ID != "resp-1" {
		t.Errorf("expected resp-1, got %s", parsed.ID)
	}
}

func TestStandardAdapter_InvalidJSON(t *testing.T) {
	adapter := NewStandardAdapter(ExchangeConfig{ID: "test", Name: "Test", Enabled: true})

	_, err := adapter.ParseBidRequest([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestCustomAdapter_CustomParseFn(t *testing.T) {
	// Simulate an exchange that wraps OpenRTB in a custom envelope
	adapter := NewCustomAdapter(
		ExchangeConfig{ID: "custom-1", Name: "Custom Exchange", Enabled: true},
		func(raw []byte) (*openrtb2.BidRequest, error) {
			var envelope struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(raw, &envelope); err != nil {
				return nil, err
			}
			var req openrtb2.BidRequest
			if err := json.Unmarshal(envelope.Data, &req); err != nil {
				return nil, err
			}
			return &req, nil
		},
		nil, // use default format
	)

	raw := `{"data":{"id":"wrapped-req","imp":[{"id":"imp-1","banner":{"w":300,"h":250}}]}}`
	req, err := adapter.ParseBidRequest([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if req.ID != "wrapped-req" {
		t.Errorf("expected wrapped-req, got %s", req.ID)
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	adapter := NewStandardAdapter(ExchangeConfig{ID: "ex-1", Name: "Exchange 1", Enabled: true})

	if err := r.Register(adapter); err != nil {
		t.Fatalf("register: %v", err)
	}

	got, ok := r.Get("ex-1")
	if !ok {
		t.Fatal("expected to find ex-1")
	}
	if got.Name() != "Exchange 1" {
		t.Errorf("expected Exchange 1, got %s", got.Name())
	}

	// Duplicate registration should fail
	if err := r.Register(adapter); err == nil {
		t.Error("expected error on duplicate register")
	}
}

func TestRegistry_ListEnabled(t *testing.T) {
	r := NewRegistry()
	r.Register(NewStandardAdapter(ExchangeConfig{ID: "a", Name: "A", Enabled: true}))
	r.Register(NewStandardAdapter(ExchangeConfig{ID: "b", Name: "B", Enabled: false}))
	r.Register(NewStandardAdapter(ExchangeConfig{ID: "c", Name: "C", Enabled: true}))

	enabled := r.ListEnabled()
	if len(enabled) != 2 {
		t.Errorf("expected 2 enabled, got %d", len(enabled))
	}
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry("http://localhost:8180")

	self, ok := r.Get("self")
	if !ok {
		t.Fatal("expected self exchange")
	}
	if self.Name() != "自有 Exchange" {
		t.Errorf("expected 自有 Exchange, got %s", self.Name())
	}
}
