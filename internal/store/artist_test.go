package store

import (
	"testing"

	"github.com/cehbz/discogs/internal/parse"
)

func TestInsertArtist(t *testing.T) {
	db := newTestDB(t)
	a := &parse.Artist{
		ID: 1, Name: "Aphex Twin", RealName: "Richard James", DataQuality: "Correct",
		NameVariations: []string{"AFX", "Polygon Window"},
		URLs:           []string{"https://aphextwin.com"},
		Aliases:        []parse.ArtistRef{{ID: 2, Name: "AFX"}},
		Groups:         []parse.ArtistRef{{ID: 9, Name: "The Tuss"}},
	}
	tx, _ := db.Begin()
	if err := InsertArtist(tx, a); err != nil {
		t.Fatalf("InsertArtist: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var name string
	if err := db.QueryRow(`SELECT name FROM artist WHERE id=1`).Scan(&name); err != nil || name != "Aphex Twin" {
		t.Fatalf("artist: %q %v", name, err)
	}
	var nv int
	db.QueryRow(`SELECT count(*) FROM artist_namevariation WHERE artist_id=1`).Scan(&nv)
	if nv != 2 {
		t.Fatalf("namevars=%d want 2", nv)
	}
	var ali int
	db.QueryRow(`SELECT count(*) FROM artist_alias WHERE artist_id=1 AND alias_artist_id=2`).Scan(&ali)
	if ali != 1 {
		t.Fatalf("alias missing")
	}
	// groups: artist 1 is a member of group 9
	var grp int
	db.QueryRow(`SELECT count(*) FROM artist_membership WHERE group_id=9 AND member_id=1`).Scan(&grp)
	if grp != 1 {
		t.Fatalf("group membership missing")
	}
}
