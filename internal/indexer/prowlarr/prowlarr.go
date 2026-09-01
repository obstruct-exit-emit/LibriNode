// Package prowlarr is a native indexer source that searches a self-hosted
// Prowlarr instance (https://prowlarr.com) directly through its own
// GET /api/v1/search endpoint — the same call Prowlarr's own search page
// makes — instead of LibriNode pretending to be a Readarr application that
// Prowlarr pushes indexers into. One Prowlarr connection here fans out to
// every indexer configured *inside* Prowlarr; there's no per-indexer
// duplication in LibriNode's own settings, and no Readarr-shaped API surface
// for LibriNode to fake.
//
// A search result already carries a Prowlarr-proxied download link (routed
// back through Prowlarr, not the indexer directly). LibriNode's existing
// qBittorrent/SABnzbd clients already resolve a generic download URL into a
// magnet or the actual .torrent/.nzb bytes — the same way they do for every
// Newznab/Torznab indexer's results — so no separate content-fetching logic is
// needed here: a Prowlarr release rides the exact same grab path as any other
// indexer's, and gets scored against quality profiles the same way too
// (Prowlarr's own search has no such concept).
package prowlarr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/librinode/librinode/internal/indexer"
	"github.com/librinode/librinode/internal/redact"
)

// Def is the native-indexer definition; register it with
// indexer.RegisterNative at startup.
func Def() indexer.NativeDef {
	return indexer.NativeDef{
		Name:        "prowlarr",
		DisplayName: "Prowlarr",
		// A Prowlarr connection aggregates every indexer it has configured, so
		// it returns both torrent and usenet results — each release carries its
		// own real protocol (see release.toRelease); this marker is never used
		// to route a grab.
		Protocol:     indexer.ProtocolMixed,
		NeedsBaseURL: true,
		NeedsAPIKey:  true,
		New: func(ind *indexer.Indexer, httpc *http.Client) indexer.Searcher {
			return &searcher{ind: ind, httpc: httpc}
		},
	}
}

// categoriesFor scopes a search to a media type's Newznab category ids,
// mirroring the defaults every Newznab/Torznab indexer gets in
// internal/api/indexers.go (the Settings UI has no per-category field for
// native sources). Ebook 7000 (Books) + 7020 (EBook); audiobook 3030
// (Audio/Audiobook); manga/comic 7030 (Books/Comics); magazine 7010
// (Books/Mags).
func categoriesFor(mediaType string) string {
	switch mediaType {
	case "audiobook":
		return "3030"
	case "manga", "comic":
		return "7030"
	case "magazine":
		return "7010"
	default:
		return "7000,7020"
	}
}

type searcher struct {
	ind   *indexer.Indexer
	httpc *http.Client
}

func (s *searcher) baseURL() string {
	return strings.TrimRight(strings.TrimSpace(s.ind.BaseURL), "/")
}

// perSubIndexerTimeout bounds each individual sub-indexer request Search fans
// out to — comfortably longer than a healthy indexer's real response time,
// comfortably shorter than the caller's overall search timeout, so a slow
// sub-indexer misses its round instead of blocking the fast ones. A var, not
// a const, so a test can shrink it rather than actually sleeping.
var perSubIndexerTimeout = 20 * time.Second

// subIndexer is Prowlarr's own indexer-list entry (GET /api/v1/indexer) —
// only the fields Search's fan-out needs.
type subIndexer struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
}

func (s *searcher) listEnabledSubIndexers(ctx context.Context) ([]subIndexer, error) {
	body, err := s.get(ctx, "/api/v1/indexer")
	if err != nil {
		return nil, err
	}
	var all []subIndexer
	if err := json.Unmarshal(body, &all); err != nil {
		return nil, fmt.Errorf("prowlarr: parsing indexer list: %w", err)
	}
	enabled := make([]subIndexer, 0, len(all))
	for _, ind := range all {
		if ind.Enable {
			enabled = append(enabled, ind)
		}
	}
	return enabled, nil
}

// buildSearchURL renders the /api/v1/search query. indexerID, when > 0, scopes
// the search to that one Prowlarr sub-indexer (its indexerIds param) instead
// of every indexer Prowlarr has configured.
func buildSearchURL(query, categories string, indexerID int) string {
	q := url.Values{"query": {query}, "type": {"search"}}
	for _, cat := range strings.Split(categories, ",") {
		if cat = strings.TrimSpace(cat); cat != "" {
			q.Add("categories", cat)
		}
	}
	if indexerID > 0 {
		q.Set("indexerIds", strconv.Itoa(indexerID))
	}
	return "/api/v1/search?" + q.Encode()
}

// Search queries Prowlarr's own /api/v1/search, one sub-indexer at a time in
// parallel (each bounded by perSubIndexerTimeout) rather than Prowlarr's own
// aggregate call — which can't respond faster than its slowest sub-indexer,
// and in practice one scraped torrent site regularly takes far longer than the
// rest. A slow or already-resting (see backoff.go) sub-indexer contributes
// nothing to this round instead of holding up the others. Falls back to the
// single aggregate call if the sub-indexer list itself can't be fetched, so
// this is never worse than before it existed.
func (s *searcher) Search(ctx context.Context, query, mediaType string) ([]indexer.Release, error) {
	if s.baseURL() == "" {
		return nil, fmt.Errorf("prowlarr: a base URL (your Prowlarr instance's address) is required")
	}
	categories := categoriesFor(mediaType)

	subs, err := s.listEnabledSubIndexers(ctx)
	if err != nil || len(subs) == 0 {
		return s.searchAggregate(ctx, query, categories)
	}

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		releases = []indexer.Release{}
	)
	for _, sub := range subs {
		if subIndexerBackoff.resting(s.ind.ID, sub.ID) {
			continue
		}
		wg.Add(1)
		go func(sub subIndexer) {
			defer wg.Done()
			sctx, cancel := context.WithTimeout(ctx, perSubIndexerTimeout)
			defer cancel()
			body, err := s.get(sctx, buildSearchURL(query, categories, sub.ID))
			subIndexerBackoff.record(s.ind.ID, sub.ID, err)
			if err != nil {
				return // best-effort: one slow/failed sub-indexer doesn't sink the batch
			}
			found, err := parseReleases(body, s.ind)
			if err != nil {
				return
			}
			mu.Lock()
			releases = append(releases, found...)
			mu.Unlock()
		}(sub)
	}
	wg.Wait()
	return releases, nil
}

// searchAggregate is Search's pre-fan-out behavior: one call covering every
// indexer Prowlarr has configured. Used only when the sub-indexer list itself
// couldn't be fetched, so the feature is never worse than before it existed.
func (s *searcher) searchAggregate(ctx context.Context, query, categories string) ([]indexer.Release, error) {
	body, err := s.get(ctx, buildSearchURL(query, categories, 0))
	if err != nil {
		return nil, err
	}
	return parseReleases(body, s.ind)
}

func parseReleases(body []byte, ind *indexer.Indexer) ([]indexer.Release, error) {
	var results []release
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("prowlarr: parsing search response: %w", err)
	}
	releases := make([]indexer.Release, 0, len(results))
	for _, r := range results {
		rel := r.toRelease(ind)
		// Drop a result whose protocol didn't resolve to torrent or usenet.
		// A grab is routed to a download client by protocol, so a result with
		// no usable protocol can't be grabbed — and since the failed grab
		// records nothing, it wouldn't be blocklisted either, so autosearch
		// could re-pick and re-fail it every sweep. Prowlarr virtually always
		// sets a protocol; this only drops genuinely unusable results.
		if rel.Protocol != indexer.ProtocolTorrent && rel.Protocol != indexer.ProtocolUsenet {
			continue
		}
		releases = append(releases, rel)
	}
	return releases, nil
}

// Test verifies the connection against Prowlarr's own system-status endpoint —
// cheap, authenticated, and doesn't touch any indexer Prowlarr itself has
// configured (a search does, and could trip a real indexer's own rate limit
// just to prove connectivity).
func (s *searcher) Test(ctx context.Context) error {
	if s.baseURL() == "" {
		return fmt.Errorf("prowlarr: a base URL (your Prowlarr instance's address) is required")
	}
	_, err := s.get(ctx, "/api/v1/system/status")
	return err
}

func (s *searcher) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL()+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", s.ind.APIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LibriNode")

	resp, err := s.httpc.Do(req)
	if err != nil {
		return nil, redact.URLError(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, fmt.Errorf("prowlarr: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// The key rides in a header, not the URL, but redact it from the body
		// too in case a Prowlarr error page echoes the request.
		return nil, fmt.Errorf("prowlarr: HTTP %d: %.150s", resp.StatusCode,
			redact.Text(string(body), []string{s.ind.APIKey}))
	}
	return body, nil
}

// release is the subset of Prowlarr's own (much larger) ReleaseResource that
// LibriNode actually uses.
type release struct {
	GUID        string     `json:"guid"`
	Title       string     `json:"title"`
	Size        int64      `json:"size"`
	Indexer     string     `json:"indexer"`
	PublishDate time.Time  `json:"publishDate"`
	DownloadURL string     `json:"downloadUrl"`
	MagnetURL   string     `json:"magnetUrl"`
	InfoURL     string     `json:"infoUrl"`
	Protocol    protocol   `json:"protocol"`
	Seeders     *int       `json:"seeders"`
	Leechers    *int       `json:"leechers"`
	Categories  []category `json:"categories"`
}

type category struct {
	ID int `json:"id"`
}

// protocol tolerates Prowlarr's DownloadProtocol enum coming back as either a
// bare integer (the *arr family's C# enum: Usenet=1, Torrent=2) or its string
// name — observed to vary across Prowlarr versions/configurations. Anything
// else (including absent) decodes to "", handled the same as usenet by
// internal/release's scoring (a flat availability bonus, no seeder count
// expected).
type protocol string

func (p *protocol) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		switch strings.ToLower(s) {
		case "torrent":
			*p = protocol(indexer.ProtocolTorrent)
		case "usenet":
			*p = protocol(indexer.ProtocolUsenet)
		}
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		switch n {
		case 2:
			*p = protocol(indexer.ProtocolTorrent)
		case 1:
			*p = protocol(indexer.ProtocolUsenet)
		}
	}
	return nil
}

// toRelease maps a Prowlarr result onto LibriNode's own Release shape, which
// the rest of the pipeline (scoring, grab, download-client routing) already
// treats every indexer's results the same way.
func (r release) toRelease(ind *indexer.Indexer) indexer.Release {
	// MagnetURL, when Prowlarr includes it, is directly usable with no HTTP
	// fetch; DownloadURL still needs resolving (redirect-to-magnet, or the raw
	// .torrent/.nzb bytes) — the qBittorrent/SABnzbd clients already do exactly
	// that for every other indexer.
	downloadURL := r.MagnetURL
	if downloadURL == "" {
		downloadURL = r.DownloadURL
	}

	seeders, peers := -1, -1
	if r.Protocol == protocol(indexer.ProtocolTorrent) {
		if r.Seeders != nil {
			seeders = *r.Seeders
		}
		if r.Leechers != nil {
			peers = *r.Leechers
		}
	}

	cats := make([]int, 0, len(r.Categories))
	for _, c := range r.Categories {
		cats = append(cats, c.ID)
	}

	rel := indexer.Release{
		IndexerID: ind.ID,
		// Name which indexer inside Prowlarr actually answered — otherwise every
		// result would just say "Prowlarr" and be indistinguishable.
		Indexer:     ind.Name + " (" + r.Indexer + ")",
		Protocol:    string(r.Protocol),
		Title:       r.Title,
		GUID:        r.GUID,
		InfoURL:     r.InfoURL,
		DownloadURL: downloadURL,
		Size:        r.Size,
		Categories:  cats,
		Seeders:     seeders,
		Peers:       peers,
	}
	if !r.PublishDate.IsZero() {
		rel.PublishDate = r.PublishDate.UTC().Format(time.RFC3339)
	}
	return rel
}
