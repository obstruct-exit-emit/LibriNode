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

// SeriesFactory builds a series provider (manga/comic) from its settings.
// ErrNotConfigured means "valid but disabled" (e.g. no API key yet).
type SeriesFactory func(Settings) (SeriesProvider, error)

var seriesFactories = map[string]SeriesFactory{}

// RegisterSeries makes a series provider available under name.
func RegisterSeries(name string, f SeriesFactory) {
	regMu.Lock()
	defer regMu.Unlock()
	seriesFactories[name] = f
}

// SeriesAvailable lists registered series provider names, sorted.
func SeriesAvailable() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	names := make([]string, 0, len(seriesFactories))
	for name := range seriesFactories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BuildSeries constructs a registered series provider from settings.
func BuildSeries(name string, s Settings) (SeriesProvider, error) {
	regMu.RLock()
	f, ok := seriesFactories[name]
	regMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown series provider %q", name)
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
	series map[string]SeriesProvider // keyed by media type (manga, comic)
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

// ConfigureSeries (re)builds every registered series provider from settings;
// providers reporting ErrNotConfigured are simply left inactive. The last
// configured provider per media type wins.
func (m *Manager) ConfigureSeries(settings map[string]Settings) {
	built := map[string]SeriesProvider{}
	for _, name := range SeriesAvailable() {
		p, err := BuildSeries(name, settings[name])
		if err != nil {
			continue // not configured or misconfigured — inactive
		}
		built[p.MediaType()] = p
	}
	m.mu.Lock()
	m.series = built
	m.mu.Unlock()
}

// SeriesFor returns the active series provider for a media type, or nil.
func (m *Manager) SeriesFor(mediaType string) SeriesProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.series[mediaType]
}

// SetSeries injects a series provider directly (tests).
func (m *Manager) SetSeries(p SeriesProvider) {
	m.mu.Lock()
	if m.series == nil {
		m.series = map[string]SeriesProvider{}
	}
	m.series[p.MediaType()] = p
	m.mu.Unlock()
}
