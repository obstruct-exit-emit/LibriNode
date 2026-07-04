package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"time"

	"github.com/librinode/librinode/internal/config"
	"github.com/librinode/librinode/internal/metadata"
	"github.com/librinode/librinode/internal/naming"
)

// metadataSettingsResponse is the settings UI's view of metadata config:
// which providers exist, which one is active, and their stored settings.
type metadataSettingsResponse struct {
	Active          string                       `json:"active"`
	Available       []string                     `json:"available"`
	SeriesAvailable []string                     `json:"seriesAvailable"`
	Providers       map[string]metadata.Settings `json:"providers"`
}

func (s *server) metadataSettingsResponse() metadataSettingsResponse {
	ms := s.cfg.MetadataSettings()
	resp := metadataSettingsResponse{
		Active:          ms.Active,
		Available:       metadata.Available(),
		SeriesAvailable: metadata.SeriesAvailable(),
		Providers:       ms.Providers,
	}
	// Every registered provider shows up in the form, configured or not.
	for _, name := range append(append([]string{}, resp.Available...), resp.SeriesAvailable...) {
		if _, ok := resp.Providers[name]; !ok {
			resp.Providers[name] = metadata.Settings{}
		}
	}
	return resp
}

func (s *server) handleGetMetadataSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.metadataSettingsResponse())
}

// handlePutMetadataSettings saves provider settings, persists them to
// config.yaml, and hot-swaps the active provider — no restart needed.
func (s *server) handlePutMetadataSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Active    string                       `json:"active"`
		Providers map[string]metadata.Settings `json:"providers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Active != "" && !slices.Contains(metadata.Available(), req.Active) {
		writeError(w, http.StatusBadRequest, "unknown provider: "+req.Active)
		return
	}
	if req.Providers == nil {
		req.Providers = map[string]metadata.Settings{}
	}

	ms := config.MetadataSettings{Active: req.Active, Providers: req.Providers}
	if err := s.metadata.Configure(ms.Active, ms.Providers); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.metadata.ConfigureSeries(ms.Providers)
	if err := s.cfg.SetMetadata(ms); err != nil {
		writeError(w, http.StatusInternalServerError, "saving config: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.metadataSettingsResponse())
}

// --- Naming settings ---

// exampleTokenData renders template previews with a recognizable book.
var exampleTokenData = naming.TokenData{
	AuthorName:     "Terry Pratchett",
	AuthorSortName: "Pratchett, Terry",
	BookTitle:      "The Colour of Magic",
	SeriesTitle:    "Discworld",
	SeriesPosition: 1,
	ReleaseYear:    "1983",
}

type namingSettingsResponse struct {
	config.NamingSettings
	Tokens           []string `json:"tokens"`
	Example          string   `json:"example"`
	AudiobookExample string   `json:"audiobookExample"`
}

func namingResponse(ns config.NamingSettings) namingSettingsResponse {
	audiobookDir := naming.Format(ns.AudiobookFile, exampleTokenData)
	return namingSettingsResponse{
		NamingSettings: ns,
		Tokens:         naming.Tokens,
		Example: filepath.ToSlash(filepath.Join(
			naming.Format(ns.EbookFolder, exampleTokenData),
			naming.Format(ns.EbookFile, exampleTokenData)+".epub",
		)),
		AudiobookExample: filepath.ToSlash(filepath.Join(
			naming.Format(ns.AudiobookFolder, exampleTokenData),
			audiobookDir,
			audiobookDir+".m4b",
		)),
	}
}

func (s *server) handleGetNamingSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, namingResponse(s.cfg.NamingSettings()))
}

func (s *server) handlePutNamingSettings(w http.ResponseWriter, r *http.Request) {
	var req config.NamingSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.EbookFolder == "" || req.EbookFile == "" {
		writeError(w, http.StatusBadRequest, "ebookFolder and ebookFile are required")
		return
	}
	// Audiobook templates keep their defaults when omitted (older clients).
	if req.AudiobookFolder == "" {
		req.AudiobookFolder = "{Author Name}"
	}
	if req.AudiobookFile == "" {
		req.AudiobookFile = "{Book Title}"
	}
	if err := s.cfg.SetNaming(req); err != nil {
		writeError(w, http.StatusInternalServerError, "saving config: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, namingResponse(req))
}

// handleTestMetadataProvider builds a provider from the submitted (unsaved)
// settings and checks it against the live API.
func (s *server) handleTestMetadataProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string            `json:"provider"`
		Settings metadata.Settings `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}

	p, err := metadata.Build(req.Provider, req.Settings)
	if errors.Is(err, metadata.ErrNotConfigured) {
		writeError(w, http.StatusBadRequest, "provider is not configured — enter a token first")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if v, ok := p.(metadata.Validator); ok {
		err = v.Validate(ctx)
	} else {
		// No cheap validation call — a tiny search exercises auth instead.
		_, err = p.SearchBooks(ctx, "test")
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": req.Provider})
}
