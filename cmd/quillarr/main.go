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

	"github.com/quillarr/quillarr/internal/api"
	"github.com/quillarr/quillarr/internal/config"
	"github.com/quillarr/quillarr/internal/database"
	"github.com/quillarr/quillarr/internal/metadata"
	"github.com/quillarr/quillarr/internal/metadata/hardcover"
)

// version is overridden at build time via -ldflags "-X main.version=x.y.z".
var version = "0.0.1-alpha"

func main() {
	dataDir := flag.String("data", "", "path to the data directory (default: OS-specific config dir)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("Quillarr", version)
		return
	}

	if err := run(*dataDir); err != nil {
		slog.Error("quillarr exited with error", "error", err)
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

	logger.Info("starting Quillarr",
		"version", version,
		"dataDir", cfg.DataDir(),
		"listen", cfg.ListenAddr(),
	)

	db, err := database.Open(cfg.DatabasePath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	var provider metadata.Provider
	if cfg.HardcoverToken != "" {
		provider = hardcover.New(cfg.HardcoverToken)
		logger.Info("metadata provider configured", "provider", provider.Name())
	} else {
		logger.Warn("no metadata provider configured — set hardcover_token in config.yaml to enable search and add")
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           api.NewRouter(cfg, db, provider, version),
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
