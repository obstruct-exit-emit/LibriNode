package ebookmeta

import (
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/librinode/librinode/internal/scanner"
)

// readPDF pulls title/author (and an ISBN if it's sitting in Keywords) out of a
// PDF's document-information dictionary. Best-effort by design: it scans the
// raw bytes for the /Title, /Author, /Keywords entries rather than parsing the
// xref/object graph, so it reads the common case (an uncompressed Info dict)
// and returns empty for PDFs that hide metadata in compressed object streams.
func readPDF(path string) (*Tags, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64<<20))
	if err != nil {
		return &Tags{}, nil
	}
	t := &Tags{
		Title:  pdfField(data, "Title"),
		Author: pdfField(data, "Author"),
	}
	if kw := pdfField(data, "Keywords"); kw != "" {
		if isbn := scanner.NormalizeISBN(kw); isbn != "" {
			t.ISBN = isbn
		}
	}
	return t, nil
}

// pdfField finds the first /<key> ( … ) or /<key> < … > Info-dict value.
func pdfField(data []byte, key string) string {
	re := regexp.MustCompile(`/` + key + `\s*(\((?:[^\\()]|\\.)*\)|<[0-9A-Fa-f\s]*>)`)
	m := re.FindSubmatch(data)
	if m == nil {
		return ""
	}
	return decodePDFString(string(m[1]))
}

// decodePDFString decodes a PDF literal "(...)" or hex "<...>" string. A hex
// string starting with the UTF-16 BOM (FEFF) is decoded as UTF-16BE.
func decodePDFString(s string) string {
	if strings.HasPrefix(s, "<") {
		h := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
				return -1
			}
			return r
		}, strings.Trim(s, "<>"))
		if len(h)%2 == 1 {
			h += "0"
		}
		b, err := hex.DecodeString(h)
		if err != nil {
			return ""
		}
		if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
			u := make([]uint16, 0, (len(b)-2)/2)
			for i := 2; i+1 < len(b); i += 2 {
				u = append(u, binary.BigEndian.Uint16(b[i:i+2]))
			}
			return strings.TrimSpace(string(utf16.Decode(u)))
		}
		return strings.TrimSpace(string(b))
	}
	// Literal string: strip the outer parens and unescape.
	s = strings.TrimSuffix(strings.TrimPrefix(s, "("), ")")
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			default:
				sb.WriteByte(s[i])
			}
			continue
		}
		sb.WriteByte(s[i])
	}
	return strings.TrimSpace(sb.String())
}
