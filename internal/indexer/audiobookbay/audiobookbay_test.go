package audiobookbay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/librinode/librinode/internal/indexer"
)

const listingHTML = `
<html><body>
  <div class="post">
    <div class="postTitle"><h2><a href="/audio-books/the-hobbit-unabridged/">The Hobbit (Unabridged) [MP3]</a></h2></div>
  </div>
  <div class="post">
    <div class="postTitle"><h2><a href="https://audiobookbay.lu/audio-books/dune-frank-herbert/">Dune &#8211; Frank Herbert</a></h2></div>
  </div>
</body></html>`

const detailHTML = `
<html><body>
  <h1 class="postTitle">The Hobbit (Unabridged)</h1>
  <p>Format: MP3<br>Bitrate: 128 Kbps<br>File Size: 512.5 MBs</p>
  <table>
    <tr><td>Info Hash:</td><td>0123456789ABCDEF0123456789ABCDEF01234567</td></tr>
    <tr><td>Tracker:</td><td>udp://tracker.openbittorrent.com:80/announce</td></tr>
    <tr><td></td><td>udp://tracker.opentrackr.org:1337/announce</td></tr>
    <tr><td></td><td>http://tracker.example.org/announce.php?x=1</td></tr>
  </table>
</body></html>`

func TestParseListing(t *testing.T) {
	posts := parseListing(listingHTML, "https://audiobookbay.lu")
	if len(posts) != 2 {
		t.Fatalf("parsed %d posts, want 2: %+v", len(posts), posts)
	}
	// ABB's canonical permalinks keep their trailing slash — the slash-less form
	// 301s with no Location header, breaking the grab-time Resolve fetch.
	if posts[0].URL != "https://audiobookbay.lu/audio-books/the-hobbit-unabridged/" {
		t.Errorf("post[0].URL = %q", posts[0].URL)
	}
	if posts[0].Title != "The Hobbit (Unabridged) [MP3]" {
		t.Errorf("post[0].Title = %q", posts[0].Title)
	}
	// Entity-decoded and absolute already.
	if posts[1].Title != "Dune – Frank Herbert" {
		t.Errorf("post[1].Title = %q", posts[1].Title)
	}
	if posts[1].URL != "https://audiobookbay.lu/audio-books/dune-frank-herbert/" {
		t.Errorf("post[1].URL = %q", posts[1].URL)
	}
}

func TestParseDetailAndMagnet(t *testing.T) {
	hash, trackers, size, ok := parseDetail(detailHTML)
	if !ok {
		t.Fatal("expected a parseable detail page")
	}
	if hash != "0123456789abcdef0123456789abcdef01234567" {
		t.Errorf("hash = %q (want lowercased)", hash)
	}
	if len(trackers) != 3 {
		t.Fatalf("trackers = %v, want 3", trackers)
	}
	if size != int64(512.5*(1<<20)) {
		t.Errorf("size = %d, want %d", size, int64(512.5*(1<<20)))
	}

	magnet := buildMagnet(hash, "The Hobbit (Unabridged)", trackers)
	if !strings.HasPrefix(magnet, "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567") {
		t.Errorf("magnet prefix wrong: %q", magnet)
	}
	if !strings.Contains(magnet, "dn=The%20Hobbit%20%28Unabridged%29") {
		t.Errorf("magnet missing display name: %q", magnet)
	}
	if strings.Count(magnet, "&tr=") != 3 {
		t.Errorf("magnet should carry 3 trackers: %q", magnet)
	}
}

func TestParseDetailNoHash(t *testing.T) {
	if _, _, _, ok := parseDetail("<html>no info hash here</html>"); ok {
		t.Error("a page without an info hash must not parse as a release")
	}
}

func TestParseBases(t *testing.T) {
	cases := map[string][]string{
		"":                                      {DefaultBaseURL},
		"https://a.example/":                    {"https://a.example"},
		"https://a.example, https://b.example/": {"https://a.example", "https://b.example"},
		" , https://only.example":               {"https://only.example"},
	}
	for in, want := range cases {
		got := ParseBases(in)
		if len(got) != len(want) {
			t.Errorf("ParseBases(%q) = %v, want %v", in, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("ParseBases(%q)[%d] = %q, want %q", in, i, got[i], want[i])
			}
		}
	}
}

func TestMagnetEscapeUsesPercent20(t *testing.T) {
	magnet := buildMagnet("0123456789abcdef0123456789abcdef01234567", "A Title With Spaces", []string{"udp://t.example:80/announce"})
	if strings.Contains(magnet, "+") {
		t.Errorf("magnet must not use '+' for spaces: %q", magnet)
	}
	if !strings.Contains(magnet, "dn=A%20Title%20With%20Spaces") {
		t.Errorf("magnet dn should be %%20-escaped: %q", magnet)
	}
}

func TestParseListingSkipsNavLinks(t *testing.T) {
	html := `
	<a href="/audio-books/type/fiction/">Fiction</a>
	<a href="/audio-books/page/2/">2</a>
	<a href="/audio-books/the-real-book/">The Real Book</a>`
	posts := parseListing(html, "https://abb.example")
	if len(posts) != 1 || posts[0].Title != "The Real Book" {
		t.Fatalf("posts = %+v, want only The Real Book", posts)
	}
}

// abbServer serves ABB's shape: a homepage (session warm-up), a search listing,
// and detail pages. It counts requests by kind so tests can assert search never
// crawls detail pages.
type abbServer struct {
	*httptest.Server
	homeHits, searchHits, detailHits int
}

func newABBServer(t *testing.T, listing string) *abbServer {
	t.Helper()
	a := &abbServer{}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Query().Get("s") != "":
			a.searchHits++
			_, _ = w.Write([]byte(listing))
		case strings.Contains(r.URL.Path, "/audio-books/"):
			a.detailHits++
			_, _ = w.Write([]byte(detailHTML))
		default:
			a.homeHits++
			_, _ = w.Write([]byte("<html>abb home</html>"))
		}
	}))
	t.Cleanup(a.Close)
	return a
}

const oneResultListing = `<div class="postTitle"><a href="/audio-books/the-hobbit/">The Hobbit</a></div>`

// TestSearchDefersDetailFetch: a search returns releases whose download URL is
// the release page (not a magnet), and it never fetches a detail page — the
// magnet is assembled only when Resolve is called at grab time.
func TestSearchDefersDetailFetch(t *testing.T) {
	srv := newABBServer(t, oneResultListing)

	s := Def().New(&indexer.Indexer{Name: "ABB", BaseURL: srv.URL}, srv.Client())
	releases, err := s.Search(context.Background(), "hobbit", "audiobook")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("releases = %+v, want 1", releases)
	}
	if releases[0].DownloadURL != srv.URL+"/audio-books/the-hobbit/" {
		t.Errorf("download URL = %q, want the release page", releases[0].DownloadURL)
	}
	if srv.detailHits != 0 {
		t.Errorf("search fetched %d detail page(s); it must fetch none", srv.detailHits)
	}
	if srv.homeHits == 0 {
		t.Error("search should warm up the session by hitting the homepage")
	}

	// Resolve (grab time) fetches exactly the one detail page and builds the magnet.
	r, ok := s.(indexer.Resolver)
	if !ok {
		t.Fatal("ABB searcher must implement indexer.Resolver")
	}
	magnet, err := r.Resolve(context.Background(), releases[0].DownloadURL)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.HasPrefix(magnet, "magnet:?xt=urn:btih:0123456789abcdef") {
		t.Errorf("resolved magnet = %q", magnet)
	}
	if srv.detailHits != 1 {
		t.Errorf("Resolve fetched %d detail page(s), want 1", srv.detailHits)
	}
}

// TestSearchFailsOverToFallbackURL: the primary site is down; the search must
// succeed transparently through the fallback mirror.
func TestSearchFailsOverToFallbackURL(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer dead.Close()
	mirror := newABBServer(t, oneResultListing)

	def := Def()
	s := def.New(&indexer.Indexer{Name: "ABB", BaseURL: dead.URL + "," + mirror.URL}, mirror.Client())
	releases, err := s.Search(context.Background(), "hobbit", "audiobook")
	if err != nil {
		t.Fatalf("Search with fallback: %v", err)
	}
	if len(releases) != 1 || releases[0].DownloadURL != mirror.URL+"/audio-books/the-hobbit/" {
		t.Fatalf("releases = %+v", releases)
	}
	if err := s.Test(context.Background()); err != nil {
		t.Errorf("Test with fallback: %v", err)
	}
	deadOnly := def.New(&indexer.Indexer{Name: "ABB", BaseURL: dead.URL}, dead.Client())
	if err := deadOnly.Test(context.Background()); err == nil {
		t.Error("Test against only a dead site should fail")
	}
}

// TestSearchDetectsHomepageBlock: a search redirected to the homepage (ABB's
// rate-limit/block behaviour) surfaces as an error, not empty success.
func TestSearchDetectsHomepageBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("s") != "" {
			http.Redirect(w, r, "/", http.StatusFound) // block → bounce to home
			return
		}
		_, _ = w.Write([]byte("<html>abb home</html>"))
	}))
	defer srv.Close()

	s := Def().New(&indexer.Indexer{Name: "ABB", BaseURL: srv.URL}, srv.Client())
	_, err := s.Search(context.Background(), "blocked", "audiobook")
	if err == nil || !strings.Contains(err.Error(), "homepage") {
		t.Errorf("expected a homepage-block error, got %v", err)
	}
}

// TestDefRegistersAsTorrentAudiobook: the definition advertises what the
// framework and UI rely on.
func TestDef(t *testing.T) {
	def := Def()
	if def.Name != "audiobookbay" || def.Protocol != indexer.ProtocolTorrent {
		t.Errorf("def = %+v", def)
	}
	if !def.Serves("audiobook") || def.Serves("ebook") {
		t.Error("ABB should serve audiobook only")
	}
	if def.DefaultBaseURL == "" || def.New == nil {
		t.Error("def needs a default base URL and constructor")
	}
	// A native searcher that isn't asked for audiobooks returns nothing.
	s := def.New(&indexer.Indexer{Name: "ABB"}, nil)
	if rels, err := s.Search(nil, "anything", "ebook"); err != nil || rels != nil {
		t.Errorf("non-audiobook search = (%v, %v), want (nil, nil)", rels, err)
	}
}
