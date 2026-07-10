package refresh

import (
	"context"
	"fmt"
	"strings"

	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/metadata"
	"github.com/librinode/librinode/internal/scanner"
)

// SyncSeries fetches a manga/comic series from its media type's provider and
// persists it with volume/issue books. Existing volumes keep their monitored
// flags (upsert semantics); newly discovered ones get newVolumesMonitored —
// the caller passes the add-time choice or, on refresh, the series'
// monitor_new flag ("monitor future volumes").
func (s *Service) SyncSeries(ctx context.Context, mediaType, foreignID string, monitored, monitorNew, newVolumesMonitored bool) (*library.Series, error) {
	p := s.providers.SeriesFor(mediaType)
	if p == nil {
		return nil, metadata.ErrNotConfigured
	}
	return s.syncSeriesWith(ctx, p, mediaType, foreignID, monitored, monitorNew, newVolumesMonitored)
}

// syncSeriesWith persists a series using the given provider — the caller picks
// it (the media type's active provider when adding, or the series' own source
// provider when refreshing, so a manga added from AniList keeps refreshing
// from AniList even after the manga provider is switched to Hardcover).
func (s *Service) syncSeriesWith(ctx context.Context, p metadata.SeriesProvider, mediaType, foreignID string, monitored, monitorNew, newVolumesMonitored bool) (*library.Series, error) {
	remote, err := p.GetSeries(ctx, foreignID)
	if err != nil {
		return nil, err
	}
	source := p.Name()

	// The creator gets an author stub (volumes need an author row); keyed by
	// name so one mangaka's series share it.
	authorName := remote.AuthorName
	if authorName == "" {
		authorName = "Unknown Creator"
	}
	author, err := s.store.GetAuthorByForeignID(source, "creator:"+scanner.Normalize(authorName))
	if err != nil {
		author = &library.Author{
			Source:    source,
			ForeignID: "creator:" + scanner.Normalize(authorName),
			Name:      authorName,
			Monitored: false,
		}
		if err := s.store.UpsertAuthor(author); err != nil {
			return nil, err
		}
	}

	series := &library.Series{
		Source:      source,
		ForeignID:   remote.ForeignID,
		Title:       remote.Title,
		Description: remote.Description,
		MediaType:   mediaType,
		Monitored:   monitored,
		MonitorNew:  monitorNew,
		CoverURL:    remote.CoverURL,
	}
	if err := s.store.UpsertSeries(series); err != nil {
		return nil, err
	}

	numberPrefix := "Vol. "
	if mediaType == "comic" {
		numberPrefix = "#"
	}
	for _, issue := range remote.Issues {
		volMonitored := newVolumesMonitored
		if existing, err := s.store.GetBookByForeignID(source, issue.ForeignID); err == nil {
			volMonitored = existing.Monitored // preserved by upsert anyway
		}
		// Use the volume's own description. Comics rarely carry a per-issue
		// synopsis, so the issue name stands in there; manga volumes are left
		// blank when the provider has none (AniList synthesizes volumes with
		// no descriptions) rather than repeating the series blurb on every one.
		description := issue.Description
		if description == "" && mediaType == "comic" {
			description = issue.Title
		}
		coverURL := issue.CoverURL
		if coverURL == "" {
			coverURL = remote.CoverURL
		}
		book := &library.Book{
			AuthorID:    author.ID,
			Source:      source,
			MediaType:   mediaType,
			ForeignID:   issue.ForeignID,
			Title:       fmt.Sprintf("%s %s%v", remote.Title, numberPrefix, trimFloat(issue.Number)),
			Description: description,
			ReleaseDate: issue.ReleaseDate,
			CoverURL:    coverURL,
			Monitored:   volMonitored,
		}
		if err := s.store.UpsertBook(book); err != nil {
			return nil, err
		}
		if err := s.store.LinkBookSeries(book.ID, series.ID, issue.Number); err != nil {
			return nil, err
		}
	}
	return series, nil
}

// trimFloat renders 5 as "5" and 5.5 as "5.5".
func trimFloat(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}

// RefreshSeries re-syncs an existing series; new volumes follow monitor_new.
// Manual series (magazines) have no provider to refresh — their issues come
// from grabs and scans — so they're a quiet no-op.
//
// A refresh always honors the provider currently selected for the media type:
// switch manga from AniList to Hardcover, hit refresh, and the series re-binds
// to Hardcover (re-matched by title). It only falls back to the series' own
// source provider when the selected one isn't configured.
func (s *Service) RefreshSeries(ctx context.Context, id int64) error {
	series, err := s.store.GetSeries(id)
	if err != nil {
		return err
	}
	if series.Source == "manual" {
		return nil
	}
	active := s.providers.SeriesFor(series.MediaType)
	if active == nil {
		active = s.providers.SeriesProviderByName(series.Source)
	}
	if active == nil {
		return nil // no provider configured — leave the series as-is
	}
	if active.Name() == series.Source {
		_, err = s.syncSeriesWith(ctx, active, series.MediaType, series.ForeignID,
			series.Monitored, series.MonitorNew, series.MonitorNew)
		return err
	}
	// The selected provider differs from the one this series was added with;
	// re-match it on the new provider and re-bind in place.
	return s.rematchSeries(ctx, series, active)
}

// rematchSeries moves a series to a different provider: it searches the active
// provider for the series title, re-binds the row to the best match (keeping
// the local id and monitoring), re-syncs volumes from the new provider, and
// drops the previous provider's volume books (owned volumes are kept so their
// files survive). If nothing matches, the series is left untouched rather than
// emptied.
func (s *Service) rematchSeries(ctx context.Context, series *library.Series, p metadata.SeriesProvider) error {
	results, err := p.SearchSeries(ctx, series.Title)
	if err != nil {
		return err
	}
	match := pickSeriesMatch(results, series.Title)
	if match == nil {
		return nil // no counterpart on the new provider — keep the old data
	}
	remote, err := p.GetSeries(ctx, match.ForeignID)
	if err != nil {
		return err
	}

	oldVolumes, err := s.store.ListVolumes(series.ID)
	if err != nil {
		return err
	}
	oldPositions, err := s.store.SeriesBookPositions(series.ID)
	if err != nil {
		return err
	}

	if err := s.store.RebindSeries(series.ID, p.Name(), remote.ForeignID,
		remote.Title, remote.Description, remote.CoverURL); err != nil {
		return err
	}
	series.Source = p.Name()
	series.ForeignID = remote.ForeignID

	if _, err := s.syncSeriesWith(ctx, p, series.MediaType, remote.ForeignID,
		series.Monitored, series.MonitorNew, series.MonitorNew); err != nil {
		return err
	}

	// Index the new provider's volumes by number so an owned old volume can
	// hand its files to the same-numbered new volume.
	newByNumber := map[float64]int64{}
	for _, iss := range remote.Issues {
		if b, err := s.store.GetBookByForeignID(p.Name(), iss.ForeignID); err == nil {
			newByNumber[iss.Number] = b.ID
		}
	}

	// Retire the previous provider's volume books. Owned volumes migrate their
	// files (and monitored flag) onto the matching new volume before deletion;
	// an owned volume with no counterpart is kept so its file isn't lost. Prose
	// books (media_type "book") that merely reference the series are left be.
	for _, v := range oldVolumes {
		if v.MediaType == "book" || v.Source == p.Name() {
			continue
		}
		hasFiles, err := s.store.BookHasFiles(v.ID)
		if err != nil {
			return err
		}
		if hasFiles {
			newID, ok := newByNumber[oldPositions[v.ID]]
			if !ok {
				continue // no same-numbered new volume — keep the owned one
			}
			if err := s.store.MoveBookFiles(v.ID, newID); err != nil {
				return err
			}
			if err := s.store.SetBookMonitored(newID, v.Monitored); err != nil {
				return err
			}
		}
		if err := s.store.DeleteBook(v.ID); err != nil {
			return err
		}
	}
	return nil
}

// pickSeriesMatch chooses the provider result that best corresponds to title:
// an exact case-insensitive title match wins; otherwise the first result, since
// providers return search hits by relevance.
func pickSeriesMatch(results []metadata.SeriesResult, title string) *metadata.SeriesResult {
	if len(results) == 0 {
		return nil
	}
	want := strings.ToLower(strings.TrimSpace(title))
	for i := range results {
		if strings.ToLower(strings.TrimSpace(results[i].Title)) == want {
			return &results[i]
		}
	}
	return &results[0]
}

// refreshAllSeries re-syncs every manga/comic series (called from RefreshAll).
func (s *Service) refreshAllSeries(ctx context.Context) {
	for _, mediaType := range []string{"manga", "comic"} {
		seriesList, err := s.store.ListSeries(mediaType)
		if err != nil {
			continue
		}
		for _, sr := range seriesList {
			if ctx.Err() != nil {
				return
			}
			if err := s.RefreshSeries(ctx, sr.ID); err != nil {
				continue // one dead record can't stall the rest
			}
		}
	}
}
