// Package autosearch closes the *arr loop: it finds monitored books without
// files, searches the indexers, and grabs the best-scoring approved release —
// on a schedule, for the whole wanted list, or on demand for one book.
// Completed Download Handling then imports whatever the client finishes.
package autosearch

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/librinode/librinode/internal/download"
	"github.com/librinode/librinode/internal/indexer"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/release"
)

type Service struct {
	store     *library.Store
	indexers  *indexer.Service
	downloads *download.Service
}

func New(store *library.Store, indexers *indexer.Service, downloads *download.Service) *Service {
	return &Service{store: store, indexers: indexers, downloads: downloads}
}

// BookOutcome reports what one book's automatic search did.
type BookOutcome struct {
	BookID    int64  `json:"bookId"`
	BookTitle string `json:"bookTitle"`
	Grabbed   bool   `json:"grabbed"`
	Release   string `json:"release,omitempty"`
	Client    string `json:"client,omitempty"`
	Message   string `json:"message,omitempty"`
}

// SearchBook searches for one book and grabs the best approved release.
// Never returns an error for "nothing found" — that's an outcome.
func (s *Service) SearchBook(ctx context.Context, bookID int64) (*BookOutcome, error) {
	book, err := s.store.GetBook(bookID)
	if err != nil {
		return nil, err
	}
	if pending, err := s.pendingBookIDs(); err != nil {
		return nil, err
	} else if pending[bookID] {
		return &BookOutcome{BookID: bookID, BookTitle: book.Title,
			Message: "a grab is already pending for this book"}, nil
	}
	return s.searchOne(ctx, book)
}

func (s *Service) searchOne(ctx context.Context, book *library.Book) (*BookOutcome, error) {
	outcome := &BookOutcome{BookID: book.ID, BookTitle: book.Title}

	author, err := s.store.GetAuthor(book.AuthorID)
	if err != nil {
		return nil, err
	}

	found, indexerErrs, err := s.indexers.SearchAll(ctx, author.Name+" "+book.Title)
	if err != nil {
		return nil, err
	}

	prefs := release.PreferencesForEbook(s.store)
	candidates := make([]release.Candidate, 0, len(found))
	for _, rel := range found {
		candidates = append(candidates, release.Score(rel, prefs, book, author))
	}
	release.Rank(candidates)

	var best *release.Candidate
	for i := range candidates {
		if candidates[i].Approved {
			best = &candidates[i]
			break
		}
	}
	if best == nil {
		outcome.Message = fmt.Sprintf("no approved release among %d candidates", len(candidates))
		if len(indexerErrs) > 0 {
			outcome.Message += " (" + strings.Join(indexerErrs, "; ") + ")"
		}
		return outcome, nil
	}

	result, _, err := s.downloads.GrabRelease(ctx, best.Protocol, best.DownloadURL, best.Title, book.ID)
	if err != nil {
		outcome.Message = "grab failed: " + err.Error()
		return outcome, nil
	}
	outcome.Grabbed = true
	outcome.Release = best.Title
	outcome.Client = result.Client
	slog.Info("auto-grabbed release", "book", book.Title, "release", best.Title, "client", result.Client)
	return outcome, nil
}

// pendingBookIDs is the set of books that already have an unresolved grab —
// searching those again would double-download.
func (s *Service) pendingBookIDs() (map[int64]bool, error) {
	grabs, err := s.downloads.Store().ListGrabs(download.GrabStatusGrabbed)
	if err != nil {
		return nil, err
	}
	pending := map[int64]bool{}
	for _, g := range grabs {
		if g.BookID > 0 {
			pending[g.BookID] = true
		}
	}
	return pending, nil
}

// SearchWanted searches every monitored book without a file, politely pacing
// indexer traffic. Books with pending grabs are skipped.
func (s *Service) SearchWanted(ctx context.Context) ([]BookOutcome, error) {
	books, err := s.store.ListBooks(0)
	if err != nil {
		return nil, err
	}
	pending, err := s.pendingBookIDs()
	if err != nil {
		return nil, err
	}

	outcomes := []BookOutcome{}
	for i := range books {
		book := &books[i]
		if !book.Monitored || book.HasFile || pending[book.ID] {
			continue
		}
		if ctx.Err() != nil {
			return outcomes, ctx.Err()
		}
		outcome, err := s.searchOne(ctx, book)
		if err != nil {
			outcomes = append(outcomes, BookOutcome{
				BookID: book.ID, BookTitle: book.Title, Message: err.Error(),
			})
			continue
		}
		outcomes = append(outcomes, *outcome)

		// Pace between books so a big wanted list doesn't hammer indexers.
		select {
		case <-ctx.Done():
			return outcomes, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}

	grabbed := 0
	for _, o := range outcomes {
		if o.Grabbed {
			grabbed++
		}
	}
	if len(outcomes) > 0 {
		slog.Info("wanted search complete", "searched", len(outcomes), "grabbed", grabbed)
	}
	return outcomes, nil
}

// RunPeriodic searches the wanted list on the interval until ctx is
// cancelled. It quietly does nothing when no indexers are enabled.
func (s *Service) RunPeriodic(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			enabled, err := s.indexers.Store().ListEnabled()
			if err != nil || len(enabled) == 0 {
				continue
			}
			if _, err := s.SearchWanted(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("wanted search failed", "error", err)
			}
		}
	}
}
