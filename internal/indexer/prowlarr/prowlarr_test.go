package prowlarr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/librinode/librinode/internal/indexer"
)

type mockOpts struct{ indexerListFails bool }

// mockProwlarr fakes the three Prowlarr endpoints the source uses. The two
// enabled sub-indexers return one torrent and one usenet result; the disabled
// one must never be queried.
func mockProwlarr(t *testing.T, opts mockOpts) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "pk" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"bad key"}`))
			return
		}
		switch r.URL.Path {
		case "/api/v1/system/status":
			w.Write([]byte(`{"version":"1.0"}`))
		case "/api/v1/indexer":
			if opts.indexerListFails {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write([]byte(`[
				{"id":1,"name":"Anna","enable":true},
				{"id":2,"name":"Disabled","enable":false},
				{"id":3,"name":"UsenetOne","enable":true}
			]`))
		case "/api/v1/search":
			switch r.URL.Query().Get("indexerIds") {
			case "1":
				w.Write([]byte(`[{"guid":"g-torrent","title":"Dune EPUB","size":1000,"indexer":"Anna","protocol":"torrent","downloadUrl":"http://x/torrent","seeders":10,"leechers":2,"categories":[{"id":7020}]}]`))
			case "3":
				w.Write([]byte(`[{"guid":"g-usenet","title":"Dune Usenet","size":2000,"indexer":"UsenetOne","protocol":1,"downloadUrl":"http://x/nzb"}]`))
			case "2":
				t.Error("disabled sub-indexer 2 must not be queried")
				w.Write([]byte(`[]`))
			default: // aggregate (no indexerIds)
				w.Write([]byte(`[{"guid":"g-agg","title":"Dune Aggregate","size":3000,"indexer":"Anna","protocol":"torrent","downloadUrl":"http://x/agg","seeders":5,"categories":[{"id":7020}]}]`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newSearcher(t *testing.T, id int64, baseURL string) indexer.Searcher {
	t.Helper()
	ind := &indexer.Indexer{ID: id, Name: "Prowlarr", Type: "prowlarr", BaseURL: baseURL, APIKey: "pk"}
	return Def().New(ind, http.DefaultClient)
}

// TestProwlarrSearchFansOutMixedProtocols: Search queries each enabled
// sub-indexer, skips the disabled one, and preserves each result's own
// protocol — a torrent keeps its seeders, a usenet result reports -1.
func TestProwlarrSearchFansOutMixedProtocols(t *testing.T) {
	srv := mockProwlarr(t, mockOpts{})
	defer srv.Close()
	s := newSearcher(t, 1, srv.URL)

	rels, err := s.Search(context.Background(), "dune", "ebook")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 2 {
		t.Fatalf("got %d releases, want 2 (from the two enabled sub-indexers)", len(rels))
	}
	var gotTorrent, gotUsenet bool
	for _, r := range rels {
		switch r.Protocol {
		case indexer.ProtocolTorrent:
			gotTorrent = true
			if r.Seeders != 10 {
				t.Errorf("torrent seeders = %d, want 10", r.Seeders)
			}
			if !strings.Contains(r.Indexer, "Anna") {
				t.Errorf("indexer label = %q, want it to name the sub-indexer", r.Indexer)
			}
		case indexer.ProtocolUsenet:
			gotUsenet = true
			if r.Seeders != -1 {
				t.Errorf("usenet seeders = %d, want -1 (n/a)", r.Seeders)
			}
		default:
			t.Errorf("unexpected protocol %q", r.Protocol)
		}
	}
	if !gotTorrent || !gotUsenet {
		t.Errorf("want both a torrent and a usenet result; got torrent=%v usenet=%v", gotTorrent, gotUsenet)
	}
}

// TestProwlarrSearchFallsBackToAggregate: if the sub-indexer list can't be
// fetched, Search uses Prowlarr's single aggregate call instead of failing.
func TestProwlarrSearchFallsBackToAggregate(t *testing.T) {
	srv := mockProwlarr(t, mockOpts{indexerListFails: true})
	defer srv.Close()
	s := newSearcher(t, 2, srv.URL)

	rels, err := s.Search(context.Background(), "dune", "ebook")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 1 || rels[0].GUID != "g-agg" {
		t.Fatalf("got %+v, want the single aggregate result when the sub-indexer list fails", rels)
	}
}

func TestProwlarrTest(t *testing.T) {
	srv := mockProwlarr(t, mockOpts{})
	defer srv.Close()
	if err := newSearcher(t, 3, srv.URL).Test(context.Background()); err != nil {
		t.Fatalf("Test() = %v, want nil", err)
	}
}

// TestProwlarrSkipsUnresolvableProtocol: a result whose protocol isn't
// torrent or usenet (absent, or an unrecognized value) can't be routed to a
// download client, so it's dropped rather than surfaced as an un-grabbable
// candidate autosearch would re-pick every sweep.
func TestProwlarrSkipsUnresolvableProtocol(t *testing.T) {
	body := []byte(`[
		{"guid":"ok","title":"Good","protocol":"torrent","downloadUrl":"http://x/t"},
		{"guid":"none","title":"NoProtocol","downloadUrl":"http://x/n"},
		{"guid":"weird","title":"Unknown","protocol":"carrier-pigeon","downloadUrl":"http://x/w"}
	]`)
	rels, err := parseReleases(body, &indexer.Indexer{ID: 1, Name: "Prowlarr"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 1 || rels[0].GUID != "ok" {
		t.Fatalf("parseReleases = %+v, want only the torrent result (unresolvable protocols dropped)", rels)
	}
}

func TestProwlarrCategoriesPerMediaType(t *testing.T) {
	cases := map[string]string{
		"ebook":     "7000,7020",
		"audiobook": "3030",
		"comic":     "7030",
		"manga":     "7030",
		"magazine":  "7010",
		"":          "7000,7020",
	}
	for mt, want := range cases {
		if got := categoriesFor(mt); got != want {
			t.Errorf("categoriesFor(%q) = %q, want %q", mt, got, want)
		}
	}
}
