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
	if err := s.MatchAudiobookNarrators(b.ID, func(string) (int, error) { return 968, nil }); err != nil {
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
	if err := s.MatchAudiobookNarrators(b.ID, func(string) (int, error) { return 600, nil }); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetBookFile(f.ID)
	if got.RuntimeMinutes != 600 || got.Narrator != "" {
		t.Fatalf("off-match = %dm / %q, want 600 / \"\" (neither edition within tolerance)", got.RuntimeMinutes, got.Narrator)
	}
}
