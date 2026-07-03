package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/quillarr/quillarr/internal/config"
	"github.com/quillarr/quillarr/internal/database"
	"github.com/quillarr/quillarr/internal/library"
	"github.com/quillarr/quillarr/internal/metadata"
)

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

	srv := httptest.NewServer(NewRouter(cfg, db, provider, "test"))
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
