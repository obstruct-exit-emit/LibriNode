package ebookmeta

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"strings"

	"github.com/librinode/librinode/internal/scanner"
)

// readEPUB opens the epub (a zip), locates its OPF package document, and parses
// the Dublin Core metadata out of it.
func readEPUB(path string) (*Tags, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	f := findOPFFile(zr)
	if f == nil {
		return &Tags{}, nil // no package document — nothing to read
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return parseOPF(rc), nil
}

// findOPFFile locates the package document: META-INF/container.xml names it, a
// direct scan for a *.opf entry is the fallback. (Mirrors scanner.findOPF, kept
// local so this package stands alone.)
func findOPFFile(zr *zip.ReadCloser) *zip.File {
	var firstOPF *zip.File
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, "META-INF/container.xml") {
			if p := opfPathFromContainer(f); p != "" {
				for _, g := range zr.File {
					if g.Name == p {
						return g
					}
				}
			}
		}
		if firstOPF == nil && strings.HasSuffix(strings.ToLower(f.Name), ".opf") {
			firstOPF = f
		}
	}
	return firstOPF
}

func opfPathFromContainer(f *zip.File) string {
	rc, err := f.Open()
	if err != nil {
		return ""
	}
	defer rc.Close()
	var doc struct {
		Rootfiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if xml.NewDecoder(rc).Decode(&doc) != nil || len(doc.Rootfiles) == 0 {
		return ""
	}
	return doc.Rootfiles[0].FullPath
}

// parseOPF walks the OPF's <metadata> by local element name (producers vary in
// namespace prefixes) and fills a Tags.
func parseOPF(r io.Reader) *Tags {
	t := &Tags{}
	var creators, subjects []string
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "title":
			if t.Title == "" {
				t.Title = strings.TrimSpace(elemText(dec, &se))
			}
		case "creator":
			// Skip explicitly non-author roles (editor, illustrator…).
			if role := attrLocal(se, "role"); role != "" && role != "aut" {
				continue
			}
			if name := strings.TrimSpace(elemText(dec, &se)); name != "" {
				creators = append(creators, name)
			}
		case "description":
			if t.Description == "" {
				t.Description = strings.TrimSpace(elemText(dec, &se))
			}
		case "date":
			if t.Date == "" {
				t.Date = strings.TrimSpace(elemText(dec, &se))
			}
		case "language":
			if t.Language == "" {
				t.Language = strings.TrimSpace(elemText(dec, &se))
			}
		case "publisher":
			if t.Publisher == "" {
				t.Publisher = strings.TrimSpace(elemText(dec, &se))
			}
		case "subject":
			if s := strings.TrimSpace(elemText(dec, &se)); s != "" {
				subjects = append(subjects, s)
			}
		case "identifier":
			readIdentifier(t, attrLocal(se, "scheme"), strings.TrimSpace(elemText(dec, &se)))
		case "meta":
			name := attrLocal(se, "name")
			content := attrLocal(se, "content")
			prop := attrLocal(se, "property")
			text := strings.TrimSpace(elemText(dec, &se)) // consumes the element
			switch {
			case name == "calibre:series" && t.Series == "":
				t.Series = content
			case name == "calibre:series_index" && t.SeriesIndex == "":
				t.SeriesIndex = content
			case prop == "belongs-to-collection" && t.Series == "":
				t.Series = text // EPUB3 collection
			case prop == "group-position" && t.SeriesIndex == "":
				t.SeriesIndex = text
			}
		}
	}
	t.Author = strings.Join(creators, ", ")
	t.Genre = strings.Join(subjects, ", ")
	return t
}

// readIdentifier tries a dc:identifier value as an ISBN first (checksum-
// validated), then as an ASIN when the scheme hints Amazon/MOBI.
func readIdentifier(t *Tags, scheme, text string) {
	scheme = strings.ToUpper(scheme)
	if i := strings.LastIndex(strings.ToLower(text), "isbn:"); i >= 0 {
		text = text[i+len("isbn:"):]
	}
	if t.ISBN == "" {
		if v := scanner.NormalizeISBN(text); v != "" {
			t.ISBN = v
		}
	}
	if t.ASIN == "" && (strings.Contains(scheme, "ASIN") || strings.Contains(scheme, "AMAZON") || strings.Contains(scheme, "MOBI")) {
		if v := scanner.ASINFromName(text); v != "" {
			t.ASIN = v
		}
	}
}

// elemText decodes se's text content, consuming through its end tag (so a
// self-closing element yields "" and is still consumed).
func elemText(dec *xml.Decoder, se *xml.StartElement) string {
	var s string
	_ = dec.DecodeElement(&s, se)
	return s
}

func attrLocal(se xml.StartElement, local string) string {
	for _, a := range se.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}
