package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/librinode/librinode/internal/database"
)

// mockQbit fakes qBittorrent's Web API v2 with session-cookie auth.
func mockQbit(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			r.ParseForm()
			if r.Form.Get("username") != "admin" || r.Form.Get("password") != "secret" {
				w.Write([]byte("Fails."))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "mock-session", Path: "/"})
			w.Write([]byte("Ok."))
		default:
			if c, err := r.Cookie("SID"); err != nil || c.Value != "mock-session" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			switch r.URL.Path {
			case "/api/v2/app/version":
				w.Write([]byte("v5.0.0"))
			case "/api/v2/torrents/createCategory":
				w.WriteHeader(http.StatusConflict) // already exists
			case "/api/v2/torrents/add":
				r.ParseForm()
				if r.Form.Get("category") != "librinode" || r.Form.Get("urls") == "" {
					w.Write([]byte("Fails."))
					return
				}
				w.Write([]byte("Ok."))
			case "/api/v2/torrents/info":
				if r.URL.Query().Get("category") != "librinode" {
					w.Write([]byte("[]"))
					return
				}
				w.Write([]byte(`[
					{"hash":"aaa","name":"Mort.epub","state":"downloading","progress":0.42,"content_path":"","save_path":"/dl"},
					{"hash":"bbb","name":"Guards.epub","state":"stalledUP","progress":1,"content_path":"/dl/Guards.epub"},
					{"hash":"ccc","name":"Bad.epub","state":"error","progress":0.1}
				]`))
			case "/api/v2/torrents/delete":
				r.ParseForm()
				if r.Form.Get("hashes") == "" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Write([]byte(""))
			default:
				http.NotFound(w, r)
			}
		}
	}))
}

func qbitConfig(host string) *ClientConfig {
	return &ClientConfig{
		Name: "qbit", Type: TypeQBittorrent, Host: host,
		Username: "admin", Password: "secret", Category: "librinode",
		Enabled: true, Priority: 1,
	}
}

func TestQBittorrent(t *testing.T) {
	srv := mockQbit(t)
	defer srv.Close()
	ctx := context.Background()

	c, err := New(qbitConfig(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Test(ctx); err != nil {
		t.Fatalf("Test: %v", err)
	}

	// Wrong password surfaces as a login failure.
	bad := qbitConfig(srv.URL)
	bad.Password = "nope"
	badClient, _ := New(bad)
	if err := badClient.Test(ctx); err == nil || !strings.Contains(err.Error(), "login failed") {
		t.Fatalf("bad credentials: err = %v", err)
	}

	if _, err := c.Add(ctx, "magnet:?xt=urn:btih:abc", "Mort"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	items, err := c.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %+v", items)
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	if byID["aaa"].Status != "downloading" || byID["aaa"].Progress != 0.42 {
		t.Errorf("downloading item = %+v", byID["aaa"])
	}
	if byID["bbb"].Status != "completed" || byID["bbb"].Path != "/dl/Guards.epub" {
		t.Errorf("completed item = %+v", byID["bbb"])
	}
	if byID["ccc"].Status != "failed" {
		t.Errorf("failed item = %+v", byID["ccc"])
	}

	if err := c.Remove(ctx, "aaa", true); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

// mockSab fakes SABnzbd's single-endpoint API.
func mockSab(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("apikey") != "sab-key" {
			w.Write([]byte(`{"status": false, "error": "API Key Incorrect"}`))
			return
		}
		switch q.Get("mode") {
		case "version":
			w.Write([]byte(`{"version": "4.3.2"}`))
		case "addurl":
			if q.Get("cat") != "librinode" || q.Get("name") == "" {
				w.Write([]byte(`{"status": false, "error": "bad request"}`))
				return
			}
			w.Write([]byte(`{"status": true, "nzo_ids": ["SABnzbd_nzo_x1"]}`))
		case "queue":
			w.Write([]byte(`{"queue": {"slots": [
				{"nzo_id": "SABnzbd_nzo_q1", "filename": "Mort", "status": "Downloading", "percentage": "34", "category": "librinode"}
			]}}`))
		case "history":
			w.Write([]byte(`{"history": {"slots": [
				{"nzo_id": "SABnzbd_nzo_h1", "name": "Guards", "status": "Completed", "storage": "/complete/Guards", "category": "librinode"},
				{"nzo_id": "SABnzbd_nzo_h2", "name": "Broken", "status": "Failed", "fail_message": "crc", "category": "librinode"}
			]}}`))
		default:
			w.Write([]byte(`{"status": false, "error": "unknown mode"}`))
		}
	}))
}

func sabConfig(host string) *ClientConfig {
	return &ClientConfig{
		Name: "sab", Type: TypeSABnzbd, Host: host,
		APIKey: "sab-key", Category: "librinode", Enabled: true, Priority: 1,
	}
}

func TestSABnzbd(t *testing.T) {
	srv := mockSab(t)
	defer srv.Close()
	ctx := context.Background()

	c, err := New(sabConfig(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Test(ctx); err != nil {
		t.Fatalf("Test: %v", err)
	}

	bad := sabConfig(srv.URL)
	bad.APIKey = "wrong"
	badClient, _ := New(bad)
	if err := badClient.Test(ctx); err == nil || !strings.Contains(err.Error(), "API Key Incorrect") {
		t.Fatalf("bad key: err = %v", err)
	}

	id, err := c.Add(ctx, "https://indexer/get/abc.nzb", "Mort")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if id != "SABnzbd_nzo_x1" {
		t.Errorf("nzo id = %q", id)
	}

	items, err := c.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %+v", items)
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	if byID["SABnzbd_nzo_q1"].Status != "downloading" || byID["SABnzbd_nzo_q1"].Progress != 0.34 {
		t.Errorf("queue item = %+v", byID["SABnzbd_nzo_q1"])
	}
	if byID["SABnzbd_nzo_h1"].Status != "completed" || byID["SABnzbd_nzo_h1"].Path != "/complete/Guards" {
		t.Errorf("history item = %+v", byID["SABnzbd_nzo_h1"])
	}
	if byID["SABnzbd_nzo_h2"].Status != "failed" {
		t.Errorf("failed item = %+v", byID["SABnzbd_nzo_h2"])
	}

	if err := c.Remove(ctx, "SABnzbd_nzo_q1", false); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewService(NewStore(db))
}

func TestServiceGrabAndQueue(t *testing.T) {
	qbit := mockQbit(t)
	defer qbit.Close()
	sab := mockSab(t)
	defer sab.Close()
	ctx := context.Background()

	svc := newTestService(t)
	qc := qbitConfig(qbit.URL)
	sc := sabConfig(sab.URL)
	if err := svc.Store().Add(qc); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store().Add(sc); err != nil {
		t.Fatal(err)
	}

	// Grabs route by protocol.
	torrentGrab, err := svc.Grab(ctx, ProtocolTorrent, "magnet:?xt=urn:btih:abc", "Mort")
	if err != nil {
		t.Fatalf("torrent grab: %v", err)
	}
	if torrentGrab.Client != "qbit" {
		t.Errorf("torrent grab = %+v", torrentGrab)
	}
	usenetGrab, err := svc.Grab(ctx, ProtocolUsenet, "https://indexer/get/abc.nzb", "Mort")
	if err != nil {
		t.Fatalf("usenet grab: %v", err)
	}
	if usenetGrab.Client != "sab" || usenetGrab.ID == "" {
		t.Errorf("usenet grab = %+v", usenetGrab)
	}

	// Queue aggregates both clients.
	items, errs, err := svc.Queue(ctx)
	if err != nil {
		t.Fatalf("Queue: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("queue errs = %v", errs)
	}
	if len(items) != 6 { // 3 qbit + 3 sab
		t.Fatalf("%d items, want 6: %+v", len(items), items)
	}

	// No client for a protocol → ErrNoClient.
	qc.Enabled = false
	if err := svc.Store().Update(qc); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Grab(ctx, ProtocolTorrent, "magnet:x", "y"); err != ErrNoClient {
		t.Errorf("grab without client: err = %v, want ErrNoClient", err)
	}
}

func TestStoreCRUD(t *testing.T) {
	svc := newTestService(t)
	s := svc.Store()

	c := qbitConfig("http://localhost:8080")
	if err := s.Add(c); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if c.ID == 0 {
		t.Fatal("id not set")
	}
	dup := qbitConfig("http://other")
	if err := s.Add(dup); err == nil {
		t.Error("duplicate name should fail")
	}

	c.Category = "books"
	if err := s.Update(c); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := s.Get(c.ID)
	if got.Category != "books" {
		t.Errorf("updated = %+v", got)
	}

	if err := s.Delete(c.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete(c.ID); err != ErrNotFound {
		t.Errorf("double delete: %v", err)
	}
}
