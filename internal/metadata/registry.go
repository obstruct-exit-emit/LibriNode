package metadata

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Settings holds one provider's configuration. Every current provider needs
// at most a token; provider-specific fields (base URL, language, ...) get
// added here as new sources land, so the config file and settings API don't
// change shape per provider.
type Settings struct {
	Token string `yaml:"token" json:"token"`
}

// Factory builds a provider from its settings. Returning ErrNotConfigured
// means "valid but disabled" (e.g. no token yet) — not a hard error.
type Factory func(Settings) (Provider, error)

// Validator is optionally implemented by providers that can cheaply verify
// their credentials against the live API (used by the settings Test button).
type Validator interface {
	Validate(ctx context.Context) error
}

var (
	regMu     sync.RWMutex
	factories = map[string]Factory{}
)

// Register makes a provider available under name. Registering the same name
// again replaces the factory (convenient for tests).
func Register(name string, f Factory) {
	regMu.Lock()
	defer regMu.Unlock()
	factories[name] = f
}

// Available lists registered provider names, sorted.
func Available() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Build constructs a registered provider from settings.
func Build(name string, s Settings) (Provider, error) {
	regMu.RLock()
	f, ok := factories[name]
	regMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown metadata provider %q", name)
	}
	return f(s)
}

// Manager holds the active provider and allows swapping it at runtime, so
// saving a token in the settings UI takes effect without a restart. The
// zero-ish state (no provider) makes metadata operations return
// ErrNotConfigured.
type Manager struct {
	mu     sync.RWMutex
	active Provider
}

func NewManager() *Manager {
	return &Manager{}
}

// Current returns the active provider, or nil when none is configured.
func (m *Manager) Current() Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// Set replaces the active provider directly (tests and special wiring).
func (m *Manager) Set(p Provider) {
	m.mu.Lock()
	m.active = p
	m.mu.Unlock()
}

// Configure builds and activates the named provider from settings. An empty
// name, or a factory reporting ErrNotConfigured, deactivates metadata
// cleanly; other build errors leave the previous provider in place.
func (m *Manager) Configure(name string, settings map[string]Settings) error {
	if name == "" {
		m.Set(nil)
		return nil
	}
	p, err := Build(name, settings[name])
	if err != nil {
		if errors.Is(err, ErrNotConfigured) {
			m.Set(nil)
			return nil
		}
		return err
	}
	m.Set(p)
	return nil
}
