// Package importer is Completed Download Handling: it watches download
// clients for finished grabs, copies the book file into the library laid out
// by the naming templates, records it, and resolves the grab. Files are
// copied (never moved) so torrents keep seeding; usenet history entries are
// cleaned up after import.
package importer

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/librinode/librinode/internal/comicinfo"
	"github.com/librinode/librinode/internal/download"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/organize"
	"github.com/librinode/librinode/internal/scanner"
)

type Service struct {
	store     *library.Store
	downloads *download.Service
	organize  *organize.Service
}

func New(store *library.Store, downloads *download.Service, org *organize.Service) *Service {
	return &Service{store: store, downloads: downloads, organize: org}
}

// Result summarizes one import pass.
type Result struct {
	Imported int      `json:"imported"`
	Failed   int      `json:"failed"`
	Skipped  int      `json:"skipped"`
	Messages []string `json:"messages,omitempty"`
}

func (r *Result) note(format string, args ...any) {
	r.Messages = append(r.Messages, fmt.Sprintf(format, args...))
}

// Run performs one import pass over all download clients.
func (s *Service) Run(ctx context.Context) (*Result, error) {
	result := &Result{Messages: []string{}}

	items, clientErrs, err := s.downloads.Queue(ctx)
	if err != nil {
		return nil, err
	}
	result.Messages = append(result.Messages, clientErrs...)

	pending, err := s.downloads.Store().ListGrabs(download.GrabStatusGrabbed)
	if err != nil {
		return nil, err
	}

	for i := range items {
		item := &items[i]
		grab := matchGrab(pending, item)
		switch item.Status {
		case "completed":
			s.importItem(ctx, item, grab, result)
		case "failed":
			if grab == nil {
				continue
			}
			_ = s.downloads.Store().ResolveGrab(grab.ID, download.GrabStatusFailed, "download failed in client")
			// Failed downloads are junk in the client; clean up data too.
			if err := s.downloads.Remove(ctx, item.ConfigID, item.ID, true); err != nil {
				result.note("removing failed %s: %v", item.Title, err)
			}
			result.Failed++
		}
	}

	if result.Imported > 0 || result.Failed > 0 {
		slog.Info("import pass complete",
			"imported", result.Imported, "failed", result.Failed, "skipped", result.Skipped)
	}
	return result, nil
}

// matchGrab pairs a queue item with its grab record: by the client's item id
// when we have one (SABnzbd), by normalized title otherwise (qBittorrent's
// add endpoint returns no id).
func matchGrab(pending []download.GrabRecord, item *download.Item) *download.GrabRecord {
	for i := range pending {
		g := &pending[i]
		if g.ClientItemID != "" && g.ClientItemID == item.ID {
			return g
		}
	}
	itemTitle := scanner.Normalize(item.Title)
	for i := range pending {
		g := &pending[i]
		if g.ClientItemID == "" && scanner.Normalize(g.Title) == itemTitle {
			return g
		}
	}
	return nil
}

func (s *Service) importItem(ctx context.Context, item *download.Item, grab *download.GrabRecord, result *Result) {
	// Which book is this? Grab record first, title parse as fallback for
	// downloads added outside LibriNode's grab flow (ebook-only fallback:
	// audiobook imports always come from tracked grabs).
	mediaType := "ebook"
	var book *library.Book
	var err error
	if grab != nil && grab.BookID > 0 {
		if grab.MediaType != "" {
			mediaType = grab.MediaType
		}
		book, err = s.store.GetBook(grab.BookID)
		if err != nil {
			s.resolve(grab, download.GrabStatusFailed, "book no longer in library")
			result.Failed++
			return
		}
	} else {
		book = s.matchByTitle(item.Title)
	}
	if book == nil {
		result.Skipped++
		return // not ours to import (yet); stays in the client
	}
	owned := book.HasEbookFile
	if mediaType == "audiobook" {
		owned = book.HasAudiobookFile
	}
	if owned {
		if grab != nil {
			s.resolve(grab, download.GrabStatusImported, "book already has a "+mediaType+" file")
		}
		result.Skipped++
		return
	}

	var sources []string
	var format string
	switch mediaType {
	case "audiobook":
		sources, format, err = pickAudioFiles(item.Path)
	case "manga", "comic":
		var source string
		source, err = pickLargestFile(item.Path, scanner.IsComicPath, "comic archive")
		sources = []string{source}
		format = strings.TrimPrefix(strings.ToLower(filepath.Ext(source)), ".")
	default:
		var source string
		source, err = pickEbookFile(item.Path)
		sources = []string{source}
		format = strings.TrimPrefix(strings.ToLower(filepath.Ext(source)), ".")
	}
	if err != nil {
		if grab != nil {
			s.resolve(grab, download.GrabStatusFailed, err.Error())
			result.Failed++
		} else {
			result.Skipped++
		}
		return
	}

	place, err := s.organize.PlaceFile(book, format, mediaType)
	if err != nil {
		result.note("%s: %v", item.Title, err)
		result.Skipped++
		return
	}

	var target string
	var size int64
	if mediaType == "audiobook" && len(sources) > 1 {
		// Multi-file audiobook: the per-book folder is the unit; original
		// track names are preserved inside it.
		target = place.Dir
		if _, err := os.Stat(target); err == nil {
			result.note("%s: target already exists: %s", item.Title, target)
			result.Skipped++
			return
		}
		for _, src := range sources {
			n, err := copyFile(src, filepath.Join(target, filepath.Base(src)))
			if err != nil {
				result.note("%s: %v", item.Title, err)
				result.Skipped++
				return
			}
			size += n
		}
	} else {
		target = filepath.Join(place.Dir, place.FileName)
		if _, err := os.Stat(target); err == nil {
			result.note("%s: target already exists: %s", item.Title, target)
			result.Skipped++
			return
		}
		if size, err = copyFile(sources[0], target); err != nil {
			result.note("%s: %v", item.Title, err)
			result.Skipped++
			return
		}
	}

	// Comic archives get a ComicInfo.xml sidecar inside the CBZ so Kavita/
	// Komga pick up series metadata; failures aren't fatal to the import.
	if (mediaType == "manga" || mediaType == "comic") && format == "cbz" {
		if err := s.writeComicInfo(target, book); err != nil {
			result.note("%s: writing ComicInfo.xml: %v", item.Title, err)
		}
	}

	file := &library.BookFile{
		RootFolderID: place.RootFolderID,
		BookID:       book.ID,
		MediaType:    mediaType,
		Path:         target,
		Size:         size,
		Format:       format,
		ModifiedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.store.UpsertBookFile(file); err != nil {
		result.note("%s: recording file: %v", item.Title, err)
		result.Skipped++
		return
	}

	if grab != nil {
		s.resolve(grab, download.GrabStatusImported, "imported to "+target)
		// Usenet leaves nothing to seed; clear the history entry (data stays
		// on disk — we only copied from it). Torrents keep seeding.
		if grab.Protocol == download.ProtocolUsenet {
			if err := s.downloads.Remove(ctx, item.ConfigID, item.ID, false); err != nil {
				result.note("cleaning up %s: %v", item.Title, err)
			}
		}
	}
	result.Imported++
	slog.Info("imported download", "book", book.Title, "path", target)
}

func (s *Service) resolve(grab *download.GrabRecord, status, message string) {
	if err := s.downloads.Store().ResolveGrab(grab.ID, status, message); err != nil {
		slog.Warn("resolving grab", "grab", grab.ID, "error", err)
	}
}

// matchByTitle finds the library book an untracked download belongs to; nil
// unless the parsed title matches exactly one monitored, fileless book.
func (s *Service) matchByTitle(title string) *library.Book {
	books, err := s.store.ListBooks(0)
	if err != nil {
		return nil
	}
	norm := scanner.Normalize(title)
	var match *library.Book
	for i := range books {
		b := &books[i]
		if b.HasEbookFile || !b.Monitored {
			continue
		}
		for _, key := range scanner.TitleKeys(b.Title) {
			if key != "" && strings.Contains(norm, key) {
				if match != nil {
					return nil // ambiguous
				}
				match = b
				break
			}
		}
	}
	return match
}

// pickEbookFile returns the largest ebook file at path (a file or directory).
func pickEbookFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("client reported no path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("download path missing: %w", err)
	}
	if !info.IsDir() {
		if !scanner.IsEbookPath(path) {
			return "", fmt.Errorf("%s is not an ebook", filepath.Base(path))
		}
		return path, nil
	}

	var best string
	var bestSize int64
	err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !scanner.IsEbookPath(p) {
			return err
		}
		if fi, err := d.Info(); err == nil && fi.Size() > bestSize {
			best, bestSize = p, fi.Size()
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if best == "" {
		return "", fmt.Errorf("no ebook file found in download")
	}
	return best, nil
}

// writeComicInfo injects a ComicInfo.xml built from the volume's library
// metadata into an imported CBZ.
func (s *Service) writeComicInfo(cbzPath string, book *library.Book) error {
	info := comicinfo.Info{
		Title:   book.Description, // issue title lives in the description
		Summary: "",
		Writer:  "",
	}
	if author, err := s.store.GetAuthor(book.AuthorID); err == nil {
		info.Writer = author.Name
	}
	if links, err := s.store.ListSeriesForBook(book.ID); err == nil && len(links) > 0 {
		info.Series = links[0].Title
		info.Number = strconv.FormatFloat(links[0].Position, 'f', -1, 64)
	}
	if len(book.ReleaseDate) >= 4 {
		if y, err := strconv.Atoi(book.ReleaseDate[:4]); err == nil {
			info.Year = y
		}
	}
	return comicinfo.Inject(cbzPath, info)
}

// pickLargestFile returns the largest file at path (a file or directory)
// accepted by the matcher.
func pickLargestFile(path string, accept func(string) bool, kind string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("client reported no path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("download path missing: %w", err)
	}
	if !info.IsDir() {
		if !accept(path) {
			return "", fmt.Errorf("%s is not a %s", filepath.Base(path), kind)
		}
		return path, nil
	}
	var best string
	var bestSize int64
	err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !accept(p) {
			return err
		}
		if fi, err := d.Info(); err == nil && fi.Size() > bestSize {
			best, bestSize = p, fi.Size()
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if best == "" {
		return "", fmt.Errorf("no %s found in download", kind)
	}
	return best, nil
}

// pickAudioFiles returns the audio content at path: all audio files under a
// directory (multi-file audiobook), or the single audio file itself. The
// format is the largest file's extension.
func pickAudioFiles(path string) ([]string, string, error) {
	if path == "" {
		return nil, "", fmt.Errorf("client reported no path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", fmt.Errorf("download path missing: %w", err)
	}
	if !info.IsDir() {
		if !scanner.IsAudioPath(path) {
			return nil, "", fmt.Errorf("%s is not an audiobook file", filepath.Base(path))
		}
		return []string{path}, strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."), nil
	}

	var files []string
	var largest int64
	var format string
	err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !scanner.IsAudioPath(p) {
			return err
		}
		files = append(files, p)
		if fi, err := d.Info(); err == nil && fi.Size() > largest {
			largest = fi.Size()
			format = strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".")
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	if len(files) == 0 {
		return nil, "", fmt.Errorf("no audio files found in download")
	}
	return files, format, nil
}

// copyFile copies (never moves — torrents keep seeding) source into place.
func copyFile(source, target string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	in, err := os.Open(source)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, err
	}
	size, err := io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(target)
		return 0, err
	}
	return size, nil
}

// RunPeriodic runs import passes on the interval until ctx is cancelled.
func (s *Service) RunPeriodic(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.Run(ctx); err != nil {
				slog.Warn("import pass failed", "error", err)
			}
		}
	}
}
