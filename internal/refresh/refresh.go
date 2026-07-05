// Package refresh syncs library records with the metadata provider: adding
// authors/books pulls them in, manual refresh re-fetches them, and a periodic
// loop keeps the whole library current. Store upserts preserve user-owned
// monitored flags, so refreshing never undoes monitoring choices.
package refresh

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/metadata"
)

type Service struct {
	store     *library.Store
	providers *metadata.Manager
}

func New(store *library.Store, providers *metadata.Manager) *Service {
	return &Service{store: store, providers: providers}
}

// provider returns the active metadata provider, or ErrNotConfigured. Looked
// up per call so a provider configured in the settings UI takes effect
// immediately.
func (s *Service) provider() (metadata.Provider, error) {
	if p := s.providers.Current(); p != nil {
		return p, nil
	}
	return nil, metadata.ErrNotConfigured
}

// SyncAuthor fetches an author and their full bibliography from the provider
// and persists everything. New books inherit the monitored flag; existing
// rows keep theirs. Returns the local author (without books).
// targetLibrary ("ebook"/"audiobook") is the format library new books are
// enrolled in — the area the user added from.
func (s *Service) SyncAuthor(ctx context.Context, foreignID string, monitored bool, targetLibrary string) (*library.Author, error) {
	p, err := s.provider()
	if err != nil {
		return nil, err
	}
	remote, err := p.GetAuthor(ctx, foreignID)
	if err != nil {
		return nil, err
	}

	author := &library.Author{
		Source:      p.Name(),
		ForeignID:   remote.ForeignID,
		Name:        remote.Name,
		Description: remote.Description,
		ImageURL:    remote.ImageURL,
		Monitored:   monitored,
	}
	if err := s.store.UpsertAuthor(author); err != nil {
		return nil, err
	}
	for i := range remote.Books {
		if err := s.persistBook(p, &remote.Books[i], author.ID, monitored, targetLibrary); err != nil {
			return nil, err
		}
	}
	return author, nil
}

// SyncBook fetches one book (with editions and series) from the provider and
// persists it. The author is created as an unmonitored stub when not in the
// library yet — adding a single book must not pull in the whole bibliography.
func (s *Service) SyncBook(ctx context.Context, foreignID string, monitored bool, targetLibrary string) (*library.Book, error) {
	p, err := s.provider()
	if err != nil {
		return nil, err
	}
	remote, err := p.GetBook(ctx, foreignID)
	if err != nil {
		return nil, err
	}
	if remote.AuthorForeignID == "" {
		return nil, fmt.Errorf("provider returned book %s without an author", foreignID)
	}

	source := p.Name()
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
		return nil, err
	}

	if err := s.persistBook(p, remote, author.ID, monitored, targetLibrary); err != nil {
		return nil, err
	}
	return s.store.GetBookByForeignID(source, remote.ForeignID)
}

// RefreshAuthor re-syncs an existing author by local id. Books discovered
// since the last sync are added with the author's monitored flag.
func (s *Service) RefreshAuthor(ctx context.Context, id int64) error {
	if _, err := s.provider(); err != nil {
		return err
	}
	author, err := s.store.GetAuthor(id)
	if err != nil {
		return err
	}
	_, err = s.SyncAuthor(ctx, author.ForeignID, author.Monitored, "ebook")
	return err
}

// RefreshBook re-syncs an existing book by local id, updating its metadata,
// series links, and editions.
func (s *Service) RefreshBook(ctx context.Context, id int64) error {
	if _, err := s.provider(); err != nil {
		return err
	}
	book, err := s.store.GetBook(id)
	if err != nil {
		return err
	}
	_, err = s.SyncBook(ctx, book.ForeignID, book.Monitored, "ebook")
	return err
}

// persistBook stores a provider book plus its series links and editions
// under the given author. New ebook editions inherit the book's monitored
// flag (Phase 1 is ebook-first; audiobook monitoring arrives in Phase 3).
func (s *Service) persistBook(p metadata.Provider, remote *metadata.Book, authorID int64, monitored bool, targetLibrary string) error {
	source := p.Name()
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
	switch targetLibrary {
	case "audiobook":
		book.InAudiobookLibrary = true
		book.AudiobookMonitored = monitored
	default:
		book.InEbookLibrary = true
		book.EbookMonitored = monitored
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

// RefreshAll re-syncs every author and manga/comic series in the library.
// Individual failures are logged and skipped so one dead provider record
// can't stall the rest.
func (s *Service) RefreshAll(ctx context.Context) {
	s.refreshAllSeries(ctx)
	if _, err := s.provider(); err != nil {
		return
	}
	authors, err := s.store.ListAuthors()
	if err != nil {
		slog.Error("metadata refresh: listing authors", "error", err)
		return
	}
	bookProvider, _ := s.provider()
	for _, a := range authors {
		if ctx.Err() != nil {
			return
		}
		// Creator stubs from series providers aren't the book provider's to
		// refresh.
		if a.Source != bookProvider.Name() {
			continue
		}
		if _, err := s.SyncAuthor(ctx, a.ForeignID, a.Monitored, "ebook"); err != nil {
			slog.Warn("metadata refresh failed", "author", a.Name, "error", err)
			continue
		}
	}
	if len(authors) > 0 {
		slog.Info("metadata refresh complete", "authors", len(authors))
	}
}

// RunPeriodic refreshes the whole library on the given interval until ctx is
// cancelled. The first run happens after one interval, not at startup.
func (s *Service) RunPeriodic(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RefreshAll(ctx)
		}
	}
}
