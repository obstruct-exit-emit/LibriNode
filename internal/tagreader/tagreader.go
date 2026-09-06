// Package tagreader reads an audiobook file's own embedded tags live off disk,
// so the UI can answer "what does this file actually have on it right now" —
// which the scan-time snapshot can't, since nothing re-reads a file after a
// "Write tags" call. It reads through go.senan.xyz/taglib, the same backend the
// tag writer uses, so the freeform atoms it writes (series, ISBN, ASIN) read
// back clean rather than with dhowden/tag's 4-NUL MP4 prefix.
package tagreader

import (
	"time"

	taglib "go.senan.xyz/taglib"
)

// Tags is an audiobook file's embedded tags plus its audio properties. The tag
// fields mirror what tagwriter writes, so the viewer and the writer agree.
type Tags struct {
	Title       string `json:"title"`
	Author      string `json:"author"`       // artist
	AlbumArtist string `json:"albumArtist"`  // album artist
	Album       string `json:"album"`
	Narrator    string `json:"narrator"`     // composer
	Series      string `json:"series"`       // movement name / SERIES
	SeriesPart  string `json:"seriesPart"`   // movement number / SERIES-PART
	Genre       string `json:"genre"`
	Date        string `json:"date"`
	Description string `json:"description"`  // comment / DESCRIPTION
	ISBN        string `json:"isbn"`
	ASIN        string `json:"asin"`
	// Audio properties (best-effort; zero when unreadable).
	Format          string `json:"format"`
	Codec           string `json:"codec,omitempty"`
	DurationSeconds int    `json:"durationSeconds"`
	Bitrate         int    `json:"bitrate"`
	SampleRate      int    `json:"sampleRate"`
	Channels        int    `json:"channels"`
	HasCover        bool   `json:"hasCover"`
}

// Read parses the audio file at path with TagLib and returns its tags.
func Read(path string) (*Tags, error) {
	raw, err := taglib.ReadTags(path)
	if err != nil {
		return nil, err
	}
	t := &Tags{
		Title:       first(raw, taglib.Title),
		Author:      first(raw, taglib.Artist),
		AlbumArtist: first(raw, taglib.AlbumArtist),
		Album:       first(raw, taglib.Album),
		Narrator:    first(raw, taglib.Composer),
		Series:      first(raw, taglib.MovementName, "SERIES"),
		SeriesPart:  first(raw, taglib.MovementNumber, "SERIES-PART"),
		Genre:       first(raw, taglib.Genre),
		Date:        first(raw, taglib.Date),
		Description: first(raw, "DESCRIPTION", taglib.Comment),
		ISBN:        first(raw, "ISBN"),
		ASIN:        first(raw, "ASIN"),
	}
	// Properties are a separate read; a failure there still leaves the tags.
	if props, perr := taglib.ReadProperties(path); perr == nil {
		t.Format = props.Format
		t.Codec = props.InnerCodec
		t.DurationSeconds = int(props.Length / time.Second)
		t.Bitrate = int(props.BitRate)
		t.SampleRate = int(props.SampleRate)
		t.Channels = int(props.Channels)
		t.HasCover = len(props.Images) > 0
	}
	return t, nil
}

// first returns the first non-empty value among the given tag keys — so a field
// written under either of two conventions (movement name vs SERIES) is found.
func first(m map[string][]string, keys ...string) string {
	for _, k := range keys {
		if v := m[k]; len(v) > 0 && v[0] != "" {
			return v[0]
		}
	}
	return ""
}
