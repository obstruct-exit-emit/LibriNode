// Package scanner walks ebook root folders, matches the files it finds
// against library books by parsed author/title, and reconciles the
// book_files table — giving the library its "owned vs. wanted" signal.
package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/librinode/librinode/internal/library"
)

type Service struct {
	store *library.Store
}

func New(store *library.Store) *Service {
	return &Service{store: store}
}

// Result summarizes one scan run.
type Result struct {
	Roots     int      `json:"roots"`
	Scanned   int      `json:"scanned"`
	Matched   int      `json:"matched"`
	Unmatched int      `json:"unmatched"`
	Removed   int      `json:"removed"`
	Errors    []string `json:"errors,omitempty"`
}

// Scan walks every ebook root folder. Roots that fail (missing drive, ...)
// are reported in Result.Errors without aborting the others.
func (s *Service) Scan(ctx context.Context) (*Result, error) {
	roots, err := s.store.ListRootFolders()
	if err != nil {
		return nil, err
	}
	index, err := s.buildIndex()
	if err != nil {
		return nil, err
	}

	result := &Result{Errors: []string{}}
	for _, root := range roots {
		if root.MediaType != "ebook" {
			continue // other media types get their own scanners in later phases
		}
		result.Roots++
		if err := s.scanRoot(ctx, root, index, result); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", root.Path, err))
		}
	}
	slog.Info("library scan complete",
		"roots", result.Roots, "scanned", result.Scanned,
		"matched", result.Matched, "unmatched", result.Unmatched,
		"removed", result.Removed, "errors", len(result.Errors))
	return result, nil
}

func (s *Service) scanRoot(ctx context.Context, root library.RootFolder, index *matchIndex, result *Result) error {
	known, err := s.store.BookFilePathsUnderRoot(root.ID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}

	err = filepath.WalkDir(root.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != root.Path {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !ebookExtensions[ext] {
			return nil
		}

		rel, err := filepath.Rel(root.Path, path)
		if err != nil {
			return err
		}
		parsed := ParsePath(rel)
		bookID := index.match(parsed)

		file := &library.BookFile{
			RootFolderID: root.ID,
			BookID:       bookID,
			Path:         path,
			Format:       strings.TrimPrefix(ext, "."),
		}
		if info, err := d.Info(); err == nil {
			file.Size = info.Size()
			file.ModifiedAt = info.ModTime().UTC().Format(time.RFC3339)
		}
		if err := s.store.UpsertBookFile(file); err != nil {
			return err
		}

		seen[path] = true
		result.Scanned++
		if bookID > 0 {
			result.Matched++
		} else {
			result.Unmatched++
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Prune records whose files are gone from disk.
	for path, id := range known {
		if seen[path] {
			continue
		}
		if err := s.store.DeleteBookFile(id); err != nil && err != library.ErrNotFound {
			return err
		}
		result.Removed++
	}
	return nil
}

// matchIndex holds normalized lookups over the whole library, built once per
// scan run.
type matchIndex struct {
	authorsByName map[string]int64           // Normalize(author name) → author id
	byAuthorTitle map[int64]map[string]int64 // author id → title key → book id
	byTitle       map[string]map[int64]bool  // title key → set of book ids
}

func (s *Service) buildIndex() (*matchIndex, error) {
	idx := &matchIndex{
		authorsByName: map[string]int64{},
		byAuthorTitle: map[int64]map[string]int64{},
		byTitle:       map[string]map[int64]bool{},
	}

	authors, err := s.store.ListAuthors()
	if err != nil {
		return nil, err
	}
	for _, a := range authors {
		idx.authorsByName[Normalize(a.Name)] = a.ID
	}

	books, err := s.store.ListBooks(0)
	if err != nil {
		return nil, err
	}
	for _, b := range books {
		if idx.byAuthorTitle[b.AuthorID] == nil {
			idx.byAuthorTitle[b.AuthorID] = map[string]int64{}
		}
		for _, key := range TitleKeys(b.Title) {
			if key == "" {
				continue
			}
			idx.byAuthorTitle[b.AuthorID][key] = b.ID
			if idx.byTitle[key] == nil {
				idx.byTitle[key] = map[int64]bool{}
			}
			idx.byTitle[key][b.ID] = true
		}
	}
	return idx, nil
}

// match resolves a parsed file to a book id, or 0 when there is no confident
// match. Author+title wins; a title-only match is accepted only when it is
// unambiguous across the whole library.
func (idx *matchIndex) match(p ParsedFile) int64 {
	if p.Title == "" {
		return 0
	}
	keys := TitleKeys(p.Title)

	if p.Author != "" {
		if authorID, ok := idx.authorsByName[Normalize(p.Author)]; ok {
			for _, key := range keys {
				if bookID, ok := idx.byAuthorTitle[authorID][key]; ok {
					return bookID
				}
			}
		}
	}

	for _, key := range keys {
		if candidates := idx.byTitle[key]; len(candidates) == 1 {
			for bookID := range candidates {
				return bookID
			}
		}
	}
	return 0
}
