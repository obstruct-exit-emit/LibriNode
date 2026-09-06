package api

import (
	"net/http"

	"github.com/librinode/librinode/internal/tagreader"
)

// handleGetBookFileTags reads one audiobook file's embedded tags fresh off
// disk (not the scan-time snapshot, which never refreshes after a "Write tags"
// call) — the trustworthy answer to "what is actually on this file now." A
// multi-file audiobook stores the folder as its path, so the first audio file
// under it stands in for the unit.
func (s *server) handleGetBookFileTags(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	f, err := s.store.GetBookFile(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	path := f.Path
	if paths := audioFilesUnder(path); len(paths) > 0 {
		path = paths[0]
	}
	tags, err := tagreader.Read(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "reading tags: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tags)
}
