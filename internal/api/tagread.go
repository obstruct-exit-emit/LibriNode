package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/librinode/librinode/internal/ebookmeta"
	"github.com/librinode/librinode/internal/tagreader"
)

// fileTagsResponse carries one file's embedded tags, discriminated by kind so
// the UI knows which field set to render (an audiobook's taglib tags vs an
// ebook's OPF/EXTH/Info-dict metadata).
type fileTagsResponse struct {
	Kind      string          `json:"kind"` // "audiobook" | "ebook"
	Audiobook *tagreader.Tags `json:"audiobook,omitempty"`
	Ebook     *ebookmeta.Tags `json:"ebook,omitempty"`
}

// handleGetBookFileTags reads one file's embedded tags fresh off disk (not the
// scan-time snapshot, which never refreshes after a "Write tags" call) — the
// trustworthy answer to "what is actually on this file now."
//
// A multi-file audiobook stores the folder as its path: with no ?track it reads
// the first audio file under the folder; with ?track=<folder-relative name> it
// reads that exact file. An ebook file is read directly, dispatched by format.
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
	} else if ebookmeta.IsEbookPath(path) {
		// read the ebook file directly (below)
	} else if paths := audioFilesUnder(path); len(paths) > 0 {
		path = paths[0]
	}

	if ebookmeta.IsEbookPath(path) {
		tags, err := ebookmeta.Read(path)
		if err != nil {
			writeError(w, http.StatusBadGateway, "reading tags: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, fileTagsResponse{Kind: "ebook", Ebook: tags})
		return
	}

	tags, err := tagreader.Read(path)
	if err != nil {
		writeError(w, http.StatusBadGateway, "reading tags: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, fileTagsResponse{Kind: "audiobook", Audiobook: tags})
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
