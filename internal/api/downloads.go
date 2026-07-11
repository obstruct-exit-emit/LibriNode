package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/librinode/librinode/internal/download"
)

const downloadTimeout = 60 * time.Second

func writeDownloadError(w http.ResponseWriter, err error) {
	if errors.Is(err, download.ErrNotFound) {
		writeError(w, http.StatusNotFound, "download client not found")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

// decodeDownloadClient reads and validates a client config from the body.
func decodeDownloadClient(r *http.Request) (*download.ClientConfig, string) {
	var c download.ClientConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		return nil, "invalid JSON body"
	}
	c.Name = strings.TrimSpace(c.Name)
	c.Host = strings.TrimRight(strings.TrimSpace(c.Host), "/")
	if c.Name == "" {
		return nil, "name is required"
	}
	if c.Type != download.TypeQBittorrent && c.Type != download.TypeSABnzbd {
		return nil, "type must be qbittorrent or sabnzbd"
	}
	if !strings.HasPrefix(c.Host, "http://") && !strings.HasPrefix(c.Host, "https://") {
		return nil, "host must be an http(s) URL"
	}
	// A SABnzbd API key is optional: SABnzbd-compatible endpoints such as
	// Real-Debrid's (which downloads NZBs behind a fake-SABnzbd interface)
	// need no key. Real SABnzbd will reject unauthenticated calls, which the
	// connection Test surfaces — so we let it be entered without one.
	if c.Category == "" {
		c.Category = "librinode"
	}
	if c.Priority <= 0 || c.Priority > 50 {
		c.Priority = 1
	}
	return &c, ""
}

func (s *server) handleListDownloadClients(w http.ResponseWriter, r *http.Request) {
	configs, err := s.downloads.Store().List()
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configs)
}

func (s *server) handleAddDownloadClient(w http.ResponseWriter, r *http.Request) {
	c, msg := decodeDownloadClient(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.downloads.Store().Add(c); err != nil {
		writeError(w, http.StatusConflict, "could not save client (duplicate name?): "+err.Error())
		return
	}
	s.refreshHealth()
	writeJSON(w, http.StatusCreated, c)
}

func (s *server) handleUpdateDownloadClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, msg := decodeDownloadClient(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	c.ID = id
	if err := s.downloads.Store().Update(c); err != nil {
		writeDownloadError(w, err)
		return
	}
	updated, err := s.downloads.Store().Get(id)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	s.refreshHealth()
	writeJSON(w, http.StatusOK, updated)
}

func (s *server) handleDeleteDownloadClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.downloads.Store().Delete(id); err != nil {
		writeDownloadError(w, err)
		return
	}
	s.refreshHealth()
	w.WriteHeader(http.StatusNoContent)
}

// handleTestDownloadClient checks an unsaved client config against the live
// service.
func (s *server) handleTestDownloadClient(w http.ResponseWriter, r *http.Request) {
	c, msg := decodeDownloadClient(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	client, err := download.New(c)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), downloadTimeout)
	defer cancel()

	if err := client.Test(ctx); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleGrabRelease sends a release to the matching download client — the
// button behind interactive search results. bookId ties the download to a
// book so Completed Download Handling can import it automatically.
func (s *server) handleGrabRelease(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string `json:"title"`
		DownloadURL string `json:"downloadUrl"`
		GUID        string `json:"guid"`
		Protocol    string `json:"protocol"`
		BookID      int64  `json:"bookId"`
		MediaType   string `json:"mediaType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DownloadURL == "" {
		writeError(w, http.StatusBadRequest, "downloadUrl is required")
		return
	}
	if req.Protocol != download.ProtocolTorrent && req.Protocol != download.ProtocolUsenet {
		writeError(w, http.StatusBadRequest, "protocol must be torrent or usenet")
		return
	}
	if req.MediaType == "" {
		req.MediaType = "ebook"
	}
	if req.BookID > 0 {
		if _, err := s.store.GetBook(req.BookID); err != nil {
			writeStoreError(w, err)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), downloadTimeout)
	defer cancel()

	result, grab, err := s.downloads.GrabRelease(ctx, req.Protocol, req.DownloadURL, req.Title, req.GUID, req.BookID, req.MediaType)
	if errors.Is(err, download.ErrNoClient) {
		writeError(w, http.StatusServiceUnavailable,
			"no enabled "+req.Protocol+" download client — add one under Settings")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"client": result.Client, "id": result.ID, "grabId": grab.ID,
	})
}

// handleAutoSearchBook searches indexers for one book and grabs the best
// approved release automatically.
func (s *server) handleAutoSearchBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	outcome, err := s.search.SearchBook(ctx, id, r.URL.Query().Get("mediaType"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, outcome)
}

// handleSearchWanted runs the automatic search over every wanted book now
// (the background loop does the same on a schedule).
func (s *server) handleSearchWanted(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	outcomes, err := s.search.SearchWanted(ctx)
	if err != nil && len(outcomes) == 0 {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	grabbed := 0
	for _, o := range outcomes {
		if o.Grabbed {
			grabbed++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"searched": len(outcomes), "grabbed": grabbed, "outcomes": outcomes,
	})
}

// handleImport runs one Completed Download Handling pass on demand.
func (s *server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), downloadTimeout)
	defer cancel()

	result, err := s.importer.Run(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleHistory lists grab records, newest first (?status= filters).
func (s *server) handleHistory(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	switch status {
	case "", download.GrabStatusGrabbed, download.GrabStatusImported, download.GrabStatusFailed:
	default:
		writeError(w, http.StatusBadRequest, "status must be grabbed, imported, or failed")
		return
	}
	grabs, err := s.downloads.Store().ListGrabs(status)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, grabs)
}

// handleBlocklist lists releases blocked after failed downloads.
func (s *server) handleBlocklist(w http.ResponseWriter, r *http.Request) {
	entries, err := s.downloads.Store().ListBlocklist()
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// handleUnblock removes one blocklist entry so the release can be grabbed
// again.
func (s *server) handleUnblock(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.downloads.Store().DeleteBlock(id); err != nil {
		writeDownloadError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleQueue shows every LibriNode download across all enabled clients.
func (s *server) handleQueue(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), downloadTimeout)
	defer cancel()

	items, errs, err := s.downloads.Queue(ctx)
	if err != nil {
		writeDownloadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "errors": errs})
}
