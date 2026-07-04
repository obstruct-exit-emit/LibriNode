package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/metadata"
)

func seriesMediaType(v string) (string, bool) {
	return v, v == "manga" || v == "comic"
}

// handleSearchSeries proxies series search to the media type's provider
// (reached through GET /api/v1/search?type=manga|comic).
func (s *server) handleSearchSeries(w http.ResponseWriter, r *http.Request, mediaType, term string) {
	p := s.metadata.SeriesFor(mediaType)
	if p == nil {
		writeError(w, http.StatusServiceUnavailable,
			"no "+mediaType+" metadata provider configured — ComicVine needs an API key under Settings")
		return
	}
	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	results, err := p.SearchSeries(ctx, term)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *server) handleListSeries(w http.ResponseWriter, r *http.Request) {
	mediaType := r.URL.Query().Get("mediaType")
	if mediaType != "" {
		if _, ok := seriesMediaType(mediaType); !ok {
			writeError(w, http.StatusBadRequest, "mediaType must be manga or comic")
			return
		}
	}
	out := []library.Series{}
	for _, mt := range []string{"manga", "comic"} {
		if mediaType != "" && mt != mediaType {
			continue
		}
		list, err := s.store.ListSeries(mt)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		out = append(out, list...)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAddSeries syncs a manga/comic series (with all volumes) from its
// provider.
func (s *server) handleAddSeries(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MediaType       string `json:"mediaType"`
		ForeignSeriesID string `json:"foreignSeriesId"`
		Monitored       *bool  `json:"monitored"`
		MonitorNew      *bool  `json:"monitorNew"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ForeignSeriesID == "" {
		writeError(w, http.StatusBadRequest, "foreignSeriesId is required")
		return
	}
	mediaType, ok := seriesMediaType(req.MediaType)
	if !ok {
		writeError(w, http.StatusBadRequest, "mediaType must be manga or comic")
		return
	}
	monitored := true
	if req.Monitored != nil {
		monitored = *req.Monitored
	}
	monitorNew := monitored
	if req.MonitorNew != nil {
		monitorNew = *req.MonitorNew
	}

	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	series, err := s.refresh.SyncSeries(ctx, mediaType, req.ForeignSeriesID, monitored, monitorNew, monitored)
	if err != nil {
		writeSeriesSyncError(w, err)
		return
	}
	s.rematchFiles()
	s.writeSeriesDetail(w, http.StatusCreated, series.ID)
}

func writeSeriesSyncError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, metadata.ErrNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "no metadata provider configured for this media type")
	case errors.Is(err, metadata.ErrNotFound):
		writeError(w, http.StatusNotFound, "series not found at metadata provider")
	case errors.Is(err, library.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	default:
		writeError(w, http.StatusBadGateway, err.Error())
	}
}

func (s *server) writeSeriesDetail(w http.ResponseWriter, status int, id int64) {
	series, err := s.store.GetSeries(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if series.Volumes, err = s.store.ListVolumes(id); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, status, series)
}

func (s *server) handleGetSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	s.writeSeriesDetail(w, http.StatusOK, id)
}

func (s *server) handleMonitorSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Monitored  bool `json:"monitored"`
		MonitorNew bool `json:"monitorNew"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.store.SetSeriesMonitored(id, req.Monitored, req.MonitorNew); err != nil {
		writeStoreError(w, err)
		return
	}
	s.writeSeriesDetail(w, http.StatusOK, id)
}

func (s *server) handleRefreshSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	if err := s.refresh.RefreshSeries(ctx, id); err != nil {
		writeSeriesSyncError(w, err)
		return
	}
	s.rematchFiles()
	s.writeSeriesDetail(w, http.StatusOK, id)
}

func (s *server) handleDeleteSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeleteSeries(id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
