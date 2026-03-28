package exchange

import (
	"fmt"
	"log"
	"sync"
)

// Adapter defines the interface for connecting to an ad exchange.
// Both the self-owned exchange and external exchanges implement this.
// From the design doc: "接入外部 exchange 和接入自有 exchange 是完全一样的,
// 都是遵循 OpenRTB 的协议标准"
type Adapter interface {
	// ID returns a unique identifier for this exchange.
	ID() string
	// Name returns the display name.
	Name() string
	// Endpoint returns the OpenRTB bid endpoint URL.
	Endpoint() string
	// Enabled returns whether this exchange is active.
	Enabled() bool
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

// exchange implements the Adapter interface.
type exchange struct {
	config ExchangeConfig
}

func (e *exchange) ID() string       { return e.config.ID }
func (e *exchange) Name() string     { return e.config.Name }
func (e *exchange) Endpoint() string { return e.config.Endpoint }
func (e *exchange) Enabled() bool    { return e.config.Enabled }

// Registry manages all connected exchanges.
type Registry struct {
	mu        sync.RWMutex
	exchanges map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{
		exchanges: make(map[string]Adapter),
	}
}

// Register adds an exchange to the registry.
func (r *Registry) Register(config ExchangeConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.exchanges[config.ID]; exists {
		return fmt.Errorf("exchange %s already registered", config.ID)
	}

	r.exchanges[config.ID] = &exchange{config: config}
	log.Printf("[EXCHANGE] Registered: %s (%s) transparency=%s",
		config.Name, config.Endpoint, config.TransparencyLevel)
	return nil
}

// Get returns an exchange by ID.
func (r *Registry) Get(id string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.exchanges[id]
	return e, ok
}

// ListEnabled returns all enabled exchanges.
func (r *Registry) ListEnabled() []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Adapter
	for _, e := range r.exchanges {
		if e.Enabled() {
			result = append(result, e)
		}
	}
	return result
}

// ListAll returns all registered exchanges.
func (r *Registry) ListAll() []ExchangeConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []ExchangeConfig
	for _, e := range r.exchanges {
		ex := e.(*exchange)
		result = append(result, ex.config)
	}
	return result
}

// DefaultRegistry creates a registry with the self-owned exchange pre-registered.
func DefaultRegistry(selfEndpoint string) *Registry {
	r := NewRegistry()
	r.Register(ExchangeConfig{
		ID:                "self",
		Name:              "自有 Exchange",
		Endpoint:          selfEndpoint,
		Enabled:           true,
		TransparencyLevel: "full",
	})
	return r
}
