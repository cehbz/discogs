package store

import (
	"testing"

	"github.com/cehbz/discogs/internal/parse"
)

func TestRunIntegrityReport_DetectsOrphanArtistCredit(t *testing.T) {
	db := newTestDB(t)
	tx, _ := db.Begin()
	// release credits artist 3, but we never insert artist 3 => orphan
	if err := InsertRelease(tx, sampleRelease()); err != nil {
		t.Fatal(err)
	}
	tx.Commit()

	rep, err := RunIntegrityReport(db)
	if err != nil {
		t.Fatalf("RunIntegrityReport: %v", err)
	}
	if rep.Orphans["release_artist.artist_id->artist"] < 1 {
		t.Fatalf("expected orphan release_artist credit, got %d", rep.Orphans["release_artist.artist_id->artist"])
	}
}

func TestRunIntegrityReport_CleanWhenArtistPresent(t *testing.T) {
	db := newTestDB(t)
	tx, _ := db.Begin()
	if err := InsertArtist(tx, &parse.Artist{ID: 3, Name: "Josh Wink"}); err != nil {
		t.Fatal(err)
	}
	if err := InsertArtist(tx, &parse.Artist{ID: 99, Name: "Some Engineer"}); err != nil {
		t.Fatal(err)
	}
	if err := InsertRelease(tx, sampleRelease()); err != nil {
		t.Fatal(err)
	}
	tx.Commit()

	rep, _ := RunIntegrityReport(db)
	if rep.Orphans["release_artist.artist_id->artist"] != 0 {
		t.Fatalf("expected 0 orphans, got %d", rep.Orphans["release_artist.artist_id->artist"])
	}
}

func TestSetMeta(t *testing.T) {
	db := newTestDB(t)
	tx, _ := db.Begin()
	if err := SetMeta(tx, map[string]string{"dump_date": "20260601"}); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	var v string
	db.QueryRow(`SELECT value FROM meta WHERE key='dump_date'`).Scan(&v)
	if v != "20260601" {
		t.Fatalf("meta=%q", v)
	}
}
