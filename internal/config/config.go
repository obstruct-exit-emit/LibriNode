// Package config loads and persists Quillarr's server configuration.
//
// Precedence (highest wins): environment variables (QUILLARR_*),
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

	"gopkg.in/yaml.v3"
)

type Config struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	APIKey   string `yaml:"api_key"`
	LogLevel string `yaml:"log_level"` // debug, info, warn, error

	// HardcoverToken is the Hardcover.app API token used for book and
	// audiobook metadata. Empty disables the provider (search/add return 503).
	HardcoverToken string `yaml:"hardcover_token"`

	dataDir string
}

func defaults() *Config {
	return &Config{
		Host:     "0.0.0.0",
		Port:     7845, // Q-U-I-L on a phone keypad
		LogLevel: "info",
	}
}

// DefaultDataDir returns the OS-appropriate data directory:
// %AppData%\Quillarr on Windows, ~/.config/quillarr on Linux.
func DefaultDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	name := "quillarr"
	if runtime.GOOS == "windows" {
		name = "Quillarr"
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
	if v := os.Getenv("QUILLARR_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("QUILLARR_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	if v := os.Getenv("QUILLARR_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("QUILLARR_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("QUILLARR_HARDCOVER_TOKEN"); v != "" {
		cfg.HardcoverToken = v
	}
}

func newAPIKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand unavailable: %v", err))
	}
	return hex.EncodeToString(b)
}

func (c *Config) save() error {
	out, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.filePath(), out, 0o600)
}

func (c *Config) filePath() string     { return filepath.Join(c.dataDir, "config.yaml") }
func (c *Config) DataDir() string      { return c.dataDir }
func (c *Config) DatabasePath() string { return filepath.Join(c.dataDir, "quillarr.db") }

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
