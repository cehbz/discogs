package store

import (
	"database/sql"
	"testing"

	"github.com/cehbz/discogs/internal/parse"
)

func TestInsertMaster(t *testing.T) {
	db := newTestDB(t)
	m := &parse.Master{
		ID: 100, MainRelease: 555, Title: "Selected Ambient Works", Year: 1992, DataQuality: "Correct",
		Artists: []parse.ArtistCredit{{ID: 1, Name: "Aphex Twin"}},
		Genres:  []string{"Electronic"},
		Styles:  []string{"Ambient", "IDM"},
		Videos:  []parse.Video{{URI: "https://y.t/x", Duration: 362, Title: "Xtal"}},
	}
	tx, _ := db.Begin()
	if err := InsertMaster(tx, m); err != nil {
		t.Fatalf("InsertMaster: %v", err)
	}
	tx.Commit()

	var year int
	var mainRel int
	if err := db.QueryRow(`SELECT year,main_release_id FROM master WHERE id=100`).Scan(&year, &mainRel); err != nil {
		t.Fatal(err)
	}
	if year != 1992 || mainRel != 555 {
		t.Fatalf("year=%d main=%d", year, mainRel)
	}
	var styles int
	db.QueryRow(`SELECT count(*) FROM master_style WHERE master_id=100`).Scan(&styles)
	if styles != 2 {
		t.Fatalf("styles=%d want 2", styles)
	}
	var dur int
	db.QueryRow(`SELECT duration FROM master_video WHERE master_id=100 AND seq=0`).Scan(&dur)
	if dur != 362 {
		t.Fatalf("video duration=%d", dur)
	}
}

func TestInsertMaster_ZeroYearIsNull(t *testing.T) {
	db := newTestDB(t)
	tx, _ := db.Begin()
	if err := InsertMaster(tx, &parse.Master{ID: 1, Title: "x"}); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	var year sql.NullInt64
	if err := db.QueryRow(`SELECT year FROM master WHERE id=1`).Scan(&year); err != nil {
		t.Fatal(err)
	}
	if year.Valid {
		t.Fatalf("year should be NULL, got %d", year.Int64)
	}
}
