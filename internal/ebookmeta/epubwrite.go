package ebookmeta

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Fields are the values to embed into an epub — LibriNode's metadata for the
// book. Only non-empty fields are applied (merge semantics, like the audio
// writer): a blank never wipes what the file already has.
type Fields struct {
	Title       string
	Author      string
	Series      string
	SeriesIndex string
	Genres      []string
	Description string
	Language    string
	Date        string
	ISBN        string
	ASIN        string
}

// WriteEPUB embeds f into the epub at path by rewriting its OPF's <metadata>
// block and leaving every other zip entry byte-for-byte unchanged. Managed
// fields that already exist have their text replaced in place (preserving the
// element's id/attributes, so EPUB3 refines and the package unique-identifier
// keep pointing at valid targets); missing ones are appended. Identifiers are
// only added, never edited or removed, so the book's unique id is never
// disturbed. The new archive is written to a temp file and atomically renamed.
func WriteEPUB(path string, f Fields) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()

	opf := findOPFFile(&zr.Reader)
	if opf == nil {
		return errors.New("ebookmeta: epub has no OPF package document")
	}
	opfBytes, err := readAll(opf)
	if err != nil {
		return err
	}
	newOPF, err := rewriteMetadata(opfBytes, f)
	if err != nil {
		return fmt.Errorf("rewriting OPF: %w", err)
	}

	tmp := path + ".libri.tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	writeErr := func() error {
		for _, file := range zr.File {
			if file.Name == opf.Name {
				// Rewrite the OPF (compressed); its bytes changed.
				w, err := zw.Create(file.Name)
				if err != nil {
					return err
				}
				if _, err := w.Write(newOPF); err != nil {
					return err
				}
				continue
			}
			// Everything else copied byte-exact, preserving the entry's
			// compression method — crucial for the stored, first "mimetype".
			hdr := file.FileHeader
			w, err := zw.CreateRaw(&hdr)
			if err != nil {
				return err
			}
			rc, err := file.OpenRaw()
			if err != nil {
				return err
			}
			if _, err := io.Copy(w, rc); err != nil {
				return err
			}
		}
		return nil
	}()
	if writeErr == nil {
		writeErr = zw.Close()
	} else {
		_ = zw.Close()
	}
	if cerr := out.Close(); writeErr == nil {
		writeErr = cerr
	}
	if writeErr != nil {
		os.Remove(tmp)
		return writeErr
	}
	// zr still holds the original open; close it before replacing on platforms
	// that dislike renaming over an open file.
	zr.Close()
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// childClass tags a direct child of <metadata> by how the writer treats it.
type childClass int

const (
	classKeep childClass = iota
	classTitle
	classCreator
	classDescription
	classDate
	classLanguage
	classSubject
	classCalibreSeries // calibre:series or calibre:series_index meta
	classIdentifier
)

// rewriteMetadata returns the OPF with its <metadata> children updated: managed
// fields replaced in place or appended, unmanaged children kept verbatim.
func rewriteMetadata(opf []byte, f Fields) ([]byte, error) {
	innerStart, innerEnd, children, err := scanMetadata(opf)
	if err != nil {
		return nil, err
	}
	if innerStart < 0 || innerEnd < 0 {
		return nil, errors.New("no <metadata> element")
	}

	var b bytes.Buffer
	b.Write(opf[:innerStart])
	b.WriteByte('\n')

	titleDone, authorDone, descDone, dateDone, langDone := false, false, false, false, false
	haveISBN, haveASIN := false, false
	emit := func(raw []byte) {
		b.WriteString("    ")
		b.Write(bytes.TrimSpace(raw))
		b.WriteByte('\n')
	}
	for _, c := range children {
		raw := opf[c.start:c.end]
		switch c.class {
		case classTitle:
			if f.Title != "" && !titleDone {
				emit(replaceElementText(raw, f.Title))
				titleDone = true
			} else {
				emit(raw)
			}
		case classCreator:
			if f.Author != "" && !authorDone {
				emit(replaceElementText(raw, f.Author))
				authorDone = true
			} else {
				emit(raw)
			}
		case classDescription:
			if f.Description != "" && !descDone {
				emit(replaceElementText(raw, f.Description))
				descDone = true
			} else {
				emit(raw)
			}
		case classDate:
			if f.Date != "" && !dateDone {
				emit(replaceElementText(raw, f.Date))
				dateDone = true
			} else {
				emit(raw)
			}
		case classLanguage:
			if f.Language != "" && !langDone {
				emit(replaceElementText(raw, f.Language))
				langDone = true
			} else {
				emit(raw)
			}
		case classSubject:
			if len(f.Genres) == 0 { // only replace subjects when we have genres
				emit(raw)
			} // else drop; fresh subjects appended below
		case classCalibreSeries:
			if f.Series == "" {
				emit(raw)
			} // else drop; fresh series meta appended below
		case classIdentifier:
			if c.scheme == "ISBN" {
				haveISBN = true
			}
			if c.scheme == "ASIN" || c.scheme == "AMAZON" || c.scheme == "MOBI-ASIN" {
				haveASIN = true
			}
			emit(raw) // identifiers are never edited or removed
		default:
			emit(raw)
		}
	}

	// Append managed fields that had no existing element to replace.
	if f.Title != "" && !titleDone {
		emit([]byte(el("dc:title", f.Title)))
	}
	if f.Author != "" && !authorDone {
		emit([]byte(`<dc:creator opf:role="aut">` + esc(f.Author) + `</dc:creator>`))
	}
	if f.Description != "" && !descDone {
		emit([]byte(el("dc:description", f.Description)))
	}
	if f.Date != "" && !dateDone {
		emit([]byte(el("dc:date", f.Date)))
	}
	if f.Language != "" && !langDone {
		emit([]byte(el("dc:language", f.Language)))
	}
	for _, g := range f.Genres {
		if strings.TrimSpace(g) != "" {
			emit([]byte(el("dc:subject", g)))
		}
	}
	if f.Series != "" {
		emit([]byte(`<meta name="calibre:series" content="` + esc(f.Series) + `"/>`))
		if f.SeriesIndex != "" {
			emit([]byte(`<meta name="calibre:series_index" content="` + esc(f.SeriesIndex) + `"/>`))
		}
	}
	if f.ISBN != "" && !haveISBN {
		emit([]byte(`<dc:identifier opf:scheme="ISBN">` + esc(f.ISBN) + `</dc:identifier>`))
	}
	if f.ASIN != "" && !haveASIN {
		emit([]byte(`<dc:identifier opf:scheme="ASIN">` + esc(f.ASIN) + `</dc:identifier>`))
	}

	b.WriteString("  ")
	b.Write(opf[innerEnd:])
	return b.Bytes(), nil
}

// metaChild is one direct child element of <metadata> with its byte span.
type metaChild struct {
	start, end int
	class      childClass
	scheme     string // for identifiers
}

// scanMetadata locates the inner byte range of <metadata> and classifies each
// direct child element, recording its exact byte span for verbatim reuse.
func scanMetadata(opf []byte) (innerStart, innerEnd int, children []metaChild, err error) {
	innerStart, innerEnd = -1, -1
	dec := xml.NewDecoder(bytes.NewReader(opf))
	depth := 0
	inMeta := false
	metaDepth := 0
	for {
		off := dec.InputOffset()
		tok, terr := dec.Token()
		if terr == io.EOF {
			break
		}
		if terr != nil {
			return 0, 0, nil, terr
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if !inMeta && t.Name.Local == "metadata" {
				inMeta = true
				metaDepth = depth
				innerStart = int(dec.InputOffset())
				depth++
				continue
			}
			if inMeta && depth == metaDepth+1 {
				start := int(off)
				if serr := dec.Skip(); serr != nil {
					return 0, 0, nil, serr
				}
				children = append(children, metaChild{
					start:  start,
					end:    int(dec.InputOffset()),
					class:  classify(t),
					scheme: strings.ToUpper(attrLocal(t, "scheme")),
				})
				continue
			}
			depth++
		case xml.EndElement:
			if inMeta && t.Name.Local == "metadata" && depth == metaDepth+1 {
				innerEnd = int(off)
				inMeta = false
			}
			depth--
		}
	}
	return innerStart, innerEnd, children, nil
}

func classify(se xml.StartElement) childClass {
	switch se.Name.Local {
	case "title":
		return classTitle
	case "creator":
		if role := attrLocal(se, "role"); role != "" && role != "aut" {
			return classKeep // editor/illustrator/etc. left alone
		}
		return classCreator
	case "description":
		return classDescription
	case "date":
		return classDate
	case "language":
		return classLanguage
	case "subject":
		return classSubject
	case "identifier":
		return classIdentifier
	case "meta":
		switch attrLocal(se, "name") {
		case "calibre:series", "calibre:series_index":
			return classCalibreSeries
		}
	}
	return classKeep
}

// replaceElementText swaps an element's text content, keeping its start tag
// (and thus every attribute/id) and end tag intact. A self-closing element has
// no text slot, so it's returned unchanged.
func replaceElementText(raw []byte, newText string) []byte {
	gt := bytes.IndexByte(raw, '>')
	if gt <= 0 || raw[gt-1] == '/' {
		return raw
	}
	lt := bytes.LastIndexByte(raw, '<')
	if lt <= gt {
		return raw
	}
	var b bytes.Buffer
	b.Write(raw[:gt+1])
	b.WriteString(esc(newText))
	b.Write(raw[lt:])
	return b.Bytes()
}

func el(tag, text string) string { return "<" + tag + ">" + esc(text) + "</" + tag + ">" }

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func readAll(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
