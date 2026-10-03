// Package comicinfo reads and writes ComicInfo.xml — the metadata format
// Kavita, Komga, and comic readers use, embedded at a CBZ/CBR archive's root.
// Writing is CBZ-only (CBR/RAR can't be written by pure Go); reading covers
// both, since many "proper" digital releases already ship one.
package comicinfo

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nwaples/rardecode/v2"
)

// BlackAndWhite values, per the anansi-project ComicInfo schema
// (https://anansi-project.github.io/docs/comicinfo/documentation) — the
// standard Kavita/Komga read to tell a colorized edition from a monochrome
// one. "Unknown" is the schema's default/absent state; LibriNode never writes
// it explicitly (see Info.BlackAndWhite's omitempty) — only Yes or No, when
// it actually knows.
const (
	BlackAndWhiteYes = "Yes"
	BlackAndWhiteNo  = "No"
)

// Info is the subset of the ComicInfo schema LibriNode reads and writes.
type Info struct {
	XMLName xml.Name `xml:"ComicInfo"`
	Series  string   `xml:"Series,omitempty"`
	Number  string   `xml:"Number,omitempty"`
	Title   string   `xml:"Title,omitempty"`
	Writer  string   `xml:"Writer,omitempty"`
	Summary string   `xml:"Summary,omitempty"`
	Year    int      `xml:"Year,omitempty"`
	// BlackAndWhite is "Yes", "No", or "Unknown" (the schema's three valid
	// values) — a reliable signal for manga/comic color-vs-monochrome
	// detection when the archive carries one, since it's a deliberate choice
	// by whoever produced the release, not a guess.
	BlackAndWhite string `xml:"BlackAndWhite,omitempty"`
}

// Read parses the ComicInfo.xml embedded in a .cbz or .cbr archive, if one is
// present. Returns (nil, nil) — not an error — when the archive has no
// ComicInfo.xml, or isn't a format this package can open: that's the common
// case, and callers fall back to their own detection.
func Read(archivePath string) (*Info, error) {
	switch strings.ToLower(filepath.Ext(archivePath)) {
	case ".cbz", ".zip":
		return readZip(archivePath)
	case ".cbr", ".rar":
		return readRar(archivePath)
	}
	return nil, nil
}

func readZip(p string) (*Info, error) {
	r, err := zip.OpenReader(p)
	if err != nil {
		return nil, fmt.Errorf("comicinfo: opening %s: %w", p, err)
	}
	defer r.Close()
	for _, f := range r.File {
		if strings.EqualFold(filepath.Base(f.Name), "ComicInfo.xml") {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return decode(rc)
		}
	}
	return nil, nil
}

func readRar(p string) (*Info, error) {
	files, err := rardecode.List(p)
	if err != nil {
		return nil, fmt.Errorf("comicinfo: opening %s: %w", p, err)
	}
	for _, f := range files {
		if !f.IsDir && strings.EqualFold(filepath.Base(f.Name), "ComicInfo.xml") {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return decode(rc)
		}
	}
	return nil, nil
}

func decode(r io.Reader) (*Info, error) {
	var info Info
	if err := xml.NewDecoder(r).Decode(&info); err != nil {
		return nil, fmt.Errorf("comicinfo: decoding ComicInfo.xml: %w", err)
	}
	return &info, nil
}

// Inject rewrites a .cbz adding (or replacing) ComicInfo.xml at the archive
// root. Non-cbz paths are ignored without error.
func Inject(cbzPath string, info Info) error {
	if strings.ToLower(filepath.Ext(cbzPath)) != ".cbz" {
		return nil
	}
	reader, err := zip.OpenReader(cbzPath)
	if err != nil {
		return fmt.Errorf("comicinfo: opening %s: %w", cbzPath, err)
	}
	defer reader.Close()

	tmp, err := os.CreateTemp(filepath.Dir(cbzPath), ".comicinfo-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	writer := zip.NewWriter(tmp)
	for _, entry := range reader.File {
		if strings.EqualFold(filepath.Base(entry.Name), "ComicInfo.xml") {
			continue // replaced below
		}
		if err := copyZipEntry(writer, entry); err != nil {
			writer.Close()
			tmp.Close()
			return err
		}
	}

	w, err := writer.Create("ComicInfo.xml")
	if err != nil {
		writer.Close()
		tmp.Close()
		return err
	}
	payload, err := xml.MarshalIndent(info, "", "  ")
	if err != nil {
		writer.Close()
		tmp.Close()
		return err
	}
	if _, err := w.Write(append([]byte(xml.Header), payload...)); err != nil {
		writer.Close()
		tmp.Close()
		return err
	}

	if err := writer.Close(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	reader.Close()
	return os.Rename(tmpPath, cbzPath)
}

func copyZipEntry(writer *zip.Writer, entry *zip.File) error {
	w, err := writer.CreateHeader(&zip.FileHeader{
		Name:   entry.Name,
		Method: entry.Method,
	})
	if err != nil {
		return err
	}
	r, err := entry.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	_, err = io.Copy(w, r)
	return err
}
