package refresh

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/librinode/librinode/internal/database"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/metadata"
)

// TestSyncSeriesFetchesSiblingTitles: adding a series searches the provider
// for its own title and keeps the OTHER results it surfaces — e.g. "Dragon
// Ball Super" alongside "Dragon Ball" — as SiblingTitles, excluding itself
// and anything with the same normalized title.
func TestSyncSeriesFetchesSiblingTitles(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := library.NewStore(db)

	p := &fakeSeriesProvider{
		name: "anifake",
		search: []metadata.SeriesResult{
			{ForeignID: "db", Title: "Dragon Ball"},        // itself — excluded
			{ForeignID: "db", Title: "dragon ball"},        // itself, different case — still excluded
			{ForeignID: "dbs", Title: "Dragon Ball Super"}, // a real sibling
			{ForeignID: "dbgt", Title: "Dragon Ball GT"},   // another real sibling
		},
		series: map[string]*metadata.SeriesResult{
			"db": {ForeignID: "db", Title: "Dragon Ball", IssueCount: 1,
				Issues: []metadata.Issue{{ForeignID: "db-v1", Number: 1}}},
		},
	}
	mgr := metadata.NewManager()
	mgr.SetSeries(p)
	svc := New(store, mgr)

	added, err := svc.SyncSeries(context.Background(), "manga", "db", true, true, true)
	if err != nil {
		t.Fatalf("SyncSeries: %v", err)
	}
	want := map[string]bool{"Dragon Ball Super": true, "Dragon Ball GT": true}
	if len(added.SiblingTitles) != len(want) {
		t.Fatalf("SiblingTitles = %v, want %v", added.SiblingTitles, want)
	}
	for _, s := range added.SiblingTitles {
		if !want[s] {
			t.Errorf("unexpected sibling %q", s)
		}
	}

	// Persisted, not just returned in-memory.
	got, err := store.GetSeries(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SiblingTitles) != 2 {
		t.Errorf("stored SiblingTitles = %v, want 2 entries", got.SiblingTitles)
	}
}

// TestRefreshSeriesPreservesSiblingsOnSearchFailure: a refresh whose sibling
// search comes back empty (provider hiccup, or genuinely nothing found this
// time) must not erase a previously discovered sibling list — see
// UpsertSeries's preserve-on-empty handling.
func TestRefreshSeriesPreservesSiblingsOnSearchFailure(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := library.NewStore(db)

	p := &fakeSeriesProvider{
		name:   "anifake",
		search: []metadata.SeriesResult{{ForeignID: "dbs", Title: "Dragon Ball Super"}},
		series: map[string]*metadata.SeriesResult{
			"db": {ForeignID: "db", Title: "Dragon Ball", IssueCount: 1,
				Issues: []metadata.Issue{{ForeignID: "db-v1", Number: 1}}},
		},
	}
	mgr := metadata.NewManager()
	mgr.SetSeries(p)
	svc := New(store, mgr)

	added, err := svc.SyncSeries(context.Background(), "manga", "db", true, true, true)
	if err != nil {
		t.Fatalf("SyncSeries: %v", err)
	}
	if len(added.SiblingTitles) != 1 {
		t.Fatalf("initial SiblingTitles = %v, want 1 entry", added.SiblingTitles)
	}

	// The provider's search now returns nothing (a transient hiccup, from
	// this test's point of view) — refreshing must keep the old list.
	p.search = nil
	if err := svc.RefreshSeries(context.Background(), added.ID); err != nil {
		t.Fatalf("RefreshSeries: %v", err)
	}
	got, err := store.GetSeries(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SiblingTitles) != 1 || got.SiblingTitles[0] != "Dragon Ball Super" {
		t.Errorf("SiblingTitles after empty-search refresh = %v, want [Dragon Ball Super] preserved", got.SiblingTitles)
	}
}

// fakeRelatedSeriesProvider adds metadata.RelatedSeriesProvider to
// fakeSeriesProvider — a provider whose relations graph is authoritative,
// not just inferred from title search (AniList; ComicVine has no
// equivalent, so it only ever satisfies the base SeriesProvider).
type fakeRelatedSeriesProvider struct {
	fakeSeriesProvider
	related map[string][]metadata.SeriesResult
}

func (f *fakeRelatedSeriesProvider) RelatedSeries(_ context.Context, foreignID string) ([]metadata.SeriesResult, error) {
	return f.related[foreignID], nil
}

// TestSyncSeriesMergesRelatedSeriesWithSearch: a provider whose relations
// graph names a sibling the title search doesn't surface at all (no textual
// overlap) still gets it into SiblingTitles — and a sibling present in both
// sources isn't duplicated.
func TestSyncSeriesMergesRelatedSeriesWithSearch(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := library.NewStore(db)

	p := &fakeRelatedSeriesProvider{
		fakeSeriesProvider: fakeSeriesProvider{
			name: "anifake",
			search: []metadata.SeriesResult{
				{ForeignID: "db", Title: "Dragon Ball"},        // itself — excluded
				{ForeignID: "dbs", Title: "Dragon Ball Super"}, // found by both sources
			},
			series: map[string]*metadata.SeriesResult{
				"db": {ForeignID: "db", Title: "Dragon Ball", IssueCount: 1,
					Issues: []metadata.Issue{{ForeignID: "db-v1", Number: 1}}},
			},
		},
		related: map[string][]metadata.SeriesResult{
			"db": {
				{ForeignID: "dbs", Title: "Dragon Ball Super"},           // duplicate of the search hit
				{ForeignID: "cross", Title: "A Totally Unrelated Title"}, // search alone would never find this
			},
		},
	}
	mgr := metadata.NewManager()
	mgr.SetSeries(p)
	svc := New(store, mgr)

	added, err := svc.SyncSeries(context.Background(), "manga", "db", true, true, true)
	if err != nil {
		t.Fatalf("SyncSeries: %v", err)
	}
	want := map[string]bool{"Dragon Ball Super": true, "A Totally Unrelated Title": true}
	if len(added.SiblingTitles) != len(want) {
		t.Fatalf("SiblingTitles = %v, want %v (no duplicate for the overlapping sibling)", added.SiblingTitles, want)
	}
	for _, s := range added.SiblingTitles {
		if !want[s] {
			t.Errorf("unexpected sibling %q", s)
		}
	}
}

// TestSyncSeriesDropsSelfDuplicateUnderDifferentScript: a search result that
// is really the SAME work under a different surface form — here a
// non-Latin-script catalog duplicate — must never be stored as a sibling,
// even though its raw text differs from self's title. Reproduced live:
// "うずまき [Uzumaki]" normalizes (scanner.Normalize strips what isn't
// a-z0-9) down to just "uzumaki", identical to self. Before this fix, that
// one-word "sibling" matched the first word of every real Uzumaki release in
// release.matchesKnownSibling, silently rejecting all of them as if each one
// named a different work.
func TestSyncSeriesDropsSelfDuplicateUnderDifferentScript(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := library.NewStore(db)

	p := &fakeSeriesProvider{
		name: "hardcoverfake",
		search: []metadata.SeriesResult{
			{ForeignID: "uz", Title: "Uzumaki"},                         // itself — excluded
			{ForeignID: "uz-jp", Title: "うずまき [Uzumaki]"},               // same work, different script — must be dropped
			{ForeignID: "uz-alt", Title: "Uzumaki: Spiral into Horror"}, // real, longer sibling — kept
		},
		series: map[string]*metadata.SeriesResult{
			"uz": {ForeignID: "uz", Title: "Uzumaki", IssueCount: 1,
				Issues: []metadata.Issue{{ForeignID: "uz-v1", Number: 1}}},
		},
	}
	mgr := metadata.NewManager()
	mgr.SetSeries(p)
	svc := New(store, mgr)

	added, err := svc.SyncSeries(context.Background(), "manga", "uz", true, true, true)
	if err != nil {
		t.Fatalf("SyncSeries: %v", err)
	}
	want := map[string]bool{"Uzumaki: Spiral into Horror": true}
	if len(added.SiblingTitles) != len(want) {
		t.Fatalf("SiblingTitles = %v, want %v (same-work duplicate dropped)", added.SiblingTitles, want)
	}
	for _, s := range added.SiblingTitles {
		if !want[s] {
			t.Errorf("unexpected sibling %q", s)
		}
	}
}

// TestSyncSeriesStripsParentheticalFromSiblingSearchQuery: a catalog title
// with a trailing parenthetical ("Parasyte (8-Volume Edition)") must not go
// to the provider's own title search verbatim either — reproduced live, that
// exact query found zero siblings, silently missing the real "Parasyte
// Reversi" spin-off, which would then have passed seriesTitleMatches
// unchallenged as if it were the original on the next individual-volume
// search. The query is widened; self-exclusion still compares against the
// full, untouched self.Title.
func TestSyncSeriesStripsParentheticalFromSiblingSearchQuery(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := library.NewStore(db)

	var gotQuery string
	p := &fakeSeriesProvider{
		name:     "hardcoverfake",
		gotQuery: &gotQuery,
		search: []metadata.SeriesResult{
			{ForeignID: "para", Title: "Parasyte (8-Volume Edition)"}, // itself — excluded
			{ForeignID: "reversi", Title: "Parasyte Reversi"},         // real, distinct spin-off
		},
		series: map[string]*metadata.SeriesResult{
			"para": {ForeignID: "para", Title: "Parasyte (8-Volume Edition)", IssueCount: 1,
				Issues: []metadata.Issue{{ForeignID: "para-v1", Number: 1}}},
		},
	}
	mgr := metadata.NewManager()
	mgr.SetSeries(p)
	svc := New(store, mgr)

	added, err := svc.SyncSeries(context.Background(), "manga", "para", true, true, true)
	if err != nil {
		t.Fatalf("SyncSeries: %v", err)
	}
	if gotQuery != "Parasyte" {
		t.Errorf("sibling-search query = %q, want %q", gotQuery, "Parasyte")
	}
	want := map[string]bool{"Parasyte Reversi": true}
	if len(added.SiblingTitles) != len(want) {
		t.Fatalf("SiblingTitles = %v, want %v", added.SiblingTitles, want)
	}
	for _, s := range added.SiblingTitles {
		if !want[s] {
			t.Errorf("unexpected sibling %q", s)
		}
	}
}

// TestSyncSeriesDropsNonOverlappingSearchResults: a provider's title search
// can surface loosely-relevant noise alongside true siblings (Hardcover more
// than AniList, in practice) — a result sharing no words with self's own
// title can never actually match release.matchesKnownSibling (which only
// checks a sibling's words against a release already confirmed to start with
// self's own), so it's dropped as inert rather than stored.
func TestSyncSeriesDropsNonOverlappingSearchResults(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := library.NewStore(db)

	p := &fakeSeriesProvider{
		name: "hardcoverfake",
		search: []metadata.SeriesResult{
			{ForeignID: "dn", Title: "Death Note"},                  // itself — excluded
			{ForeignID: "dnbe", Title: "Death Note: Black Edition"}, // real sibling, shares "Death"
			{ForeignID: "noise1", Title: "Detective Ruby Preston"},  // noise, no overlap — dropped
			{ForeignID: "noise2", Title: "Pliny the Younger"},       // noise, no overlap — dropped
		},
		series: map[string]*metadata.SeriesResult{
			"dn": {ForeignID: "dn", Title: "Death Note", IssueCount: 1,
				Issues: []metadata.Issue{{ForeignID: "dn-v1", Number: 1}}},
		},
	}
	mgr := metadata.NewManager()
	mgr.SetSeries(p)
	svc := New(store, mgr)

	added, err := svc.SyncSeries(context.Background(), "manga", "dn", true, true, true)
	if err != nil {
		t.Fatalf("SyncSeries: %v", err)
	}
	want := map[string]bool{"Death Note: Black Edition": true}
	if len(added.SiblingTitles) != len(want) {
		t.Fatalf("SiblingTitles = %v, want %v (noise dropped)", added.SiblingTitles, want)
	}
	for _, s := range added.SiblingTitles {
		if !want[s] {
			t.Errorf("unexpected sibling %q", s)
		}
	}
}
