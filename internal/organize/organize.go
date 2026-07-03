// Package organize moves matched book files into their template-defined
// locations: <root>/<folder template>/<file template>.<ext>. Plans are
// computed separately from application so the UI can preview before touching
// disk.
package organize

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/librinode/librinode/internal/config"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/naming"
)

type Service struct {
	store *library.Store
	cfg   *config.Config
}

func New(store *library.Store, cfg *config.Config) *Service {
	return &Service{store: store, cfg: cfg}
}

// Move is one planned (or applied) file relocation.
type Move struct {
	FileID    int64  `json:"fileId"`
	BookID    int64  `json:"bookId"`
	BookTitle string `json:"bookTitle"`
	From      string `json:"from"`
	To        string `json:"to"`
}

// Plan computes the moves needed to bring matched files in line with the
// naming templates — for one book (bookID > 0) or the whole library. Files
// already in place produce no move. Problems with individual files (missing
// book, vanished root) are reported as skips, not errors.
func (s *Service) Plan(bookID int64) ([]Move, []string, error) {
	var files []library.BookFile
	var err error
	if bookID > 0 {
		files, err = s.store.ListBookFiles(bookID)
	} else {
		files, err = s.store.ListMatchedBookFiles()
	}
	if err != nil {
		return nil, nil, err
	}

	roots, err := s.store.ListRootFolders()
	if err != nil {
		return nil, nil, err
	}
	rootByID := map[int64]string{}
	for _, r := range roots {
		rootByID[r.ID] = r.Path
	}

	moves := []Move{}
	skips := []string{}
	for _, f := range files {
		target, title, err := s.targetPath(&f, rootByID)
		if err != nil {
			skips = append(skips, fmt.Sprintf("%s: %v", f.Path, err))
			continue
		}
		if sameFile(f.Path, target) {
			continue
		}
		moves = append(moves, Move{
			FileID:    f.ID,
			BookID:    f.BookID,
			BookTitle: title,
			From:      f.Path,
			To:        target,
		})
	}
	return moves, skips, nil
}

// Apply executes planned moves: create the target directory, rename the
// file, record the new path, and sweep now-empty source directories (never
// the root folder itself). Moves whose target already exists are skipped
// (never overwrite).
func (s *Service) Apply(moves []Move) (applied []Move, skips []string, err error) {
	roots, err := s.store.ListRootFolders()
	if err != nil {
		return nil, nil, err
	}

	applied = []Move{}
	skips = []string{}
	for _, m := range moves {
		if _, err := os.Stat(m.To); err == nil {
			skips = append(skips, fmt.Sprintf("%s: target already exists", m.To))
			continue
		}
		if err := os.MkdirAll(filepath.Dir(m.To), 0o755); err != nil {
			skips = append(skips, fmt.Sprintf("%s: %v", m.To, err))
			continue
		}
		if err := os.Rename(m.From, m.To); err != nil {
			skips = append(skips, fmt.Sprintf("%s: %v", m.From, err))
			continue
		}
		if err := s.store.SetBookFilePath(m.FileID, m.To); err != nil {
			return applied, skips, fmt.Errorf("recording move of %s: %w", m.From, err)
		}
		for _, r := range roots {
			if strings.HasPrefix(m.From, r.Path+string(filepath.Separator)) {
				sweepEmptyDirs(filepath.Dir(m.From), r.Path)
				break
			}
		}
		applied = append(applied, m)
	}
	if len(applied) > 0 {
		slog.Info("organized files", "moved", len(applied), "skipped", len(skips))
	}
	return applied, skips, nil
}

// targetPath renders <root>/<folder>/<file>.<ext> for one file.
func (s *Service) targetPath(f *library.BookFile, rootByID map[int64]string) (string, string, error) {
	root, ok := rootByID[f.RootFolderID]
	if !ok {
		return "", "", fmt.Errorf("root folder %d no longer exists", f.RootFolderID)
	}
	book, err := s.store.GetBook(f.BookID)
	if err != nil {
		return "", "", fmt.Errorf("book %d: %w", f.BookID, err)
	}
	target, err := s.renderPath(root, book, f.Format)
	if err != nil {
		return "", "", err
	}
	return target, book.Title, nil
}

// renderPath renders <root>/<folder template>/<file template>.<ext> for a book.
func (s *Service) renderPath(root string, book *library.Book, format string) (string, error) {
	author, err := s.store.GetAuthor(book.AuthorID)
	if err != nil {
		return "", fmt.Errorf("author %d: %w", book.AuthorID, err)
	}
	series, err := s.store.ListSeriesForBook(book.ID)
	if err != nil {
		return "", err
	}

	data := naming.TokenData{
		AuthorName:     author.Name,
		AuthorSortName: author.SortName,
		BookTitle:      book.Title,
	}
	if len(series) > 0 {
		data.SeriesTitle = series[0].Title
		data.SeriesPosition = series[0].Position
	}
	if len(book.ReleaseDate) >= 4 {
		data.ReleaseYear = book.ReleaseDate[:4]
	}

	ns := s.cfg.NamingSettings()
	folder := naming.Format(ns.EbookFolder, data)
	file := naming.Format(ns.EbookFile, data)
	return filepath.Join(root, folder, file+"."+format), nil
}

// PlaceFile computes where a newly imported file for book belongs: the first
// ebook root folder plus the naming templates. Returns the root folder id
// and the absolute target path.
func (s *Service) PlaceFile(book *library.Book, format string) (int64, string, error) {
	roots, err := s.store.ListRootFolders()
	if err != nil {
		return 0, "", err
	}
	for _, root := range roots {
		if root.MediaType != "ebook" {
			continue
		}
		target, err := s.renderPath(root.Path, book, format)
		if err != nil {
			return 0, "", err
		}
		return root.ID, target, nil
	}
	return 0, "", fmt.Errorf("no ebook root folder configured")
}

// sameFile compares paths the way the target filesystem does (Windows and
// macOS are usually case-insensitive).
func sameFile(a, b string) bool {
	if a == b {
		return true
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// sweepEmptyDirs removes dir and its now-empty parents, stopping at (and
// never removing) the root folder. os.Remove fails on non-empty directories,
// which is the other loop exit — not a problem to report.
func sweepEmptyDirs(dir, root string) {
	root = filepath.Clean(root)
	for {
		dir = filepath.Clean(dir)
		if dir == root || !strings.HasPrefix(dir, root+string(filepath.Separator)) {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
