// Package tagwriter writes LibriNode's metadata into an audiobook file's own
// embedded tags, so other players — Audiobookshelf, Plex, a phone app — read
// consistent info straight from the file rather than whatever a rip shipped
// with. It routes through go.senan.xyz/taglib (upstream TagLib compiled to
// WASM, run via wazero — no cgo), the same writer CantiNode uses: it only
// touches the fields set (never replacing the whole tag block, so an existing
// comment/cover/rating survives) and rewrites MP4 atom offset tables correctly
// when the metadata size changes.
//
// This is the one place LibriNode mutates a file's contents (Organize only
// moves/renames), so it is always an explicit, opt-in action.
package tagwriter

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrUnsupportedFormat is returned by Write for a file type TagLib can't tag.
var ErrUnsupportedFormat = errors.New("tagwriter: unsupported format")

// Tags is the audiobook metadata Write embeds — sourced from LibriNode's
// database (the matched book/author, and the narrator it read or matched), not
// re-derived from the file's own existing tags.
type Tags struct {
	Title    string // → title
	Author   string // → artist and album artist (Audiobookshelf reads authors from these)
	Album    string // → album (usually the title, or the series)
	Narrator string // → composer, the long-standing audiobook narrator convention
	Date     string // → date (release date or year)
	// CoverImage, when non-empty and enabled, is embedded as the front cover.
	CoverImage []byte
}

// Toggles gates which of Tags' fields Write actually touches — one bool per
// field. A disabled field is omitted from the write entirely (never set, never
// cleared). Settings-driven; AllEnabled is the default.
type Toggles struct {
	Title      bool
	Author     bool
	Album      bool
	Narrator   bool
	Date       bool
	CoverImage bool
}

// AllEnabled writes every field — the default when a caller has no per-field
// preference.
var AllEnabled = Toggles{Title: true, Author: true, Album: true, Narrator: true, Date: true, CoverImage: true}

// Write embeds tags into the audio file at path. Only enabled, non-empty fields
// are touched; everything else in the file is left alone. clear first strips
// the tags LibriNode doesn't manage (comments, ratings, an existing cover…)
// before writing — a deliberate clean-slate rewrite.
func Write(path string, tags Tags, clear bool, enabled Toggles) error {
	if !IsSupported(path) {
		return ErrUnsupportedFormat
	}
	return writeTagLib(path, tags, clear, enabled)
}

// IsSupported reports whether Write can tag path's format — used by the API/UI
// to decide whether to offer "Write tags" rather than failing after the fact.
func IsSupported(path string) bool {
	switch extOf(path) {
	case "mp3", "flac", "m4a", "m4b", "m4p", "ogg", "oga", "opus", "dsf", "wav":
		return true
	default:
		return false
	}
}

func extOf(path string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
}
