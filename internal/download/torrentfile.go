package download

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strconv"
)

// torrentInfoHash computes a .torrent's v1 info hash: the SHA-1 of the bencoded
// "info" dictionary's exact bytes. This is the hash every client (qBittorrent
// included) identifies the torrent by, independent of whatever name our add
// request asks for — so tracking a grab by it is exact where a title lookup is
// a guess a tracker or debrid bridge can silently defeat by renaming. Mirrors
// how Sonarr/Radarr key a torrent add. Returns "" (with an error) for anything
// that isn't a well-formed .torrent.
func torrentInfoHash(data []byte) (string, error) {
	p := &benParser{data: data}
	start, end, err := p.dictValueSpan("info")
	if err != nil {
		return "", err
	}
	sum := sha1.Sum(data[start:end])
	return hex.EncodeToString(sum[:]), nil
}

var errBencode = errors.New("malformed bencode")

// benParser is a minimal bencode reader — just enough to locate a top-level
// dictionary value's raw byte span. It never allocates a decoded tree.
type benParser struct {
	data []byte
	pos  int
}

// dictValueSpan parses the dictionary at the current position and returns the
// [start,end) byte span of the value for wantKey.
func (p *benParser) dictValueSpan(wantKey string) (int, int, error) {
	if p.pos >= len(p.data) || p.data[p.pos] != 'd' {
		return 0, 0, errBencode
	}
	p.pos++ // consume 'd'
	for p.pos < len(p.data) && p.data[p.pos] != 'e' {
		key, err := p.readString()
		if err != nil {
			return 0, 0, err
		}
		valStart := p.pos
		if err := p.skipValue(); err != nil {
			return 0, 0, err
		}
		if key == wantKey {
			return valStart, p.pos, nil
		}
	}
	return 0, 0, errors.New("bencode: key not found: " + wantKey)
}

// readString reads a bencoded byte string ("<len>:<bytes>").
func (p *benParser) readString() (string, error) {
	start := p.pos
	for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == start || p.pos >= len(p.data) || p.data[p.pos] != ':' {
		return "", errBencode
	}
	n, err := strconv.Atoi(string(p.data[start:p.pos]))
	if err != nil || n < 0 {
		return "", errBencode
	}
	p.pos++ // consume ':'
	if p.pos+n > len(p.data) {
		return "", errBencode
	}
	s := string(p.data[p.pos : p.pos+n])
	p.pos += n
	return s, nil
}

// skipValue advances past one bencoded value (integer, string, list, or dict).
func (p *benParser) skipValue() error {
	if p.pos >= len(p.data) {
		return errBencode
	}
	switch c := p.data[p.pos]; {
	case c == 'i': // i<digits>e
		p.pos++
		for p.pos < len(p.data) && p.data[p.pos] != 'e' {
			p.pos++
		}
		if p.pos >= len(p.data) {
			return errBencode
		}
		p.pos++ // consume 'e'
		return nil
	case c == 'l' || c == 'd': // list or dict, both 'e'-terminated
		isDict := c == 'd'
		p.pos++
		for p.pos < len(p.data) && p.data[p.pos] != 'e' {
			if isDict { // dict entries are key(string)/value pairs
				if _, err := p.readString(); err != nil {
					return err
				}
			}
			if err := p.skipValue(); err != nil {
				return err
			}
		}
		if p.pos >= len(p.data) {
			return errBencode
		}
		p.pos++ // consume 'e'
		return nil
	case c >= '0' && c <= '9': // byte string
		_, err := p.readString()
		return err
	default:
		return errBencode
	}
}
