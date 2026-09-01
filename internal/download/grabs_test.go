package download

import (
	"errors"
	"testing"
)

// TestClaimGrabIsExclusivePerBookAndMediaType is the regression test for the
// duplicate-grab compare-and-swap: a double-clicked grab, or an autosearch
// sweep overlapping a manual grab, must not send the same book to the client
// twice. ClaimGrab is the atomic claim that makes the second attempt fail
// before it ever reaches the client.
func TestClaimGrabIsExclusivePerBookAndMediaType(t *testing.T) {
	store := newTestService(t).Store()

	// grabs.book_id has a foreign key to books(id); seed a minimal author +
	// book (id 7) so the claims below reference a real book.
	if _, err := store.db.Exec(
		`INSERT INTO authors (id, foreign_id, name, sort_name) VALUES (7, 'a7', 'Frank Herbert', 'Herbert, Frank')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO books (id, author_id, foreign_id, title, sort_title) VALUES (7, 7, 'b7', 'Dune', 'Dune')`); err != nil {
		t.Fatal(err)
	}

	// The first claim on a book wins and gets a real pending row.
	g1 := &GrabRecord{BookID: 7, Title: "Dune", Protocol: ProtocolTorrent, MediaType: "ebook"}
	if err := store.ClaimGrab(g1); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if g1.ID == 0 || g1.Status != GrabStatusGrabbed {
		t.Fatalf("first claim = %+v, want a grabbed row with an id", g1)
	}

	// FinishGrabClaim fills in the client details ClaimGrab left blank; the
	// claim stays pending, so it still blocks a duplicate. client_config_id
	// has its own FK, so seed a real client to reference.
	client := sabConfig("http://example.invalid")
	if err := store.Add(client); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishGrabClaim(g1.ID, client.ID, "hash-xyz"); err != nil {
		t.Fatalf("finish claim: %v", err)
	}
	pending, err := store.ListGrabs(GrabStatusGrabbed)
	if err != nil {
		t.Fatal(err)
	}
	var found *GrabRecord
	for i := range pending {
		if pending[i].ID == g1.ID {
			found = &pending[i]
		}
	}
	if found == nil || found.ClientItemID != "hash-xyz" || found.ClientConfigID != client.ID {
		t.Fatalf("after FinishGrabClaim, grab = %+v, want client item id hash-xyz / config %d", found, client.ID)
	}

	// A second claim for the same book + media type is refused.
	g2 := &GrabRecord{BookID: 7, Title: "Dune (again)", Protocol: ProtocolTorrent, MediaType: "ebook"}
	if err := store.ClaimGrab(g2); !errors.Is(err, ErrGrabInFlight) {
		t.Fatalf("second claim err = %v, want ErrGrabInFlight", err)
	}

	// The same book in a DIFFERENT media type is independent and allowed — a
	// book can be wanted as both an ebook and an audiobook.
	g3 := &GrabRecord{BookID: 7, Title: "Dune (audio)", Protocol: ProtocolTorrent, MediaType: "audiobook"}
	if err := store.ClaimGrab(g3); err != nil {
		t.Fatalf("audiobook claim for the same book should be allowed: %v", err)
	}

	// Once the first claim resolves, the book + media type is claimable again.
	if err := store.ResolveGrab(g1.ID, GrabStatusFailed, "test"); err != nil {
		t.Fatal(err)
	}
	g4 := &GrabRecord{BookID: 7, Title: "Dune (retry)", Protocol: ProtocolTorrent, MediaType: "ebook"}
	if err := store.ClaimGrab(g4); err != nil {
		t.Fatalf("claim after the prior grab resolved should succeed: %v", err)
	}

	// DeleteGrab (the client-rejected path) frees a claim immediately too.
	if err := store.DeleteGrab(g4.ID); err != nil {
		t.Fatal(err)
	}
	g5 := &GrabRecord{BookID: 7, Title: "Dune (after release)", Protocol: ProtocolTorrent, MediaType: "ebook"}
	if err := store.ClaimGrab(g5); err != nil {
		t.Fatalf("claim after DeleteGrab released the prior claim should succeed: %v", err)
	}
}

// TestClearHistoryKeepsPendingGrabs: clearing history removes resolved records
// (imported/failed) but must never drop a still-pending grab — an in-flight
// download would otherwise be forgotten.
func TestClearHistoryKeepsPendingGrabs(t *testing.T) {
	store := newTestService(t).Store()

	// book_id 0 stores NULL (no FK), so these need no seeded book.
	grabs := []*GrabRecord{
		{Title: "still pending", Protocol: ProtocolTorrent, MediaType: "ebook", Status: GrabStatusGrabbed},
		{Title: "imported one", Protocol: ProtocolTorrent, MediaType: "ebook", Status: GrabStatusImported},
		{Title: "failed one", Protocol: ProtocolTorrent, MediaType: "ebook", Status: GrabStatusFailed},
	}
	for _, g := range grabs {
		if err := store.AddGrab(g); err != nil {
			t.Fatal(err)
		}
	}

	n, err := store.ClearHistory()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("ClearHistory removed %d rows, want 2 (imported + failed)", n)
	}

	remaining, err := store.ListGrabs("")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Status != GrabStatusGrabbed {
		t.Fatalf("remaining = %+v, want only the pending grab", remaining)
	}
}
