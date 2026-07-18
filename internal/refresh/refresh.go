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

// unreachableStreak aborts a refresh sweep after several consecutive
// "provider didn't respond" outcomes in a row, instead of letting a mid-sweep
// outage turn into every remaining record timing out one at a time — a
// library of a few hundred authors could otherwise turn a brief Hardcover
// outage into an hours-long stuck refresh. Any other outcome (success, or an
// error that isn't about connectivity) resets the streak, since that means
// the provider IS answering.
type unreachableStreak struct{ n int }

const unreachableAbortThreshold = 3

// hit records one call's outcome and reports whether the streak just crossed
// the abort threshold — the caller should stop the sweep.
func (u *unreachableStreak) hit(err error) bool {
	if err != nil && errors.Is(err, metadata.ErrUnreachable) {
		u.n++
		return u.n >= unreachableAbortThreshold
	}
	u.n = 0
	return false
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

// SyncAuthor fetches an author and their full bibliography from the active
// provider and persists everything. Books are stored as metadata only —
// library membership is never touched here, so refreshes can't enroll,
// un-enroll, or re-monitor anything; new books surface in the author's
// Missing section.
func (s *Service) SyncAuthor(ctx context.Context, foreignID string, monitored bool) (*library.Author, error) {
	p, err := s.provider()
	if err != nil {
		return nil, err
	}
	return s.syncAuthorWith(ctx, p, foreignID, monitored)
}

// syncAuthorWith is SyncAuthor through an explicit provider — the caller
// resolves it (the active provider on add, or the author's provider override
// on refresh).
func (s *Service) syncAuthorWith(ctx context.Context, p metadata.Provider, foreignID string, monitored bool) (*library.Author, error) {
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
		if err := s.persistBook(p, &remote.Books[i], author.ID, monitored); err != nil {
			return nil, err
		}
	}
	return author, nil
}

// SyncBook fetches one book (with editions and series) from the provider and
// persists it. The author is created as an unmonitored stub when not in the
// library yet — adding a single book must not pull in the whole bibliography.
// Library membership is the caller's job (handleAddBook enrolls explicitly).
func (s *Service) SyncBook(ctx context.Context, foreignID string, monitored bool) (*library.Book, error) {
	p, err := s.provider()
	if err != nil {
		return nil, err
	}
	return s.syncBookWith(ctx, p, foreignID, monitored)
}

// syncBookWith is SyncBook through an explicit provider — the caller
// resolves it (the active provider on add, or the author's provider override
// on refresh).
func (s *Service) syncBookWith(ctx context.Context, p metadata.Provider, foreignID string, monitored bool) (*library.Book, error) {
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

	if err := s.persistBook(p, remote, author.ID, monitored); err != nil {
		return nil, err
	}
	return s.store.GetBookByForeignID(source, remote.ForeignID)
}

// bookProviderFor resolves the provider for an author-scoped refresh: the
// author's provider override wins when set; otherwise the active provider —
// which is nil when Settings → Metadata says None, and libraries always
// honor the settings.
func (s *Service) bookProviderFor(author *library.Author) (metadata.Provider, error) {
	if author.ProviderOverride != "" {
		if p := s.providers.ProviderByName(author.ProviderOverride); p != nil {
			return p, nil
		}
		return nil, metadata.ErrNotConfigured
	}
	return s.provider()
}

// RefreshAuthor re-syncs an existing author by local id. Books discovered
// since the last sync are added with the author's monitored flag.
func (s *Service) RefreshAuthor(ctx context.Context, id int64) error {
	author, err := s.store.GetAuthor(id)
	if err != nil {
		return err
	}
	p, err := s.bookProviderFor(author)
	if err != nil {
		return err
	}
	_, err = s.syncAuthorWith(ctx, p, author.ForeignID, author.Monitored)
	return err
}

// RefreshBook re-syncs an existing book by local id, updating its metadata,
// series links, and editions. It follows the book's author's provider
// override.
func (s *Service) RefreshBook(ctx context.Context, id int64) error {
	book, err := s.store.GetBook(id)
	if err != nil {
		return err
	}
	author, err := s.store.GetAuthor(book.AuthorID)
	if err != nil {
		return err
	}
	p, err := s.bookProviderFor(author)
	if err != nil {
		return err
	}
	_, err = s.syncBookWith(ctx, p, book.ForeignID, book.Monitored)
	return err
}

// persistBook stores a provider book plus its series links and editions
// under the given author. Library membership columns are left at their
// defaults (new books) or preserved (existing books) — enrollment is always
// an explicit user action. (New ebook editions still inherit the book's
// monitored flag into the legacy editions.monitored column.)
func (s *Service) persistBook(p metadata.Provider, remote *metadata.Book, authorID int64, monitored bool) error {
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
// RefreshLibrary re-syncs one library's records from their providers: a
// format library's member authors (ebook/audiobook) or a series library's
// series (manga/comic) — the library-wide twin of the per-author/per-series
// Refresh buttons, honoring per-record provider overrides the same way.
// Individual failures are logged and skipped; the count of successfully
// refreshed records is returned.
func (s *Service) RefreshLibrary(ctx context.Context, mediaType string) (int, error) {
	done := 0
	switch mediaType {
	case "ebook", "audiobook":
		authors, err := s.store.ListAuthors()
		if err != nil {
			return 0, err
		}
		var unreachable unreachableStreak
		for i := range authors {
			a := &authors[i]
			if mediaType == "ebook" && !a.InEbookLibrary {
				continue
			}
			if mediaType == "audiobook" && !a.InAudiobookLibrary {
				continue
			}
			if ctx.Err() != nil {
				return done, ctx.Err()
			}
			err := s.RefreshAuthor(ctx, a.ID)
			if err != nil {
				slog.Warn("library refresh: author failed", "author", a.Name, "error", err)
			}
			if unreachable.hit(err) {
				slog.Warn("library refresh: provider unreachable, aborting the rest of this sweep",
					"mediaType", mediaType)
				return done, nil
			}
			if err == nil {
				done++
			}
		}
	case "manga", "comic":
		seriesList, err := s.store.ListSeries(mediaType)
		if err != nil {
			return 0, err
		}
		var unreachable unreachableStreak
		for i := range seriesList {
			if ctx.Err() != nil {
				return done, ctx.Err()
			}
			err := s.RefreshSeries(ctx, seriesList[i].ID)
			if err != nil {
				slog.Warn("library refresh: series failed", "series", seriesList[i].Title, "error", err)
			}
			if unreachable.hit(err) {
				slog.Warn("library refresh: provider unreachable, aborting the rest of this sweep",
					"mediaType", mediaType)
				return done, nil
			}
			if err == nil {
				done++
			}
		}
	default:
		return 0, fmt.Errorf("metadata refresh is not available for %s", mediaType)
	}
	slog.Info("library metadata refresh complete", "mediaType", mediaType, "refreshed", done)
	return done, nil
}

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
	var unreachable unreachableStreak
	for _, a := range authors {
		if ctx.Err() != nil {
			return
		}
		// Creator stubs from series providers aren't the book provider's to
		// refresh.
		if a.Source != bookProvider.Name() {
			continue
		}
		_, err := s.SyncAuthor(ctx, a.ForeignID, a.Monitored)
		if err != nil {
			slog.Warn("metadata refresh failed", "author", a.Name, "error", err)
		}
		if unreachable.hit(err) {
			slog.Warn("metadata refresh: provider unreachable, aborting the rest of this sweep",
				"provider", bookProvider.Name())
			return
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
