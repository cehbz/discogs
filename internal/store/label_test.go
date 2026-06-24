package store

import (
	"testing"

	"github.com/cehbz/discogs/internal/parse"
)

func TestInsertLabel(t *testing.T) {
	db := newTestDB(t)
	l := &parse.Label{
		ID: 1, Name: "Warp", ContactInfo: "info", DataQuality: "Correct",
		URLs:        []string{"https://warp.net"},
		ParentLabel: &parse.LabelRef{ID: 42, Name: "Parent Co"},
		SubLabels:   []parse.LabelRef{{ID: 7, Name: "Arancia"}},
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := InsertLabel(tx, l); err != nil {
		t.Fatalf("InsertLabel: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var name string
	if err := db.QueryRow(`SELECT name FROM label WHERE id=1`).Scan(&name); err != nil || name != "Warp" {
		t.Fatalf("label row: name=%q err=%v", name, err)
	}
	var url string
	if err := db.QueryRow(`SELECT url FROM label_url WHERE label_id=1 AND seq=0`).Scan(&url); err != nil || url != "https://warp.net" {
		t.Fatalf("label_url: %q %v", url, err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM label_relationship WHERE parent_label_id=42 AND sublabel_id=1`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("parent rel missing: %d %v", n, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM label_relationship WHERE parent_label_id=1 AND sublabel_id=7`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("sublabel rel missing: %d %v", n, err)
	}
}
