package scanner

import (
	"regexp"

	"github.com/librinode/librinode/internal/comicinfo"
)

// colorizedKeywords matches a release/filename announcing itself as a
// deliberately colorized digital edition — the convention scanlation and
// digital-release groups use when a volume has been recolored from its
// original black-and-white printing. There is no reliable mono-side keyword
// to match against: a plain, unlabeled release is the overwhelming majority
// case and simply doesn't say anything about color at all, so its absence is
// never treated as proof of monochrome (see DetectVariant).
var colorizedKeywords = regexp.MustCompile(`(?i)\b(colou?r(?:ized|ed)|colou?r(?:ed)?[\s._-]?edition|digital(?:ly)?[\s._-]?colou?r(?:ed)?)\b`)

// DetectVariant figures out whether a manga/comic archive is a colorized or
// monochrome edition, trying the most reliable signal first:
//
//  1. The embedded ComicInfo.xml's BlackAndWhite field — the standard
//     Kavita/Komga-recognized signal, set by many "proper" digital releases
//     and a deliberate choice by whoever produced it, not a guess.
//  2. A colorized keyword in the archive's own filename or containing
//     folder — the convention scanlation/digital groups use to flag a
//     recolored edition.
//
// Returns "color", "mono", or "" when neither signal says anything definite.
// "" is NOT a claim the file is monochrome — just that nothing detected a
// color edition — because the keyword fallback can only ever assert "color"
// (a plain release that says nothing is the normal case, not evidence of
// anything). Callers fall back to their own default (typically the
// configured root folder's variant) when this returns "".
func DetectVariant(path string) string {
	if info, err := comicinfo.Read(path); err == nil && info != nil {
		switch info.BlackAndWhite {
		case comicinfo.BlackAndWhiteYes:
			return "mono"
		case comicinfo.BlackAndWhiteNo:
			return "color"
		}
	}
	if colorizedKeywords.MatchString(path) {
		return "color"
	}
	return ""
}

// LooksColorized reports whether s (a release title, a filename, a folder
// name — anything) carries a colorized-edition keyword. Exported so a
// release's own title can be checked the same way a scanned/downloaded
// file's path already is — release.ScoreVolume uses this at search time,
// before there's a file to open. Same asymmetry as DetectVariant: a true
// result is a confident, deliberate signal; false proves nothing about
// monochrome, since a plain release simply doesn't say anything about color
// either way.
func LooksColorized(s string) bool {
	return colorizedKeywords.MatchString(s)
}
