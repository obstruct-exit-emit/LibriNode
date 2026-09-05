package tagwriter

import (
	"fmt"
	"strings"

	taglib "go.senan.xyz/taglib"
)

// writeTagLib builds the tag map and hands it to TagLib. An audiobook author is
// written to both artist and album-artist (what Audiobookshelf reads as the
// authors); the narrator goes to composer (the reader convention tagreader
// already parses back).
func writeTagLib(path string, tags Tags, clear bool, enabled Toggles) error {
	set := map[string][]string{}
	if enabled.Title {
		setFieldIfPresent(set, taglib.Title, tags.Title)
	}
	if enabled.Author {
		setFieldIfPresent(set, taglib.Artist, tags.Author)
		setFieldIfPresent(set, taglib.AlbumArtist, tags.Author)
	}
	if enabled.Album {
		setFieldIfPresent(set, taglib.Album, tags.Album)
	}
	if enabled.Narrator {
		setFieldIfPresent(set, taglib.Composer, tags.Narrator)
	}
	if enabled.Date {
		setFieldIfPresent(set, taglib.Date, tags.Date)
	}
	if enabled.Series {
		// Movement name/number is the MP4 series convention Audiobookshelf and
		// Apple Books read; the plain SERIES/SERIES-PART pair covers ID3/Vorbis
		// players. Write both so a series shelves correctly wherever it lands.
		setFieldIfPresent(set, taglib.MovementName, tags.Series)
		setFieldIfPresent(set, "SERIES", tags.Series)
		setFieldIfPresent(set, taglib.MovementNumber, tags.SeriesIndex)
		setFieldIfPresent(set, "SERIES-PART", tags.SeriesIndex)
	}
	if enabled.Description {
		setFieldIfPresent(set, taglib.Comment, tags.Description)
		setFieldIfPresent(set, "DESCRIPTION", tags.Description)
	}
	if enabled.Identifier {
		setFieldIfPresent(set, "ISBN", tags.ISBN)
		setFieldIfPresent(set, "ASIN", tags.ASIN)
	}

	var opts taglib.WriteOption
	if clear {
		opts = taglib.Clear
	}
	if err := taglib.WriteTags(path, set, opts); err != nil {
		return err
	}

	// Embedded images are their own concept in TagLib, not part of the tag map,
	// so Clear never touches them; writing at the front-cover slot overwrites
	// whatever was there.
	if enabled.CoverImage && len(tags.CoverImage) > 0 {
		if err := taglib.WriteImage(path, tags.CoverImage); err != nil {
			return fmt.Errorf("write cover image: %w", err)
		}
	}
	return nil
}

// setFieldIfPresent adds key only when value is non-blank — LibriNode never
// clears a field just because it has no value of its own; a blank stays out of
// the map so an existing tag survives a merge write (and is stripped only by an
// explicit clear).
func setFieldIfPresent(set map[string][]string, key, value string) {
	if v := strings.TrimSpace(value); v != "" {
		set[key] = []string{v}
	}
}
