package api

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// scans can walk large libraries on slow disks; generous but bounded.
const scanTimeout = 10 * time.Minute

// handleScan walks all ebook root folders synchronously and reports what it
// found. Fine for Phase 1 library sizes; a queued background command system
// arrives with the acquisition pipeline.
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
