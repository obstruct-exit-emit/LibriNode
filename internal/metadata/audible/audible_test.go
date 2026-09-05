package audible

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A search response shaped like Audible's catalog API, with an exact English
// unabridged match, an abridged one, a wrong-author decoy, and an Italian
// edition — enough to exercise parsing, filtering, and ordering.
const sampleResponse = `{"products":[
  {"asin":"IT1","title":"Project Hail Mary (Italian edition)","authors":[{"name":"Andy Weir"}],"narrators":[{"name":"William Angiuli"}],"runtime_length_min":1025,"format_type":"unabridged","language":"italian","product_images":{"500":"https://img/it.jpg"}},
  {"asin":"EN1","title":"Project Hail Mary","authors":[{"name":"Andy Weir"}],"narrators":[{"name":"Ray Porter"}],"runtime_length_min":970,"format_type":"unabridged","publisher_name":"Audible Studios","language":"english","release_date":"2021-05-04","product_images":{"500":"https://img/en.jpg"}},
  {"asin":"WRONG","title":"Project Hail Mary","authors":[{"name":"Somebody Else"}],"narrators":[{"name":"Nobody"}],"runtime_length_min":600,"format_type":"unabridged","language":"english"},
  {"asin":"ABR","title":"Project Hail Mary","authors":[{"name":"Andy Weir"}],"narrators":[{"name":"Ray Porter"}],"runtime_length_min":300,"format_type":"abridged","language":"english"}
]}`

func serve(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFindEditionsParsesFiltersOrders(t *testing.T) {
	srv := serve(t, sampleResponse)
	c := New(WithEndpoint(srv.URL), WithLanguage("english"))

	eds, err := c.FindEditions(context.Background(), "Project Hail Mary", "Andy Weir")
	if err != nil {
		t.Fatal(err)
	}
	// Wrong author dropped, and the Italian edition dropped by the English
	// language preference — the two English Andy Weir editions remain.
	if len(eds) != 2 {
		t.Fatalf("editions = %d, want 2 (wrong author + non-English filtered): %+v", len(eds), eds)
	}

	// English unabridged sorts first, fully mapped.
	e := eds[0]
	if e.ASIN != "EN1" || e.ForeignID != "EN1" {
		t.Errorf("first edition = %q, want EN1", e.ASIN)
	}
	if e.Format != "audiobook" {
		t.Errorf("format = %q, want audiobook", e.Format)
	}
	if e.Narrator != "Ray Porter" {
		t.Errorf("narrator = %q, want Ray Porter", e.Narrator)
	}
	if e.RuntimeMinutes != 970 {
		t.Errorf("runtime = %d, want 970", e.RuntimeMinutes)
	}
	if e.Abridged {
		t.Errorf("unabridged edition marked abridged")
	}
	if e.CoverURL != "https://img/en.jpg" {
		t.Errorf("cover = %q, want https://img/en.jpg", e.CoverURL)
	}
	if e.Publisher != "Audible Studios" || e.Language != "english" || e.ReleaseDate != "2021-05-04" {
		t.Errorf("metadata mismatch: %+v", e)
	}

	// The abridged English edition is second and flagged.
	if eds[1].ASIN != "ABR" || !eds[1].Abridged {
		t.Errorf("second = %+v, want ABR abridged", eds[1])
	}
}

// TestFindEditionsLanguagePreference: a configured language keeps that language
// (and unknown-language editions) and drops the others — but falls back to
// everything when the work has no edition in the preferred language.
func TestFindEditionsLanguagePreference(t *testing.T) {
	body := `{"products":[
	  {"asin":"EN","title":"Dune","authors":[{"name":"Frank Herbert"}],"narrators":[{"name":"Scott Brick"}],"runtime_length_min":1262,"format_type":"unabridged","language":"english"},
	  {"asin":"DE","title":"Dune","authors":[{"name":"Frank Herbert"}],"narrators":[{"name":"Mark Bremer"}],"runtime_length_min":1484,"format_type":"unabridged","language":"german"},
	  {"asin":"ES","title":"Dune","authors":[{"name":"Frank Herbert"}],"narrators":[{"name":"Daniel Garcia"}],"runtime_length_min":1540,"format_type":"unabridged","language":"spanish"}
	]}`
	eds, err := New(WithEndpoint(serve(t, body).URL), WithLanguage("english")).
		FindEditions(context.Background(), "Dune", "Frank Herbert")
	if err != nil {
		t.Fatal(err)
	}
	if len(eds) != 1 || eds[0].ASIN != "EN" {
		t.Fatalf("editions = %+v, want only the English one", eds)
	}

	// A language with no match at all falls back to every edition, not none.
	eds, err = New(WithEndpoint(serve(t, body).URL), WithLanguage("japanese")).
		FindEditions(context.Background(), "Dune", "Frank Herbert")
	if err != nil {
		t.Fatal(err)
	}
	if len(eds) != 3 {
		t.Fatalf("editions = %d, want 3 (fallback when no preferred-language match)", len(eds))
	}
}

func TestFindEditionsJoinsNarrators(t *testing.T) {
	srv := serve(t, `{"products":[{"asin":"A1","title":"Good Omens","authors":[{"name":"Neil Gaiman"}],"narrators":[{"name":"Michael Sheen"},{"name":"David Tennant"}],"runtime_length_min":746,"format_type":"unabridged","language":"english"}]}`)
	c := New(WithEndpoint(srv.URL), WithLanguage("english"))

	eds, err := c.FindEditions(context.Background(), "Good Omens", "Neil Gaiman")
	if err != nil {
		t.Fatal(err)
	}
	if len(eds) != 1 {
		t.Fatalf("editions = %d, want 1", len(eds))
	}
	if eds[0].Narrator != "Michael Sheen, David Tennant" {
		t.Errorf("narrator = %q, want joined 'Michael Sheen, David Tennant'", eds[0].Narrator)
	}
}

// TestFindEditionsRejectsLongerTitles: a search for "Dune" must not attribute
// "Dune Messiah" (a different book) as an edition of it, while still accepting a
// subtitle/edition tail like "Dune (Unabridged)".
func TestFindEditionsRejectsLongerTitles(t *testing.T) {
	srv := serve(t, `{"products":[
	  {"asin":"D1","title":"Dune","authors":[{"name":"Frank Herbert"}],"narrators":[{"name":"Scott Brick"}],"runtime_length_min":1262,"format_type":"unabridged","language":"english"},
	  {"asin":"DM","title":"Dune Messiah","authors":[{"name":"Frank Herbert"}],"narrators":[{"name":"Scott Brick"}],"runtime_length_min":536,"format_type":"unabridged","language":"english"},
	  {"asin":"DU","title":"Dune (Unabridged)","authors":[{"name":"Frank Herbert"}],"narrators":[{"name":"Simon Vance"}],"runtime_length_min":1265,"format_type":"unabridged","language":"english"}
	]}`)
	c := New(WithEndpoint(srv.URL), WithLanguage("english"))

	eds, err := c.FindEditions(context.Background(), "Dune", "Frank Herbert")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range eds {
		if e.ASIN == "DM" {
			t.Errorf("'Dune Messiah' wrongly matched as an edition of 'Dune'")
		}
	}
	if len(eds) != 2 {
		t.Fatalf("editions = %d, want 2 (Dune + Dune Unabridged, not Dune Messiah): %+v", len(eds), eds)
	}
}

// TestFindEditionsMergesQueryVariants: Audible ranks "title author" and the
// bare "title" differently — one narration surfaces only in the second. The
// merge must return both.
func TestFindEditionsMergesQueryVariants(t *testing.T) {
	wilson := `{"asin":"W","title":"Brightness Reef","authors":[{"name":"David Brin"}],"narrators":[{"name":"George K. Wilson"}],"runtime_length_min":1548,"format_type":"unabridged","language":"english"}`
	berkrot := `{"asin":"B","title":"Brightness Reef","authors":[{"name":"David Brin"}],"narrators":[{"name":"Peter Berkrot"}],"runtime_length_min":774,"format_type":"unabridged","language":"english"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(strings.ToLower(r.URL.Query().Get("keywords")), "brin") {
			_, _ = w.Write([]byte(`{"products":[` + wilson + `]}`)) // "title author" — only Wilson
		} else {
			_, _ = w.Write([]byte(`{"products":[` + wilson + `,` + berkrot + `]}`)) // bare title — both
		}
	}))
	defer srv.Close()

	eds, err := New(WithEndpoint(srv.URL), WithLanguage("english")).
		FindEditions(context.Background(), "Brightness Reef", "David Brin")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range eds {
		got[e.Narrator] = true
	}
	if !got["George K. Wilson"] || !got["Peter Berkrot"] {
		t.Fatalf("editions = %+v, want both narrators (merged from both query variants)", eds)
	}
}

func TestFindEditionsNoAuthorStillMatchesOnTitle(t *testing.T) {
	srv := serve(t, sampleResponse)
	c := New(WithEndpoint(srv.URL))
	eds, err := c.FindEditions(context.Background(), "Project Hail Mary", "")
	if err != nil {
		t.Fatal(err)
	}
	// With no author to filter on, every title match is kept (all four).
	if len(eds) != 4 {
		t.Fatalf("editions = %d, want 4 (no author filter): %+v", len(eds), eds)
	}
}
