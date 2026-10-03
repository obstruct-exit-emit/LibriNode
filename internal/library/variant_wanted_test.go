package library

import "testing"

// TestVariantMissing covers all four TargetVariant branches directly.
func TestVariantMissing(t *testing.T) {
	cases := []struct {
		name             string
		target           string
		mono, color, any bool
		want             bool
	}{
		{"blind default, no file", "", false, false, false, true},
		{"blind default, any file satisfies it", "", true, false, true, false},
		{"mono wanted, missing", "mono", false, true, true, true},
		{"mono wanted, owned", "mono", true, false, true, false},
		{"color wanted, missing", "color", true, false, true, true},
		{"color wanted, owned", "color", false, true, true, false},
		{"both wanted, owns mono only", "both", true, false, true, true},
		{"both wanted, owns color only", "both", false, true, true, true},
		{"both wanted, owns neither", "both", false, false, false, true},
		{"both wanted, owns both", "both", true, true, true, false},
	}
	for _, c := range cases {
		if got := VariantMissing(c.target, c.mono, c.color, c.any); got != c.want {
			t.Errorf("%s: VariantMissing(%q, mono=%v, color=%v, any=%v) = %v, want %v",
				c.name, c.target, c.mono, c.color, c.any, got, c.want)
		}
	}
}

// TestWantedRespectsSeriesTargetVariant: a manga series opted into "both"
// keeps a volume wanted until it owns both variants — not just one, the
// variant-blind default's rule — and a series that never opts in behaves
// exactly as before.
func TestWantedRespectsSeriesTargetVariant(t *testing.T) {
	s := newTestStore(t)
	author := &Author{Source: "anilist", ForeignID: "creator:x", Name: "X"}
	if err := s.UpsertAuthor(author); err != nil {
		t.Fatal(err)
	}
	series := &Series{Source: "anilist", ForeignID: "s1", Title: "Berserk", MediaType: "manga", Monitored: true}
	if err := s.UpsertSeries(series); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSeriesTargetVariant(series.ID, "both"); err != nil {
		t.Fatal(err)
	}
	book := &Book{AuthorID: author.ID, Source: "anilist", MediaType: "manga",
		ForeignID: "s1-v1", Title: "Berserk Vol. 1", Monitored: true}
	if err := s.UpsertBook(book); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkBookSeries(book.ID, series.ID, 1); err != nil {
		t.Fatal(err)
	}

	if _, err := s.db.Exec(`INSERT INTO root_folders (id, media_type, variant, path) VALUES (1, 'manga', 'mono', '/mono')`); err != nil {
		t.Fatal(err)
	}

	assertWanted := func(want bool) {
		t.Helper()
		items, err := s.Wanted("manga")
		if err != nil {
			t.Fatal(err)
		}
		got := len(items) == 1 && items[0].BookID == book.ID
		if got != want {
			t.Fatalf("Wanted(manga) for book %d = %v (items=%+v), want %v", book.ID, got, items, want)
		}
	}

	// No file at all: wanted.
	assertWanted(true)

	// Owns mono only: target_variant=both still wants the missing color.
	if err := s.UpsertBookFile(&BookFile{RootFolderID: 1, BookID: book.ID, MediaType: "manga",
		Variant: "mono", Path: "/mono/Berserk v01.cbz", Format: "cbz"}); err != nil {
		t.Fatal(err)
	}
	assertWanted(true)

	// Owns both: finally satisfied.
	if _, err := s.db.Exec(`INSERT INTO root_folders (id, media_type, variant, path) VALUES (2, 'manga', 'color', '/color')`); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertBookFile(&BookFile{RootFolderID: 2, BookID: book.ID, MediaType: "manga",
		Variant: "color", Path: "/color/Berserk v01.cbz", Format: "cbz"}); err != nil {
		t.Fatal(err)
	}
	assertWanted(false)

	// A series that never opts in (target_variant stays "") is satisfied by
	// either variant alone — today's unchanged default behavior.
	if err := s.SetSeriesTargetVariant(series.ID, ""); err != nil {
		t.Fatal(err)
	}
	// Re-verify against a fresh book owning only one variant.
	book2 := &Book{AuthorID: author.ID, Source: "anilist", MediaType: "manga",
		ForeignID: "s1-v2", Title: "Berserk Vol. 2", Monitored: true}
	if err := s.UpsertBook(book2); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkBookSeries(book2.ID, series.ID, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertBookFile(&BookFile{RootFolderID: 1, BookID: book2.ID, MediaType: "manga",
		Variant: "mono", Path: "/mono/Berserk v02.cbz", Format: "cbz"}); err != nil {
		t.Fatal(err)
	}
	items, err := s.Wanted("manga")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.BookID == book2.ID {
			t.Errorf("book 2 (blind default, owns mono) wrongly still wanted: %+v", it)
		}
	}
}
