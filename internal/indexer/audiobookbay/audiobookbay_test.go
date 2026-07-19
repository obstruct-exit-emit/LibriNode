package audiobookbay

import (
	"net/url"
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
	if posts[0].URL != "https://audiobookbay.lu/audio-books/the-hobbit-unabridged" {
		t.Errorf("post[0].URL = %q", posts[0].URL)
	}
	if posts[0].Title != "The Hobbit (Unabridged) [MP3]" {
		t.Errorf("post[0].Title = %q", posts[0].Title)
	}
	// Entity-decoded and absolute already.
	if posts[1].Title != "Dune – Frank Herbert" {
		t.Errorf("post[1].Title = %q", posts[1].Title)
	}
	if posts[1].URL != "https://audiobookbay.lu/audio-books/dune-frank-herbert" {
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
	if !strings.Contains(magnet, "dn="+url.QueryEscape("The Hobbit (Unabridged)")) {
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
