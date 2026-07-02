package hardcover

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/quillarr/quillarr/internal/metadata"
)

// mockAPI serves canned GraphQL responses keyed by operation name and
// verifies auth + request shape on every call.
func mockAPI(t *testing.T, responses map[string]string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want Bearer test-token", got)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		for op, resp := range responses {
			if len(req.Query) >= len("query "+op) && req.Query[:len("query "+op)] == "query "+op {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(resp))
				return
			}
		}
		t.Errorf("no canned response for query: %.60s", req.Query)
		http.Error(w, "unexpected query", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	return New("test-token", WithEndpoint(srv.URL))
}

func TestSearchAuthors(t *testing.T) {
	c := mockAPI(t, map[string]string{
		"Search": `{"data": {"search": {"results": {
			"hits": [
				{"document": {"id": "123", "name": "Terry Pratchett", "books_count": 41,
					"image": {"url": "https://img.example/tp.jpg"}}},
				{"document": {"id": 456, "name": "Neil Gaiman", "books_count": 30, "image": null}}
			]
		}}}}`,
	})

	authors, err := c.SearchAuthors(context.Background(), "pratchett")
	if err != nil {
		t.Fatalf("SearchAuthors: %v", err)
	}
	if len(authors) != 2 {
		t.Fatalf("got %d authors, want 2", len(authors))
	}
	if authors[0].ForeignID != "123" || authors[0].Name != "Terry Pratchett" {
		t.Errorf("author[0] = %+v", authors[0])
	}
	if authors[0].ImageURL != "https://img.example/tp.jpg" {
		t.Errorf("image url = %q", authors[0].ImageURL)
	}
	if authors[0].BookCount != 41 {
		t.Errorf("book count = %d", authors[0].BookCount)
	}
	// Numeric id normalized to string.
	if authors[1].ForeignID != "456" {
		t.Errorf("author[1] id = %q, want 456", authors[1].ForeignID)
	}
}

func TestSearchBooks_ResultsAsJSONString(t *testing.T) {
	// The Typesense payload can arrive as a JSON-encoded string; ensure we
	// unwrap it.
	inner := `{"hits": [{"document": {"id": 99, "title": "Mort", "author_names": ["Terry Pratchett"], "release_year": 1987, "rating": 4.25, "image": {"url": "https://img.example/mort.jpg"}}}]}`
	payload, _ := json.Marshal(inner)
	c := mockAPI(t, map[string]string{
		"Search": `{"data": {"search": {"results": ` + string(payload) + `}}}`,
	})

	books, err := c.SearchBooks(context.Background(), "mort")
	if err != nil {
		t.Fatalf("SearchBooks: %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("got %d books, want 1", len(books))
	}
	b := books[0]
	if b.ForeignID != "99" || b.Title != "Mort" || b.AuthorName != "Terry Pratchett" {
		t.Errorf("book = %+v", b)
	}
	if b.ReleaseDate != "1987" {
		t.Errorf("release date = %q, want 1987 (from release_year)", b.ReleaseDate)
	}
	if b.Rating != 4.25 {
		t.Errorf("rating = %v", b.Rating)
	}
}

func TestGetAuthor(t *testing.T) {
	c := mockAPI(t, map[string]string{
		"Author": `{"data": {"authors": [{
			"id": 123,
			"name": "Terry Pratchett",
			"bio": "Sir Terry.",
			"cached_image": {"url": "https://img.example/tp.jpg"},
			"contributions": [
				{"book": {"id": 1, "title": "The Colour of Magic", "release_date": "1983-11-24",
					"rating": 4.1, "cached_image": {"url": "https://img.example/tcom.jpg"},
					"book_series": [{"position": 1, "series": {"id": 7, "name": "Discworld", "description": "The Disc."}}]}},
				{"book": {"id": 1, "title": "The Colour of Magic (dupe contribution)"}},
				{"book": null}
			]
		}]}}`,
	})

	a, err := c.GetAuthor(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetAuthor: %v", err)
	}
	if a.Name != "Terry Pratchett" || a.Description != "Sir Terry." {
		t.Errorf("author = %+v", a)
	}
	if len(a.Books) != 1 {
		t.Fatalf("got %d books, want 1 (dupes and null books skipped)", len(a.Books))
	}
	b := a.Books[0]
	if b.ForeignID != "1" || b.AuthorForeignID != "123" || b.AuthorName != "Terry Pratchett" {
		t.Errorf("book = %+v", b)
	}
	if len(b.Series) != 1 || b.Series[0].Title != "Discworld" || b.Series[0].Position != 1 {
		t.Errorf("series = %+v", b.Series)
	}
}

func TestGetAuthorNotFound(t *testing.T) {
	c := mockAPI(t, map[string]string{
		"Author": `{"data": {"authors": []}}`,
	})
	if _, err := c.GetAuthor(context.Background(), "999"); !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	// Non-numeric ids can't exist at Hardcover.
	if _, err := c.GetAuthor(context.Background(), "abc"); !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("non-numeric id: err = %v, want ErrNotFound", err)
	}
}

func TestGetBook(t *testing.T) {
	c := mockAPI(t, map[string]string{
		"Book": `{"data": {"books": [{
			"id": 1,
			"title": "The Colour of Magic",
			"description": "The Discworld begins.",
			"release_date": "1983-11-24",
			"rating": 4.1,
			"cached_image": {"url": "https://img.example/tcom.jpg"},
			"contributions": [{"author": {"id": 123, "name": "Terry Pratchett"}}],
			"book_series": [{"position": 1, "series": {"id": 7, "name": "Discworld"}}],
			"editions": [
				{"id": 11, "title": "The Colour of Magic", "isbn_13": "9780061020711", "asin": "",
				 "release_date": "2009-10-13", "reading_format_id": 4,
				 "cached_image": {"url": "https://img.example/ed11.jpg"},
				 "publisher": {"name": "Harper"}, "language": {"language": "English"}},
				{"id": 12, "isbn_13": "", "asin": "B000W94ATC", "reading_format_id": 2,
				 "publisher": null, "language": null},
				{"id": 13, "reading_format_id": 1}
			]
		}]}}`,
	})

	b, err := c.GetBook(context.Background(), "1")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if b.Title != "The Colour of Magic" || b.AuthorForeignID != "123" {
		t.Errorf("book = %+v", b)
	}
	if len(b.Editions) != 3 {
		t.Fatalf("got %d editions, want 3", len(b.Editions))
	}
	ebook := b.Editions[0]
	if ebook.Format != "ebook" || ebook.ISBN13 != "9780061020711" || ebook.Publisher != "Harper" || ebook.Language != "English" {
		t.Errorf("ebook edition = %+v", ebook)
	}
	if b.Editions[1].Format != "audiobook" || b.Editions[1].ASIN != "B000W94ATC" {
		t.Errorf("audio edition = %+v", b.Editions[1])
	}
	if b.Editions[2].Format != "physical" {
		t.Errorf("physical edition = %+v", b.Editions[2])
	}
	if len(b.Series) != 1 || b.Series[0].ForeignID != "7" {
		t.Errorf("series = %+v", b.Series)
	}
}

func TestGraphQLErrorSurfaces(t *testing.T) {
	c := mockAPI(t, map[string]string{
		"Book": `{"errors": [{"message": "field 'bogus' not found in type: 'books'"}]}`,
	})
	_, err := c.GetBook(context.Background(), "1")
	if err == nil {
		t.Fatal("expected error from GraphQL errors array")
	}
}
