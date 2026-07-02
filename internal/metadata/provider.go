// Package metadata defines the pluggable metadata-provider interface and the
// provider-neutral types it returns. Hardcover is the first implementation
// (books/audiobooks); manga and comic providers slot in behind the same
// interface in Phase 4.
package metadata

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a foreign id does not exist at the provider.
var ErrNotFound = errors.New("metadata: not found")

// ErrNotConfigured is returned by handlers when no provider is set up
// (e.g. no Hardcover API token yet).
var ErrNotConfigured = errors.New("metadata: no provider configured")

// Provider is a remote metadata source. Foreign ids are provider-scoped
// strings; Name() is stored alongside them as metadata_source.
type Provider interface {
	// Name is the stable identifier persisted in metadata_source columns.
	Name() string
	// SearchAuthors returns authors matching a free-text query.
	SearchAuthors(ctx context.Context, query string) ([]Author, error)
	// SearchBooks returns books matching a free-text query.
	SearchBooks(ctx context.Context, query string) ([]Book, error)
	// GetAuthor returns one author with their Books populated
	// (books carry series links but not editions).
	GetAuthor(ctx context.Context, foreignID string) (*Author, error)
	// GetBook returns one book with Editions and Series populated.
	GetBook(ctx context.Context, foreignID string) (*Book, error)
}

type Author struct {
	ForeignID   string `json:"foreignAuthorId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ImageURL    string `json:"imageUrl"`
	BookCount   int    `json:"bookCount,omitempty"`
	Books       []Book `json:"books,omitempty"`
}

type Book struct {
	ForeignID       string       `json:"foreignBookId"`
	Title           string       `json:"title"`
	Description     string       `json:"description"`
	ReleaseDate     string       `json:"releaseDate"`
	Rating          float64      `json:"rating"`
	CoverURL        string       `json:"coverUrl"`
	AuthorForeignID string       `json:"foreignAuthorId"`
	AuthorName      string       `json:"authorName"`
	Series          []SeriesLink `json:"series,omitempty"`
	Editions        []Edition    `json:"editions,omitempty"`
}

type SeriesLink struct {
	ForeignID   string  `json:"foreignSeriesId"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	Position    float64 `json:"position"`
}

// Edition formats use the same strings as the library package:
// ebook, audiobook, physical, unknown.
type Edition struct {
	ForeignID   string `json:"foreignEditionId"`
	Title       string `json:"title"`
	ISBN13      string `json:"isbn13"`
	ASIN        string `json:"asin"`
	Format      string `json:"format"`
	Publisher   string `json:"publisher"`
	Language    string `json:"language"`
	ReleaseDate string `json:"releaseDate"`
	CoverURL    string `json:"coverUrl"`
}
