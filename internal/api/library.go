package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/quillarr/quillarr/internal/library"
	"github.com/quillarr/quillarr/internal/metadata"
)

const metadataTimeout = 60 * time.Second

func (s *server) metadataCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), metadataTimeout)
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// writeStoreError maps store errors to HTTP responses.
func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, library.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

const notConfiguredMsg = "no metadata provider configured — add a provider token under Settings"

// writeSyncError maps refresh-service errors to HTTP responses.
func writeSyncError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, metadata.ErrNotConfigured):
		writeError(w, http.StatusServiceUnavailable, notConfiguredMsg)
	case errors.Is(err, library.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, metadata.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found at metadata provider")
	default:
		writeError(w, http.StatusBadGateway, err.Error())
	}
}

// --- Search (metadata provider proxy) ---

func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	provider := s.metadata.Current()
	if provider == nil {
		writeError(w, http.StatusServiceUnavailable, notConfiguredMsg)
		return
	}
	term := r.URL.Query().Get("term")
	if term == "" {
		writeError(w, http.StatusBadRequest, "term is required")
		return
	}
	kind := r.URL.Query().Get("type")
	if kind == "" {
		kind = "book"
	}

	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	switch kind {
	case "author":
		results, err := provider.SearchAuthors(ctx, term)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, results)
	case "book":
		results, err := provider.SearchBooks(ctx, term)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, results)
	default:
		writeError(w, http.StatusBadRequest, "type must be author or book")
	}
}

// --- Authors ---

func (s *server) handleListAuthors(w http.ResponseWriter, r *http.Request) {
	authors, err := s.store.ListAuthors()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, authors)
}

// handleAddAuthor syncs an author from the metadata provider with their full
// bibliography (monitored by default). Editions are pulled in when a specific
// book is added or refreshed.
func (s *server) handleAddAuthor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ForeignAuthorID string `json:"foreignAuthorId"`
		Monitored       *bool  `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ForeignAuthorID == "" {
		writeError(w, http.StatusBadRequest, "foreignAuthorId is required")
		return
	}
	monitored := true
	if req.Monitored != nil {
		monitored = *req.Monitored
	}

	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	author, err := s.refresh.SyncAuthor(ctx, req.ForeignAuthorID, monitored)
	if err != nil {
		writeSyncError(w, err)
		return
	}
	s.writeAuthorDetail(w, http.StatusCreated, author.ID)
}

// handleRefreshAuthor re-syncs an existing author (and bibliography) from the
// metadata provider.
func (s *server) handleRefreshAuthor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	if err := s.refresh.RefreshAuthor(ctx, id); err != nil {
		writeSyncError(w, err)
		return
	}
	s.writeAuthorDetail(w, http.StatusOK, id)
}

func (s *server) handleGetAuthor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	s.writeAuthorDetail(w, http.StatusOK, id)
}

func (s *server) writeAuthorDetail(w http.ResponseWriter, status int, id int64) {
	author, err := s.store.GetAuthor(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	books, err := s.store.ListBooks(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	author.Books = books
	writeJSON(w, status, author)
}

func (s *server) handleMonitorAuthor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Monitored bool `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.store.SetAuthorMonitored(id, req.Monitored); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "monitored": req.Monitored})
}

func (s *server) handleDeleteAuthor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeleteAuthor(id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Books ---

func (s *server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	var authorID int64
	if v := r.URL.Query().Get("authorId"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid authorId")
			return
		}
		authorID = id
	}
	books, err := s.store.ListBooks(authorID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, books)
}

// handleAddBook syncs one book (with editions) from the metadata provider.
// The author is created as an unmonitored stub when not in the library yet;
// ebook editions start monitored (Phase 1 is ebook-first).
func (s *server) handleAddBook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ForeignBookID string `json:"foreignBookId"`
		Monitored     *bool  `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ForeignBookID == "" {
		writeError(w, http.StatusBadRequest, "foreignBookId is required")
		return
	}
	monitored := true
	if req.Monitored != nil {
		monitored = *req.Monitored
	}

	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	book, err := s.refresh.SyncBook(ctx, req.ForeignBookID, monitored)
	if err != nil {
		writeSyncError(w, err)
		return
	}
	s.writeBookDetail(w, http.StatusCreated, book.ID)
}

// handleRefreshBook re-syncs an existing book's metadata and editions.
func (s *server) handleRefreshBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	if err := s.refresh.RefreshBook(ctx, id); err != nil {
		writeSyncError(w, err)
		return
	}
	s.writeBookDetail(w, http.StatusOK, id)
}

func (s *server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	s.writeBookDetail(w, http.StatusOK, id)
}

func (s *server) writeBookDetail(w http.ResponseWriter, status int, id int64) {
	book, err := s.store.GetBook(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if book.Editions, err = s.store.ListEditions(id); err != nil {
		writeStoreError(w, err)
		return
	}
	if book.Series, err = s.store.ListSeriesForBook(id); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, status, book)
}

func (s *server) handleMonitorBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Monitored bool `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.store.SetBookMonitored(id, req.Monitored); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "monitored": req.Monitored})
}

func (s *server) handleDeleteBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeleteBook(id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Editions ---

func (s *server) handleMonitorEdition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Monitored bool `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.store.SetEditionMonitored(id, req.Monitored); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "monitored": req.Monitored})
}
