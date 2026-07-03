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
	if c.Type == download.TypeSABnzbd && c.APIKey == "" {
		return nil, "apiKey is required for SABnzbd"
	}
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
// button behind interactive search results.
func (s *server) handleGrabRelease(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string `json:"title"`
		DownloadURL string `json:"downloadUrl"`
		Protocol    string `json:"protocol"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DownloadURL == "" {
		writeError(w, http.StatusBadRequest, "downloadUrl is required")
		return
	}
	if req.Protocol != download.ProtocolTorrent && req.Protocol != download.ProtocolUsenet {
		writeError(w, http.StatusBadRequest, "protocol must be torrent or usenet")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), downloadTimeout)
	defer cancel()

	result, err := s.downloads.Grab(ctx, req.Protocol, req.DownloadURL, req.Title)
	if errors.Is(err, download.ErrNoClient) {
		writeError(w, http.StatusServiceUnavailable,
			"no enabled "+req.Protocol+" download client — add one under Settings")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
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
