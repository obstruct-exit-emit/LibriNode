package download

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// qBittorrent Web API v2: cookie-session auth via /api/v2/auth/login, then
// form-encoded endpoints under /api/v2. Notable quirk: torrents/add returns
// no hash, so grabs report an empty id and tracking goes by category.
type qbittorrent struct {
	cfg   *ClientConfig
	httpc *http.Client
}

func newQBittorrent(cfg *ClientConfig) *qbittorrent {
	jar, _ := cookiejar.New(nil)
	return &qbittorrent{
		cfg: cfg,
		// A debrid bridge (TorBox/Real-Debrid presenting a qBittorrent API)
		// adds a magnet synchronously — it waits on the debrid service to
		// accept it, which routinely takes longer than a plain qBittorrent's
		// instant add. A short timeout fires mid-add: the torrent still lands,
		// but the grab goes unrecorded. Give adds generous headroom; List
		// bounds its own context so a hung bridge can't stall the import loop.
		httpc: &http.Client{Timeout: 120 * time.Second, Jar: jar},
	}
}

func (q *qbittorrent) base() string {
	return strings.TrimRight(q.cfg.Host, "/")
}

func (q *qbittorrent) login(ctx context.Context) error {
	form := url.Values{"username": {q.cfg.Username}, "password": {q.cfg.Password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		q.base()+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := q.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("qbittorrent: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(string(body), "Ok") {
		return fmt.Errorf("qbittorrent: login failed (%d): %.80s", resp.StatusCode, body)
	}
	return nil
}

// do posts a form (or GETs when form is nil), logging in on the first 403.
func (q *qbittorrent) do(ctx context.Context, path string, form url.Values) ([]byte, error) {
	attempt := func() (*http.Response, error) {
		var req *http.Request
		var err error
		if form == nil {
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, q.base()+path, nil)
		} else {
			req, err = http.NewRequestWithContext(ctx, http.MethodPost,
				q.base()+path, strings.NewReader(form.Encode()))
			if err == nil {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
		}
		if err != nil {
			return nil, err
		}
		return q.httpc.Do(req)
	}

	resp, err := attempt()
	if err != nil {
		return nil, fmt.Errorf("qbittorrent: %w", err)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		if err := q.login(ctx); err != nil {
			return nil, err
		}
		if resp, err = attempt(); err != nil {
			return nil, fmt.Errorf("qbittorrent: %w", err)
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("qbittorrent: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qbittorrent: %s: HTTP %d: %.100s", path, resp.StatusCode, body)
	}
	return body, nil
}

func (q *qbittorrent) Test(ctx context.Context) error {
	if err := q.login(ctx); err != nil {
		return err
	}
	_, err := q.do(ctx, "/api/v2/app/version", nil)
	return err
}

// Add sends a release to qBittorrent. Like the SABnzbd client, it resolves the
// release on LibriNode's side first: the download client is often a NAT'd/cloud
// client (or a debrid bridge) that can't fetch our LAN indexer/Prowlarr URL, so
// handing it that URL fails ("hostname could not be parsed") or stalls.
// LibriNode can reach the indexer, so it follows the URL to the magnet, or
// downloads the .torrent and uploads its bytes — either way the client gets
// something self-contained. A magnet URL is passed straight through; if
// resolution fails, it falls back to handing the client the URL.
func (q *qbittorrent) Add(ctx context.Context, dlURL, title string) (string, error) {
	// Make sure our category exists; qBittorrent 409s when it already does.
	_, _ = q.do(ctx, "/api/v2/torrents/createCategory",
		url.Values{"category": {q.cfg.Category}, "savePath": {""}})

	if strings.HasPrefix(dlURL, "magnet:") {
		return q.addURLs(ctx, dlURL, title)
	}
	if magnet, torrent, err := q.resolve(ctx, dlURL); err == nil {
		if magnet != "" {
			return q.addURLs(ctx, magnet, title)
		}
		if len(torrent) > 0 {
			return q.addFile(ctx, torrent, title)
		}
	}
	return q.addURLs(ctx, dlURL, title)
}

// addURLs hands qBittorrent a magnet (or a URL it can fetch itself) via the
// urls field. The add endpoint doesn't return the hash, so the id is empty.
func (q *qbittorrent) addURLs(ctx context.Context, urls, title string) (string, error) {
	body, err := q.do(ctx, "/api/v2/torrents/add",
		url.Values{"urls": {urls}, "category": {q.cfg.Category}, "rename": {title}})
	if err != nil {
		// A debrid bridge can accept the magnet yet respond too slowly, tripping
		// the client timeout even though the torrent lands. Confirm via the list
		// before giving up, so the grab is still recorded.
		if q.landed(title) {
			return "", nil
		}
		return "", err
	}
	if strings.HasPrefix(string(body), "Fails") {
		return "", fmt.Errorf("qbittorrent rejected the torrent")
	}
	return "", nil
}

// landed reports whether a torrent matching title is now in our category. It
// confirms a slow add (see addURLs/addFile) actually took effect, using a fresh
// short-lived context so a stalled add request doesn't taint the check.
func (q *qbittorrent) landed(title string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	items, err := q.List(ctx)
	if err != nil {
		return false
	}
	want := normalizeTitle(title)
	for _, it := range items {
		got := normalizeTitle(it.Title)
		if want != "" && (got == want || strings.Contains(got, want)) {
			return true
		}
	}
	return false
}

// normalizeTitle lowercases a title to space-separated alphanumeric runs so
// cosmetic punctuation differences don't defeat the landed() match.
func normalizeTitle(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// resolve follows a release's download URL to what the client actually needs:
// a magnet link (indexers redirect magnet-only results to one) or the raw
// .torrent bytes. It doesn't follow redirects automatically — a redirect to a
// magnet: scheme isn't an HTTP URL the client can follow — inspecting the
// Location header instead.
func (q *qbittorrent) resolve(ctx context.Context, dlURL string) (string, []byte, error) {
	client := &http.Client{
		Timeout:       60 * time.Second,
		Jar:           q.httpc.Jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	cur := dlURL
	for i := 0; i < 5; i++ {
		if strings.HasPrefix(cur, "magnet:") {
			return cur, nil, nil
		}
		if !strings.HasPrefix(cur, "http://") && !strings.HasPrefix(cur, "https://") {
			return "", nil, fmt.Errorf("unsupported url scheme")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cur, nil)
		if err != nil {
			return "", nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", nil, err
		}
		if loc := resp.Header.Get("Location"); resp.StatusCode >= 300 && resp.StatusCode < 400 && loc != "" {
			resp.Body.Close()
			cur = loc
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
		ct := resp.Header.Get("Content-Type")
		status := resp.StatusCode
		resp.Body.Close()
		if status != http.StatusOK {
			return "", nil, fmt.Errorf("resolving torrent: HTTP %d", status)
		}
		trimmed := bytes.TrimSpace(body)
		if bytes.HasPrefix(trimmed, []byte("magnet:")) {
			return string(trimmed), nil, nil
		}
		// A bencoded .torrent starts with a dictionary ('d') and names the
		// info/announce keys; some indexers omit the content-type.
		if strings.Contains(ct, "bittorrent") ||
			(len(trimmed) > 0 && trimmed[0] == 'd' &&
				(bytes.Contains(trimmed, []byte("4:info")) || bytes.Contains(trimmed, []byte("announce")))) {
			return "", body, nil
		}
		return "", nil, fmt.Errorf("response is neither a magnet nor a .torrent")
	}
	return "", nil, fmt.Errorf("too many redirects")
}

// addFile uploads .torrent bytes to qBittorrent (multipart torrents field), so
// a client that can't reach our indexer still gets the file.
func (q *qbittorrent) addFile(ctx context.Context, torrent []byte, title string) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("category", q.cfg.Category)
	if title != "" {
		_ = mw.WriteField("rename", title)
	}
	fw, err := mw.CreateFormFile("torrents", torrentFilename(title))
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(torrent); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	payload := buf.Bytes()
	ct := mw.FormDataContentType()

	attempt := func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			q.base()+"/api/v2/torrents/add", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", ct)
		return q.httpc.Do(req)
	}
	resp, err := attempt()
	if err != nil {
		// Slow bridge: the upload may have landed despite the timeout.
		if q.landed(title) {
			return "", nil
		}
		return "", fmt.Errorf("qbittorrent: %w", err)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		if err := q.login(ctx); err != nil {
			return "", err
		}
		if resp, err = attempt(); err != nil {
			if q.landed(title) {
				return "", nil
			}
			return "", fmt.Errorf("qbittorrent: %w", err)
		}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("qbittorrent: /api/v2/torrents/add: HTTP %d: %.100s", resp.StatusCode, body)
	}
	if strings.HasPrefix(string(body), "Fails") {
		return "", fmt.Errorf("qbittorrent rejected the torrent")
	}
	return "", nil
}

// torrentFilename makes a filesystem-safe "<title>.torrent" for the upload.
func torrentFilename(title string) string {
	safe := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\n', '\r':
			return '_'
		}
		return r
	}, title)
	if safe == "" {
		safe = "download"
	}
	return safe + ".torrent"
}

func (q *qbittorrent) List(ctx context.Context) ([]Item, error) {
	// The client timeout is generous for slow adds; listing should stay snappy
	// so a hung bridge can't stall the periodic import loop.
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	body, err := q.do(ctx, "/api/v2/torrents/info?category="+url.QueryEscape(q.cfg.Category), nil)
	if err != nil {
		return nil, err
	}
	var torrents []struct {
		Hash        string  `json:"hash"`
		Name        string  `json:"name"`
		State       string  `json:"state"`
		Progress    float64 `json:"progress"`
		ContentPath string  `json:"content_path"`
		SavePath    string  `json:"save_path"`
	}
	if err := json.Unmarshal(body, &torrents); err != nil {
		return nil, fmt.Errorf("qbittorrent: decoding torrent list: %w", err)
	}

	items := make([]Item, 0, len(torrents))
	for _, t := range torrents {
		item := Item{
			Client:   q.cfg.Name,
			ConfigID: q.cfg.ID,
			ID:       t.Hash,
			Title:    t.Name,
			Progress: t.Progress,
			Path:     t.ContentPath,
		}
		if item.Path == "" {
			item.Path = t.SavePath
		}
		item.Status = qbitStatus(t.State, t.Progress)
		items = append(items, item)
	}
	return items, nil
}

// qbitStatus normalizes qBittorrent's many states. Anything actively seeding
// or finished counts as completed — the file is on disk. A finished torrent
// that qBittorrent has paused/stopped (seed ratio or time goal reached, or
// paused by hand) reports "seeded": done seeding, safe to remove.
func qbitStatus(state string, progress float64) string {
	switch state {
	case "missingFiles":
		return "failed"
	case "error":
		// A finished torrent the client flags as errored still has its data on
		// disk — some debrid bridges mark a cached torrent "error" once it's
		// done downloading. Import it; only an error before completion is a
		// real failure.
		if progress >= 1 {
			return "completed"
		}
		return "failed"
	case "pausedDL", "stoppedDL":
		return "paused"
	case "queuedDL", "allocating", "metaDL", "checkingDL":
		return "queued"
	case "pausedUP", "stoppedUP":
		return "seeded"
	}
	if strings.HasSuffix(state, "UP") || progress >= 1 {
		return "completed"
	}
	return "downloading"
}

func (q *qbittorrent) Remove(ctx context.Context, id string, deleteData bool) error {
	del := "false"
	if deleteData {
		del = "true"
	}
	_, err := q.do(ctx, "/api/v2/torrents/delete",
		url.Values{"hashes": {id}, "deleteFiles": {del}})
	return err
}
