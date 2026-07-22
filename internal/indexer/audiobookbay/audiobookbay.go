// Package audiobookbay is a native indexer for AudioBook Bay (ABB): an
// audiobook torrent site with no Newznab/Torznab API, so Prowlarr can't reach
// it. ABB never publishes a .torrent or a ready magnet — a release page carries
// the info hash and a tracker list, and the magnet is assembled from them here
// (the exact step that breaks a Prowlarr→Jackett definition). The result is an
// ordinary torrent Release that rides LibriNode's existing qBittorrent path.
//
// Ban avoidance is the whole game with ABB, and mirrors how a browser behaves:
//   - Search hits the site ONCE (the results listing). It does NOT crawl every
//     result's detail page — that per-search fan-out is what earns an IP ban.
//     Each release's magnet is assembled lazily at grab time (Resolve), for the
//     one release actually grabbed.
//   - Every request rides a warmed-up session (a PHPSESSID cookie fetched from
//     the homepage first) with full browser headers; ABB serves reliable
//     search pages only to an initialized, browser-like session.
//   - A search redirected to the homepage means ABB is rate-limiting/blocking
//     (common on a shared VPN exit IP). The search retries a few times with
//     backoff — a browser-like "try again" — and only then surfaces a clear
//     error, so a transient bounce doesn't fail an otherwise-working source.
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
	"net/http/cookiejar"
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

	// A current desktop-Chrome UA — ABB filters obvious scrapers.
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36"
	// legacyCategory is the category query ABB's search expects (already
	// percent-encoded); shelfmark found it necessary for reliable results.
	legacyCategory = "undefined%2Cundefined"

	// searchAttempts is how many times a homepage-bounced search is retried
	// before surfacing the rate-limit error. ABB bounces are transient —
	// especially on a shared VPN exit IP — and a browser-like "try again"
	// usually succeeds where the first attempt was blocked.
	searchAttempts = 3
	// searchBackoff scales the pause between retries: attempt N waits N× this.
	searchBackoff = 700 * time.Millisecond
)

// defaultTrackers back-fill a magnet when a release page lists none, so the
// torrent can still find peers.
var defaultTrackers = []string{
	"udp://tracker.openbittorrent.com:80/announce",
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://tracker.coppersurfer.tk:6969/announce",
	"udp://exodus.desync.com:6969/announce",
	"udp://tracker.internetwarriors.net:1337/announce",
}

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

// session returns a client with its own fresh cookie jar (so a search's homepage
// warm-up + listing, and a grab's warm-up + detail, share one PHPSESSID like a
// browser tab) AND its own connection — keep-alives are disabled.
//
// The app-wide indexer client pools keep-alive connections for its whole
// lifetime. AudioBook Bay (Cloudflare-fronted) throttles a connection once it
// has served enough requests: the reused connection then quietly returns empty
// result pages or bounces the search to the homepage, while a *fresh* connection
// keeps working — which is why a browser, curl, and a just-started process all
// succeed against the exact same site and IP where a long-running server fails.
// Opening a fresh connection per request sidesteps that entirely; a scraped
// source is low-volume, so the extra handshakes cost nothing that matters.
func (s *searcher) session() *http.Client {
	c := *s.httpc
	c.Transport = &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		DisableKeepAlives: true,
		ForceAttemptHTTP2: true,
	}
	if jar, err := cookiejar.New(nil); err == nil {
		c.Jar = jar
	}
	return &c
}

// Test confirms the site is reachable on at least one configured URL.
func (s *searcher) Test(ctx context.Context) error {
	var err error
	for _, base := range s.bases {
		if _, _, err = fetch(ctx, s.session(), base+"/"); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no configured site URL answered (tried %d): %w", len(s.bases), err)
}

// Search finds audiobook torrents from ONE listing request per site (after a
// session warm-up). Each result's magnet is deferred to Resolve (grab time),
// so a search never crawls detail pages — the behaviour that gets ABB to ban an
// IP. The first configured site that answers serves the search; a homepage
// redirect means the search was blocked/rate-limited.
func (s *searcher) Search(ctx context.Context, query, mediaType string) ([]indexer.Release, error) {
	if mediaType != "audiobook" {
		return nil, nil
	}
	var lastErr error
	for _, base := range s.bases {
		posts, err := s.searchBase(ctx, base, query)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		releases := make([]indexer.Release, 0, len(posts))
		for _, p := range posts {
			releases = append(releases, indexer.Release{
				IndexerID: s.ind.ID,
				Indexer:   s.ind.Name,
				Protocol:  indexer.ProtocolTorrent,
				Title:     p.Title,
				GUID:      p.URL,
				InfoURL:   p.URL,
				// The release page itself — Resolve turns it into a magnet at
				// grab time (one request, only when grabbed).
				DownloadURL: p.URL,
				Seeders:     -1, // ABB doesn't report swarm health; unknown, not dead
				Peers:       -1,
			})
		}
		return releases, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no configured site URL answered")
	}
	return nil, lastErr
}

// searchBase warms a fresh session and runs ONE listing request against a site.
// A homepage bounce (ABB rate-limiting/blocking, common on a shared VPN IP) is
// retried a few times with backoff — re-warming a fresh session each time, the
// way hitting "search" again in a browser would — before giving up. A warm-up or
// listing transport error isn't retried here; the caller falls to the next site.
func (s *searcher) searchBase(ctx context.Context, base, query string) ([]post, error) {
	var lastErr error
	for attempt := 0; attempt < searchAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * searchBackoff):
			}
		}
		client := s.session()
		// Warm up the session from the homepage first, then search on it.
		if _, _, err := fetch(ctx, client, base+"/"); err != nil {
			return nil, err
		}
		listing, finalURL, err := fetch(ctx, client, base+"/?s="+url.QueryEscape(query)+"&cat="+legacyCategory)
		if err != nil {
			return nil, err
		}
		if isHomepageRedirect(finalURL, base) {
			lastErr = fmt.Errorf("AudioBook Bay redirected the search to its homepage — it is likely rate-limiting or temporarily blocking this IP; try again later")
			continue
		}
		return parseListing(listing, base), nil
	}
	return nil, lastErr
}

// Resolve turns a release-page URL into an assembled magnet — called at grab
// time for exactly the release the user grabbed. It warms a fresh session, then
// fetches that one page for its info hash + trackers.
func (s *searcher) Resolve(ctx context.Context, downloadURL string) (string, error) {
	if strings.HasPrefix(downloadURL, "magnet:") {
		return downloadURL, nil // already resolved
	}
	u, err := url.Parse(downloadURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid AudioBook Bay release URL")
	}
	client := s.session()
	home := u.Scheme + "://" + u.Host + "/"
	_, _, _ = fetch(ctx, client, home) // best-effort warm-up
	page, _, err := fetch(ctx, client, downloadURL)
	if err != nil {
		return "", fmt.Errorf("fetching AudioBook Bay release page: %w", err)
	}
	hash, trackers, _, ok := parseDetail(page)
	if !ok {
		return "", fmt.Errorf("no info hash on the AudioBook Bay release page (its layout may have changed)")
	}
	if len(trackers) == 0 {
		trackers = defaultTrackers
	}
	return buildMagnet(hash, titleFromURL(u), trackers), nil
}

// fetch GETs a URL on the given session client and returns the body plus the
// final URL (after redirects), for homepage-redirect detection. Headers are kept
// deliberately minimal — a browser User-Agent and Accept, nothing more. The
// shelfmark reference client (and a plain curl) get search results this way,
// whereas the fuller "browser navigation" header set (Sec-Fetch-*, Referer,
// Upgrade-Insecure-Requests) paired with a Go client's TLS/HTTP fingerprint
// reads as a spoofed browser and gets the search bounced to the homepage.
func fetch(ctx context.Context, client *http.Client, rawURL string) (body, finalURL string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return "", "", fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}
	final := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	return string(b), final, nil
}

// isHomepageRedirect reports whether a search landed back on the site's
// homepage — ABB's tell for a blocked or rate-limited search.
func isHomepageRedirect(finalURL, base string) bool {
	fu, err := url.Parse(finalURL)
	if err != nil {
		return false
	}
	bu, err := url.Parse(base)
	if err != nil {
		return false
	}
	return strings.EqualFold(fu.Host, bu.Host) && (fu.Path == "" || fu.Path == "/") && fu.RawQuery == ""
}

// titleFromURL derives a magnet display name from a release page's slug.
func titleFromURL(u *url.URL) string {
	seg := strings.Trim(u.Path, "/")
	if i := strings.LastIndex(seg, "/"); i >= 0 {
		seg = seg[i+1:]
	}
	seg = strings.ReplaceAll(seg, "-", " ")
	if seg == "" {
		return "audiobook"
	}
	return seg
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

// absURL resolves a possibly-relative href against the site base. It preserves
// the href's trailing slash: ABB's canonical release permalinks end in "/", and
// requesting the slash-less form returns a 301 with NO Location header — an
// unfollowable dead-end redirect — so the grab-time Resolve fetch of that URL
// would fail with "HTTP 301" and no magnet would ever be assembled.
func absURL(base, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	if !strings.HasPrefix(href, "/") {
		href = "/" + href
	}
	return strings.TrimRight(base, "/") + href
}

// cleanText strips tags, decodes HTML entities, and collapses whitespace from a
// title fragment.
func cleanText(s string) string {
	return strings.Join(strings.Fields(stripTags(s)), " ")
}
