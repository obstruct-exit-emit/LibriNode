package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/librinode/librinode/internal/library"
)

func TestDeleteWithFiles(t *testing.T) {
	a := newTestAPI(t, fakeProvider{})

	rootDir := t.TempDir()
	bookPath := filepath.Join(rootDir, "Terry Pratchett", "The Colour of Magic.epub")
	if err := os.MkdirAll(filepath.Dir(bookPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bookPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.want(a.call("POST", "/api/v1/rootfolder",
		map[string]string{"mediaType": "ebook", "path": rootDir}, nil), http.StatusCreated)

	var author library.Author
	a.want(a.call("POST", "/api/v1/author", map[string]string{"foreignAuthorId": "100"}, &author), http.StatusCreated)
	a.want(a.call("POST", "/api/v1/library/scan", nil, nil), http.StatusOK)

	// Default delete is DB-only: the file survives on disk (a later scan
	// re-finds it as a stray).
	a.want(a.call("DELETE", fmt.Sprintf("/api/v1/author/%d", author.ID), nil, nil), http.StatusNoContent)
	if _, err := os.Stat(bookPath); err != nil {
		t.Fatalf("file should survive a plain delete: %v", err)
	}

	// Re-add and re-scan, then delete WITH files.
	a.want(a.call("POST", "/api/v1/author", map[string]string{"foreignAuthorId": "100"}, &author), http.StatusCreated)
	a.want(a.call("POST", "/api/v1/library/scan", nil, nil), http.StatusOK)

	var report struct {
		DeletedFiles int      `json:"deletedFiles"`
		Errors       []string `json:"errors"`
	}
	a.want(a.call("DELETE", fmt.Sprintf("/api/v1/author/%d?deleteFiles=true", author.ID), nil, &report), http.StatusOK)
	if report.DeletedFiles != 1 || len(report.Errors) != 0 {
		t.Fatalf("delete report = %+v", report)
	}
	if _, err := os.Stat(bookPath); !os.IsNotExist(err) {
		t.Fatal("file should be deleted from disk")
	}
	if _, err := os.Stat(filepath.Dir(bookPath)); !os.IsNotExist(err) {
		t.Fatal("emptied author directory should be pruned")
	}
	if _, err := os.Stat(rootDir); err != nil {
		t.Fatalf("root folder itself must survive: %v", err)
	}
}
