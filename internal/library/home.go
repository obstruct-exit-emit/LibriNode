package library

// LibraryStatus describes one media-type library for the Plex-style UI: a
// library is active — and visible — once it's set up (has a root folder) or
// already holds content.
type LibraryStatus struct {
	MediaType string `json:"mediaType"`
	Active    bool   `json:"active"`
	Items     int    `json:"items"`
	Wanted    int    `json:"wanted"`
}

// MediaTypes in display order.
var MediaTypes = []string{"ebook", "audiobook", "manga", "comic", "magazine"}

// itemsWhere returns the SQL predicate selecting a library's items.
func itemsWhere(mediaType string) string {
	switch mediaType {
	case "ebook":
		return `books.media_type = 'book' AND books.in_ebook_library = 1`
	case "audiobook":
		return `books.media_type = 'book' AND books.in_audiobook_library = 1`
	}
	return `books.media_type = '` + mediaType + `'`
}

// wantedWhere returns the SQL predicate selecting a library's wanted items
// (monitored, missing their format's file).
func wantedWhere(mediaType string) string {
	fileClause := func(mt string) string {
		return `NOT EXISTS (SELECT 1 FROM book_files f WHERE f.book_id = books.id AND f.media_type = '` + mt + `')`
	}
	switch mediaType {
	case "ebook":
		return itemsWhere("ebook") + ` AND books.ebook_monitored = 1 AND ` + fileClause("ebook")
	case "audiobook":
		return itemsWhere("audiobook") + ` AND books.audiobook_monitored = 1 AND ` + fileClause("audiobook")
	}
	return itemsWhere(mediaType) + ` AND books.monitored = 1 AND ` + fileClause(mediaType)
}

// LibraryStatuses reports every media type's activity and counts.
func (s *Store) LibraryStatuses() ([]LibraryStatus, error) {
	roots, err := s.ListRootFolders()
	if err != nil {
		return nil, err
	}
	hasRoot := map[string]bool{}
	for _, r := range roots {
		hasRoot[r.MediaType] = true
	}

	statuses := []LibraryStatus{}
	for _, mt := range MediaTypes {
		st := LibraryStatus{MediaType: mt}
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM books WHERE ` + itemsWhere(mt)).Scan(&st.Items); err != nil {
			return nil, err
		}
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM books WHERE ` + wantedWhere(mt)).Scan(&st.Wanted); err != nil {
			return nil, err
		}
		// Magazines count series, not materialized issues, as their items.
		if mt == "magazine" {
			var seriesCount int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM series WHERE media_type = 'magazine'`).Scan(&seriesCount); err != nil {
				return nil, err
			}
			if seriesCount > st.Items {
				st.Items = seriesCount
			}
		}
		st.Active = hasRoot[mt] || st.Items > 0
		statuses = append(statuses, st)
	}
	return statuses, nil
}

// HomeItem is one entry in a Home page row.
type HomeItem struct {
	BookID   int64  `json:"bookId"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"` // author or series
	CoverURL string `json:"coverUrl,omitempty"`
	HasFile  bool   `json:"hasFile"`
}

// HomeSection is one library's block on the Home page — rows never mix
// media types.
type HomeSection struct {
	MediaType     string     `json:"mediaType"`
	Items         int        `json:"items"`
	WantedCount   int        `json:"wantedCount"`
	RecentlyAdded []HomeItem `json:"recentlyAdded"`
	Wanted        []HomeItem `json:"wanted"`
}

func (s *Store) homeItems(where, order string, limit int, mediaType string) ([]HomeItem, error) {
	fileMT := mediaType
	rows, err := s.db.Query(`
		SELECT books.id, books.title, COALESCE(a.name, ''), books.cover_url,
			EXISTS (SELECT 1 FROM book_files f WHERE f.book_id = books.id AND f.media_type = '`+fileMT+`')
		FROM books LEFT JOIN authors a ON a.id = books.author_id
		WHERE `+where+` ORDER BY `+order+` LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []HomeItem{}
	for rows.Next() {
		var it HomeItem
		if err := rows.Scan(&it.BookID, &it.Title, &it.Subtitle, &it.CoverURL, &it.HasFile); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// Home builds the per-library sections for active libraries only.
func (s *Store) Home(limit int) ([]HomeSection, error) {
	statuses, err := s.LibraryStatuses()
	if err != nil {
		return nil, err
	}
	sections := []HomeSection{}
	for _, st := range statuses {
		if !st.Active {
			continue
		}
		section := HomeSection{MediaType: st.MediaType, Items: st.Items, WantedCount: st.Wanted}
		if section.RecentlyAdded, err = s.homeItems(
			itemsWhere(st.MediaType), "books.added_at DESC, books.id DESC", limit, st.MediaType); err != nil {
			return nil, err
		}
		if section.Wanted, err = s.homeItems(
			wantedWhere(st.MediaType), "books.added_at DESC, books.id DESC", limit, st.MediaType); err != nil {
			return nil, err
		}
		sections = append(sections, section)
	}
	return sections, nil
}

// ListAuthorsInLibrary returns authors with at least one book in the given
// format library (the Ebooks/Audiobooks area's author list).
func (s *Store) ListAuthorsInLibrary(mediaType string) ([]Author, error) {
	rows, err := s.db.Query(`
		SELECT ` + authorCols + ` FROM authors
		WHERE EXISTS (SELECT 1 FROM books WHERE books.author_id = authors.id AND ` + itemsWhere(mediaType) + `)
		ORDER BY sort_name`)
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
