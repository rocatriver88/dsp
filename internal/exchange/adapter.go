package exchange

import (
	"fmt"
	"log"
	"sync"

	"github.com/prebid/openrtb/v20/openrtb2"
)

// Adapter defines the interface for connecting to an ad exchange.
// Each exchange implements ParseBidRequest/FormatBidResponse to normalize
// its protocol variant into standard OpenRTB used internally by the bidder.
//
//	Exchange (non-standard) → ParseBidRequest → Engine.Bid() → FormatBidResponse → Exchange
//
// # Price-unit contract (F8, issue #29)
//
// ParseBidRequest MUST return a BidRequest whose `imp[i].bidfloor` is
// **per-impression, in bid currency units** — matching the OpenRTB 2.5
// spec. Likewise, FormatBidResponse MUST emit a clearing-price
// (`${AUCTION_PRICE}` substituted into NURL) in the same per-impression
// unit. This is the unit convention the /win handler assumes when it
// caps the URL `price` by the signed `bid_price_cents` at
// cmd/bidder/main.go (see bidder_clearing_price_capped_total metric).
//
// If an exchange quotes price in CPM dollars, the adapter MUST divide
// by 1000 inside ParseBidRequest (and multiply by 1000 in
// FormatBidResponse). Onboarding checklist:
// docs/contracts/exchange-onboarding.md.
type Adapter interface {
	// ID returns a unique identifier for this exchange.
	ID() string
	// Name returns the display name.
	Name() string
	// Endpoint returns the OpenRTB bid endpoint URL.
	Endpoint() string
	// Enabled returns whether this exchange is active.
	Enabled() bool
	// ParseBidRequest normalizes an exchange-specific bid request into standard OpenRTB.
	// Price fields MUST be per-impression in bid currency units (see price-unit contract above).
	ParseBidRequest(raw []byte) (*openrtb2.BidRequest, error)
	// FormatBidResponse converts a standard OpenRTB bid response into exchange-specific format.
	// Price fields MUST be per-impression in bid currency units (see price-unit contract above).
	FormatBidResponse(resp *openrtb2.BidResponse) ([]byte, error)
}

// ExchangeConfig holds configuration for an exchange connection.
type ExchangeConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	Enabled  bool   `json:"enabled"`
	// Transparency level: "full" (self-owned) or "limited" (external)
	TransparencyLevel string `json:"transparency_level"`
}

// Registry manages all connected exchanges.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
	}
}

// Register adds an exchange adapter to the registry.
func (r *Registry) Register(adapter Adapter) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.adapters[adapter.ID()]; exists {
		return fmt.Errorf("exchange %s already registered", adapter.ID())
	}

	r.adapters[adapter.ID()] = adapter
	log.Printf("[EXCHANGE] Registered: %s (%s)", adapter.Name(), adapter.Endpoint())
	return nil
}

// Get returns an exchange adapter by ID.
func (r *Registry) Get(id string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[id]
	return a, ok
}

// ListEnabled returns all enabled exchange adapters.
func (r *Registry) ListEnabled() []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Adapter
	for _, a := range r.adapters {
		if a.Enabled() {
			result = append(result, a)
		}
	}
	return result
}

// ListAll returns all registered exchange configs.
func (r *Registry) ListAll() []ExchangeConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []ExchangeConfig
	for _, a := range r.adapters {
		result = append(result, ExchangeConfig{
			ID:       a.ID(),
			Name:     a.Name(),
			Endpoint: a.Endpoint(),
			Enabled:  a.Enabled(),
		})
	}
	return result
}
