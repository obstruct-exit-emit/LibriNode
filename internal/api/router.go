// Package api exposes Quillarr's versioned REST API and serves the web UI.
// Every endpoint under /api/v1 requires the API key via the X-Api-Key header
// (or ?apikey= query parameter); /ping is open for health checks.
package api

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/quillarr/quillarr/internal/config"
	"github.com/quillarr/quillarr/internal/library"
	"github.com/quillarr/quillarr/internal/metadata"
	"github.com/quillarr/quillarr/internal/organize"
	"github.com/quillarr/quillarr/internal/refresh"
	"github.com/quillarr/quillarr/internal/scanner"
	"github.com/quillarr/quillarr/web"
)

type server struct {
	cfg      *config.Config
	db       *sql.DB
	store    *library.Store
	metadata *metadata.Manager // active provider is swappable at runtime
	refresh  *refresh.Service
	scanner  *scanner.Service
	organize *organize.Service
	webFS    fs.FS // nil when no frontend build is embedded
	version  string
}

func NewRouter(cfg *config.Config, db *sql.DB, providers *metadata.Manager, version string) http.Handler {
	store := library.NewStore(db)
	s := &server{
		cfg:      cfg,
		db:       db,
		store:    store,
		metadata: providers,
		refresh:  refresh.New(store, providers),
		scanner:  scanner.New(store),
		organize: organize.New(store, cfg),
		version:  version,
	}
	if dist, ok := web.FS(); ok {
		s.webFS = dist
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", s.handlePing)
	mux.HandleFunc("GET /api/v1/system/status", s.auth(s.handleSystemStatus))
	mux.HandleFunc("GET /api/v1/rootfolder", s.auth(s.handleListRootFolders))
	mux.HandleFunc("POST /api/v1/rootfolder", s.auth(s.handleAddRootFolder))
	mux.HandleFunc("DELETE /api/v1/rootfolder/{id}", s.auth(s.handleDeleteRootFolder))

	mux.HandleFunc("GET /api/v1/search", s.auth(s.handleSearch))
	mux.HandleFunc("GET /api/v1/author", s.auth(s.handleListAuthors))
	mux.HandleFunc("POST /api/v1/author", s.auth(s.handleAddAuthor))
	mux.HandleFunc("GET /api/v1/author/{id}", s.auth(s.handleGetAuthor))
	mux.HandleFunc("PUT /api/v1/author/{id}/monitor", s.auth(s.handleMonitorAuthor))
	mux.HandleFunc("POST /api/v1/author/{id}/refresh", s.auth(s.handleRefreshAuthor))
	mux.HandleFunc("DELETE /api/v1/author/{id}", s.auth(s.handleDeleteAuthor))
	mux.HandleFunc("GET /api/v1/book", s.auth(s.handleListBooks))
	mux.HandleFunc("POST /api/v1/book", s.auth(s.handleAddBook))
	mux.HandleFunc("GET /api/v1/book/{id}", s.auth(s.handleGetBook))
	mux.HandleFunc("PUT /api/v1/book/{id}/monitor", s.auth(s.handleMonitorBook))
	mux.HandleFunc("POST /api/v1/book/{id}/refresh", s.auth(s.handleRefreshBook))
	mux.HandleFunc("DELETE /api/v1/book/{id}", s.auth(s.handleDeleteBook))
	mux.HandleFunc("PUT /api/v1/edition/{id}/monitor", s.auth(s.handleMonitorEdition))
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

	mux.HandleFunc("/", s.handleIndex)

	return logRequests(mux)
}

func (s *server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Api-Key")
		if key == "" {
			key = r.URL.Query().Get("apikey")
		}
		if key != s.cfg.APIKey {
			writeError(w, http.StatusUnauthorized, "invalid or missing API key")
			return
		}
		next(w, r)
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
