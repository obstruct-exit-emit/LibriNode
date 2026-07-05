package refresh

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/librinode/librinode/internal/database"
	"github.com/librinode/librinode/internal/library"
	"github.com/librinode/librinode/internal/metadata"
)

// mutableProvider serves in-memory metadata that tests can change between
// calls to simulate the provider gaining books or updating fields.
type mutableProvider struct {
	authors map[string]*metadata.Author
	books   map[string]*metadata.Book
}

func (mutableProvider) Name() string { return "fake" }

func (p mutableProvider) SearchAuthors(context.Context, string) ([]metadata.Author, error) {
	return nil, nil
}

func (p mutableProvider) SearchBooks(context.Context, string) ([]metadata.Book, error) {
	return nil, nil
}

func (p mutableProvider) GetAuthor(_ context.Context, id string) (*metadata.Author, error) {
	a, ok := p.authors[id]
	if !ok {
		return nil, metadata.ErrNotFound
	}
	return a, nil
}

func (p mutableProvider) GetBook(_ context.Context, id string) (*metadata.Book, error) {
	b, ok := p.books[id]
	if !ok {
		return nil, metadata.ErrNotFound
	}
	return b, nil
}

func newFixture(t *testing.T) (*Service, *library.Store, mutableProvider) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	store := library.NewStore(db)
	provider := mutableProvider{
		authors: map[string]*metadata.Author{
			"100": {
				ForeignID: "100", Name: "Terry Pratchett", Description: "Sir Terry.",
				Books: []metadata.Book{
					{ForeignID: "1", Title: "The Colour of Magic", AuthorForeignID: "100", AuthorName: "Terry Pratchett",
						Series: []metadata.SeriesLink{{ForeignID: "7", Title: "Discworld", Position: 1}}},
				},
			},
		},
		books: map[string]*metadata.Book{
			"1": {
				ForeignID: "1", Title: "The Colour of Magic", AuthorForeignID: "100", AuthorName: "Terry Pratchett",
				Editions: []metadata.Edition{
					{ForeignID: "11", ISBN13: "9780061020711", Format: "ebook"},
					{ForeignID: "12", ASIN: "B000W94ATC", Format: "audiobook"},
				},
			},
		},
	}
	mgr := metadata.NewManager()
	mgr.Set(provider)
	return New(store, mgr), store, provider
}

func TestSyncWithoutProviderReturnsNotConfigured(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	svc := New(library.NewStore(db), metadata.NewManager())
	if _, err := svc.SyncAuthor(context.Background(), "100", true, "ebook"); !errors.Is(err, metadata.ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}

func TestSyncAuthorThenRefreshPicksUpNewBooks(t *testing.T) {
	svc, store, provider := newFixture(t)
	ctx := context.Background()

	author, err := svc.SyncAuthor(ctx, "100", true, "ebook")
	if err != nil {
		t.Fatalf("SyncAuthor: %v", err)
	}
	books, _ := store.ListBooks(author.ID)
	if len(books) != 1 {
		t.Fatalf("after sync: %d books, want 1", len(books))
	}

	// User unmonitors the book; provider later publishes a new one.
	if err := store.SetBookMonitored(books[0].ID, false); err != nil {
		t.Fatal(err)
	}
	provider.authors["100"].Books = append(provider.authors["100"].Books,
		metadata.Book{ForeignID: "2", Title: "Mort", AuthorForeignID: "100", AuthorName: "Terry Pratchett"})
	provider.authors["100"].Description = "Updated bio."

	if err := svc.RefreshAuthor(ctx, author.ID); err != nil {
		t.Fatalf("RefreshAuthor: %v", err)
	}

	got, _ := store.GetAuthor(author.ID)
	if got.Description != "Updated bio." {
		t.Errorf("description not refreshed: %q", got.Description)
	}
	books, _ = store.ListBooks(author.ID)
	if len(books) != 2 {
		t.Fatalf("after refresh: %d books, want 2", len(books))
	}
	for _, b := range books {
		switch b.ForeignID {
		case "1":
			if b.Monitored {
				t.Error("refresh clobbered the user's unmonitored flag")
			}
		case "2":
			if !b.Monitored {
				t.Error("new book should inherit the author's monitored flag")
			}
		}
	}
}

func TestSyncBookCreatesStubAuthorAndEditions(t *testing.T) {
	svc, store, _ := newFixture(t)
	ctx := context.Background()

	book, err := svc.SyncBook(ctx, "1", true, "ebook")
	if err != nil {
		t.Fatalf("SyncBook: %v", err)
	}
	author, err := store.GetAuthor(book.AuthorID)
	if err != nil {
		t.Fatalf("stub author missing: %v", err)
	}
	if author.Monitored {
		t.Error("stub author should be unmonitored")
	}

	editions, _ := store.ListEditions(book.ID)
	if len(editions) != 2 {
		t.Fatalf("%d editions, want 2", len(editions))
	}
	for _, e := range editions {
		wantMonitored := e.Format == library.FormatEbook
		if e.Monitored != wantMonitored {
			t.Errorf("edition %s (format %s): monitored = %v, want %v", e.ForeignID, e.Format, e.Monitored, wantMonitored)
		}
	}
}

func TestRefreshBookUpdatesEditions(t *testing.T) {
	svc, store, provider := newFixture(t)
	ctx := context.Background()

	book, err := svc.SyncBook(ctx, "1", true, "ebook")
	if err != nil {
		t.Fatalf("SyncBook: %v", err)
	}

	provider.books["1"].Editions = append(provider.books["1"].Editions,
		metadata.Edition{ForeignID: "13", ISBN13: "9999999999999", Format: "ebook"})
	if err := svc.RefreshBook(ctx, book.ID); err != nil {
		t.Fatalf("RefreshBook: %v", err)
	}
	editions, _ := store.ListEditions(book.ID)
	if len(editions) != 3 {
		t.Errorf("%d editions after refresh, want 3", len(editions))
	}
}

func TestRefreshAllSkipsFailures(t *testing.T) {
	svc, store, provider := newFixture(t)
	ctx := context.Background()

	author, err := svc.SyncAuthor(ctx, "100", true, "ebook")
	if err != nil {
		t.Fatal(err)
	}
	// A second author that the provider no longer knows about.
	ghost := &library.Author{Source: "fake", ForeignID: "666", Name: "Ghost Writer", Monitored: true}
	if err := store.UpsertAuthor(ghost); err != nil {
		t.Fatal(err)
	}
	provider.authors["100"].Description = "Refreshed."

	svc.RefreshAll(ctx) // must not abort on the ghost author

	got, _ := store.GetAuthor(author.ID)
	if got.Description != "Refreshed." {
		t.Error("healthy author was not refreshed after a failing one")
	}
}

func TestRefreshAuthorNotFound(t *testing.T) {
	svc, _, _ := newFixture(t)
	if err := svc.RefreshAuthor(context.Background(), 42); !errors.Is(err, library.ErrNotFound) {
		t.Errorf("err = %v, want library.ErrNotFound", err)
	}
}
