package importer

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/cehbz/discogs/internal/dumps"
	_ "modernc.org/sqlite"
)

func writeGz(t *testing.T, path, body string) {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(body))
	zw.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImport_EndToEnd(t *testing.T) {
	const date = "20260601"
	dir := t.TempDir()
	writeGz(t, filepath.Join(dir, dumps.FileName(date, "artists")),
		`<artists><artist><id>3</id><name>Josh Wink</name></artist></artists>`)
	writeGz(t, filepath.Join(dir, dumps.FileName(date, "labels")),
		`<labels><label><id>6</id><name>Ruffhouse Records</name></label></labels>`)
	writeGz(t, filepath.Join(dir, dumps.FileName(date, "masters")),
		`<masters><master id="100"><title>M</title><year>1999</year></master></masters>`)
	writeGz(t, filepath.Join(dir, dumps.FileName(date, "releases")),
		`<releases><release id="3" status="Accepted"><title>Profound Sounds Vol. 1</title>`+
			`<artists><artist><id>3</id><name>Josh Wink</name></artist></artists>`+
			`<tracklist><track><position>1</position><title>Untitled 8</title></track></tracklist>`+
			`</release></releases>`)

	dbPath := filepath.Join(dir, "out.db")
	rep, err := Import(dbPath, dir, date)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if rep.Counts["releases"] != 1 || rep.Counts["artists"] != 1 {
		t.Fatalf("counts: %+v", rep.Counts)
	}

	db, _ := sql.Open("sqlite", dbPath)
	defer db.Close()
	var id int
	if err := db.QueryRow(`SELECT rowid FROM release_fts WHERE release_fts MATCH 'profound wink'`).Scan(&id); err != nil || id != 3 {
		t.Fatalf("fts search failed: id=%d err=%v", id, err)
	}
	var dumpDate string
	db.QueryRow(`SELECT value FROM meta WHERE key='dump_date'`).Scan(&dumpDate)
	if dumpDate != date {
		t.Fatalf("meta dump_date=%q", dumpDate)
	}
}
