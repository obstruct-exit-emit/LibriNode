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
	"testing"

	"github.com/librinode/librinode/internal/config"
	"github.com/librinode/librinode/internal/database"
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
	return &testAPI{srv: srv, apiKey: cfg.APIKey, t: t}
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
				<torznab:attr name="seeders" value="5"/><torznab:attr name="size" value="1000"/>
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
