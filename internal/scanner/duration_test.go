package scanner

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// box builds an MP4 atom: 4-byte big-endian size, 4-byte type, payload.
func box(typ string, payload []byte) []byte {
	b := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(b[0:4], uint32(8+len(payload)))
	copy(b[4:8], typ)
	copy(b[8:], payload)
	return b
}

// mp4Fixture is ftyp + moov(mvhd) with the given timescale/duration (v0).
func mp4Fixture(timescale, duration uint32) []byte {
	mvhd := make([]byte, 20) // ver+flags(4), creation+modif(8), timescale(4), duration(4)
	binary.BigEndian.PutUint32(mvhd[12:16], timescale)
	binary.BigEndian.PutUint32(mvhd[16:20], duration)
	ftyp := box("ftyp", []byte("isom\x00\x00\x00\x00"))
	moov := box("moov", box("mvhd", mvhd))
	return append(ftyp, moov...)
}

func TestMP4DurationReadsMVHD(t *testing.T) {
	data := mp4Fixture(1000, 600000) // 600000 / 1000 = 600s
	secs, err := mp4Duration(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if secs != 600 {
		t.Errorf("duration = %v, want 600s", secs)
	}
}

func TestMP3DurationCBREstimate(t *testing.T) {
	// MPEG1 Layer3, 128kbps, 44.1kHz, stereo, no Xing tag (zeros follow).
	buf := append([]byte{0xFF, 0xFB, 0x90, 0x00}, make([]byte, 100)...)
	// A 128000-byte CBR file at 128kbps is exactly 8 seconds.
	secs, err := mp3Duration(bytes.NewReader(buf), 128000)
	if err != nil {
		t.Fatal(err)
	}
	if secs < 7.9 || secs > 8.1 {
		t.Errorf("CBR duration = %v, want ~8s", secs)
	}
}

func TestMP3XingZeroFramesFallsBackToCBR(t *testing.T) {
	// MPEG1 Layer3 128kbps stereo frame, then a Xing tag (offset 32) whose
	// frame-count field is 0 — must not report 0s, but fall back to the CBR
	// estimate (8s for a 128000-byte file at 128kbps).
	buf := []byte{0xFF, 0xFB, 0x90, 0x00}
	buf = append(buf, make([]byte, 32)...)
	buf = append(buf, []byte("Xing")...)
	buf = append(buf, 0, 0, 0, 0x01) // flags: frames field present
	buf = append(buf, 0, 0, 0, 0x00) // frames = 0
	secs, err := mp3Duration(bytes.NewReader(buf), 128000)
	if err != nil {
		t.Fatal(err)
	}
	if secs < 7.9 || secs > 8.1 {
		t.Errorf("zero-frame Xing duration = %v, want CBR fallback ~8s", secs)
	}
}

func TestMP3SkipsBadBitrateSync(t *testing.T) {
	// A false sync (0xFF 0xFB with the "bad" bitrate index 0xF) precedes the
	// real frame; the parser must skip it and estimate from the real one.
	buf := []byte{0xFF, 0xFB, 0xF0, 0x00} // bitrate index 15 = bad
	buf = append(buf, 0xFF, 0xFB, 0x90, 0x00)
	buf = append(buf, make([]byte, 64)...)
	secs, err := mp3Duration(bytes.NewReader(buf), 128000)
	if err != nil {
		t.Fatalf("bad-bitrate sync not skipped: %v", err)
	}
	if secs < 7.9 || secs > 8.1 {
		t.Errorf("duration = %v, want ~8s from the real frame", secs)
	}
}

func TestAudioDurationSumsFolder(t *testing.T) {
	dir := t.TempDir()
	data := mp4Fixture(1000, 600000) // 10 minutes each
	for _, name := range []string{"CD 01 - 01.m4b", "CD 02 - 01.m4b"} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A non-audio sibling must be ignored, not fail the sum.
	if err := os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mins, err := AudioDuration(dir)
	if err != nil {
		t.Fatal(err)
	}
	if mins != 20 {
		t.Errorf("folder duration = %d min, want 20 (two 10-minute files)", mins)
	}
}

func TestAudioDurationSingleFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "book.m4b")
	if err := os.WriteFile(p, mp4Fixture(1000, 630000), 0o644); err != nil { // 10.5 min → 11 rounded
		t.Fatal(err)
	}
	mins, err := AudioDuration(p)
	if err != nil {
		t.Fatal(err)
	}
	if mins != 11 {
		t.Errorf("duration = %d min, want 11 (630s rounds up)", mins)
	}
}
