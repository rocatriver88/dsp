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
//   Exchange (non-standard) → ParseBidRequest → Engine.Bid() → FormatBidResponse → Exchange
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
	ParseBidRequest(raw []byte) (*openrtb2.BidRequest, error)
	// FormatBidResponse converts a standard OpenRTB bid response into exchange-specific format.
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
