// Package api exposes LibriNode's versioned REST API and serves the web UI.
// Every endpoint under /api/v1 requires the API key via the X-Api-Key header
// (or ?apikey= query parameter); /ping is open for health checks.
package api

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/librinode/librinode/internal/autosearch"
	"github.com/librinode/librinode/internal/config"
	"github.com/librinode/librinode/internal/download"
	"github.com/librinode/librinode/internal/health"
	"github.com/librinode/librinode/internal/importer"
	"github.com/librinode/librinode/internal/indexer"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/metadata"
	"github.com/librinode/librinode/internal/organize"
	"github.com/librinode/librinode/internal/refresh"
	"github.com/librinode/librinode/internal/scanner"
	"github.com/librinode/librinode/web"
)

type server struct {
	cfg       *config.Config
	db        *sql.DB
	store     *library.Store
	metadata  *metadata.Manager // active provider is swappable at runtime
	refresh   *refresh.Service
	scanner   *scanner.Service
	organize  *organize.Service
	indexers  *indexer.Service
	downloads *download.Service
	importer  *importer.Service
	search    *autosearch.Service
	health    *health.Service
	sessions  *sessionStore
	webFS     fs.FS // nil when no frontend build is embedded
	version   string
}

// NewRouter builds the API handler. The returned health service is the
// caller's to run periodically (main starts it alongside the other
// background loops); its endpoints are already wired into the handler.
func NewRouter(cfg *config.Config, db *sql.DB, providers *metadata.Manager, version string) (http.Handler, *health.Service) {
	store := library.NewStore(db)
	org := organize.New(store, cfg)
	downloads := download.NewService(download.NewStore(db))
	indexers := indexer.NewService(indexer.NewStore(db))
	s := &server{
		cfg:       cfg,
		db:        db,
		store:     store,
		metadata:  providers,
		refresh:   refresh.New(store, providers),
		scanner:   scanner.New(store),
		organize:  org,
		indexers:  indexers,
		downloads: downloads,
		importer:  importer.New(store, downloads, org),
		search:    autosearch.New(store, indexers, downloads),
		health:    health.New(store, indexers, downloads, providers),
		sessions:  newSessionStore(),
		version:   version,
	}
	if dist, ok := web.FS(); ok {
		s.webFS = dist
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", s.handlePing)
	// Auth endpoints: status and login are unauthenticated by nature; the
	// rest require an existing session or the API key.
	mux.HandleFunc("GET /api/v1/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
	mux.HandleFunc("PUT /api/v1/auth/credentials", s.auth(s.handleSetCredentials))
	mux.HandleFunc("POST /api/v1/auth/apikey/regenerate", s.auth(s.handleRegenerateAPIKey))
	mux.HandleFunc("GET /api/v1/system/status", s.auth(s.handleSystemStatus))
	mux.HandleFunc("GET /api/v1/backup", s.auth(s.handleListBackups))
	mux.HandleFunc("POST /api/v1/backup", s.auth(s.handleCreateBackup))
	mux.HandleFunc("DELETE /api/v1/backup/{name}", s.auth(s.handleDeleteBackup))
	mux.HandleFunc("POST /api/v1/backup/{name}/restore", s.auth(s.handleRestoreBackup))
	mux.HandleFunc("GET /api/v1/backup/{name}/download", s.auth(s.handleDownloadBackup))
	mux.HandleFunc("GET /api/v1/health", s.auth(s.handleHealth))
	mux.HandleFunc("POST /api/v1/health/check", s.auth(s.handleHealthCheck))
	mux.HandleFunc("GET /api/v1/log", s.auth(s.handleLogTail))
	mux.HandleFunc("GET /api/v1/rootfolder", s.auth(s.handleListRootFolders))
	mux.HandleFunc("POST /api/v1/rootfolder", s.auth(s.handleAddRootFolder))
	mux.HandleFunc("DELETE /api/v1/rootfolder/{id}", s.auth(s.handleDeleteRootFolder))

	mux.HandleFunc("GET /api/v1/search", s.auth(s.handleSearch))
	mux.HandleFunc("GET /api/v1/author", s.auth(s.handleListAuthors))
	mux.HandleFunc("POST /api/v1/author", s.auth(s.handleAddAuthor))
	mux.HandleFunc("GET /api/v1/author/{id}", s.auth(s.handleGetAuthor))
	mux.HandleFunc("PUT /api/v1/author/{id}/monitor", s.auth(s.handleMonitorAuthor))
	mux.HandleFunc("POST /api/v1/author/{id}/refresh", s.auth(s.handleRefreshAuthor))
	mux.HandleFunc("GET /api/v1/author/{id}/missing", s.auth(s.handleAuthorMissing))
	mux.HandleFunc("PUT /api/v1/author/{id}/library", s.auth(s.handleAuthorLibrary))
	mux.HandleFunc("POST /api/v1/author/{id}/search", s.auth(s.handleAuthorSearch))
	mux.HandleFunc("DELETE /api/v1/author/{id}", s.auth(s.handleDeleteAuthor))
	mux.HandleFunc("GET /api/v1/book", s.auth(s.handleListBooks))
	mux.HandleFunc("POST /api/v1/book", s.auth(s.handleAddBook))
	mux.HandleFunc("GET /api/v1/book/{id}", s.auth(s.handleGetBook))
	mux.HandleFunc("PUT /api/v1/book/{id}/monitor", s.auth(s.handleMonitorBook))
	mux.HandleFunc("PUT /api/v1/book/{id}/library", s.auth(s.handleBookLibrary))
	mux.HandleFunc("GET /api/v1/libraries", s.auth(s.handleLibraries))
	mux.HandleFunc("GET /api/v1/home", s.auth(s.handleHome))
	mux.HandleFunc("GET /api/v1/wanted", s.auth(s.handleWanted))
	mux.HandleFunc("GET /api/v1/calendar", s.auth(s.handleCalendar))
	mux.HandleFunc("POST /api/v1/book/{id}/refresh", s.auth(s.handleRefreshBook))
	mux.HandleFunc("DELETE /api/v1/book/{id}", s.auth(s.handleDeleteBook))
	mux.HandleFunc("GET /api/v1/series", s.auth(s.handleListSeries))
	mux.HandleFunc("POST /api/v1/series", s.auth(s.handleAddSeries))
	mux.HandleFunc("GET /api/v1/series/{id}", s.auth(s.handleGetSeries))
	mux.HandleFunc("PUT /api/v1/series/{id}/monitor", s.auth(s.handleMonitorSeries))
	mux.HandleFunc("POST /api/v1/series/{id}/refresh", s.auth(s.handleRefreshSeries))
	mux.HandleFunc("DELETE /api/v1/series/{id}", s.auth(s.handleDeleteSeries))
	mux.HandleFunc("POST /api/v1/library/scan", s.auth(s.handleScan))
	mux.HandleFunc("GET /api/v1/library/rename", s.auth(s.handleRenamePreview))
	mux.HandleFunc("POST /api/v1/library/rename", s.auth(s.handleRenameApply))
	mux.HandleFunc("GET /api/v1/bookfile", s.auth(s.handleListBookFiles))
	mux.HandleFunc("POST /api/v1/bookfile/{id}/match", s.auth(s.handleMatchBookFile))
	mux.HandleFunc("DELETE /api/v1/bookfile/{id}", s.auth(s.handleDeleteBookFile))

	mux.HandleFunc("GET /api/v1/settings/metadata", s.auth(s.handleGetMetadataSettings))
	mux.HandleFunc("PUT /api/v1/settings/metadata", s.auth(s.handlePutMetadataSettings))
	mux.HandleFunc("POST /api/v1/settings/metadata/test", s.auth(s.handleTestMetadataProvider))
	mux.HandleFunc("GET /api/v1/settings/naming", s.auth(s.handleGetNamingSettings))
	mux.HandleFunc("PUT /api/v1/settings/naming", s.auth(s.handlePutNamingSettings))

	mux.HandleFunc("GET /api/v1/qualityprofile", s.auth(s.handleListProfiles))
	mux.HandleFunc("POST /api/v1/qualityprofile", s.auth(s.handleAddProfile))
	mux.HandleFunc("PUT /api/v1/qualityprofile/{id}", s.auth(s.handleUpdateProfile))
	mux.HandleFunc("PUT /api/v1/qualityprofile/{id}/default", s.auth(s.handleDefaultProfile))
	mux.HandleFunc("DELETE /api/v1/qualityprofile/{id}", s.auth(s.handleDeleteProfile))

	mux.HandleFunc("GET /api/v1/indexer", s.auth(s.handleListIndexers))
	mux.HandleFunc("POST /api/v1/indexer", s.auth(s.handleAddIndexer))
	mux.HandleFunc("GET /api/v1/indexer/schema", s.auth(s.handleIndexerSchema))
	mux.HandleFunc("GET /api/v1/indexer/{id}", s.auth(s.handleGetIndexer))
	mux.HandleFunc("PUT /api/v1/indexer/{id}", s.auth(s.handleUpdateIndexer))
	mux.HandleFunc("DELETE /api/v1/indexer/{id}", s.auth(s.handleDeleteIndexer))
	mux.HandleFunc("POST /api/v1/indexer/test", s.auth(s.handleTestIndexer))
	mux.HandleFunc("GET /api/v1/tag", s.auth(s.handleListTags))
	mux.HandleFunc("GET /api/v1/release", s.auth(s.handleSearchReleases))
	mux.HandleFunc("POST /api/v1/release/grab", s.auth(s.handleGrabRelease))

	mux.HandleFunc("GET /api/v1/downloadclient", s.auth(s.handleListDownloadClients))
	mux.HandleFunc("POST /api/v1/downloadclient", s.auth(s.handleAddDownloadClient))
	mux.HandleFunc("PUT /api/v1/downloadclient/{id}", s.auth(s.handleUpdateDownloadClient))
	mux.HandleFunc("DELETE /api/v1/downloadclient/{id}", s.auth(s.handleDeleteDownloadClient))
	mux.HandleFunc("POST /api/v1/downloadclient/test", s.auth(s.handleTestDownloadClient))
	mux.HandleFunc("GET /api/v1/queue", s.auth(s.handleQueue))
	mux.HandleFunc("GET /api/v1/blocklist", s.auth(s.handleBlocklist))
	mux.HandleFunc("DELETE /api/v1/blocklist/{id}", s.auth(s.handleUnblock))
	mux.HandleFunc("POST /api/v1/library/import", s.auth(s.handleImport))
	mux.HandleFunc("GET /api/v1/history", s.auth(s.handleHistory))
	mux.HandleFunc("POST /api/v1/book/{id}/search", s.auth(s.handleAutoSearchBook))
	mux.HandleFunc("POST /api/v1/library/search", s.auth(s.handleSearchWanted))

	mux.HandleFunc("/", s.handleIndex)

	return logRequests(mux), s.health
}

// handleHealth returns the cached result of the last background health run
// (checkedAt is the zero time before the first run completes).
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.health.Last())
}

// handleHealthCheck re-runs every check now — the System page's button.
func (s *server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.health.Check(r.Context()))
}

// auth admits requests carrying the API key (scripts, Prowlarr) or a valid
// login session cookie (the web UI once authentication is enabled).
func (s *server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Api-Key")
		if key == "" {
			key = r.URL.Query().Get("apikey")
		}
		if key != "" && key == s.cfg.CurrentAPIKey() {
			next(w, r)
			return
		}
		if s.hasSession(r) {
			next(w, r)
			return
		}
		writeError(w, http.StatusUnauthorized, "invalid or missing API key")
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		slog.Debug("request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encoding response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
