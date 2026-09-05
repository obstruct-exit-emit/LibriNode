package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNarratorFromComment(t *testing.T) {
	cases := map[string]string{
		"Read by Erin Jones":                     "Erin Jones",
		"Narrated by Ray Porter":                 "Ray Porter",
		"Narrated by Scott Brick. Recorded 2009": "Scott Brick",
		"Narrator: Simon Vance\nUnabridged":      "Simon Vance",
		"An epic space opera":                    "",
		"":                                       "",
	}
	for in, want := range cases {
		if got := narratorFromComment(in); got != want {
			t.Errorf("narratorFromComment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseOPFNarrator(t *testing.T) {
	// LazyLibrarian-style meta.
	ll := `<?xml version="1.0" encoding="UTF-8"?>
<package version="2.0" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>Brightness Reef</dc:title>
    <meta content="Erin Jones" name="lazylibrarian:narrator"/>
  </metadata>
</package>`
	// Standard OPF narrator role (nrt).
	nrt := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:creator opf:role="aut">David Brin</dc:creator>
    <dc:contributor opf:role="nrt">George K. Wilson</dc:contributor>
  </metadata>
</package>`
	for name, body := range map[string]string{"Erin Jones": ll, "George K. Wilson": nrt} {
		dir := t.TempDir()
		p := filepath.Join(dir, "book.opf")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := parseOPFNarrator(p); got != name {
			t.Errorf("parseOPFNarrator = %q, want %q", got, name)
		}
		// AudioNarrator should find the sidecar when the dir has no readable audio.
		if got := opfNarrator(dir); got != name {
			t.Errorf("opfNarrator = %q, want %q", got, name)
		}
	}
}
