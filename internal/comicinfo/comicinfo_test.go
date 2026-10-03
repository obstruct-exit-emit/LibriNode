package comicinfo

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeCbz(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, content := range files {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write([]byte(content))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func readEntry(t *testing.T, path, name string) string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, entry := range r.File {
		if entry.Name == name {
			rc, _ := entry.Open()
			defer rc.Close()
			data, _ := io.ReadAll(rc)
			return string(data)
		}
	}
	return ""
}

func TestInject(t *testing.T) {
	cbz := filepath.Join(t.TempDir(), "Berserk v05.cbz")
	makeCbz(t, cbz, map[string]string{
		"page01.jpg":    "img1",
		"ComicInfo.xml": "<ComicInfo><Series>stale</Series></ComicInfo>",
	})

	err := Inject(cbz, Info{Series: "Berserk", Number: "5", Writer: "Kentarou Miura", Year: 1990})
	if err != nil {
		t.Fatalf("Inject: %v", err)
	}

	xml := readEntry(t, cbz, "ComicInfo.xml")
	for _, want := range []string{"<Series>Berserk</Series>", "<Number>5</Number>", "<Writer>Kentarou Miura</Writer>", "<Year>1990</Year>"} {
		if !strings.Contains(xml, want) {
			t.Errorf("ComicInfo.xml missing %s:\n%s", want, xml)
		}
	}
	if strings.Contains(xml, "stale") {
		t.Error("old ComicInfo.xml not replaced")
	}
	if readEntry(t, cbz, "page01.jpg") != "img1" {
		t.Error("page content lost during rewrite")
	}

	// Non-cbz is a quiet no-op.
	if err := Inject(filepath.Join(t.TempDir(), "x.cbr"), Info{}); err != nil {
		t.Errorf("cbr inject should no-op: %v", err)
	}
}

func TestReadZip(t *testing.T) {
	cbz := filepath.Join(t.TempDir(), "Dune v01.cbz")
	makeCbz(t, cbz, map[string]string{
		"page01.jpg": "img1",
		"ComicInfo.xml": `<?xml version="1.0"?>
<ComicInfo><Series>Dune</Series><Number>1</Number><BlackAndWhite>Yes</BlackAndWhite></ComicInfo>`,
	})

	info, err := Read(cbz)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if info == nil {
		t.Fatal("Read returned nil, want a parsed Info")
	}
	if info.Series != "Dune" || info.Number != "1" || info.BlackAndWhite != BlackAndWhiteYes {
		t.Errorf("Read = %+v, want Series=Dune Number=1 BlackAndWhite=Yes", info)
	}
}

func TestReadZipCaseInsensitiveAndNested(t *testing.T) {
	// Some producers lowercase the filename, or nest it under a subdirectory —
	// both should still be found.
	cbz := filepath.Join(t.TempDir(), "Berserk v01.cbz")
	makeCbz(t, cbz, map[string]string{
		"pages/page01.jpg":    "img1",
		"pages/comicinfo.xml": `<ComicInfo><BlackAndWhite>No</BlackAndWhite></ComicInfo>`,
	})
	info, err := Read(cbz)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if info == nil || info.BlackAndWhite != BlackAndWhiteNo {
		t.Errorf("Read = %+v, want BlackAndWhite=No", info)
	}
}

func TestReadZipNoComicInfo(t *testing.T) {
	cbz := filepath.Join(t.TempDir(), "noinfo.cbz")
	makeCbz(t, cbz, map[string]string{"page01.jpg": "img1"})

	info, err := Read(cbz)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if info != nil {
		t.Errorf("Read = %+v, want nil (no ComicInfo.xml present)", info)
	}
}

func TestReadUnsupportedExtension(t *testing.T) {
	// Not an error — callers (scanner.DetectVariant) fall through to their own
	// detection for anything this package can't open.
	info, err := Read(filepath.Join(t.TempDir(), "book.pdf"))
	if err != nil || info != nil {
		t.Errorf("Read(.pdf) = %+v, %v; want nil, nil", info, err)
	}
}

func TestInjectThenReadRoundTrip(t *testing.T) {
	cbz := filepath.Join(t.TempDir(), "roundtrip.cbz")
	makeCbz(t, cbz, map[string]string{"page01.jpg": "img1"})

	if err := Inject(cbz, Info{Series: "Trigun", BlackAndWhite: BlackAndWhiteNo}); err != nil {
		t.Fatalf("Inject: %v", err)
	}
	info, err := Read(cbz)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if info == nil || info.Series != "Trigun" || info.BlackAndWhite != BlackAndWhiteNo {
		t.Errorf("round-tripped Info = %+v, want Series=Trigun BlackAndWhite=No", info)
	}
}
