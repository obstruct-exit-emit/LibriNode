// Package config loads and persists LibriNode's server configuration.
//
// Precedence (highest wins): environment variables (LIBRINODE_*),
// values in <dataDir>/config.yaml, built-in defaults. The config file is
// created with defaults (including a freshly generated API key) on first run.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/librinode/librinode/internal/metadata"
)

// MetadataSettings selects the active metadata provider and stores each
// provider's settings (kept even while inactive, so switching back is
// painless).
type MetadataSettings struct {
	Active    string                       `yaml:"active"`
	Providers map[string]metadata.Settings `yaml:"providers"`
}

// NamingSettings holds the file-organization templates (per media type as
// later phases land; ebooks first). Rendered per path segment by the naming
// package.
type NamingSettings struct {
	EbookFolder string `yaml:"ebook_folder" json:"ebookFolder"`
	EbookFile   string `yaml:"ebook_file" json:"ebookFile"`
	// Audiobooks use Audiobookshelf's Author/Book-folder layout: the "file"
	// template names the per-book folder (and the audio file inside, for
	// single-file books).
	AudiobookFolder string `yaml:"audiobook_folder" json:"audiobookFolder"`
	AudiobookFile   string `yaml:"audiobook_file" json:"audiobookFile"`
	// Manga/comics use Kavita/Komga's Series/File layout.
	MangaFolder string `yaml:"manga_folder" json:"mangaFolder"`
	MangaFile   string `yaml:"manga_file" json:"mangaFile"`
	ComicFolder string `yaml:"comic_folder" json:"comicFolder"`
	ComicFile   string `yaml:"comic_file" json:"comicFile"`
}

func defaultNaming() NamingSettings {
	return NamingSettings{
		EbookFolder:     "{Author Name}",
		EbookFile:       "{Series Title} {Series Position} - {Book Title}",
		AudiobookFolder: "{Author Name}",
		AudiobookFile:   "{Book Title}",
		MangaFolder:     "{Series Title}",
		MangaFile:       "{Series Title} Vol. {Series Position}",
		ComicFolder:     "{Series Title}",
		ComicFile:       "{Series Title} #{Series Position}",
	}
}

type Config struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	APIKey   string `yaml:"api_key"`
	LogLevel string `yaml:"log_level"` // debug, info, warn, error

	Metadata MetadataSettings `yaml:"metadata"`
	Naming   NamingSettings   `yaml:"naming"`

	// Legacy flat field, migrated into Metadata.Providers on load and
	// dropped from the file on the next save.
	LegacyHardcoverToken string `yaml:"hardcover_token,omitempty"`

	mu      sync.Mutex
	dataDir string
}

func defaults() *Config {
	return &Config{
		Host:     "0.0.0.0",
		Port:     7845,
		LogLevel: "info",
		Metadata: MetadataSettings{
			Active:    "hardcover",
			Providers: map[string]metadata.Settings{},
		},
		Naming: defaultNaming(),
	}
}

// DefaultDataDir returns the OS-appropriate data directory:
// %AppData%\LibriNode on Windows, ~/.config/librinode on Linux.
func DefaultDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	name := "librinode"
	if runtime.GOOS == "windows" {
		name = "LibriNode"
	}
	return filepath.Join(base, name), nil
}

// Load reads the config from dataDir (or the OS default when empty),
// creating the directory and a default config file on first run.
func Load(dataDir string) (*Config, error) {
	if dataDir == "" {
		var err error
		if dataDir, err = DefaultDataDir(); err != nil {
			return nil, fmt.Errorf("resolving default data dir: %w", err)
		}
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating data dir: %w", err)
	}

	cfg := defaults()
	cfg.dataDir = dataDir

	path := cfg.filePath()
	raw, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		// First run: fall through and persist defaults below.
	case err != nil:
		return nil, fmt.Errorf("reading %s: %w", path, err)
	default:
		if err := yaml.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
	}

	applyEnvOverrides(cfg)

	if cfg.Metadata.Providers == nil {
		cfg.Metadata.Providers = map[string]metadata.Settings{}
	}
	// Migrate the legacy flat token into the provider map; omitempty drops
	// the old field from the file on save.
	if cfg.LegacyHardcoverToken != "" {
		if cfg.Metadata.Providers["hardcover"].Token == "" {
			cfg.setProviderToken("hardcover", cfg.LegacyHardcoverToken)
		}
		cfg.LegacyHardcoverToken = ""
	}
	if v := os.Getenv("LIBRINODE_HARDCOVER_TOKEN"); v != "" {
		cfg.setProviderToken("hardcover", v)
	}

	// Empty templates (fresh section, hand-edited file) fall back to defaults.
	if cfg.Naming.EbookFolder == "" {
		cfg.Naming.EbookFolder = defaultNaming().EbookFolder
	}
	if cfg.Naming.EbookFile == "" {
		cfg.Naming.EbookFile = defaultNaming().EbookFile
	}
	if cfg.Naming.AudiobookFolder == "" {
		cfg.Naming.AudiobookFolder = defaultNaming().AudiobookFolder
	}
	if cfg.Naming.AudiobookFile == "" {
		cfg.Naming.AudiobookFile = defaultNaming().AudiobookFile
	}
	if cfg.Naming.MangaFolder == "" {
		cfg.Naming.MangaFolder = defaultNaming().MangaFolder
	}
	if cfg.Naming.MangaFile == "" {
		cfg.Naming.MangaFile = defaultNaming().MangaFile
	}
	if cfg.Naming.ComicFolder == "" {
		cfg.Naming.ComicFolder = defaultNaming().ComicFolder
	}
	if cfg.Naming.ComicFile == "" {
		cfg.Naming.ComicFile = defaultNaming().ComicFile
	}

	if cfg.APIKey == "" {
		cfg.APIKey = newAPIKey()
	}

	// Persist so the generated API key (and any new defaults) survive restarts.
	if err := cfg.save(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("LIBRINODE_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("LIBRINODE_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	if v := os.Getenv("LIBRINODE_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("LIBRINODE_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
}

func (c *Config) setProviderToken(provider, token string) {
	s := c.Metadata.Providers[provider]
	s.Token = token
	c.Metadata.Providers[provider] = s
}

// SetMetadata replaces the metadata settings and persists the config.
// Safe for concurrent use from API handlers.
func (c *Config) SetMetadata(ms MetadataSettings) error {
	c.mu.Lock()
	c.Metadata = ms
	c.mu.Unlock()
	return c.save()
}

// SetNaming replaces the naming templates and persists the config.
func (c *Config) SetNaming(ns NamingSettings) error {
	c.mu.Lock()
	c.Naming = ns
	c.mu.Unlock()
	return c.save()
}

// NamingSettings returns the current naming templates.
func (c *Config) NamingSettings() NamingSettings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Naming
}

// MetadataSettings returns a deep copy so callers can't mutate shared state.
func (c *Config) MetadataSettings() MetadataSettings {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := MetadataSettings{
		Active:    c.Metadata.Active,
		Providers: make(map[string]metadata.Settings, len(c.Metadata.Providers)),
	}
	for name, s := range c.Metadata.Providers {
		out.Providers[name] = s
	}
	return out
}

func newAPIKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand unavailable: %v", err))
	}
	return hex.EncodeToString(b)
}

func (c *Config) save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	out, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.filePath(), out, 0o600)
}

func (c *Config) filePath() string     { return filepath.Join(c.dataDir, "config.yaml") }
func (c *Config) DataDir() string      { return c.dataDir }
func (c *Config) DatabasePath() string { return filepath.Join(c.dataDir, "librinode.db") }

func (c *Config) ListenAddr() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

func (c *Config) SlogLevel() slog.Level {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
