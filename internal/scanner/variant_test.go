package scanner

import (
	"archive/zip"
	"os"
	"path/filepath"
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
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestDetectVariantFromComicInfo(t *testing.T) {
	cases := []struct {
		blackAndWhite string
		want          string
	}{
		{"Yes", "mono"},
		{"No", "color"},
		{"Unknown", ""}, // explicit "Unknown" asserts nothing either way
		{"", ""},        // no BlackAndWhite element at all
	}
	for _, c := range cases {
		dir := t.TempDir()
		cbz := filepath.Join(dir, "Plain Title v01.cbz") // no color keyword in the name
		info := ""
		if c.blackAndWhite != "" {
			info = "<BlackAndWhite>" + c.blackAndWhite + "</BlackAndWhite>"
		}
		makeCbz(t, cbz, map[string]string{
			"ComicInfo.xml": "<ComicInfo>" + info + "</ComicInfo>",
		})
		if got := DetectVariant(cbz); got != c.want {
			t.Errorf("BlackAndWhite=%q: DetectVariant = %q, want %q", c.blackAndWhite, got, c.want)
		}
	}
}

func TestDetectVariantFilenameFallback(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Berserk v05 (Colorized).cbz", "color"},
		{"Berserk v05 [Color Edition].cbz", "color"},
		{"Berserk v05 (Digital-Colour).cbz", "color"},
		{"Berserk v05.cbz", ""},             // no signal at all — plain releases are the norm
		{"Colorful Adventures v01.cbz", ""}, // "color" must be a whole word, not a substring
	}
	for _, c := range cases {
		dir := t.TempDir()
		cbz := filepath.Join(dir, c.name)
		makeCbz(t, cbz, map[string]string{"page01.jpg": "img"}) // no ComicInfo.xml
		if got := DetectVariant(cbz); got != c.want {
			t.Errorf("%q: DetectVariant = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDetectVariantComicInfoBeatsFilename(t *testing.T) {
	// A file whose name says "Colorized" but whose own ComicInfo.xml says
	// BlackAndWhite=Yes trusts the ComicInfo.xml — it's the deliberate,
	// structured signal, checked first.
	dir := t.TempDir()
	cbz := filepath.Join(dir, "Berserk v05 (Colorized).cbz")
	makeCbz(t, cbz, map[string]string{
		"ComicInfo.xml": "<ComicInfo><BlackAndWhite>Yes</BlackAndWhite></ComicInfo>",
	})
	if got := DetectVariant(cbz); got != "mono" {
		t.Errorf("DetectVariant = %q, want mono (ComicInfo.xml should win over the filename)", got)
	}
}

func TestDetectVariantFolderKeyword(t *testing.T) {
	// The colorized tag often sits on the containing folder, not the file.
	dir := filepath.Join(t.TempDir(), "Berserk (Colorized)")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cbz := filepath.Join(dir, "Berserk v05.cbz")
	makeCbz(t, cbz, map[string]string{"page01.jpg": "img"})
	if got := DetectVariant(cbz); got != "color" {
		t.Errorf("DetectVariant = %q, want color (folder name carries the keyword)", got)
	}
}
