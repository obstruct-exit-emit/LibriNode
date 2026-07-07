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

	// Adding an author enrolls NO books — the whole bibliography starts as
	// Missing, for the user to monitor selectively.
	var missing []library.Book
	a.want(a.call("GET", fmt.Sprintf("/api/v1/author/%d/missing?library=ebook", author.ID), nil, &missing), http.StatusOK)
	if len(missing) != 2 {
		t.Fatalf("fresh author has %d missing, want the whole bibliography (2)", len(missing))
	}

	// Same from the audiobook side.
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

	// Monitoring a book (the one-click Monitor button) clears its gap.
	var books []library.Book
	a.want(a.call("GET", fmt.Sprintf("/api/v1/book?authorId=%d", author.ID), nil, &books), http.StatusOK)
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/book/%d/library", books[0].ID),
		map[string]any{"library": "ebook", "member": true, "monitored": true}, nil), http.StatusOK)
	a.want(a.call("GET", fmt.Sprintf("/api/v1/author/%d/missing?library=ebook", author.ID), nil, &missing), http.StatusOK)
	if len(missing) != 1 || missing[0].ID == books[0].ID {
		t.Fatalf("after monitor, ebook missing = %+v, want only the other book", missing)
	}

	// Unmonitoring the unowned book returns it to Missing.
	a.want(a.call("PUT", fmt.Sprintf("/api/v1/book/%d/library", books[0].ID),
		map[string]any{"library": "ebook", "member": true, "monitored": false}, nil), http.StatusOK)
	a.want(a.call("GET", fmt.Sprintf("/api/v1/author/%d/missing?library=ebook", author.ID), nil, &missing), http.StatusOK)
	if len(missing) != 2 {
		t.Fatalf("after unmonitor, missing = %d, want 2", len(missing))
	}
}
