// Package annasarchive is a native indexer for Anna's Archive (AA): a
// shadow-library search engine over ebook collections. Search is keyless
// (scraped from the public results page), but downloads need a paid AA
// membership key — the release then carries AA's fast-download API URL, which
// answers JSON naming the real file URL; LibriNode's direct download client
// follows that hop and streams the file itself. Without a key the source is
// search-only: releases carry no download URL and scoring marks them
// ungrabbable with the reason.
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
)

const (
	// Name is both the registry key and the stored indexer type.
	Name = "annas-archive"
	// DefaultBaseURL is AA's main domain; it runs mirrors, so the indexer's
	// site URLs (primary + comma-separated fallbacks) can override it.
	DefaultBaseURL = "https://annas-archive.org"

	maxResults = 50
	userAgent  = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

// Def is the native-indexer definition; register it with indexer.RegisterNative.
func Def() indexer.NativeDef {
	return indexer.NativeDef{
		Name:           Name,
		DisplayName:    "Anna's Archive",
		Protocol:       indexer.ProtocolDirect,
		MediaTypes:     []string{"ebook"},
		DefaultBaseURL: DefaultBaseURL,
		// The key is optional: keyless = search-only (downloads need an AA
		// membership key), so it isn't required to add the indexer.
		NeedsAPIKey: false,
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

// Search scrapes AA's search results into releases. With a membership key each
// release's download URL is the fast-download API (a JSON hop the direct
// client follows); without one releases are informational only.
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
		if key := strings.TrimSpace(s.ind.APIKey); key != "" {
			rel.DownloadURL = base + "/dyn/api/fast_download.json?md5=" + res.MD5 + "&key=" + url.QueryEscape(key)
		}
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
