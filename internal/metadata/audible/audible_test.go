package audible

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	// The wrong-author decoy is filtered; three Andy Weir editions remain.
	if len(eds) != 3 {
		t.Fatalf("editions = %d, want 3 (wrong author filtered): %+v", len(eds), eds)
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

	// The abridged English edition is second and flagged; the Italian is last.
	if eds[1].ASIN != "ABR" || !eds[1].Abridged {
		t.Errorf("second = %+v, want ABR abridged", eds[1])
	}
	if eds[2].ASIN != "IT1" {
		t.Errorf("third = %q, want IT1 (other language ranks last)", eds[2].ASIN)
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
