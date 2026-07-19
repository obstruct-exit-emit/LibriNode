package download

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/librinode/librinode/internal/redact"
)

// The direct client is LibriNode's own downloader — a third protocol beside
// torrent and usenet for sources that hand out plain HTTP file links (Anna's
// Archive fast-download, Libgen mirrors, open-book collections). There is no
// external program: Add streams the file into the configured download folder
// itself, Completed Download Handling imports the result like any other
// finished download, and Remove deletes it.
//
// A release's download URL may carry several mirrors separated by "|" — they
// are tried in order until one delivers. A fetched body that turns out to be
// JSON with a "download_url" field (the shape membership APIs like Anna's
// fast_download.json answer with) is followed one hop to the real file.
//
// The in-flight queue is in-memory: a restart forgets active downloads (the
// files on disk keep whatever bytes arrived; the importer's orphan sweep
// resolves their grabs). Completed items survive as files and are re-listed
// until removed.

const (
	// directTimeout bounds one download end to end.
	directTimeout = 2 * time.Hour
	// directUA is sent on every request; some file hosts refuse blank agents.
	directUA = "LibriNode"
)

type directItem struct {
	id       string
	title    string
	status   string // queued | downloading | completed | failed
	progress float64
	path     string
	cancel   context.CancelFunc
	err      string
}

type direct struct {
	cfg   *ClientConfig
	httpc *http.Client

	mu    sync.Mutex
	items map[string]*directItem
}

func newDirect(cfg *ClientConfig) *direct {
	// No overall client timeout — downloads are long; each download's context
	// carries its own deadline instead.
	return &direct{cfg: cfg, httpc: &http.Client{}, items: map[string]*directItem{}}
}

// dir is the configured download folder (stored in the config's Host field —
// the direct client has no host; the UI labels the field accordingly).
func (d *direct) dir() string { return strings.TrimSpace(d.cfg.Host) }

// Test verifies the download folder exists (creating it if needed) and is
// writable.
func (d *direct) Test(ctx context.Context) error {
	dir := d.dir()
	if dir == "" {
		return fmt.Errorf("no download folder configured")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating download folder: %w", err)
	}
	probe := filepath.Join(dir, ".librinode-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return fmt.Errorf("download folder not writable: %w", err)
	}
	os.Remove(probe)
	return nil
}

// Add starts downloading the release; the returned id identifies it in List/
// Remove. The download itself runs in the background — Add returns as soon as
// it's underway, like handing a URL to an external client would.
func (d *direct) Add(ctx context.Context, rawURL, title string) (string, error) {
	mirrors := splitMirrors(rawURL)
	if len(mirrors) == 0 {
		return "", fmt.Errorf("release has no download URL (the source may need a membership/API key for downloads)")
	}
	if err := d.Test(ctx); err != nil {
		return "", err
	}

	id := randomID()
	// Independent of the request context: the HTTP request that triggered the
	// grab finishes long before the download does.
	dctx, cancel := context.WithTimeout(context.Background(), directTimeout)
	item := &directItem{id: id, title: title, status: "queued", cancel: cancel}
	d.mu.Lock()
	d.items[id] = item
	d.mu.Unlock()

	go d.run(dctx, item, mirrors)
	return id, nil
}

// run tries each mirror in order until one delivers the file.
func (d *direct) run(ctx context.Context, item *directItem, mirrors []string) {
	defer item.cancel()
	var lastErr error
	for _, m := range mirrors {
		if ctx.Err() != nil {
			lastErr = ctx.Err()
			break
		}
		d.setStatus(item, "downloading", "")
		path, err := d.download(ctx, item, m)
		if err == nil {
			d.mu.Lock()
			item.path = path
			item.status = "completed"
			item.progress = 1
			d.mu.Unlock()
			return
		}
		lastErr = err
	}
	msg := "download failed"
	if lastErr != nil {
		msg = redact.URLError(lastErr).Error()
	}
	d.setStatus(item, "failed", msg)
}

func (d *direct) setStatus(item *directItem, status, errMsg string) {
	d.mu.Lock()
	item.status = status
	item.err = errMsg
	d.mu.Unlock()
}

// download fetches one URL into the folder, following a JSON "download_url"
// answer (membership fast-download APIs) one hop to the real file.
func (d *direct) download(ctx context.Context, item *directItem, rawURL string) (string, error) {
	resp, err := d.get(ctx, rawURL)
	if err != nil {
		return "", err
	}

	// A JSON answer isn't the file — it's an API envelope naming the file.
	if strings.Contains(resp.Header.Get("Content-Type"), "json") {
		defer resp.Body.Close()
		var envelope struct {
			DownloadURL string `json:"download_url"`
			Error       string `json:"error"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); err != nil {
			return "", fmt.Errorf("unexpected JSON answer: %w", err)
		}
		if envelope.DownloadURL == "" {
			if envelope.Error != "" {
				return "", fmt.Errorf("download API: %s", envelope.Error)
			}
			return "", fmt.Errorf("download API answered without a download_url")
		}
		if resp, err = d.get(ctx, envelope.DownloadURL); err != nil {
			return "", err
		}
	}
	defer resp.Body.Close()

	dest := filepath.Join(d.dir(), safeFilename(item.title)+extensionFor(resp))
	part := dest + ".part"
	f, err := os.Create(part)
	if err != nil {
		return "", err
	}

	total := resp.ContentLength
	var written int64
	buf := make([]byte, 128<<10)
	for {
		if ctx.Err() != nil {
			f.Close()
			os.Remove(part)
			return "", ctx.Err()
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				os.Remove(part)
				return "", werr
			}
			written += int64(n)
			if total > 0 {
				d.mu.Lock()
				item.progress = min(float64(written)/float64(total), 0.999)
				d.mu.Unlock()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(part)
			return "", rerr
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(part)
		return "", err
	}
	if written == 0 {
		os.Remove(part)
		return "", fmt.Errorf("mirror delivered an empty file")
	}
	if err := os.Rename(part, dest); err != nil {
		os.Remove(part)
		return "", err
	}
	return dest, nil
}

func (d *direct) get(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", directUA)
	resp, err := d.httpc.Do(req)
	if err != nil {
		return nil, redact.URLError(err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d from mirror", resp.StatusCode)
	}
	return resp, nil
}

// List reports the tracked downloads (this client's own queue).
func (d *direct) List(ctx context.Context) ([]Item, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	items := make([]Item, 0, len(d.items))
	for _, it := range d.items {
		items = append(items, Item{
			Client:   d.cfg.Name,
			ConfigID: d.cfg.ID,
			ID:       it.id,
			Title:    it.title,
			Status:   it.status,
			Progress: it.progress,
			Path:     it.path,
		})
	}
	return items, nil
}

// Remove cancels/forgets a download, optionally deleting its file.
func (d *direct) Remove(ctx context.Context, id string, deleteData bool) error {
	d.mu.Lock()
	it, ok := d.items[id]
	if ok {
		delete(d.items, id)
	}
	d.mu.Unlock()
	if !ok {
		return nil // already gone — removing twice is fine
	}
	it.cancel()
	if deleteData && it.path != "" {
		os.Remove(it.path)
	}
	return nil
}

// splitMirrors splits a "url|url|url" mirror list, dropping empties.
func splitMirrors(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, "|") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// safeFilename reduces a release title to a filesystem-safe base name.
func safeFilename(title string) string {
	title = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return ' '
		}
		return r
	}, title)
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		title = "download"
	}
	if len(title) > 150 {
		title = title[:150]
	}
	return title
}

// extensionFor picks the downloaded file's extension: the URL path's, else the
// Content-Disposition filename's, else one mapped from the Content-Type.
func extensionFor(resp *http.Response) string {
	if ext := path.Ext(resp.Request.URL.Path); plausibleExt(ext) {
		return strings.ToLower(ext)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if i := strings.LastIndex(cd, "filename="); i >= 0 {
			name := strings.Trim(strings.TrimSpace(cd[i+len("filename="):]), `"'`)
			if ext := path.Ext(name); plausibleExt(ext) {
				return strings.ToLower(ext)
			}
		}
	}
	switch ct := resp.Header.Get("Content-Type"); {
	case strings.Contains(ct, "epub"):
		return ".epub"
	case strings.Contains(ct, "pdf"):
		return ".pdf"
	case strings.Contains(ct, "mobi"):
		return ".mobi"
	}
	return ".bin" // unknown — the importer will name what it expected instead
}

// plausibleExt filters URL-path "extensions" that aren't file types.
func plausibleExt(ext string) bool {
	return len(ext) >= 3 && len(ext) <= 6 && !strings.ContainsAny(ext[1:], "./\\?&=")
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "direct-" + hex.EncodeToString(b)
}
