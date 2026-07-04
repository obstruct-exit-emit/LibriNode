package scanner

import (
	"path/filepath"
	"regexp"
	"strconv"
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

// IsEbookPath reports whether a filename has one of the ebook extensions
// LibriNode handles (used by the scanner and the download importer).
func IsEbookPath(name string) bool {
	return ebookExtensions[strings.ToLower(filepath.Ext(name))]
}

// audioExtensions are the audiobook file types (m4a covers m4b-style
// containers that ship misnamed).
var audioExtensions = map[string]bool{
	".m4b":  true,
	".m4a":  true,
	".mp3":  true,
	".flac": true,
	".opus": true,
}

// IsAudioPath reports whether a filename is an audiobook audio file.
func IsAudioPath(name string) bool {
	return audioExtensions[strings.ToLower(filepath.Ext(name))]
}

// comicExtensions are the archive types manga/comic roots scan for.
var comicExtensions = map[string]bool{
	".cbz":  true,
	".cbr":  true,
	".pdf":  true,
	".epub": true,
}

// IsComicPath reports whether a filename is a comic/manga archive.
func IsComicPath(name string) bool {
	return comicExtensions[strings.ToLower(filepath.Ext(name))]
}

var volumeMarker = regexp.MustCompile(`(?i)(?:\bv|\bvol\.?\s*|\bvolume\s+|#)(\d{1,4}(?:\.\d+)?)`)

// VolumeFromName extracts a volume/issue number from a filename ("Berserk
// v05", "Berserk Vol. 5", "The Walking Dead #12"); 0 means none found.
func VolumeFromName(name string) float64 {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	if m := volumeMarker.FindStringSubmatch(base); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		return v
	}
	return 0
}

// ParsedFile is the scanner's best guess at what a file is, derived purely
// from its path. Zero fields mean "unknown". AltTitle holds the segment
// after the last " - " when the primary title contains one — how
// "Discworld 8 - Guards! Guards!.epub" (our own naming template's output)
// still matches the book "Guards! Guards!".
type ParsedFile struct {
	Author   string
	Title    string
	AltTitle string
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
		p.AltTitle = lastDashSegment(p.Title)
		return p
	}

	// Flat file: "Author - Title.ext", else just a title.
	if author, title, ok := strings.Cut(base, " - "); ok {
		p.Author = strings.TrimSpace(author)
		p.Title = strings.TrimSpace(title)
		p.AltTitle = lastDashSegment(p.Title)
		return p
	}
	p.Title = base
	return p
}

// lastDashSegment returns the text after the last " - ", when different from
// the whole ("Discworld 8 - Guards! Guards!" → "Guards! Guards!").
func lastDashSegment(s string) string {
	if i := strings.LastIndex(s, " - "); i >= 0 {
		if seg := strings.TrimSpace(s[i+3:]); seg != "" && seg != s {
			return seg
		}
	}
	return ""
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
