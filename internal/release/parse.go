// Package release parses raw indexer release titles into structured data and
// scores them against wanted books. This is the decision engine behind
// interactive search now and automatic grabbing later.
package release

import (
	"regexp"
	"strconv"
	"strings"
)

// Parsed is what a release title tells us, best-effort. Zero values mean
// "not stated".
type Parsed struct {
	Author   string   `json:"author,omitempty"`
	Title    string   `json:"title,omitempty"`
	Year     int      `json:"year,omitempty"`
	Formats  []string `json:"formats,omitempty"` // epub, mobi, azw3, pdf
	Language string   `json:"language,omitempty"`
	Retail   bool     `json:"retail"`
	Group    string   `json:"group,omitempty"`
}

var ebookFormats = map[string]bool{"epub": true, "mobi": true, "azw3": true, "pdf": true}

// languages maps release-title tokens to a normalized language name.
var languages = map[string]string{
	"english": "english", "eng": "english", "en": "english",
	"german": "german", "ger": "german", "deutsch": "german", "de": "german",
	"french": "french", "fre": "french", "fr": "french",
	"spanish": "spanish", "spa": "spanish", "es": "spanish",
	"italian": "italian", "ita": "italian", "it": "italian",
	"dutch": "dutch", "nl": "dutch",
	"russian": "russian", "rus": "russian",
	"portuguese": "portuguese", "por": "portuguese",
	"polish": "polish", "pol": "polish",
	"japanese": "japanese", "jpn": "japanese",
}

var (
	bracketTags = regexp.MustCompile(`[\[({]([^\][(){}]*)[\])}]`)
	yearToken   = regexp.MustCompile(`^(19|20)\d{2}$`)
	sceneGroup  = regexp.MustCompile(`-([A-Za-z0-9][A-Za-z0-9_]{1,15})$`)
	byPattern   = regexp.MustCompile(`(?i)^(.+?)\s+by\s+(.+)$`)
)

// Parse extracts structured info from one release title.
func Parse(title string) Parsed {
	var p Parsed
	working := strings.TrimSpace(title)

	// Scene-style dotted names: many dots, few spaces.
	if strings.Count(working, " ") <= 1 && strings.Count(working, ".") >= 3 {
		if m := sceneGroup.FindStringSubmatch(working); m != nil {
			p.Group = m[1]
			working = strings.TrimSuffix(working, m[0])
		}
		working = strings.ReplaceAll(working, ".", " ")
	}

	// Pull bracketed tags out: they hold formats, language, year, retail.
	working = bracketTags.ReplaceAllStringFunc(working, func(m string) string {
		inner := m[1 : len(m)-1]
		for _, tok := range strings.FieldsFunc(inner, func(r rune) bool {
			return r == ' ' || r == ',' || r == '/' || r == '|' || r == '+'
		}) {
			p.absorbToken(tok)
		}
		return " "
	})

	// Scan remaining words for trailing metadata tokens.
	words := strings.Fields(working)
	kept := make([]string, 0, len(words))
	for _, w := range words {
		if p.absorbToken(strings.Trim(w, ",;")) {
			continue
		}
		kept = append(kept, w)
	}
	working = strings.Join(kept, " ")
	working = strings.TrimSpace(strings.Trim(working, "-–"))

	// Author/title split: "Author - [Series NN -] Title" or "Title by Author".
	if parts := splitDash(working); len(parts) >= 2 {
		p.Author = strings.TrimSpace(parts[0])
		p.Title = strings.TrimSpace(parts[len(parts)-1])
	} else if m := byPattern.FindStringSubmatch(working); m != nil {
		p.Title = strings.TrimSpace(m[1])
		p.Author = strings.TrimSpace(m[2])
	} else {
		p.Title = working
	}
	return p
}

// absorbToken records a metadata token, reporting whether it was one.
func (p *Parsed) absorbToken(tok string) bool {
	t := strings.ToLower(strings.TrimSpace(tok))
	switch {
	case t == "":
		return true
	case ebookFormats[t]:
		for _, f := range p.Formats {
			if f == t {
				return true
			}
		}
		p.Formats = append(p.Formats, t)
		return true
	case t == "retail":
		p.Retail = true
		return true
	case yearToken.MatchString(t):
		if p.Year == 0 {
			p.Year, _ = strconv.Atoi(t)
		}
		return true
	}
	if lang, ok := languages[t]; ok {
		if p.Language == "" {
			p.Language = lang
		}
		return true
	}
	return false
}

func splitDash(s string) []string {
	parts := strings.Split(s, " - ")
	out := parts[:0]
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, part)
		}
	}
	return out
}
