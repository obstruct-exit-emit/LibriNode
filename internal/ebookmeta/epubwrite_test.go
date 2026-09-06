package ebookmeta

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleOPF = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>Old Title</dc:title>
    <dc:creator opf:role="aut">Old Author</dc:creator>
    <dc:creator opf:role="ill">An Illustrator</dc:creator>
    <dc:identifier id="bookid" opf:scheme="UUID">urn:uuid:12345678</dc:identifier>
    <dc:language>en</dc:language>
    <dc:subject>OldGenre</dc:subject>
    <meta name="cover" content="cover-img"/>
  </metadata>
  <manifest>
    <item id="cover-img" href="cover.jpg" media-type="image/jpeg"/>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx">
    <itemref idref="c1"/>
  </spine>
</package>`

const containerXML = `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`

func writeSampleEpub(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.epub")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	// mimetype must be first and stored (uncompressed).
	mw, err := zw.CreateRaw(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	mimetype := []byte("application/epub+zip")
	mw.Write(mimetype)
	for name, body := range map[string]string{
		"META-INF/container.xml": containerXML,
		"content.opf":            sampleOPF,
		"ch1.xhtml":              "<html><body>Chapter 1</body></html>",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWriteEPUB(t *testing.T) {
	path := writeSampleEpub(t)

	// Snapshot the non-OPF entries to prove they survive byte-for-byte.
	before := readEntries(t, path)

	f := Fields{
		Title:       "New Title",
		Author:      "New Author",
		Series:      "My Series",
		SeriesIndex: "3",
		Genres:      []string{"Science Fiction", "Fantasy"},
		Language:    "en-US",
		Description: "A brand new description with <angle> & ampersand.",
		Date:        "2020-01-01",
		ISBN:        "9780765396358",
	}
	if err := WriteEPUB(path, f); err != nil {
		t.Fatalf("WriteEPUB: %v", err)
	}

	// Round-trip: the reader sees the new values.
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	checks := map[string]struct{ got, want string }{
		"title":       {got.Title, "New Title"},
		"author":      {got.Author, "New Author"},
		"series":      {got.Series, "My Series"},
		"seriesIndex": {got.SeriesIndex, "3"},
		"language":    {got.Language, "en-US"},
		"description": {got.Description, "A brand new description with <angle> & ampersand."},
		"isbn":        {got.ISBN, "9780765396358"},
		"genre":       {got.Genre, "Science Fiction, Fantasy"},
	}
	for name, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", name, c.got, c.want)
		}
	}

	// The archive stays a valid epub: mimetype first and stored.
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer zr.Close()
	if zr.File[0].Name != "mimetype" || zr.File[0].Method != zip.Store {
		t.Errorf("mimetype entry: name=%q method=%d, want mimetype/Store first", zr.File[0].Name, zr.File[0].Method)
	}

	// Non-OPF entries are byte-identical to before.
	after := readEntries(t, path)
	for name, data := range before {
		if name == "content.opf" {
			continue
		}
		if !bytes.Equal(after[name], data) {
			t.Errorf("entry %q changed; must be preserved verbatim", name)
		}
	}

	// The OPF still has its manifest/spine, cover meta, illustrator, and the
	// untouched unique-identifier — and the old genre is gone.
	opf := string(after["content.opf"])
	for _, want := range []string{
		"<manifest", "<spine", `href="ch1.xhtml"`, `name="cover"`,
		"urn:uuid:12345678", "An Illustrator", `name="calibre:series"`, `content="3"`,
	} {
		if !strings.Contains(opf, want) {
			t.Errorf("rewritten OPF missing %q\n---\n%s", want, opf)
		}
	}
	if strings.Contains(opf, "OldGenre") {
		t.Errorf("rewritten OPF still has the replaced OldGenre subject")
	}
	if strings.Count(opf, "<dc:title") != 1 {
		t.Errorf("expected exactly one dc:title, got %d", strings.Count(opf, "<dc:title"))
	}
}

func readEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := map[string][]byte{}
	for _, f := range zr.File {
		b, err := readAll(f)
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = b
	}
	return out
}
