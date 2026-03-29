package exchange

import (
	"encoding/json"

	"github.com/prebid/openrtb/v20/openrtb2"
)

// StandardAdapter handles exchanges that follow OpenRTB 2.x with no deviations.
// Used for the self-owned exchange and any standard-compliant external exchange.
type StandardAdapter struct {
	config ExchangeConfig
}

func NewStandardAdapter(config ExchangeConfig) *StandardAdapter {
	return &StandardAdapter{config: config}
}

func (a *StandardAdapter) ID() string       { return a.config.ID }
func (a *StandardAdapter) Name() string     { return a.config.Name }
func (a *StandardAdapter) Endpoint() string { return a.config.Endpoint }
func (a *StandardAdapter) Enabled() bool    { return a.config.Enabled }

func (a *StandardAdapter) ParseBidRequest(raw []byte) (*openrtb2.BidRequest, error) {
	var req openrtb2.BidRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (a *StandardAdapter) FormatBidResponse(resp *openrtb2.BidResponse) ([]byte, error) {
	return json.Marshal(resp)
}

// DefaultRegistry creates a registry with the self-owned exchange pre-registered.
func DefaultRegistry(selfEndpoint string) *Registry {
	r := NewRegistry()
	r.Register(NewStandardAdapter(ExchangeConfig{
		ID:                "self",
		Name:              "自有 Exchange",
		Endpoint:          selfEndpoint,
		Enabled:           true,
		TransparencyLevel: "full",
	}))
	return r
}
