package store

import (
	"testing"

	"github.com/cehbz/discogs/internal/parse"
)

func TestBuildFTS_ReleaseSearchByArtistAndTitle(t *testing.T) {
	db := newTestDB(t)
	tx, _ := db.Begin()
	// artist must exist so artist_names can resolve
	if err := InsertArtist(tx, &parse.Artist{ID: 3, Name: "Josh Wink"}); err != nil {
		t.Fatal(err)
	}
	if err := InsertRelease(tx, sampleRelease()); err != nil {
		t.Fatal(err)
	}
	tx.Commit()

	if err := BuildFTS(db); err != nil {
		t.Fatalf("BuildFTS: %v", err)
	}
	// title match
	var id int
	if err := db.QueryRow(`SELECT rowid FROM release_fts WHERE release_fts MATCH 'profound'`).Scan(&id); err != nil || id != 3 {
		t.Fatalf("title match: id=%d err=%v", id, err)
	}
	// artist_names match via the denormalized column
	if err := db.QueryRow(`SELECT rowid FROM release_fts WHERE release_fts MATCH 'artist_names:wink'`).Scan(&id); err != nil || id != 3 {
		t.Fatalf("artist match: id=%d err=%v", id, err)
	}
}

func TestBuildIndexes_Idempotent(t *testing.T) {
	db := newTestDB(t)
	if err := BuildIndexes(db); err != nil {
		t.Fatalf("BuildIndexes: %v", err)
	}
	if err := BuildIndexes(db); err != nil { // IF NOT EXISTS => safe twice
		t.Fatalf("BuildIndexes second run: %v", err)
	}
}
