package download

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// qbitStub simulates just enough of qBittorrent's Web API for Add/List:
// login always succeeds, createCategory and add are no-ops, and torrents/info
// reports whatever torrents are configured on the stub.
type qbitStub struct {
	torrents []struct {
		Hash     string  `json:"hash"`
		Name     string  `json:"name"`
		State    string  `json:"state"`
		Progress float64 `json:"progress"`
		Category string  `json:"category"`
	}
}

func (q *qbitStub) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/createCategory":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/torrents/add":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_ = json.NewEncoder(w).Encode(q.torrents)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newTestQBittorrent(t *testing.T, srv *httptest.Server) *qbittorrent {
	t.Helper()
	q := newQBittorrent(&ClientConfig{ID: 1, Name: "qBit", Type: TypeQBittorrent, Host: srv.URL, Category: "librinode"})
	t.Cleanup(srv.Close)
	return q
}

// TestQBittorrentAddReturnsHash: the add endpoint itself never echoes a hash,
// but Add must still come back with one — looked up from the list right
// after — so the grab it's recorded under can be found again by id instead of
// only by a fragile title match. Regression for grabs permanently stuck
// "pending" after being removed from the queue, since a torrent grab's
// missing id meant removal could only resolve the grab by an exact title
// match.
func TestQBittorrentAddReturnsHash(t *testing.T) {
	stub := &qbitStub{}
	stub.torrents = append(stub.torrents, struct {
		Hash     string  `json:"hash"`
		Name     string  `json:"name"`
		State    string  `json:"state"`
		Progress float64 `json:"progress"`
		Category string  `json:"category"`
	}{Hash: "abc123", Name: "Dune Messiah", State: "downloading", Progress: 0.1, Category: "librinode"})
	q := newTestQBittorrent(t, stub.server())

	id, err := q.Add(context.Background(), "magnet:?xt=urn:btih:abc123", "Dune Messiah")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if id != "abc123" {
		t.Errorf("Add returned id %q, want the real hash %q", id, "abc123")
	}
}

// TestQBittorrentAddFindsHashOnSubstringMatch: a tracker or the client itself
// can mutate the name away from an exact match (appending tags, stripping
// punctuation); an exact match is preferred but a substring match still
// resolves the hash rather than giving up and leaving the grab untraceable.
func TestQBittorrentAddFindsHashOnSubstringMatch(t *testing.T) {
	stub := &qbitStub{}
	stub.torrents = append(stub.torrents, struct {
		Hash     string  `json:"hash"`
		Name     string  `json:"name"`
		State    string  `json:"state"`
		Progress float64 `json:"progress"`
		Category string  `json:"category"`
	}{Hash: "def456", Name: "Dune Messiah [FL] {Narrator} 2023", State: "downloading", Progress: 0, Category: "librinode"})
	q := newTestQBittorrent(t, stub.server())

	id, err := q.Add(context.Background(), "magnet:?xt=urn:btih:def456", "Dune Messiah")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if id != "def456" {
		t.Errorf("Add returned id %q, want the fuzzily-matched hash %q", id, "def456")
	}
}
