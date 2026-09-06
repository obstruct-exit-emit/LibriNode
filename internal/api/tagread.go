package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/librinode/librinode/internal/tagreader"
)

// handleGetBookFileTags reads one audiobook file's embedded tags fresh off
// disk (not the scan-time snapshot, which never refreshes after a "Write tags"
// call) — the trustworthy answer to "what is actually on this file now."
//
// A multi-file audiobook stores the folder as its path: with no ?track it
// reads the first audio file under the folder (the unit's stand-in); with
// ?track=<folder-relative name> (one of the unit's listed tracks) it reads
// that exact file.
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
	if track := r.URL.Query().Get("track"); track != "" {
		p, ok := trackFilePath(f.Path, track)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid track")
			return
		}
		path = p
	} else if paths := audioFilesUnder(path); len(paths) > 0 {
		path = paths[0]
	}
	tags, err := tagreader.Read(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "reading tags: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

// trackFilePath resolves a folder-relative track name (as listed by
// fillAudioTracks, forward-slashed) to an absolute path under base, refusing
// anything that would escape the folder.
func trackFilePath(base, track string) (string, bool) {
	full := filepath.Join(base, filepath.FromSlash(track))
	rel, err := filepath.Rel(base, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(track) {
		return "", false
	}
	return full, true
}
