package library

import "testing"

// TestMatchAudiobookNarrators: a probed file runtime picks the closest audiobook
// edition's narrator, but only within tolerance — an off-by-a-couple-minutes
// file matches, a wildly-off one names no one (left for a person to resolve).
func TestMatchAudiobookNarrators(t *testing.T) {
	s := newTestStore(t)
	res, err := s.db.Exec(`INSERT INTO root_folders (media_type, path) VALUES ('audiobook', ?)`, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rootID, _ := res.LastInsertId()

	a := &Author{Source: "t", ForeignID: "a1", Name: "Andy Weir"}
	if err := s.UpsertAuthor(a); err != nil {
		t.Fatal(err)
	}
	b := &Book{AuthorID: a.ID, Source: "t", ForeignID: "b1", Title: "Project Hail Mary"}
	if err := s.UpsertBook(b); err != nil {
		t.Fatal(err)
	}
	for _, e := range []*Edition{
		{BookID: b.ID, Source: "audible", ForeignID: "EN1", Format: "audiobook", Narrator: "Ray Porter", RuntimeMinutes: 970},
		{BookID: b.ID, Source: "audible", ForeignID: "ABR", Format: "audiobook", Narrator: "Quick Reader", RuntimeMinutes: 300},
	} {
		if err := s.UpsertEdition(e); err != nil {
			t.Fatal(err)
		}
	}
	f := &BookFile{RootFolderID: rootID, BookID: b.ID, MediaType: "audiobook", Path: "/audio/phm.m4b", Format: "m4b"}
	if err := s.UpsertBookFile(f); err != nil {
		t.Fatal(err)
	}

	// 968 minutes ≈ the 970-minute Ray Porter edition (not the 300m abridged).
	if err := s.MatchAudiobookNarrators(b.ID, func(string) (int, error) { return 968, nil }, nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetBookFile(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RuntimeMinutes != 968 || got.Narrator != "Ray Porter" {
		t.Fatalf("match = %dm / %q, want 968 / Ray Porter", got.RuntimeMinutes, got.Narrator)
	}

	// 600 minutes is far from both editions → runtime recorded, no narrator.
	if err := s.MatchAudiobookNarrators(b.ID, func(string) (int, error) { return 600, nil }, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetBookFile(f.ID)
	if got.RuntimeMinutes != 600 || got.Narrator != "" {
		t.Fatalf("off-match = %dm / %q, want 600 / \"\" (neither edition within tolerance)", got.RuntimeMinutes, got.Narrator)
	}
}

// TestMatchAudiobookNarratorFromFileTags: what the file itself declares (a tag /
// .opf narrator) wins over duration-matching — the case where the owned edition
// isn't in any catalog.
func TestMatchAudiobookNarratorFromFileTags(t *testing.T) {
	s := newTestStore(t)
	res, err := s.db.Exec(`INSERT INTO root_folders (media_type, path) VALUES ('audiobook', ?)`, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rootID, _ := res.LastInsertId()
	a := &Author{Source: "t", ForeignID: "a1", Name: "David Brin"}
	if err := s.UpsertAuthor(a); err != nil {
		t.Fatal(err)
	}
	b := &Book{AuthorID: a.ID, Source: "t", ForeignID: "b1", Title: "Brightness Reef"}
	if err := s.UpsertBook(b); err != nil {
		t.Fatal(err)
	}
	// A catalog edition at 968m — duration alone would pick Ray Porter.
	if err := s.UpsertEdition(&Edition{BookID: b.ID, Source: "audible", ForeignID: "E", Format: "audiobook", Narrator: "Ray Porter", RuntimeMinutes: 968}); err != nil {
		t.Fatal(err)
	}
	f := &BookFile{RootFolderID: rootID, BookID: b.ID, MediaType: "audiobook", Path: "/audio/br.m4b", Format: "m4b"}
	if err := s.UpsertBookFile(f); err != nil {
		t.Fatal(err)
	}

	// The file names its own narrator; that wins over the 968m match.
	if err := s.MatchAudiobookNarrators(b.ID,
		func(string) (int, error) { return 968, nil },
		func(string) string { return "Erin Jones" }); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetBookFile(f.ID)
	if got.Narrator != "Erin Jones" {
		t.Fatalf("narrator = %q, want Erin Jones (file's own tag wins over duration-match)", got.Narrator)
	}
	if got.RuntimeMinutes != 968 {
		t.Errorf("runtime = %d, want 968 (still recorded)", got.RuntimeMinutes)
	}
}

// TestBestNarratorAmbiguity: two distinct narrators of near-equal length can't
// be told apart by duration, so no one is named — but a clear winner, a
// single in-tolerance narrator, or the same narrator twice all resolve.
func TestBestNarratorAmbiguity(t *testing.T) {
	pp := []Edition{
		{Format: "audiobook", RuntimeMinutes: 734, Narrator: "Stevie Zimmerman"},
		{Format: "audiobook", RuntimeMinutes: 734, Narrator: "Elizabeth Grace"},
		{Format: "audiobook", RuntimeMinutes: 695, Narrator: "Rosamund Pike"},
	}
	if got := bestNarrator(734, pp); got != "" {
		t.Errorf("bestNarrator(734) = %q, want \"\" (two 734-min narrators tie)", got)
	}
	if got := bestNarrator(695, pp); got != "Rosamund Pike" {
		t.Errorf("bestNarrator(695) = %q, want Rosamund Pike (only one in tolerance)", got)
	}

	clear := []Edition{
		{Format: "audiobook", RuntimeMinutes: 970, Narrator: "Ray Porter"},
		{Format: "audiobook", RuntimeMinutes: 900, Narrator: "Someone Else"},
	}
	if got := bestNarrator(968, clear); got != "Ray Porter" {
		t.Errorf("bestNarrator(968) = %q, want Ray Porter (clear winner)", got)
	}

	same := []Edition{
		{Format: "audiobook", RuntimeMinutes: 734, Narrator: "Scott Brick"},
		{Format: "audiobook", RuntimeMinutes: 736, Narrator: "Scott Brick"},
	}
	if got := bestNarrator(735, same); got != "Scott Brick" {
		t.Errorf("bestNarrator(735) = %q, want Scott Brick (same narrator is not ambiguous)", got)
	}
}

// TestRetireSourceEditions: re-enrichment prunes a source's editions no longer
// returned (an earlier over-broad match), leaving other sources untouched.
func TestRetireSourceEditions(t *testing.T) {
	s := newTestStore(t)
	a := &Author{Source: "t", ForeignID: "a1", Name: "Frank Herbert"}
	if err := s.UpsertAuthor(a); err != nil {
		t.Fatal(err)
	}
	b := &Book{AuthorID: a.ID, Source: "t", ForeignID: "b1", Title: "Dune"}
	if err := s.UpsertBook(b); err != nil {
		t.Fatal(err)
	}
	for _, e := range []*Edition{
		{BookID: b.ID, Source: "audible", ForeignID: "D1", Format: "audiobook", Narrator: "Scott Brick"},
		{BookID: b.ID, Source: "audible", ForeignID: "DM", Format: "audiobook", Narrator: "Wrong Book"}, // stale/wrong
		{BookID: b.ID, Source: "hardcover", ForeignID: "hc1", Format: "ebook"},
	} {
		if err := s.UpsertEdition(e); err != nil {
			t.Fatal(err)
		}
	}

	// Re-enrichment keeps only D1 from audible.
	if err := s.RetireSourceEditions(b.ID, "audible", []string{"D1"}); err != nil {
		t.Fatal(err)
	}
	eds, err := s.ListEditions(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	var audible, hardcover []string
	for _, e := range eds {
		if e.Source == "audible" {
			audible = append(audible, e.ForeignID)
		} else {
			hardcover = append(hardcover, e.ForeignID)
		}
	}
	if len(audible) != 1 || audible[0] != "D1" {
		t.Errorf("audible editions = %v, want [D1] (DM retired)", audible)
	}
	if len(hardcover) != 1 {
		t.Errorf("hardcover editions = %v, want the ebook untouched", hardcover)
	}
}
