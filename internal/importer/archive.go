package importer

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/librinode/librinode/internal/scanner"
	"github.com/nwaples/rardecode/v2"
)

// audioArchiveExts are the archive types an audiobook can ship packed inside —
// a single .zip/.rar holding the tracks, common on usenet.
var audioArchiveExts = map[string]bool{".zip": true, ".rar": true}

func isAudioArchive(name string) bool {
	return audioArchiveExts[strings.ToLower(filepath.Ext(name))]
}

// rarContinuation matches a multi-volume RAR's SECOND-and-later parts
// ("book.part2.rar", "book.r00") — rardecode follows them automatically from
// the first volume, so extracting one on its own would fail. They're skipped
// when collecting archives to process.
var rarContinuation = regexp.MustCompile(`(?i)\.(?:part0*[2-9][0-9]*\.rar|r[0-9]{2})$`)

// maxArchiveEntryBytes caps a single extracted file. Audiobook tracks (and a
// lone m4b) run large, so it's deliberately generous; it exists only so a
// crafted entry claiming to be audio can't fill the disk.
const maxArchiveEntryBytes int64 = 8 << 30

// extractAudioArchives handles an audiobook shipped as one or more .zip/.rar
// archives: when the download carries archives but no loose audio, it extracts
// the audio entries to a fresh temp dir — each archive under a folder named for
// it, so a multi-disc "CD1.zip"/"CD2.zip" release becomes disc subfolders — and
// returns that dir plus a cleanup func to run once the import has copied from
// it. Returns "" (and a no-op cleanup) when there's nothing to extract: no
// archives, loose audio already present (the normal path handles it), or the
// archives held no audio. A read error — a truncated archive still syncing on a
// debrid mount, or a genuinely corrupt one — is returned for the caller to
// retry and, past the grace period, abandon like any other not-ready download.
func (s *Service) extractAudioArchives(root string) (dir string, cleanup func(), err error) {
	noop := func() {}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", noop, nil // a bare file path is handled by the normal flow
	}

	var archives []string
	looseAudio := false
	walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if scanner.IsAudioPath(p) {
			looseAudio = true
		}
		if isAudioArchive(p) && !rarContinuation.MatchString(p) {
			archives = append(archives, p)
		}
		return nil
	})
	if walkErr != nil {
		return "", noop, walkErr
	}
	if looseAudio || len(archives) == 0 {
		return "", noop, nil
	}

	tmp, err := os.MkdirTemp("", "librinode-audio-*")
	if err != nil {
		return "", noop, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }

	extracted := 0
	for _, a := range archives {
		stem := strings.TrimSuffix(filepath.Base(a), filepath.Ext(a))
		n, err := extractAudioFromArchive(a, filepath.Join(tmp, stem))
		if err != nil {
			cleanup()
			return "", noop, fmt.Errorf("%s: %w", filepath.Base(a), err)
		}
		extracted += n
	}
	if extracted == 0 {
		cleanup()
		return "", noop, nil // archives carried no audio — not an audiobook-in-archive
	}
	return tmp, cleanup, nil
}

func extractAudioFromArchive(archivePath, destDir string) (int, error) {
	switch strings.ToLower(filepath.Ext(archivePath)) {
	case ".zip":
		return extractAudioZip(archivePath, destDir)
	case ".rar":
		return extractAudioRar(archivePath, destDir)
	}
	return 0, nil
}

func extractAudioZip(archivePath, destDir string) (int, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return 0, err
	}
	defer r.Close()
	n := 0
	for _, f := range r.File {
		if f.FileInfo().IsDir() || !scanner.IsAudioPath(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return n, err
		}
		err = writeExtracted(destDir, f.Name, rc)
		rc.Close()
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func extractAudioRar(archivePath, destDir string) (int, error) {
	files, err := rardecode.List(archivePath)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, f := range files {
		if f.IsDir || !scanner.IsAudioPath(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return n, err
		}
		err = writeExtracted(destDir, f.Name, rc)
		rc.Close()
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// writeExtracted writes one archive entry under destDir, preserving the entry's
// internal path (so disc subfolders inside the archive survive). It rejects an
// entry whose path escapes destDir — the classic "Zip Slip" traversal — so a
// malicious archive can't write outside the temp dir.
func writeExtracted(destDir, entryName string, r io.Reader) error {
	clean := filepath.Clean(filepath.FromSlash(entryName))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("unsafe archive entry %q", entryName)
	}
	dest := filepath.Join(destDir, clean)
	if rel, err := filepath.Rel(destDir, dest); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("unsafe archive entry %q", entryName)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(r, maxArchiveEntryBytes))
	return err
}
