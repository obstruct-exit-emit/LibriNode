package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/scanner"
)

// Existing-file import: the white-glove path for files a scan found but could
// not confidently match. Each unmatched prose file gets its author's
// bibliography as candidates (excluding books already owned in that format);
// when the filename singles out exactly one candidate the match is confident —
// importable in one click individually, or all at once via import-matched.
// Matching a file enrolls the book in the format's library and monitors it,
// so an adopted book behaves like one added by hand.

// unmatchedCandidate is one of the author's books offered for manual selection.
type unmatchedCandidate struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Year  string `json:"year,omitempty"`
}

// unmatchedOption is an unmatched file plus everything the import flow needs.
type unmatchedOption struct {
	File       library.BookFile     `json:"file"`
	AuthorName string               `json:"authorName,omitempty"`
	AuthorID   int64                `json:"authorId,omitempty"`
	Suggested  int64                `json:"suggested,omitempty"` // candidate id; confident when Confident
	Confident  bool                 `json:"confident"`
	Confidence int                  `json:"confidence"` // 0–100, how sure the suggestion is
	Candidates []unmatchedCandidate `json:"candidates"`
}

// unmatchedOptions builds the import options for every unmatched file of one
// prose media type (ebook/audiobook).
func (s *server) unmatchedOptions(mediaType string) ([]unmatchedOption, error) {
	files, err := s.store.ListUnmatchedBookFiles()
	if err != nil {
		return nil, err
	}
	roots, err := s.store.ListRootFolders()
	if err != nil {
		return nil, err
	}
	rootByID := map[int64]string{}
	for _, r := range roots {
		rootByID[r.ID] = r.Path
	}
	authors, err := s.store.ListAuthors()
	if err != nil {
		return nil, err
	}
	authorByName := map[string]*library.Author{}
	for i := range authors {
		authorByName[scanner.Normalize(authors[i].Name)] = &authors[i]
	}

	out := []unmatchedOption{}
	for i := range files {
		f := files[i]
		if f.MediaType != mediaType {
			continue
		}
		opt := unmatchedOption{File: f, Candidates: []unmatchedCandidate{}}

		rel := f.Path
		if root, ok := rootByID[f.RootFolderID]; ok {
			if r, err := filepath.Rel(root, f.Path); err == nil {
				rel = r
			}
		}
		parsed := scanner.ParsePath(rel)
		opt.AuthorName = parsed.Author

		author := authorByName[scanner.Normalize(parsed.Author)]
		if parsed.Author == "" || author == nil {
			out = append(out, opt) // author unknown — nothing to offer
			continue
		}
		opt.AuthorID = author.ID
		if opt.AuthorName == "" {
			opt.AuthorName = author.Name
		}

		books, err := s.store.ListBooks(author.ID)
		if err != nil {
			return nil, err
		}
		// The filename's normalized text; a candidate whose title appears in
		// it is a hit. The candidate matching the LONGEST title wins — "Dune
		// Messiah retail" contains both "dune messiah" and "dune", and the
		// longer match is the real one. Confident only when exactly one
		// candidate attains the longest match.
		normTitle := scanner.Normalize(parsed.Title)
		normAlt := scanner.Normalize(parsed.AltTitle) // "" when no alt
		// hitIn reports whether a key appears in the parsed title (or its
		// after-the-dash alt) and how much of that text the key explains —
		// the coverage half of the confidence rating.
		hitIn := func(key string) (bool, float64) {
			if normAlt != "" && strings.Contains(normAlt, key) {
				return true, float64(len(key)) / float64(len(normAlt))
			}
			if normTitle != "" && strings.Contains(normTitle, key) {
				return true, float64(len(key)) / float64(len(normTitle))
			}
			return false, 0
		}
		bestLen, secondLen, bestCount := 0, 0, 0
		bestCov := 0.0
		exact := false
		for j := range books {
			b := &books[j]
			if b.MediaType != "book" {
				continue // volumes/issues import through their series
			}
			owned := b.HasEbookFile
			if mediaType == "audiobook" {
				owned = b.HasAudiobookFile
			}
			if owned {
				continue // spec: never offer books already owned in this format
			}
			cand := unmatchedCandidate{ID: b.ID, Title: b.Title}
			if len(b.ReleaseDate) >= 4 {
				cand.Year = b.ReleaseDate[:4]
			}
			opt.Candidates = append(opt.Candidates, cand)
			// This book's longest matching key and its coverage.
			hit := 0
			cov := 0.0
			isExact := false
			for _, key := range scanner.TitleKeys(b.Title) {
				if key == "" || len(key) <= hit {
					continue
				}
				if ok, c := hitIn(key); ok {
					hit, cov = len(key), c
					isExact = key == normTitle || key == normAlt
				}
			}
			switch {
			case hit == 0:
			case hit > bestLen:
				secondLen = bestLen
				bestLen, bestCov, bestCount, exact = hit, cov, 1, isExact
				opt.Suggested = b.ID
			case hit == bestLen:
				bestCount++
			default:
				if hit > secondLen {
					secondLen = hit
				}
			}
		}
		opt.Confident = bestLen > 0 && bestCount == 1
		opt.Confidence = matchConfidence(bestLen, secondLen, bestCount, bestCov, exact)
		if !opt.Confident {
			opt.Suggested = 0 // ambiguous (or nothing) — the user picks
		}
		out = append(out, opt)
	}
	return out, nil
}

// matchConfidence turns the match signals into a 0–100 rating: an exact title
// is certain; a unique longest match scores by how much of the filename the
// title explains (coverage) plus how far ahead of the runner-up it is (gap);
// a tie can't be trusted regardless of length.
func matchConfidence(bestLen, secondLen, bestCount int, coverage float64, exact bool) int {
	switch {
	case bestLen == 0:
		return 0
	case bestCount > 1:
		return 40 // something matched, but it's a coin toss
	case exact:
		return 100
	default:
		gap := float64(bestLen-secondLen) / float64(bestLen)
		return 55 + int(25*coverage+0.5) + int(20*gap+0.5)
	}
}

// handleUnmatchedOptions serves the existing-file import view for a library:
// GET /api/v1/bookfile/unmatched/options?mediaType=ebook|audiobook.
func (s *server) handleUnmatchedOptions(w http.ResponseWriter, r *http.Request) {
	mediaType := r.URL.Query().Get("mediaType")
	if mediaType != "ebook" && mediaType != "audiobook" {
		writeError(w, http.StatusBadRequest, "mediaType must be ebook or audiobook")
		return
	}
	options, err := s.unmatchedOptions(mediaType)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, options)
}

// handleImportMatched imports every confident match in one go:
// POST /api/v1/bookfile/import-matched {"mediaType":"ebook"}. Files without a
// confident match are left for per-file review.
func (s *server) handleImportMatched(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MediaType string `json:"mediaType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		(req.MediaType != "ebook" && req.MediaType != "audiobook") {
		writeError(w, http.StatusBadRequest, "mediaType must be ebook or audiobook")
		return
	}
	options, err := s.unmatchedOptions(req.MediaType)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	imported := 0
	review := 0
	messages := []string{}
	for _, opt := range options {
		if !opt.Confident {
			review++
			continue
		}
		skips, err := s.adoptFile(opt.File.ID, opt.Suggested)
		if err != nil {
			messages = append(messages, filepath.Base(opt.File.Path)+": "+err.Error())
			review++
			continue
		}
		messages = append(messages, skips...)
		imported++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"imported": imported, "needsReview": review, "messages": messages,
	})
}
