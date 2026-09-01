package download

import (
	"database/sql"
	"errors"
	"strings"
)

// Grab statuses.
const (
	GrabStatusGrabbed  = "grabbed"
	GrabStatusImported = "imported"
	GrabStatusFailed   = "failed"
)

// ErrGrabInFlight is returned by ClaimGrab when a grab for the same book and
// media type is already pending — the compare-and-swap that stops a
// double-clicked, or concurrently-swept, grab from reaching the client twice.
var ErrGrabInFlight = errors.New("a grab for this book is already in flight")

// GrabRecord tracks one release sent to a download client and its outcome.
type GrabRecord struct {
	ID             int64  `json:"id"`
	BookID         int64  `json:"bookId,omitempty"`
	ClientConfigID int64  `json:"clientConfigId,omitempty"`
	ClientItemID   string `json:"clientItemId,omitempty"`
	Title          string `json:"title"`
	GUID           string `json:"guid,omitempty"` // release guid, for the blocklist
	Protocol       string `json:"protocol"`
	MediaType      string `json:"mediaType"`
	Status         string `json:"status"`
	Message        string `json:"message,omitempty"`
	GrabbedAt      string `json:"grabbedAt"`
	CompletedAt    string `json:"completedAt,omitempty"`
}

const grabCols = `id, COALESCE(book_id, 0), COALESCE(client_config_id, 0), client_item_id,
	title, guid, protocol, media_type, status, message, grabbed_at, COALESCE(completed_at, '')`

func scanGrab(row interface{ Scan(...any) error }) (*GrabRecord, error) {
	var g GrabRecord
	err := row.Scan(&g.ID, &g.BookID, &g.ClientConfigID, &g.ClientItemID,
		&g.Title, &g.GUID, &g.Protocol, &g.MediaType, &g.Status, &g.Message, &g.GrabbedAt, &g.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// AddGrab records a release sent to a client.
func (s *Store) AddGrab(g *GrabRecord) error {
	bookID := sql.NullInt64{Int64: g.BookID, Valid: g.BookID > 0}
	configID := sql.NullInt64{Int64: g.ClientConfigID, Valid: g.ClientConfigID > 0}
	if g.Status == "" {
		g.Status = GrabStatusGrabbed
	}
	if g.MediaType == "" {
		g.MediaType = "ebook"
	}
	return s.db.QueryRow(`
		INSERT INTO grabs (book_id, client_config_id, client_item_id, title, guid, protocol, media_type, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id, grabbed_at`,
		bookID, configID, g.ClientItemID, g.Title, g.GUID, g.Protocol, g.MediaType, g.Status,
	).Scan(&g.ID, &g.GrabbedAt)
}

// ClaimGrab atomically records a pending grab for a book, but only when no
// other grab for the same book and media type is already pending. It is the
// compare-and-swap that must run BEFORE the client network call (see
// Service.GrabRelease): two grabs racing for the same book — a double-click,
// or an autosearch sweep overlapping a manual grab — would otherwise both
// reach the client, since autosearch's own pending-book pre-filter isn't
// atomic with the grab. The loser gets ErrGrabInFlight. The row carries no
// client details yet: FinishGrabClaim fills them in once the client accepts,
// or DeleteGrab releases the claim if it doesn't.
func (s *Store) ClaimGrab(g *GrabRecord) error {
	if g.MediaType == "" {
		g.MediaType = "ebook"
	}
	bookID := sql.NullInt64{Int64: g.BookID, Valid: g.BookID > 0}
	// One atomic statement: INSERT ... SELECT ... WHERE NOT EXISTS runs under
	// SQLite's single-writer lock, so two concurrent claims can't both pass
	// the existence check — the second inserts zero rows, so RETURNING yields
	// sql.ErrNoRows.
	err := s.db.QueryRow(`
		INSERT INTO grabs (book_id, client_config_id, client_item_id, title, guid, protocol, media_type, status)
		SELECT ?, NULL, '', ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM grabs WHERE book_id = ? AND media_type = ? AND status = ?)
		RETURNING id, grabbed_at`,
		bookID, g.Title, g.GUID, g.Protocol, g.MediaType, GrabStatusGrabbed,
		g.BookID, g.MediaType, GrabStatusGrabbed,
	).Scan(&g.ID, &g.GrabbedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrGrabInFlight
	}
	if err != nil {
		return err
	}
	g.Status = GrabStatusGrabbed
	return nil
}

// FinishGrabClaim fills in the client details on a grab claimed by ClaimGrab,
// once the download client has accepted the release.
func (s *Store) FinishGrabClaim(id, configID int64, clientItemID string) error {
	cfg := sql.NullInt64{Int64: configID, Valid: configID > 0}
	res, err := s.db.Exec(
		`UPDATE grabs SET client_config_id = ?, client_item_id = ? WHERE id = ?`,
		cfg, clientItemID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteGrab removes a grab row outright — used to release a ClaimGrab claim
// when the client never accepted the release, leaving no phantom "grabbed" row
// behind so the book is immediately grabbable again.
func (s *Store) DeleteGrab(id int64) error {
	_, err := s.db.Exec(`DELETE FROM grabs WHERE id = ?`, id)
	return err
}

// ClearHistory deletes resolved grab history (imported or failed), leaving
// pending ("grabbed") records untouched so an in-flight download is never
// forgotten. Returns the number of rows removed.
func (s *Store) ClearHistory() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM grabs WHERE status != ?`, GrabStatusGrabbed)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// GrabHistory returns grab history newest first with paging and an optional
// case-insensitive title filter; the second return is the total matching
// count so the UI can page through a busy instance's full history.
func (s *Store) GrabHistory(search string, limit, offset int) ([]GrabRecord, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	where := ""
	args := []any{}
	if search != "" {
		where = ` WHERE title LIKE ? ESCAPE '\' COLLATE NOCASE`
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(search)
		args = append(args, "%"+esc+"%")
	}

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM grabs`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(
		`SELECT `+grabCols+` FROM grabs`+where+` ORDER BY grabbed_at DESC, id DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	grabs := []GrabRecord{}
	for rows.Next() {
		g, err := scanGrab(rows)
		if err != nil {
			return nil, 0, err
		}
		grabs = append(grabs, *g)
	}
	return grabs, total, rows.Err()
}

// ListGrabs returns grab history, optionally filtered by status, newest first.
func (s *Store) ListGrabs(status string) ([]GrabRecord, error) {
	query := `SELECT ` + grabCols + ` FROM grabs`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY grabbed_at DESC, id DESC LIMIT 200`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grabs := []GrabRecord{}
	for rows.Next() {
		g, err := scanGrab(rows)
		if err != nil {
			return nil, err
		}
		grabs = append(grabs, *g)
	}
	return grabs, rows.Err()
}

// ResolveGrab marks a grab imported or failed.
func (s *Store) ResolveGrab(id int64, status, message string) error {
	res, err := s.db.Exec(`
		UPDATE grabs SET status = ?, message = ?, completed_at = datetime('now')
		WHERE id = ?`, status, message, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
