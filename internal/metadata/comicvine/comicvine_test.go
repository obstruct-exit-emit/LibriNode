package comicvine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/librinode/librinode/internal/metadata"
)

const volumeJSON = `{
	"id": 18166, "name": "The Walking Dead",
	"description": "<p>Zombies.</p>", "start_year": "2003",
	"count_of_issues": 193,
	"image": {"medium_url": "https://cv/twd.jpg"},
	"person_credits": [{"name": "Robert Kirkman", "role": "writer"}],
	"issues": [
		{"id": 111, "issue_number": "1", "name": "Days Gone Bye"},
		{"id": 112, "issue_number": "2", "name": ""},
		{"id": 113, "issue_number": "1.AU", "name": "Weird Special"}
	]
}`

func mockComicVine(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "LibriNode" {
			http.Error(w, "blocked", http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("api_key") != "cv-key" {
			w.Write([]byte(`{"status_code": 100, "error": "Invalid API Key"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/search"):
			w.Write([]byte(`{"status_code": 1, "error": "OK", "results": [` + volumeJSON + `]}`))
		case strings.HasPrefix(r.URL.Path, "/volume/4050-18166"):
			w.Write([]byte(`{"status_code": 1, "error": "OK", "results": ` + volumeJSON + `}`))
		default:
			w.Write([]byte(`{"status_code": 101, "error": "Object Not Found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return New("cv-key", WithEndpoint(srv.URL))
}

func TestSearchSeries(t *testing.T) {
	c := mockComicVine(t)
	results, err := c.SearchSeries(context.Background(), "walking dead")
	if err != nil {
		t.Fatalf("SearchSeries: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v", results)
	}
	r := results[0]
	if r.ForeignID != "18166" || r.Title != "The Walking Dead" || r.AuthorName != "Robert Kirkman" {
		t.Errorf("result = %+v", r)
	}
	if r.Year != 2003 || r.IssueCount != 193 || r.Description != "Zombies." {
		t.Errorf("fields = %+v", r)
	}
}

func TestGetSeries(t *testing.T) {
	c := mockComicVine(t)
	s, err := c.GetSeries(context.Background(), "18166")
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}
	// Oddly-numbered specials are skipped.
	if len(s.Issues) != 2 {
		t.Fatalf("issues = %+v", s.Issues)
	}
	if s.Issues[0].Number != 1 || s.Issues[0].Title != "Days Gone Bye" {
		t.Errorf("issue 1 = %+v", s.Issues[0])
	}

	if _, err := c.GetSeries(context.Background(), "999"); !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("missing: err = %v, want ErrNotFound", err)
	}
}

func TestBadKey(t *testing.T) {
	c := mockComicVine(t)
	c.apiKey = "wrong"
	if _, err := c.SearchSeries(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "Invalid API Key") {
		t.Errorf("err = %v, want invalid key error", err)
	}
}
