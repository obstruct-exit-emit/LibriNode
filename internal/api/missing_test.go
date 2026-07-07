package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/librinode/librinode/internal/library"
)

// TestAuthorMissing: books neither monitored nor owned in a format library
// surface as the author's bibliography gaps, with series info for grouping.
func TestAuthorMissing(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	var author library.Author
	a.want(a.call("POST", "/api/v1/author", map[string]string{"foreignAuthorId": "100"}, &author), http.StatusCreated)

	// Adding an author monitors the whole bibliography — no gaps yet.
	var missing []library.Book
	a.want(a.call("GET", fmt.Sprintf("/api/v1/author/%d/missing?library=ebook", author.ID), nil, &missing), http.StatusOK)
	if len(missing) != 0 {
		t.Fatalf("fresh author has %d missing, want 0: %+v", len(missing), missing)
	}

	// Every book is missing from the audiobook library (never added there).
	a.want(a.call("GET", fmt.Sprintf("/api/v1/author/%d/missing?library=audiobook", author.ID), nil, &missing), http.StatusOK)
	if len(missing) != 2 {
		t.Fatalf("audiobook missing = %d, want 2", len(missing))
	}
	// Series books sort before standalones, and carry their series link.
	if missing[0].Title != "The Colour of Magic" || len(missing[0].Series) != 1 || missing[0].Series[0].Title != "Discworld" {
		t.Errorf("first missing = %+v, want The Colour of Magic with Discworld link", missing[0])
	}
	if missing[1].Title != "Mort" || len(missing[1].Series) != 0 {
		t.Errorf("second missing = %+v, want standalone Mort", missing[1])
	}

	// Unmonitoring an unowned ebook makes it a gap there too.
	var books []library.Book
	a.want(a.call("GET", fmt.Sprintf("/api/v1/book?authorId=%d", author.ID), nil, &books), http.StatusOK)
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/book/%d/library", books[0].ID),
		map[string]any{"library": "ebook", "member": true, "monitored": false}, nil), http.StatusOK)
	a.want(a.call("GET", fmt.Sprintf("/api/v1/author/%d/missing?library=ebook", author.ID), nil, &missing), http.StatusOK)
	if len(missing) != 1 || missing[0].ID != books[0].ID {
		t.Fatalf("after unmonitor, ebook missing = %+v, want just book %d", missing, books[0].ID)
	}

	// Re-monitoring clears the gap (the one-click Monitor button's call).
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/book/%d/library", books[0].ID),
		map[string]any{"library": "ebook", "member": true, "monitored": true}, nil), http.StatusOK)
	a.want(a.call("GET", fmt.Sprintf("/api/v1/author/%d/missing?library=ebook", author.ID), nil, &missing), http.StatusOK)
	if len(missing) != 0 {
		t.Fatalf("after re-monitor, missing = %+v, want none", missing)
	}
}
