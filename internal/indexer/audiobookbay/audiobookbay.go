// Package audiobookbay is a native indexer for AudioBook Bay (ABB): an
// audiobook torrent site with no Newznab/Torznab API, so Prowlarr can't reach
// it. ABB never publishes a .torrent or a ready magnet — a release page carries
// the info hash and a tracker list, and the magnet is assembled from them here
// (the exact step that breaks a Prowlarr→Jackett definition). The result is an
// ordinary torrent Release that rides LibriNode's existing qBittorrent path.
//
// This is a dual-use shadow-library source: it is never bundled or enabled by
// default; a user adds it deliberately and is responsible for its use. HTML
// selectors target ABB's known layout and may need updating if the site
// changes — the inherent fragility of a scraped source.
package audiobookbay

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/librinode/librinode/internal/indexer"
)

const (
	// Name is both the registry key and the stored indexer type.
	Name = "audiobookbay"
	// DefaultBaseURL is ABB's domain at time of writing; it rotates, so the
	// user can override it on the indexer (Settings → Indexers → Site URL).
	DefaultBaseURL = "https://audiobookbay.lu"

	maxDetails  = 12                     // detail pages fetched per search (each is a request)
	detailPause = 200 * time.Millisecond // politeness delay between detail fetches
	// A real browser UA — ABB sits behind anti-bot filtering that rejects
	// obvious scrapers; this is best-effort, not a guarantee.
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

// Def is the native-indexer definition; register it with indexer.RegisterNative.
func Def() indexer.NativeDef {
	return indexer.NativeDef{
		Name:           Name,
		DisplayName:    "AudioBook Bay",
		Protocol:       indexer.ProtocolTorrent,
		MediaTypes:     []string{"audiobook"},
		DefaultBaseURL: DefaultBaseURL,
		WIP:            true,
		New: func(ind *indexer.Indexer, httpc *http.Client) indexer.Searcher {
			return &searcher{ind: ind, bases: ParseBases(ind.BaseURL), httpc: httpc}
		},
	}
}

// ParseBases splits the indexer's base-URL field — a primary site URL plus
// optional comma-separated fallbacks (ABB runs several mirror domains) — into
// a cleaned, ordered list. Empty input yields the default site.
func ParseBases(raw string) []string {
	bases := []string{}
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimRight(strings.TrimSpace(part), "/"); p != "" {
			bases = append(bases, p)
		}
	}
	if len(bases) == 0 {
		bases = []string{DefaultBaseURL}
	}
	return bases
}

type searcher struct {
	ind   *indexer.Indexer
	bases []string // site URLs, tried in order (primary first, then fallbacks)
	httpc *http.Client
}

// Test confirms the site is reachable on at least one configured URL.
func (s *searcher) Test(ctx context.Context) error {
	var err error
	for _, base := range s.bases {
		if _, err = s.fetch(ctx, base+"/"); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no configured site URL answered (tried %d): %w", len(s.bases), err)
}

// Search finds audiobook torrents: scrape the listing for release pages, then
// each page for its info hash + trackers, and assemble a magnet. The listing is
// tried on each configured site URL in order — the primary, then fallbacks —
// and the first that answers serves the whole search (detail links point at
// the host that produced them).
func (s *searcher) Search(ctx context.Context, query, mediaType string) ([]indexer.Release, error) {
	if mediaType != "audiobook" {
		return nil, nil
	}
	var listing, base string
	var err error
	for _, b := range s.bases {
		if listing, err = s.fetch(ctx, b+"/?s="+url.QueryEscape(query)); err == nil {
			base = b
			break
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	if base == "" {
		return nil, fmt.Errorf("no configured site URL answered (tried %d): %w", len(s.bases), err)
	}

	releases := []indexer.Release{}
	fetched := 0
	for _, post := range parseListing(listing, base) {
		// Cap detail-page REQUESTS, not successes — a listing full of hash-less
		// pages must not turn into an unbounded crawl.
		if fetched >= maxDetails {
			break
		}
		if fetched > 0 {
			// Politeness delay between detail fetches, honoring cancellation.
			select {
			case <-ctx.Done():
				return releases, nil
			case <-time.After(detailPause):
			}
		}
		if ctx.Err() != nil {
			break
		}
		fetched++
		page, err := s.fetch(ctx, post.URL)
		if err != nil {
			continue // one dead post shouldn't sink the search
		}
		hash, trackers, size, ok := parseDetail(page)
		if !ok {
			continue // no info hash — nothing to build a magnet from
		}
		releases = append(releases, indexer.Release{
			IndexerID:   s.ind.ID,
			Indexer:     s.ind.Name,
			Protocol:    indexer.ProtocolTorrent,
			Title:       post.Title,
			GUID:        post.URL,
			InfoURL:     post.URL,
			DownloadURL: buildMagnet(hash, post.Title, trackers),
			Size:        size,
			Seeders:     -1, // ABB doesn't report swarm health; unknown, not dead
			Peers:       -1,
		})
	}
	return releases, nil
}

func (s *searcher) fetch(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}
	return string(body), nil
}

// --- Parsing (pure functions; fixture-tested) ---

type post struct {
	URL   string
	Title string
}

var (
	// A listing's release links: ABB wraps each post title in
	// <div class="postTitle">…<a href="URL">Title</a>. Fallback below covers
	// bare /audio-books/ permalinks if the wrapper markup shifts.
	postTitleRe = regexp.MustCompile(`(?is)<div class="postTitle">.*?<a\s+href="([^"]+)"[^>]*>(.*?)</a>`)
	audioLinkRe = regexp.MustCompile(`(?i)<a\s+href="([^"]*/audio-?books?/[^"]+)"[^>]*>([^<]+)</a>`)
	tagRe       = regexp.MustCompile(`(?s)<[^>]+>`)

	// A detail page's 40-hex info hash: the "Info Hash:" label, then the hash
	// within a short window (lazily skipping any punctuation/markup between).
	infoHashRe = regexp.MustCompile(`(?is)info\s*hash.{0,40}?([0-9a-f]{40})`)
	// Tracker announce URLs (matched on the raw HTML, so hrefs count too).
	trackerRe = regexp.MustCompile(`(?i)(udp://[^\s"'<>]+|https?://[^\s"'<>]*announce[^\s"'<>]*)`)
	// "File Size: 512.5 MB" / "Size: 1.2 GB" (matched on tag-stripped text).
	sizeRe = regexp.MustCompile(`(?i)(?:file\s*)?size:?\s*([0-9][0-9.,]*)\s*(kb|mb|gb|tb)`)
)

// navPath marks hrefs that are site navigation, not release pages — ABB's
// category/tag/pagination links live under these segments and would otherwise
// slip through the fallback link matcher as bogus "posts".
var navPath = regexp.MustCompile(`(?i)/(?:type|tag|cat|category|page|member|profile)/`)

// parseListing extracts release-page links (absolute URLs) and titles from a
// search results page. Duplicate URLs and navigation links are dropped.
func parseListing(html, base string) []post {
	seen := map[string]bool{}
	out := []post{}
	add := func(href, title string) {
		u := absURL(base, href)
		title = cleanText(title)
		if u == "" || title == "" || seen[u] || navPath.MatchString(u) {
			return
		}
		seen[u] = true
		out = append(out, post{URL: u, Title: title})
	}
	for _, m := range postTitleRe.FindAllStringSubmatch(html, -1) {
		add(m[1], m[2])
	}
	if len(out) == 0 { // markup changed — fall back to permalink shape
		for _, m := range audioLinkRe.FindAllStringSubmatch(html, -1) {
			add(m[1], m[2])
		}
	}
	return out
}

// parseDetail extracts the info hash, tracker list, and size from a release
// page. ok is false when there's no info hash (nothing to grab). The label and
// hash often straddle table tags, so hash/size are read from tag-stripped text;
// trackers are read from the raw HTML so URLs inside attributes still count.
func parseDetail(pageHTML string) (hash string, trackers []string, size int64, ok bool) {
	text := stripTags(pageHTML)
	m := infoHashRe.FindStringSubmatch(text)
	if m == nil {
		return "", nil, 0, false
	}
	hash = strings.ToLower(m[1])

	seen := map[string]bool{}
	for _, t := range trackerRe.FindAllString(pageHTML, -1) {
		t = strings.TrimRight(t, "/")
		if !seen[t] {
			seen[t] = true
			trackers = append(trackers, t)
		}
	}
	size = parseSize(text)
	return hash, trackers, size, true
}

// stripTags removes HTML tags and decodes entities, yielding plain text.
func stripTags(s string) string {
	return html.UnescapeString(tagRe.ReplaceAllString(s, " "))
}

// buildMagnet assembles a magnet URI from the info hash, a display name, and
// trackers.
func buildMagnet(hash, title string, trackers []string) string {
	var b strings.Builder
	b.WriteString("magnet:?xt=urn:btih:")
	b.WriteString(strings.ToLower(hash))
	b.WriteString("&dn=")
	b.WriteString(magnetEscape(title))
	for _, tr := range trackers {
		b.WriteString("&tr=")
		b.WriteString(magnetEscape(tr))
	}
	return b.String()
}

// magnetEscape percent-encodes a magnet parameter. QueryEscape's '+' for
// spaces is form-encoding, which not every torrent client decodes inside a
// magnet URI — %20 is the universally-parsed form (what the *arr stack emits).
func magnetEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func parseSize(html string) int64 {
	m := sizeRe.FindStringSubmatch(html)
	if m == nil {
		return 0
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil {
		return 0
	}
	switch strings.ToLower(m[2]) {
	case "kb":
		n *= 1 << 10
	case "mb":
		n *= 1 << 20
	case "gb":
		n *= 1 << 30
	case "tb":
		n *= 1 << 40
	}
	return int64(n)
}

// absURL resolves a possibly-relative href against the site base.
func absURL(base, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return strings.TrimRight(href, "/")
	}
	if !strings.HasPrefix(href, "/") {
		href = "/" + href
	}
	return strings.TrimRight(base, "/") + strings.TrimRight(href, "/")
}

// cleanText strips tags, decodes HTML entities, and collapses whitespace from a
// title fragment.
func cleanText(s string) string {
	return strings.Join(strings.Fields(stripTags(s)), " ")
}
