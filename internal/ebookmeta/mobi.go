package ebookmeta

import (
	"encoding/binary"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/librinode/librinode/internal/scanner"
)

// readMOBI reads title/author/identifiers from a MOBI or AZW3 file's EXTH
// header. Both share the Palm-database + MOBI-header layout: record 0 holds a
// PalmDOC header (16 bytes), the MOBI header, and — when its EXTH flag is set —
// an EXTH block of typed records. Best-effort: any malformed structure yields
// whatever was read so far (often just the database name as the title).
func readMOBI(path string) (*Tags, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	hdr := make([]byte, 78)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return &Tags{}, nil
	}
	numRecords := binary.BigEndian.Uint16(hdr[76:78])
	if numRecords == 0 {
		return &Tags{}, nil
	}
	ri := make([]byte, int(numRecords)*8)
	if _, err := io.ReadFull(f, ri); err != nil {
		return &Tags{}, nil
	}
	rec0Off := binary.BigEndian.Uint32(ri[0:4])
	rec0End := uint32(0)
	if numRecords >= 2 {
		rec0End = binary.BigEndian.Uint32(ri[8:12])
	} else if fi, err := f.Stat(); err == nil {
		rec0End = uint32(fi.Size())
	}
	if rec0End <= rec0Off {
		return &Tags{}, nil
	}

	rec0 := make([]byte, rec0End-rec0Off)
	if _, err := f.ReadAt(rec0, int64(rec0Off)); err != nil {
		return &Tags{}, nil
	}

	t := &Tags{Title: decodeMOBI(hdr[0:32], 0)} // PDB name is the title fallback
	if len(rec0) < 0x84 || string(rec0[16:20]) != "MOBI" {
		return t, nil
	}
	mobiHdrLen := binary.BigEndian.Uint32(rec0[20:24])
	enc := binary.BigEndian.Uint32(rec0[28:32])
	fullNameOff := binary.BigEndian.Uint32(rec0[0x54:0x58])
	fullNameLen := binary.BigEndian.Uint32(rec0[0x58:0x5C])
	exthFlags := binary.BigEndian.Uint32(rec0[0x80:0x84])

	if fullNameLen > 0 && fullNameOff+fullNameLen <= uint32(len(rec0)) {
		if name := decodeMOBI(rec0[fullNameOff:fullNameOff+fullNameLen], enc); name != "" {
			t.Title = name
		}
	}
	if exthFlags&0x40 != 0 {
		parseEXTH(rec0, int(16+mobiHdrLen), enc, t)
	}

	if v := scanner.NormalizeISBN(t.ISBN); v != "" {
		t.ISBN = v
	} else {
		t.ISBN = ""
	}
	if v := scanner.ASINFromName(t.ASIN); v != "" {
		t.ASIN = v
	} else {
		t.ASIN = ""
	}
	return t, nil
}

// parseEXTH walks the EXTH records and fills the tags it recognizes.
func parseEXTH(rec0 []byte, start int, enc uint32, t *Tags) {
	if start+12 > len(rec0) || string(rec0[start:start+4]) != "EXTH" {
		return
	}
	count := binary.BigEndian.Uint32(rec0[start+8 : start+12])
	p := start + 12
	for i := uint32(0); i < count; i++ {
		if p+8 > len(rec0) {
			break
		}
		typ := binary.BigEndian.Uint32(rec0[p : p+4])
		length := int(binary.BigEndian.Uint32(rec0[p+4 : p+8]))
		if length < 8 || p+length > len(rec0) {
			break
		}
		val := decodeMOBI(rec0[p+8:p+length], enc)
		switch typ {
		case 100: // author
			t.Author = appendCSV(t.Author, val)
		case 101: // publisher
			if t.Publisher == "" {
				t.Publisher = val
			}
		case 103: // description
			if t.Description == "" {
				t.Description = val
			}
		case 104: // isbn
			if t.ISBN == "" {
				t.ISBN = val
			}
		case 105: // subject
			t.Genre = appendCSV(t.Genre, val)
		case 106: // publishing date
			if t.Date == "" {
				t.Date = val
			}
		case 113, 504: // ASIN
			if t.ASIN == "" {
				t.ASIN = val
			}
		case 503: // updated title
			if val != "" {
				t.Title = val
			}
		case 524: // language
			if t.Language == "" {
				t.Language = val
			}
		}
		p += length
	}
}

// decodeMOBI turns EXTH/name bytes into a trimmed string, honoring the MOBI
// text encoding (65001 UTF-8, 1252 Windows-1252 as latin1 best-effort).
func decodeMOBI(b []byte, enc uint32) string {
	s := strings.TrimRight(string(b), "\x00")
	if enc == 1252 && !utf8.ValidString(s) {
		var sb strings.Builder
		for _, c := range b {
			if c != 0 {
				sb.WriteRune(rune(c))
			}
		}
		s = sb.String()
	}
	return strings.TrimSpace(s)
}

func appendCSV(existing, v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return existing
	}
	if existing == "" {
		return v
	}
	return existing + ", " + v
}
