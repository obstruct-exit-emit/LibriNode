package scanner

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

// AudioNarrator returns the narrator an audiobook declares about itself — read
// straight from the file rather than inferred. It looks first at the audio
// file's embedded tags (the Composer field, "©wrt" on MP4 / "TCOM" on MP3, the
// long-standing audiobook convention for the reader; then a "Read by …"
// comment; then a raw "narrator" field), and failing that at an .opf sidecar in
// the book folder. Empty when nothing names one.
//
// path is a single file or a book folder (multi-file); a folder reads its first
// audio track's tags and looks for the sidecar alongside. The embedded tag/OPF
// use dhowden/tag (pure Go, no cgo) — the same reader CantiNode uses.
func AudioNarrator(path string) string {
	dir, audioFile := path, path
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		audioFile = firstAudioFile(path)
	} else {
		dir = filepath.Dir(path)
	}
	if audioFile != "" {
		if n := tagNarrator(audioFile); n != "" {
			return n
		}
	}
	return opfNarrator(dir)
}

func firstAudioFile(dir string) string {
	var found string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || found != "" || !IsAudioPath(p) {
			return nil
		}
		found = p
		return nil
	})
	return found
}

// tagNarrator reads the narrator from one audio file's embedded metadata.
func tagNarrator(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		return ""
	}
	if c := strings.TrimSpace(m.Composer()); c != "" {
		return c
	}
	if n := narratorFromComment(m.Comment()); n != "" {
		return n
	}
	for k, v := range m.Raw() {
		if !strings.Contains(strings.ToLower(k), "narrator") {
			continue
		}
		if s, ok := v.(string); ok {
			// MP4 freeform values can carry leading NUL padding (a dhowden/tag
			// quirk CantiNode documents); trim it.
			if s = strings.TrimSpace(strings.TrimLeft(s, "\x00")); s != "" {
				return s
			}
		}
	}
	return ""
}

// narratorFromComment pulls a name out of "Read by X" / "Narrated by X",
// stopping at the first sentence/line break so trailing production notes don't
// leak in.
func narratorFromComment(comment string) string {
	c := strings.TrimSpace(comment)
	lower := strings.ToLower(c)
	for _, p := range []string{"narrated by ", "read by ", "narrator: ", "narrator "} {
		i := strings.Index(lower, p)
		if i < 0 {
			continue
		}
		after := c[i+len(p):]
		if j := strings.IndexAny(after, ".;\n\r"); j >= 0 {
			after = after[:j]
		}
		if after = strings.TrimSpace(after); after != "" {
			return after
		}
	}
	return ""
}

// opfNarrator reads a narrator from an .opf sidecar in dir: a
// <meta name="…narrator…" content="X"/> (LazyLibrarian/Calibre style) or a
// <dc:contributor opf:role="nrt">X</dc:contributor> (the OPF standard role).
func opfNarrator(dir string) string {
	if dir == "" {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.opf"))
	for _, opf := range matches {
		if n := parseOPFNarrator(opf); n != "" {
			return n
		}
	}
	return ""
}

func parseOPFNarrator(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pkg struct {
		Metadata struct {
			Meta []struct {
				Name    string `xml:"name,attr"`
				Content string `xml:"content,attr"`
			} `xml:"meta"`
			Contributors []struct {
				Role string `xml:"role,attr"`
				Text string `xml:",chardata"`
			} `xml:"contributor"`
		} `xml:"metadata"`
	}
	if xml.Unmarshal(data, &pkg) != nil {
		return ""
	}
	for _, m := range pkg.Metadata.Meta {
		if strings.Contains(strings.ToLower(m.Name), "narrator") {
			if v := strings.TrimSpace(m.Content); v != "" {
				return v
			}
		}
	}
	for _, c := range pkg.Metadata.Contributors {
		if strings.EqualFold(strings.TrimSpace(c.Role), "nrt") {
			if v := strings.TrimSpace(c.Text); v != "" {
				return v
			}
		}
	}
	return ""
}
