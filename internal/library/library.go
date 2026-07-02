// Package library holds Quillarr's library domain model — authors, series,
// books, and editions — and the SQLite persistence layer for it. Types carry
// JSON tags so API handlers can serialize them directly, *arr-style.
package library

// Edition formats. "physical" editions are tracked for completeness but are
// never grabbed; ebook is the Phase 1 focus, audiobook lands in Phase 3.
const (
	FormatEbook     = "ebook"
	FormatAudiobook = "audiobook"
	FormatPhysical  = "physical"
	FormatUnknown   = "unknown"
)

type Author struct {
	ID          int64  `json:"id"`
	Source      string `json:"metadataSource"`
	ForeignID   string `json:"foreignAuthorId"`
	Name        string `json:"name"`
	SortName    string `json:"sortName"`
	Description string `json:"description"`
	ImageURL    string `json:"imageUrl"`
	Monitored   bool   `json:"monitored"`
	AddedAt     string `json:"addedAt"`
	UpdatedAt   string `json:"updatedAt"`
	// Populated on detail endpoints, not by ListAuthors.
	Books []Book `json:"books,omitempty"`
}

type Book struct {
	ID          int64   `json:"id"`
	AuthorID    int64   `json:"authorId"`
	Source      string  `json:"metadataSource"`
	ForeignID   string  `json:"foreignBookId"`
	Title       string  `json:"title"`
	SortTitle   string  `json:"sortTitle"`
	Description string  `json:"description"`
	ReleaseDate string  `json:"releaseDate"`
	Rating      float64 `json:"rating"`
	CoverURL    string  `json:"coverUrl"`
	Monitored   bool    `json:"monitored"`
	AddedAt     string  `json:"addedAt"`
	UpdatedAt   string  `json:"updatedAt"`
	// Populated on detail endpoints.
	Editions []Edition    `json:"editions,omitempty"`
	Series   []SeriesLink `json:"series,omitempty"`
}

type Edition struct {
	ID          int64  `json:"id"`
	BookID      int64  `json:"bookId"`
	Source      string `json:"metadataSource"`
	ForeignID   string `json:"foreignEditionId"`
	Title       string `json:"title"`
	ISBN13      string `json:"isbn13"`
	ASIN        string `json:"asin"`
	Format      string `json:"format"`
	Publisher   string `json:"publisher"`
	Language    string `json:"language"`
	ReleaseDate string `json:"releaseDate"`
	CoverURL    string `json:"coverUrl"`
	Monitored   bool   `json:"monitored"`
}

type Series struct {
	ID          int64  `json:"id"`
	Source      string `json:"metadataSource"`
	ForeignID   string `json:"foreignSeriesId"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// SeriesLink is a book's membership in a series ("book 3 of Discworld").
type SeriesLink struct {
	SeriesID int64   `json:"seriesId"`
	Title    string  `json:"title"`
	Position float64 `json:"position"`
}
