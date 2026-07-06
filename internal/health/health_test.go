package health

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/librinode/librinode/internal/database"
	"github.com/librinode/librinode/internal/download"
	"github.com/librinode/librinode/internal/indexer"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/metadata"
)

func TestCheckFindsIssues(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// One root folder that exists, one that has vanished since it was added.
	ok := t.TempDir()
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct{ mt, path string }{{"ebook", ok}, {"audiobook", gone}} {
		if _, err := db.Exec(`INSERT INTO root_folders (media_type, path) VALUES (?, ?)`, f.mt, f.path); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	svc := New(
		library.NewStore(db),
		indexer.NewService(indexer.NewStore(db)),
		download.NewService(download.NewStore(db)),
		metadata.NewManager(),
	)

	if !svc.Last().CheckedAt.IsZero() {
		t.Error("Last() before any check should have zero CheckedAt")
	}

	res := svc.Check(context.Background())
	if res.CheckedAt.IsZero() {
		t.Error("Check() result missing CheckedAt")
	}
	if got := svc.Last(); got.CheckedAt != res.CheckedAt {
		t.Error("Last() should return the cached result of Check()")
	}

	// Expected: the vanished root folder (error), plus warnings for no
	// metadata provider, no indexers, and no download clients.
	levels := map[string]string{}
	counts := map[string]int{}
	for _, is := range res.Issues {
		levels[is.Source] = is.Level
		counts[is.Source]++
	}
	want := map[string]string{
		"root-folder":     LevelError,
		"metadata":        LevelWarning,
		"indexer":         LevelWarning,
		"download-client": LevelWarning,
	}
	for src, lvl := range want {
		if levels[src] != lvl {
			t.Errorf("source %s: level = %q, want %q (issues: %+v)", src, levels[src], lvl, res.Issues)
		}
	}
	if counts["root-folder"] != 1 {
		t.Errorf("root-folder issues = %d, want 1 (the accessible folder must not be flagged)", counts["root-folder"])
	}
	if len(res.Issues) > 0 && res.Issues[0].Level != LevelError {
		t.Errorf("issues not sorted errors-first: %+v", res.Issues)
	}
}
