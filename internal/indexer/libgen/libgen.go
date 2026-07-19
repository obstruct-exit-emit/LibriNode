// Package libgen is a native indexer for Library Genesis: the long-running
// open ebook mirror network. It has no Newznab API, so LibriNode scrapes its
// search (both the non-fiction and fiction indexes) and builds direct-protocol
// releases whose download URLs point at the open mirror hosts, keyed by the
// file's MD5 — the identifier every Libgen mirror serves by. The direct
// download client follows each mirror's landing page to the real file and
// fails over between mirrors.
//
// This is a dual-use shadow-library source: it is never bundled or enabled by
// default; a user adds it deliberately and is responsible for its use. HTML
// selectors target Libgen's known layout and may need updating when the site
// changes — the inherent fragility of a scraped source.
package libgen

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
	Name = "libgen"
	// DefaultBaseURL is the classic mirror; the site runs several (libgen.is,
	// libgen.rs, libgen.st), so the indexer's site URLs can override it.
	DefaultBaseURL = "https://libgen.is"

	maxResults = 50
	userAgent  = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

// MirrorDownloadURLs builds the "|"-separated direct-protocol download URL for
// a file known by MD5: the open mirror hosts that serve Libgen content, tried
// in order by the direct client (each is a landing page it knows how to
// follow). Shared with Anna's Archive, whose keyless downloads resolve through
// these same mirrors.
func MirrorDownloadURLs(md5 string) string {
	md5 = strings.ToLower(md5)
	return strings.Join([]string{
		"https://library.lol/main/" + md5,
		"https://libgen.li/ads.php?md5=" + md5,
	}, "|")
}

// Def is the native-indexer definition; register it with indexer.RegisterNative.
func Def() indexer.NativeDef {
	return indexer.NativeDef{
		Name:           Name,
		DisplayName:    "Library Genesis",
		Protocol:       indexer.ProtocolDirect,
		MediaTypes:     []string{"ebook"},
		DefaultBaseURL: DefaultBaseURL,
		WIP:            true,
		New: func(ind *indexer.Indexer, httpc *http.Client) indexer.Searcher {
			return &searcher{ind: ind, bases: parseBases(ind.BaseURL), httpc: httpc}
		},
	}
}

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

// Search queries both Libgen indexes — non-fiction (search.php) and fiction
// (/fiction/?q=) — on the first site URL that answers, and merges the results.
func (s *searcher) Search(ctx context.Context, query, mediaType string) ([]indexer.Release, error) {
	if mediaType != "ebook" {
		return nil, nil
	}
	var base string
	var nonFiction string
	var err error
	for _, b := range s.bases {
		nonFiction, err = s.fetch(ctx, b+"/search.php?req="+url.QueryEscape(query)+"&res=50&view=simple&phrase=1&column=def")
		if err == nil {
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

	results := parseResults(nonFiction)
	// Fiction lives in its own index; a failure here shouldn't sink the search.
	if fiction, ferr := s.fetch(ctx, base+"/fiction/?q="+url.QueryEscape(query)); ferr == nil {
		results = append(results, parseResults(fiction)...)
	}

	seen := map[string]bool{}
	releases := []indexer.Release{}
	for _, res := range results {
		if len(releases) >= maxResults {
			break
		}
		if seen[res.MD5] {
			continue
		}
		seen[res.MD5] = true
		releases = append(releases, indexer.Release{
			IndexerID:   s.ind.ID,
			Indexer:     s.ind.Name,
			Protocol:    indexer.ProtocolDirect,
			Title:       res.Title,
			GUID:        res.MD5,
			InfoURL:     base + "/book/index.php?md5=" + res.MD5,
			DownloadURL: MirrorDownloadURLs(res.MD5),
			Size:        res.Size,
			Seeders:     -1,
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

type result struct {
	MD5   string
	Title string
	Size  int64
}

var (
	// A result row's md5: both indexes link through it — md5.php?..., an
	// ads/landing href, or a plain md5= query on the row's links.
	md5Re = regexp.MustCompile(`(?i)md5=([0-9a-f]{32})`)
	// Row-splitting: results are table rows in both indexes.
	rowRe = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	// The title cell links to the book page; grab the anchor text with the
	// longest cleaned form in the row (author links are shorter).
	anchorTextRe = regexp.MustCompile(`(?is)<a\s+[^>]*>(.*?)</a>`)
	tagRe        = regexp.MustCompile(`(?s)<[^>]+>`)
	// Sizes render like "1 MB", "435 KB", "1.2Mb" in the row text.
	sizeRe = regexp.MustCompile(`(?i)\b([0-9][0-9.,]*)\s*(kb|mb|gb)\b`)
)

// parseResults extracts (md5, title, size) from a Libgen results table —
// either index, since both render rows whose links carry md5= and whose cells
// carry a size.
func parseResults(page string) []result {
	out := []result{}
	for _, row := range rowRe.FindAllStringSubmatch(page, -1) {
		block := row[1]
		m := md5Re.FindStringSubmatch(block)
		if m == nil {
			continue
		}
		title := ""
		for _, a := range anchorTextRe.FindAllStringSubmatch(block, -1) {
			if t := cleanText(a[1]); len(t) > len(title) && !md5Re.MatchString(t) {
				title = t
			}
		}
		if title == "" {
			continue
		}
		out = append(out, result{MD5: strings.ToLower(m[1]), Title: title, Size: parseSize(block)})
	}
	return out
}

func parseSize(block string) int64 {
	m := sizeRe.FindStringSubmatch(tagRe.ReplaceAllString(block, " "))
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

func cleanText(s string) string {
	return strings.Join(strings.Fields(tagRe.ReplaceAllString(s, " ")), " ")
}
