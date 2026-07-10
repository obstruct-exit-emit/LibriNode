package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	db      *sql.DB
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
	f := &fx{store: store, db: db, history: []map[string]any{}}

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

// TestSeededTorrentRemovedAfterImport: a finished torrent the client has
// stopped seeding (goal reached) is removed with its data — but only once
// its grab is imported.
func TestSeededTorrentRemovedAfterImport(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()

	var deleted []string
	qbit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/login"):
			w.Write([]byte("Ok."))
		case strings.HasSuffix(r.URL.Path, "/torrents/info"):
			w.Write([]byte(`[{"hash":"h1","name":"Terry Pratchett - Mort EPUB","state":"pausedUP","progress":1,"content_path":"/downloads/mort"}]`))
		case strings.HasSuffix(r.URL.Path, "/torrents/delete"):
			r.ParseForm()
			deleted = append(deleted, r.FormValue("hashes")+":"+r.FormValue("deleteFiles"))
			w.Write([]byte("Ok."))
		default:
			w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(qbit.Close)
	if err := f.grabs.Add(&download.ClientConfig{
		Name: "qbit", Type: download.TypeQBittorrent, Host: qbit.URL,
		Category: "librinode", Enabled: true, Priority: 2,
	}); err != nil {
		t.Fatal(err)
	}

	// Grab already imported earlier → the seeded torrent gets cleaned up.
	if err := f.grabs.AddGrab(&download.GrabRecord{
		BookID: f.book.ID, ClientConfigID: 2,
		Title: "Terry Pratchett - Mort EPUB", Protocol: download.ProtocolTorrent,
		MediaType: "ebook",
	}); err != nil {
		t.Fatal(err)
	}
	grabs, _ := f.grabs.ListGrabs("")
	if err := f.grabs.ResolveGrab(grabs[0].ID, download.GrabStatusImported, "test"); err != nil {
		t.Fatal(err)
	}

	if _, err := f.svc.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "h1:true" {
		t.Fatalf("deleted = %v, want [h1:true] (remove with data)", deleted)
	}

	// A torrent with no imported LibriNode grab is never touched.
	deleted = nil
	if err := f.grabs.ResolveGrab(grabs[0].ID, download.GrabStatusFailed, "test reset"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 0 {
		t.Fatalf("foreign seeded torrent was removed: %v", deleted)
	}
}

func TestImportAudiobookGrab(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()

	// Audiobook root folder alongside the ebook one.
	abRoot := t.TempDir()
	if _, err := f.db.Exec(`INSERT INTO root_folders (media_type, path) VALUES ('audiobook', ?)`, abRoot); err != nil {
		t.Fatal(err)
	}

	// Multi-file audiobook download, tracked as an audiobook grab.
	f.completedDownload(t, "nzo_ab", "Terry Pratchett - Mort Unabridged M4B",
		"Mort - 01.mp3", "Mort - 02.mp3", "cover.jpg")
	if err := f.grabs.AddGrab(&download.GrabRecord{
		BookID: f.book.ID, ClientConfigID: 1, ClientItemID: "nzo_ab",
		Title: "Terry Pratchett - Mort Unabridged M4B", Protocol: download.ProtocolUsenet,
		MediaType: "audiobook",
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

	// Tracks landed inside the Audiobookshelf-style book folder.
	bookDir := filepath.Join(abRoot, "Terry Pratchett", "Mort")
	for _, name := range []string{"Mort - 01.mp3", "Mort - 02.mp3"} {
		if _, err := os.Stat(filepath.Join(bookDir, name)); err != nil {
			t.Fatalf("track missing: %v", err)
		}
	}
	// Non-audio junk excluded.
	if _, err := os.Stat(filepath.Join(bookDir, "cover.jpg")); !os.IsNotExist(err) {
		t.Error("non-audio file should not be imported")
	}

	// Recorded as an audiobook unit on the book (ebook side untouched).
	book, _ := f.store.GetBook(f.book.ID)
	if !book.HasAudiobookFile || book.HasEbookFile {
		t.Fatalf("book flags = ebook %v audio %v", book.HasEbookFile, book.HasAudiobookFile)
	}
	files, _ := f.store.ListBookFiles(f.book.ID)
	if len(files) != 1 || files[0].MediaType != "audiobook" || files[0].Path != bookDir {
		t.Fatalf("files = %+v", files)
	}
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

func TestFailedDownloadIsBlocklisted(t *testing.T) {
	f := fixture(t)

	f.history = append(f.history, map[string]any{
		"nzo_id": "nzo_bad2", "name": "Mort broken", "status": "Failed",
		"fail_message": "crc error", "category": "librinode",
	})
	if err := f.grabs.AddGrab(&download.GrabRecord{
		BookID: f.book.ID, ClientConfigID: 1, ClientItemID: "nzo_bad2",
		Title: "Mort broken", GUID: "guid-bad", Protocol: download.ProtocolUsenet,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.svc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries, err := f.grabs.ListBlocklist()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].GUID != "guid-bad" || entries[0].Title != "Mort broken" {
		t.Fatalf("blocklist = %+v", entries)
	}
	// Both keys block, and title matching survives case/spacing changes.
	blocked, _ := f.grabs.BlockedKeys()
	if !download.IsBlocked(blocked, "guid-bad", "") || !download.IsBlocked(blocked, "", "mort  BROKEN") {
		t.Error("blocklist keys don't match by guid/title")
	}
}

func TestImportUpgradeReplacesFile(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()

	// The book owns a PDF on disk.
	oldPath := filepath.Join(f.rootDir, "Terry Pratchett", "Mort.pdf")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("old-pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.store.UpsertBookFile(&library.BookFile{
		RootFolderID: 1, BookID: f.book.ID, MediaType: "ebook", Path: oldPath, Format: "pdf",
	}); err != nil {
		t.Fatal(err)
	}

	// A tracked grab delivers an EPUB (better per the default profile).
	f.completedDownload(t, "nzo_up", "Terry Pratchett - Mort Retail EPUB", "mort.epub")
	if err := f.grabs.AddGrab(&download.GrabRecord{
		BookID: f.book.ID, ClientConfigID: 1, ClientItemID: "nzo_up",
		Title: "Terry Pratchett - Mort Retail EPUB", Protocol: download.ProtocolUsenet,
		MediaType: "ebook",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := f.svc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 {
		t.Fatalf("result = %+v", result)
	}

	// New epub recorded, old pdf gone from disk and library.
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("upgraded-away pdf still on disk")
	}
	files, _ := f.store.ListBookFiles(f.book.ID)
	if len(files) != 1 || files[0].Format != "epub" {
		t.Fatalf("files after upgrade = %+v", files)
	}
	grabs, _ := f.grabs.ListGrabs("")
	if grabs[0].Status != download.GrabStatusImported ||
		!strings.Contains(grabs[0].Message, "upgraded (pdf → epub)") {
		t.Fatalf("grab = %+v", grabs[0])
	}
}

func TestImportNotAnUpgradeSkips(t *testing.T) {
	f := fixture(t)

	// The book owns an EPUB; a grabbed PDF must not replace it.
	if err := f.store.UpsertBookFile(&library.BookFile{
		RootFolderID: 1, BookID: f.book.ID, MediaType: "ebook",
		Path: filepath.Join(f.rootDir, "m.epub"), Format: "epub",
	}); err != nil {
		t.Fatal(err)
	}
	f.completedDownload(t, "nzo_dn", "Terry Pratchett - Mort PDF", "mort.pdf")
	if err := f.grabs.AddGrab(&download.GrabRecord{
		BookID: f.book.ID, ClientConfigID: 1, ClientItemID: "nzo_dn",
		Title: "Terry Pratchett - Mort PDF", Protocol: download.ProtocolUsenet,
		MediaType: "ebook",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := f.svc.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 0 || result.Skipped == 0 {
		t.Fatalf("result = %+v", result)
	}
	files, _ := f.store.ListBookFiles(f.book.ID)
	if len(files) != 1 || files[0].Format != "epub" {
		t.Fatalf("files = %+v", files)
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

// mangaSeries adds a manga series with three volumes (positions 1–3) and a
// manga root folder: vol 1 monitored, vol 2 monitored (the grab target),
// vol 3 unmonitored.
func (f *fx) mangaSeries(t *testing.T) (v1, v2, v3 *library.Book) {
	t.Helper()
	series := &library.Series{Source: "hardcover", ForeignID: "7310",
		Title: "Death Note", MediaType: "manga", Monitored: true}
	if err := f.store.UpsertSeries(series); err != nil {
		t.Fatal(err)
	}
	vol := func(fid string, pos float64, monitored bool) *library.Book {
		b := &library.Book{AuthorID: f.book.AuthorID, Source: "hardcover",
			ForeignID: fid, MediaType: "manga", Monitored: monitored,
			Title: fmt.Sprintf("Death Note Vol. %.0f", pos)}
		if err := f.store.UpsertBook(b); err != nil {
			t.Fatal(err)
		}
		if err := f.store.LinkBookSeries(b.ID, series.ID, pos); err != nil {
			t.Fatal(err)
		}
		return b
	}
	if _, err := f.db.Exec(
		`INSERT INTO root_folders (media_type, path, variant) VALUES ('manga', ?, 'mono')`,
		t.TempDir()); err != nil {
		t.Fatal(err)
	}
	return vol("dn1", 1, true), vol("dn2", 2, true), vol("dn3", 3, false)
}

// TestPackImportsMonitoredVolumesOnly: grabbing one volume from a
// complete-series bundle imports the grabbed volume (matched by number, not
// size) plus any other *monitored* volumes — never the unmonitored ones.
func TestPackImportsMonitoredVolumesOnly(t *testing.T) {
	f := fixture(t)
	v1, v2, v3 := f.mangaSeries(t)

	// The unmonitored volume's file is the largest in the bundle — size must
	// not decide which file the grabbed volume gets.
	f.completedDownload(t, "nzo_pack", "Death Note v01-v03 Complete Digital",
		"Death Note v01.cbz",
		"Death Note v02.cbz",
		"Death Note v03 Extended Collectors Special Edition.cbz")
	if err := f.grabs.AddGrab(&download.GrabRecord{
		BookID: v2.ID, MediaType: "manga", ClientConfigID: 1, ClientItemID: "nzo_pack",
		Title: "Death Note v01-v03 Complete Digital", Protocol: download.ProtocolUsenet,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := f.svc.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 {
		t.Fatalf("imported = %d, want 2 (grabbed v2 + monitored v1): %+v", result.Imported, result)
	}

	// The grabbed volume got ITS file (v02), not the bundle's largest (v03).
	files, _ := f.store.ListBookFiles(v2.ID)
	if len(files) != 1 {
		t.Fatalf("v2 files = %+v", files)
	}
	if got, _ := os.ReadFile(files[0].Path); string(got) != "book-bytes-Death Note v02.cbz" {
		t.Fatalf("v2 imported the wrong source file: %q", got)
	}

	// Monitored v1 came along; unmonitored v3 did not.
	if files, _ := f.store.ListBookFiles(v1.ID); len(files) != 1 {
		t.Fatalf("v1 files = %+v, want the pack extra", files)
	}
	if files, _ := f.store.ListBookFiles(v3.ID); len(files) != 0 {
		t.Fatalf("v3 files = %+v, want none (unmonitored)", files)
	}

	grabs, _ := f.grabs.ListGrabs("")
	if grabs[0].Status != download.GrabStatusImported {
		t.Errorf("grab = %+v", grabs[0])
	}
}

// TestPackEbookImportsMonitoredByTitle: an ebook bundle fills the author's
// monitored books by title match; unmonitored books are left alone.
func TestPackEbookImportsMonitoredByTitle(t *testing.T) {
	f := fixture(t)

	guards := &library.Book{AuthorID: f.book.AuthorID, Source: "hardcover", ForeignID: "10",
		Title: "Guards! Guards!", InEbookLibrary: true, EbookMonitored: true}
	if err := f.store.UpsertBook(guards); err != nil {
		t.Fatal(err)
	}
	sourcery := &library.Book{AuthorID: f.book.AuthorID, Source: "hardcover", ForeignID: "11",
		Title: "Sourcery"} // enrolled nowhere, monitored nowhere
	if err := f.store.UpsertBook(sourcery); err != nil {
		t.Fatal(err)
	}

	f.completedDownload(t, "nzo_epack", "Terry Pratchett - Discworld Collection EPUB",
		"Mort.epub", "Guards! Guards! (1989).epub", "Sourcery.epub")
	if err := f.grabs.AddGrab(&download.GrabRecord{
		BookID: f.book.ID, ClientConfigID: 1, ClientItemID: "nzo_epack",
		Title: "Terry Pratchett - Discworld Collection EPUB", Protocol: download.ProtocolUsenet,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := f.svc.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 {
		t.Fatalf("imported = %d, want 2 (grabbed Mort + monitored Guards): %+v", result.Imported, result)
	}

	// The grabbed book got its own file, not another book's.
	files, _ := f.store.ListBookFiles(f.book.ID)
	if len(files) != 1 {
		t.Fatalf("Mort files = %+v", files)
	}
	if got, _ := os.ReadFile(files[0].Path); string(got) != "book-bytes-Mort.epub" {
		t.Fatalf("Mort imported the wrong source file: %q", got)
	}
	if files, _ := f.store.ListBookFiles(guards.ID); len(files) != 1 {
		t.Fatalf("Guards files = %+v, want the pack extra", files)
	}
	if files, _ := f.store.ListBookFiles(sourcery.ID); len(files) != 0 {
		t.Fatalf("Sourcery files = %+v, want none (unmonitored)", files)
	}
}

// TestPackSkipsOwnedBookUnlessUpgrade: a monitored book that already owns the
// format is not re-imported from a pack (same format is not an upgrade).
func TestPackSkipsOwnedBookUnlessUpgrade(t *testing.T) {
	f := fixture(t)

	guards := &library.Book{AuthorID: f.book.AuthorID, Source: "hardcover", ForeignID: "10",
		Title: "Guards! Guards!", InEbookLibrary: true, EbookMonitored: true}
	if err := f.store.UpsertBook(guards); err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(t.TempDir(), "guards.epub")
	if err := os.WriteFile(owned, []byte("already-owned"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.store.UpsertBookFile(&library.BookFile{
		RootFolderID: 1, BookID: guards.ID, MediaType: "ebook",
		Path: owned, Size: 13, Format: "epub",
	}); err != nil {
		t.Fatal(err)
	}

	f.completedDownload(t, "nzo_epack2", "Terry Pratchett - Two Book Bundle EPUB",
		"Mort.epub", "Guards! Guards!.epub")
	if err := f.grabs.AddGrab(&download.GrabRecord{
		BookID: f.book.ID, ClientConfigID: 1, ClientItemID: "nzo_epack2",
		Title: "Terry Pratchett - Two Book Bundle EPUB", Protocol: download.ProtocolUsenet,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := f.svc.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 {
		t.Fatalf("imported = %d, want 1 (Guards owns an equal-quality epub): %+v", result.Imported, result)
	}
	files, _ := f.store.ListBookFiles(guards.ID)
	if len(files) != 1 || files[0].Path != owned {
		t.Fatalf("Guards files = %+v, want only the pre-owned epub", files)
	}
}
