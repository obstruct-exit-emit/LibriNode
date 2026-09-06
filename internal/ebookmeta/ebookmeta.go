// Package ebookmeta reads (and, for EPUB, writes) an ebook file's own embedded
// metadata, so the UI can show "what's actually on this file" and the tag
// writer can embed LibriNode's metadata into it. EPUB is the one format with a
// standard, openable container (a ZIP holding an OPF XML document) — it is read
// in full and written. MOBI/AZW3 (Amazon's binary EXTH) and PDF (Info dict) are
// read best-effort only: their identifiers and title/author where present, but
// never written, since rewriting those proprietary structures risks corrupting
// files the user's other apps read.
package ebookmeta

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Tags is an ebook file's embedded metadata. Empty fields mean the format
// didn't carry that value (or we couldn't read it — best-effort by design).
type Tags struct {
	Title       string `json:"title"`
	Author      string `json:"author"`
	Series      string `json:"series"`
	SeriesIndex string `json:"seriesIndex"`
	Genre       string `json:"genre"`
	Description string `json:"description"`
	Language    string `json:"language"`
	Publisher   string `json:"publisher"`
	Date        string `json:"date"`
	ISBN        string `json:"isbn"`
	ASIN        string `json:"asin"`
	// Format is the lowercased extension (epub/pdf/mobi/azw3).
	Format string `json:"format"`
	// Writable reports whether Write can embed tags into this format (EPUB only).
	Writable bool `json:"writable"`
}

// IsEbookPath reports whether path is a format this package can read.
func IsEbookPath(path string) bool {
	switch ext(path) {
	case "epub", "mobi", "azw3", "azw", "pdf":
		return true
	default:
		return false
	}
}

// IsWritable reports whether Write can embed tags into path's format (EPUB only).
func IsWritable(path string) bool { return ext(path) == "epub" }

// Read parses the ebook file at path and returns its embedded metadata.
func Read(path string) (*Tags, error) {
	e := ext(path)
	var (
		t   *Tags
		err error
	)
	switch e {
	case "epub":
		t, err = readEPUB(path)
	case "mobi", "azw3", "azw":
		t, err = readMOBI(path)
	case "pdf":
		t, err = readPDF(path)
	default:
		return nil, fmt.Errorf("ebookmeta: unsupported format %q", e)
	}
	if err != nil {
		return nil, err
	}
	if t == nil {
		t = &Tags{}
	}
	t.Format = e
	t.Writable = e == "epub"
	return t, nil
}

func ext(path string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
}
