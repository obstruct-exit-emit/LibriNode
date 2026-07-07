package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/metadata"
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
	term := r.URL.Query().Get("term")
	if term == "" {
		writeError(w, http.StatusBadRequest, "term is required")
		return
	}
	kind := r.URL.Query().Get("type")
	if kind == "" {
		kind = "book"
	}

	// Series-first types go to their own providers.
	if mediaType, ok := seriesMediaType(kind); ok {
		s.handleSearchSeries(w, r, mediaType, term)
		return
	}

	provider := s.metadata.Current()
	if provider == nil {
		writeError(w, http.StatusServiceUnavailable, notConfiguredMsg)
		return
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
		writeError(w, http.StatusBadRequest, "type must be author, book, manga, or comic")
	}
}

// --- Authors ---

func (s *server) handleListAuthors(w http.ResponseWriter, r *http.Request) {
	var authors []library.Author
	var err error
	if lib := r.URL.Query().Get("library"); lib != "" {
		if _, ok := formatLibrary(lib); !ok {
			writeError(w, http.StatusBadRequest, "library must be ebook or audiobook")
			return
		}
		authors, err = s.store.ListAuthorsInLibrary(lib)
	} else {
		authors, err = s.store.ListAuthors()
	}
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
		Library         string `json:"library"` // which format library to add into
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ForeignAuthorID == "" {
		writeError(w, http.StatusBadRequest, "foreignAuthorId is required")
		return
	}
	monitored := true
	if req.Monitored != nil {
		monitored = *req.Monitored
	}
	lib, ok := formatLibrary(req.Library)
	if !ok {
		writeError(w, http.StatusBadRequest, "library must be ebook or audiobook")
		return
	}

	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	author, err := s.refresh.SyncAuthor(ctx, req.ForeignAuthorID, monitored, lib)
	if err != nil {
		writeSyncError(w, err)
		return
	}
	// Re-adds enroll existing books too (upserts preserve membership).
	if err := s.store.SetAuthorBooksLibrary(author.ID, lib, monitored); err != nil {
		writeStoreError(w, err)
		return
	}
	s.rematchFiles()
	s.writeAuthorDetail(w, http.StatusCreated, author.ID)
}

// formatLibrary validates the target format library ("" defaults to ebook).
func formatLibrary(v string) (string, bool) {
	switch v {
	case "", "ebook":
		return "ebook", true
	case "audiobook":
		return "audiobook", true
	}
	return "", false
}

// rematchFiles attaches previously scanned unmatched files to newly added
// books, so "add the book" is all a user needs to do after a scan.
func (s *server) rematchFiles() {
	if _, err := s.scanner.RematchUnmatched(); err != nil {
		slog.Warn("rematching unmatched files", "error", err)
	}
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
	deleteFiles := wantsFileDeletion(r)
	var paths []string
	if deleteFiles {
		var err error
		if paths, err = s.store.FilePathsForAuthor(id); err != nil {
			writeStoreError(w, err)
			return
		}
	}
	if err := s.store.DeleteAuthor(id); err != nil {
		writeStoreError(w, err)
		return
	}
	s.finishDelete(w, deleteFiles, paths)
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
// the book joins the library named in the request ("ebook" by default).
func (s *server) handleAddBook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ForeignBookID string `json:"foreignBookId"`
		Monitored     *bool  `json:"monitored"`
		Library       string `json:"library"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ForeignBookID == "" {
		writeError(w, http.StatusBadRequest, "foreignBookId is required")
		return
	}
	monitored := true
	if req.Monitored != nil {
		monitored = *req.Monitored
	}
	lib, ok := formatLibrary(req.Library)
	if !ok {
		writeError(w, http.StatusBadRequest, "library must be ebook or audiobook")
		return
	}

	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	book, err := s.refresh.SyncBook(ctx, req.ForeignBookID, monitored, lib)
	if err != nil {
		writeSyncError(w, err)
		return
	}
	// Re-adds enroll the existing book too (upserts preserve membership).
	if err := s.store.SetBookLibrary(book.ID, lib, true, monitored); err != nil {
		writeStoreError(w, err)
		return
	}
	s.rematchFiles()
	s.writeBookDetail(w, http.StatusCreated, book.ID)
}

// handleBookLibrary is the cross-add/remove: PUT /book/{id}/library puts a
// prose book into (or out of) a format library, with its monitored choice.
func (s *server) handleBookLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Library   string `json:"library"`
		Member    bool   `json:"member"`
		Monitored bool   `json:"monitored"`
		// DeleteFiles removes this format's files from disk when leaving
		// the library (ignored when member is true).
		DeleteFiles bool `json:"deleteFiles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	lib, ok := formatLibrary(req.Library)
	if !ok || req.Library == "" {
		writeError(w, http.StatusBadRequest, "library must be ebook or audiobook")
		return
	}
	deleteFiles := req.DeleteFiles && !req.Member
	var paths []string
	if deleteFiles {
		var err error
		if paths, err = s.store.FilePathsForBookFormat(id, lib); err != nil {
			writeStoreError(w, err)
			return
		}
	}
	if err := s.store.SetBookLibrary(id, lib, req.Member, req.Monitored); err != nil {
		writeStoreError(w, err)
		return
	}
	if deleteFiles {
		if _, errs := s.removeFilesFromDisk(paths); len(errs) > 0 {
			slog.Warn("deleting files on library removal", "bookId", id, "errors", strings.Join(errs, "; "))
		}
		// The book row survives, so its file rows must go explicitly (disk
		// deletion alone would leave them stale until the next scan).
		if err := s.store.DeleteBookFilesForFormat(id, lib); err != nil {
			writeStoreError(w, err)
			return
		}
	}
	s.writeBookDetail(w, http.StatusOK, id)
}

// handleLibraries reports which media-type libraries are set up (Plex-style:
// the UI only shows active ones).
func (s *server) handleLibraries(w http.ResponseWriter, r *http.Request) {
	statuses, err := s.store.LibraryStatuses()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statuses)
}

// handleHome builds the Home page: per-library sections, active libraries
// only, types never mixed within a row.
func (s *server) handleHome(w http.ResponseWriter, r *http.Request) {
	sections, err := s.store.Home(12)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sections)
}

// handleAuthorMissing lists an author's bibliography gaps for one format
// library — prose books neither monitored nor owned there (hidden from the
// Books grid) — for the author page's Missing section.
func (s *server) handleAuthorMissing(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	lib, ok := formatLibrary(r.URL.Query().Get("library"))
	if !ok {
		writeError(w, http.StatusBadRequest, "library must be ebook or audiobook")
		return
	}
	books, err := s.store.MissingForAuthor(id, lib)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, books)
}

// handleWanted lists one library's wanted items — monitored but missing
// that format's file — for the per-library Wanted page.
func (s *server) handleWanted(w http.ResponseWriter, r *http.Request) {
	mediaType := r.URL.Query().Get("library")
	if !slices.Contains(library.MediaTypes, mediaType) {
		writeError(w, http.StatusBadRequest, "library must be one of: "+strings.Join(library.MediaTypes, ", "))
		return
	}
	items, err := s.store.Wanted(mediaType)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleCalendar returns dated releases across all libraries. ?past= and
// ?days= (defaults 30/90) bound the window around today.
func (s *server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	intParam := func(name string, def, max int) int {
		v, err := strconv.Atoi(r.URL.Query().Get(name))
		if err != nil || v < 0 {
			return def
		}
		return min(v, max)
	}
	past := intParam("past", 30, 365)
	days := intParam("days", 90, 365)
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -past).Format("2006-01-02")
	to := now.AddDate(0, 0, days).Format("2006-01-02")
	items, err := s.store.Calendar(from, to)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "from": from, "to": to})
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
	if book.Files, err = s.store.ListBookFiles(id); err != nil {
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
	deleteFiles := wantsFileDeletion(r)
	var paths []string
	if deleteFiles {
		var err error
		if paths, err = s.store.FilePathsForBook(id); err != nil {
			writeStoreError(w, err)
			return
		}
	}
	if err := s.store.DeleteBook(id); err != nil {
		writeStoreError(w, err)
		return
	}
	s.finishDelete(w, deleteFiles, paths)
}

