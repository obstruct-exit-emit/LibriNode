package tagwriter

import (
	"strings"

	"github.com/librinode/librinode/internal/ebookmeta"
)

// writeEbook maps the enabled Tags onto an EPUB's OPF metadata. Narrator, album,
// and cover-image don't apply to ebooks and are ignored; everything else mirrors
// the audio writer's per-field toggles.
func writeEbook(path string, tags Tags, enabled Toggles) error {
	f := ebookmeta.Fields{}
	if enabled.Title {
		f.Title = tags.Title
	}
	if enabled.Author {
		f.Author = tags.Author
	}
	if enabled.Series {
		f.Series = tags.Series
		f.SeriesIndex = tags.SeriesIndex
	}
	if enabled.Genre {
		f.Genres = splitGenres(tags.Genre)
	}
	if enabled.Description {
		f.Description = tags.Description
	}
	if enabled.Date {
		f.Date = tags.Date
	}
	if enabled.Identifier {
		// ISBN is the ebook identifier; the tags' ASIN is the audiobook
		// edition's, so it's not written into an ebook.
		f.ISBN = tags.ISBN
	}
	return ebookmeta.WriteEPUB(path, f)
}

// splitGenres turns the comma-joined Genre string back into individual subjects
// (the api handler joins book.Genres with ", ").
func splitGenres(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ", ") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
