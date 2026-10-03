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
