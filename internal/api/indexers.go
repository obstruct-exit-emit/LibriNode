package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/librinode/librinode/internal/indexer"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/release"
)

const indexerTestTimeout = 30 * time.Second

func writeIndexerError(w http.ResponseWriter, err error) {
	if errors.Is(err, indexer.ErrNotFound) {
		writeError(w, http.StatusNotFound, "indexer not found")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

// decodeIndexer reads and validates an indexer definition from the body.
// Two dialects arrive here: LibriNode's native flat JSON and the Readarr v1
// resource Prowlarr pushes (marked by an "implementation" key with fields[]).
func decodeIndexer(r *http.Request) (*indexer.Indexer, string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, "reading body"
	}

	var probe struct {
		Implementation string `json:"implementation"`
	}
	if json.Unmarshal(body, &probe) == nil && probe.Implementation != "" {
		var res arrIndexerResource
		if err := json.Unmarshal(body, &res); err != nil {
			return nil, "invalid JSON body"
		}
		in, err := res.toIndexer()
		if err != nil {
			return nil, err.Error()
		}
		if in.Name == "" {
			return nil, "name is required"
		}
		return in, ""
	}

	var in indexer.Indexer
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, "invalid JSON body"
	}
	in.Name = strings.TrimSpace(in.Name)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if in.Name == "" {
		return nil, "name is required"
	}
	if in.Type != indexer.TypeNewznab && in.Type != indexer.TypeTorznab {
		return nil, "type must be newznab or torznab"
	}
	if !strings.HasPrefix(in.BaseURL, "http://") && !strings.HasPrefix(in.BaseURL, "https://") {
		return nil, "baseUrl must be an http(s) URL"
	}
	if in.Categories == "" {
		in.Categories = "7000,7020"
	}
	if in.AudioCategories == "" {
		in.AudioCategories = "3030"
	}
	if in.ComicCategories == "" {
		in.ComicCategories = "7030"
	}
	if in.MagazineCategories == "" {
		in.MagazineCategories = "7010"
	}
	if in.Priority <= 0 || in.Priority > 50 {
		in.Priority = 25
	}
	return &in, ""
}

func (s *server) handleListIndexers(w http.ResponseWriter, r *http.Request) {
	indexers, err := s.indexers.Store().List()
	if err != nil {
		writeIndexerError(w, err)
		return
	}
	resources := make([]map[string]any, 0, len(indexers))
	for i := range indexers {
		resources = append(resources, mergedIndexerResource(&indexers[i]))
	}
	writeJSON(w, http.StatusOK, resources)
}

func (s *server) handleGetIndexer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ind, err := s.indexers.Store().Get(id)
	if err != nil {
		writeIndexerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mergedIndexerResource(ind))
}

func (s *server) handleAddIndexer(w http.ResponseWriter, r *http.Request) {
	in, msg := decodeIndexer(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.indexers.Store().Add(in); err != nil {
		writeError(w, http.StatusConflict, "could not save indexer (duplicate name?): "+err.Error())
		return
	}
	s.refreshHealth()
	writeJSON(w, http.StatusCreated, mergedIndexerResource(in))
}

func (s *server) handleUpdateIndexer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	in, msg := decodeIndexer(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	in.ID = id
	if err := s.indexers.Store().Update(in); err != nil {
		writeIndexerError(w, err)
		return
	}
	updated, err := s.indexers.Store().Get(id)
	if err != nil {
		writeIndexerError(w, err)
		return
	}
	s.refreshHealth()
	writeJSON(w, http.StatusOK, mergedIndexerResource(updated))
}

func (s *server) handleDeleteIndexer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.indexers.Store().Delete(id); err != nil {
		writeIndexerError(w, err)
		return
	}
	s.refreshHealth()
	w.WriteHeader(http.StatusNoContent)
}

// handleTestIndexer checks an unsaved indexer definition against the live
// endpoint (fetches its capabilities).
func (s *server) handleTestIndexer(w http.ResponseWriter, r *http.Request) {
	in, msg := decodeIndexer(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), indexerTestTimeout)
	defer cancel()

	if err := s.indexers.Client().Test(ctx, in); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleSearchReleases searches every enabled indexer and returns parsed,
// scored candidates. Two modes: ?term= is a free search (generic scoring
// only), ?bookId=N builds the query from the book and rejects releases that
// aren't it — the interactive-search backend. Per-indexer failures come back
// in "errors" alongside the results that did arrive.
func (s *server) handleSearchReleases(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")

	mediaType := r.URL.Query().Get("mediaType")
	if mediaType == "" {
		mediaType = "ebook"
	}

	var book *library.Book
	var author *library.Author
	var seriesTitle string
	var volumeNumber float64
	if v := r.URL.Query().Get("bookId"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid bookId")
			return
		}
		if book, err = s.store.GetBook(id); err != nil {
			writeStoreError(w, err)
			return
		}
		if book.MediaType == "manga" || book.MediaType == "comic" {
			// Volumes dictate their own media type and search by series.
			mediaType = book.MediaType
			links, err := s.store.ListSeriesForBook(book.ID)
			if err != nil || len(links) == 0 {
				writeError(w, http.StatusInternalServerError, "volume has no series link")
				return
			}
			seriesTitle, volumeNumber = links[0].Title, links[0].Position
			if term == "" {
				term = seriesTitle
			}
		} else {
			if author, err = s.store.GetAuthor(book.AuthorID); err != nil {
				writeStoreError(w, err)
				return
			}
			if term == "" {
				term = author.Name + " " + book.Title
			}
		}
	}
	if term == "" {
		writeError(w, http.StatusBadRequest, "term or bookId is required")
		return
	}
	switch mediaType {
	case "ebook", "audiobook", "manga", "comic":
	case "magazine":
		writeError(w, http.StatusBadRequest,
			"magazine acquisition is disabled — the magazine library is organize-only for now")
		return
	default:
		writeError(w, http.StatusBadRequest, "mediaType must be ebook, audiobook, manga, or comic")
		return
	}

	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	found, errs, err := s.indexers.SearchAll(ctx, term, mediaType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	prefs := release.PreferencesFor(s.store, mediaType)
	candidates := make([]release.Candidate, 0, len(found))
	for _, rel := range found {
		if seriesTitle != "" {
			candidates = append(candidates, release.ScoreVolume(rel, prefs, seriesTitle, volumeNumber))
		} else {
			candidates = append(candidates, release.Score(rel, prefs, book, author))
		}
	}
	release.Rank(candidates)
	writeJSON(w, http.StatusOK, map[string]any{"releases": candidates, "errors": errs})
}

// handleSearchSeriesPacks serves the series-level pack search:
// GET /api/v1/release/packs?seriesId=N. Candidates are whole-series /
// multi-volume releases; the UI grabs one through the normal grab endpoint
// using the returned grabBookId, and the pack importer files every matching
// volume when the download lands.
func (s *server) handleSearchSeriesPacks(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("seriesId"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "seriesId is required")
		return
	}

	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	result, err := s.search.SearchSeriesPacks(ctx, id)
	if err != nil {
		if errors.Is(err, library.ErrNotFound) {
			writeStoreError(w, err)
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
