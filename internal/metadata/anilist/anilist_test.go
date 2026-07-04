package anilist

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/librinode/librinode/internal/metadata"
)

const mediaJSON = `{
	"id": 30002, "title": {"english": "Berserk", "romaji": "Berserk"},
	"description": "<p>Guts, a former mercenary.</p>",
	"volumes": 41, "startDate": {"year": 1989},
	"coverImage": {"large": "https://img.anilist/berserk.jpg"},
	"staff": {"edges": [
		{"role": "Story & Art", "node": {"name": {"full": "Kentarou Miura"}}},
		{"role": "Assistant", "node": {"name": {"full": "Someone Else"}}}
	]}
}`

func mockAniList(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, ok := req.Variables["search"]; ok {
			w.Write([]byte(`{"data": {"Page": {"media": [` + mediaJSON + `]}}}`))
			return
		}
		if id, ok := req.Variables["id"].(float64); ok && int(id) == 30002 {
			w.Write([]byte(`{"data": {"Media": ` + mediaJSON + `}}`))
			return
		}
		w.Write([]byte(`{"data": null, "errors": [{"message": "Not Found."}]}`))
	}))
	t.Cleanup(srv.Close)
	return New(WithEndpoint(srv.URL))
}

func TestSearchSeries(t *testing.T) {
	c := mockAniList(t)
	results, err := c.SearchSeries(context.Background(), "berserk")
	if err != nil {
		t.Fatalf("SearchSeries: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v", results)
	}
	r := results[0]
	if r.ForeignID != "30002" || r.Title != "Berserk" || r.AuthorName != "Kentarou Miura" {
		t.Errorf("result = %+v", r)
	}
	if r.IssueCount != 41 || r.Year != 1989 {
		t.Errorf("counts = %+v", r)
	}
	if r.Description != "Guts, a former mercenary." {
		t.Errorf("description not de-HTML'd: %q", r.Description)
	}
}

func TestGetSeries(t *testing.T) {
	c := mockAniList(t)
	s, err := c.GetSeries(context.Background(), "30002")
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}
	if len(s.Issues) != 41 {
		t.Fatalf("issues = %d, want 41 synthesized volumes", len(s.Issues))
	}
	if s.Issues[4].Number != 5 || s.Issues[4].Title != "Vol. 5" {
		t.Errorf("issue 5 = %+v", s.Issues[4])
	}

	if _, err := c.GetSeries(context.Background(), "999"); !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("missing series: err = %v, want ErrNotFound", err)
	}
}
