package library

import "testing"

// TestMirrorReconcileUnifiesExistingBooks: turning mirror on brings every prose
// book that is in (or monitored in) either format into both, and enrolls the
// author in both libraries.
func TestMirrorReconcileUnifiesExistingBooks(t *testing.T) {
	s := newTestStore(t)
	a := &Author{Source: "t", ForeignID: "a1", Name: "Terry"}
	if err := s.UpsertAuthor(a); err != nil {
		t.Fatal(err)
	}
	ebookOnly := &Book{AuthorID: a.ID, Source: "t", ForeignID: "b1", Title: "Mort"}
	audioOnly := &Book{AuthorID: a.ID, Source: "t", ForeignID: "b2", Title: "Reaper Man"}
	for _, b := range []*Book{ebookOnly, audioOnly} {
		if err := s.UpsertBook(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetBookLibrary(ebookOnly.ID, "ebook", true, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBookLibrary(audioOnly.ID, "audiobook", true, true); err != nil {
		t.Fatal(err)
	}

	if err := s.SetAuthorMirror(a.ID, true); err != nil {
		t.Fatal(err)
	}

	for _, id := range []int64{ebookOnly.ID, audioOnly.ID} {
		b, err := s.GetBook(id)
		if err != nil {
			t.Fatal(err)
		}
		if !b.InEbookLibrary || !b.InAudiobookLibrary || !b.EbookMonitored || !b.AudiobookMonitored {
			t.Errorf("book %d after mirror = %+v, want in+monitored in both formats", id, b)
		}
	}
	got, err := s.GetAuthor(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Mirror || !got.InEbookLibrary || !got.InAudiobookLibrary {
		t.Errorf("author after mirror: mirror=%v ebook=%v audio=%v, want all true",
			got.Mirror, got.InEbookLibrary, got.InAudiobookLibrary)
	}
}

// TestMirrorTogglePropagates: with mirror on, a per-format library write lands
// on both formats, and a removal clears both.
func TestMirrorTogglePropagates(t *testing.T) {
	s := newTestStore(t)
	a := &Author{Source: "t", ForeignID: "a1", Name: "Terry"}
	if err := s.UpsertAuthor(a); err != nil {
		t.Fatal(err)
	}
	b := &Book{AuthorID: a.ID, Source: "t", ForeignID: "b1", Title: "Mort"}
	if err := s.UpsertBook(b); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAuthorMirror(a.ID, true); err != nil {
		t.Fatal(err)
	}

	if err := s.SetBookLibrary(b.ID, "ebook", true, true); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetBook(b.ID)
	if !got.InEbookLibrary || !got.InAudiobookLibrary || !got.EbookMonitored || !got.AudiobookMonitored {
		t.Fatalf("after ebook add = %+v, want both formats in+monitored", got)
	}

	if err := s.SetBookLibrary(b.ID, "ebook", false, false); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetBook(b.ID)
	if got.InEbookLibrary || got.InAudiobookLibrary || got.EbookMonitored || got.AudiobookMonitored {
		t.Fatalf("after ebook removal = %+v, want both formats cleared", got)
	}
}

// TestMirrorImportWantsOtherFormat: owning one format for a mirrored author
// makes the missing format a wanted item (in library + monitored, no file),
// and a later re-scan of the owned book respects a manual un-monitor.
func TestMirrorImportWantsOtherFormat(t *testing.T) {
	s := newTestStore(t)
	a := &Author{Source: "t", ForeignID: "a1", Name: "Terry"}
	if err := s.UpsertAuthor(a); err != nil {
		t.Fatal(err)
	}
	b := &Book{AuthorID: a.ID, Source: "t", ForeignID: "b1", Title: "Mort"}
	if err := s.UpsertBook(b); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAuthorMirror(a.ID, true); err != nil {
		t.Fatal(err)
	}

	// Own the ebook: the audiobook becomes wanted.
	if err := s.EnsureBookLibrary(b.ID, "ebook"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetBook(b.ID)
	if !got.InEbookLibrary || !got.InAudiobookLibrary || !got.AudiobookMonitored {
		t.Fatalf("after owning ebook = %+v, want audiobook wanted (in library + monitored)", got)
	}

	// User un-monitors; a subsequent re-scan must not re-assert monitoring.
	if err := s.SetBookLibrary(b.ID, "ebook", true, false); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureBookLibrary(b.ID, "ebook"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetBook(b.ID)
	if got.EbookMonitored || got.AudiobookMonitored {
		t.Fatalf("re-scan re-monitored after manual un-monitor: %+v", got)
	}
	if !got.InEbookLibrary || !got.InAudiobookLibrary {
		t.Fatalf("membership lost after un-monitor: %+v", got)
	}
}

// TestMirrorOffKeepsFormatsIndependent: the default (mirror off) writes only the
// named format — neither a library toggle nor owning a file bleeds across.
func TestMirrorOffKeepsFormatsIndependent(t *testing.T) {
	s := newTestStore(t)
	a := &Author{Source: "t", ForeignID: "a1", Name: "Terry"}
	if err := s.UpsertAuthor(a); err != nil {
		t.Fatal(err)
	}
	b := &Book{AuthorID: a.ID, Source: "t", ForeignID: "b1", Title: "Mort"}
	if err := s.UpsertBook(b); err != nil {
		t.Fatal(err)
	}

	if err := s.SetBookLibrary(b.ID, "ebook", true, true); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetBook(b.ID)
	if got.InAudiobookLibrary || got.AudiobookMonitored {
		t.Fatalf("mirror off leaked to audiobook on toggle: %+v", got)
	}
	if err := s.EnsureBookLibrary(b.ID, "ebook"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetBook(b.ID)
	if got.InAudiobookLibrary {
		t.Fatalf("mirror off: owning ebook pulled in audiobook: %+v", got)
	}
}
