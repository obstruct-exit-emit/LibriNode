package download

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		cfg:   cfg,
		httpc: &http.Client{Timeout: 30 * time.Second, Jar: jar},
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

func (q *qbittorrent) Add(ctx context.Context, dlURL, title string) (string, error) {
	// Make sure our category exists; qBittorrent 409s when it already does.
	_, _ = q.do(ctx, "/api/v2/torrents/createCategory",
		url.Values{"category": {q.cfg.Category}, "savePath": {""}})

	body, err := q.do(ctx, "/api/v2/torrents/add",
		url.Values{"urls": {dlURL}, "category": {q.cfg.Category}, "rename": {title}})
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(string(body), "Fails") {
		return "", fmt.Errorf("qbittorrent rejected the torrent")
	}
	// The add endpoint doesn't return the hash.
	return "", nil
}

func (q *qbittorrent) List(ctx context.Context) ([]Item, error) {
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
	case "error", "missingFiles":
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
