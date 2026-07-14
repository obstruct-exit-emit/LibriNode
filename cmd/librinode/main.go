package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/librinode/librinode/internal/api"
	"github.com/librinode/librinode/internal/autosearch"
	"github.com/librinode/librinode/internal/config"
	"github.com/librinode/librinode/internal/database"
	"github.com/librinode/librinode/internal/download"
	"github.com/librinode/librinode/internal/importer"
	"github.com/librinode/librinode/internal/indexer"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/logging"
	"github.com/librinode/librinode/internal/metadata"
	"github.com/librinode/librinode/internal/metadata/anilist"
	"github.com/librinode/librinode/internal/metadata/comicvine"
	"github.com/librinode/librinode/internal/metadata/hardcover"
	"github.com/librinode/librinode/internal/organize"
	"github.com/librinode/librinode/internal/refresh"
)

// metadataRefreshInterval is how often the whole library is re-synced with
// the metadata provider (not yet configurable).
const metadataRefreshInterval = 24 * time.Hour

// importInterval is how often Completed Download Handling checks the
// download clients for finished grabs.
const importInterval = time.Minute

// wantedSearchInterval is how often the automatic search sweeps the wanted
// list. Kept conservative to be polite to indexers; per-book and search-all
// endpoints cover "right now".
const wantedSearchInterval = 6 * time.Hour

// healthCheckInterval is how often background health checks re-run (root
// folders, indexers, download clients, metadata token). The System page can
// re-run them on demand.
const healthCheckInterval = 15 * time.Minute

// version is overridden at build time via -ldflags "-X main.version=x.y.z"
// (the release workflow stamps tags). Unstamped builds fall back to the git
// revision Go embeds in the binary, so even a dev build identifies itself.
var version = "dev"

// resolveVersion returns the stamped version, or derives one from the build
// info of an unstamped build: dev-<short-sha>[+dirty] (<commit-date>).
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	var rev, date, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			date = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "+dirty"
			}
		}
	}
	if rev == "" {
		return version
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if len(date) > 10 {
		date = date[:10]
	}
	v := "dev-" + rev + dirty
	if date != "" {
		v += " (" + date + ")"
	}
	return v
}

func main() {
	version = resolveVersion()
	dataDir := flag.String("data", "", "path to the data directory (default: OS-specific config dir)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("LibriNode", version)
		return
	}

	if err := run(*dataDir); err != nil {
		slog.Error("librinode exited with error", "error", err)
		os.Exit(1)
	}
}

func run(dataDir string) error {
	if dataDir == "" {
		var err error
		if dataDir, err = config.DefaultDataDir(); err != nil {
			return fmt.Errorf("resolving default data dir: %w", err)
		}
	}
	// A staged backup restore (POST /backup/{name}/restore) swaps in before
	// anything opens the config or database.
	if err := applyPendingRestore(dataDir); err != nil {
		return fmt.Errorf("applying staged restore: %w", err)
	}

	cfg, err := config.Load(dataDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Logs go to stdout and to a size-rotated file (5 MB, 3 old files kept)
	// that the UI's System → Log viewer reads back.
	logWriter := io.Writer(os.Stdout)
	if err := os.MkdirAll(filepath.Dir(cfg.LogPath()), 0o755); err == nil {
		if lf, err := logging.NewRotatingFile(cfg.LogPath(), 5<<20, 3); err == nil {
			defer lf.Close()
			logWriter = io.MultiWriter(os.Stdout, lf)
		} else {
			fmt.Fprintf(os.Stderr, "opening log file: %v (logging to stdout only)\n", err)
		}
	}
	logger := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{
		Level: cfg.SlogLevel(),
	}))
	slog.SetDefault(logger)

	logger.Info("starting LibriNode",
		"version", version,
		"dataDir", cfg.DataDir(),
		"listen", cfg.ListenAddr(),
	)

	db, err := database.Open(cfg.DatabasePath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// Register all built-in metadata providers, then activate the configured
	// one. New providers are one Register call away; the settings UI and API
	// pick them up automatically.
	metadata.Register("hardcover", hardcover.Factory)
	metadata.RegisterSeries("anilist", anilist.Factory)
	metadata.RegisterSeries("hardcover", hardcover.SeriesFactory)
	// Hardcover serves comics too — a distinct registry key, but the provider
	// reports itself as "hardcover" (what series.Source records).
	metadata.RegisterSeries("hardcover-comics", hardcover.ComicSeriesFactory)
	metadata.RegisterSeries("comicvine", comicvine.Factory)

	providers := metadata.NewManager()
	// ProviderSettings carries the global metadata preferences (language,
	// country, adult filter) into every provider build.
	if err := providers.Configure(cfg.Metadata.Active, cfg.ProviderSettings()); err != nil {
		logger.Warn("activating metadata provider failed", "provider", cfg.Metadata.Active, "error", err)
	}
	providers.ConfigureSeries(cfg.ProviderSettings(), cfg.SeriesSelection())
	if p := providers.Current(); p != nil {
		logger.Info("metadata provider active", "provider", p.Name())
	} else {
		logger.Warn("no metadata provider configured — add a provider token under Settings in the web UI")
	}

	// Background loops: metadata refresh polls the provider manager (so a
	// provider added later via settings is picked up without a restart), and
	// Completed Download Handling imports finished grabs.
	bgCtx, cancelBg := context.WithCancel(context.Background())
	defer cancelBg()
	store := library.NewStore(db)
	downloads := download.NewService(download.NewStore(db))
	go refresh.New(store, providers).RunPeriodic(bgCtx, metadataRefreshInterval)
	imp := importer.New(store, downloads, organize.New(store, cfg), cfg.ImportSettings)
	search := autosearch.New(store, indexer.NewService(indexer.NewStore(db)), downloads)
	// After the importer blocklists a junk/spam download, search for a
	// replacement right away instead of waiting for the next periodic sweep.
	imp.OnJunkBlocklist(func(bookID int64, mediaType string) {
		ctx, cancel := context.WithTimeout(bgCtx, 5*time.Minute)
		defer cancel()
		_, _ = search.SearchBook(ctx, bookID, mediaType)
	})
	go imp.RunPeriodic(bgCtx, importInterval)
	go search.RunPeriodic(bgCtx, wantedSearchInterval)

	handler, healthSvc := api.NewRouter(cfg, db, providers, version)
	go healthSvc.RunPeriodic(bgCtx, healthCheckInterval)

	srv := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("web server listening", "url", fmt.Sprintf("http://%s", cfg.ListenAddr()))
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		logger.Info("shutting down", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}

// applyPendingRestore swaps staged *.restore files (written by the backup
// restore endpoint) into place, keeping the replaced files as *.pre-restore.
func applyPendingRestore(dataDir string) error {
	for _, name := range []string{"config.yaml", "librinode.db"} {
		staged := filepath.Join(dataDir, name+".restore")
		if _, err := os.Stat(staged); err != nil {
			continue
		}
		live := filepath.Join(dataDir, name)
		if _, err := os.Stat(live); err == nil {
			os.Remove(live + ".pre-restore")
			if err := os.Rename(live, live+".pre-restore"); err != nil {
				return err
			}
		}
		if err := os.Rename(staged, live); err != nil {
			return err
		}
		slog.Info("restored from backup", "file", name)
	}
	return nil
}
