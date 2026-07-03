package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/quillarr/quillarr/internal/indexer"
)

const indexerTestTimeout = 30 * time.Second

func writeIndexerError(w http.ResponseWriter, err error) {
	if errors.Is(err, indexer.ErrNotFound) {
		writeError(w, http.StatusNotFound, "indexer not found")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

// decodeIndexer reads and validates an indexer definition from the body.
func decodeIndexer(r *http.Request) (*indexer.Indexer, string) {
	var in indexer.Indexer
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return nil, "invalid JSON body"
	}
	in.Name = strings.TrimSpace(in.Name)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if in.Name == "" {
		return nil, "name is required"
	}
	if in.Type != indexer.TypeNewznab && in.Type != indexer.TypeTorznab {
		return nil, "type must be newznab or torznab"
	}
	if !strings.HasPrefix(in.BaseURL, "http://") && !strings.HasPrefix(in.BaseURL, "https://") {
		return nil, "baseUrl must be an http(s) URL"
	}
	if in.Categories == "" {
		in.Categories = "7000,7020"
	}
	if in.Priority <= 0 || in.Priority > 50 {
		in.Priority = 25
	}
	return &in, ""
}

func (s *server) handleListIndexers(w http.ResponseWriter, r *http.Request) {
	indexers, err := s.indexers.Store().List()
	if err != nil {
		writeIndexerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, indexers)
}

func (s *server) handleAddIndexer(w http.ResponseWriter, r *http.Request) {
	in, msg := decodeIndexer(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.indexers.Store().Add(in); err != nil {
		writeError(w, http.StatusConflict, "could not save indexer (duplicate name?): "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, in)
}

func (s *server) handleUpdateIndexer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	in, msg := decodeIndexer(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	in.ID = id
	if err := s.indexers.Store().Update(in); err != nil {
		writeIndexerError(w, err)
		return
	}
	updated, err := s.indexers.Store().Get(id)
	if err != nil {
		writeIndexerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *server) handleDeleteIndexer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.indexers.Store().Delete(id); err != nil {
		writeIndexerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTestIndexer checks an unsaved indexer definition against the live
// endpoint (fetches its capabilities).
func (s *server) handleTestIndexer(w http.ResponseWriter, r *http.Request) {
	in, msg := decodeIndexer(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), indexerTestTimeout)
	defer cancel()

	if err := s.indexers.Client().Test(ctx, in); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleSearchReleases is the manual release search: query every enabled
// indexer and return merged candidates. Per-indexer failures come back in
// "errors" alongside the results that did arrive.
func (s *server) handleSearchReleases(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if term == "" {
		writeError(w, http.StatusBadRequest, "term is required")
		return
	}
	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	releases, errs, err := s.indexers.SearchAll(ctx, term)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"releases": releases, "errors": errs})
}
