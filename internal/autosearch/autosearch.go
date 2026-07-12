// Package autosearch closes the *arr loop: it finds monitored books without
// files, searches the indexers, and grabs the best-scoring approved release —
// on a schedule, for the whole wanted list, or on demand for one book.
// Completed Download Handling then imports whatever the client finishes.
package autosearch

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
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
	MediaType string `json:"mediaType"`
	Grabbed   bool   `json:"grabbed"`
	Release   string `json:"release,omitempty"`
	Client    string `json:"client,omitempty"`
	Message   string `json:"message,omitempty"`
}

// SearchBook searches for one book as the given media type and grabs the
// best approved release. Never returns an error for "nothing found" —
// that's an outcome. When the book is already owned in that format and its
// profile allows upgrades, the search runs in upgrade mode: only strictly
// better formats approve.
func (s *Service) SearchBook(ctx context.Context, bookID int64, mediaType string) (*BookOutcome, error) {
	if mediaType == "" {
		mediaType = "ebook"
	}
	book, err := s.store.GetBook(bookID)
	if err != nil {
		return nil, err
	}
	// Volumes dictate their own media type.
	if book.MediaType == "manga" || book.MediaType == "comic" {
		mediaType = book.MediaType
	}
	if book.MediaType == "magazine" {
		return &BookOutcome{BookID: bookID, BookTitle: book.Title, MediaType: "magazine",
			Message: "magazine issues are searched at the series level"}, nil
	}
	// Prose searches require library membership — the Plex-style model:
	// a book is only acquirable as a format it was added as.
	if book.MediaType == "book" {
		member := book.InEbookLibrary
		if mediaType == "audiobook" {
			member = book.InAudiobookLibrary
		}
		if !member {
			return &BookOutcome{BookID: bookID, BookTitle: book.Title, MediaType: mediaType,
				Message: "not in the " + mediaType + " library — add it there first"}, nil
		}
	}

	if pending, err := s.pendingBookIDs(); err != nil {
		return nil, err
	} else if pending[pendingKey(bookID, mediaType)] {
		return &BookOutcome{BookID: bookID, BookTitle: book.Title, MediaType: mediaType,
			Message: "a grab is already pending for this book"}, nil
	}

	minScore := 0
	if s.ownedIn(book, mediaType) {
		min, upgradable := s.upgradeMinScore(book, mediaType)
		if !upgradable {
			return &BookOutcome{BookID: bookID, BookTitle: book.Title, MediaType: mediaType,
				Message: "already owned — enable upgrades in the quality profile to search for better formats"}, nil
		}
		minScore = min
	}
	return s.searchOne(ctx, book, mediaType, minScore)
}

// ownedIn reports whether the book has a file of the given media type.
func (s *Service) ownedIn(book *library.Book, mediaType string) bool {
	switch mediaType {
	case "ebook":
		return book.HasEbookFile
	case "audiobook":
		return book.HasAudiobookFile
	}
	return book.HasFile
}

// upgradeMinScore computes upgrade mode for an owned book: the owned
// format's score (candidates must beat it), and whether an upgrade is
// wanted at all (profile allows upgrades and the owned format is below the
// cutoff — empty cutoff means the profile's best format).
func (s *Service) upgradeMinScore(book *library.Book, mediaType string) (int, bool) {
	profile, err := s.store.DefaultProfile(mediaType)
	if err != nil || !profile.UpgradesAllowed || len(profile.Formats) == 0 {
		return 0, false
	}
	prefs := release.PreferencesFromProfile(*profile)

	cutoff := profile.Cutoff
	if cutoff == "" {
		cutoff = profile.Formats[0]
	}
	cutoffScore := prefs.FormatScores[cutoff]

	files, err := s.store.ListBookFiles(book.ID)
	if err != nil {
		return 0, false
	}
	owned := 0
	seen := false
	for _, f := range files {
		if f.MediaType != mediaType {
			continue
		}
		seen = true
		if sc, ok := prefs.FormatScores[f.Format]; ok && sc > owned {
			owned = sc
		}
	}
	if !seen {
		return 0, false
	}
	if owned == 0 {
		// The owned format isn't in the profile at all — anything listed
		// is an upgrade.
		return 1, true
	}
	if owned >= cutoffScore {
		return 0, false // cutoff met; done upgrading
	}
	return owned, true
}

func (s *Service) searchOne(ctx context.Context, book *library.Book, mediaType string, minFormatScore int) (*BookOutcome, error) {
	outcome := &BookOutcome{BookID: book.ID, BookTitle: book.Title, MediaType: mediaType}
	prefs := release.PreferencesFor(s.store, mediaType)
	prefs.MinFormatScore = minFormatScore

	var query string
	var score func(indexer.Release) release.Candidate

	if mediaType == "manga" || mediaType == "comic" {
		// Volumes are searched by series title; the volume number filters
		// candidates during scoring.
		links, err := s.store.ListSeriesForBook(book.ID)
		if err != nil {
			return nil, err
		}
		if len(links) == 0 {
			outcome.Message = "volume has no series link"
			return outcome, nil
		}
		seriesTitle, number := links[0].Title, links[0].Position
		query = seriesTitle
		score = func(rel indexer.Release) release.Candidate {
			return release.ScoreVolume(rel, prefs, seriesTitle, number)
		}
	} else {
		author, err := s.store.GetAuthor(book.AuthorID)
		if err != nil {
			return nil, err
		}
		query = author.Name + " " + book.Title
		if mediaType == "audiobook" {
			// Categories do most of the filtering; the keyword helps
			// indexers with sloppy category mapping.
			query += " audiobook"
		}
		score = func(rel indexer.Release) release.Candidate {
			return release.Score(rel, prefs, book, author)
		}
	}

	found, indexerErrs, err := s.indexers.SearchAll(ctx, query, mediaType)
	if err != nil {
		return nil, err
	}

	candidates := make([]release.Candidate, 0, len(found))
	for _, rel := range found {
		candidates = append(candidates, score(rel))
	}
	s.markBlocked(candidates)
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

	result, _, err := s.downloads.GrabRelease(ctx, best.Protocol, best.DownloadURL, best.Title, best.GUID, book.ID, mediaType)
	if err != nil {
		outcome.Message = "grab failed: " + err.Error()
		return outcome, nil
	}
	outcome.Grabbed = true
	outcome.Release = best.Title
	outcome.Client = result.Client
	slog.Info("auto-grabbed release",
		"book", book.Title, "mediaType", mediaType, "release", best.Title, "client", result.Client)
	return outcome, nil
}

// markBlocked rejects candidates on the failed-release blocklist.
func (s *Service) markBlocked(candidates []release.Candidate) {
	blocked, err := s.downloads.Store().BlockedKeys()
	if err != nil || len(blocked) == 0 {
		return
	}
	for i := range candidates {
		c := &candidates[i]
		if download.IsBlocked(blocked, c.GUID, c.Title) {
			c.Approved = false
			c.Rejections = append(c.Rejections, "blocklisted (failed to download previously)")
		}
	}
}

func pendingKey(bookID int64, mediaType string) string {
	return fmt.Sprintf("%d/%s", bookID, mediaType)
}

// pendingBookIDs is the set of book/media-type pairs that already have an
// unresolved grab — searching those again would double-download.
func (s *Service) pendingBookIDs() (map[string]bool, error) {
	grabs, err := s.downloads.Store().ListGrabs(download.GrabStatusGrabbed)
	if err != nil {
		return nil, err
	}
	pending := map[string]bool{}
	for _, g := range grabs {
		if g.BookID > 0 {
			pending[pendingKey(g.BookID, g.MediaType)] = true
		}
	}
	return pending, nil
}

// wants lists the media types a book should be searched as: ebooks whenever
// monitored and fileless; audiobooks additionally require a monitored
// audiobook edition (the per-book opt-in). Owned formats stay wanted in
// upgrade mode (minScore > 0) while the profile allows upgrades and the
// owned format is below the cutoff.
type want struct {
	mediaType string
	minScore  int
}

func (s *Service) wants(book *library.Book) []want {
	// Magazine issues are acquired at the series level (searchMagazine),
	// never through per-book search.
	if book.MediaType == "magazine" {
		return nil
	}
	// Manga volumes / comic issues want exactly their own type (no upgrade
	// mode for volumes yet); the classic monitored flag governs them.
	if book.MediaType == "manga" || book.MediaType == "comic" {
		if !book.Monitored || book.HasFile {
			return nil
		}
		return []want{{mediaType: book.MediaType}}
	}
	// Prose books: each format library membership carries its own monitored
	// flag (Plex-style explicit membership).
	wants := []want{}
	if book.InEbookLibrary && book.EbookMonitored {
		if !book.HasEbookFile {
			wants = append(wants, want{mediaType: "ebook"})
		} else if min, ok := s.upgradeMinScore(book, "ebook"); ok {
			wants = append(wants, want{mediaType: "ebook", minScore: min})
		}
	}
	if book.InAudiobookLibrary && book.AudiobookMonitored {
		if !book.HasAudiobookFile {
			wants = append(wants, want{mediaType: "audiobook"})
		} else if min, ok := s.upgradeMinScore(book, "audiobook"); ok {
			wants = append(wants, want{mediaType: "audiobook", minScore: min})
		}
	}
	return wants
}

// SearchMagazineSeries searches one magazine for new issues and grabs them —
// the per-series Search button's magazine path. Magazine issues are
// materialized on grab, so there are no per-volume books to sweep; this drives
// searchMagazine directly.
func (s *Service) SearchMagazineSeries(ctx context.Context, series *library.Series) ([]BookOutcome, error) {
	return s.searchMagazine(ctx, series)
}

// searchMagazine looks for new issues of a monitored magazine: any release
// matching the title whose issue date/number isn't in the library yet. Found
// issues are materialized as books and grabbed (capped per pass so a fresh
// magazine doesn't flood the download client).
func (s *Service) searchMagazine(ctx context.Context, series *library.Series) ([]BookOutcome, error) {
	prefs := release.PreferencesFor(s.store, "magazine")

	// Everything already in the library — by identifier — is not wanted.
	volumes, err := s.store.ListVolumes(series.ID)
	if err != nil {
		return nil, err
	}
	owned := map[string]bool{}
	for _, v := range volumes {
		if id := strings.TrimPrefix(v.ForeignID, series.ForeignID+":"); id != v.ForeignID {
			owned[id] = true
		}
	}

	found, _, err := s.indexers.SearchAll(ctx, series.Title, "magazine")
	if err != nil {
		return nil, err
	}

	// Best approved candidate per unowned issue identifier.
	scored := make([]release.Candidate, 0, len(found))
	identifiersByIdx := make([]string, 0, len(found))
	for _, rel := range found {
		cand, identifier := release.ScoreMagazine(rel, prefs, series.Title, owned)
		scored = append(scored, cand)
		identifiersByIdx = append(identifiersByIdx, identifier)
	}
	s.markBlocked(scored)

	best := map[string]release.Candidate{}
	for i, cand := range scored {
		identifier := identifiersByIdx[i]
		if !cand.Approved || identifier == "" {
			continue
		}
		if cur, ok := best[identifier]; !ok || cand.Score > cur.Score {
			best[identifier] = cand
		}
	}

	identifiers := make([]string, 0, len(best))
	for id := range best {
		identifiers = append(identifiers, id)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(identifiers))) // newest dates first
	const maxGrabsPerPass = 3
	if len(identifiers) > maxGrabsPerPass {
		identifiers = identifiers[:maxGrabsPerPass]
	}

	outcomes := []BookOutcome{}
	for _, identifier := range identifiers {
		cand := best[identifier]
		book, err := s.store.CreateMagazineIssue(series, identifier, true)
		if err != nil {
			return outcomes, err
		}
		outcome := BookOutcome{BookID: book.ID, BookTitle: book.Title, MediaType: "magazine"}
		result, _, err := s.downloads.GrabRelease(ctx, cand.Protocol, cand.DownloadURL, cand.Title, cand.GUID, book.ID, "magazine")
		if err != nil {
			outcome.Message = "grab failed: " + err.Error()
		} else {
			outcome.Grabbed = true
			outcome.Release = cand.Title
			outcome.Client = result.Client
			slog.Info("auto-grabbed magazine issue", "magazine", series.Title, "issue", identifier, "client", result.Client)
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

// SearchWanted searches every monitored book missing a wanted format and
// every monitored magazine for new issues, politely pacing indexer traffic.
// Pending grabs are skipped per media type.
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

	magazines, err := s.store.ListSeries("magazine")
	if err != nil {
		return nil, err
	}
	for i := range magazines {
		if !magazines[i].Monitored {
			continue
		}
		if ctx.Err() != nil {
			return outcomes, ctx.Err()
		}
		found, err := s.searchMagazine(ctx, &magazines[i])
		if err != nil {
			outcomes = append(outcomes, BookOutcome{
				BookTitle: magazines[i].Title, MediaType: "magazine", Message: err.Error(),
			})
			continue
		}
		outcomes = append(outcomes, found...)
	}
	for i := range books {
		book := &books[i]
		for _, w := range s.wants(book) {
			if pending[pendingKey(book.ID, w.mediaType)] {
				continue
			}
			if ctx.Err() != nil {
				return outcomes, ctx.Err()
			}
			outcome, err := s.searchOne(ctx, book, w.mediaType, w.minScore)
			if err != nil {
				outcomes = append(outcomes, BookOutcome{
					BookID: book.ID, BookTitle: book.Title, MediaType: w.mediaType, Message: err.Error(),
				})
				continue
			}
			outcomes = append(outcomes, *outcome)

			// Pace between searches so a big wanted list doesn't hammer indexers.
			select {
			case <-ctx.Done():
				return outcomes, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
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
