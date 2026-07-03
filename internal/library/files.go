package library

import (
	"database/sql"
	"errors"
)

// RootFolder mirrors the root_folders table (managed by the rootfolder API);
// the scanner needs them to know where to look.
type RootFolder struct {
	ID        int64  `json:"id"`
	MediaType string `json:"mediaType"`
	Path      string `json:"path"`
}

// BookFile is a file found on disk by a library scan. BookID is nil-like (0)
// when the scanner could not match it to a library book.
type BookFile struct {
	ID           int64  `json:"id"`
	RootFolderID int64  `json:"rootFolderId"`
	BookID       int64  `json:"bookId,omitempty"`
	Path         string `json:"path"`
	Size         int64  `json:"size"`
	Format       string `json:"format"`
	ModifiedAt   string `json:"modifiedAt"`
	AddedAt      string `json:"addedAt"`
}

func (s *Store) ListRootFolders() ([]RootFolder, error) {
	rows, err := s.db.Query(`SELECT id, media_type, path FROM root_folders ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	folders := []RootFolder{}
	for rows.Next() {
		var f RootFolder
		if err := rows.Scan(&f.ID, &f.MediaType, &f.Path); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

// UpsertBookFile inserts or refreshes a file by path. The scanner owns the
// book match, so book_id is updated on re-scan (a file can gain a match after
// its book is added to the library).
func (s *Store) UpsertBookFile(f *BookFile) error {
	bookID := sql.NullInt64{Int64: f.BookID, Valid: f.BookID > 0}
	return s.db.QueryRow(`
		INSERT INTO book_files (root_folder_id, book_id, path, size, format, modified_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (path) DO UPDATE SET
			root_folder_id = excluded.root_folder_id,
			book_id = excluded.book_id,
			size = excluded.size,
			format = excluded.format,
			modified_at = excluded.modified_at
		RETURNING id`,
		f.RootFolderID, bookID, f.Path, f.Size, f.Format, f.ModifiedAt,
	).Scan(&f.ID)
}

const bookFileCols = `id, root_folder_id, COALESCE(book_id, 0), path, size, format, modified_at, added_at`

func scanBookFile(row interface{ Scan(...any) error }) (*BookFile, error) {
	var f BookFile
	err := row.Scan(&f.ID, &f.RootFolderID, &f.BookID, &f.Path, &f.Size, &f.Format, &f.ModifiedAt, &f.AddedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (s *Store) listBookFiles(where string, args ...any) ([]BookFile, error) {
	rows, err := s.db.Query(`SELECT `+bookFileCols+` FROM book_files `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := []BookFile{}
	for rows.Next() {
		f, err := scanBookFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, *f)
	}
	return files, rows.Err()
}

func (s *Store) GetBookFile(id int64) (*BookFile, error) {
	return scanBookFile(s.db.QueryRow(`SELECT `+bookFileCols+` FROM book_files WHERE id = ?`, id))
}

func (s *Store) ListBookFiles(bookID int64) ([]BookFile, error) {
	return s.listBookFiles(`WHERE book_id = ? ORDER BY path`, bookID)
}

func (s *Store) ListMatchedBookFiles() ([]BookFile, error) {
	return s.listBookFiles(`WHERE book_id IS NOT NULL ORDER BY path`)
}

func (s *Store) ListUnmatchedBookFiles() ([]BookFile, error) {
	return s.listBookFiles(`WHERE book_id IS NULL ORDER BY path`)
}

// SetBookFileBook assigns (or clears, with 0) a file's book — the manual
// import action.
func (s *Store) SetBookFileBook(id, bookID int64) error {
	b := sql.NullInt64{Int64: bookID, Valid: bookID > 0}
	res, err := s.db.Exec(`UPDATE book_files SET book_id = ? WHERE id = ?`, b, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetBookFilePath records a file's new location after a rename/move.
func (s *Store) SetBookFilePath(id int64, path string) error {
	res, err := s.db.Exec(`UPDATE book_files SET path = ? WHERE id = ?`, path, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// BookFilePathsUnderRoot returns path → id for every recorded file in a root
// folder, so the scanner can prune records whose files vanished from disk.
func (s *Store) BookFilePathsUnderRoot(rootFolderID int64) (map[string]int64, error) {
	rows, err := s.db.Query(`SELECT id, path FROM book_files WHERE root_folder_id = ?`, rootFolderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := map[string]int64{}
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		paths[path] = id
	}
	return paths, rows.Err()
}

func (s *Store) DeleteBookFile(id int64) error {
	res, err := s.db.Exec(`DELETE FROM book_files WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
