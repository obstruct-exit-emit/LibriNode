package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/librinode/librinode/internal/api"
	"github.com/librinode/librinode/internal/config"
	"github.com/librinode/librinode/internal/database"
	"github.com/librinode/librinode/internal/download"
	"github.com/librinode/librinode/internal/importer"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/metadata"
	"github.com/librinode/librinode/internal/metadata/hardcover"
	"github.com/librinode/librinode/internal/organize"
	"github.com/librinode/librinode/internal/refresh"
)

// metadataRefreshInterval is how often the whole library is re-synced with
// the metadata provider. Configurable scheduling can come with the settings
// UI in Phase 5.
const metadataRefreshInterval = 24 * time.Hour

// importInterval is how often Completed Download Handling checks the
// download clients for finished grabs.
const importInterval = time.Minute

// version is overridden at build time via -ldflags "-X main.version=x.y.z".
var version = "0.0.1-alpha"

func main() {
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
	cfg, err := config.Load(dataDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
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

	providers := metadata.NewManager()
	if err := providers.Configure(cfg.Metadata.Active, cfg.Metadata.Providers); err != nil {
		logger.Warn("activating metadata provider failed", "provider", cfg.Metadata.Active, "error", err)
	}
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
	go refresh.New(store, providers).RunPeriodic(bgCtx, metadataRefreshInterval)
	go importer.New(store, download.NewService(download.NewStore(db)), organize.New(store, cfg)).
		RunPeriodic(bgCtx, importInterval)

	srv := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           api.NewRouter(cfg, db, providers, version),
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
