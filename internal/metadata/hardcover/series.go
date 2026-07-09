package hardcover

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/librinode/librinode/internal/metadata"
)

// SeriesClient adapts the Hardcover book API to metadata.SeriesProvider for
// manga: a Hardcover *series* is the manga, and the books linked to it via
// book_series are its volumes. Hardcover's manga series are messier than
// AniList's (spin-offs mixed in, positions often 0), so volume numbers fall
// back to sequential order when the positions aren't a clean 1..N sequence.
type SeriesClient struct {
	*Client
}

// SeriesFactory builds the Hardcover manga series provider; it shares the
// book provider's token.
func SeriesFactory(s metadata.Settings) (metadata.SeriesProvider, error) {
	if s.Token == "" {
		return nil, metadata.ErrNotConfigured
	}
	return &SeriesClient{New(s.Token)}, nil
}

func (sc *SeriesClient) MediaType() string { return "manga" }

func (sc *SeriesClient) SearchSeries(ctx context.Context, query string) ([]metadata.SeriesResult, error) {
	docs, err := sc.search(ctx, query, "Series")
	if err != nil {
		return nil, err
	}
	results := []metadata.SeriesResult{}
	for _, doc := range docs {
		var d struct {
			ID         flexID `json:"id"`
			Name       string `json:"name"`
			BooksCount int    `json:"books_count"`
		}
		if err := json.Unmarshal(doc, &d); err != nil || d.ID == "" {
			continue
		}
		results = append(results, metadata.SeriesResult{
			ForeignID:  string(d.ID),
			Title:      d.Name,
			IssueCount: d.BooksCount,
		})
	}
	return results, nil
}

const seriesQuery = `query Series($id: Int!) {
  series(where: {id: {_eq: $id}}, limit: 1) {
    id
    name
    description
    books_count
    author { name }
    book_series(order_by: {position: asc_nulls_last}) {
      position
      book { id title description release_date cached_image }
    }
  }
}`

func (sc *SeriesClient) GetSeries(ctx context.Context, foreignID string) (*metadata.SeriesResult, error) {
	id, err := strconv.Atoi(foreignID)
	if err != nil {
		return nil, fmt.Errorf("hardcover: invalid series id %q: %w", foreignID, metadata.ErrNotFound)
	}

	var env struct {
		Series []struct {
			ID          json.Number `json:"id"`
			Name        string      `json:"name"`
			Description string      `json:"description"`
			BooksCount  int         `json:"books_count"`
			Author      *struct {
				Name string `json:"name"`
			} `json:"author"`
			BookSeries []struct {
				Position float64 `json:"position"`
				Book     *struct {
					ID          json.Number     `json:"id"`
					Title       string          `json:"title"`
					Description string          `json:"description"`
					ReleaseDate string          `json:"release_date"`
					CachedImage json.RawMessage `json:"cached_image"`
				} `json:"book"`
			} `json:"book_series"`
		} `json:"series"`
	}
	if err := sc.do(ctx, seriesQuery, map[string]any{"id": id}, &env); err != nil {
		return nil, err
	}
	if len(env.Series) == 0 {
		return nil, metadata.ErrNotFound
	}
	s := env.Series[0]

	// Keep only entries with a book, in the order Hardcover returned them.
	type entry struct {
		pos  float64
		book struct {
			ID          json.Number
			Title       string
			Description string
			ReleaseDate string
			CachedImage json.RawMessage
		}
	}
	entries := []entry{}
	for _, bs := range s.BookSeries {
		if bs.Book == nil || bs.Book.ID.String() == "" {
			continue
		}
		var e entry
		e.pos = bs.Position
		e.book.ID = bs.Book.ID
		e.book.Title = bs.Book.Title
		e.book.Description = bs.Book.Description
		e.book.ReleaseDate = bs.Book.ReleaseDate
		e.book.CachedImage = bs.Book.CachedImage
		entries = append(entries, e)
	}

	// Use the provider positions only when they form clean, distinct volume
	// numbers; otherwise number sequentially by order.
	clean := len(entries) > 0
	seen := map[float64]bool{}
	for _, e := range entries {
		if e.pos < 1 || seen[e.pos] {
			clean = false
			break
		}
		seen[e.pos] = true
	}

	result := &metadata.SeriesResult{
		ForeignID:   s.ID.String(),
		Title:       s.Name,
		Description: s.Description,
		IssueCount:  len(entries),
	}
	if s.Author != nil {
		result.AuthorName = s.Author.Name
	}
	for i, e := range entries {
		num := float64(i + 1)
		if clean {
			num = e.pos
		}
		cover := imageURL(e.book.CachedImage)
		if i == 0 {
			result.CoverURL = cover
		}
		result.Issues = append(result.Issues, metadata.Issue{
			ForeignID:   e.book.ID.String(),
			Number:      num,
			Title:       e.book.Title,
			Description: e.book.Description,
			CoverURL:    cover,
			ReleaseDate: e.book.ReleaseDate,
		})
	}
	return result, nil
}
