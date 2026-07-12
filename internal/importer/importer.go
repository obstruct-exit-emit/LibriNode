// Package importer is Completed Download Handling: it watches download
// clients for finished grabs, copies the book file into the library laid out
// by the naming templates, records it, and resolves the grab. Files are
// copied (never moved) so torrents keep seeding; usenet history entries are
// cleaned up after import.
package importer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/librinode/librinode/internal/comicinfo"
	"github.com/librinode/librinode/internal/config"
	"github.com/librinode/librinode/internal/download"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/opf"
	"github.com/librinode/librinode/internal/organize"
	"github.com/librinode/librinode/internal/release"
	"github.com/librinode/librinode/internal/scanner"
)

type Service struct {
	store     *library.Store
	downloads *download.Service
	organize  *organize.Service
	// settings (optional) reports the current Completed Download Handling
	// options — pack-import-all, and the post-import client/file cleanup.
	settings func() config.ImportSettings
}

func New(store *library.Store, downloads *download.Service, org *organize.Service, settings func() config.ImportSettings) *Service {
	return &Service{store: store, downloads: downloads, organize: org, settings: settings}
}

// opts returns the current import settings, tolerating a nil provider.
func (s *Service) opts() config.ImportSettings {
	if s.settings == nil {
		return config.ImportSettings{}
	}
	return s.settings()
}

// errDownloadPending marks a download the client reports as done but whose
// files aren't readable yet — the path is missing because it's still syncing
// (a network share, or a debrid bridge that finalizes after reporting
// complete). The import is retried on the next pass instead of being failed and
// the release blocklisted.
var errDownloadPending = errors.New("download not ready")

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
	imported, err := s.downloads.Store().ListGrabs(download.GrabStatusImported)
	if err != nil {
		return nil, err
	}

	for i := range items {
		item := &items[i]
		grab := matchGrab(pending, item)
		switch item.Status {
		case "completed":
			s.importItem(ctx, item, grab, result)
		case "seeded":
			// Seed goal reached (the client paused/stopped the finished
			// torrent). Import it if we never did, then clean up: an
			// already-imported grab's torrent is removed with its data.
			if grab != nil {
				s.importItem(ctx, item, grab, result)
				continue
			}
			if g := matchGrab(imported, item); g != nil {
				if err := s.downloads.Remove(ctx, item.ConfigID, item.ID, true); err != nil {
					result.note("removing seeded %s: %v", item.Title, err)
				} else {
					slog.Info("removed torrent after seeding goal", "title", item.Title)
					result.note("removed %s after seeding goal", item.Title)
				}
			}
		case "failed":
			if grab == nil {
				continue
			}
			_ = s.downloads.Store().ResolveGrab(grab.ID, download.GrabStatusFailed, "download failed in client")
			// Never grab this release again; search falls to the next candidate.
			if err := s.downloads.Store().AddBlock(grab.GUID, grab.Title, "download failed in client"); err != nil {
				result.note("blocklisting %s: %v", grab.Title, err)
			}
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

// writeOPF drops the metadata sidecar next to an imported book: metadata.opf
// in the per-book folder for audiobooks, <file>.opf beside flat ebook files.
func (s *Service) writeOPF(book *library.Book, mediaType, target, dir string) error {
	author, err := s.store.GetAuthor(book.AuthorID)
	if err != nil {
		return err
	}
	series, err := s.store.ListSeriesForBook(book.ID)
	if err != nil {
		return err
	}
	full, err := s.store.GetBook(book.ID) // detail includes editions (ISBN, language)
	if err != nil {
		full = book
	}
	data, err := opf.Render(full, author.Name, series)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "metadata.opf")
	if mediaType == "ebook" {
		path = strings.TrimSuffix(target, filepath.Ext(target)) + ".opf"
	}
	return os.WriteFile(path, data, 0o644)
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
	// Untracked downloads never replace existing files; tracked grabs for
	// owned books may be upgrades — decided below once the format is known.
	if owned && grab == nil {
		result.Skipped++
		return
	}

	var sources []string
	var format string
	var pack *packPlan // set when the download is a multi-book release
	switch mediaType {
	case "audiobook":
		sources, format, err = pickAudioFiles(item.Path)
	case "manga", "comic":
		var source string
		source, pack, err = s.pickPackAware(item.Path, scanner.IsComicPath, "comic archive", grab, book, mediaType)
		sources = []string{source}
		format = fileFormat(source)
	case "magazine":
		var source string
		source, err = pickLargestFile(item.Path, scanner.IsMagazinePath, "magazine file")
		sources = []string{source}
		format = fileFormat(source)
	default:
		var source string
		source, pack, err = s.pickPackAware(item.Path, scanner.IsEbookPath, "ebook", grab, book, mediaType)
		sources = []string{source}
		format = fileFormat(source)
	}
	if err != nil {
		if grab != nil && !errors.Is(err, errDownloadPending) {
			s.resolve(grab, download.GrabStatusFailed, err.Error())
			result.Failed++
		} else {
			// Files not ready yet (still syncing), or an untracked download —
			// leave the grab pending and retry next pass rather than failing.
			result.Skipped++
		}
		return
	}

	// Owned + tracked grab: only proceed when the new format genuinely
	// upgrades the owned one; the old files are replaced after import.
	var replacing []library.BookFile
	if owned {
		old, better := s.upgradeCheck(book, mediaType, format)
		if !better {
			s.resolve(grab, download.GrabStatusImported,
				"book already has a "+mediaType+" file (not an upgrade)")
			result.Skipped++
			return
		}
		replacing = old
	}

	target, ok := s.placeAndRecord(book, mediaType, format, sources, replacing, item.Title, result)
	if !ok {
		return
	}

	if grab != nil {
		message := "imported to " + target
		if len(replacing) > 0 {
			message = "upgraded (" + replacing[0].Format + " → " + format + "), imported to " + target
		}
		s.resolve(grab, download.GrabStatusImported, message)
	}
	result.Imported++
	slog.Info("imported download", "book", book.Title, "path", target)

	// Multi-book pack: the download's other files fill more books. This reads
	// from the download folder, so it must run before any cleanup deletes it.
	if grab != nil && pack != nil {
		s.importPackExtras(pack, sources[0], book, mediaType, result)
	}
	if grab != nil {
		s.cleanupAfterImport(ctx, item, grab, result)
	}
}

// cleanupAfterImport removes an imported download from its client per the
// Completed Download Handling settings. With both options off (the default),
// usenet history entries are cleared — the file stays, LibriNode only copied
// it — and torrents keep seeding. RemoveCompleted removes the download from the
// client for both protocols; DeleteCompletedFiles additionally deletes the
// downloaded files from disk.
func (s *Service) cleanupAfterImport(ctx context.Context, item *download.Item, grab *download.GrabRecord, result *Result) {
	opts := s.opts()
	if opts.RemoveCompleted || opts.DeleteCompletedFiles {
		if err := s.downloads.Remove(ctx, item.ConfigID, item.ID, opts.DeleteCompletedFiles); err != nil {
			result.note("removing %s from client: %v", item.Title, err)
		}
		// Some clients (debrid bridges) acknowledge the removal but ignore the
		// delete-files flag. LibriNode imported from this path, so delete it
		// directly to be sure the source is gone.
		if opts.DeleteCompletedFiles {
			deleteDownloadData(item.Path, result)
		}
		return
	}
	// Default: clear the usenet history entry (no data deleted); leave torrents
	// seeding until the client's own goal is reached.
	if grab.Protocol == download.ProtocolUsenet {
		if err := s.downloads.Remove(ctx, item.ConfigID, item.ID, false); err != nil {
			result.note("cleaning up %s: %v", item.Title, err)
		}
	}
}

// deleteDownloadData removes the download's own files after import, for the
// DeleteCompletedFiles option, guarding against a misreported path: it must be
// absolute and nested at least three segments deep (…/downloads/<client>/
// <release>) so a bad path can never wipe a mount root or top-level directory.
func deleteDownloadData(path string, result *Result) {
	if path == "" || !filepath.IsAbs(path) {
		return
	}
	clean := filepath.Clean(path)
	segs := strings.FieldsFunc(clean, func(r rune) bool { return r == '/' || r == '\\' })
	if len(segs) < 3 {
		result.note("refusing to delete shallow download path %q", clean)
		return
	}
	if err := os.RemoveAll(clean); err != nil {
		result.note("deleting download files %s: %v", clean, err)
	}
}

// placeAndRecord copies the source files into the library at the naming
// template's path, writes the format's sidecars, records the book file, and
// removes any replaced (upgraded) files. Returns the target path; false means
// the import was skipped and noted in result.
func (s *Service) placeAndRecord(book *library.Book, mediaType, format string, sources []string, replacing []library.BookFile, itemTitle string, result *Result) (string, bool) {
	place, err := s.organize.PlaceFile(book, format, mediaType)
	if err != nil {
		result.note("%s: %v", itemTitle, err)
		result.Skipped++
		return "", false
	}

	var target string
	var size int64
	if mediaType == "audiobook" && len(sources) > 1 {
		// Multi-file audiobook: the per-book folder is the unit; original
		// track names are preserved inside it.
		target = place.Dir
		if _, err := os.Stat(target); err == nil && len(replacing) == 0 {
			result.note("%s: target already exists: %s", itemTitle, target)
			result.Skipped++
			return "", false
		}
		for _, src := range sources {
			n, err := copyFile(src, filepath.Join(target, filepath.Base(src)))
			if err != nil {
				result.note("%s: %v", itemTitle, err)
				result.Skipped++
				return "", false
			}
			size += n
		}
	} else {
		target = filepath.Join(place.Dir, place.FileName)
		if _, err := os.Stat(target); err == nil {
			result.note("%s: target already exists: %s", itemTitle, target)
			result.Skipped++
			return "", false
		}
		if size, err = copyFile(sources[0], target); err != nil {
			result.note("%s: %v", itemTitle, err)
			result.Skipped++
			return "", false
		}
	}

	// Comic archives get a ComicInfo.xml sidecar inside the CBZ so Kavita/
	// Komga pick up series metadata; failures aren't fatal to the import.
	if (mediaType == "manga" || mediaType == "comic") && format == "cbz" {
		if err := s.writeComicInfo(target, book); err != nil {
			result.note("%s: writing ComicInfo.xml: %v", itemTitle, err)
		}
	}

	// Ebooks and audiobooks get an OPF sidecar (Calibre/Audiobookshelf);
	// failures aren't fatal to the import.
	if mediaType == "ebook" || mediaType == "audiobook" {
		if err := s.writeOPF(book, mediaType, target, place.Dir); err != nil {
			result.note("%s: writing OPF sidecar: %v", itemTitle, err)
		}
	}

	file := &library.BookFile{
		RootFolderID: place.RootFolderID,
		BookID:       book.ID,
		MediaType:    mediaType,
		Variant:      place.Variant, // manga colorized/monochrome; '' otherwise
		Path:         target,
		Size:         size,
		Format:       format,
		ModifiedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.store.UpsertBookFile(file); err != nil {
		result.note("%s: recording file: %v", itemTitle, err)
		result.Skipped++
		return "", false
	}

	// Upgrade: the replaced files leave disk and library together.
	for _, old := range replacing {
		if strings.EqualFold(old.Path, target) {
			continue
		}
		if err := os.RemoveAll(old.Path); err != nil {
			result.note("removing upgraded file %s: %v", old.Path, err)
		}
		if err := s.store.DeleteBookFile(old.ID); err != nil && !errorsIsNotFound(err) {
			result.note("forgetting upgraded file %s: %v", old.Path, err)
		}
	}
	return target, true
}

// importPackExtras imports the remaining files of a multi-book release
// ("complete series" bundles). Default policy: only files matching a
// *monitored* library book are imported — grabbing one volume from a pack
// never auto-imports unmonitored ones. The opt-in pack-import-all setting
// lifts that to every matching book (imported ebooks/audiobooks then join
// their format library, like scanned files do). Either way, a book that
// already has this format's file is only replaced when the pack's copy is a
// genuine quality upgrade.
func (s *Service) importPackExtras(pack *packPlan, primary string, grabbed *library.Book, mediaType string, result *Result) {
	importAll := s.opts().PackImportAll
	done := map[int64]bool{grabbed.ID: true}
	for _, f := range pack.files {
		if f == primary {
			continue
		}
		b := pack.matcher.match(f)
		if b == nil || done[b.ID] {
			continue
		}
		done[b.ID] = true
		if !importAll && !monitoredFor(b, mediaType) {
			continue
		}
		format := fileFormat(f)
		var replacing []library.BookFile
		if len(s.ownedFiles(b.ID, mediaType)) > 0 {
			old, better := s.upgradeCheck(b, mediaType, format)
			if !better {
				continue
			}
			replacing = old
		}
		target, ok := s.placeAndRecord(b, mediaType, format, []string{f}, replacing, filepath.Base(f), result)
		if !ok {
			continue
		}
		// Owning a file puts a prose book in the format's library (same as
		// scan); volumes already belong to their series.
		if err := s.store.EnsureBookLibrary(b.ID, mediaType); err != nil {
			result.note("pack: enrolling %s: %v", b.Title, err)
		}
		result.Imported++
		result.note("pack: imported %s for %s", filepath.Base(f), b.Title)
		slog.Info("imported pack extra", "book", b.Title, "path", target)
	}
}

// packMatcher resolves a pack's files to library books from data fetched
// once per download: the grabbed volume's series (manga/comic) or the
// grabbed book's author's bibliography (ebooks).
type packMatcher struct {
	mediaType string
	volumes   []library.Book    // manga/comic: the series' volumes…
	positions map[int64]float64 // …and their volume numbers
	books     []library.Book    // ebook: the author's books
}

func (s *Service) newPackMatcher(grabbed *library.Book, mediaType string) *packMatcher {
	m := &packMatcher{mediaType: mediaType}
	switch mediaType {
	case "manga", "comic":
		links, err := s.store.ListSeriesForBook(grabbed.ID)
		if err != nil || len(links) == 0 {
			return m
		}
		if m.positions, err = s.store.SeriesBookPositions(links[0].SeriesID); err != nil {
			return m
		}
		m.volumes, _ = s.store.ListVolumes(links[0].SeriesID)
	default: // ebook
		m.books, _ = s.store.ListBooks(grabbed.AuthorID)
	}
	return m
}

// match resolves one file to a library book: manga/comic files match by
// volume number within the series; ebooks match by title, and only when the
// match is unambiguous.
func (m *packMatcher) match(path string) *library.Book {
	switch m.mediaType {
	case "manga", "comic":
		number := scanner.VolumeFromName(filepath.Base(path))
		if number == 0 {
			return nil
		}
		for i := range m.volumes {
			if m.positions[m.volumes[i].ID] == number {
				return &m.volumes[i]
			}
		}
		return nil
	default: // ebook
		norm := scanner.Normalize(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		var match *library.Book
		for i := range m.books {
			b := &m.books[i]
			if b.MediaType != "book" {
				continue
			}
			for _, key := range scanner.TitleKeys(b.Title) {
				if key != "" && strings.Contains(norm, key) {
					if match != nil && match.ID != b.ID {
						return nil // ambiguous — two different books match
					}
					match = b
					break
				}
			}
		}
		return match
	}
}

// monitoredFor reports whether the book is monitored for the media type —
// prose books monitor per format library, volumes/issues use the plain flag.
func monitoredFor(b *library.Book, mediaType string) bool {
	switch mediaType {
	case "ebook":
		return b.InEbookLibrary && b.EbookMonitored
	case "audiobook":
		return b.InAudiobookLibrary && b.AudiobookMonitored
	default:
		return b.Monitored
	}
}

// ownedFiles returns the book's files of one media type.
func (s *Service) ownedFiles(bookID int64, mediaType string) []library.BookFile {
	files, err := s.store.ListBookFiles(bookID)
	if err != nil {
		return nil
	}
	owned := []library.BookFile{}
	for _, f := range files {
		if f.MediaType == mediaType {
			owned = append(owned, f)
		}
	}
	return owned
}

// upgradeCheck decides whether newFormat genuinely upgrades the book's
// owned files of this media type (per the type's quality profile), returning
// the files to replace.
func (s *Service) upgradeCheck(book *library.Book, mediaType, newFormat string) ([]library.BookFile, bool) {
	prefs := release.PreferencesFor(s.store, mediaType)
	newScore, ok := prefs.FormatScores[newFormat]
	if !ok {
		return nil, false
	}
	files, err := s.store.ListBookFiles(book.ID)
	if err != nil {
		return nil, false
	}
	old := []library.BookFile{}
	ownedBest := 0
	for _, f := range files {
		if f.MediaType != mediaType {
			continue
		}
		old = append(old, f)
		if sc, ok := prefs.FormatScores[f.Format]; ok && sc > ownedBest {
			ownedBest = sc
		}
	}
	if len(old) == 0 {
		return nil, false
	}
	return old, newScore > ownedBest
}

func errorsIsNotFound(err error) bool {
	return errors.Is(err, library.ErrNotFound)
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
		// Only prose books: the fallback imports as ebook, and volumes/issues
		// are always acquired through tracked grabs.
		if b.MediaType != "book" || b.HasEbookFile || !b.Monitored {
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

// pickPackAware selects the file to import for the grabbed book and, when the
// download is a multi-book pack, also returns the full candidate list for
// pack-extra imports. Single-candidate downloads behave as always: the
// largest acceptable file wins (releases ship samples and extras). Tracked
// multi-file downloads are packs — the grabbed book's file is identified by
// volume number (manga/comic) or title (ebooks), never by size: the largest
// file of a v01–v12 bundle is rarely the volume that was grabbed.
func (s *Service) pickPackAware(path string, accept func(string) bool, kind string, grab *download.GrabRecord, book *library.Book, mediaType string) (string, *packPlan, error) {
	files, err := listAcceptable(path, accept, kind)
	if err != nil {
		return "", nil, err
	}
	if grab == nil || len(files) < 2 {
		return largestFile(files), nil, nil
	}
	matcher := s.newPackMatcher(book, mediaType)
	var match string
	var matchSize int64
	for _, f := range files {
		b := matcher.match(f.path)
		if b != nil && b.ID == book.ID && f.size > matchSize {
			match, matchSize = f.path, f.size
		}
	}
	if match == "" {
		return "", nil, fmt.Errorf("multi-file download has no file matching %q", book.Title)
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.path)
	}
	return match, &packPlan{files: paths, matcher: matcher}, nil
}

// packPlan carries a multi-book download's candidate files and their
// matcher from primary-file selection to the pack-extras pass.
type packPlan struct {
	files   []string
	matcher *packMatcher
}

type candidateFile struct {
	path string
	size int64
}

// listAcceptable returns every file at path (a file or directory) the matcher
// accepts, with sizes; an error when there are none.
func listAcceptable(path string, accept func(string) bool, kind string) ([]candidateFile, error) {
	if path == "" {
		return nil, fmt.Errorf("client reported no path yet: %w", errDownloadPending)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("download path missing (%v): %w", err, errDownloadPending)
	}
	if !info.IsDir() {
		if !accept(path) {
			return nil, fmt.Errorf("%s is not a %s", filepath.Base(path), kind)
		}
		return []candidateFile{{path, info.Size()}}, nil
	}
	var files []candidateFile
	err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !accept(p) {
			return err
		}
		if fi, err := d.Info(); err == nil {
			files = append(files, candidateFile{p, fi.Size()})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no %s found in download", kind)
	}
	return files, nil
}

// largestFile picks the biggest candidate (callers guarantee at least one).
func largestFile(files []candidateFile) string {
	best := files[0]
	for _, f := range files[1:] {
		if f.size > best.size {
			best = f
		}
	}
	return best.path
}

// fileFormat is the lowercased extension without the dot.
func fileFormat(path string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
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
	files, err := listAcceptable(path, accept, kind)
	if err != nil {
		return "", err
	}
	return largestFile(files), nil
}

// pickAudioFiles returns the audio content at path: all audio files under a
// directory (multi-file audiobook), or the single audio file itself. The
// format is the largest file's extension.
func pickAudioFiles(path string) ([]string, string, error) {
	if path == "" {
		return nil, "", fmt.Errorf("client reported no path yet: %w", errDownloadPending)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", fmt.Errorf("download path missing (%v): %w", err, errDownloadPending)
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
