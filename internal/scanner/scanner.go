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
		case "magazine":
			result.Roots++
			scanErr = s.scanMagazineRoot(ctx, root, index, result)
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
		// file.BookID is the effective match after the upsert — a manual
		// match the walk couldn't reproduce is preserved, and counts as such.
		if file.BookID > 0 {
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
		if file.BookID > 0 { // effective match after upsert (see scanRoot)
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
		allDisc := true
		for _, e := range entries {
			if e.IsDir() {
				hasSubdir = true
				if !IsDiscFolder(e.Name()) {
					allDisc = false
				}
			} else if IsAudioPath(e.Name()) {
				hasAudio = true
			}
		}
		// A book unit is a leaf dir with audio, or a dir whose subdirs are all
		// disc-style (CD1/CD2 …) with audio somewhere inside — a multi-disc
		// book, not a navigation level.
		if hasSubdir && allDisc {
			if size, format, modified := audiobookDirInfo(path); size > 0 {
				if err := record(path, size, format, modified); err != nil {
					return err
				}
				return filepath.SkipDir
			}
			return nil
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
			Variant:      root.Variant, // colorized/monochrome for manga; '' otherwise
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
		if file.BookID > 0 { // effective match after upsert (see scanRoot)
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

// magazineGuess extracts the magazine title and issue identifier from a
// relative file path: the parent directory names the magazine when present,
// otherwise the filename prefix before the last " - ".
func magazineGuess(rel string) (string, string) {
	name := filepath.Base(rel)
	identifier := IssueIdentifier(name)
	if dir := filepath.Dir(rel); dir != "." {
		return filepath.Base(dir), identifier
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if i := strings.LastIndex(base, " - "); i > 0 {
		return strings.TrimSpace(base[:i]), identifier
	}
	return base, identifier
}

// matchMagazineFile resolves (or materializes) the issue book for a scanned
// magazine file. Issues under a known magazine are created on the spot —
// scanning an existing archive populates the library. Unmonitored: we
// already own them.
func (s *Service) matchMagazineFile(index *matchIndex, rel string) (int64, error) {
	guess, identifier := magazineGuess(rel)
	if identifier == "" {
		return 0, nil
	}
	sr := index.magazines[Normalize(guess)]
	if sr == nil {
		return 0, nil
	}
	if existing, err := s.store.GetBookByForeignID(sr.Source, sr.ForeignID+":"+identifier); err == nil {
		return existing.ID, nil
	}
	book, err := s.store.CreateMagazineIssue(sr, identifier, false)
	if err != nil {
		return 0, err
	}
	return book.ID, nil
}

// scanMagazineRoot walks a magazine root where each pdf/epub/cbz is one
// issue: Magazine/Magazine - 2026-07.pdf, matched (and created) by magazine
// title + issue date or number.
func (s *Service) scanMagazineRoot(ctx context.Context, root library.RootFolder, index *matchIndex, result *Result) error {
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
		if !IsMagazinePath(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(root.Path, path)
		if err != nil {
			return err
		}
		bookID, err := s.matchMagazineFile(index, rel)
		if err != nil {
			return err
		}

		file := &library.BookFile{
			RootFolderID: root.ID,
			BookID:       bookID,
			MediaType:    "magazine",
			Path:         path,
			Format:       strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
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
		if file.BookID > 0 { // effective match after upsert (see scanRoot)
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
		switch root.MediaType {
		case "manga", "comic":
			seriesGuess, number := comicGuess(rel)
			bookID = index.matchVolume(root.MediaType, seriesGuess, number)
		case "magazine":
			bookID, err = s.matchMagazineFile(index, rel)
			if err != nil {
				return matched, err
			}
		default:
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
	authorsByName map[string]int64               // Normalize(author name) → author id
	byAuthorTitle map[int64]map[string]keyedBook // author id → title key → best claimant
	byTitle       map[string]map[int64]bool      // title key → set of book ids
	volumes       map[string]map[float64]int64   // mediaType/series key → number → book id
	magazines     map[string]*library.Series     // Normalize(title) → magazine series
}

// keyedBook is one book's claim on a title key. Several books can emit the
// same key — "The Martian" (full title) and "The Martian: Stranded" (subtitle
// variant) both produce "the martian" — and the wrong winner files imports
// under a derivative work. Priority: a full-title claim beats a variant
// claim, then a library member beats a stray, then the first stays.
type keyedBook struct {
	id      int64
	primary bool // the key IS the book's full title, not a variant
	inLib   bool
}

// claim records a book's claim on a key when it beats the current holder.
func (idx *matchIndex) claim(authorID int64, key string, b keyedBook) {
	if idx.byAuthorTitle[authorID] == nil {
		idx.byAuthorTitle[authorID] = map[string]keyedBook{}
	}
	cur, taken := idx.byAuthorTitle[authorID][key]
	if taken {
		if cur.primary != b.primary {
			if cur.primary {
				return
			}
		} else if cur.inLib || !b.inLib {
			return
		}
	}
	idx.byAuthorTitle[authorID][key] = b
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
		byAuthorTitle: map[int64]map[string]keyedBook{},
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

	magazines, err := s.store.ListSeries("magazine")
	if err != nil {
		return nil, err
	}
	idx.magazines = map[string]*library.Series{}
	for i := range magazines {
		idx.magazines[Normalize(magazines[i].Title)] = &magazines[i]
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
		inLib := b.InEbookLibrary || b.InAudiobookLibrary || b.Monitored
		for i, key := range TitleKeys(b.Title) {
			if key == "" {
				continue
			}
			// TitleKeys' first entry is the full title; the rest are variants
			// (subtitle cut, parentheticals stripped) with weaker claims.
			idx.claim(b.AuthorID, key, keyedBook{id: b.ID, primary: i == 0, inLib: inLib})
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
// unambiguous across the whole library. The alt title (after the last dash,
// e.g. our own "Series N - Title" template output) is a fallback candidate.
func (idx *matchIndex) match(p ParsedFile) int64 {
	if p.Title == "" {
		return 0
	}
	keys := TitleKeys(p.Title)
	if p.AltTitle != "" {
		keys = append(keys, TitleKeys(p.AltTitle)...)
	}

	if p.Author != "" {
		if authorID, ok := idx.authorsByName[Normalize(p.Author)]; ok {
			for _, key := range keys {
				if kb, ok := idx.byAuthorTitle[authorID][key]; ok {
					return kb.id
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
