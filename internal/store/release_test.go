package store

import (
	"testing"

	"github.com/cehbz/discogs/internal/parse"
)

func sampleRelease() *parse.Release {
	return &parse.Release{
		ID: 3, Status: "Accepted", Title: "Profound Sounds Vol. 1",
		Country: "US", Released: "1999-07-13", DataQuality: "Correct",
		Master:       &parse.MasterRef{ID: 66526, IsMainRelease: false},
		Artists:      []parse.ArtistCredit{{ID: 3, Name: "Josh Wink"}},
		ExtraArtists: []parse.ArtistCredit{{ID: 99, Name: "Some Engineer", Role: "Mastered By"}},
		Labels:       []parse.ReleaseLabel{{ID: 6, Name: "Ruffhouse Records", Catno: "CK 63628"}},
		Formats:      []parse.Format{{Name: "CD", Qty: "1", Descriptions: []string{"Compilation", "Mixed"}}},
		Genres:       []string{"Electronic"},
		Styles:       []string{"Techno"},
		Identifiers:  []parse.Identifier{{Type: "Barcode", Value: "074646362822"}},
		Companies:    []parse.Company{{ID: 5, Name: "PolyGram", EntityTypeName: "Manufactured By"}},
		Videos:       []parse.Video{{URI: "https://y.t/v", Duration: 420, Title: "V"}},
	}
}

func TestInsertRelease_Tracklist(t *testing.T) {
	db := newTestDB(t)
	r := sampleRelease()
	r.Tracklist = []parse.Track{
		{Position: "1", Title: "Suite", Duration: "20:00",
			Artists: []parse.ArtistCredit{{ID: 3, Name: "Josh Wink"}},
			SubTracks: []parse.Track{
				{Position: "1.a", Title: "Part One", Duration: "10:00"},
				{Position: "1.b", Title: "Part Two", Duration: "10:00",
					ExtraArtists: []parse.ArtistCredit{{ID: 50, Name: "Remixer", Role: "Remix"}}},
			}},
	}
	tx, _ := db.Begin()
	if err := InsertRelease(tx, r); err != nil {
		t.Fatalf("InsertRelease: %v", err)
	}
	tx.Commit()

	var total int
	db.QueryRow(`SELECT count(*) FROM track WHERE release_id=3`).Scan(&total)
	if total != 3 {
		t.Fatalf("tracks=%d want 3 (1 parent + 2 sub)", total)
	}
	// parent of the two sub-tracks is the top-level track
	var parentTitle string
	if err := db.QueryRow(`
		SELECT p.title FROM track c JOIN track p ON p.id=c.parent_track_id
		WHERE c.title='Part Two'`).Scan(&parentTitle); err != nil {
		t.Fatal(err)
	}
	if parentTitle != "Suite" {
		t.Fatalf("parent title=%q want Suite", parentTitle)
	}
	// track-level credit recorded
	var credits int
	db.QueryRow(`
		SELECT count(*) FROM track_artist ta JOIN track t ON t.id=ta.track_id
		WHERE t.title='Part Two' AND ta.kind='extra'`).Scan(&credits)
	if credits != 1 {
		t.Fatalf("track credits=%d want 1", credits)
	}
}

func TestInsertRelease_Flat(t *testing.T) {
	db := newTestDB(t)
	tx, _ := db.Begin()
	if err := InsertRelease(tx, sampleRelease()); err != nil {
		t.Fatalf("InsertRelease: %v", err)
	}
	tx.Commit()

	var title, country string
	var masterID int
	var isMain int
	if err := db.QueryRow(`SELECT title,country,master_id,is_main_release FROM release WHERE id=3`).
		Scan(&title, &country, &masterID, &isMain); err != nil {
		t.Fatal(err)
	}
	if title != "Profound Sounds Vol. 1" || country != "US" || masterID != 66526 || isMain != 0 {
		t.Fatalf("core: %q %q %d %d", title, country, masterID, isMain)
	}
	var main, extra int
	db.QueryRow(`SELECT count(*) FROM release_artist WHERE release_id=3 AND kind='main'`).Scan(&main)
	db.QueryRow(`SELECT count(*) FROM release_artist WHERE release_id=3 AND kind='extra'`).Scan(&extra)
	if main != 1 || extra != 1 {
		t.Fatalf("credits main=%d extra=%d", main, extra)
	}
	var descs int
	db.QueryRow(`SELECT count(*) FROM release_format_description d JOIN release_format f ON f.id=d.format_id WHERE f.release_id=3`).Scan(&descs)
	if descs != 2 {
		t.Fatalf("format descriptions=%d want 2", descs)
	}
	var catno string
	db.QueryRow(`SELECT catno FROM release_label WHERE release_id=3 AND seq=0`).Scan(&catno)
	if catno != "CK 63628" {
		t.Fatalf("catno=%q", catno)
	}
}
