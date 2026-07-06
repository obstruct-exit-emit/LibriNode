package release

import (
	"fmt"
	"sort"
	"strings"

	"github.com/librinode/librinode/internal/indexer"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/scanner"
)

// Preferences drive scoring. Each media type's default quality profile
// produces these (PreferencesFor); the Default*Preferences constructors are
// the built-in fallbacks when no profile exists.
type Preferences struct {
	// FormatScores ranks acceptable formats; formats absent from the map
	// are rejected.
	FormatScores map[string]int
	RetailBonus  int
	// Language "" accepts anything; otherwise releases stating a different
	// language are rejected (unstated passes).
	Language string
	MinSize  int64
	MaxSize  int64
	// MinFormatScore, when > 0, rejects formats scoring at or below it —
	// used by upgrade searches so only genuinely better formats approve.
	MinFormatScore int
	// RejectAbridged drops releases that state they are abridged
	// (audiobook profiles default to true).
	RejectAbridged bool
}

// PreferencesFor resolves the active scoring rules for a media type: its
// default quality profile when one exists, built-in defaults otherwise.
func PreferencesFor(store *library.Store, mediaType string) Preferences {
	if p, err := store.DefaultProfile(mediaType); err == nil {
		prefs := PreferencesFromProfile(*p)
		prefs.RejectAbridged = mediaType == "audiobook"
		return prefs
	}
	switch mediaType {
	case "audiobook":
		return DefaultAudiobookPreferences()
	case "manga":
		return DefaultMangaPreferences()
	case "comic":
		return DefaultComicPreferences()
	case "magazine":
		return DefaultMagazinePreferences()
	}
	return DefaultEbookPreferences()
}

// DefaultMagazinePreferences: periodicals ship as pdf first.
func DefaultMagazinePreferences() Preferences {
	return Preferences{
		FormatScores: map[string]int{"pdf": 100, "epub": 70, "cbz": 50},
		Language:     "english",
		MinSize:      1 << 20,
		MaxSize:      1 << 30,
	}
}

// DefaultMangaPreferences prefer lossless archives; scanlation sizes run
// from a few MB per volume to a GB+ for omnibus scans.
func DefaultMangaPreferences() Preferences {
	return Preferences{
		FormatScores: map[string]int{"cbz": 100, "cbr": 80, "epub": 60, "pdf": 40},
		Language:     "english",
		MinSize:      2 << 20,
		MaxSize:      2 << 30,
	}
}

// DefaultComicPreferences mirror manga but drop epub (comics don't ship it).
func DefaultComicPreferences() Preferences {
	return Preferences{
		FormatScores: map[string]int{"cbz": 100, "cbr": 80, "pdf": 40},
		Language:     "english",
		MinSize:      2 << 20,
		MaxSize:      2 << 30,
	}
}

// DefaultAudiobookPreferences prefers single-file m4b (Audiobookshelf's
// favorite), then space-efficient formats; abridged versions are rejected.
func DefaultAudiobookPreferences() Preferences {
	return Preferences{
		FormatScores:   map[string]int{"m4b": 100, "m4a": 85, "opus": 75, "mp3": 70, "flac": 55},
		RetailBonus:    10,
		Language:       "english",
		MinSize:        5 << 20, // 5 MiB — shorter than a short story
		MaxSize:        4 << 30, // 4 GiB — beyond even long unabridged epics
		RejectAbridged: true,
	}
}

func DefaultEbookPreferences() Preferences {
	return Preferences{
		FormatScores: map[string]int{"epub": 100, "azw3": 80, "mobi": 60, "pdf": 30},
		RetailBonus:  25,
		Language:     "english",
		MinSize:      20 << 10,  // 20 KiB — anything smaller isn't a book
		MaxSize:      500 << 20, // 500 MiB — anything bigger isn't an ebook
	}
}

// PreferencesFromProfile converts a quality profile into scoring
// preferences. Format scores derive from list order: best 100, then
// descending in steps of 20 (floored at 20).
func PreferencesFromProfile(p library.QualityProfile) Preferences {
	prefs := Preferences{
		FormatScores: make(map[string]int, len(p.Formats)),
		RetailBonus:  p.RetailBonus,
		Language:     p.Language,
		MinSize:      p.MinSize,
		MaxSize:      p.MaxSize,
	}
	for i, f := range p.Formats {
		score := 100 - 20*i
		if score < 20 {
			score = 20
		}
		prefs.FormatScores[f] = score
	}
	return prefs
}

// Candidate is a release with its parse, score, and verdict. Release fields
// stay flat in JSON via embedding.
type Candidate struct {
	indexer.Release
	Parsed     Parsed   `json:"parsed"`
	Score      int      `json:"score"`
	Approved   bool     `json:"approved"`
	Rejections []string `json:"rejections,omitempty"`
}

// Score evaluates one release. book and author are optional: without them
// only generic checks run (format, size, health); with them the release must
// actually be the wanted book.
func Score(rel indexer.Release, prefs Preferences, book *library.Book, author *library.Author) Candidate {
	c := Candidate{Release: rel, Parsed: Parse(rel.Title)}

	// Format: best recognized format wins; none recognized is fatal.
	best := -1
	for _, f := range c.Parsed.Formats {
		if s, ok := prefs.FormatScores[f]; ok && s > best {
			best = s
		}
	}
	switch {
	case len(c.Parsed.Formats) == 0:
		c.reject("no recognized ebook format in release name")
	case best < 0:
		c.reject(fmt.Sprintf("format %s not wanted", strings.Join(c.Parsed.Formats, "/")))
	case prefs.MinFormatScore > 0 && best <= prefs.MinFormatScore:
		c.reject("not an upgrade over the owned format")
	default:
		c.Score += best
	}

	if c.Parsed.Retail {
		c.Score += prefs.RetailBonus
	}

	if prefs.Language != "" && c.Parsed.Language != "" && c.Parsed.Language != prefs.Language {
		c.reject("language " + c.Parsed.Language + " not wanted")
	}

	if prefs.RejectAbridged && c.Parsed.Abridged {
		c.reject("abridged")
	}

	if rel.Size > 0 {
		if rel.Size < prefs.MinSize {
			c.reject("suspiciously small file")
		}
		if rel.Size > prefs.MaxSize {
			c.reject("too large for an ebook")
		}
	}

	// Protocol health: dead torrents are useless; live ones get a bounded
	// seeder bonus, usenet a flat availability bonus.
	if rel.Protocol == indexer.ProtocolTorrent {
		if rel.Seeders == 0 {
			c.reject("no seeders")
		} else if rel.Seeders > 0 {
			c.Score += min(rel.Seeders, 20)
		}
	} else {
		c.Score += 10
	}

	if book != nil {
		c.matchBook(book, author)
	}

	c.Approved = len(c.Rejections) == 0
	return c
}

// ScoreVolume evaluates a release against a wanted manga volume / comic
// issue: generic checks plus the series title and the exact volume number.
func ScoreVolume(rel indexer.Release, prefs Preferences, seriesTitle string, number float64) Candidate {
	c := Score(rel, prefs, nil, nil)

	relNorm := scanner.Normalize(rel.Title)
	matched := false
	for _, key := range scanner.TitleKeys(seriesTitle) {
		if key != "" && strings.Contains(relNorm, key) {
			matched = true
			break
		}
	}
	if !matched {
		c.reject("does not contain the series title")
	}

	switch {
	case c.Parsed.Volume == 0:
		c.reject("no volume/issue number in release name")
	case c.Parsed.Volume != number:
		c.reject(fmt.Sprintf("volume %v, wanted %v", c.Parsed.Volume, number))
	}

	c.Approved = len(c.Rejections) == 0
	return c
}

// ScoreMagazine evaluates a release for a magazine: generic checks, the
// magazine's title, and an issue identifier (date or number) that isn't
// already owned. The identifier is returned so the caller can materialize
// the issue on grab.
func ScoreMagazine(rel indexer.Release, prefs Preferences, title string, owned map[string]bool) (Candidate, string) {
	c := Score(rel, prefs, nil, nil)

	relNorm := scanner.Normalize(rel.Title)
	matched := false
	for _, key := range scanner.TitleKeys(title) {
		if key != "" && strings.Contains(relNorm, key) {
			matched = true
			break
		}
	}
	if !matched {
		c.reject("does not contain the magazine title")
	}

	identifier := scanner.IssueIdentifier(rel.Title)
	if identifier == "" {
		c.reject("no issue date or number in release name")
	} else if owned[identifier] {
		c.reject("issue " + identifier + " already owned")
	}

	c.Approved = len(c.Rejections) == 0
	return c, identifier
}

// matchBook rejects releases that don't look like the wanted book.
func (c *Candidate) matchBook(book *library.Book, author *library.Author) {
	relNorm := scanner.Normalize(c.Release.Title)

	matched := false
	for _, key := range scanner.TitleKeys(book.Title) {
		if key != "" && strings.Contains(relNorm, key) {
			matched = true
			break
		}
	}
	if !matched {
		c.reject("does not contain the book title")
	}

	if author != nil {
		authorNorm := scanner.Normalize(author.Name)
		if authorNorm != "" && !strings.Contains(relNorm, authorNorm) {
			c.reject("does not mention the author")
		}
	}

	// A stated year far from the book's original release is suspicious but
	// not fatal (editions differ): small penalty.
	if c.Parsed.Year > 0 && len(book.ReleaseDate) >= 4 {
		if bookYear, err := parseYear(book.ReleaseDate[:4]); err == nil {
			diff := c.Parsed.Year - bookYear
			if diff < 0 {
				diff = -diff
			}
			if diff > 1 {
				c.Score -= 5
			}
		}
	}
}

func parseYear(s string) (int, error) {
	var y int
	_, err := fmt.Sscanf(s, "%d", &y)
	return y, err
}

func (c *Candidate) reject(reason string) {
	c.Rejections = append(c.Rejections, reason)
}

// Rank sorts candidates in place: approved before rejected, then by score.
func Rank(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Approved != candidates[j].Approved {
			return candidates[i].Approved
		}
		return candidates[i].Score > candidates[j].Score
	})
}
