package libgen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/librinode/librinode/internal/indexer"
)

// A non-fiction results table row (search.php) and a fiction row share the
// shape that matters: links carrying md5=, a title anchor, a size cell.
const nonFictionHTML = `
<table class="c"><tr><td>ID</td><td>Author</td><td>Title</td></tr>
<tr>
  <td>1234</td>
  <td><a href="search.php?req=le+guin">Ursula K. Le Guin</a></td>
  <td><a href="book/index.php?md5=0123456789ABCDEF0123456789ABCDEF">The Left Hand of Darkness</a></td>
  <td>1969</td><td>english</td><td>1 MB</td><td>epub</td>
</tr>
</table>`

const fictionHTML = `
<table><tr>
  <td><a href="/fiction/authors/le-guin">Le Guin, Ursula</a></td>
  <td><a href="/fiction/FFFF6789ABCDEF0123456789ABCDEF01">The Dispossessed: An Ambiguous Utopia</a>
      <a href="/ads.php?md5=FFFF6789ABCDEF0123456789ABCDEF01">mirror</a></td>
  <td>EPUB / 435 KB</td>
</tr></table>`

func TestParseResults(t *testing.T) {
	nf := parseResults(nonFictionHTML)
	if len(nf) != 1 || nf[0].MD5 != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("non-fiction = %+v", nf)
	}
	if nf[0].Title != "The Left Hand of Darkness" {
		t.Errorf("title = %q", nf[0].Title)
	}
	if nf[0].Size != 1<<20 {
		t.Errorf("size = %d, want 1MB", nf[0].Size)
	}

	f := parseResults(fictionHTML)
	if len(f) != 1 || f[0].MD5 != "ffff6789abcdef0123456789abcdef01" {
		t.Fatalf("fiction = %+v", f)
	}
	if f[0].Title != "The Dispossessed: An Ambiguous Utopia" {
		t.Errorf("fiction title = %q", f[0].Title)
	}
}

func TestMirrorDownloadURLs(t *testing.T) {
	got := MirrorDownloadURLs("ABCDEF0123456789ABCDEF0123456789")
	want := "https://library.lol/main/abcdef0123456789abcdef0123456789|https://libgen.li/ads.php?md5=abcdef0123456789abcdef0123456789"
	if got != want {
		t.Errorf("MirrorDownloadURLs = %q, want %q", got, want)
	}
}

// TestSearchMergesIndexesAndBuildsMirrors: the real Search hits both indexes,
// dedupes by md5, and every release carries the mirror download URLs.
func TestSearchMergesIndexesAndBuildsMirrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search.php"):
			_, _ = w.Write([]byte(nonFictionHTML))
		case strings.HasPrefix(r.URL.Path, "/fiction/"):
			_, _ = w.Write([]byte(fictionHTML))
		default:
			_, _ = w.Write([]byte("<html>libgen</html>"))
		}
	}))
	defer srv.Close()

	def := Def()
	if def.Name != "libgen" || def.Protocol != indexer.ProtocolDirect || !def.Serves("ebook") || def.Serves("audiobook") {
		t.Fatalf("def = %+v", def)
	}

	s := def.New(&indexer.Indexer{Name: "LG", BaseURL: srv.URL}, srv.Client())
	rels, err := s.Search(context.Background(), "le guin", "ebook")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(rels) != 2 {
		t.Fatalf("releases = %d, want 2 (one per index): %+v", len(rels), rels)
	}
	for _, r := range rels {
		if !strings.HasPrefix(r.DownloadURL, "https://library.lol/main/") ||
			!strings.Contains(r.DownloadURL, "|https://libgen.li/ads.php?md5=") {
			t.Errorf("release %q download URL = %q, want the mirror pair", r.Title, r.DownloadURL)
		}
	}
	if err := s.Test(context.Background()); err != nil {
		t.Errorf("Test: %v", err)
	}
	if got, err := s.Search(context.Background(), "x", "audiobook"); err != nil || got != nil {
		t.Errorf("audiobook search = %v, %v; want nil, nil", got, err)
	}
}
