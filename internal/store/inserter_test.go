package store

import (
	"testing"

	"github.com/cehbz/discogs/internal/parse"
)

// TestArtistInserter_StatementReuse creates a single ArtistInserter on one tx,
// inserts two distinct artists via the same prepared statements, commits, and
// asserts both landed correctly. This proves statement reuse across multiple rows.
func TestArtistInserter_StatementReuse(t *testing.T) {
	db := newTestDB(t)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}

	ins, err := NewArtistInserter(tx)
	if err != nil {
		t.Fatalf("NewArtistInserter: %v", err)
	}

	artists := []*parse.Artist{
		{
			ID: 10, Name: "Aphex Twin", RealName: "Richard James", DataQuality: "Correct",
			NameVariations: []string{"AFX", "Polygon Window"},
			URLs:           []string{"https://aphextwin.warp.net"},
			Aliases:        []parse.ArtistRef{{ID: 11, Name: "AFX"}},
		},
		{
			ID: 20, Name: "Autechre", DataQuality: "Correct",
			NameVariations: []string{"AE"},
			Members: []parse.ArtistRef{
				{ID: 21, Name: "Rob Brown"},
				{ID: 22, Name: "Sean Booth"},
			},
		},
	}

	for _, a := range artists {
		if err := ins.Insert(a); err != nil {
			t.Fatalf("Insert artist %d: %v", a.ID, err)
		}
	}

	if err := ins.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Verify both artists landed.
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM artist WHERE id IN (10, 20)`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("artist count = %d, want 2", count)
	}

	// Verify artist 10's namevariations (2 rows).
	var nv10 int
	db.QueryRow(`SELECT count(*) FROM artist_namevariation WHERE artist_id=10`).Scan(&nv10)
	if nv10 != 2 {
		t.Fatalf("artist 10 namevariations = %d, want 2", nv10)
	}

	// Verify artist 20's namevariations (1 row).
	var nv20 int
	db.QueryRow(`SELECT count(*) FROM artist_namevariation WHERE artist_id=20`).Scan(&nv20)
	if nv20 != 1 {
		t.Fatalf("artist 20 namevariations = %d, want 1", nv20)
	}

	// Verify artist 20's members via artist_membership.
	var members int
	db.QueryRow(`SELECT count(*) FROM artist_membership WHERE group_id=20`).Scan(&members)
	if members != 2 {
		t.Fatalf("artist 20 members = %d, want 2", members)
	}
}
