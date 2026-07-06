// Package download talks to download clients — qBittorrent for torrents,
// SABnzbd for usenet — behind one interface: send a release, watch its
// progress, remove it. Completed Download Handling builds on this in the
// next Phase 2 slice.
package download

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const (
	TypeQBittorrent = "qbittorrent"
	TypeSABnzbd     = "sabnzbd"

	ProtocolTorrent = "torrent"
	ProtocolUsenet  = "usenet"
)

// ErrNotFound is returned when a requested client config does not exist.
var ErrNotFound = errors.New("download client not found")

// ErrNoClient is returned when no enabled client handles a protocol.
var ErrNoClient = errors.New("no enabled download client for this protocol")

// ClientConfig is one configured download client.
type ClientConfig struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
	APIKey   string `json:"apiKey"`
	Category string `json:"category"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"`
	AddedAt  string `json:"addedAt"`
}

// Protocol reports which release protocol this client downloads.
func (c *ClientConfig) Protocol() string {
	if c.Type == TypeQBittorrent {
		return ProtocolTorrent
	}
	return ProtocolUsenet
}

// Item is one download in a client, normalized across implementations.
// Status is one of: queued, downloading, paused, completed, seeded, failed
// (seeded = finished torrent the client has stopped seeding — goal reached).
type Item struct {
	Client   string  `json:"client"`
	ConfigID int64   `json:"clientConfigId"`
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Status   string  `json:"status"`
	Progress float64 `json:"progress"` // 0-1
	Path     string  `json:"path,omitempty"`
}

// Client is the operations LibriNode needs from any download client.
type Client interface {
	// Test verifies connectivity and credentials.
	Test(ctx context.Context) error
	// Add sends a release URL for download; the returned id may be empty
	// when the client doesn't report one (qBittorrent).
	Add(ctx context.Context, url, title string) (string, error)
	// List returns LibriNode's downloads (the client's category).
	List(ctx context.Context) ([]Item, error)
	// Remove deletes a download, optionally with its data.
	Remove(ctx context.Context, id string, deleteData bool) error
}

// New builds a protocol client from a config row.
func New(cfg *ClientConfig) (Client, error) {
	switch cfg.Type {
	case TypeQBittorrent:
		return newQBittorrent(cfg), nil
	case TypeSABnzbd:
		return newSABnzbd(cfg), nil
	}
	return nil, fmt.Errorf("unknown download client type %q", cfg.Type)
}

// --- Config store ---

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

const cols = `id, name, type, host, username, password, api_key, category, enabled, priority, added_at`

func scanConfig(row interface{ Scan(...any) error }) (*ClientConfig, error) {
	var c ClientConfig
	err := row.Scan(&c.ID, &c.Name, &c.Type, &c.Host, &c.Username, &c.Password,
		&c.APIKey, &c.Category, &c.Enabled, &c.Priority, &c.AddedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) Add(c *ClientConfig) error {
	return s.db.QueryRow(`
		INSERT INTO download_clients (name, type, host, username, password, api_key, category, enabled, priority)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id, added_at`,
		c.Name, c.Type, c.Host, c.Username, c.Password, c.APIKey, c.Category, c.Enabled, c.Priority,
	).Scan(&c.ID, &c.AddedAt)
}

func (s *Store) Update(c *ClientConfig) error {
	res, err := s.db.Exec(`
		UPDATE download_clients
		SET name = ?, type = ?, host = ?, username = ?, password = ?, api_key = ?, category = ?, enabled = ?, priority = ?
		WHERE id = ?`,
		c.Name, c.Type, c.Host, c.Username, c.Password, c.APIKey, c.Category, c.Enabled, c.Priority, c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Get(id int64) (*ClientConfig, error) {
	return scanConfig(s.db.QueryRow(`SELECT `+cols+` FROM download_clients WHERE id = ?`, id))
}

func (s *Store) List() ([]ClientConfig, error) {
	rows, err := s.db.Query(`SELECT ` + cols + ` FROM download_clients ORDER BY priority, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	configs := []ClientConfig{}
	for rows.Next() {
		c, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, *c)
	}
	return configs, rows.Err()
}

func (s *Store) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM download_clients WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Service ---

// Service picks clients and aggregates across them.
type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) Store() *Store { return s.store }

// GrabResult reports where a release was sent.
type GrabResult struct {
	Client   string `json:"client"`
	ClientID int64  `json:"clientId"`
	ID       string `json:"id,omitempty"`
}

// Grab sends a release to the best enabled client for its protocol
// (lowest priority number wins).
func (s *Service) Grab(ctx context.Context, protocol, url, title string) (*GrabResult, error) {
	configs, err := s.store.List()
	if err != nil {
		return nil, err
	}
	for i := range configs {
		cfg := &configs[i]
		if !cfg.Enabled || cfg.Protocol() != protocol {
			continue
		}
		client, err := New(cfg)
		if err != nil {
			return nil, err
		}
		id, err := client.Add(ctx, url, title)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", cfg.Name, err)
		}
		return &GrabResult{Client: cfg.Name, ClientID: cfg.ID, ID: id}, nil
	}
	return nil, ErrNoClient
}

// GrabRelease sends a release to the best client for its protocol and
// records the grab (tied to a book when bookID > 0) so Completed Download
// Handling can import the result. Used by both the grab endpoint and
// automatic search.
func (s *Service) GrabRelease(ctx context.Context, protocol, url, title, guid string, bookID int64, mediaType string) (*GrabResult, *GrabRecord, error) {
	result, err := s.Grab(ctx, protocol, url, title)
	if err != nil {
		return nil, nil, err
	}
	grab := &GrabRecord{
		BookID:         bookID,
		ClientConfigID: result.ClientID,
		ClientItemID:   result.ID,
		Title:          title,
		GUID:           guid,
		Protocol:       protocol,
		MediaType:      mediaType,
	}
	if err := s.store.AddGrab(grab); err != nil {
		return result, nil, fmt.Errorf("recording grab: %w", err)
	}
	return result, grab, nil
}

// Remove deletes an item from the client identified by its config id.
func (s *Service) Remove(ctx context.Context, configID int64, itemID string, deleteData bool) error {
	cfg, err := s.store.Get(configID)
	if err != nil {
		return err
	}
	client, err := New(cfg)
	if err != nil {
		return err
	}
	return client.Remove(ctx, itemID, deleteData)
}

// Queue aggregates the download queues of all enabled clients. Client
// failures come back as messages, not errors, so one dead client doesn't
// blank the whole view.
func (s *Service) Queue(ctx context.Context) ([]Item, []string, error) {
	configs, err := s.store.List()
	if err != nil {
		return nil, nil, err
	}

	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		items = []Item{}
		errs  = []string{}
	)
	for i := range configs {
		cfg := configs[i]
		if !cfg.Enabled {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			client, err := New(&cfg)
			if err != nil {
				return
			}
			found, err := client.List(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", cfg.Name, err))
				return
			}
			items = append(items, found...)
		}()
	}
	wg.Wait()

	sort.SliceStable(items, func(a, b int) bool {
		if items[a].Status != items[b].Status {
			return items[a].Status < items[b].Status
		}
		return strings.ToLower(items[a].Title) < strings.ToLower(items[b].Title)
	})
	sort.Strings(errs)
	return items, errs, nil
}
