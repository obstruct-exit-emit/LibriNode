package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/organize"
)

// scans can walk large libraries on slow disks; generous but bounded.
const scanTimeout = 10 * time.Minute

// handleScan walks all root folders synchronously and reports what it
// found. Fine for current library sizes; a queued background command system
// can replace it if scans ever get slow.
func (s *server) handleScan(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), scanTimeout)
	defer cancel()

	result, err := s.scanner.Scan(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type renameResponse struct {
	Moves []organizeMove `json:"moves"`
	Skips []string       `json:"skips"`
}

// organizeMove aliases organize.Move for JSON without importing it here twice.
type organizeMove = organize.Move

func renameBookID(r *http.Request) (int64, bool) {
	v := r.URL.Query().Get("bookId")
	if v == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(v, 10, 64)
	return id, err == nil && id > 0
}

func renameIDParam(r *http.Request, name string) (int64, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(v, 10, 64)
	return id, err == nil && id > 0
}

// planRename picks the organize scope: one book, one series, one author, or
// everything.
func (s *server) planRename(bookID, authorID, seriesID int64) ([]organizeMove, []string, error) {
	if seriesID > 0 {
		return s.organize.PlanSeries(seriesID)
	}
	if authorID > 0 {
		return s.organize.PlanAuthor(authorID)
	}
	return s.organize.Plan(bookID)
}

// handleRenamePreview computes what organizing would move, without touching
// disk. ?bookId=N scopes to one book, ?authorId=N to one author, ?seriesId=N
// to one series.
func (s *server) handleRenamePreview(w http.ResponseWriter, r *http.Request) {
	bookID, ok := renameBookID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid bookId")
		return
	}
	authorID, ok := renameIDParam(r, "authorId")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid authorId")
		return
	}
	seriesID, ok := renameIDParam(r, "seriesId")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid seriesId")
		return
	}
	moves, skips, err := s.planRename(bookID, authorID, seriesID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, renameResponse{Moves: moves, Skips: skips})
}

// handleRenameApply plans and executes the moves. The body may scope with
// {"bookId": N}, {"authorId": N}, or {"seriesId": N}; empty organizes all.
func (s *server) handleRenameApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BookID   int64 `json:"bookId"`
		AuthorID int64 `json:"authorId"`
		SeriesID int64 `json:"seriesId"`
	}
	// Body is optional.
	_ = json.NewDecoder(r.Body).Decode(&req)

	moves, skips, err := s.planRename(req.BookID, req.AuthorID, req.SeriesID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	applied, applySkips, err := s.organize.Apply(moves)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, renameResponse{Moves: applied, Skips: append(skips, applySkips...)})
}

// adoptFile is the existing-file import core: assign an unmatched file to a
// book, enroll a prose book in the file's format library and monitor it (an
// adopted book behaves like one added by hand — upgrades can find it), then
// move the file into its template-defined place. A failed move keeps the
// match and reports why.
func (s *server) adoptFile(fileID, bookID int64) ([]string, error) {
	book, err := s.store.GetBook(bookID)
	if err != nil {
		return nil, err
	}
	file, err := s.store.GetBookFile(fileID)
	if err != nil {
		return nil, err
	}
	if err := s.store.SetBookFileBook(fileID, bookID); err != nil {
		return nil, err
	}
	if book.MediaType == "book" &&
		(file.MediaType == "ebook" || file.MediaType == "audiobook") {
		if err := s.store.SetBookLibrary(bookID, file.MediaType, true, true); err != nil {
			return nil, err
		}
	}

	skips := []string{}
	moves, planSkips, err := s.organize.Plan(bookID)
	if err == nil {
		scoped := []organize.Move{}
		for _, m := range moves {
			if m.FileID == fileID {
				scoped = append(scoped, m)
			}
		}
		_, applySkips, applyErr := s.organize.Apply(scoped)
		skips = append(planSkips, applySkips...)
		err = applyErr
	}
	return skips, err
}

// handleMatchBookFile is manual import: adopt an unmatched file for a book.
func (s *server) handleMatchBookFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		BookID int64 `json:"bookId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BookID <= 0 {
		writeError(w, http.StatusBadRequest, "bookId is required")
		return
	}
	skips, err := s.adoptFile(id, req.BookID)
	if err != nil {
		if err == library.ErrNotFound {
			writeStoreError(w, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	file, err := s.store.GetBookFile(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"file": file, "skips": skips})
}

// handleDeleteBookFile removes a file record (disk is never touched) —
// the "dismiss" action for junk in the unmatched list.
func (s *server) handleDeleteBookFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeleteBookFile(id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListBookFiles lists scanned files: ?bookId=N for one book's files,
// ?unmatched=true for files no book claimed, no filter for both is invalid —
// pick one, the full table has no UI use.
func (s *server) handleListBookFiles(w http.ResponseWriter, r *http.Request) {
	if v := r.URL.Query().Get("bookId"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid bookId")
			return
		}
		files, err := s.store.ListBookFiles(id)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, files)
		return
	}
	if r.URL.Query().Get("unmatched") == "true" {
		files, err := s.store.ListUnmatchedBookFiles()
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, files)
		return
	}
	writeError(w, http.StatusBadRequest, "specify bookId=N or unmatched=true")
}
