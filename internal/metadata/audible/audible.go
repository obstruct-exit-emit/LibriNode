// Package audible implements metadata.AudiobookProvider against Audible's
// public catalog API (https://api.audible.com/1.0/catalog/products). It needs
// no key. Given a work's title and author it returns that work's audiobook
// editions, each carrying the narrator, total runtime, and abridged flag that
// the book/ebook providers don't have — the data the audiobook side of the
// library is missing.
package audible

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/librinode/librinode/internal/metadata"
)

const DefaultEndpoint = "https://api.audible.com/1.0/catalog"

// searchGroups asks for the contributor list (authors + narrators), runtime and
// format attributes, publisher/language, and cover images in one call.
const searchGroups = "contributors,media,product_attrs,product_desc"

// maxEditions caps how many audiobook editions of one work we keep — a popular
// classic can have dozens of narrations; the most relevant handful is plenty.
const maxEditions = 12

// Factory builds the provider. Audible's catalog needs no credential, so it is
// always available (never ErrNotConfigured); the global language/adult
// preferences on the settings shape ranking and filtering.
func Factory(s metadata.Settings) (metadata.AudiobookProvider, error) {
	return New(WithSettings(s)), nil
}

type Client struct {
	endpoint     string
	httpc        *http.Client
	language     string // preferred edition language, normalized ("" = no preference)
	includeAdult bool
}

type Option func(*Client)

func WithEndpoint(u string) Option { return func(c *Client) { c.endpoint = u } }

// WithLanguage sets the preferred edition language; matching editions rank
// above others (e.g. the English narration over the Italian one).
func WithLanguage(lang string) Option {
	return func(c *Client) { c.language = normalize(lang) }
}

// WithSettings carries the global metadata preferences: preferred language and
// whether adult titles are included.
func WithSettings(s metadata.Settings) Option {
	return func(c *Client) {
		c.language = normalize(s.Language)
		c.includeAdult = s.IncludeAdult
	}
}

func New(opts ...Option) *Client {
	c := &Client{
		endpoint: DefaultEndpoint,
		httpc:    &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) Name() string { return "audible" }

// product is the slice of an Audible catalog product we use.
type product struct {
	ASIN           string            `json:"asin"`
	Title          string            `json:"title"`
	Subtitle       string            `json:"subtitle"`
	Authors        []named           `json:"authors"`
	Narrators      []named           `json:"narrators"`
	RuntimeMinutes int               `json:"runtime_length_min"`
	FormatType     string            `json:"format_type"` // "unabridged" | "abridged"
	ReleaseDate    string            `json:"release_date"`
	PublisherName  string            `json:"publisher_name"`
	Language       string            `json:"language"`
	IsAdultProduct bool              `json:"is_adult_product"`
	ProductImages  map[string]string `json:"product_images"`
}

type named struct {
	Name string `json:"name"`
}

type searchResponse struct {
	Products []product `json:"products"`
}

// FindEditions searches Audible for the work and returns its audiobook editions,
// best match first: exact title over subtitle-tail matches, the preferred
// language over others, unabridged over abridged, then the more complete
// listing. A work Audible doesn't carry returns an empty slice, not an error.
func (c *Client) FindEditions(ctx context.Context, title, author string) ([]metadata.Edition, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, nil
	}
	products, err := c.search(ctx, strings.TrimSpace(title+" "+author))
	if err != nil {
		return nil, err
	}

	wantTitle := normalize(title)
	wantAuthor := normalize(author)

	type scored struct {
		ed        metadata.Edition
		score     int
		langMatch bool
	}
	var matches []scored
	for _, p := range products {
		if !c.includeAdult && p.IsAdultProduct {
			continue
		}
		s := titleScore(wantTitle, p)
		if s == 0 {
			continue // title doesn't match the work
		}
		if wantAuthor != "" && !authorMatches(p.Authors, wantAuthor) {
			continue
		}
		ed := toEdition(p)
		matches = append(matches, scored{
			ed:        ed,
			score:     s,
			langMatch: c.language != "" && ed.Language == c.language,
		})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.langMatch != b.langMatch {
			return a.langMatch // preferred language first
		}
		if a.ed.Abridged != b.ed.Abridged {
			return !a.ed.Abridged // unabridged first
		}
		return completeness(a.ed) > completeness(b.ed)
	})

	out := make([]metadata.Edition, 0, len(matches))
	for _, m := range matches {
		if len(out) >= maxEditions {
			break
		}
		out = append(out, m.ed)
	}
	return out, nil
}

func (c *Client) search(ctx context.Context, keywords string) ([]product, error) {
	params := url.Values{
		"keywords":         {keywords},
		"num_results":      {"20"},
		"products_sort_by": {"Relevance"},
		"response_groups":  {searchGroups},
	}
	endpoint := strings.TrimRight(c.endpoint, "/") + "/products?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("audible: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("audible: search returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("audible: decoding search: %w", err)
	}
	return out.Products, nil
}

func toEdition(p product) metadata.Edition {
	names := make([]string, 0, len(p.Narrators))
	for _, n := range p.Narrators {
		if n.Name != "" {
			names = append(names, n.Name)
		}
	}
	return metadata.Edition{
		ForeignID:      p.ASIN,
		ASIN:           p.ASIN,
		Title:          p.Title,
		Format:         "audiobook",
		Publisher:      p.PublisherName,
		Language:       strings.ToLower(p.Language),
		ReleaseDate:    p.ReleaseDate,
		CoverURL:       largestImage(p.ProductImages),
		Narrator:       strings.Join(names, ", "),
		RuntimeMinutes: p.RuntimeMinutes,
		Abridged:       strings.EqualFold(p.FormatType, "abridged"),
	}
}

// titleScore rates how well a product's title matches the wanted work: 2 for an
// exact normalized match, 1 when the product title starts with the wanted title
// (a subtitle or edition tail), 0 for no match.
func titleScore(wantTitle string, p product) int {
	pt := normalize(p.Title)
	switch {
	case pt == wantTitle:
		return 2
	case strings.HasPrefix(pt, wantTitle+" "):
		return 1
	default:
		return 0
	}
}

func authorMatches(authors []named, wantAuthor string) bool {
	for _, a := range authors {
		na := normalize(a.Name)
		if na == "" {
			continue
		}
		if na == wantAuthor || strings.Contains(na, wantAuthor) || strings.Contains(wantAuthor, na) {
			return true
		}
	}
	return false
}

func completeness(e metadata.Edition) int {
	n := 0
	if e.Narrator != "" {
		n++
	}
	if e.RuntimeMinutes > 0 {
		n++
	}
	return n
}

func largestImage(images map[string]string) string {
	best, bestSize := "", -1
	for k, v := range images {
		size := 0
		fmt.Sscanf(k, "%d", &size)
		if size > bestSize {
			best, bestSize = v, size
		}
	}
	return best
}

// normalize lowercases, drops punctuation, and collapses whitespace so titles
// and names compare regardless of "&"/"and", commas, and casing.
func normalize(s string) string {
	var b strings.Builder
	lastSpace := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		case unicode.IsSpace(r):
			if !lastSpace && b.Len() > 0 {
				b.WriteRune(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}
