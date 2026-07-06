package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

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

// handleRenamePreview computes what organizing would move, without touching
// disk. ?bookId=N scopes to one book.
func (s *server) handleRenamePreview(w http.ResponseWriter, r *http.Request) {
	bookID, ok := renameBookID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid bookId")
		return
	}
	moves, skips, err := s.organize.Plan(bookID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, renameResponse{Moves: moves, Skips: skips})
}

// handleRenameApply plans and executes the moves. The body may scope with
// {"bookId": N}; empty body organizes everything.
func (s *server) handleRenameApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BookID int64 `json:"bookId"`
	}
	// Body is optional.
	_ = json.NewDecoder(r.Body).Decode(&req)

	moves, skips, err := s.organize.Plan(req.BookID)
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

// handleMatchBookFile is manual import: assign an unmatched file to a book,
// then move it into its template-defined place.
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
	if _, err := s.store.GetBook(req.BookID); err != nil {
		writeStoreError(w, err)
		return
	}
	if err := s.store.SetBookFileBook(id, req.BookID); err != nil {
		writeStoreError(w, err)
		return
	}

	// Move the file into place; a failed move keeps the match but reports why.
	skips := []string{}
	moves, planSkips, err := s.organize.Plan(req.BookID)
	if err == nil {
		scoped := []organize.Move{}
		for _, m := range moves {
			if m.FileID == id {
				scoped = append(scoped, m)
			}
		}
		_, applySkips, applyErr := s.organize.Apply(scoped)
		skips = append(planSkips, applySkips...)
		err = applyErr
	}
	if err != nil {
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
