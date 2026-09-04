package scanner

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// AudioDuration returns the total playtime, in whole minutes, of an audiobook
// at path — a single file, or the sum of every audio file when path is a folder
// (a multi-file / multi-disc book). It reads only what each container needs
// (MP4 atom headers, an MP3's first frame or Xing header), never the whole
// file, so probing a multi-hour book is cheap. A format it can't parse
// contributes 0 rather than failing the whole book.
func AudioDuration(path string) (int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	var total float64
	if info.IsDir() {
		err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !IsAudioPath(p) {
				return nil
			}
			if secs, derr := fileDuration(p); derr == nil {
				total += secs
			}
			return nil
		})
		if err != nil {
			return 0, err
		}
	} else {
		if total, err = fileDuration(path); err != nil {
			return 0, err
		}
	}
	return int(total/60 + 0.5), nil
}

// fileDuration returns one audio file's length in seconds.
func fileDuration(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m4b", ".m4a", ".mp4", ".aac", ".m4v":
		return mp4Duration(f, info.Size())
	case ".mp3":
		return mp3Duration(f, info.Size())
	case ".flac":
		return flacDuration(f)
	case ".opus":
		return opusDuration(f, info.Size())
	default:
		return 0, errUnsupportedAudio
	}
}

var (
	errUnsupportedAudio = errors.New("scanner: unsupported audio format")
	errNoMVHD           = errors.New("scanner: mp4 has no mvhd")
	errNoMP3Frame       = errors.New("scanner: no mp3 frame found")
	errNoStreamInfo     = errors.New("scanner: flac has no STREAMINFO")
	errNoOggPage        = errors.New("scanner: no ogg page with a granule")
)

// --- MP4 / M4B (moov → mvhd is the exact duration) ---

// mp4Duration walks the atom tree to moov/mvhd and returns duration/timescale.
func mp4Duration(rs io.ReadSeeker, size int64) (float64, error) {
	moovStart, moovEnd, err := findAtom(rs, 0, size, "moov")
	if err != nil {
		return 0, err
	}
	mvhdStart, _, err := findAtom(rs, moovStart, moovEnd, "mvhd")
	if err != nil {
		return 0, err
	}
	if _, err := rs.Seek(mvhdStart, io.SeekStart); err != nil {
		return 0, err
	}
	var head [8]byte // version(1) + flags(3) + creation(4...)
	if _, err := io.ReadFull(rs, head[:4]); err != nil {
		return 0, err
	}
	version := head[0]
	// Skip creation/modification times: 8 bytes (v0) or 16 bytes (v1).
	skip := int64(8)
	if version == 1 {
		skip = 16
	}
	if _, err := rs.Seek(skip, io.SeekCurrent); err != nil {
		return 0, err
	}
	var ts [4]byte
	if _, err := io.ReadFull(rs, ts[:]); err != nil {
		return 0, err
	}
	timescale := binary.BigEndian.Uint32(ts[:])
	if timescale == 0 {
		return 0, errNoMVHD
	}
	var duration uint64
	if version == 1 {
		var d [8]byte
		if _, err := io.ReadFull(rs, d[:]); err != nil {
			return 0, err
		}
		duration = binary.BigEndian.Uint64(d[:])
	} else {
		var d [4]byte
		if _, err := io.ReadFull(rs, d[:]); err != nil {
			return 0, err
		}
		duration = uint64(binary.BigEndian.Uint32(d[:]))
	}
	return float64(duration) / float64(timescale), nil
}

// findAtom scans the atom siblings in [start,end) for one of the given type and
// returns the [payloadStart, payloadEnd) of its contents.
func findAtom(rs io.ReadSeeker, start, end int64, want string) (int64, int64, error) {
	pos := start
	for pos+8 <= end {
		if _, err := rs.Seek(pos, io.SeekStart); err != nil {
			return 0, 0, err
		}
		var hdr [8]byte
		if _, err := io.ReadFull(rs, hdr[:]); err != nil {
			return 0, 0, err
		}
		size := int64(binary.BigEndian.Uint32(hdr[0:4]))
		typ := string(hdr[4:8])
		headerLen := int64(8)
		switch size {
		case 1: // 64-bit largesize follows the type
			var big [8]byte
			if _, err := io.ReadFull(rs, big[:]); err != nil {
				return 0, 0, err
			}
			size = int64(binary.BigEndian.Uint64(big[:]))
			headerLen = 16
		case 0: // extends to the end of the enclosing box
			size = end - pos
		}
		if size < headerLen || pos+size > end {
			return 0, 0, errNoMVHD
		}
		if typ == want {
			return pos + headerLen, pos + size, nil
		}
		pos += size
	}
	return 0, 0, errNoMVHD
}

// --- FLAC (STREAMINFO gives exact total samples / sample rate) ---

// flacDuration reads the mandatory first metadata block (STREAMINFO) and
// returns total_samples / sample_rate.
func flacDuration(rs io.ReadSeeker) (float64, error) {
	var magic [4]byte
	if _, err := io.ReadFull(rs, magic[:]); err != nil {
		return 0, err
	}
	if string(magic[:]) != "fLaC" {
		return 0, errUnsupportedAudio
	}
	// Metadata block header: 1 byte (last-flag<<7 | type), 3-byte length. The
	// first block is always STREAMINFO (type 0).
	var hdr [4]byte
	if _, err := io.ReadFull(rs, hdr[:]); err != nil {
		return 0, err
	}
	if hdr[0]&0x7f != 0 {
		return 0, errNoStreamInfo
	}
	// STREAMINFO: min/max block (4) + min/max frame (6) = 10 bytes, then a
	// 64-bit pack of sample_rate(20) | channels(3) | bits_per_sample(5) |
	// total_samples(36). Only the pack is needed.
	var info [18]byte
	if _, err := io.ReadFull(rs, info[:]); err != nil {
		return 0, err
	}
	packed := binary.BigEndian.Uint64(info[10:18])
	sampleRate := packed >> 44
	totalSamples := packed & ((1 << 36) - 1)
	if sampleRate == 0 {
		return 0, errNoStreamInfo
	}
	return float64(totalSamples) / float64(sampleRate), nil
}

// --- Opus in Ogg (granule position of the last page, at the 48kHz clock) ---

// opusDuration returns the playtime of an Ogg-Opus file: the largest granule
// position across the Ogg pages near the end of the file, divided by 48000
// (Opus always counts granules at 48kHz regardless of input rate). The tiny
// pre-skip (sub-second) is ignored — negligible at minute resolution.
func opusDuration(rs io.ReadSeeker, size int64) (float64, error) {
	const window = 131072 // enough to hold the final page even at max page size
	start := size - window
	if start < 0 {
		start = 0
	}
	if _, err := rs.Seek(start, io.SeekStart); err != nil {
		return 0, err
	}
	buf := make([]byte, size-start)
	if _, err := io.ReadFull(rs, buf); err != nil {
		return 0, err
	}
	var maxGranule uint64
	found := false
	for i := 0; i+14 <= len(buf); i++ {
		if buf[i] == 'O' && buf[i+1] == 'g' && buf[i+2] == 'g' && buf[i+3] == 'S' {
			g := binary.LittleEndian.Uint64(buf[i+6 : i+14])
			// 0xFFFF… means "no packet completes on this page" — not a total.
			if g != ^uint64(0) && (!found || g > maxGranule) {
				maxGranule, found = g, true
			}
		}
	}
	if !found {
		return 0, errNoOggPage
	}
	return float64(maxGranule) / 48000.0, nil
}

// --- MP3 (Xing/Info frame count when present, else a CBR estimate) ---

var mp3BitrateV1L3 = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
var mp3BitrateV2L3 = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
var mp3SampleRate = [4][4]int{
	{11025, 12000, 8000, 0},  // MPEG 2.5
	{0, 0, 0, 0},             // reserved
	{22050, 24000, 16000, 0}, // MPEG 2
	{44100, 48000, 32000, 0}, // MPEG 1
}

func mp3Duration(rs io.ReadSeeker, size int64) (float64, error) {
	// Skip an ID3v2 tag if present (10-byte header + syncsafe size).
	audioStart := int64(0)
	var id3 [10]byte
	if _, err := io.ReadFull(rs, id3[:]); err == nil && string(id3[0:3]) == "ID3" {
		tagSize := int64(id3[6]&0x7f)<<21 | int64(id3[7]&0x7f)<<14 | int64(id3[8]&0x7f)<<7 | int64(id3[9]&0x7f)
		audioStart = 10 + tagSize
	}

	// Read a window and find the first frame sync.
	if _, err := rs.Seek(audioStart, io.SeekStart); err != nil {
		return 0, err
	}
	buf := make([]byte, 4096)
	n, _ := io.ReadFull(rs, buf)
	buf = buf[:n]
	fi := -1
	for i := 0; i+4 <= len(buf); i++ {
		if buf[i] == 0xFF && buf[i+1]&0xE0 == 0xE0 {
			if h, ok := parseMP3Header(buf[i : i+4]); ok {
				fi = i
				_ = h
				break
			}
		}
	}
	if fi < 0 {
		return 0, errNoMP3Frame
	}
	h, _ := parseMP3Header(buf[fi : fi+4])

	// Xing/Info (offset depends on MPEG version + channel mode) carries the
	// exact frame count for VBR files — the accurate path.
	xingOff := fi + 4 + xingOffset(h)
	if xingOff+8 <= len(buf) {
		tag := string(buf[xingOff : xingOff+4])
		if tag == "Xing" || tag == "Info" {
			flags := binary.BigEndian.Uint32(buf[xingOff+4 : xingOff+8])
			if flags&0x1 != 0 && xingOff+12 <= len(buf) { // frames field present
				frames := binary.BigEndian.Uint32(buf[xingOff+8 : xingOff+12])
				// Some encoders write a Xing tag with a zero frame count; fall
				// through to the CBR estimate rather than reporting 0 seconds.
				if frames > 0 {
					return float64(frames) * float64(h.samplesPerFrame) / float64(h.sampleRate), nil
				}
			}
		}
	}

	// CBR estimate: audio bytes at the first frame's bitrate.
	if h.bitrate == 0 {
		return 0, errNoMP3Frame
	}
	audioBytes := size - audioStart
	return float64(audioBytes) * 8 / float64(h.bitrate), nil
}

type mp3Header struct {
	bitrate         int // bits per second
	sampleRate      int
	samplesPerFrame int
	version         int // 3=MPEG1, 2=MPEG2, 0=MPEG2.5
	channelMode     int
}

func parseMP3Header(b []byte) (mp3Header, bool) {
	version := int(b[1] >> 3 & 0x3)
	layer := int(b[1] >> 1 & 0x3)
	if layer != 1 { // 1 == Layer III
		return mp3Header{}, false
	}
	bitrateIdx := int(b[2] >> 4 & 0xF)
	if bitrateIdx == 15 {
		return mp3Header{}, false // "bad" bitrate value — never a real frame
	}
	srIdx := int(b[2] >> 2 & 0x3)
	sr := mp3SampleRate[version][srIdx]
	if sr == 0 {
		return mp3Header{}, false
	}
	var kbps int
	if version == 3 { // MPEG 1
		kbps = mp3BitrateV1L3[bitrateIdx]
	} else { // MPEG 2 / 2.5
		kbps = mp3BitrateV2L3[bitrateIdx]
	}
	spf := 1152
	if version != 3 {
		spf = 576
	}
	return mp3Header{
		bitrate:         kbps * 1000,
		sampleRate:      sr,
		samplesPerFrame: spf,
		version:         version,
		channelMode:     int(b[3] >> 6 & 0x3),
	}, true
}

// xingOffset is the gap between the frame header and the Xing/Info tag: it
// depends on MPEG version and whether the stream is mono.
func xingOffset(h mp3Header) int {
	mono := h.channelMode == 3
	if h.version == 3 { // MPEG 1
		if mono {
			return 17
		}
		return 32
	}
	if mono { // MPEG 2 / 2.5
		return 9
	}
	return 17
}
