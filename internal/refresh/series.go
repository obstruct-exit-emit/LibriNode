package refresh

import (
	"context"
	"fmt"

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
		// Prefer the volume's own description; fall back to the series
		// description for context rather than the volume title (which just
		// repeats the "Vol. N" line). Comics keep the issue name, which reads
		// as a reasonable per-issue blurb.
		description := issue.Description
		if description == "" {
			if mediaType == "comic" {
				description = issue.Title
			} else {
				description = remote.Description
			}
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
func (s *Service) RefreshSeries(ctx context.Context, id int64) error {
	series, err := s.store.GetSeries(id)
	if err != nil {
		return err
	}
	if series.Source == "manual" {
		return nil
	}
	// Refresh through the provider that created the series, not whatever is
	// currently selected for the media type — their ids aren't interchangeable.
	p := s.providers.SeriesProviderByName(series.Source)
	if p == nil {
		return nil // source provider not configured — leave the series as-is
	}
	_, err = s.syncSeriesWith(ctx, p, series.MediaType, series.ForeignID,
		series.Monitored, series.MonitorNew, series.MonitorNew)
	return err
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
