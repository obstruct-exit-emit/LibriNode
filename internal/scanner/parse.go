package scanner

import (
	"path/filepath"
	"regexp"
	"strings"
)

// ebookExtensions are the file types Phase 1 scans for (per the README's
// ebook format list). Audiobook/manga/comic extensions come with their
// phases.
var ebookExtensions = map[string]bool{
	".epub": true,
	".mobi": true,
	".azw3": true,
	".pdf":  true,
}

// ParsedFile is the scanner's best guess at what a file is, derived purely
// from its path. Zero fields mean "unknown".
type ParsedFile struct {
	Author string
	Title  string
}

// leadingIndex matches "01 - " / "1.5 - " series-position prefixes.
var leadingIndex = regexp.MustCompile(`^\d+(\.\d+)?\s*-\s*`)

// ParsePath guesses author and title from a path relative to the root
// folder. Recognized layouts:
//
//	Author/Title.epub
//	Author/Series/01 - Title.epub  (series dir ignored, index stripped)
//	Author - Title.epub            (flat)
//	Title.epub                     (title only)
func ParsePath(relPath string) ParsedFile {
	relPath = filepath.ToSlash(relPath)
	parts := strings.Split(relPath, "/")

	base := parts[len(parts)-1]
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = leadingIndex.ReplaceAllString(base, "")

	var p ParsedFile
	if len(parts) >= 2 {
		// First directory is the author by convention.
		p.Author = parts[0]
		p.Title = base
		// "Author - Title" inside an author dir: drop the redundant prefix.
		if prefix, rest, ok := strings.Cut(base, " - "); ok && strings.EqualFold(strings.TrimSpace(prefix), p.Author) {
			p.Title = strings.TrimSpace(rest)
		}
		return p
	}

	// Flat file: "Author - Title.ext", else just a title.
	if author, title, ok := strings.Cut(base, " - "); ok {
		p.Author = strings.TrimSpace(author)
		p.Title = strings.TrimSpace(title)
		return p
	}
	p.Title = base
	return p
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// Normalize reduces a name/title to a matching key: lowercase, punctuation
// collapsed to single spaces, leading English article dropped.
func Normalize(s string) string {
	s = strings.ToLower(s)
	s = nonAlnum.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(s, article) {
			s = s[len(article):]
			break
		}
	}
	return s
}

// TitleKeys returns the normalized match keys for a title: the full title
// and, when a subtitle is present ("Title: Subtitle"), the main title alone.
func TitleKeys(title string) []string {
	keys := []string{Normalize(title)}
	if main, _, ok := strings.Cut(title, ":"); ok {
		if k := Normalize(main); k != "" && k != keys[0] {
			keys = append(keys, k)
		}
	}
	return keys
}
