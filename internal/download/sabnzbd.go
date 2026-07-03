package download

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SABnzbd's API is a single endpoint: GET /api?mode=...&apikey=...&output=json.
// Queue and history are separate calls; LibriNode merges them into one view.
type sabnzbd struct {
	cfg   *ClientConfig
	httpc *http.Client
}

func newSABnzbd(cfg *ClientConfig) *sabnzbd {
	return &sabnzbd{cfg: cfg, httpc: &http.Client{Timeout: 30 * time.Second}}
}

func (s *sabnzbd) api(ctx context.Context, params url.Values, out any) error {
	params.Set("apikey", s.cfg.APIKey)
	params.Set("output", "json")
	endpoint := strings.TrimRight(s.cfg.Host, "/") + "/api?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := s.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("sabnzbd: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return fmt.Errorf("sabnzbd: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sabnzbd: HTTP %d: %.100s", resp.StatusCode, body)
	}
	// Errors arrive as {"status": false, "error": "..."} regardless of mode.
	var apiErr struct {
		Status *bool  `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(body, &apiErr); err == nil &&
		apiErr.Status != nil && !*apiErr.Status && apiErr.Error != "" {
		return fmt.Errorf("sabnzbd: %s", apiErr.Error)
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("sabnzbd: decoding response: %w", err)
		}
	}
	return nil
}

func (s *sabnzbd) Test(ctx context.Context) error {
	var out struct {
		Version string `json:"version"`
	}
	if err := s.api(ctx, url.Values{"mode": {"version"}}, &out); err != nil {
		return err
	}
	if out.Version == "" {
		return fmt.Errorf("sabnzbd: no version in response — wrong endpoint?")
	}
	// version works without auth on some setups; queue verifies the key.
	return s.api(ctx, url.Values{"mode": {"queue"}, "limit": {"1"}}, nil)
}

func (s *sabnzbd) Add(ctx context.Context, dlURL, title string) (string, error) {
	var out struct {
		Status bool     `json:"status"`
		NzoIDs []string `json:"nzo_ids"`
	}
	params := url.Values{
		"mode":    {"addurl"},
		"name":    {dlURL},
		"nzbname": {title},
		"cat":     {s.cfg.Category},
	}
	if err := s.api(ctx, params, &out); err != nil {
		return "", err
	}
	if !out.Status || len(out.NzoIDs) == 0 {
		return "", fmt.Errorf("sabnzbd did not accept the NZB")
	}
	return out.NzoIDs[0], nil
}

// sabSlots covers both queue and history slot shapes (numbers arrive as
// strings).
type sabSlot struct {
	NzoID       string `json:"nzo_id"`
	Filename    string `json:"filename"` // queue
	Name        string `json:"name"`     // history
	Status      string `json:"status"`
	Percentage  string `json:"percentage"`
	Storage     string `json:"storage"`
	FailMessage string `json:"fail_message"`
	Category    string `json:"category"`
}

func (s *sabnzbd) List(ctx context.Context) ([]Item, error) {
	var queue struct {
		Queue struct {
			Slots []sabSlot `json:"slots"`
		} `json:"queue"`
	}
	if err := s.api(ctx, url.Values{"mode": {"queue"}, "category": {s.cfg.Category}}, &queue); err != nil {
		return nil, err
	}
	var history struct {
		History struct {
			Slots []sabSlot `json:"slots"`
		} `json:"history"`
	}
	if err := s.api(ctx, url.Values{"mode": {"history"}, "limit": {"100"}, "category": {s.cfg.Category}}, &history); err != nil {
		return nil, err
	}

	items := []Item{}
	for _, slot := range queue.Queue.Slots {
		item := Item{Client: s.cfg.Name, ID: slot.NzoID, Title: slot.Filename}
		if pct, err := strconv.ParseFloat(slot.Percentage, 64); err == nil {
			item.Progress = pct / 100
		}
		switch strings.ToLower(slot.Status) {
		case "paused":
			item.Status = "paused"
		case "queued", "grabbing":
			item.Status = "queued"
		default:
			item.Status = "downloading"
		}
		items = append(items, item)
	}
	for _, slot := range history.History.Slots {
		item := Item{Client: s.cfg.Name, ID: slot.NzoID, Title: slot.Name, Path: slot.Storage}
		switch strings.ToLower(slot.Status) {
		case "completed":
			item.Status = "completed"
			item.Progress = 1
		case "failed":
			item.Status = "failed"
		default:
			// Verifying/Repairing/Extracting — post-processing counts as
			// still downloading.
			item.Status = "downloading"
			item.Progress = 1
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *sabnzbd) Remove(ctx context.Context, id string, deleteData bool) error {
	// The id lives in either the queue or the history; delete is idempotent
	// so hit both.
	if err := s.api(ctx, url.Values{"mode": {"queue"}, "name": {"delete"}, "value": {id}}, nil); err != nil {
		return err
	}
	params := url.Values{"mode": {"history"}, "name": {"delete"}, "value": {id}}
	if deleteData {
		params.Set("del_files", "1")
	}
	return s.api(ctx, params, nil)
}
