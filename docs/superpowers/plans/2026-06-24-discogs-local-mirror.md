# Local Discogs Mirror Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Import the four monthly Discogs XML dumps into a single fully-normalized (3NF) SQLite database with FTS5 search, queryable directly via native SQLite bindings.

**Architecture:** A Go CLI (`discogs`) with three internal packages — `dumps` (download + SHA-256 verify), `parse` (streaming XML → structs), `store` (3NF schema + insert layer + FTS/index build + integrity report) — plus an `importer` package that orchestrates a fresh-DB build. Parsing is strictly streaming (token decoder over gzip); each import produces a fresh `discogs-YYYYMMDD.db`.

**Tech Stack:** Go 1.25, `modernc.org/sqlite` (cgo-free SQLite driver with FTS5), `spf13/cobra` for the CLI, `encoding/xml` token streaming.

## Global Constraints

- Module path: `github.com/cehbz/discogs`; Go 1.25; install binaries to `GOBIN=~/bin` via `go install ./cmd/discogs`.
- SQLite driver: `modernc.org/sqlite` (driver name `"sqlite"`), required because it is cgo-free and ships FTS5. **If Task 1's schema test fails because FTS5 is unavailable, switch the driver to `github.com/mattn/go-sqlite3` built with `-tags fts5`** and adjust the driver name to `"sqlite3"`; everything else is unchanged.
- Core entities (`artist`, `label`, `master`, `release`) are keyed on **Discogs' own integer id** as PRIMARY KEY — never a synthetic autoincrement. This is required for future MusicBrainz cross-DB joins.
- Controlled vocabularies (genre, style, role, identifier type, entity type, format name/description) are stored as plain `TEXT`. No lookup tables.
- XML parsing is **streaming only** (`encoding/xml` token decoder over `gzip.NewReader`). Never DOM-parse a whole dump file — the releases file is ~32 GB uncompressed.
- The Discogs `join` attribute maps to the SQL column `join_str` (`join` is a SQL reserved word).
- Each import builds a **fresh** database file; there is no in-place migration or delta update.
- FTS5 tables are **contentless** (`content=''`), with `rowid` = the Discogs id; populated after the bulk row load; tokenizer `unicode61 remove_diacritics 2`.
- **Commits require explicit per-commit user approval** (the user's workflow). At every "Commit" step the executor MUST show the exact message and wait for approval — never commit autonomously.

---

## File Structure

```
go.mod
cmd/discogs/
  main.go        # cobra root, wires subcommands
  fetch.go       # `discogs fetch`  -> dumps.Download + dumps.VerifyChecksums
  verify.go      # `discogs verify` -> dumps.VerifyChecksums
  import.go      # `discogs import` -> importer.Import
internal/dumps/
  dumps.go       # Download, VerifyChecksums, file-name helpers
  dumps_test.go
internal/parse/
  stream.go      # generic streamRecords + shared types (ArtistRef, ArtistCredit, Video, LabelRef)
  label.go       # Label struct, ParseLabels
  artist.go      # Artist struct, ParseArtists
  master.go      # Master struct, ParseMasters
  release.go     # Release struct (+ Format/Identifier/Company/Track), ParseReleases
  *_test.go
internal/store/
  schema.go      # DDL constant, CreateSchema, newTestDB helper (test)
  label.go       # InsertLabel
  artist.go      # InsertArtist
  master.go      # InsertMaster, nullIfZero
  release.go     # InsertRelease (+ tracks), insertReleaseCredits, insertTracks
  fts.go         # BuildFTS, BuildIndexes
  integrity.go   # RunIntegrityReport, IntegrityReport
  meta.go        # SetMeta
  *_test.go
internal/importer/
  importer.go    # Import orchestrator, Report, importStream
  importer_test.go
```

---

## Task 1: Project scaffold + 3NF schema

**Files:**
- Create: `go.mod`
- Create: `internal/store/schema.go`
- Test: `internal/store/schema_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `store.CreateSchema(db *sql.DB) error`; test helper `newTestDB(t *testing.T) *sql.DB` (creates an in-memory DB with the schema applied) reused by later store tests.

- [ ] **Step 1: Initialize the module and add the driver**

Run:
```bash
cd ~/projects/discogs
go mod init github.com/cehbz/discogs
go get modernc.org/sqlite
```
Expected: `go.mod` created with `module github.com/cehbz/discogs` and a `require modernc.org/sqlite` line.

- [ ] **Step 2: Write the failing schema test**

Create `internal/store/schema_test.go`:
```go
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
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/store/ -run TestCreateSchema -v`
Expected: FAIL — `undefined: CreateSchema`.

- [ ] **Step 4: Write the schema**

Create `internal/store/schema.go`:
```go
package store

import "database/sql"

const schemaSQL = `
CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE artist (
  id INTEGER PRIMARY KEY, name TEXT NOT NULL, realname TEXT, profile TEXT, data_quality TEXT
);
CREATE TABLE label (
  id INTEGER PRIMARY KEY, name TEXT NOT NULL, contactinfo TEXT, profile TEXT, data_quality TEXT
);
CREATE TABLE master (
  id INTEGER PRIMARY KEY, main_release_id INTEGER, title TEXT, year INTEGER, data_quality TEXT
);
CREATE TABLE release (
  id INTEGER PRIMARY KEY, status TEXT, title TEXT, country TEXT, released_raw TEXT,
  notes TEXT, data_quality TEXT, master_id INTEGER, is_main_release INTEGER
);

CREATE TABLE artist_namevariation (artist_id INTEGER NOT NULL, seq INTEGER NOT NULL, name TEXT NOT NULL, PRIMARY KEY(artist_id,seq));
CREATE TABLE artist_url (artist_id INTEGER NOT NULL, seq INTEGER NOT NULL, url TEXT NOT NULL, PRIMARY KEY(artist_id,seq));
CREATE TABLE artist_alias (artist_id INTEGER NOT NULL, alias_artist_id INTEGER NOT NULL, name TEXT, PRIMARY KEY(artist_id,alias_artist_id));
CREATE TABLE artist_membership (group_id INTEGER NOT NULL, member_id INTEGER NOT NULL, name TEXT, PRIMARY KEY(group_id,member_id));

CREATE TABLE label_url (label_id INTEGER NOT NULL, seq INTEGER NOT NULL, url TEXT NOT NULL, PRIMARY KEY(label_id,seq));
CREATE TABLE label_relationship (parent_label_id INTEGER NOT NULL, sublabel_id INTEGER NOT NULL, PRIMARY KEY(parent_label_id,sublabel_id));

CREATE TABLE master_artist (master_id INTEGER NOT NULL, seq INTEGER NOT NULL, artist_id INTEGER NOT NULL, anv TEXT, join_str TEXT, role TEXT, PRIMARY KEY(master_id,seq));
CREATE TABLE master_genre (master_id INTEGER NOT NULL, seq INTEGER NOT NULL, genre TEXT NOT NULL, PRIMARY KEY(master_id,seq));
CREATE TABLE master_style (master_id INTEGER NOT NULL, seq INTEGER NOT NULL, style TEXT NOT NULL, PRIMARY KEY(master_id,seq));
CREATE TABLE master_video (master_id INTEGER NOT NULL, seq INTEGER NOT NULL, uri TEXT, title TEXT, description TEXT, duration INTEGER, PRIMARY KEY(master_id,seq));

CREATE TABLE release_artist (id INTEGER PRIMARY KEY, release_id INTEGER NOT NULL, seq INTEGER NOT NULL, artist_id INTEGER NOT NULL, anv TEXT, join_str TEXT, role TEXT, tracks TEXT, kind TEXT NOT NULL);
CREATE TABLE release_label (release_id INTEGER NOT NULL, seq INTEGER NOT NULL, label_id INTEGER, name TEXT, catno TEXT, PRIMARY KEY(release_id,seq));
CREATE TABLE release_format (id INTEGER PRIMARY KEY, release_id INTEGER NOT NULL, seq INTEGER NOT NULL, name TEXT, qty TEXT, text TEXT);
CREATE TABLE release_format_description (format_id INTEGER NOT NULL, seq INTEGER NOT NULL, description TEXT NOT NULL, PRIMARY KEY(format_id,seq));
CREATE TABLE release_genre (release_id INTEGER NOT NULL, seq INTEGER NOT NULL, genre TEXT NOT NULL, PRIMARY KEY(release_id,seq));
CREATE TABLE release_style (release_id INTEGER NOT NULL, seq INTEGER NOT NULL, style TEXT NOT NULL, PRIMARY KEY(release_id,seq));
CREATE TABLE release_identifier (release_id INTEGER NOT NULL, seq INTEGER NOT NULL, type TEXT, value TEXT, description TEXT, PRIMARY KEY(release_id,seq));
CREATE TABLE release_company (release_id INTEGER NOT NULL, seq INTEGER NOT NULL, company_label_id INTEGER, name TEXT, catno TEXT, entity_type TEXT, entity_type_name TEXT, PRIMARY KEY(release_id,seq));
CREATE TABLE release_video (release_id INTEGER NOT NULL, seq INTEGER NOT NULL, uri TEXT, title TEXT, description TEXT, duration INTEGER, PRIMARY KEY(release_id,seq));

CREATE TABLE track (id INTEGER PRIMARY KEY, release_id INTEGER NOT NULL, parent_track_id INTEGER, seq INTEGER NOT NULL, position TEXT, title TEXT, duration TEXT);
CREATE TABLE track_artist (id INTEGER PRIMARY KEY, track_id INTEGER NOT NULL, seq INTEGER NOT NULL, artist_id INTEGER NOT NULL, anv TEXT, join_str TEXT, role TEXT, kind TEXT NOT NULL);

CREATE VIRTUAL TABLE artist_fts  USING fts5(name,                content='', tokenize='unicode61 remove_diacritics 2');
CREATE VIRTUAL TABLE label_fts   USING fts5(name,                content='', tokenize='unicode61 remove_diacritics 2');
CREATE VIRTUAL TABLE master_fts  USING fts5(title, artist_names, content='', tokenize='unicode61 remove_diacritics 2');
CREATE VIRTUAL TABLE release_fts USING fts5(title, artist_names, content='', tokenize='unicode61 remove_diacritics 2');
`

// CreateSchema applies the full 3NF schema and FTS5 virtual tables to db.
func CreateSchema(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	return err
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/store/ -run TestCreateSchema -v`
Expected: PASS. (If `TestCreateSchema_FTS5Available` fails with a "no such module: fts5"-style error, apply the driver swap noted in Global Constraints, then re-run.)

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/store/schema.go internal/store/schema_test.go
git commit -m "feat: project scaffold and 3NF SQLite schema with FTS5"
```

---

## Task 2: Parse foundation + labels (parse & insert)

**Files:**
- Create: `internal/parse/stream.go`
- Create: `internal/parse/label.go`
- Create: `internal/store/label.go`
- Test: `internal/parse/label_test.go`, `internal/store/label_test.go`

**Interfaces:**
- Consumes: `store.CreateSchema`, test helper `newTestDB`.
- Produces:
  - `parse.streamRecords[T any](r io.Reader, local string, fn func(*T) error) error`
  - shared types `parse.ArtistRef{ID int; Name string}`, `parse.LabelRef{ID int; Name string}`, `parse.ArtistCredit{ID int; Name, ANV, Join, Role, Tracks string}`, `parse.Video{URI string; Duration int; Title, Description string}`
  - `parse.Label{ID int; Name, ContactInfo, Profile, DataQuality string; URLs []string; ParentLabel *LabelRef; SubLabels []LabelRef}`
  - `parse.ParseLabels(r io.Reader, fn func(*Label) error) error`
  - `store.InsertLabel(tx *sql.Tx, l *parse.Label) error`

- [ ] **Step 1: Write the failing parser test**

Create `internal/parse/label_test.go`:
```go
package parse

import (
	"strings"
	"testing"
)

func TestParseLabels(t *testing.T) {
	const xml = `<labels>
<label><id>1</id><name>Warp</name><contactinfo>info</contactinfo><profile>p</profile>
  <data_quality>Correct</data_quality>
  <urls><url>https://warp.net</url></urls>
  <parentLabel id="42">Parent Co</parentLabel>
  <sublabels><label id="7">Arancia</label><label id="8">Nuts</label></sublabels>
</label>
<label><id>42</id><name>Parent Co</name></label>
</labels>`
	var got []*Label
	err := ParseLabels(strings.NewReader(xml), func(l *Label) error { got = append(got, l); return nil })
	if err != nil {
		t.Fatalf("ParseLabels: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d labels, want 2", len(got))
	}
	w := got[0]
	if w.ID != 1 || w.Name != "Warp" || w.ContactInfo != "info" || w.DataQuality != "Correct" {
		t.Fatalf("bad core fields: %+v", w)
	}
	if len(w.URLs) != 1 || w.URLs[0] != "https://warp.net" {
		t.Fatalf("bad urls: %v", w.URLs)
	}
	if w.ParentLabel == nil || w.ParentLabel.ID != 42 {
		t.Fatalf("bad parent: %+v", w.ParentLabel)
	}
	if len(w.SubLabels) != 2 || w.SubLabels[0].ID != 7 || w.SubLabels[1].Name != "Nuts" {
		t.Fatalf("bad sublabels: %+v", w.SubLabels)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/parse/ -run TestParseLabels -v`
Expected: FAIL — `undefined: ParseLabels`.

- [ ] **Step 3: Write the streaming foundation and shared types**

Create `internal/parse/stream.go`:
```go
package parse

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

// ArtistRef is an id+name reference to another artist (aliases, members, groups).
type ArtistRef struct {
	ID   int    `xml:"id,attr"`
	Name string `xml:",chardata"`
}

// LabelRef is an id+name reference to another label (parentLabel, sublabels).
type LabelRef struct {
	ID   int    `xml:"id,attr"`
	Name string `xml:",chardata"`
}

// ArtistCredit is a credited artist on a master, release, or track.
type ArtistCredit struct {
	ID     int    `xml:"id"`
	Name   string `xml:"name"`
	ANV    string `xml:"anv"`
	Join   string `xml:"join"`
	Role   string `xml:"role"`
	Tracks string `xml:"tracks"`
}

// Video is a linked video on a master or release.
type Video struct {
	URI         string `xml:"src,attr"`
	Duration    int    `xml:"duration,attr"`
	Title       string `xml:"title"`
	Description string `xml:"description"`
}

// streamRecords streams top-level <local> elements from r, decoding each into a
// fresh T and invoking fn. It never holds more than one record in memory.
func streamRecords[T any](r io.Reader, local string, fn func(*T) error) error {
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != local {
			continue
		}
		var rec T
		if err := dec.DecodeElement(&rec, &se); err != nil {
			return fmt.Errorf("decode <%s>: %w", local, err)
		}
		if err := fn(&rec); err != nil {
			return err
		}
	}
}
```

Create `internal/parse/label.go`:
```go
package parse

import "io"

type Label struct {
	ID          int       `xml:"id"`
	Name        string    `xml:"name"`
	ContactInfo string    `xml:"contactinfo"`
	Profile     string    `xml:"profile"`
	DataQuality string    `xml:"data_quality"`
	URLs        []string  `xml:"urls>url"`
	ParentLabel *LabelRef `xml:"parentLabel"`
	SubLabels   []LabelRef `xml:"sublabels>label"`
}

func ParseLabels(r io.Reader, fn func(*Label) error) error {
	return streamRecords(r, "label", fn)
}
```

- [ ] **Step 4: Run the parser test to verify it passes**

Run: `go test ./internal/parse/ -run TestParseLabels -v`
Expected: PASS.

- [ ] **Step 5: Write the failing insert test**

Create `internal/store/label_test.go`:
```go
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
```

- [ ] **Step 6: Run the insert test to verify it fails**

Run: `go test ./internal/store/ -run TestInsertLabel -v`
Expected: FAIL — `undefined: InsertLabel`.

- [ ] **Step 7: Write the label insert**

Create `internal/store/label.go`:
```go
package store

import (
	"database/sql"
	"fmt"

	"github.com/cehbz/discogs/internal/parse"
)

func InsertLabel(tx *sql.Tx, l *parse.Label) error {
	if _, err := tx.Exec(
		`INSERT INTO label(id,name,contactinfo,profile,data_quality) VALUES(?,?,?,?,?)`,
		l.ID, l.Name, l.ContactInfo, l.Profile, l.DataQuality); err != nil {
		return fmt.Errorf("insert label %d: %w", l.ID, err)
	}
	for i, u := range l.URLs {
		if _, err := tx.Exec(`INSERT INTO label_url(label_id,seq,url) VALUES(?,?,?)`, l.ID, i, u); err != nil {
			return fmt.Errorf("insert label_url %d/%d: %w", l.ID, i, err)
		}
	}
	if l.ParentLabel != nil && l.ParentLabel.ID != 0 {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO label_relationship(parent_label_id,sublabel_id) VALUES(?,?)`,
			l.ParentLabel.ID, l.ID); err != nil {
			return fmt.Errorf("insert parent rel %d: %w", l.ID, err)
		}
	}
	for _, sub := range l.SubLabels {
		if sub.ID == 0 {
			continue
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO label_relationship(parent_label_id,sublabel_id) VALUES(?,?)`,
			l.ID, sub.ID); err != nil {
			return fmt.Errorf("insert sublabel rel %d: %w", l.ID, err)
		}
	}
	return nil
}
```

- [ ] **Step 8: Run all package tests to verify pass**

Run: `go test ./internal/parse/ ./internal/store/ -v`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/parse/stream.go internal/parse/label.go internal/parse/label_test.go internal/store/label.go internal/store/label_test.go
git commit -m "feat: streaming parse foundation and label ingest"
```

---

## Task 3: Artists (parse & insert)

**Files:**
- Create: `internal/parse/artist.go`, `internal/store/artist.go`
- Test: `internal/parse/artist_test.go`, `internal/store/artist_test.go`

**Interfaces:**
- Consumes: `parse.streamRecords`, `parse.ArtistRef`, `store.CreateSchema`, `newTestDB`.
- Produces:
  - `parse.Artist{ID int; Name, RealName, Profile, DataQuality string; NameVariations, URLs []string; Aliases, Members, Groups []ArtistRef}`
  - `parse.ParseArtists(r io.Reader, fn func(*Artist) error) error`
  - `store.InsertArtist(tx *sql.Tx, a *parse.Artist) error`

- [ ] **Step 1: Write the failing parser test**

Create `internal/parse/artist_test.go`:
```go
package parse

import (
	"strings"
	"testing"
)

func TestParseArtists(t *testing.T) {
	const xml = `<artists>
<artist><id>1</id><name>Aphex Twin</name><realname>Richard James</realname>
  <profile>bio</profile><data_quality>Correct</data_quality>
  <namevariations><name>AFX</name><name>Polygon Window</name></namevariations>
  <urls><url>https://aphextwin.com</url></urls>
  <aliases><name id="2">AFX</name></aliases>
  <groups><name id="9">The Tuss</name></groups>
</artist>
<artist><id>5</id><name>Autechre</name>
  <members><id>6</id><name id="6">Sean Booth</name><id>7</id><name id="7">Rob Brown</name></members>
</artist>
</artists>`
	var got []*Artist
	if err := ParseArtists(strings.NewReader(xml), func(a *Artist) error { got = append(got, a); return nil }); err != nil {
		t.Fatalf("ParseArtists: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	a := got[0]
	if a.ID != 1 || a.Name != "Aphex Twin" || a.RealName != "Richard James" {
		t.Fatalf("core: %+v", a)
	}
	if len(a.NameVariations) != 2 || a.NameVariations[1] != "Polygon Window" {
		t.Fatalf("namevars: %v", a.NameVariations)
	}
	if len(a.Aliases) != 1 || a.Aliases[0].ID != 2 {
		t.Fatalf("aliases: %+v", a.Aliases)
	}
	if len(a.Groups) != 1 || a.Groups[0].ID != 9 {
		t.Fatalf("groups: %+v", a.Groups)
	}
	if len(got[1].Members) != 2 || got[1].Members[0].ID != 6 || got[1].Members[1].Name != "Rob Brown" {
		t.Fatalf("members: %+v", got[1].Members)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/parse/ -run TestParseArtists -v`
Expected: FAIL — `undefined: ParseArtists`.

- [ ] **Step 3: Write the artist parser**

Create `internal/parse/artist.go`:
```go
package parse

import "io"

type Artist struct {
	ID             int         `xml:"id"`
	Name           string      `xml:"name"`
	RealName       string      `xml:"realname"`
	Profile        string      `xml:"profile"`
	DataQuality    string      `xml:"data_quality"`
	NameVariations []string    `xml:"namevariations>name"`
	URLs           []string    `xml:"urls>url"`
	Aliases        []ArtistRef `xml:"aliases>name"`
	Members        []ArtistRef `xml:"members>name"`
	Groups         []ArtistRef `xml:"groups>name"`
}

func ParseArtists(r io.Reader, fn func(*Artist) error) error {
	return streamRecords(r, "artist", fn)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/parse/ -run TestParseArtists -v`
Expected: PASS.

- [ ] **Step 5: Write the failing insert test**

Create `internal/store/artist_test.go`:
```go
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
```

- [ ] **Step 6: Run to verify it fails**

Run: `go test ./internal/store/ -run TestInsertArtist -v`
Expected: FAIL — `undefined: InsertArtist`.

- [ ] **Step 7: Write the artist insert**

Create `internal/store/artist.go`:
```go
package store

import (
	"database/sql"
	"fmt"

	"github.com/cehbz/discogs/internal/parse"
)

func InsertArtist(tx *sql.Tx, a *parse.Artist) error {
	if _, err := tx.Exec(
		`INSERT INTO artist(id,name,realname,profile,data_quality) VALUES(?,?,?,?,?)`,
		a.ID, a.Name, a.RealName, a.Profile, a.DataQuality); err != nil {
		return fmt.Errorf("insert artist %d: %w", a.ID, err)
	}
	for i, nv := range a.NameVariations {
		if _, err := tx.Exec(`INSERT INTO artist_namevariation(artist_id,seq,name) VALUES(?,?,?)`, a.ID, i, nv); err != nil {
			return fmt.Errorf("insert namevar %d/%d: %w", a.ID, i, err)
		}
	}
	for i, u := range a.URLs {
		if _, err := tx.Exec(`INSERT INTO artist_url(artist_id,seq,url) VALUES(?,?,?)`, a.ID, i, u); err != nil {
			return fmt.Errorf("insert artist_url %d/%d: %w", a.ID, i, err)
		}
	}
	for _, al := range a.Aliases {
		if al.ID == 0 {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO artist_alias(artist_id,alias_artist_id,name) VALUES(?,?,?)`, a.ID, al.ID, al.Name); err != nil {
			return fmt.Errorf("insert alias %d: %w", a.ID, err)
		}
	}
	for _, m := range a.Members {
		if m.ID == 0 {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO artist_membership(group_id,member_id,name) VALUES(?,?,?)`, a.ID, m.ID, m.Name); err != nil {
			return fmt.Errorf("insert member %d: %w", a.ID, err)
		}
	}
	for _, g := range a.Groups {
		if g.ID == 0 {
			continue
		}
		// inverse view: this artist is a member of group g
		if _, err := tx.Exec(`INSERT OR IGNORE INTO artist_membership(group_id,member_id,name) VALUES(?,?,?)`, g.ID, a.ID, a.Name); err != nil {
			return fmt.Errorf("insert group %d: %w", a.ID, err)
		}
	}
	return nil
}
```

- [ ] **Step 8: Run to verify it passes**

Run: `go test ./internal/store/ -run TestInsertArtist -v`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/parse/artist.go internal/parse/artist_test.go internal/store/artist.go internal/store/artist_test.go
git commit -m "feat: artist ingest"
```

---

## Task 4: Masters (parse & insert)

**Files:**
- Create: `internal/parse/master.go`, `internal/store/master.go`
- Test: `internal/parse/master_test.go`, `internal/store/master_test.go`

**Interfaces:**
- Consumes: `parse.streamRecords`, `parse.ArtistCredit`, `parse.Video`, `newTestDB`.
- Produces:
  - `parse.Master{ID int; MainRelease int; Title string; Year int; DataQuality string; Artists []ArtistCredit; Genres, Styles []string; Videos []Video}`
  - `parse.ParseMasters(r io.Reader, fn func(*Master) error) error`
  - `store.InsertMaster(tx *sql.Tx, m *parse.Master) error`
  - `store.nullIfZero(n int) any` (returns nil when n==0, else n) — used by master & release inserts.

- [ ] **Step 1: Write the failing parser test**

Create `internal/parse/master_test.go`:
```go
package parse

import (
	"strings"
	"testing"
)

func TestParseMasters(t *testing.T) {
	const xml = `<masters>
<master id="100"><main_release>555</main_release><title>Selected Ambient Works</title>
  <year>1992</year><data_quality>Correct</data_quality>
  <artists><artist><id>1</id><name>Aphex Twin</name><anv/><join/><role/></artist></artists>
  <genres><genre>Electronic</genre></genres>
  <styles><style>Ambient</style><style>IDM</style></styles>
  <videos><video src="https://y.t/x" duration="362"><title>Xtal</title><description>d</description></video></videos>
</master>
</masters>`
	var got []*Master
	if err := ParseMasters(strings.NewReader(xml), func(m *Master) error { got = append(got, m); return nil }); err != nil {
		t.Fatalf("ParseMasters: %v", err)
	}
	m := got[0]
	if m.ID != 100 || m.MainRelease != 555 || m.Year != 1992 || m.Title != "Selected Ambient Works" {
		t.Fatalf("core: %+v", m)
	}
	if len(m.Artists) != 1 || m.Artists[0].ID != 1 {
		t.Fatalf("artists: %+v", m.Artists)
	}
	if len(m.Styles) != 2 || m.Styles[1] != "IDM" {
		t.Fatalf("styles: %v", m.Styles)
	}
	if len(m.Videos) != 1 || m.Videos[0].Duration != 362 || m.Videos[0].Title != "Xtal" {
		t.Fatalf("videos: %+v", m.Videos)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/parse/ -run TestParseMasters -v`
Expected: FAIL — `undefined: ParseMasters`.

- [ ] **Step 3: Write the master parser**

Create `internal/parse/master.go`:
```go
package parse

import "io"

type Master struct {
	ID          int            `xml:"id,attr"`
	MainRelease int            `xml:"main_release"`
	Title       string         `xml:"title"`
	Year        int            `xml:"year"`
	DataQuality string         `xml:"data_quality"`
	Artists     []ArtistCredit `xml:"artists>artist"`
	Genres      []string       `xml:"genres>genre"`
	Styles      []string       `xml:"styles>style"`
	Videos      []Video        `xml:"videos>video"`
}

func ParseMasters(r io.Reader, fn func(*Master) error) error {
	return streamRecords(r, "master", fn)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/parse/ -run TestParseMasters -v`
Expected: PASS.

- [ ] **Step 5: Write the failing insert test**

Create `internal/store/master_test.go`:
```go
package store

import (
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
```
(Add `"database/sql"` to the import block when you paste this second test.)

- [ ] **Step 6: Run to verify it fails**

Run: `go test ./internal/store/ -run TestInsertMaster -v`
Expected: FAIL — `undefined: InsertMaster`.

- [ ] **Step 7: Write the master insert**

Create `internal/store/master.go`:
```go
package store

import (
	"database/sql"
	"fmt"

	"github.com/cehbz/discogs/internal/parse"
)

// nullIfZero returns nil for 0 so the column stores SQL NULL, else n.
func nullIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func InsertMaster(tx *sql.Tx, m *parse.Master) error {
	if _, err := tx.Exec(
		`INSERT INTO master(id,main_release_id,title,year,data_quality) VALUES(?,?,?,?,?)`,
		m.ID, nullIfZero(m.MainRelease), m.Title, nullIfZero(m.Year), m.DataQuality); err != nil {
		return fmt.Errorf("insert master %d: %w", m.ID, err)
	}
	for i, c := range m.Artists {
		if _, err := tx.Exec(
			`INSERT INTO master_artist(master_id,seq,artist_id,anv,join_str,role) VALUES(?,?,?,?,?,?)`,
			m.ID, i, c.ID, c.ANV, c.Join, c.Role); err != nil {
			return fmt.Errorf("insert master_artist %d/%d: %w", m.ID, i, err)
		}
	}
	for i, g := range m.Genres {
		if _, err := tx.Exec(`INSERT INTO master_genre(master_id,seq,genre) VALUES(?,?,?)`, m.ID, i, g); err != nil {
			return fmt.Errorf("insert master_genre %d/%d: %w", m.ID, i, err)
		}
	}
	for i, s := range m.Styles {
		if _, err := tx.Exec(`INSERT INTO master_style(master_id,seq,style) VALUES(?,?,?)`, m.ID, i, s); err != nil {
			return fmt.Errorf("insert master_style %d/%d: %w", m.ID, i, err)
		}
	}
	for i, v := range m.Videos {
		if _, err := tx.Exec(
			`INSERT INTO master_video(master_id,seq,uri,title,description,duration) VALUES(?,?,?,?,?,?)`,
			m.ID, i, v.URI, v.Title, v.Description, nullIfZero(v.Duration)); err != nil {
			return fmt.Errorf("insert master_video %d/%d: %w", m.ID, i, err)
		}
	}
	return nil
}
```

- [ ] **Step 8: Run to verify it passes**

Run: `go test ./internal/store/ -run TestInsertMaster -v`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/parse/master.go internal/parse/master_test.go internal/store/master.go internal/store/master_test.go
git commit -m "feat: master ingest"
```

---

## Task 5: Releases — parse + insert core & flat children

**Files:**
- Create: `internal/parse/release.go`, `internal/store/release.go`
- Test: `internal/parse/release_test.go`, `internal/store/release_test.go`

**Interfaces:**
- Consumes: `parse.streamRecords`, `parse.ArtistCredit`, `parse.Video`, `store.nullIfZero`, `newTestDB`.
- Produces:
  - `parse.Release` and nested `parse.ReleaseLabel{ID int; Name, Catno string}`, `parse.Format{Name, Qty, Text string; Descriptions []string}`, `parse.Identifier{Type, Value, Description string}`, `parse.Company{ID int; Name, Catno, EntityType, EntityTypeName string}`, `parse.MasterRef{ID int; IsMainRelease bool}`, `parse.Track{Position, Title, Duration string; Artists, ExtraArtists []ArtistCredit; SubTracks []Track}`.
  - `parse.ParseReleases(r io.Reader, fn func(*Release) error) error`
  - `store.InsertRelease(tx *sql.Tx, r *parse.Release) error` (this task inserts the release row + all flat children; **tracklist is added in Task 6**)
  - `store.insertReleaseCredits(tx *sql.Tx, releaseID int, kind string, credits []parse.ArtistCredit) error`

- [ ] **Step 1: Write the failing parser test**

Create `internal/parse/release_test.go`:
```go
package parse

import (
	"strings"
	"testing"
)

const releaseXML = `<releases>
<release id="3" status="Accepted">
  <artists><artist><id>3</id><name>Josh Wink</name><anv/><join/><role/><tracks/></artist></artists>
  <title>Profound Sounds Vol. 1</title>
  <labels><label name="Ruffhouse Records" catno="CK 63628" id="6"/></labels>
  <extraartists><artist><id>99</id><name>Some Engineer</name><role>Mastered By</role></artist></extraartists>
  <formats><format name="CD" qty="1" text="">
    <descriptions><description>Compilation</description><description>Mixed</description></descriptions>
  </format></formats>
  <genres><genre>Electronic</genre></genres>
  <styles><style>Techno</style></styles>
  <country>US</country><released>1999-07-13</released><data_quality>Correct</data_quality>
  <master_id is_main_release="false">66526</master_id>
  <tracklist><track><position>1</position><title>Untitled 8</title><duration>7:00</duration></track></tracklist>
  <identifiers><identifier type="Barcode" value="074646362822" description="text"/></identifiers>
  <companies><company><id>5</id><name>PolyGram</name><catno>x</catno><entity_type>13</entity_type><entity_type_name>Manufactured By</entity_type_name></company></companies>
  <videos><video src="https://y.t/v" duration="420"><title>V</title><description>d</description></video></videos>
</release>
</releases>`

func TestParseReleases(t *testing.T) {
	var got []*Release
	if err := ParseReleases(strings.NewReader(releaseXML), func(r *Release) error { got = append(got, r); return nil }); err != nil {
		t.Fatalf("ParseReleases: %v", err)
	}
	r := got[0]
	if r.ID != 3 || r.Status != "Accepted" || r.Title != "Profound Sounds Vol. 1" || r.Country != "US" || r.Released != "1999-07-13" {
		t.Fatalf("core: %+v", r)
	}
	if r.Master == nil || r.Master.ID != 66526 || r.Master.IsMainRelease {
		t.Fatalf("master ref: %+v", r.Master)
	}
	if len(r.Artists) != 1 || r.Artists[0].ID != 3 {
		t.Fatalf("artists: %+v", r.Artists)
	}
	if len(r.ExtraArtists) != 1 || r.ExtraArtists[0].Role != "Mastered By" {
		t.Fatalf("extraartists: %+v", r.ExtraArtists)
	}
	if len(r.Labels) != 1 || r.Labels[0].Catno != "CK 63628" || r.Labels[0].ID != 6 {
		t.Fatalf("labels: %+v", r.Labels)
	}
	if len(r.Formats) != 1 || len(r.Formats[0].Descriptions) != 2 || r.Formats[0].Name != "CD" {
		t.Fatalf("formats: %+v", r.Formats)
	}
	if len(r.Identifiers) != 1 || r.Identifiers[0].Value != "074646362822" {
		t.Fatalf("identifiers: %+v", r.Identifiers)
	}
	if len(r.Companies) != 1 || r.Companies[0].EntityTypeName != "Manufactured By" {
		t.Fatalf("companies: %+v", r.Companies)
	}
	if len(r.Tracklist) != 1 || r.Tracklist[0].Title != "Untitled 8" {
		t.Fatalf("tracklist: %+v", r.Tracklist)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/parse/ -run TestParseReleases -v`
Expected: FAIL — `undefined: ParseReleases`.

- [ ] **Step 3: Write the release parser**

Create `internal/parse/release.go`:
```go
package parse

import "io"

type MasterRef struct {
	ID            int  `xml:",chardata"`
	IsMainRelease bool `xml:"is_main_release,attr"`
}

type ReleaseLabel struct {
	ID    int    `xml:"id,attr"`
	Name  string `xml:"name,attr"`
	Catno string `xml:"catno,attr"`
}

type Format struct {
	Name         string   `xml:"name,attr"`
	Qty          string   `xml:"qty,attr"`
	Text         string   `xml:"text,attr"`
	Descriptions []string `xml:"descriptions>description"`
}

type Identifier struct {
	Type        string `xml:"type,attr"`
	Value       string `xml:"value,attr"`
	Description string `xml:"description,attr"`
}

type Company struct {
	ID             int    `xml:"id"`
	Name           string `xml:"name"`
	Catno          string `xml:"catno"`
	EntityType     string `xml:"entity_type"`
	EntityTypeName string `xml:"entity_type_name"`
}

type Track struct {
	Position     string         `xml:"position"`
	Title        string         `xml:"title"`
	Duration     string         `xml:"duration"`
	Artists      []ArtistCredit `xml:"artists>artist"`
	ExtraArtists []ArtistCredit `xml:"extraartists>artist"`
	SubTracks    []Track        `xml:"sub_tracks>track"`
}

type Release struct {
	ID           int            `xml:"id,attr"`
	Status       string         `xml:"status,attr"`
	Title        string         `xml:"title"`
	Country      string         `xml:"country"`
	Released     string         `xml:"released"`
	Notes        string         `xml:"notes"`
	DataQuality  string         `xml:"data_quality"`
	Master       *MasterRef     `xml:"master_id"`
	Artists      []ArtistCredit `xml:"artists>artist"`
	ExtraArtists []ArtistCredit `xml:"extraartists>artist"`
	Labels       []ReleaseLabel `xml:"labels>label"`
	Formats      []Format       `xml:"formats>format"`
	Genres       []string       `xml:"genres>genre"`
	Styles       []string       `xml:"styles>style"`
	Identifiers  []Identifier   `xml:"identifiers>identifier"`
	Companies    []Company      `xml:"companies>company"`
	Videos       []Video        `xml:"videos>video"`
	Tracklist    []Track        `xml:"tracklist>track"`
}

func ParseReleases(r io.Reader, fn func(*Release) error) error {
	return streamRecords(r, "release", fn)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/parse/ -run TestParseReleases -v`
Expected: PASS.

- [ ] **Step 5: Write the failing insert test (flat children only)**

Create `internal/store/release_test.go`:
```go
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
```

- [ ] **Step 6: Run to verify it fails**

Run: `go test ./internal/store/ -run TestInsertRelease_Flat -v`
Expected: FAIL — `undefined: InsertRelease`.

- [ ] **Step 7: Write the release insert (flat children; tracklist call is added in Task 6)**

Create `internal/store/release.go`:
```go
package store

import (
	"database/sql"
	"fmt"

	"github.com/cehbz/discogs/internal/parse"
)

func InsertRelease(tx *sql.Tx, r *parse.Release) error {
	var masterID any
	isMain := 0
	if r.Master != nil && r.Master.ID != 0 {
		masterID = r.Master.ID
		if r.Master.IsMainRelease {
			isMain = 1
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO release(id,status,title,country,released_raw,notes,data_quality,master_id,is_main_release)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Status, r.Title, r.Country, r.Released, r.Notes, r.DataQuality, masterID, isMain); err != nil {
		return fmt.Errorf("insert release %d: %w", r.ID, err)
	}
	if err := insertReleaseCredits(tx, r.ID, "main", r.Artists); err != nil {
		return err
	}
	if err := insertReleaseCredits(tx, r.ID, "extra", r.ExtraArtists); err != nil {
		return err
	}
	for i, l := range r.Labels {
		if _, err := tx.Exec(
			`INSERT INTO release_label(release_id,seq,label_id,name,catno) VALUES(?,?,?,?,?)`,
			r.ID, i, nullIfZero(l.ID), l.Name, l.Catno); err != nil {
			return fmt.Errorf("insert release_label %d/%d: %w", r.ID, i, err)
		}
	}
	for i, f := range r.Formats {
		res, err := tx.Exec(
			`INSERT INTO release_format(release_id,seq,name,qty,text) VALUES(?,?,?,?,?)`,
			r.ID, i, f.Name, f.Qty, f.Text)
		if err != nil {
			return fmt.Errorf("insert release_format %d/%d: %w", r.ID, i, err)
		}
		fid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for j, d := range f.Descriptions {
			if _, err := tx.Exec(
				`INSERT INTO release_format_description(format_id,seq,description) VALUES(?,?,?)`,
				fid, j, d); err != nil {
				return fmt.Errorf("insert format_desc %d/%d: %w", r.ID, j, err)
			}
		}
	}
	for i, g := range r.Genres {
		if _, err := tx.Exec(`INSERT INTO release_genre(release_id,seq,genre) VALUES(?,?,?)`, r.ID, i, g); err != nil {
			return fmt.Errorf("insert release_genre %d/%d: %w", r.ID, i, err)
		}
	}
	for i, s := range r.Styles {
		if _, err := tx.Exec(`INSERT INTO release_style(release_id,seq,style) VALUES(?,?,?)`, r.ID, i, s); err != nil {
			return fmt.Errorf("insert release_style %d/%d: %w", r.ID, i, err)
		}
	}
	for i, id := range r.Identifiers {
		if _, err := tx.Exec(
			`INSERT INTO release_identifier(release_id,seq,type,value,description) VALUES(?,?,?,?,?)`,
			r.ID, i, id.Type, id.Value, id.Description); err != nil {
			return fmt.Errorf("insert release_identifier %d/%d: %w", r.ID, i, err)
		}
	}
	for i, c := range r.Companies {
		if _, err := tx.Exec(
			`INSERT INTO release_company(release_id,seq,company_label_id,name,catno,entity_type,entity_type_name)
			 VALUES(?,?,?,?,?,?,?)`,
			r.ID, i, nullIfZero(c.ID), c.Name, c.Catno, c.EntityType, c.EntityTypeName); err != nil {
			return fmt.Errorf("insert release_company %d/%d: %w", r.ID, i, err)
		}
	}
	for i, v := range r.Videos {
		if _, err := tx.Exec(
			`INSERT INTO release_video(release_id,seq,uri,title,description,duration) VALUES(?,?,?,?,?,?)`,
			r.ID, i, v.URI, v.Title, v.Description, nullIfZero(v.Duration)); err != nil {
			return fmt.Errorf("insert release_video %d/%d: %w", r.ID, i, err)
		}
	}
	return nil
}

func insertReleaseCredits(tx *sql.Tx, releaseID int, kind string, credits []parse.ArtistCredit) error {
	for i, c := range credits {
		if _, err := tx.Exec(
			`INSERT INTO release_artist(release_id,seq,artist_id,anv,join_str,role,tracks,kind)
			 VALUES(?,?,?,?,?,?,?,?)`,
			releaseID, i, c.ID, c.ANV, c.Join, c.Role, c.Tracks, kind); err != nil {
			return fmt.Errorf("insert release_artist %d/%d: %w", releaseID, i, err)
		}
	}
	return nil
}
```

- [ ] **Step 8: Run to verify it passes**

Run: `go test ./internal/store/ -run TestInsertRelease_Flat -v`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/parse/release.go internal/parse/release_test.go internal/store/release.go internal/store/release_test.go
git commit -m "feat: release ingest (core and flat children)"
```

---

## Task 6: Releases — recursive tracklist

**Files:**
- Modify: `internal/store/release.go` (add `insertTracks`, `insertTrackCredits`; call from `InsertRelease`)
- Test: `internal/store/release_test.go` (add tracklist test)

**Interfaces:**
- Consumes: `parse.Track`, `parse.ArtistCredit`, existing `InsertRelease`.
- Produces: `store.insertTracks(tx *sql.Tx, releaseID int, parentID any, tracks []parse.Track) error`, `store.insertTrackCredits(tx *sql.Tx, trackID int64, kind string, credits []parse.ArtistCredit) error`. After this task, `InsertRelease` also persists `track` and `track_artist` rows including recursive `sub_tracks`.

- [ ] **Step 1: Write the failing tracklist test**

Add to `internal/store/release_test.go`:
```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/store/ -run TestInsertRelease_Tracklist -v`
Expected: FAIL — the `track` table is empty (`tracks=0 want 3`).

- [ ] **Step 3: Add the recursive track insert and wire it into InsertRelease**

In `internal/store/release.go`, add the call at the end of `InsertRelease` (just before its final `return nil`):
```go
	if err := insertTracks(tx, r.ID, nil, r.Tracklist); err != nil {
		return err
	}
```
Then append these functions to the file:
```go
func insertTracks(tx *sql.Tx, releaseID int, parentID any, tracks []parse.Track) error {
	for i, t := range tracks {
		res, err := tx.Exec(
			`INSERT INTO track(release_id,parent_track_id,seq,position,title,duration) VALUES(?,?,?,?,?,?)`,
			releaseID, parentID, i, t.Position, t.Title, t.Duration)
		if err != nil {
			return fmt.Errorf("insert track (release %d): %w", releaseID, err)
		}
		tid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if err := insertTrackCredits(tx, tid, "main", t.Artists); err != nil {
			return err
		}
		if err := insertTrackCredits(tx, tid, "extra", t.ExtraArtists); err != nil {
			return err
		}
		if len(t.SubTracks) > 0 {
			if err := insertTracks(tx, releaseID, tid, t.SubTracks); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertTrackCredits(tx *sql.Tx, trackID int64, kind string, credits []parse.ArtistCredit) error {
	for i, c := range credits {
		if _, err := tx.Exec(
			`INSERT INTO track_artist(track_id,seq,artist_id,anv,join_str,role,kind) VALUES(?,?,?,?,?,?,?)`,
			trackID, i, c.ID, c.ANV, c.Join, c.Role, kind); err != nil {
			return fmt.Errorf("insert track_artist (track %d): %w", trackID, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/store/ -run TestInsertRelease -v`
Expected: PASS (both `_Flat` and `_Tracklist`).

- [ ] **Step 5: Commit**

```bash
git add internal/store/release.go internal/store/release_test.go
git commit -m "feat: recursive tracklist and track credits"
```

---

## Task 7: FTS population + secondary indexes

**Files:**
- Create: `internal/store/fts.go`
- Test: `internal/store/fts_test.go`

**Interfaces:**
- Consumes: populated base tables; `newTestDB`, `InsertArtist`, `InsertRelease`.
- Produces: `store.BuildFTS(db *sql.DB) error`, `store.BuildIndexes(db *sql.DB) error`.

- [ ] **Step 1: Write the failing FTS test**

Create `internal/store/fts_test.go`:
```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/store/ -run 'TestBuildFTS|TestBuildIndexes' -v`
Expected: FAIL — `undefined: BuildFTS`.

- [ ] **Step 3: Write the FTS and index builders**

Create `internal/store/fts.go`:
```go
package store

import "database/sql"

// BuildFTS populates the contentless FTS5 tables from the base tables. Run after
// all rows are loaded. artist_names is a denormalized bag of credited names.
func BuildFTS(db *sql.DB) error {
	stmts := []string{
		`INSERT INTO artist_fts(rowid,name) SELECT id, COALESCE(name,'') FROM artist`,
		`INSERT INTO label_fts(rowid,name) SELECT id, COALESCE(name,'') FROM label`,
		`INSERT INTO master_fts(rowid,title,artist_names)
		   SELECT m.id, COALESCE(m.title,''),
		     COALESCE((SELECT group_concat(COALESCE(NULLIF(ma.anv,''), a.name), ' ')
		                 FROM master_artist ma JOIN artist a ON a.id = ma.artist_id
		                WHERE ma.master_id = m.id), '')
		   FROM master m`,
		`INSERT INTO release_fts(rowid,title,artist_names)
		   SELECT r.id, COALESCE(r.title,''),
		     COALESCE((SELECT group_concat(COALESCE(NULLIF(ra.anv,''), a.name), ' ')
		                 FROM release_artist ra JOIN artist a ON a.id = ra.artist_id
		                WHERE ra.release_id = r.id AND ra.kind = 'main'), '')
		   FROM release r`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

// BuildIndexes creates secondary indexes on FK/join columns. Idempotent.
func BuildIndexes(db *sql.DB) error {
	idx := []string{
		`CREATE INDEX IF NOT EXISTS idx_release_artist_release ON release_artist(release_id)`,
		`CREATE INDEX IF NOT EXISTS idx_release_artist_artist ON release_artist(artist_id)`,
		`CREATE INDEX IF NOT EXISTS idx_track_release ON track(release_id)`,
		`CREATE INDEX IF NOT EXISTS idx_track_parent ON track(parent_track_id)`,
		`CREATE INDEX IF NOT EXISTS idx_track_artist_track ON track_artist(track_id)`,
		`CREATE INDEX IF NOT EXISTS idx_release_label_release ON release_label(release_id)`,
		`CREATE INDEX IF NOT EXISTS idx_release_format_release ON release_format(release_id)`,
		`CREATE INDEX IF NOT EXISTS idx_rfd_format ON release_format_description(format_id)`,
		`CREATE INDEX IF NOT EXISTS idx_release_identifier_release ON release_identifier(release_id)`,
		`CREATE INDEX IF NOT EXISTS idx_release_company_release ON release_company(release_id)`,
		`CREATE INDEX IF NOT EXISTS idx_master_artist_master ON master_artist(master_id)`,
		`CREATE INDEX IF NOT EXISTS idx_release_master ON release(master_id)`,
		`CREATE INDEX IF NOT EXISTS idx_master_main_release ON master(main_release_id)`,
	}
	for _, s := range idx {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/store/ -run 'TestBuildFTS|TestBuildIndexes' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/fts.go internal/store/fts_test.go
git commit -m "feat: FTS5 population and secondary indexes"
```

---

## Task 8: Referential integrity report + meta

**Files:**
- Create: `internal/store/integrity.go`, `internal/store/meta.go`
- Test: `internal/store/integrity_test.go`

**Interfaces:**
- Consumes: populated tables; `newTestDB`, `InsertRelease`, `InsertArtist`.
- Produces:
  - `store.IntegrityReport{Orphans map[string]int64}`
  - `store.RunIntegrityReport(db *sql.DB) (*IntegrityReport, error)`
  - `store.SetMeta(tx *sql.Tx, kv map[string]string) error`

- [ ] **Step 1: Write the failing integrity test**

Create `internal/store/integrity_test.go`:
```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/store/ -run 'TestRunIntegrity|TestSetMeta' -v`
Expected: FAIL — `undefined: RunIntegrityReport`.

- [ ] **Step 3: Write the integrity report and meta helpers**

Create `internal/store/integrity.go`:
```go
package store

import (
	"database/sql"
	"fmt"
)

// IntegrityReport holds orphan counts per documented foreign-key relationship.
type IntegrityReport struct {
	Orphans map[string]int64
}

// checks maps a relationship label to a query counting rows whose referent is absent.
// LEFT JOIN ... IS NULL is used (fast and NULL-safe vs NOT IN).
var integrityChecks = map[string]string{
	"release.master_id->master": `
		SELECT count(*) FROM release r LEFT JOIN master m ON m.id = r.master_id
		WHERE r.master_id IS NOT NULL AND m.id IS NULL`,
	"master.main_release_id->release": `
		SELECT count(*) FROM master m LEFT JOIN release r ON r.id = m.main_release_id
		WHERE m.main_release_id IS NOT NULL AND r.id IS NULL`,
	"release_artist.artist_id->artist": `
		SELECT count(*) FROM release_artist ra LEFT JOIN artist a ON a.id = ra.artist_id
		WHERE a.id IS NULL`,
	"track_artist.artist_id->artist": `
		SELECT count(*) FROM track_artist ta LEFT JOIN artist a ON a.id = ta.artist_id
		WHERE a.id IS NULL`,
	"master_artist.artist_id->artist": `
		SELECT count(*) FROM master_artist ma LEFT JOIN artist a ON a.id = ma.artist_id
		WHERE a.id IS NULL`,
	"release_label.label_id->label": `
		SELECT count(*) FROM release_label rl LEFT JOIN label l ON l.id = rl.label_id
		WHERE rl.label_id IS NOT NULL AND l.id IS NULL`,
}

func RunIntegrityReport(db *sql.DB) (*IntegrityReport, error) {
	rep := &IntegrityReport{Orphans: make(map[string]int64, len(integrityChecks))}
	for name, q := range integrityChecks {
		var n int64
		if err := db.QueryRow(q).Scan(&n); err != nil {
			return nil, fmt.Errorf("integrity check %s: %w", name, err)
		}
		rep.Orphans[name] = n
	}
	return rep, nil
}
```

Create `internal/store/meta.go`:
```go
package store

import (
	"database/sql"
	"fmt"
)

func SetMeta(tx *sql.Tx, kv map[string]string) error {
	for k, v := range kv {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO meta(key,value) VALUES(?,?)`, k, v); err != nil {
			return fmt.Errorf("set meta %s: %w", k, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/store/ -run 'TestRunIntegrity|TestSetMeta' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/integrity.go internal/store/meta.go internal/store/integrity_test.go
git commit -m "feat: referential integrity report and meta provenance"
```

---

## Task 9: Dump fetcher (download + SHA-256 verify)

**Files:**
- Create: `internal/dumps/dumps.go`
- Test: `internal/dumps/dumps_test.go`

**Interfaces:**
- Consumes: nothing internal.
- Produces:
  - `dumps.Types = []string{"artists","labels","masters","releases"}`
  - `dumps.FileName(date, typ string) string`, `dumps.ChecksumName(date string) string`
  - `dumps.Download(ctx context.Context, baseURL, date, outDir string, client *http.Client) error`
  - `dumps.VerifyChecksums(dir, date string) error`

- [ ] **Step 1: Write the failing test**

Create `internal/dumps/dumps_test.go`:
```go
package dumps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeServer serves four tiny "dump" files plus a CHECKSUM.txt computed over them.
func fakeServer(t *testing.T, date string) *httptest.Server {
	t.Helper()
	bodies := map[string]string{}
	var lines []string
	for _, typ := range Types {
		name := FileName(date, typ)
		body := "GZIPDATA-" + typ
		bodies[name] = body
		sum := sha256.Sum256([]byte(body))
		lines = append(lines, fmt.Sprintf("%s  %s", hex.EncodeToString(sum[:]), name))
	}
	bodies[ChecksumName(date)] = strings.Join(lines, "\n") + "\n"

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dl := r.URL.Query().Get("download") // e.g. data/2026/discogs_20260601_releases.xml.gz
		name := dl[strings.LastIndex(dl, "/")+1:]
		body, ok := bodies[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
}

func TestDownloadAndVerify(t *testing.T) {
	const date = "20260601"
	srv := fakeServer(t, date)
	defer srv.Close()
	dir := t.TempDir()

	if err := Download(context.Background(), srv.URL, date, dir, srv.Client()); err != nil {
		t.Fatalf("Download: %v", err)
	}
	for _, typ := range Types {
		if _, err := os.Stat(filepath.Join(dir, FileName(date, typ))); err != nil {
			t.Fatalf("missing %s: %v", typ, err)
		}
	}
	if err := VerifyChecksums(dir, date); err != nil {
		t.Fatalf("VerifyChecksums: %v", err)
	}
}

func TestVerifyChecksums_DetectsCorruption(t *testing.T) {
	const date = "20260601"
	srv := fakeServer(t, date)
	defer srv.Close()
	dir := t.TempDir()
	if err := Download(context.Background(), srv.URL, date, dir, srv.Client()); err != nil {
		t.Fatal(err)
	}
	// corrupt one file
	os.WriteFile(filepath.Join(dir, FileName(date, "labels")), []byte("tampered"), 0o644)
	if err := VerifyChecksums(dir, date); err == nil {
		t.Fatalf("expected checksum mismatch error, got nil")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/dumps/ -v`
Expected: FAIL — `undefined: Download`.

- [ ] **Step 3: Write the fetcher**

Create `internal/dumps/dumps.go`:
```go
package dumps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Types are the four Discogs dump categories, in load order.
var Types = []string{"artists", "labels", "masters", "releases"}

// DefaultBaseURL is the public Discogs dumps front end.
const DefaultBaseURL = "https://data.discogs.com"

func FileName(date, typ string) string {
	return fmt.Sprintf("discogs_%s_%s.xml.gz", date, typ)
}

func ChecksumName(date string) string {
	return fmt.Sprintf("discogs_%s_CHECKSUM.txt", date)
}

// downloadURL builds the front-end download URL: base/?download=data%2F<year>%2F<file>
func downloadURL(baseURL, date, file string) string {
	year := date[:4]
	return fmt.Sprintf("%s/?download=data%%2F%s%%2F%s", strings.TrimRight(baseURL, "/"), year, file)
}

// Download fetches the four dump files and the CHECKSUM file into outDir.
func Download(ctx context.Context, baseURL, date, outDir string, client *http.Client) error {
	if client == nil {
		client = http.DefaultClient
	}
	files := make([]string, 0, len(Types)+1)
	for _, typ := range Types {
		files = append(files, FileName(date, typ))
	}
	files = append(files, ChecksumName(date))

	for _, file := range files {
		if err := downloadOne(ctx, client, downloadURL(baseURL, date, file), filepath.Join(outDir, file)); err != nil {
			return fmt.Errorf("download %s: %w", file, err)
		}
	}
	return nil
}

func downloadOne(ctx context.Context, client *http.Client, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// VerifyChecksums recomputes SHA-256 for each file listed in the CHECKSUM file and
// compares. Returns an error on the first mismatch or missing file.
func VerifyChecksums(dir, date string) error {
	data, err := os.ReadFile(filepath.Join(dir, ChecksumName(date)))
	if err != nil {
		return fmt.Errorf("read checksum file: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		want, name := fields[0], fields[1]
		got, err := sha256File(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("hash %s: %w", name, err)
		}
		if !strings.EqualFold(got, want) {
			return fmt.Errorf("checksum mismatch for %s: got %s want %s", name, got, want)
		}
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/dumps/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/dumps/dumps.go internal/dumps/dumps_test.go
git commit -m "feat: dump fetcher with SHA-256 verification"
```

---

## Task 10: Import orchestrator + CLI

**Files:**
- Create: `internal/importer/importer.go`, `internal/importer/importer_test.go`
- Create: `cmd/discogs/main.go`, `cmd/discogs/fetch.go`, `cmd/discogs/verify.go`, `cmd/discogs/import.go`

**Interfaces:**
- Consumes: `parse.Parse*`, `store.*`, `dumps.*`.
- Produces:
  - `importer.Report{Counts map[string]int64; Integrity *store.IntegrityReport}`
  - `importer.Import(dbPath, dir, date string) (*Report, error)`
  - cobra command tree: `discogs fetch|verify|import`.

- [ ] **Step 1: Write the failing orchestrator test**

Create `internal/importer/importer_test.go`:
```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/importer/ -v`
Expected: FAIL — `undefined: Import`.

- [ ] **Step 3: Write the orchestrator**

Create `internal/importer/importer.go`:
```go
package importer

import (
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/cehbz/discogs/internal/dumps"
	"github.com/cehbz/discogs/internal/parse"
	"github.com/cehbz/discogs/internal/store"
	_ "modernc.org/sqlite"
)

const batchSize = 50000

type Report struct {
	Counts    map[string]int64
	Integrity *store.IntegrityReport
}

// Import builds a fresh SQLite DB at dbPath from the four dumps in dir for date.
func Import(dbPath, dir, date string) (*Report, error) {
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove existing db: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1) // single writer; keeps PRAGMAs and tx on one connection

	for _, p := range []string{
		`PRAGMA journal_mode=OFF`, `PRAGMA synchronous=OFF`,
		`PRAGMA temp_store=MEMORY`, `PRAGMA cache_size=-200000`, `PRAGMA foreign_keys=OFF`,
	} {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}
	if err := store.CreateSchema(db); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	counts := map[string]int64{}
	path := func(typ string) string { return filepath.Join(dir, dumps.FileName(date, typ)) }

	if counts["artists"], err = importStream(db, path("artists"), parse.ParseArtists, store.InsertArtist); err != nil {
		return nil, fmt.Errorf("import artists: %w", err)
	}
	if counts["labels"], err = importStream(db, path("labels"), parse.ParseLabels, store.InsertLabel); err != nil {
		return nil, fmt.Errorf("import labels: %w", err)
	}
	if counts["masters"], err = importStream(db, path("masters"), parse.ParseMasters, store.InsertMaster); err != nil {
		return nil, fmt.Errorf("import masters: %w", err)
	}
	if counts["releases"], err = importStream(db, path("releases"), parse.ParseReleases, store.InsertRelease); err != nil {
		return nil, fmt.Errorf("import releases: %w", err)
	}

	if err := store.BuildFTS(db); err != nil {
		return nil, fmt.Errorf("build fts: %w", err)
	}
	if err := store.BuildIndexes(db); err != nil {
		return nil, fmt.Errorf("build indexes: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	meta := map[string]string{"dump_date": date, "importer_version": "1"}
	for typ, n := range counts {
		meta["count_"+typ] = strconv.FormatInt(n, 10)
	}
	if err := store.SetMeta(tx, meta); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	for _, p := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA synchronous=NORMAL`} {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}

	rep, err := store.RunIntegrityReport(db)
	if err != nil {
		return nil, fmt.Errorf("integrity report: %w", err)
	}
	return &Report{Counts: counts, Integrity: rep}, nil
}

// importStream streams one gzip dump file through parseFn, inserting each record via
// insert, committing every batchSize records. T is the parsed record type.
func importStream[T any](
	db *sql.DB,
	path string,
	parseFn func(io.Reader, func(*T) error) error,
	insert func(*sql.Tx, *T) error,
) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	defer gz.Close()

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	var n int64
	perr := parseFn(gz, func(rec *T) error {
		if err := insert(tx, rec); err != nil {
			return err
		}
		n++
		if n%batchSize == 0 {
			if err := tx.Commit(); err != nil {
				return err
			}
			tx, err = db.Begin()
			return err
		}
		return nil
	})
	if perr != nil {
		_ = tx.Rollback()
		return n, perr
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/importer/ -v`
Expected: PASS.

- [ ] **Step 5: Add cobra and write the CLI**

Run: `go get github.com/spf13/cobra`

Create `cmd/discogs/main.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "discogs",
		Short: "Build and maintain a local Discogs SQLite mirror",
	}
	root.AddCommand(fetchCmd(), verifyCmd(), importCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

Create `cmd/discogs/fetch.go`:
```go
package main

import (
	"context"

	"github.com/cehbz/discogs/internal/dumps"
	"github.com/spf13/cobra"
)

func fetchCmd() *cobra.Command {
	var date, dir, baseURL string
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Download and verify the four dumps for a given date (YYYYMMDD)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := dumps.Download(context.Background(), baseURL, date, dir, nil); err != nil {
				return err
			}
			return dumps.VerifyChecksums(dir, date)
		},
	}
	cmd.Flags().StringVar(&date, "date", "", "dump date YYYYMMDD (required)")
	cmd.Flags().StringVar(&dir, "dir", ".", "output directory")
	cmd.Flags().StringVar(&baseURL, "base-url", dumps.DefaultBaseURL, "dumps base URL")
	cmd.MarkFlagRequired("date")
	return cmd
}
```

Create `cmd/discogs/verify.go`:
```go
package main

import (
	"github.com/cehbz/discogs/internal/dumps"
	"github.com/spf13/cobra"
)

func verifyCmd() *cobra.Command {
	var date, dir string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify SHA-256 checksums of already-downloaded dumps",
		RunE: func(cmd *cobra.Command, args []string) error {
			return dumps.VerifyChecksums(dir, date)
		},
	}
	cmd.Flags().StringVar(&date, "date", "", "dump date YYYYMMDD (required)")
	cmd.Flags().StringVar(&dir, "dir", ".", "directory containing the dumps")
	cmd.MarkFlagRequired("date")
	return cmd
}
```

Create `cmd/discogs/import.go`:
```go
package main

import (
	"fmt"
	"sort"

	"github.com/cehbz/discogs/internal/importer"
	"github.com/spf13/cobra"
)

func importCmd() *cobra.Command {
	var date, dir, out string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Build a fresh SQLite mirror from the dumps in --dir",
		RunE: func(cmd *cobra.Command, args []string) error {
			if out == "" {
				out = fmt.Sprintf("discogs-%s.db", date)
			}
			rep, err := importer.Import(out, dir, date)
			if err != nil {
				return err
			}
			fmt.Printf("Imported into %s\n", out)
			for _, typ := range []string{"artists", "labels", "masters", "releases"} {
				fmt.Printf("  %-9s %d\n", typ, rep.Counts[typ])
			}
			fmt.Println("Referential integrity (orphan counts):")
			keys := make([]string, 0, len(rep.Integrity.Orphans))
			for k := range rep.Integrity.Orphans {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("  %-40s %d\n", k, rep.Integrity.Orphans[k])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&date, "date", "", "dump date YYYYMMDD (required)")
	cmd.Flags().StringVar(&dir, "dir", ".", "directory containing the dumps")
	cmd.Flags().StringVar(&out, "out", "", "output db path (default discogs-<date>.db)")
	cmd.MarkFlagRequired("date")
	return cmd
}
```

- [ ] **Step 6: Build and verify the whole project**

Run:
```bash
go build ./...
go test ./...
```
Expected: build succeeds; all tests PASS.

- [ ] **Step 7: Manual smoke test of the CLI**

Run:
```bash
go run ./cmd/discogs --help
go run ./cmd/discogs import --help
```
Expected: help text lists `fetch`, `verify`, `import`; `import` shows `--date`, `--dir`, `--out`.

- [ ] **Step 8: Commit**

```bash
git add internal/importer/ cmd/ go.mod go.sum
git commit -m "feat: import orchestrator and discogs CLI"
```

---

## Post-implementation (manual, outside the task loop)

These require the real ~11 GB of dumps and are not unit-testable; run them once the
code is merged, on the X10:

1. `discogs fetch --date <YYYYMMDD> --dir /Volumes/X10/discogs/dumps`
2. `discogs import --date <YYYYMMDD> --dir /Volumes/X10/discogs/dumps --out /Volumes/X10/discogs/discogs-<YYYYMMDD>.db`
3. Record the import wall-clock and final DB size in the spec's "To confirm" section.
4. Review the printed integrity report; for any relationship with 0 orphans, consider
   adding an enforced `FOREIGN KEY` to `schema.go` (a follow-on change).
5. Repoint the `discogs.db` symlink: `ln -sf discogs-<YYYYMMDD>.db /Volumes/X10/discogs/discogs.db`.
6. **Performance note:** if import time is unacceptable, the first lever is prepared
   statements reused across batches (replace per-call `tx.Exec` with `tx.Stmt` of
   statements prepared once on the connection). The PRAGMAs and batched commits
   already in place are the dominant speedups; add prepared statements only if needed.

---

## Self-Review

**Spec coverage:**
- Four dump types imported — Tasks 2–6 (labels, artists, masters, releases). ✓
- Full 3NF schema, IDs as PKs, vocab as text — Task 1 schema. ✓
- FTS5 contentless, diacritic-insensitive, id-returning — Tasks 1 (schema) + 7 (populate). ✓
- Streaming parse over gzip — `parse.streamRecords` (Task 2) + `importer.importStream` (Task 10). ✓
- Fresh-DB build + meta provenance — Task 10 (`os.Remove` + `SetMeta`) / Task 8 (`SetMeta`). ✓
- Import-time vs serving PRAGMAs — Task 10. ✓
- Referential integrity report (FK off, post-import) — Task 8 + Task 10 wiring. ✓
- Fetcher download + SHA-256 verify — Task 9. ✓
- CLI fetch/verify/import — Task 10. ✓
- MB-readiness (integer-id PKs, SQLite) — satisfied structurally by Task 1; no code action. ✓
- Recursive sub_tracks + per-track credits — Task 6. ✓

**Placeholder scan:** No TBD/TODO; every code step contains complete code. The only
deferred item (prepared-statement optimization) is explicitly a post-implementation,
measure-first note, not a gap in the buildable deliverable.

**Type consistency:** `parse.ArtistCredit` fields (ID/Name/ANV/Join/Role/Tracks) used
consistently in masters/releases/tracks. `store.nullIfZero` defined in Task 4, reused
in Task 5. `importStream` generic signature matches every `parse.ParseX`
(`func(io.Reader, func(*T) error) error`) and every `store.InsertX`
(`func(*sql.Tx, *T) error`). `IntegrityReport`/`Report` names consistent across Tasks
8 and 10. `newTestDB` defined in Task 1, reused by all store tests.

---

## Execution Handoff

Two execution options for this plan:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — execute tasks in this session with checkpoints for review.
