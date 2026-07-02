package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/quillarr/quillarr/internal/library"
	"github.com/quillarr/quillarr/internal/metadata"
)

const metadataTimeout = 60 * time.Second

func (s *server) metadataCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), metadataTimeout)
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// writeStoreError maps store errors to HTTP responses.
func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, library.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

// --- Search (metadata provider proxy) ---

func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if s.metadata == nil {
		writeError(w, http.StatusServiceUnavailable,
			"no metadata provider configured — set hardcover_token in config.yaml or QUILLARR_HARDCOVER_TOKEN")
		return
	}
	term := r.URL.Query().Get("term")
	if term == "" {
		writeError(w, http.StatusBadRequest, "term is required")
		return
	}
	kind := r.URL.Query().Get("type")
	if kind == "" {
		kind = "book"
	}

	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	switch kind {
	case "author":
		results, err := s.metadata.SearchAuthors(ctx, term)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, results)
	case "book":
		results, err := s.metadata.SearchBooks(ctx, term)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, results)
	default:
		writeError(w, http.StatusBadRequest, "type must be author or book")
	}
}

// --- Authors ---

func (s *server) handleListAuthors(w http.ResponseWriter, r *http.Request) {
	authors, err := s.store.ListAuthors()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, authors)
}

// handleAddAuthor fetches an author from the metadata provider and persists
// them with all their books (monitored by default). Editions are pulled in
// when a specific book is added or on a later metadata refresh.
func (s *server) handleAddAuthor(w http.ResponseWriter, r *http.Request) {
	if s.metadata == nil {
		writeError(w, http.StatusServiceUnavailable, "no metadata provider configured")
		return
	}
	var req struct {
		ForeignAuthorID string `json:"foreignAuthorId"`
		Monitored       *bool  `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ForeignAuthorID == "" {
		writeError(w, http.StatusBadRequest, "foreignAuthorId is required")
		return
	}
	monitored := true
	if req.Monitored != nil {
		monitored = *req.Monitored
	}

	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	remote, err := s.metadata.GetAuthor(ctx, req.ForeignAuthorID)
	if errors.Is(err, metadata.ErrNotFound) {
		writeError(w, http.StatusNotFound, "author not found at metadata provider")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	author := &library.Author{
		Source:      s.metadata.Name(),
		ForeignID:   remote.ForeignID,
		Name:        remote.Name,
		Description: remote.Description,
		ImageURL:    remote.ImageURL,
		Monitored:   monitored,
	}
	if err := s.store.UpsertAuthor(author); err != nil {
		writeStoreError(w, err)
		return
	}
	for i := range remote.Books {
		if err := s.persistBook(&remote.Books[i], author.ID, monitored); err != nil {
			writeStoreError(w, err)
			return
		}
	}

	s.writeAuthorDetail(w, http.StatusCreated, author.ID)
}

func (s *server) handleGetAuthor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	s.writeAuthorDetail(w, http.StatusOK, id)
}

func (s *server) writeAuthorDetail(w http.ResponseWriter, status int, id int64) {
	author, err := s.store.GetAuthor(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	books, err := s.store.ListBooks(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	author.Books = books
	writeJSON(w, status, author)
}

func (s *server) handleMonitorAuthor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Monitored bool `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.store.SetAuthorMonitored(id, req.Monitored); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "monitored": req.Monitored})
}

func (s *server) handleDeleteAuthor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeleteAuthor(id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Books ---

func (s *server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	var authorID int64
	if v := r.URL.Query().Get("authorId"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid authorId")
			return
		}
		authorID = id
	}
	books, err := s.store.ListBooks(authorID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, books)
}

// handleAddBook fetches one book (with editions) from the metadata provider
// and persists it. The author is created as an unmonitored stub when not in
// the library yet — adding a single book must not pull in the whole
// bibliography. Ebook editions start monitored (Phase 1 is ebook-first);
// audiobook monitoring arrives in Phase 3.
func (s *server) handleAddBook(w http.ResponseWriter, r *http.Request) {
	if s.metadata == nil {
		writeError(w, http.StatusServiceUnavailable, "no metadata provider configured")
		return
	}
	var req struct {
		ForeignBookID string `json:"foreignBookId"`
		Monitored     *bool  `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ForeignBookID == "" {
		writeError(w, http.StatusBadRequest, "foreignBookId is required")
		return
	}
	monitored := true
	if req.Monitored != nil {
		monitored = *req.Monitored
	}

	ctx, cancel := s.metadataCtx(r)
	defer cancel()

	remote, err := s.metadata.GetBook(ctx, req.ForeignBookID)
	if errors.Is(err, metadata.ErrNotFound) {
		writeError(w, http.StatusNotFound, "book not found at metadata provider")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if remote.AuthorForeignID == "" {
		writeError(w, http.StatusBadGateway, "metadata provider returned a book without an author")
		return
	}

	source := s.metadata.Name()
	author, err := s.store.GetAuthorByForeignID(source, remote.AuthorForeignID)
	if errors.Is(err, library.ErrNotFound) {
		author = &library.Author{
			Source:    source,
			ForeignID: remote.AuthorForeignID,
			Name:      remote.AuthorName,
			Monitored: false,
		}
		err = s.store.UpsertAuthor(author)
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}

	if err := s.persistBook(remote, author.ID, monitored); err != nil {
		writeStoreError(w, err)
		return
	}
	book, err := s.store.GetBookByForeignID(source, remote.ForeignID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.writeBookDetail(w, http.StatusCreated, book.ID)
}

// persistBook stores a provider book plus its series links and editions
// under the given author. Ebook editions inherit the book's monitored flag.
func (s *server) persistBook(remote *metadata.Book, authorID int64, monitored bool) error {
	source := s.metadata.Name()
	book := &library.Book{
		AuthorID:    authorID,
		Source:      source,
		ForeignID:   remote.ForeignID,
		Title:       remote.Title,
		Description: remote.Description,
		ReleaseDate: remote.ReleaseDate,
		Rating:      remote.Rating,
		CoverURL:    remote.CoverURL,
		Monitored:   monitored,
	}
	if err := s.store.UpsertBook(book); err != nil {
		return err
	}
	for _, sl := range remote.Series {
		series := &library.Series{
			Source:      source,
			ForeignID:   sl.ForeignID,
			Title:       sl.Title,
			Description: sl.Description,
		}
		if err := s.store.UpsertSeries(series); err != nil {
			return err
		}
		if err := s.store.LinkBookSeries(book.ID, series.ID, sl.Position); err != nil {
			return err
		}
	}
	for _, ed := range remote.Editions {
		edition := &library.Edition{
			BookID:      book.ID,
			Source:      source,
			ForeignID:   ed.ForeignID,
			Title:       ed.Title,
			ISBN13:      ed.ISBN13,
			ASIN:        ed.ASIN,
			Format:      ed.Format,
			Publisher:   ed.Publisher,
			Language:    ed.Language,
			ReleaseDate: ed.ReleaseDate,
			CoverURL:    ed.CoverURL,
			Monitored:   monitored && ed.Format == library.FormatEbook,
		}
		if err := s.store.UpsertEdition(edition); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	s.writeBookDetail(w, http.StatusOK, id)
}

func (s *server) writeBookDetail(w http.ResponseWriter, status int, id int64) {
	book, err := s.store.GetBook(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if book.Editions, err = s.store.ListEditions(id); err != nil {
		writeStoreError(w, err)
		return
	}
	if book.Series, err = s.store.ListSeriesForBook(id); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, status, book)
}

func (s *server) handleMonitorBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Monitored bool `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.store.SetBookMonitored(id, req.Monitored); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "monitored": req.Monitored})
}

func (s *server) handleDeleteBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.DeleteBook(id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Editions ---

func (s *server) handleMonitorEdition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Monitored bool `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.store.SetEditionMonitored(id, req.Monitored); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "monitored": req.Monitored})
}
