package store

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestDB opens an in-memory SQLite DB and applies the schema.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := CreateSchema(db); err != nil {
		t.Fatalf("CreateSchema: %v", err)
	}
	return db
}

func TestCreateSchema_CoreTablesExist(t *testing.T) {
	db := newTestDB(t)
	for _, tbl := range []string{"artist", "label", "master", "release", "track", "meta"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		if err != nil {
			t.Fatalf("table %q missing: %v", tbl, err)
		}
	}
}

func TestCreateSchema_FTS5Available(t *testing.T) {
	db := newTestDB(t)
	// Proves FTS5 compiled in and contentless insert/match works.
	if _, err := db.Exec(`INSERT INTO release_fts(rowid,title,artist_names) VALUES(1,'Kind Of Blue','Miles Davis')`); err != nil {
		t.Fatalf("fts insert: %v", err)
	}
	var id int
	err := db.QueryRow(`SELECT rowid FROM release_fts WHERE release_fts MATCH 'blue'`).Scan(&id)
	if err != nil {
		t.Fatalf("fts match: %v", err)
	}
	if id != 1 {
		t.Fatalf("got rowid %d, want 1", id)
	}
}
