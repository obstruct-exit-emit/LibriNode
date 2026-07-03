package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFirstRunCreatesConfigWithAPIKey(t *testing.T) {
	dir := t.TempDir()

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey == "" {
		t.Error("expected a generated API key on first run")
	}
	if cfg.Port != 7845 {
		t.Errorf("default port = %d, want 7845", cfg.Port)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err != nil {
		t.Errorf("config.yaml not persisted: %v", err)
	}

	// Second load must reuse the same key, not regenerate.
	cfg2, err := Load(dir)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if cfg2.APIKey != cfg.APIKey {
		t.Error("API key changed between loads")
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("LIBRINODE_PORT", "9999")
	t.Setenv("LIBRINODE_LOG_LEVEL", "debug")

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 9999 {
		t.Errorf("Port = %d, want 9999 from env", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug from env", cfg.LogLevel)
	}
}
