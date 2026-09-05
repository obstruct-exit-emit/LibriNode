package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/librinode/librinode/internal/config"
	"github.com/librinode/librinode/internal/scanner"
	"github.com/librinode/librinode/internal/tagwriter"
)

func (s *server) handleGetTagWriteSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.TagWriteSettingsValue())
}

func (s *server) handleSaveTagWriteSettings(w http.ResponseWriter, r *http.Request) {
	var t config.TagWriteSettings
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.cfg.SetTagWrite(t); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.cfg.TagWriteSettingsValue())
}

// handleWriteBookTags embeds LibriNode's metadata into a book's audiobook
// file(s) — the one action that mutates file contents, so it's explicit. clear
// (from the body) wipes unmanaged tags first; which fields are written comes
// from the per-field settings.
func (s *server) handleWriteBookTags(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Clear bool `json:"clear"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // body optional (defaults to merge)

	book, err := s.store.GetBook(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	author, err := s.store.GetAuthor(book.AuthorID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	files, err := s.store.ListBookFiles(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	toggles := s.cfg.TagWriteToggles()
	narrator := ""
	var paths []string
	for _, f := range files {
		if f.MediaType != "audiobook" {
			continue
		}
		if narrator == "" {
			narrator = f.Narrator
		}
		paths = append(paths, audioFilesUnder(f.Path)...)
	}
	if len(paths) == 0 {
		writeError(w, http.StatusBadRequest, "no audiobook files to tag")
		return
	}

	var cover []byte
	if toggles.CoverImage && book.CoverURL != "" {
		cover, _ = fetchImageBytes(r.Context(), book.CoverURL) // best-effort
	}

	tags := tagwriter.Tags{
		Title:      book.Title,
		Author:     author.Name,
		Album:      book.Title,
		Narrator:   narrator,
		Date:       book.ReleaseDate,
		CoverImage: cover,
	}

	written := 0
	errs := []string{} // never nil — JSON-encodes as [] so the client can read .length
	for _, p := range paths {
		if !tagwriter.IsSupported(p) {
			continue
		}
		if err := tagwriter.Write(p, tags, req.Clear, toggles); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", filepath.Base(p), err))
			continue
		}
		written++
	}
	writeJSON(w, http.StatusOK, map[string]any{"written": written, "errors": errs})
}

// audioFilesUnder returns the audio file at path, or every audio file within it
// when path is a folder (a multi-file audiobook).
func audioFilesUnder(path string) []string {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		return []string{path}
	}
	var out []string
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && scanner.IsAudioPath(p) {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func fetchImageBytes(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover fetch: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 15<<20))
}
