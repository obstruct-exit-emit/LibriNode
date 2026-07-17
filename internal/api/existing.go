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
	// Duplicate is set when the file confidently matches a book already OWNED
	// in this format — the user resolves it: replace the library's copy with
	// this file, or delete this file from disk.
	Duplicate *duplicateInfo `json:"duplicate,omitempty"`
}

// duplicateInfo names the owned book an unmatched file duplicates, and the
// file currently holding that spot in the library.
type duplicateInfo struct {
	BookID     int64            `json:"bookId"`
	Title      string           `json:"title"`
	Year       string           `json:"year,omitempty"`
	File       library.BookFile `json:"file"` // the book's current file in this format
	Confidence int              `json:"confidence"`
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
		// Unowned books become candidates; owned books are tracked separately —
		// a confident match against one means this file is a DUPLICATE of a
		// book the library already has.
		var want, dup matchTally
		var dupBook *library.Book
		for j := range books {
			b := &books[j]
			if b.MediaType != "book" {
				continue // volumes/issues import through their series
			}
			owned := b.HasEbookFile
			if mediaType == "audiobook" {
				owned = b.HasAudiobookFile
			}
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
			if owned {
				if dup.consider(hit, cov, isExact) {
					dupBook = b
				}
				continue
			}
			cand := unmatchedCandidate{ID: b.ID, Title: b.Title}
			if len(b.ReleaseDate) >= 4 {
				cand.Year = b.ReleaseDate[:4]
			}
			opt.Candidates = append(opt.Candidates, cand)
			if want.consider(hit, cov, isExact) {
				opt.Suggested = b.ID
			}
		}
		opt.Confident = want.unique()
		opt.Confidence = want.confidence()
		if !opt.Confident {
			opt.Suggested = 0 // ambiguous (or nothing) — the user picks
		}
		// Duplicate flag: a unique confident match against an owned book, with
		// the file currently holding that spot so the user can compare.
		if dup.unique() && dupBook != nil {
			if bookFiles, err := s.store.ListBookFiles(dupBook.ID); err == nil {
				for k := range bookFiles {
					if bookFiles[k].MediaType == mediaType {
						info := &duplicateInfo{
							BookID: dupBook.ID, Title: dupBook.Title,
							File: bookFiles[k], Confidence: dup.confidence(),
						}
						if len(dupBook.ReleaseDate) >= 4 {
							info.Year = dupBook.ReleaseDate[:4]
						}
						opt.Duplicate = info
						break
					}
				}
			}
		}
		out = append(out, opt)
	}
	return out, nil
}

// matchTally accumulates one match race: the longest hit wins, ties are
// remembered (they kill confidence), and the runner-up length feeds the gap
// half of the rating.
type matchTally struct {
	bestLen, secondLen, bestCount int
	bestCov                       float64
	exact                         bool
}

// consider feeds one book's longest hit; true when it takes the lead.
func (t *matchTally) consider(hit int, cov float64, isExact bool) bool {
	switch {
	case hit == 0:
	case hit > t.bestLen:
		t.secondLen = t.bestLen
		t.bestLen, t.bestCov, t.bestCount, t.exact = hit, cov, 1, isExact
		return true
	case hit == t.bestLen:
		t.bestCount++
	default:
		if hit > t.secondLen {
			t.secondLen = hit
		}
	}
	return false
}

// unique reports a single undisputed winner.
func (t *matchTally) unique() bool { return t.bestLen > 0 && t.bestCount == 1 }

// confidence turns the tally into a 0–100 rating: an exact title is certain;
// a unique longest match scores by how much of the filename the title
// explains (coverage) plus how far ahead of the runner-up it is (gap); a tie
// can't be trusted regardless of length.
func (t *matchTally) confidence() int {
	switch {
	case t.bestLen == 0:
		return 0
	case t.bestCount > 1:
		return 40 // something matched, but it's a coin toss
	case t.exact:
		return 100
	default:
		gap := float64(t.bestLen-t.secondLen) / float64(t.bestLen)
		return 55 + int(25*t.bestCov+0.5) + int(20*gap+0.5)
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
