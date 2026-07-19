package annasarchive

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/librinode/librinode/internal/indexer"
)

const resultsHTML = `
<html><body>
<a href="/md5/0123456789abcdef0123456789abcdef" class="js-vim-focus">
  <h3 class="truncate">The Left Hand of Darkness</h3>
  <div class="truncate">Ursula K. Le Guin</div>
  <div class="text-gray-500">English [en], epub, 0.4MB, lgli</div>
</a>
<a href="/md5/0123456789abcdef0123456789abcdef"><img src="cover.jpg"></a>
<a href="/md5/ffff6789abcdef0123456789abcdef01">
  <h3>The Dispossessed</h3>
  <div>English [en], pdf, 12.5MB</div>
</a>
</body></html>`

func TestParseResults(t *testing.T) {
	results := parseResults(resultsHTML)
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2 (cover-link duplicate collapsed): %+v", len(results), results)
	}
	first := results[0]
	if first.MD5 != "0123456789abcdef0123456789abcdef" {
		t.Errorf("md5 = %q", first.MD5)
	}
	if first.Title != "The Left Hand of Darkness" {
		t.Errorf("title = %q", first.Title)
	}
	mb := float64(1 << 20)
	if want := int64(0.4 * mb); first.Size != want {
		t.Errorf("size = %d, want %d (~0.4MB)", first.Size, want)
	}
	if results[1].Title != "The Dispossessed" || results[1].Size != int64(12.5*mb) {
		t.Errorf("second = %+v", results[1])
	}
}

// serveResults serves the fixture search page (and a 200 homepage for Test).
func serveResults(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			_, _ = w.Write([]byte(resultsHTML))
			return
		}
		_, _ = w.Write([]byte("<html>aa</html>"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDefAndSearchKeyedVsKeyless(t *testing.T) {
	def := Def()
	if def.Name != "annas-archive" || def.Protocol != indexer.ProtocolDirect {
		t.Fatalf("def = %+v", def)
	}
	if !def.Serves("ebook") || def.Serves("audiobook") {
		t.Error("Anna's should serve ebook only")
	}
	if def.NeedsAPIKey {
		t.Error("the key is optional (keyless = search-only), so NeedsAPIKey must be false")
	}

	srv := serveResults(t)

	// Keyless: downloads route through the open Libgen mirrors by MD5.
	ind := &indexer.Indexer{Name: "AA", BaseURL: srv.URL}
	s := def.New(ind, srv.Client())
	rels, err := s.Search(context.Background(), "le guin", "ebook")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	md5 := "0123456789abcdef0123456789abcdef"
	mirrors := "https://library.lol/main/" + md5 + "|https://libgen.li/ads.php?md5=" + md5
	if len(rels) != 2 || rels[0].DownloadURL != mirrors {
		t.Fatalf("keyless releases = %+v, want 2 with mirror download URLs", rels)
	}
	if rels[0].Protocol != indexer.ProtocolDirect || rels[0].GUID != md5 {
		t.Errorf("release = %+v", rels[0])
	}
	if rels[0].InfoURL != srv.URL+"/md5/"+md5 {
		t.Errorf("info URL = %q", rels[0].InfoURL)
	}

	// With a key: the fast-download API first, the open mirrors as failover.
	ind.APIKey = "sekret+key"
	s = def.New(ind, srv.Client())
	rels, err = s.Search(context.Background(), "le guin", "ebook")
	if err != nil {
		t.Fatalf("keyed Search: %v", err)
	}
	want := srv.URL + "/dyn/api/fast_download.json?md5=" + md5 + "&key=sekret%2Bkey|" + mirrors
	if rels[0].DownloadURL != want {
		t.Errorf("keyed download URL = %q, want %q", rels[0].DownloadURL, want)
	}

	// Non-ebook searches yield nothing; Test passes against the live fixture.
	if got, err := s.Search(context.Background(), "x", "audiobook"); err != nil || got != nil {
		t.Errorf("audiobook search = %v, %v; want nil, nil", got, err)
	}
	if err := s.Test(context.Background()); err != nil {
		t.Errorf("Test: %v", err)
	}
}

// TestSearchFailsOverToMirror: the primary is down; the mirror serves.
func TestSearchFailsOverToMirror(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer dead.Close()
	mirror := serveResults(t)

	s := Def().New(&indexer.Indexer{Name: "AA", BaseURL: dead.URL + "," + mirror.URL}, mirror.Client())
	rels, err := s.Search(context.Background(), "le guin", "ebook")
	if err != nil || len(rels) != 2 {
		t.Fatalf("failover Search = %d releases, %v", len(rels), err)
	}
	// Info URLs point at the mirror that actually served.
	if rels[0].InfoURL != mirror.URL+"/md5/0123456789abcdef0123456789abcdef" {
		t.Errorf("info URL = %q, want the serving mirror's", rels[0].InfoURL)
	}
}
