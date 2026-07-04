package library

import (
	"database/sql"
	"errors"
	"strings"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// SortName derives a "Last, First Middle" sort key from a display name.
func SortName(name string) string {
	parts := strings.Fields(name)
	if len(parts) < 2 {
		return name
	}
	last := parts[len(parts)-1]
	return last + ", " + strings.Join(parts[:len(parts)-1], " ")
}

// SortTitle strips a leading English article for sorting.
func SortTitle(title string) string {
	lower := strings.ToLower(title)
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(lower, article) && len(title) > len(article) {
			return title[len(article):]
		}
	}
	return title
}

// --- Authors ---

// UpsertAuthor inserts the author or, when (source, foreign_id) already
// exists, refreshes its metadata without touching the user-owned monitored
// flag. The author's ID is set on return.
func (s *Store) UpsertAuthor(a *Author) error {
	if a.SortName == "" {
		a.SortName = SortName(a.Name)
	}
	return s.db.QueryRow(`
		INSERT INTO authors (metadata_source, foreign_id, name, sort_name, description, image_url, monitored)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (metadata_source, foreign_id) DO UPDATE SET
			name = excluded.name,
			sort_name = excluded.sort_name,
			description = excluded.description,
			image_url = excluded.image_url,
			updated_at = datetime('now')
		RETURNING id`,
		a.Source, a.ForeignID, a.Name, a.SortName, a.Description, a.ImageURL, a.Monitored,
	).Scan(&a.ID)
}

const authorCols = `id, metadata_source, foreign_id, name, sort_name, description, image_url, monitored, added_at, updated_at`

func scanAuthor(row interface{ Scan(...any) error }) (*Author, error) {
	var a Author
	err := row.Scan(&a.ID, &a.Source, &a.ForeignID, &a.Name, &a.SortName,
		&a.Description, &a.ImageURL, &a.Monitored, &a.AddedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) GetAuthor(id int64) (*Author, error) {
	return scanAuthor(s.db.QueryRow(`SELECT `+authorCols+` FROM authors WHERE id = ?`, id))
}

func (s *Store) GetAuthorByForeignID(source, foreignID string) (*Author, error) {
	return scanAuthor(s.db.QueryRow(
		`SELECT `+authorCols+` FROM authors WHERE metadata_source = ? AND foreign_id = ?`,
		source, foreignID))
}

func (s *Store) ListAuthors() ([]Author, error) {
	rows, err := s.db.Query(`SELECT ` + authorCols + ` FROM authors ORDER BY sort_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	authors := []Author{}
	for rows.Next() {
		a, err := scanAuthor(rows)
		if err != nil {
			return nil, err
		}
		authors = append(authors, *a)
	}
	return authors, rows.Err()
}

// DeleteAuthor removes the author; books, editions, and series links cascade.
func (s *Store) DeleteAuthor(id int64) error {
	res, err := s.db.Exec(`DELETE FROM authors WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetAuthorMonitored(id int64, monitored bool) error {
	res, err := s.db.Exec(
		`UPDATE authors SET monitored = ?, updated_at = datetime('now') WHERE id = ?`, monitored, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Books ---

// UpsertBook inserts or refreshes a book by (source, foreign_id), preserving
// the monitored flag on update. The book's ID is set on return.
func (s *Store) UpsertBook(b *Book) error {
	if b.SortTitle == "" {
		b.SortTitle = SortTitle(b.Title)
	}
	return s.db.QueryRow(`
		INSERT INTO books (author_id, metadata_source, foreign_id, title, sort_title, description, release_date, rating, cover_url, monitored)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (metadata_source, foreign_id) DO UPDATE SET
			author_id = excluded.author_id,
			title = excluded.title,
			sort_title = excluded.sort_title,
			description = excluded.description,
			release_date = excluded.release_date,
			rating = excluded.rating,
			cover_url = excluded.cover_url,
			updated_at = datetime('now')
		RETURNING id`,
		b.AuthorID, b.Source, b.ForeignID, b.Title, b.SortTitle,
		b.Description, b.ReleaseDate, b.Rating, b.CoverURL, b.Monitored,
	).Scan(&b.ID)
}

const bookCols = `id, author_id, metadata_source, foreign_id, title, sort_title, description, release_date, rating, cover_url, monitored,
	EXISTS(SELECT 1 FROM book_files WHERE book_files.book_id = books.id),
	EXISTS(SELECT 1 FROM book_files WHERE book_files.book_id = books.id AND book_files.media_type = 'ebook'),
	EXISTS(SELECT 1 FROM book_files WHERE book_files.book_id = books.id AND book_files.media_type = 'audiobook'),
	added_at, updated_at`

func scanBook(row interface{ Scan(...any) error }) (*Book, error) {
	var b Book
	err := row.Scan(&b.ID, &b.AuthorID, &b.Source, &b.ForeignID, &b.Title, &b.SortTitle,
		&b.Description, &b.ReleaseDate, &b.Rating, &b.CoverURL, &b.Monitored,
		&b.HasFile, &b.HasEbookFile, &b.HasAudiobookFile, &b.AddedAt, &b.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) GetBook(id int64) (*Book, error) {
	return scanBook(s.db.QueryRow(`SELECT `+bookCols+` FROM books WHERE id = ?`, id))
}

func (s *Store) GetBookByForeignID(source, foreignID string) (*Book, error) {
	return scanBook(s.db.QueryRow(
		`SELECT `+bookCols+` FROM books WHERE metadata_source = ? AND foreign_id = ?`,
		source, foreignID))
}

// ListBooks returns all books, or only an author's books when authorID > 0.
func (s *Store) ListBooks(authorID int64) ([]Book, error) {
	query := `SELECT ` + bookCols + ` FROM books`
	args := []any{}
	if authorID > 0 {
		query += ` WHERE author_id = ?`
		args = append(args, authorID)
	}
	query += ` ORDER BY sort_title`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	books := []Book{}
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, *b)
	}
	return books, rows.Err()
}

func (s *Store) DeleteBook(id int64) error {
	res, err := s.db.Exec(`DELETE FROM books WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetBookMonitored(id int64, monitored bool) error {
	res, err := s.db.Exec(
		`UPDATE books SET monitored = ?, updated_at = datetime('now') WHERE id = ?`, monitored, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Editions ---

// UpsertEdition inserts or refreshes an edition by (source, foreign_id),
// preserving the monitored flag on update. The edition's ID is set on return.
func (s *Store) UpsertEdition(e *Edition) error {
	if e.Format == "" {
		e.Format = FormatUnknown
	}
	return s.db.QueryRow(`
		INSERT INTO editions (book_id, metadata_source, foreign_id, title, isbn13, asin, format, publisher, language, release_date, cover_url, monitored)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (metadata_source, foreign_id) DO UPDATE SET
			book_id = excluded.book_id,
			title = excluded.title,
			isbn13 = excluded.isbn13,
			asin = excluded.asin,
			format = excluded.format,
			publisher = excluded.publisher,
			language = excluded.language,
			release_date = excluded.release_date,
			cover_url = excluded.cover_url
		RETURNING id`,
		e.BookID, e.Source, e.ForeignID, e.Title, e.ISBN13, e.ASIN, e.Format,
		e.Publisher, e.Language, e.ReleaseDate, e.CoverURL, e.Monitored,
	).Scan(&e.ID)
}

const editionCols = `id, book_id, metadata_source, foreign_id, title, isbn13, asin, format, publisher, language, release_date, cover_url, monitored`

func scanEdition(row interface{ Scan(...any) error }) (*Edition, error) {
	var e Edition
	err := row.Scan(&e.ID, &e.BookID, &e.Source, &e.ForeignID, &e.Title, &e.ISBN13, &e.ASIN,
		&e.Format, &e.Publisher, &e.Language, &e.ReleaseDate, &e.CoverURL, &e.Monitored)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Store) GetEdition(id int64) (*Edition, error) {
	return scanEdition(s.db.QueryRow(`SELECT `+editionCols+` FROM editions WHERE id = ?`, id))
}

func (s *Store) ListEditions(bookID int64) ([]Edition, error) {
	rows, err := s.db.Query(
		`SELECT `+editionCols+` FROM editions WHERE book_id = ? ORDER BY format, release_date`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	editions := []Edition{}
	for rows.Next() {
		e, err := scanEdition(rows)
		if err != nil {
			return nil, err
		}
		editions = append(editions, *e)
	}
	return editions, rows.Err()
}

func (s *Store) SetEditionMonitored(id int64, monitored bool) error {
	res, err := s.db.Exec(`UPDATE editions SET monitored = ? WHERE id = ?`, monitored, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Series ---

// UpsertSeries inserts or refreshes a series by (source, foreign_id).
// The series' ID is set on return.
func (s *Store) UpsertSeries(sr *Series) error {
	return s.db.QueryRow(`
		INSERT INTO series (metadata_source, foreign_id, title, description)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (metadata_source, foreign_id) DO UPDATE SET
			title = excluded.title,
			description = excluded.description
		RETURNING id`,
		sr.Source, sr.ForeignID, sr.Title, sr.Description,
	).Scan(&sr.ID)
}

func (s *Store) LinkBookSeries(bookID, seriesID int64, position float64) error {
	_, err := s.db.Exec(`
		INSERT INTO series_books (series_id, book_id, position)
		VALUES (?, ?, ?)
		ON CONFLICT (series_id, book_id) DO UPDATE SET position = excluded.position`,
		seriesID, bookID, position)
	return err
}

func (s *Store) ListSeriesForBook(bookID int64) ([]SeriesLink, error) {
	rows, err := s.db.Query(`
		SELECT sb.series_id, s.title, sb.position
		FROM series_books sb JOIN series s ON s.id = sb.series_id
		WHERE sb.book_id = ? ORDER BY s.title`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	links := []SeriesLink{}
	for rows.Next() {
		var l SeriesLink
		if err := rows.Scan(&l.SeriesID, &l.Title, &l.Position); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, rows.Err()
}
