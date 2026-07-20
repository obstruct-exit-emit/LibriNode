// Package annasarchive is a native indexer for Anna's Archive (AA): a
// shadow-library search engine over ebook collections. Search is keyless
// (scraped from the public results page), and so are downloads — the free
// path is primary. Every release's download chain (handled by the direct
// client, which fails over between entries) leads with AA's own free "slow"
// partner servers, then the open Libgen mirror network (AA identifies files
// by MD5, the same key those mirrors serve by). A paid AA membership key is
// optional and only appends the fast-download API as a last-resort fallback.
//
// This is a dual-use shadow-library source: it is never bundled or enabled by
// default; a user adds it deliberately and is responsible for its use. HTML
// selectors target AA's known layout and may need updating when the site
// changes — the inherent fragility of a scraped source.
package annasarchive

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/librinode/librinode/internal/indexer"
	"github.com/librinode/librinode/internal/indexer/libgen"
)

const (
	// Name is both the registry key and the stored indexer type.
	Name = "annas-archive"
	// DefaultBaseURL is an Anna's Archive mirror that resolves where the main
	// annas-archive.org domain is commonly DNS/network-blocked. It runs several
	// mirrors, so the indexer's site URLs (primary + comma-separated fallbacks)
	// can override it. NOTE: Anna's renders search results client-side behind a
	// JS/anti-bot wall, so a plain HTTP fetch returns no scrapable results —
	// search here is best-effort and effectively needs a browser/bypasser.
	// Library Genesis (which Anna's aggregates) is the reliable ebook source.
	DefaultBaseURL = "https://annas-archive.li"

	maxResults = 50
	userAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36"
)

// Def is the native-indexer definition; register it with indexer.RegisterNative.
func Def() indexer.NativeDef {
	return indexer.NativeDef{
		Name:           Name,
		DisplayName:    "Anna's Archive",
		Protocol:       indexer.ProtocolDirect,
		MediaTypes:     []string{"ebook"},
		DefaultBaseURL: DefaultBaseURL,
		// The key is optional: downloads are free by default (Anna's slow
		// servers + open mirrors); a key only adds a paid fast-path fallback.
		NeedsAPIKey: false,
		WIP:         true,
		New: func(ind *indexer.Indexer, httpc *http.Client) indexer.Searcher {
			return &searcher{ind: ind, bases: parseBases(ind.BaseURL), httpc: httpc}
		},
	}
}

// parseBases splits the primary-plus-fallback site URL list (comma-separated),
// defaulting to the main domain.
func parseBases(raw string) []string {
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
	bases []string
	httpc *http.Client
}

// Test confirms the site answers on at least one configured URL.
func (s *searcher) Test(ctx context.Context) error {
	var err error
	for _, base := range s.bases {
		if _, err = s.fetch(ctx, base+"/"); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no configured site URL answered (tried %d): %w", len(s.bases), err)
}

// downloadURLs builds a release's "|"-separated direct-download chain,
// free-first. Anna's free "slow" partner servers come first, then the open
// Libgen mirrors (both need no account); a membership key adds the paid
// fast-download API only as a last-resort fallback. This keeps the free path
// primary — a paid key is a bonus, never a requirement.
func (s *searcher) downloadURLs(base, md5 string) string {
	parts := []string{
		// Anna's own free downloads (slow partner servers).
		base + "/slow_download/" + md5 + "/0/0",
		base + "/slow_download/" + md5 + "/0/1",
		base + "/slow_download/" + md5 + "/0/2",
		// The open Libgen mirror network, keyed by the same MD5.
		libgen.MirrorDownloadURLs(md5),
	}
	if key := strings.TrimSpace(s.ind.APIKey); key != "" {
		parts = append(parts,
			base+"/dyn/api/fast_download.json?md5="+md5+"&key="+url.QueryEscape(key))
	}
	return strings.Join(parts, "|")
}

// Search scrapes AA's search results into direct-protocol releases. Downloads
// are free out of the box (Anna's slow servers + open mirrors); a membership
// key only adds a paid fast-path fallback.
func (s *searcher) Search(ctx context.Context, query, mediaType string) ([]indexer.Release, error) {
	if mediaType != "ebook" {
		return nil, nil
	}
	var page, base string
	var err error
	for _, b := range s.bases {
		if page, err = s.fetch(ctx, b+"/search?q="+url.QueryEscape(query)); err == nil {
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
	for _, res := range parseResults(page) {
		if len(releases) >= maxResults {
			break
		}
		rel := indexer.Release{
			IndexerID: s.ind.ID,
			Indexer:   s.ind.Name,
			Protocol:  indexer.ProtocolDirect,
			Title:     res.Title,
			GUID:      res.MD5,
			InfoURL:   base + "/md5/" + res.MD5,
			Size:      res.Size,
			Seeders:   -1,
			Peers:     -1,
		}
		rel.DownloadURL = s.downloadURLs(base, res.MD5)
		releases = append(releases, rel)
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

type result struct {
	MD5   string
	Title string
	Size  int64
}

var (
	// One search hit: a link to /md5/<32 hex>. The row's text (title and the
	// "language, ext, size" metadata line) lives inside the same anchor block
	// in AA's layout.
	md5LinkRe = regexp.MustCompile(`(?is)<a[^>]+href="/md5/([0-9a-f]{32})[^"]*"[^>]*>(.*?)</a>`)
	tagRe     = regexp.MustCompile(`(?s)<[^>]+>`)
	// The metadata line's size: "1.2MB", "980.5 kB".
	sizeRe = regexp.MustCompile(`(?i)([0-9][0-9.,]*)\s*(kb|mb|gb)\b`)
	// A title heading inside the anchor block (AA marks it with an h3).
	titleRe = regexp.MustCompile(`(?is)<h3[^>]*>(.*?)</h3>`)
)

// parseResults extracts search hits (md5, title, size) from a results page.
// Duplicate md5s (cover link + text link for the same book) are collapsed.
func parseResults(page string) []result {
	seen := map[string]bool{}
	out := []result{}
	for _, m := range md5LinkRe.FindAllStringSubmatch(page, -1) {
		md5, block := strings.ToLower(m[1]), m[2]
		title := ""
		if t := titleRe.FindStringSubmatch(block); t != nil {
			title = cleanText(t[1])
		}
		if title == "" {
			title = cleanText(block)
		}
		if title == "" || seen[md5] {
			continue
		}
		seen[md5] = true
		out = append(out, result{MD5: md5, Title: title, Size: parseSize(block)})
	}
	return out
}

func parseSize(block string) int64 {
	m := sizeRe.FindStringSubmatch(stripTags(block))
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
	}
	return int64(n)
}

func stripTags(s string) string {
	return tagRe.ReplaceAllString(s, " ")
}

func cleanText(s string) string {
	return strings.Join(strings.Fields(stripTags(s)), " ")
}
