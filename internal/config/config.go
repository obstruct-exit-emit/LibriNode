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
	"strings"
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
	// MangaProvider chooses the manga series provider ("anilist",
	// "hardcover", or "none" to disable); empty defaults to anilist.
	// ComicProvider chooses the comic series provider ("hardcover",
	// "comicvine", or "none"); empty defaults to hardcover. "none" turns off
	// search/adds for that library — existing series still refresh through
	// their own source.
	MangaProvider string `yaml:"manga_provider,omitempty"`
	ComicProvider string `yaml:"comic_provider,omitempty"`
	// MangaCoverSource / ComicCoverSource pick volume/issue cover art per
	// library: "file" (extract the first page of the owned archive) or
	// "provider" (the metadata provider's art). Both default to provider art.
	MangaCoverSource string `yaml:"manga_cover_source,omitempty"`
	ComicCoverSource string `yaml:"comic_cover_source,omitempty"`
	// Language / Country / IncludeAdult are global, provider-agnostic
	// metadata preferences: every provider that carries the data prefers
	// matching editions/entries and falls back to less strict picks.
	// Defaults: english, united states, adult content hidden; "none" means
	// no preference at all. They shape METADATA only — acquisition (quality
	// profiles) is untouched.
	Language     string `yaml:"language,omitempty"`
	Country      string `yaml:"country,omitempty"`
	IncludeAdult bool   `yaml:"include_adult,omitempty"`
}

// MangaSeriesProvider returns the configured manga provider name, defaulting
// to anilist.
func (c *Config) MangaSeriesProvider() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Metadata.MangaProvider == "" {
		return "anilist"
	}
	return c.Metadata.MangaProvider
}

// ComicSeriesProvider returns the configured comic provider name, defaulting
// to hardcover.
func (c *Config) ComicSeriesProvider() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Metadata.ComicProvider == "" {
		return "hardcover"
	}
	return c.Metadata.ComicProvider
}

// MetadataLanguage returns the global metadata language preference,
// defaulting to english.
func (c *Config) MetadataLanguage() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Metadata.Language == "" {
		return "english"
	}
	return c.Metadata.Language
}

// MetadataCountry returns the global metadata country preference, defaulting
// to united states.
func (c *Config) MetadataCountry() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Metadata.Country == "" {
		return "united states"
	}
	return c.Metadata.Country
}

// IncludeAdult reports whether adult-flagged results may appear in metadata
// searches (default: hidden).
func (c *Config) IncludeAdult() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Metadata.IncludeAdult
}

// ProviderSettings returns the providers map with the global metadata
// preferences injected into every entry — providers are built from Settings
// alone, so the preferences ride along to each of them, present and future.
// A "none" language/country means no preference and is injected as empty.
func (c *Config) ProviderSettings() map[string]metadata.Settings {
	ms := c.MetadataSettings()
	lang, country, adult := c.MetadataLanguage(), c.MetadataCountry(), c.IncludeAdult()
	if lang == "none" {
		lang = ""
	}
	if country == "none" {
		country = ""
	}
	for name, s := range ms.Providers {
		s.Language, s.Country, s.IncludeAdult = lang, country, adult
		ms.Providers[name] = s
	}
	return ms.Providers
}

// CoverSourceFor returns the effective volume-cover source ("file" or
// "provider") for a manga/comic media type: the per-type setting, or the
// default — the provider's art.
func (c *Config) CoverSourceFor(mediaType string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var v string
	switch mediaType {
	case "manga":
		v = c.Metadata.MangaCoverSource
	case "comic":
		v = c.Metadata.ComicCoverSource
	}
	if v == "" {
		return "provider"
	}
	return v
}

// UseProviderCovers reports whether a media type's volume covers should come
// from the metadata provider instead of the owned file.
func (c *Config) UseProviderCovers(mediaType string) bool {
	return c.CoverSourceFor(mediaType) == "provider"
}

// SeriesSelection maps each series media type to its chosen provider, for
// metadata.Manager.ConfigureSeries.
func (c *Config) SeriesSelection() map[string]string {
	return map[string]string{
		"manga": c.MangaSeriesProvider(),
		"comic": c.ComicSeriesProvider(),
	}
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
	// Magazines: issue books are titled "Magazine - <date/issue>", so the
	// file template can lean on {Book Title}.
	MagazineFolder string `yaml:"magazine_folder" json:"magazineFolder"`
	MagazineFile   string `yaml:"magazine_file" json:"magazineFile"`
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
		MagazineFolder:  "{Series Title}",
		MagazineFile:    "{Book Title}",
	}
}

// AuthSettings holds the optional login account. The password is stored
// only as a PBKDF2 hash; an empty username means authentication is disabled
// (the UI falls back to the API-key prompt).
type AuthSettings struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
}

// Enabled reports whether a login account is configured.
func (a AuthSettings) Enabled() bool { return a.Username != "" }

// ImportSettings tunes Completed Download Handling.
type ImportSettings struct {
	// PackImportAll imports every matching book from a multi-book pack, not
	// just monitored ones. Opt-in: the default (false) fills monitored books
	// only, so grabbing one volume never auto-imports the rest of a bundle.
	PackImportAll bool `yaml:"pack_import_all,omitempty" json:"packImportAll"`
}

type Config struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	APIKey   string `yaml:"api_key"`
	LogLevel string `yaml:"log_level"` // debug, info, warn, error

	Auth     AuthSettings     `yaml:"auth,omitempty"`
	Metadata MetadataSettings `yaml:"metadata"`
	Naming   NamingSettings   `yaml:"naming"`
	Import   ImportSettings   `yaml:"import,omitempty"`

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
	cfg.Naming.FillDefaults()

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

// FillDefaults replaces empty templates with the built-in defaults, so a
// partial update (or hand-edited config) can never leave a media type with
// an empty — and thus garbage-rendering — template.
func (ns *NamingSettings) FillDefaults() {
	def := defaultNaming()
	fill := func(dst *string, fallback string) {
		if strings.TrimSpace(*dst) == "" {
			*dst = fallback
		}
	}
	fill(&ns.EbookFolder, def.EbookFolder)
	fill(&ns.EbookFile, def.EbookFile)
	fill(&ns.AudiobookFolder, def.AudiobookFolder)
	fill(&ns.AudiobookFile, def.AudiobookFile)
	fill(&ns.MangaFolder, def.MangaFolder)
	fill(&ns.MangaFile, def.MangaFile)
	fill(&ns.ComicFolder, def.ComicFolder)
	fill(&ns.ComicFile, def.ComicFile)
	fill(&ns.MagazineFolder, def.MagazineFolder)
	fill(&ns.MagazineFile, def.MagazineFile)
}

// SetNaming replaces the naming templates and persists the config. Empty
// fields fall back to defaults rather than being stored.
func (c *Config) SetNaming(ns NamingSettings) error {
	ns.FillDefaults()
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

// ImportSettings returns the current import options.
func (c *Config) ImportSettings() ImportSettings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Import
}

// SetImport replaces the import options and persists the config.
func (c *Config) SetImport(is ImportSettings) error {
	c.mu.Lock()
	c.Import = is
	c.mu.Unlock()
	return c.save()
}

// PackImportAll reports whether pack imports fill every matching book
// instead of monitored ones only.
func (c *Config) PackImportAll() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Import.PackImportAll
}

// MetadataSettings returns a deep copy so callers can't mutate shared state.
func (c *Config) MetadataSettings() MetadataSettings {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := MetadataSettings{
		Active:           c.Metadata.Active,
		MangaProvider:    c.Metadata.MangaProvider,
		ComicProvider:    c.Metadata.ComicProvider,
		MangaCoverSource: c.Metadata.MangaCoverSource,
		ComicCoverSource: c.Metadata.ComicCoverSource,
		Language:         c.Metadata.Language,
		Country:          c.Metadata.Country,
		IncludeAdult:     c.Metadata.IncludeAdult,
		Providers:        make(map[string]metadata.Settings, len(c.Metadata.Providers)),
	}
	for name, s := range c.Metadata.Providers {
		out.Providers[name] = s
	}
	return out
}

// AuthSettings returns the current login account (possibly disabled).
func (c *Config) AuthSettings() AuthSettings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Auth
}

// SetAuth replaces the login account and persists the config. An empty
// username disables authentication.
func (c *Config) SetAuth(a AuthSettings) error {
	c.mu.Lock()
	c.Auth = a
	c.mu.Unlock()
	return c.save()
}

// CurrentAPIKey returns the API key, safe against concurrent regeneration.
func (c *Config) CurrentAPIKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.APIKey
}

// RegenerateAPIKey replaces the API key with a fresh one and persists it.
// Existing integrations (Prowlarr, scripts) must be updated to the new key.
func (c *Config) RegenerateAPIKey() (string, error) {
	c.mu.Lock()
	c.APIKey = newAPIKey()
	key := c.APIKey
	c.mu.Unlock()
	if err := c.save(); err != nil {
		return "", err
	}
	return key, nil
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
func (c *Config) LogPath() string      { return filepath.Join(c.dataDir, "logs", "librinode.log") }

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
