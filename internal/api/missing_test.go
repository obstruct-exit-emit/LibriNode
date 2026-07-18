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

// TestAddIsolation: adding an author or book into ONE format library must
// never enroll the other — the libraries are linked only by ownership.
func TestAddIsolation(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	var author library.Author
	a.want(a.call("POST", "/api/v1/author",
		map[string]string{"foreignAuthorId": "100", "library": "ebook"}, &author), http.StatusCreated)
	if !author.InEbookLibrary || author.InAudiobookLibrary {
		t.Fatalf("ebook author add: memberships = ebook %v audio %v, want true/false",
			author.InEbookLibrary, author.InAudiobookLibrary)
	}
	var audioAuthors []library.Author
	a.want(a.call("GET", "/api/v1/author?library=audiobook", nil, &audioAuthors), http.StatusOK)
	if len(audioAuthors) != 0 {
		t.Fatalf("audiobook library lists %d author(s) after an ebook-only add", len(audioAuthors))
	}

	// Adding one BOOK as an ebook: the book and its author stay out of
	// Audiobooks.
	var books []library.Book
	a.want(a.call("GET", fmt.Sprintf("/api/v1/book?authorId=%d", author.ID), nil, &books), http.StatusOK)
	var added library.Book
	a.want(a.call("POST", "/api/v1/book",
		map[string]string{"foreignBookId": books[0].ForeignID, "library": "ebook"}, &added), http.StatusCreated)
	if !added.InEbookLibrary || added.InAudiobookLibrary || added.AudiobookMonitored {
		t.Fatalf("ebook book add: flags ebook %v / audio %v / audioMon %v, want ebook-only",
			added.InEbookLibrary, added.InAudiobookLibrary, added.AudiobookMonitored)
	}
	a.want(a.call("GET", "/api/v1/author?library=audiobook", nil, &audioAuthors), http.StatusOK)
	if len(audioAuthors) != 0 {
		t.Fatalf("audiobook library gained an author from an ebook book-add")
	}
}

// TestRefreshPreservesMembership: metadata refresh must never enroll,
// un-enroll, or re-monitor — a deliberately added ebook stays an ebook
// library member across an author refresh.
func TestRefreshPreservesMembership(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	var author library.Author
	a.want(a.call("POST", "/api/v1/author",
		map[string]string{"foreignAuthorId": "100", "library": "ebook"}, &author), http.StatusCreated)
	var books []library.Book
	a.want(a.call("GET", fmt.Sprintf("/api/v1/book?authorId=%d", author.ID), nil, &books), http.StatusOK)
	var added library.Book
	a.want(a.call("POST", "/api/v1/book",
		map[string]string{"foreignBookId": books[0].ForeignID, "library": "ebook"}, &added), http.StatusCreated)

	a.want(a.call("POST", fmt.Sprintf("/api/v1/author/%d/refresh", author.ID), nil, nil), http.StatusOK)

	var after library.Book
	a.want(a.call("GET", fmt.Sprintf("/api/v1/book/%d", added.ID), nil, &after), http.StatusOK)
	if !after.InEbookLibrary || !after.EbookMonitored {
		t.Fatalf("after refresh: inEbook %v ebookMonitored %v — refresh un-enrolled the book",
			after.InEbookLibrary, after.EbookMonitored)
	}
	if after.InAudiobookLibrary {
		t.Fatal("after refresh: book gained audiobook membership")
	}
}

// TestLibraryRefresh: the library-wide metadata refresh counts the library's
// records, refuses provider-less magazines, and answers 202 for a real run.
func TestLibraryRefresh(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	a.want(a.call("POST", "/api/v1/library/refresh",
		map[string]string{"mediaType": "magazine"}, nil), http.StatusBadRequest)

	var res struct {
		Started int    `json:"started"`
		Message string `json:"message"`
	}
	a.want(a.call("POST", "/api/v1/library/refresh",
		map[string]string{"mediaType": "ebook"}, &res), http.StatusOK)
	if res.Started != 0 {
		t.Fatalf("empty library started = %d, want 0", res.Started)
	}

	var author library.Author
	a.want(a.call("POST", "/api/v1/author", map[string]string{"foreignAuthorId": "100"}, &author), http.StatusCreated)
	a.want(a.call("POST", "/api/v1/library/refresh",
		map[string]string{"mediaType": "ebook"}, &res), http.StatusAccepted)
	if res.Started != 1 || res.Message == "" {
		t.Fatalf("refresh response = %+v, want started 1 with a message", res)
	}
}
