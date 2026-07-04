// Package scanner walks ebook root folders, matches the files it finds
// against library books by parsed author/title, and reconciles the
// book_files table — giving the library its "owned vs. wanted" signal.
package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
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
		var scanErr error
		switch root.MediaType {
		case "ebook":
			result.Roots++
			scanErr = s.scanRoot(ctx, root, index, result)
		case "audiobook":
			result.Roots++
			scanErr = s.scanAudiobookRoot(ctx, root, index, result)
		case "manga", "comic":
			result.Roots++
			scanErr = s.scanComicRoot(ctx, root, index, result)
		default:
			continue
		}
		if scanErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", root.Path, scanErr))
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
			MediaType:    "ebook",
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

// scanAudiobookRoot walks an audiobook root where the unit is either a
// single audio file (Author/Title.m4b) or a directory whose direct children
// include audio files (Author/Title/*.mp3 — recorded once, as the
// directory, with the summed size).
func (s *Service) scanAudiobookRoot(ctx context.Context, root library.RootFolder, index *matchIndex, result *Result) error {
	known, err := s.store.BookFilePathsUnderRoot(root.ID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}

	record := func(path string, size int64, format string, modified time.Time) error {
		rel, err := filepath.Rel(root.Path, path)
		if err != nil {
			return err
		}
		bookID := index.match(ParsePath(rel))
		file := &library.BookFile{
			RootFolderID: root.ID,
			BookID:       bookID,
			MediaType:    "audiobook",
			Path:         path,
			Size:         size,
			Format:       format,
			ModifiedAt:   modified.UTC().Format(time.RFC3339),
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
	}

	err = filepath.WalkDir(root.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !d.IsDir() {
			// Loose audio file (not inside a claimed book directory).
			if !IsAudioPath(path) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			return record(path, info.Size(), strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."), info.ModTime())
		}
		if strings.HasPrefix(d.Name(), ".") && path != root.Path {
			return filepath.SkipDir
		}
		if path == root.Path {
			return nil
		}
		// Audiobook roots follow the Author/Book convention (Audiobookshelf
		// style): first-level dirs are authors, never book units — loose
		// audio files there (Author/Title.m4b) are single-file units. Deeper
		// leaf dirs (files only) with audio children are one audiobook each;
		// dirs that still contain subdirectories are navigation levels.
		rel, err := filepath.Rel(root.Path, path)
		if err != nil {
			return err
		}
		if !strings.Contains(filepath.ToSlash(rel), "/") {
			return nil // author level
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		hasAudio := false
		hasSubdir := false
		for _, e := range entries {
			if e.IsDir() {
				hasSubdir = true
			} else if IsAudioPath(e.Name()) {
				hasAudio = true
			}
		}
		if !hasAudio || hasSubdir {
			return nil
		}
		size, format, modified := audiobookDirInfo(path)
		if err := record(path, size, format, modified); err != nil {
			return err
		}
		return filepath.SkipDir
	})
	if err != nil {
		return err
	}

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

// audiobookDirInfo sums a book directory's audio content: total size, the
// format of its largest audio file, and the newest modification time.
func audiobookDirInfo(dir string) (int64, string, time.Time) {
	var total, largest int64
	var format string
	var modified time.Time
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !IsAudioPath(p) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		if info.Size() >= largest {
			largest = info.Size()
			format = strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".")
		}
		if info.ModTime().After(modified) {
			modified = info.ModTime()
		}
		return nil
	})
	return total, format, modified
}

// scanComicRoot walks a manga/comic root where each archive file is one
// volume/issue: Series/Series v05.cbz (series from the directory) or loose
// Series v05.cbz (series from the filename prefix).
func (s *Service) scanComicRoot(ctx context.Context, root library.RootFolder, index *matchIndex, result *Result) error {
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
		if !comicExtensions[ext] {
			return nil
		}

		rel, err := filepath.Rel(root.Path, path)
		if err != nil {
			return err
		}
		seriesGuess, number := comicGuess(rel)
		bookID := index.matchVolume(root.MediaType, seriesGuess, number)

		file := &library.BookFile{
			RootFolderID: root.ID,
			BookID:       bookID,
			MediaType:    root.MediaType,
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

// RematchUnmatched re-runs matching for unmatched file records against the
// current library — no disk walk, pure DB. Called after books are added so
// files found by an earlier scan attach the moment their book exists.
func (s *Service) RematchUnmatched() (int, error) {
	files, err := s.store.ListUnmatchedBookFiles()
	if err != nil || len(files) == 0 {
		return 0, err
	}
	roots, err := s.store.ListRootFolders()
	if err != nil {
		return 0, err
	}
	rootByID := map[int64]library.RootFolder{}
	for _, r := range roots {
		rootByID[r.ID] = r
	}
	index, err := s.buildIndex()
	if err != nil {
		return 0, err
	}

	matched := 0
	for _, f := range files {
		root, ok := rootByID[f.RootFolderID]
		if !ok {
			continue
		}
		rel, err := filepath.Rel(root.Path, f.Path)
		if err != nil {
			continue
		}
		var bookID int64
		if root.MediaType == "manga" || root.MediaType == "comic" {
			seriesGuess, number := comicGuess(rel)
			bookID = index.matchVolume(root.MediaType, seriesGuess, number)
		} else {
			bookID = index.match(ParsePath(rel))
		}
		if bookID == 0 {
			continue
		}
		if err := s.store.SetBookFileBook(f.ID, bookID); err != nil {
			return matched, err
		}
		matched++
	}
	if matched > 0 {
		slog.Info("rematched unmatched files", "matched", matched)
	}
	return matched, nil
}

// comicGuess extracts the series name and volume number from a relative
// archive path: the parent directory names the series when present,
// otherwise the filename prefix before the volume marker.
func comicGuess(rel string) (string, float64) {
	name := filepath.Base(rel)
	number := VolumeFromName(name)
	if dir := filepath.Dir(rel); dir != "." {
		return filepath.Base(dir), number
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if m := volumeMarker.FindStringIndex(base); m != nil {
		return strings.TrimSpace(strings.Trim(base[:m[0]], "-–")), number
	}
	return base, number
}

// matchIndex holds normalized lookups over the whole library, built once per
// scan run.
type matchIndex struct {
	authorsByName map[string]int64             // Normalize(author name) → author id
	byAuthorTitle map[int64]map[string]int64   // author id → title key → book id
	byTitle       map[string]map[int64]bool    // title key → set of book ids
	volumes       map[string]map[float64]int64 // mediaType/series key → number → book id
}

// matchVolume resolves a manga/comic archive to a volume book id, or 0.
func (idx *matchIndex) matchVolume(mediaType, seriesGuess string, number float64) int64 {
	if number == 0 || seriesGuess == "" {
		return 0
	}
	return idx.volumes[mediaType+"/"+Normalize(seriesGuess)][number]
}

func (s *Service) buildIndex() (*matchIndex, error) {
	idx := &matchIndex{
		authorsByName: map[string]int64{},
		byAuthorTitle: map[int64]map[string]int64{},
		byTitle:       map[string]map[int64]bool{},
		volumes:       map[string]map[float64]int64{},
	}

	refs, err := s.store.ListVolumeRefs()
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		key := ref.MediaType + "/" + Normalize(ref.SeriesTitle)
		if idx.volumes[key] == nil {
			idx.volumes[key] = map[float64]int64{}
		}
		idx.volumes[key][ref.Position] = ref.BookID
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
