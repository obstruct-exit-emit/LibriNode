// Package api exposes Quillarr's versioned REST API and serves the web UI.
// Every endpoint under /api/v1 requires the API key via the X-Api-Key header
// (or ?apikey= query parameter); /ping is open for health checks.
package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/quillarr/quillarr/internal/config"
	"github.com/quillarr/quillarr/internal/library"
	"github.com/quillarr/quillarr/internal/metadata"
)

type server struct {
	cfg      *config.Config
	db       *sql.DB
	store    *library.Store
	metadata metadata.Provider // nil when no provider is configured
	version  string
}

func NewRouter(cfg *config.Config, db *sql.DB, provider metadata.Provider, version string) http.Handler {
	s := &server{cfg: cfg, db: db, store: library.NewStore(db), metadata: provider, version: version}

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
	mux.HandleFunc("DELETE /api/v1/author/{id}", s.auth(s.handleDeleteAuthor))
	mux.HandleFunc("GET /api/v1/book", s.auth(s.handleListBooks))
	mux.HandleFunc("POST /api/v1/book", s.auth(s.handleAddBook))
	mux.HandleFunc("GET /api/v1/book/{id}", s.auth(s.handleGetBook))
	mux.HandleFunc("PUT /api/v1/book/{id}/monitor", s.auth(s.handleMonitorBook))
	mux.HandleFunc("DELETE /api/v1/book/{id}", s.auth(s.handleDeleteBook))
	mux.HandleFunc("PUT /api/v1/edition/{id}/monitor", s.auth(s.handleMonitorEdition))

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
