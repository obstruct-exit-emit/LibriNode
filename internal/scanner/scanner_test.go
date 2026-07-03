package scanner

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/librinode/librinode/internal/database"
	"github.com/librinode/librinode/internal/library"
)

// fx bundles everything the scanner tests need.
type fx struct {
	svc     *Service
	store   *library.Store
	db      *sql.DB
	rootDir string
}

func TestParsePath(t *testing.T) {
	cases := []struct {
		path          string
		author, title string
	}{
		{"Terry Pratchett/Mort.epub", "Terry Pratchett", "Mort"},
		{"Terry Pratchett/Discworld/01 - The Colour of Magic.epub", "Terry Pratchett", "The Colour of Magic"},
		{"Terry Pratchett/Terry Pratchett - Mort.epub", "Terry Pratchett", "Mort"},
		{"Terry Pratchett - Mort.epub", "Terry Pratchett", "Mort"},
		{"Mort.epub", "", "Mort"},
		{"Ursula K. Le Guin/1.5 - The Word for World Is Forest.pdf", "Ursula K. Le Guin", "The Word for World Is Forest"},
	}
	for _, c := range cases {
		got := ParsePath(filepath.FromSlash(c.path))
		if got.Author != c.author || got.Title != c.title {
			t.Errorf("ParsePath(%q) = %+v, want author %q title %q", c.path, got, c.author, c.title)
		}
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"The Colour of Magic": "colour of magic",
		"Mort":                "mort",
		"Ursula K. Le Guin":   "ursula k le guin",
		"Don't Panic!":        "don t panic",
		"A Hat Full of Sky":   "hat full of sky",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTitleKeys(t *testing.T) {
	keys := TitleKeys("Good Omens: The Nice and Accurate Prophecies")
	if len(keys) != 2 || keys[1] != "good omens" {
		t.Errorf("TitleKeys = %v", keys)
	}
}

// fixture creates a store with one root folder, two authors, three books,
// and a populated on-disk layout.
func fixture(t *testing.T) fx {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := library.NewStore(db)

	tp := &library.Author{Source: "hardcover", ForeignID: "100", Name: "Terry Pratchett", Monitored: true}
	if err := store.UpsertAuthor(tp); err != nil {
		t.Fatal(err)
	}
	ng := &library.Author{Source: "hardcover", ForeignID: "200", Name: "Neil Gaiman", Monitored: true}
	if err := store.UpsertAuthor(ng); err != nil {
		t.Fatal(err)
	}
	for _, b := range []*library.Book{
		{AuthorID: tp.ID, Source: "hardcover", ForeignID: "1", Title: "The Colour of Magic", Monitored: true},
		{AuthorID: tp.ID, Source: "hardcover", ForeignID: "2", Title: "Mort", Monitored: true},
		{AuthorID: ng.ID, Source: "hardcover", ForeignID: "3", Title: "Coraline", Monitored: true},
	} {
		if err := store.UpsertBook(b); err != nil {
			t.Fatal(err)
		}
	}

	rootDir := t.TempDir()
	if _, err := db.Exec(`INSERT INTO root_folders (media_type, path) VALUES ('ebook', ?)`, rootDir); err != nil {
		t.Fatal(err)
	}

	write := func(rel string) {
		path := filepath.Join(rootDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("ebook-bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Terry Pratchett/The Colour of Magic.epub") // author+title match
	write("Terry Pratchett/Discworld/02 - Mort.epub") // series dir + index prefix
	write("Coraline.epub")                            // title-only, unambiguous
	write("Terry Pratchett/notes.txt")                // ignored extension
	write("Unknown Author/Mystery Novel.epub")        // unmatched

	return fx{svc: New(store), store: store, db: db, rootDir: rootDir}
}

func TestScanMatchesAndReconciles(t *testing.T) {
	f := fixture(t)
	svc, store, rootDir := f.svc, f.store, f.rootDir
	ctx := context.Background()

	result, err := svc.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if result.Roots != 1 || result.Scanned != 4 || result.Matched != 3 || result.Unmatched != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	// hasFile flips on matched books only.
	books, _ := store.ListBooks(0)
	byTitle := map[string]library.Book{}
	for _, b := range books {
		byTitle[b.Title] = b
	}
	for _, title := range []string{"The Colour of Magic", "Mort", "Coraline"} {
		if !byTitle[title].HasFile {
			t.Errorf("%s should have a file", title)
		}
	}

	unmatched, _ := store.ListUnmatchedBookFiles()
	if len(unmatched) != 1 || filepath.Base(unmatched[0].Path) != "Mystery Novel.epub" {
		t.Fatalf("unmatched = %+v", unmatched)
	}

	// File details recorded.
	mortFiles, _ := store.ListBookFiles(byTitle["Mort"].ID)
	if len(mortFiles) != 1 || mortFiles[0].Format != "epub" || mortFiles[0].Size == 0 {
		t.Fatalf("mort files = %+v", mortFiles)
	}

	// Re-scan is idempotent.
	result2, err := svc.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result2.Scanned != 4 || result2.Removed != 0 {
		t.Fatalf("re-scan result = %+v", result2)
	}

	// Deleting a file on disk prunes its record on the next scan.
	if err := os.Remove(filepath.Join(rootDir, "Terry Pratchett", "Discworld", "02 - Mort.epub")); err != nil {
		t.Fatal(err)
	}
	result3, err := svc.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result3.Scanned != 3 || result3.Removed != 1 {
		t.Fatalf("post-delete result = %+v", result3)
	}
	books, _ = store.ListBooks(0)
	for _, b := range books {
		if b.Title == "Mort" && b.HasFile {
			t.Error("Mort still hasFile after its file was removed")
		}
	}
}

func TestScanUnmatchedGainsMatchAfterBookAdded(t *testing.T) {
	f := fixture(t)
	svc, store := f.svc, f.store
	ctx := context.Background()

	if _, err := svc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	unmatched, _ := store.ListUnmatchedBookFiles()
	if len(unmatched) != 1 {
		t.Fatalf("expected 1 unmatched file, got %d", len(unmatched))
	}

	// The mystery book gets added to the library; a re-scan matches the file.
	author := &library.Author{Source: "hardcover", ForeignID: "300", Name: "Unknown Author", Monitored: true}
	if err := store.UpsertAuthor(author); err != nil {
		t.Fatal(err)
	}
	book := &library.Book{AuthorID: author.ID, Source: "hardcover", ForeignID: "4", Title: "Mystery Novel", Monitored: true}
	if err := store.UpsertBook(book); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	unmatched, _ = store.ListUnmatchedBookFiles()
	if len(unmatched) != 0 {
		t.Fatalf("still unmatched after adding the book: %+v", unmatched)
	}
	got, _ := store.GetBook(book.ID)
	if !got.HasFile {
		t.Error("book should have gained its file on re-scan")
	}
}

func TestScanSkipsMissingRoot(t *testing.T) {
	f := fixture(t)

	// A root folder whose directory vanished after being added.
	gone := filepath.Join(t.TempDir(), "gone")
	if _, err := f.db.Exec(`INSERT INTO root_folders (media_type, path) VALUES ('ebook', ?)`, gone); err != nil {
		t.Fatal(err)
	}

	result, err := f.svc.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan should not fail outright: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 root error, got %+v", result.Errors)
	}
	if result.Scanned != 4 {
		t.Errorf("healthy root not scanned: %+v", result)
	}
}
