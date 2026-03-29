package exchange

import (
	"encoding/json"

	"github.com/prebid/openrtb/v20/openrtb2"
)

// CustomAdapter handles exchanges with non-standard OpenRTB protocol variants.
// Each external exchange subclasses this with its own parsing/formatting logic.
//
// Common deviations handled:
//   - Different field names (e.g., "adw" instead of "w" for width)
//   - Extra required fields not in OpenRTB spec
//   - Different price units (yuan vs dollars, CPM vs per-impression)
//   - Custom creative format requirements
type CustomAdapter struct {
	config    ExchangeConfig
	parseFn   func([]byte) (*openrtb2.BidRequest, error)
	formatFn  func(*openrtb2.BidResponse) ([]byte, error)
}

// NewCustomAdapter creates an adapter with custom parse/format functions.
// Use this when an exchange has protocol deviations from standard OpenRTB.
//
// Example:
//
//	adapter := NewCustomAdapter(config,
//	    func(raw []byte) (*openrtb2.BidRequest, error) {
//	        // Parse exchange-specific format, map to standard OpenRTB
//	        var custom ExchangeSpecificRequest
//	        json.Unmarshal(raw, &custom)
//	        return custom.ToOpenRTB(), nil
//	    },
//	    func(resp *openrtb2.BidResponse) ([]byte, error) {
//	        // Convert standard response to exchange-specific format
//	        return json.Marshal(FromOpenRTB(resp))
//	    },
//	)
func NewCustomAdapter(config ExchangeConfig, parseFn func([]byte) (*openrtb2.BidRequest, error), formatFn func(*openrtb2.BidResponse) ([]byte, error)) *CustomAdapter {
	return &CustomAdapter{config: config, parseFn: parseFn, formatFn: formatFn}
}

func (a *CustomAdapter) ID() string       { return a.config.ID }
func (a *CustomAdapter) Name() string     { return a.config.Name }
func (a *CustomAdapter) Endpoint() string { return a.config.Endpoint }
func (a *CustomAdapter) Enabled() bool    { return a.config.Enabled }

func (a *CustomAdapter) ParseBidRequest(raw []byte) (*openrtb2.BidRequest, error) {
	if a.parseFn != nil {
		return a.parseFn(raw)
	}
	// Fallback to standard parsing
	var req openrtb2.BidRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (a *CustomAdapter) FormatBidResponse(resp *openrtb2.BidResponse) ([]byte, error) {
	if a.formatFn != nil {
		return a.formatFn(resp)
	}
	return json.Marshal(resp)
}
