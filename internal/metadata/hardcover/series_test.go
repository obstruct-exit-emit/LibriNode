package hardcover

import (
	"context"
	"testing"
)

func TestSeriesSearchAndGet(t *testing.T) {
	c := mockAPI(t, map[string]string{
		"Search": `{"data":{"search":{"results":{"hits":[
			{"document":{"id":"7310","name":"Death Note","books_count":12}},
			{"document":{"id":5637,"name":"Death Note: Black Edition","books_count":6}}
		]}}}}`,
		"Series": `{"data":{"series":[{
			"id":7310,"name":"Death Note","description":"Light finds a notebook.","books_count":3,
			"author":{"name":"Tsugumi Ohba"},
			"book_series":[
				{"position":1,"book":{"id":100,"title":"Death Note, Vol. 1","description":"Boredom","release_date":"2005-10-04","cached_image":{"url":"https://img/1.jpg"}}},
				{"position":2,"book":{"id":101,"title":"Death Note, Vol. 2","description":"Confluence","release_date":"2005-12-06","cached_image":{"url":"https://img/2.jpg"}}},
				{"position":3,"book":{"id":102,"title":"Death Note, Vol. 3"}}
			]
		}]}}`,
	})
	sc := &SeriesClient{c}

	if sc.MediaType() != "manga" || sc.Name() != "hardcover" {
		t.Fatalf("MediaType/Name = %s/%s", sc.MediaType(), sc.Name())
	}

	results, err := sc.SearchSeries(context.Background(), "death note")
	if err != nil {
		t.Fatalf("SearchSeries: %v", err)
	}
	if len(results) != 2 || results[0].ForeignID != "7310" || results[0].IssueCount != 12 {
		t.Fatalf("search results = %+v", results)
	}

	s, err := sc.GetSeries(context.Background(), "7310")
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}
	if s.Title != "Death Note" || s.AuthorName != "Tsugumi Ohba" || s.IssueCount != 3 {
		t.Fatalf("series = %+v", s)
	}
	if s.CoverURL != "https://img/1.jpg" {
		t.Errorf("cover = %q, want the first volume's image", s.CoverURL)
	}
	if len(s.Issues) != 3 {
		t.Fatalf("issues = %d, want 3", len(s.Issues))
	}
	// Clean positions are used as volume numbers.
	for i, want := range []float64{1, 2, 3} {
		if s.Issues[i].Number != want {
			t.Errorf("issue %d number = %v, want %v", i, s.Issues[i].Number, want)
		}
	}
	if s.Issues[0].ForeignID != "100" || s.Issues[0].Title != "Death Note, Vol. 1" {
		t.Errorf("issue 0 = %+v", s.Issues[0])
	}
	// Per-volume description and cover come through.
	if s.Issues[0].Description != "Boredom" || s.Issues[0].CoverURL != "https://img/1.jpg" {
		t.Errorf("issue 0 description/cover = %q / %q", s.Issues[0].Description, s.Issues[0].CoverURL)
	}
	if s.Issues[1].Description != "Confluence" {
		t.Errorf("issue 1 description = %q", s.Issues[1].Description)
	}
}

// Hardcover's manga series often store every book at position 0; volume
// numbers must fall back to sequential order.
func TestSeriesMessyPositionsFallBackToSequential(t *testing.T) {
	c := mockAPI(t, map[string]string{
		"Series": `{"data":{"series":[{
			"id":7310,"name":"Death Note","books_count":3,
			"book_series":[
				{"position":0,"book":{"id":100,"title":"A"}},
				{"position":0,"book":{"id":101,"title":"B"}},
				{"position":0,"book":{"id":102,"title":"C"}}
			]
		}]}}`,
	})
	sc := &SeriesClient{c}

	s, err := sc.GetSeries(context.Background(), "7310")
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}
	for i, want := range []float64{1, 2, 3} {
		if s.Issues[i].Number != want {
			t.Fatalf("issue %d number = %v, want sequential %v", i, s.Issues[i].Number, want)
		}
	}
}
