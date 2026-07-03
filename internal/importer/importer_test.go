package importer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/librinode/librinode/internal/config"
	"github.com/librinode/librinode/internal/database"
	"github.com/librinode/librinode/internal/download"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/organize"
)

type fx struct {
	svc     *Service
	store   *library.Store
	grabs   *download.Store
	rootDir string
	book    *library.Book
	history []map[string]any // mutable mock SAB history
	removed []string         // nzo ids deleted from history
}

func fixture(t *testing.T) *fx {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	store := library.NewStore(db)
	f := &fx{store: store, history: []map[string]any{}}

	// Mock SABnzbd: empty queue, mutable history, delete tracking.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("mode") {
		case "version":
			w.Write([]byte(`{"version": "4.3.2"}`))
		case "queue":
			w.Write([]byte(`{"queue": {"slots": []}}`))
		case "history":
			if q.Get("name") == "delete" {
				f.removed = append(f.removed, q.Get("value"))
				w.Write([]byte(`{"status": true}`))
				return
			}
			out, _ := json.Marshal(map[string]any{"history": map[string]any{"slots": f.history}})
			w.Write(out)
		default:
			w.Write([]byte(`{"status": false, "error": "unknown mode"}`))
		}
	}))
	t.Cleanup(srv.Close)

	downloads := download.NewService(download.NewStore(db))
	f.grabs = downloads.Store()
	if err := downloads.Store().Add(&download.ClientConfig{
		Name: "sab", Type: download.TypeSABnzbd, Host: srv.URL,
		APIKey: "k", Category: "librinode", Enabled: true, Priority: 1,
	}); err != nil {
		t.Fatal(err)
	}

	// Library: one monitored, fileless book and an ebook root folder.
	author := &library.Author{Source: "hardcover", ForeignID: "100", Name: "Terry Pratchett", Monitored: true}
	if err := store.UpsertAuthor(author); err != nil {
		t.Fatal(err)
	}
	f.book = &library.Book{AuthorID: author.ID, Source: "hardcover", ForeignID: "1",
		Title: "Mort", ReleaseDate: "1987-11-12", Monitored: true}
	if err := store.UpsertBook(f.book); err != nil {
		t.Fatal(err)
	}
	f.rootDir = t.TempDir()
	if _, err := db.Exec(`INSERT INTO root_folders (media_type, path) VALUES ('ebook', ?)`, f.rootDir); err != nil {
		t.Fatal(err)
	}

	f.svc = New(store, downloads, organize.New(store, cfg))
	return f
}

// completedDownload creates a finished download on disk and a matching
// history entry.
func (f *fx) completedDownload(t *testing.T, nzoID, title string, files ...string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), title)
	for _, rel := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("book-bytes-"+rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f.history = append(f.history, map[string]any{
		"nzo_id": nzoID, "name": title, "status": "Completed", "storage": dir, "category": "librinode",
	})
	return dir
}

func TestImportTrackedGrab(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()

	f.completedDownload(t, "nzo_1", "Terry Pratchett - Mort Retail EPUB",
		"Mort.epub", "sample/tiny.txt")
	if err := f.grabs.AddGrab(&download.GrabRecord{
		BookID: f.book.ID, ClientConfigID: 1, ClientItemID: "nzo_1",
		Title: "Terry Pratchett - Mort Retail EPUB", Protocol: download.ProtocolUsenet,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := f.svc.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Imported != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}

	// File landed at the template path.
	want := filepath.Join(f.rootDir, "Terry Pratchett", "Mort.epub")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("imported file missing: %v", err)
	}
	// Book owns it now.
	book, _ := f.store.GetBook(f.book.ID)
	if !book.HasFile {
		t.Error("book should have a file after import")
	}
	files, _ := f.store.ListBookFiles(f.book.ID)
	if len(files) != 1 || files[0].Format != "epub" || files[0].Size == 0 {
		t.Fatalf("book files = %+v", files)
	}
	// Grab resolved, usenet history cleaned up.
	grabs, _ := f.grabs.ListGrabs("")
	if len(grabs) != 1 || grabs[0].Status != download.GrabStatusImported {
		t.Fatalf("grabs = %+v", grabs)
	}
	if len(f.removed) != 1 || f.removed[0] != "nzo_1" {
		t.Errorf("history cleanup = %v", f.removed)
	}

	// Second pass: nothing new, nothing re-imported.
	result, err = f.svc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 0 {
		t.Errorf("re-import happened: %+v", result)
	}
}

func TestImportUntrackedByTitle(t *testing.T) {
	f := fixture(t)

	f.completedDownload(t, "nzo_2", "Mort by Terry Pratchett epub", "mort_v2.epub")

	result, err := f.svc.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 {
		t.Fatalf("result = %+v (messages: %v)", result, result.Messages)
	}
	book, _ := f.store.GetBook(f.book.ID)
	if !book.HasFile {
		t.Error("book should have gained the untracked download")
	}
}

func TestFailedDownloadResolvesGrab(t *testing.T) {
	f := fixture(t)

	f.history = append(f.history, map[string]any{
		"nzo_id": "nzo_bad", "name": "Mort broken", "status": "Failed",
		"fail_message": "crc error", "category": "librinode",
	})
	if err := f.grabs.AddGrab(&download.GrabRecord{
		BookID: f.book.ID, ClientConfigID: 1, ClientItemID: "nzo_bad",
		Title: "Mort broken", Protocol: download.ProtocolUsenet,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := f.svc.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || result.Imported != 0 {
		t.Fatalf("result = %+v", result)
	}
	grabs, _ := f.grabs.ListGrabs("")
	if grabs[0].Status != download.GrabStatusFailed {
		t.Errorf("grab = %+v", grabs[0])
	}
	if len(f.removed) != 1 {
		t.Errorf("failed download not removed from client: %v", f.removed)
	}
}

func TestImportSkipsWhenNoEbookInDownload(t *testing.T) {
	f := fixture(t)

	f.completedDownload(t, "nzo_3", "Terry Pratchett - Mort Retail EPUB", "readme.txt")
	if err := f.grabs.AddGrab(&download.GrabRecord{
		BookID: f.book.ID, ClientConfigID: 1, ClientItemID: "nzo_3",
		Title: "Terry Pratchett - Mort Retail EPUB", Protocol: download.ProtocolUsenet,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := f.svc.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 {
		t.Fatalf("result = %+v", result)
	}
	grabs, _ := f.grabs.ListGrabs("")
	if grabs[0].Status != download.GrabStatusFailed || grabs[0].Message == "" {
		t.Errorf("grab = %+v", grabs[0])
	}
}

func TestAmbiguousUntrackedIsSkipped(t *testing.T) {
	f := fixture(t)

	// Second fileless book that also matches "Mort" releases.
	other := &library.Book{AuthorID: f.book.AuthorID, Source: "hardcover", ForeignID: "2",
		Title: "Mort", Monitored: true}
	// Same title is fine — different foreign id.
	if err := f.store.UpsertBook(other); err != nil {
		t.Fatal(err)
	}
	f.completedDownload(t, "nzo_4", "Mort epub", "mort.epub")

	result, err := f.svc.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 0 || result.Skipped == 0 {
		t.Fatalf("ambiguous match should skip: %+v", result)
	}
}
