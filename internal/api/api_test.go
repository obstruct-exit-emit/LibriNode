package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/librinode/librinode/internal/config"
	"github.com/librinode/librinode/internal/database"
	"github.com/librinode/librinode/internal/download"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/metadata"
)

func init() {
	// Register the fake provider so settings endpoints can build it.
	metadata.Register("fake", func(s metadata.Settings) (metadata.Provider, error) {
		if s.Token == "" {
			return nil, metadata.ErrNotConfigured
		}
		return fakeProvider{}, nil
	})
}

// fakeProvider is an in-memory metadata.Provider with a tiny Discworld corpus.
type fakeProvider struct{}

func (fakeProvider) Name() string { return "fake" }

func (fakeProvider) SearchAuthors(_ context.Context, query string) ([]metadata.Author, error) {
	return []metadata.Author{{ForeignID: "100", Name: "Terry Pratchett", BookCount: 2}}, nil
}

func (fakeProvider) SearchBooks(_ context.Context, query string) ([]metadata.Book, error) {
	return []metadata.Book{{ForeignID: "1", Title: "The Colour of Magic", AuthorName: "Terry Pratchett"}}, nil
}

func (p fakeProvider) GetAuthor(_ context.Context, foreignID string) (*metadata.Author, error) {
	if foreignID != "100" {
		return nil, metadata.ErrNotFound
	}
	tcom, _ := p.GetBook(context.Background(), "1")
	tcom.Editions = nil // author lookups don't include editions
	return &metadata.Author{
		ForeignID:   "100",
		Name:        "Terry Pratchett",
		Description: "Sir Terry.",
		Books: []metadata.Book{
			*tcom,
			{ForeignID: "2", Title: "Mort", AuthorForeignID: "100", AuthorName: "Terry Pratchett"},
		},
	}, nil
}

func (fakeProvider) GetBook(_ context.Context, foreignID string) (*metadata.Book, error) {
	if foreignID != "1" {
		return nil, metadata.ErrNotFound
	}
	return &metadata.Book{
		ForeignID:       "1",
		Title:           "The Colour of Magic",
		ReleaseDate:     "1983-11-24",
		Rating:          4.1,
		AuthorForeignID: "100",
		AuthorName:      "Terry Pratchett",
		Series:          []metadata.SeriesLink{{ForeignID: "7", Title: "Discworld", Position: 1}},
		Editions: []metadata.Edition{
			{ForeignID: "11", ISBN13: "9780061020711", Format: "ebook"},
			{ForeignID: "12", ASIN: "B000W94ATC", Format: "audiobook"},
			{ForeignID: "13", Format: "physical"},
		},
	}, nil
}

type testAPI struct {
	srv    *httptest.Server
	apiKey string
	mgr    *metadata.Manager
	t      *testing.T
}

func newTestAPI(t *testing.T, provider metadata.Provider) *testAPI {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	mgr := metadata.NewManager()
	if provider != nil {
		mgr.Set(provider)
	}
	srv := httptest.NewServer(NewRouter(cfg, db, mgr, "test"))
	t.Cleanup(srv.Close)
	return &testAPI{srv: srv, apiKey: cfg.APIKey, mgr: mgr, t: t}
}

// call makes an authenticated request and decodes the JSON response into out
// (skipped when out is nil or the response has no content).
func (a *testAPI) call(method, path string, body any, out any) *http.Response {
	a.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			a.t.Fatalf("encoding body: %v", err)
		}
	}
	req, err := http.NewRequest(method, a.srv.URL+path, &buf)
	if err != nil {
		a.t.Fatalf("building request: %v", err)
	}
	req.Header.Set("X-Api-Key", a.apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			a.t.Fatalf("%s %s: decoding response: %v", method, path, err)
		}
	}
	return resp
}

func (a *testAPI) want(resp *http.Response, status int) {
	a.t.Helper()
	if resp.StatusCode != status {
		a.t.Fatalf("%s %s: status %d, want %d", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, status)
	}
}

func TestSearchRequiresAuth(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})
	resp, err := http.Get(a.srv.URL + "/api/v1/search?term=x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status without API key = %d, want 401", resp.StatusCode)
	}
}

func TestSearch(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	var books []metadata.Book
	a.want(a.call("GET", "/api/v1/search?term=magic", nil, &books), http.StatusOK)
	if len(books) != 1 || books[0].Title != "The Colour of Magic" {
		t.Errorf("book search results = %+v", books)
	}

	var authors []metadata.Author
	a.want(a.call("GET", "/api/v1/search?term=pratchett&type=author", nil, &authors), http.StatusOK)
	if len(authors) != 1 || authors[0].ForeignID != "100" {
		t.Errorf("author search results = %+v", authors)
	}

	a.want(a.call("GET", "/api/v1/search?type=book", nil, nil), http.StatusBadRequest)
	a.want(a.call("GET", "/api/v1/search?term=x&type=magazine", nil, nil), http.StatusBadRequest)
}

func TestSearchWithoutProvider(t *testing.T) {
	a := newTestAPI(t, nil)
	a.want(a.call("GET", "/api/v1/search?term=x", nil, nil), http.StatusServiceUnavailable)
	a.want(a.call("POST", "/api/v1/author", map[string]string{"foreignAuthorId": "100"}, nil), http.StatusServiceUnavailable)
	a.want(a.call("POST", "/api/v1/book", map[string]string{"foreignBookId": "1"}, nil), http.StatusServiceUnavailable)
	a.want(a.call("POST", "/api/v1/author/1/refresh", nil, nil), http.StatusServiceUnavailable)
	a.want(a.call("POST", "/api/v1/book/1/refresh", nil, nil), http.StatusServiceUnavailable)
}

func TestMetadataSettingsHotSwap(t *testing.T) {
	a := newTestAPI(t, nil)

	// No provider yet: search unavailable, settings show what's registerable.
	a.want(a.call("GET", "/api/v1/search?term=x", nil, nil), http.StatusServiceUnavailable)
	var settings struct {
		Active    string                       `json:"active"`
		Available []string                     `json:"available"`
		Providers map[string]metadata.Settings `json:"providers"`
	}
	a.want(a.call("GET", "/api/v1/settings/metadata", nil, &settings), http.StatusOK)
	found := false
	for _, name := range settings.Available {
		if name == "fake" {
			found = true
		}
	}
	if !found {
		t.Fatalf("available providers %v missing 'fake'", settings.Available)
	}

	// Unknown provider name is rejected.
	a.want(a.call("PUT", "/api/v1/settings/metadata",
		map[string]any{"active": "bogus"}, nil), http.StatusBadRequest)

	// Test button: empty token rejected, real token accepted.
	a.want(a.call("POST", "/api/v1/settings/metadata/test",
		map[string]any{"provider": "fake", "settings": map[string]string{"token": ""}}, nil), http.StatusBadRequest)
	a.want(a.call("POST", "/api/v1/settings/metadata/test",
		map[string]any{"provider": "fake", "settings": map[string]string{"token": "tok"}}, nil), http.StatusOK)

	// Saving a token activates the provider without a restart.
	a.want(a.call("PUT", "/api/v1/settings/metadata", map[string]any{
		"active":    "fake",
		"providers": map[string]any{"fake": map[string]string{"token": "tok"}},
	}, &settings), http.StatusOK)
	if settings.Active != "fake" || settings.Providers["fake"].Token != "tok" {
		t.Errorf("settings after save = %+v", settings)
	}
	a.want(a.call("GET", "/api/v1/search?term=magic", nil, nil), http.StatusOK)

	// Clearing the token deactivates it again.
	a.want(a.call("PUT", "/api/v1/settings/metadata", map[string]any{
		"active":    "fake",
		"providers": map[string]any{"fake": map[string]string{"token": ""}},
	}, nil), http.StatusOK)
	a.want(a.call("GET", "/api/v1/search?term=magic", nil, nil), http.StatusServiceUnavailable)
}

// fakeSeriesProvider serves one manga series whose volume count can grow.
type fakeSeriesProvider struct {
	volumes *int
}

func (fakeSeriesProvider) Name() string      { return "fakemanga" }
func (fakeSeriesProvider) MediaType() string { return "manga" }

func (p fakeSeriesProvider) SearchSeries(context.Context, string) ([]metadata.SeriesResult, error) {
	return []metadata.SeriesResult{{ForeignID: "500", Title: "Berserk", AuthorName: "Kentarou Miura", IssueCount: *p.volumes}}, nil
}

func (p fakeSeriesProvider) GetSeries(_ context.Context, id string) (*metadata.SeriesResult, error) {
	if id != "500" {
		return nil, metadata.ErrNotFound
	}
	result := &metadata.SeriesResult{
		ForeignID: "500", Title: "Berserk", AuthorName: "Kentarou Miura",
		Description: "Guts.", CoverURL: "https://img/berserk.jpg", IssueCount: *p.volumes,
	}
	for i := 1; i <= *p.volumes; i++ {
		result.Issues = append(result.Issues, metadata.Issue{
			ForeignID: fmt.Sprintf("500-v%d", i), Number: float64(i), Title: fmt.Sprintf("Vol. %d", i),
		})
	}
	return result, nil
}

func TestSeriesFlow(t *testing.T) {
	a := newTestAPI(t, nil)
	volumes := 3
	a.mgr.SetSeries(fakeSeriesProvider{volumes: &volumes})

	// Search via the shared search endpoint.
	var results []metadata.SeriesResult
	a.want(a.call("GET", "/api/v1/search?term=berserk&type=manga", nil, &results), http.StatusOK)
	if len(results) != 1 || results[0].ForeignID != "500" {
		t.Fatalf("search = %+v", results)
	}
	// Comic search has no provider configured.
	a.want(a.call("GET", "/api/v1/search?term=x&type=comic", nil, nil), http.StatusServiceUnavailable)

	// Add the series: volumes become monitored manga books.
	var series library.Series
	a.want(a.call("POST", "/api/v1/series",
		map[string]any{"mediaType": "manga", "foreignSeriesId": "500", "monitorNew": true}, &series), http.StatusCreated)
	if series.Title != "Berserk" || !series.Monitored || !series.MonitorNew || series.MediaType != "manga" {
		t.Fatalf("series = %+v", series)
	}
	if len(series.Volumes) != 3 {
		t.Fatalf("volumes = %+v", series.Volumes)
	}
	v1 := series.Volumes[0]
	if v1.Title != "Berserk Vol. 1" || v1.MediaType != "manga" || !v1.Monitored {
		t.Fatalf("volume 1 = %+v", v1)
	}

	// List filters by media type.
	var list []library.Series
	a.want(a.call("GET", "/api/v1/series?mediaType=manga", nil, &list), http.StatusOK)
	if len(list) != 1 {
		t.Fatalf("list = %+v", list)
	}
	a.want(a.call("GET", "/api/v1/series?mediaType=comic", nil, &list), http.StatusOK)
	if len(list) != 0 {
		t.Fatalf("comic list = %+v", list)
	}

	// Refresh discovers a new volume; monitor_new makes it monitored.
	volumes = 4
	var refreshed library.Series
	a.want(a.call("POST", fmt.Sprintf("/api/v1/series/%d/refresh", series.ID), nil, &refreshed), http.StatusOK)
	if len(refreshed.Volumes) != 4 || !refreshed.Volumes[3].Monitored {
		t.Fatalf("after refresh = %+v", refreshed.Volumes)
	}

	// Unmonitor the series: volumes follow.
	var updated library.Series
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/series/%d/monitor", series.ID),
		map[string]bool{"monitored": false, "monitorNew": false}, &updated), http.StatusOK)
	for _, v := range updated.Volumes {
		if v.Monitored {
			t.Fatalf("volume still monitored: %+v", v)
		}
	}

	// Delete removes the series and its volumes.
	a.want(a.call("DELETE", fmt.Sprintf("/api/v1/series/%d", series.ID), nil, nil), http.StatusNoContent)
	var books []library.Book
	a.want(a.call("GET", "/api/v1/book", nil, &books), http.StatusOK)
	if len(books) != 0 {
		t.Fatalf("volumes survived series delete: %+v", books)
	}

	a.want(a.call("POST", "/api/v1/series",
		map[string]any{"mediaType": "manga", "foreignSeriesId": "999"}, nil), http.StatusNotFound)
}

func TestScanFlow(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	// Root folder with one matching and one stray ebook.
	rootDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rootDir, "Terry Pratchett"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		filepath.Join("Terry Pratchett", "The Colour of Magic.epub"),
		"Stray Book.epub",
	} {
		if err := os.WriteFile(filepath.Join(rootDir, rel), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a.want(a.call("POST", "/api/v1/rootfolder",
		map[string]string{"mediaType": "ebook", "path": rootDir}, nil), http.StatusCreated)

	var author library.Author
	a.want(a.call("POST", "/api/v1/author", map[string]string{"foreignAuthorId": "100"}, &author), http.StatusCreated)

	var result struct {
		Scanned   int `json:"scanned"`
		Matched   int `json:"matched"`
		Unmatched int `json:"unmatched"`
	}
	a.want(a.call("POST", "/api/v1/library/scan", nil, &result), http.StatusOK)
	if result.Scanned != 2 || result.Matched != 1 || result.Unmatched != 1 {
		t.Fatalf("scan result = %+v", result)
	}

	// hasFile shows up in listings; the file appears in book detail.
	var books []library.Book
	a.want(a.call("GET", fmt.Sprintf("/api/v1/book?authorId=%d", author.ID), nil, &books), http.StatusOK)
	var tcom library.Book
	for _, b := range books {
		if b.Title == "The Colour of Magic" {
			tcom = b
		}
	}
	if !tcom.HasFile {
		t.Error("matched book should report hasFile")
	}
	var detail library.Book
	a.want(a.call("GET", fmt.Sprintf("/api/v1/book/%d", tcom.ID), nil, &detail), http.StatusOK)
	if len(detail.Files) != 1 || detail.Files[0].Format != "epub" {
		t.Errorf("book detail files = %+v", detail.Files)
	}

	// Unmatched files are listable; bad filters rejected.
	var unmatched []library.BookFile
	a.want(a.call("GET", "/api/v1/bookfile?unmatched=true", nil, &unmatched), http.StatusOK)
	if len(unmatched) != 1 || filepath.Base(unmatched[0].Path) != "Stray Book.epub" {
		t.Errorf("unmatched = %+v", unmatched)
	}
	a.want(a.call("GET", "/api/v1/bookfile", nil, nil), http.StatusBadRequest)
}

func TestNamingSettingsAndRename(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	// Defaults come with tokens and a rendered example.
	var ns struct {
		EbookFolder string   `json:"ebookFolder"`
		EbookFile   string   `json:"ebookFile"`
		Tokens      []string `json:"tokens"`
		Example     string   `json:"example"`
	}
	a.want(a.call("GET", "/api/v1/settings/naming", nil, &ns), http.StatusOK)
	if ns.EbookFolder != "{Author Name}" || len(ns.Tokens) == 0 {
		t.Fatalf("naming defaults = %+v", ns)
	}
	if ns.Example != "Terry Pratchett/Discworld 1 - The Colour of Magic.epub" {
		t.Fatalf("example = %q", ns.Example)
	}

	// Empty templates rejected; valid update re-renders the example.
	a.want(a.call("PUT", "/api/v1/settings/naming",
		map[string]string{"ebookFolder": "", "ebookFile": "x"}, nil), http.StatusBadRequest)
	a.want(a.call("PUT", "/api/v1/settings/naming", map[string]string{
		"ebookFolder": "{Author SortName}",
		"ebookFile":   "{Book Title} ({Release Year})",
	}, &ns), http.StatusOK)
	if ns.Example != "Pratchett, Terry/The Colour of Magic (1983).epub" {
		t.Fatalf("updated example = %q", ns.Example)
	}

	// Set up a real file, misplaced, matched to a book.
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "wrong-name.epub"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.want(a.call("POST", "/api/v1/rootfolder",
		map[string]string{"mediaType": "ebook", "path": rootDir}, nil), http.StatusCreated)
	var book library.Book
	a.want(a.call("POST", "/api/v1/book", map[string]string{"foreignBookId": "1"}, &book), http.StatusCreated)
	a.want(a.call("POST", "/api/v1/library/scan", nil, nil), http.StatusOK)

	// The scan can't match "wrong-name" — import it manually.
	var unmatched []library.BookFile
	a.want(a.call("GET", "/api/v1/bookfile?unmatched=true", nil, &unmatched), http.StatusOK)
	if len(unmatched) != 1 {
		t.Fatalf("unmatched = %+v", unmatched)
	}
	var imported struct {
		File  library.BookFile `json:"file"`
		Skips []string         `json:"skips"`
	}
	a.want(a.call("POST", fmt.Sprintf("/api/v1/bookfile/%d/match", unmatched[0].ID),
		map[string]int64{"bookId": book.ID}, &imported), http.StatusOK)
	if imported.File.BookID != book.ID {
		t.Fatalf("imported file = %+v", imported)
	}
	// Manual import moved it straight into the template location.
	wantPath := filepath.Join(rootDir, "Pratchett, Terry", "The Colour of Magic (1983).epub")
	if imported.File.Path != wantPath {
		t.Fatalf("imported path = %q, want %q", imported.File.Path, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("file not on disk at target: %v", err)
	}

	// Rename preview now reports nothing to do.
	var preview struct {
		Moves []map[string]any `json:"moves"`
	}
	a.want(a.call("GET", "/api/v1/library/rename", nil, &preview), http.StatusOK)
	if len(preview.Moves) != 0 {
		t.Fatalf("preview after import = %+v", preview.Moves)
	}

	// Changing templates makes the preview propose a move; apply executes it.
	a.want(a.call("PUT", "/api/v1/settings/naming", map[string]string{
		"ebookFolder": "{Author Name}",
		"ebookFile":   "{Book Title}",
	}, nil), http.StatusOK)
	a.want(a.call("GET", "/api/v1/library/rename", nil, &preview), http.StatusOK)
	if len(preview.Moves) != 1 {
		t.Fatalf("preview after template change = %+v", preview.Moves)
	}
	var applied struct {
		Moves []map[string]any `json:"moves"`
		Skips []string         `json:"skips"`
	}
	a.want(a.call("POST", "/api/v1/library/rename", nil, &applied), http.StatusOK)
	if len(applied.Moves) != 1 || len(applied.Skips) != 0 {
		t.Fatalf("apply = %+v", applied)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "Terry Pratchett", "The Colour of Magic.epub")); err != nil {
		t.Fatalf("file not at new target: %v", err)
	}
	// Old author dir swept.
	if _, err := os.Stat(filepath.Join(rootDir, "Pratchett, Terry")); !os.IsNotExist(err) {
		t.Error("old folder not swept")
	}
}

func TestAddingBookRematchesScannedFiles(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	// Scan finds the file before its book exists.
	rootDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rootDir, "Terry Pratchett"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(rootDir, "Terry Pratchett", "The Colour of Magic.epub")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.want(a.call("POST", "/api/v1/rootfolder",
		map[string]string{"mediaType": "ebook", "path": rootDir}, nil), http.StatusCreated)
	a.want(a.call("POST", "/api/v1/library/scan", nil, nil), http.StatusOK)

	var unmatched []library.BookFile
	a.want(a.call("GET", "/api/v1/bookfile?unmatched=true", nil, &unmatched), http.StatusOK)
	if len(unmatched) != 1 {
		t.Fatalf("unmatched = %+v", unmatched)
	}

	// Adding the book attaches the file with no re-scan.
	var book library.Book
	a.want(a.call("POST", "/api/v1/book", map[string]string{"foreignBookId": "1"}, &book), http.StatusCreated)

	a.want(a.call("GET", "/api/v1/bookfile?unmatched=true", nil, &unmatched), http.StatusOK)
	if len(unmatched) != 0 {
		t.Fatalf("file still unmatched after adding its book: %+v", unmatched)
	}
	var detail library.Book
	a.want(a.call("GET", fmt.Sprintf("/api/v1/book/%d", book.ID), nil, &detail), http.StatusOK)
	if !detail.HasFile || len(detail.Files) != 1 {
		t.Fatalf("book after add = hasFile %v, files %+v", detail.HasFile, detail.Files)
	}
}

func TestDismissBookFile(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "junk.epub"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.want(a.call("POST", "/api/v1/rootfolder",
		map[string]string{"mediaType": "ebook", "path": rootDir}, nil), http.StatusCreated)
	a.want(a.call("POST", "/api/v1/library/scan", nil, nil), http.StatusOK)

	var unmatched []library.BookFile
	a.want(a.call("GET", "/api/v1/bookfile?unmatched=true", nil, &unmatched), http.StatusOK)
	if len(unmatched) != 1 {
		t.Fatalf("unmatched = %+v", unmatched)
	}
	a.want(a.call("DELETE", fmt.Sprintf("/api/v1/bookfile/%d", unmatched[0].ID), nil, nil), http.StatusNoContent)
	a.want(a.call("GET", "/api/v1/bookfile?unmatched=true", nil, &unmatched), http.StatusOK)
	if len(unmatched) != 0 {
		t.Error("dismissed file still listed")
	}
	// Disk untouched.
	if _, err := os.Stat(filepath.Join(rootDir, "junk.epub")); err != nil {
		t.Errorf("dismiss must not delete from disk: %v", err)
	}
	// Match with a bogus book id is rejected cleanly.
	a.want(a.call("POST", "/api/v1/bookfile/1/match", map[string]int64{"bookId": 999}, nil), http.StatusNotFound)
}

// mockTorznab serves a minimal caps + one-release search response.
func mockTorznab(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch r.URL.Query().Get("t") {
		case "caps":
			w.Write([]byte(`<caps><server title="mock"/></caps>`))
		case "search":
			w.Write([]byte(`<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><item>
				<title>Mort epub</title><guid>g1</guid><link>https://mock/dl/1</link>
				<torznab:attr name="seeders" value="5"/><torznab:attr name="size" value="1048576"/>
			</item></channel></rss>`))
		default:
			http.Error(w, "bad t", http.StatusBadRequest)
		}
	}))
}

func TestIndexerCRUDAndReleaseSearch(t *testing.T) {
	a := newTestAPI(t, nil)
	srv := mockTorznab(t)
	defer srv.Close()

	// Validation.
	a.want(a.call("POST", "/api/v1/indexer",
		map[string]string{"name": "x", "type": "gopher", "baseUrl": srv.URL}, nil), http.StatusBadRequest)
	a.want(a.call("POST", "/api/v1/indexer",
		map[string]string{"name": "x", "type": "torznab", "baseUrl": "ftp://nope"}, nil), http.StatusBadRequest)

	// Test-before-save against the mock endpoint.
	a.want(a.call("POST", "/api/v1/indexer/test",
		map[string]any{"name": "mock", "type": "torznab", "baseUrl": srv.URL}, nil), http.StatusOK)

	// Create, list, update, search, delete.
	var ind struct {
		ID       int64  `json:"id"`
		Priority int    `json:"priority"`
		Enabled  bool   `json:"enabled"`
		Name     string `json:"name"`
	}
	a.want(a.call("POST", "/api/v1/indexer",
		map[string]any{"name": "mock", "type": "torznab", "baseUrl": srv.URL, "enabled": true}, &ind), http.StatusCreated)
	if ind.ID == 0 || ind.Priority != 25 {
		t.Fatalf("created indexer = %+v", ind)
	}

	var list []map[string]any
	a.want(a.call("GET", "/api/v1/indexer", nil, &list), http.StatusOK)
	if len(list) != 1 {
		t.Fatalf("list = %+v", list)
	}

	var result struct {
		Releases []map[string]any `json:"releases"`
		Errors   []string         `json:"errors"`
	}
	a.want(a.call("GET", "/api/v1/release?term=mort", nil, &result), http.StatusOK)
	if len(result.Releases) != 1 || len(result.Errors) != 0 {
		t.Fatalf("release search = %+v", result)
	}
	if result.Releases[0]["title"] != "Mort epub" || result.Releases[0]["protocol"] != "torrent" {
		t.Errorf("release = %+v", result.Releases[0])
	}
	// Generic scoring fields present: epub recognized and approved.
	if result.Releases[0]["approved"] != true {
		t.Errorf("release not approved: %+v", result.Releases[0])
	}
	a.want(a.call("GET", "/api/v1/release", nil, nil), http.StatusBadRequest)

	// Disable it: searches now hit zero indexers.
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/indexer/%d", ind.ID),
		map[string]any{"name": "mock", "type": "torznab", "baseUrl": srv.URL, "enabled": false}, nil), http.StatusOK)
	a.want(a.call("GET", "/api/v1/release?term=mort", nil, &result), http.StatusOK)
	if len(result.Releases) != 0 {
		t.Errorf("disabled indexer still searched: %+v", result)
	}

	a.want(a.call("DELETE", fmt.Sprintf("/api/v1/indexer/%d", ind.ID), nil, nil), http.StatusNoContent)
	a.want(a.call("DELETE", fmt.Sprintf("/api/v1/indexer/%d", ind.ID), nil, nil), http.StatusNotFound)
}

func TestDownloadClientsAndGrab(t *testing.T) {
	a := newTestAPI(t, nil)

	// Minimal SABnzbd mock.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("mode") {
		case "version":
			w.Write([]byte(`{"version": "4.3.2"}`))
		case "addurl":
			w.Write([]byte(`{"status": true, "nzo_ids": ["nzo_1"]}`))
		case "queue":
			w.Write([]byte(`{"queue": {"slots": [{"nzo_id": "nzo_1", "filename": "Mort", "status": "Downloading", "percentage": "50"}]}}`))
		case "history":
			w.Write([]byte(`{"history": {"slots": []}}`))
		default:
			w.Write([]byte(`{"status": false, "error": "unknown mode"}`))
		}
	}))
	defer srv.Close()

	// Validation.
	a.want(a.call("POST", "/api/v1/downloadclient",
		map[string]any{"name": "x", "type": "transmission", "host": srv.URL}, nil), http.StatusBadRequest)
	a.want(a.call("POST", "/api/v1/downloadclient",
		map[string]any{"name": "x", "type": "sabnzbd", "host": srv.URL}, nil), http.StatusBadRequest) // no apiKey

	// Test-before-save, then create.
	a.want(a.call("POST", "/api/v1/downloadclient/test",
		map[string]any{"name": "sab", "type": "sabnzbd", "host": srv.URL, "apiKey": "k"}, nil), http.StatusOK)
	var client download.ClientConfig
	a.want(a.call("POST", "/api/v1/downloadclient",
		map[string]any{"name": "sab", "type": "sabnzbd", "host": srv.URL, "apiKey": "k", "enabled": true}, &client), http.StatusCreated)
	if client.Category != "librinode" || client.Priority != 1 {
		t.Fatalf("client defaults = %+v", client)
	}

	// Grab routes by protocol; no torrent client exists.
	var grab download.GrabResult
	a.want(a.call("POST", "/api/v1/release/grab",
		map[string]any{"title": "Mort", "downloadUrl": "https://idx/get/1.nzb", "protocol": "usenet"}, &grab), http.StatusOK)
	if grab.Client != "sab" || grab.ID != "nzo_1" {
		t.Fatalf("grab = %+v", grab)
	}
	a.want(a.call("POST", "/api/v1/release/grab",
		map[string]any{"title": "Mort", "downloadUrl": "magnet:?xt=x", "protocol": "torrent"}, nil), http.StatusServiceUnavailable)
	a.want(a.call("POST", "/api/v1/release/grab",
		map[string]any{"title": "Mort", "downloadUrl": "x", "protocol": "carrier-pigeon"}, nil), http.StatusBadRequest)

	// Queue shows the download.
	var queue struct {
		Items []download.Item `json:"items"`
	}
	a.want(a.call("GET", "/api/v1/queue", nil, &queue), http.StatusOK)
	if len(queue.Items) != 1 || queue.Items[0].Status != "downloading" || queue.Items[0].Progress != 0.5 {
		t.Fatalf("queue = %+v", queue.Items)
	}

	// Disable → grab loses its client.
	client.Enabled = false
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/downloadclient/%d", client.ID), client, nil), http.StatusOK)
	a.want(a.call("POST", "/api/v1/release/grab",
		map[string]any{"title": "Mort", "downloadUrl": "https://idx/get/1.nzb", "protocol": "usenet"}, nil), http.StatusServiceUnavailable)

	a.want(a.call("DELETE", fmt.Sprintf("/api/v1/downloadclient/%d", client.ID), nil, nil), http.StatusNoContent)
}

// TestProwlarrSyncFlow simulates the conversation Prowlarr has with a
// Readarr-type application: status check, schema fetch, then indexer
// add/list/update/delete using arr-style resources with fields[].
func TestProwlarrSyncFlow(t *testing.T) {
	a := newTestAPI(t, nil)

	// 1. Test: status must expose a dotted parseable version.
	var status struct {
		Version    string `json:"version"`
		AppVersion string `json:"appVersion"`
	}
	a.want(a.call("GET", "/api/v1/system/status", nil, &status), http.StatusOK)
	for _, part := range strings.Split(status.Version, ".") {
		if _, err := strconv.Atoi(part); err != nil {
			t.Fatalf("version %q is not a dotted number", status.Version)
		}
	}
	if status.AppVersion == "" {
		t.Error("real app version missing")
	}

	// 2. Schema: both implementations offered.
	var schema []struct {
		Implementation string `json:"implementation"`
		ConfigContract string `json:"configContract"`
	}
	a.want(a.call("GET", "/api/v1/indexer/schema", nil, &schema), http.StatusOK)
	if len(schema) != 2 || schema[0].Implementation != "Newznab" || schema[1].ConfigContract != "TorznabSettings" {
		t.Fatalf("schema = %+v", schema)
	}

	// 3. Tags resolve (empty).
	a.want(a.call("GET", "/api/v1/tag", nil, nil), http.StatusOK)

	// 4. Push an indexer the way Prowlarr does.
	payload := map[string]any{
		"name":                    "MyIndexer (Prowlarr)",
		"implementation":          "Torznab",
		"configContract":          "TorznabSettings",
		"enableRss":               true,
		"enableAutomaticSearch":   true,
		"enableInteractiveSearch": true,
		"priority":                20,
		"fields": []map[string]any{
			{"name": "baseUrl", "value": "http://prowlarr:9696/1/"},
			{"name": "apiPath", "value": "/api"},
			{"name": "apiKey", "value": "prowlarr-key"},
			{"name": "categories", "value": []int{7000, 7010, 7020}},
		},
	}
	var created map[string]any
	a.want(a.call("POST", "/api/v1/indexer", payload, &created), http.StatusCreated)
	id := int64(created["id"].(float64))

	// Stored natively and correctly.
	if created["type"] != "torznab" || created["baseUrl"] != "http://prowlarr:9696/1" ||
		created["categories"] != "7000,7010,7020" || created["apiKey"] != "prowlarr-key" {
		t.Fatalf("created = %+v", created)
	}
	// And the arr view round-trips for Prowlarr's diffing.
	if created["implementation"] != "Torznab" || created["protocol"] != "torrent" {
		t.Fatalf("arr view = %+v", created)
	}
	fields, ok := created["fields"].([]any)
	if !ok || len(fields) == 0 {
		t.Fatalf("fields missing from response: %+v", created)
	}

	// 5. List and get-by-id include both dialects.
	var list []map[string]any
	a.want(a.call("GET", "/api/v1/indexer", nil, &list), http.StatusOK)
	if len(list) != 1 || list[0]["implementation"] != "Torznab" || list[0]["name"] != "MyIndexer (Prowlarr)" {
		t.Fatalf("list = %+v", list)
	}
	var got map[string]any
	a.want(a.call("GET", fmt.Sprintf("/api/v1/indexer/%d", id), nil, &got), http.StatusOK)
	if got["implementation"] != "Torznab" {
		t.Fatalf("get = %+v", got)
	}

	// 6. Prowlarr updates (e.g. disables) the indexer.
	payload["enableRss"] = false
	payload["enableAutomaticSearch"] = false
	payload["enableInteractiveSearch"] = false
	var updated map[string]any
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/indexer/%d", id), payload, &updated), http.StatusOK)
	if updated["enabled"] != false || updated["enableRss"] != false {
		t.Fatalf("updated = %+v", updated)
	}

	// 7. And removes it.
	a.want(a.call("DELETE", fmt.Sprintf("/api/v1/indexer/%d", id), nil, nil), http.StatusNoContent)

	// Unsupported implementations are rejected cleanly.
	payload["implementation"] = "Omgwtfnzbs"
	a.want(a.call("POST", "/api/v1/indexer", payload, nil), http.StatusBadRequest)
}

func TestQualityProfiles(t *testing.T) {
	a := newTestAPI(t, nil)

	// Seeded default present.
	var profiles []library.QualityProfile
	a.want(a.call("GET", "/api/v1/qualityprofile", nil, &profiles), http.StatusOK)
	if len(profiles) != 1 || profiles[0].Name != "Standard Ebook" || !profiles[0].IsDefault {
		t.Fatalf("profiles = %+v", profiles)
	}
	seeded := profiles[0].ID

	// Create an epub-only profile; validation rejects junk.
	a.want(a.call("POST", "/api/v1/qualityprofile",
		map[string]any{"name": "Bad", "formats": []string{"docx"}}, nil), http.StatusBadRequest)
	var epubOnly library.QualityProfile
	a.want(a.call("POST", "/api/v1/qualityprofile",
		map[string]any{"name": "EPUB Only", "formats": []string{"epub"}, "language": "english"}, &epubOnly), http.StatusCreated)
	if epubOnly.IsDefault {
		t.Error("new profile must not steal default")
	}

	// Default swap and guarded delete.
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/qualityprofile/%d/default", epubOnly.ID), nil, nil), http.StatusOK)
	a.want(a.call("DELETE", fmt.Sprintf("/api/v1/qualityprofile/%d", epubOnly.ID), nil, nil), http.StatusBadRequest)
	a.want(a.call("DELETE", fmt.Sprintf("/api/v1/qualityprofile/%d", seeded), nil, nil), http.StatusNoContent)

	// Update the remaining profile.
	var updated library.QualityProfile
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/qualityprofile/%d", epubOnly.ID),
		map[string]any{"name": "EPUB Only", "formats": []string{"epub", "azw3"}, "retailBonus": 30}, &updated), http.StatusOK)
	if len(updated.Formats) != 2 || updated.RetailBonus != 30 || !updated.IsDefault {
		t.Errorf("updated = %+v", updated)
	}
}

func TestProfileDrivesSearchScoring(t *testing.T) {
	a := newTestAPI(t, nil)

	// Mock indexer serving one mobi release.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Query().Get("t") == "caps" {
			w.Write([]byte(`<caps><server title="mock"/></caps>`))
			return
		}
		w.Write([]byte(`<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>
			<item><title>Mort MOBI</title><guid>g1</guid><link>https://mock/dl/1</link>
			<torznab:attr name="seeders" value="5"/><torznab:attr name="size" value="1048576"/></item>
		</channel></rss>`))
	}))
	defer srv.Close()
	a.want(a.call("POST", "/api/v1/indexer",
		map[string]any{"name": "mock", "type": "torznab", "baseUrl": srv.URL, "enabled": true}, nil), http.StatusCreated)

	approved := func() bool {
		var result struct {
			Releases []struct {
				Approved   bool     `json:"approved"`
				Rejections []string `json:"rejections"`
			} `json:"releases"`
		}
		a.want(a.call("GET", "/api/v1/release?term=mort", nil, &result), http.StatusOK)
		if len(result.Releases) != 1 {
			t.Fatalf("releases = %+v", result.Releases)
		}
		return result.Releases[0].Approved
	}

	// Seeded default allows mobi.
	if !approved() {
		t.Fatal("mobi should be approved under the standard profile")
	}

	// Switch the default to an epub-only profile: same release now rejected.
	var epubOnly library.QualityProfile
	a.want(a.call("POST", "/api/v1/qualityprofile",
		map[string]any{"name": "EPUB Only", "formats": []string{"epub"}}, &epubOnly), http.StatusCreated)
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/qualityprofile/%d/default", epubOnly.ID), nil, nil), http.StatusOK)
	if approved() {
		t.Fatal("mobi should be rejected under the epub-only profile")
	}
}

func TestBookAwareReleaseSearch(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	// Indexer returning one right and one wrong release for any query.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Query().Get("t") == "caps" {
			w.Write([]byte(`<caps><server title="mock"/></caps>`))
			return
		}
		w.Write([]byte(`<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>
			<item><title>Terry Pratchett - The Colour of Magic Retail EPUB</title><guid>g1</guid>
				<link>https://mock/dl/1</link>
				<torznab:attr name="seeders" value="9"/><torznab:attr name="size" value="1048576"/></item>
			<item><title>Stephen King - It EPUB</title><guid>g2</guid>
				<link>https://mock/dl/2</link>
				<torznab:attr name="seeders" value="99"/><torznab:attr name="size" value="1048576"/></item>
		</channel></rss>`))
	}))
	defer srv.Close()
	a.want(a.call("POST", "/api/v1/indexer",
		map[string]any{"name": "mock", "type": "torznab", "baseUrl": srv.URL, "enabled": true}, nil), http.StatusCreated)

	var book library.Book
	a.want(a.call("POST", "/api/v1/book", map[string]string{"foreignBookId": "1"}, &book), http.StatusCreated)

	var result struct {
		Releases []struct {
			Title      string   `json:"title"`
			Approved   bool     `json:"approved"`
			Score      int      `json:"score"`
			Rejections []string `json:"rejections"`
		} `json:"releases"`
	}
	a.want(a.call("GET", fmt.Sprintf("/api/v1/release?bookId=%d", book.ID), nil, &result), http.StatusOK)
	if len(result.Releases) != 2 {
		t.Fatalf("releases = %+v", result.Releases)
	}
	// The right book ranks first despite fewer seeders; the wrong one is rejected.
	first, second := result.Releases[0], result.Releases[1]
	if !first.Approved || first.Title != "Terry Pratchett - The Colour of Magic Retail EPUB" {
		t.Errorf("first = %+v", first)
	}
	if second.Approved || len(second.Rejections) == 0 {
		t.Errorf("wrong book not rejected: %+v", second)
	}

	a.want(a.call("GET", "/api/v1/release?bookId=9999", nil, nil), http.StatusNotFound)
}

func TestAutoSearchEndpoints(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	// Indexer offering the right book; SAB accepting grabs.
	idx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Query().Get("t") == "caps" {
			w.Write([]byte(`<caps><server title="mock"/></caps>`))
			return
		}
		w.Write([]byte(`<rss xmlns:newznab="http://www.newznab.com/DTD/2010/feeds/attributes/"><channel>
			<item><title>Terry Pratchett - The Colour of Magic Retail EPUB</title><guid>g1</guid>
			<link>https://idx/get/tcom.nzb</link><newznab:attr name="size" value="1048576"/></item>
		</channel></rss>`))
	}))
	defer idx.Close()
	sab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("mode") {
		case "addurl":
			w.Write([]byte(`{"status": true, "nzo_ids": ["nzo_as"]}`))
		default:
			w.Write([]byte(`{"version": "4.3.2", "queue": {"slots": []}, "history": {"slots": []}}`))
		}
	}))
	defer sab.Close()

	a.want(a.call("POST", "/api/v1/indexer",
		map[string]any{"name": "mock", "type": "newznab", "baseUrl": idx.URL, "enabled": true}, nil), http.StatusCreated)
	a.want(a.call("POST", "/api/v1/downloadclient",
		map[string]any{"name": "sab", "type": "sabnzbd", "host": sab.URL, "apiKey": "k", "enabled": true}, nil), http.StatusCreated)

	var book library.Book
	a.want(a.call("POST", "/api/v1/book", map[string]string{"foreignBookId": "1"}, &book), http.StatusCreated)

	// Per-book automatic search grabs the release.
	var outcome struct {
		Grabbed bool   `json:"grabbed"`
		Release string `json:"release"`
		Message string `json:"message"`
	}
	a.want(a.call("POST", fmt.Sprintf("/api/v1/book/%d/search", book.ID), nil, &outcome), http.StatusOK)
	if !outcome.Grabbed || outcome.Release == "" {
		t.Fatalf("outcome = %+v", outcome)
	}

	// History has the grab tied to the book.
	var history []download.GrabRecord
	a.want(a.call("GET", "/api/v1/history", nil, &history), http.StatusOK)
	if len(history) != 1 || history[0].BookID != book.ID {
		t.Fatalf("history = %+v", history)
	}

	// Search-all skips the pending book.
	var all struct {
		Searched int `json:"searched"`
		Grabbed  int `json:"grabbed"`
	}
	a.want(a.call("POST", "/api/v1/library/search", nil, &all), http.StatusOK)
	if all.Searched != 0 || all.Grabbed != 0 {
		t.Fatalf("search-all = %+v", all)
	}

	a.want(a.call("POST", "/api/v1/book/9999/search", nil, nil), http.StatusNotFound)
}

func TestMagazineSeries(t *testing.T) {
	a := newTestAPI(t, nil)

	// Magazines are created by name; no provider involved.
	a.want(a.call("POST", "/api/v1/series",
		map[string]any{"mediaType": "magazine"}, nil), http.StatusBadRequest) // no title
	var mag library.Series
	a.want(a.call("POST", "/api/v1/series",
		map[string]any{"mediaType": "magazine", "title": "The Economist"}, &mag), http.StatusCreated)
	if mag.MediaType != "magazine" || !mag.Monitored || !mag.MonitorNew || mag.Source != "manual" {
		t.Fatalf("magazine = %+v", mag)
	}

	// Listed alongside other series types; filterable.
	var list []library.Series
	a.want(a.call("GET", "/api/v1/series?mediaType=magazine", nil, &list), http.StatusOK)
	if len(list) != 1 || list[0].Title != "The Economist" {
		t.Fatalf("list = %+v", list)
	}

	// Refresh is a quiet no-op (no provider), not an error.
	a.want(a.call("POST", fmt.Sprintf("/api/v1/series/%d/refresh", mag.ID), nil, nil), http.StatusOK)

	// Magazine search-by-provider is rejected with guidance.
	a.want(a.call("GET", "/api/v1/search?term=x&type=magazine", nil, nil), http.StatusBadRequest)

	a.want(a.call("DELETE", fmt.Sprintf("/api/v1/series/%d", mag.ID), nil, nil), http.StatusNoContent)
}

func TestRefreshEndpoints(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	var author library.Author
	a.want(a.call("POST", "/api/v1/author", map[string]string{"foreignAuthorId": "100"}, &author), http.StatusCreated)
	var book library.Book
	a.want(a.call("POST", "/api/v1/book", map[string]string{"foreignBookId": "1"}, &book), http.StatusCreated)

	var refreshed library.Author
	a.want(a.call("POST", fmt.Sprintf("/api/v1/author/%d/refresh", author.ID), nil, &refreshed), http.StatusOK)
	if refreshed.ID != author.ID || len(refreshed.Books) == 0 {
		t.Errorf("refreshed author = %+v", refreshed)
	}

	var refreshedBook library.Book
	a.want(a.call("POST", fmt.Sprintf("/api/v1/book/%d/refresh", book.ID), nil, &refreshedBook), http.StatusOK)
	if refreshedBook.ID != book.ID || len(refreshedBook.Editions) != 3 {
		t.Errorf("refreshed book = %+v", refreshedBook)
	}

	a.want(a.call("POST", "/api/v1/author/9999/refresh", nil, nil), http.StatusNotFound)
	a.want(a.call("POST", "/api/v1/book/9999/refresh", nil, nil), http.StatusNotFound)
}

func TestAddAuthorFlow(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	var author library.Author
	a.want(a.call("POST", "/api/v1/author", map[string]string{"foreignAuthorId": "100"}, &author), http.StatusCreated)
	if author.ID == 0 || author.Name != "Terry Pratchett" || !author.Monitored {
		t.Fatalf("created author = %+v", author)
	}
	if len(author.Books) != 2 {
		t.Fatalf("author created with %d books, want 2", len(author.Books))
	}
	for _, b := range author.Books {
		if !b.Monitored {
			t.Errorf("book %q not monitored after author add", b.Title)
		}
	}

	// Unknown author at the provider → 404.
	a.want(a.call("POST", "/api/v1/author", map[string]string{"foreignAuthorId": "999"}, nil), http.StatusNotFound)

	// Adding again is an idempotent refresh, not a duplicate.
	var again library.Author
	a.want(a.call("POST", "/api/v1/author", map[string]string{"foreignAuthorId": "100"}, &again), http.StatusCreated)
	if again.ID != author.ID {
		t.Errorf("re-add created a new author: id %d != %d", again.ID, author.ID)
	}

	var authors []library.Author
	a.want(a.call("GET", "/api/v1/author", nil, &authors), http.StatusOK)
	if len(authors) != 1 {
		t.Fatalf("listed %d authors, want 1", len(authors))
	}

	var monResp map[string]any
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/author/%d/monitor", author.ID),
		map[string]bool{"monitored": false}, &monResp), http.StatusOK)
	var detail library.Author
	a.want(a.call("GET", fmt.Sprintf("/api/v1/author/%d", author.ID), nil, &detail), http.StatusOK)
	if detail.Monitored {
		t.Error("author still monitored after unmonitor")
	}

	// Delete cascades to books.
	a.want(a.call("DELETE", fmt.Sprintf("/api/v1/author/%d", author.ID), nil, nil), http.StatusNoContent)
	var books []library.Book
	a.want(a.call("GET", "/api/v1/book", nil, &books), http.StatusOK)
	if len(books) != 0 {
		t.Errorf("%d books survived author delete", len(books))
	}
}

func TestAddBookFlow(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	var book library.Book
	a.want(a.call("POST", "/api/v1/book", map[string]string{"foreignBookId": "1"}, &book), http.StatusCreated)
	if book.ID == 0 || book.Title != "The Colour of Magic" || !book.Monitored {
		t.Fatalf("created book = %+v", book)
	}

	// Author was created as an unmonitored stub, not a full bibliography add.
	var authors []library.Author
	a.want(a.call("GET", "/api/v1/author", nil, &authors), http.StatusOK)
	if len(authors) != 1 || authors[0].Monitored {
		t.Fatalf("authors after book add = %+v", authors)
	}
	var allBooks []library.Book
	a.want(a.call("GET", "/api/v1/book", nil, &allBooks), http.StatusOK)
	if len(allBooks) != 1 {
		t.Fatalf("%d books in library, want just the added one", len(allBooks))
	}

	// Editions landed; only the ebook one is monitored (Phase 1 ebook-first).
	if len(book.Editions) != 3 {
		t.Fatalf("book has %d editions, want 3", len(book.Editions))
	}
	monitoredByFormat := map[string]bool{}
	var audioEditionID int64
	for _, e := range book.Editions {
		monitoredByFormat[e.Format] = e.Monitored
		if e.Format == "audiobook" {
			audioEditionID = e.ID
		}
	}
	if !monitoredByFormat["ebook"] || monitoredByFormat["audiobook"] || monitoredByFormat["physical"] {
		t.Errorf("edition monitoring by format = %v, want only ebook", monitoredByFormat)
	}

	// Series link persisted.
	if len(book.Series) != 1 || book.Series[0].Title != "Discworld" || book.Series[0].Position != 1 {
		t.Errorf("book series = %+v", book.Series)
	}

	// Manually monitor the audiobook edition.
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/edition/%d/monitor", audioEditionID),
		map[string]bool{"monitored": true}, nil), http.StatusOK)
	var detail library.Book
	a.want(a.call("GET", fmt.Sprintf("/api/v1/book/%d", book.ID), nil, &detail), http.StatusOK)
	for _, e := range detail.Editions {
		if e.ID == audioEditionID && !e.Monitored {
			t.Error("audiobook edition not monitored after monitor call")
		}
	}

	// Unmonitor the book itself.
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/book/%d/monitor", book.ID),
		map[string]bool{"monitored": false}, nil), http.StatusOK)
	a.want(a.call("GET", fmt.Sprintf("/api/v1/book/%d", book.ID), nil, &detail), http.StatusOK)
	if detail.Monitored {
		t.Error("book still monitored after unmonitor")
	}

	// Unknown book at the provider → 404.
	a.want(a.call("POST", "/api/v1/book", map[string]string{"foreignBookId": "999"}, nil), http.StatusNotFound)

	// Delete the book; the author stub stays.
	a.want(a.call("DELETE", fmt.Sprintf("/api/v1/book/%d", book.ID), nil, nil), http.StatusNoContent)
	a.want(a.call("GET", fmt.Sprintf("/api/v1/book/%d", book.ID), nil, nil), http.StatusNotFound)
	a.want(a.call("GET", "/api/v1/author", nil, &authors), http.StatusOK)
	if len(authors) != 1 {
		t.Errorf("author stub deleted with book")
	}
}
