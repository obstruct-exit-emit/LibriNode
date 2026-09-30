package download

import (
	"crypto/sha1"
	"encoding/hex"
	"testing"
)

func TestTorrentInfoHash(t *testing.T) {
	// A well-formed info dictionary; its SHA-1 is the expected info hash.
	info := []byte("d6:lengthi12e4:name4:test12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaae")
	sum := sha1.Sum(info)
	want := hex.EncodeToString(sum[:])

	// Wrap it in a full torrent dict with an announce key before it.
	torrent := []byte("d8:announce18:http://tracker/ann4:info")
	torrent = append(torrent, info...)
	torrent = append(torrent, 'e')

	got, err := torrentInfoHash(torrent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("info hash = %s, want %s", got, want)
	}
	if len(got) != 40 {
		t.Fatalf("info hash %q is not 40 hex chars", got)
	}
}

func TestTorrentInfoHashRejectsJunk(t *testing.T) {
	for _, in := range [][]byte{
		nil,
		[]byte("not bencode"),
		[]byte("d8:announce4:aaaae"),       // a dict, but no info key
		[]byte("d4:infod6:lengthi12e"),      // truncated info dict
		[]byte("l4:infoe"),                  // top level is a list, not a dict
	} {
		if _, err := torrentInfoHash(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}
