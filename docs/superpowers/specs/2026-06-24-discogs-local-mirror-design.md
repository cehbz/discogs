# Local Discogs Mirror — Design

- **Date:** 2026-06-24
- **Status:** Approved (design); pending implementation plan
- **Repo:** `~/projects/discogs` (`github.com/cehbz/discogs`)

## Overview

A Go project that imports the four monthly Discogs XML data dumps into a single,
fully-normalized (3NF) **SQLite** database stored on the Crucial X10 external SSD.
Consuming tools (`classical-tagger` in Go, `tidalist` in Python) read the database
**directly through native SQLite bindings** — there is no HTTP service and no JSON
serialization layer. The SQLite file and its schema are the contract.

The database is designed to be joined, via SQLite `ATTACH`, against a future local
MusicBrainz SQLite database that will be enriched with Discogs keys. The only
requirement this imposes here is that Discogs' own integer IDs are the primary keys.

## Goals

- Full local mirror of all four Discogs dump types: `artists`, `labels`, `masters`, `releases`.
- Complete fidelity to the dump contents (every field except blank `<image>` data).
- 3NF relational schema, queryable by native SQLite bindings from any language.
- FTS5 full-text search on artist/label/master/release names and titles (hard requirement).
- Discogs native integer IDs as primary keys, for cross-DB joins with MusicBrainz.
- Manual, repeatable re-import from a chosen monthly dump.

## Non-goals (out of scope for this project)

- No HTTP/REST service; no JSON output.
- No vocabulary lookup tables — controlled vocabularies (genre, style, roles,
  identifier types, etc.) are stored as plain text on their tables.
- No incremental/delta updates — each import builds a fresh database file.
- No images (the dumps carry blank image URIs).
- Refactoring `classical-tagger` / `tidalist` to use the mirror is downstream work,
  not part of this project. Their query patterns are documented here only to verify
  the schema serves them.
- The MusicBrainz PG→SQLite conversion and MB-side Discogs-key enrichment are a
  separate, future project.

## Context & constraints

- **Storage:** system disk cannot hold the data; the mirror lives on the Crucial X10
  (treated as part of the laptop, almost always plugged in). hbd remains a future
  alternative; nothing here precludes copying the file there later.
- **Dumps** (verified 2026-06-24, primary source `data.discogs.com`):
  - CC0 licensed; monthly; `discogs_YYYYMMDD_{artists,labels,masters,releases}.xml.gz`.
  - One `discogs_YYYYMMDD_CHECKSUM.txt` (SHA-256, `sha256sum`-compatible).
  - Download via `https://data.discogs.com/?download=data%2F<YEAR>%2F<file>`
    (anonymous direct S3 access now returns 403).
  - Approx compressed sizes (2026): releases ~10.3 GB, masters ~580 MB,
    artists ~465 MB, labels ~84 MB. Uncompressed releases ~32 GB (estimate).
  - Record counts (reported, ~mid-2025): releases 18.4M, masters 2.46M,
    artists 9.17M (full credited-artist graph), labels 2.27M.
  - Each file is a stream of top-level records — **streaming/SAX parsing is mandatory**;
    a DOM parse of the releases file will exhaust memory.
  - No published XSD; structure follows the Discogs API spec.

## Architecture

Three cooperating units; the database is the deliverable.

1. **fetcher** (`internal/dumps`) — downloads the four `.gz` files and `CHECKSUM.txt`
   for a chosen dump date, verifies SHA-256.
2. **importer** (`internal/parse` + `internal/store`) — streams each `.gz`
   (`gzip.NewReader` → `encoding/xml` token decoder, one element at a time),
   inserting into the normalized tables in batched transactions.
3. **schema/store** (`internal/store`) — owns the 3NF schema, the insert layer, the
   FTS5 index build, secondary-index creation, and the post-import integrity report.

**Data flow:** `.gz` on X10 → importer (streaming) → SQLite on X10 → consuming tools
(native bindings) → (future) joined with `musicbrainz.db` via `ATTACH`.

**Refresh model:** each run builds a fresh `discogs-YYYYMMDD.db`. A `discogs.db`
symlink is repointed to the new file once it is verified. No in-place migration.

## Schema

Conventions:

- Core entities are keyed on Discogs' own integer id (`artist.id`, `label.id`,
  `master.id`, `release.id`).
- Multivalued / relationship data lives in child tables, ordered by a `seq` column
  preserving document order.
- Controlled vocabularies are stored as plain `TEXT` (no lookup tables).
- `join` is a reserved word in SQL, so the Discogs `join` attribute is column `join_str`.
- Foreign-key *relationships* are documented but enforcement is decided post-import
  (see "Referential integrity"). Some references legitimately dangle in Discogs data.

### Provenance

```sql
CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
-- keys: dump_date, imported_at, importer_version,
--       checksum_artists, checksum_labels, checksum_masters, checksum_releases,
--       count_artists, count_labels, count_masters, count_releases
```

### Core entities

```sql
CREATE TABLE artist (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL,
  realname     TEXT,
  profile      TEXT,
  data_quality TEXT
);

CREATE TABLE label (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL,
  contactinfo  TEXT,
  profile      TEXT,
  data_quality TEXT
);

CREATE TABLE master (
  id              INTEGER PRIMARY KEY,
  main_release_id INTEGER,   -- -> release.id (advisory; circular with release.master_id)
  title           TEXT,
  year            INTEGER,
  data_quality    TEXT
);

CREATE TABLE release (
  id              INTEGER PRIMARY KEY,
  status          TEXT,
  title           TEXT,
  country         TEXT,
  released_raw    TEXT,      -- free-text date as in dump ('1999-07-13', '1999', '')
  notes           TEXT,
  data_quality    TEXT,
  master_id       INTEGER,   -- -> master.id (advisory)
  is_main_release INTEGER    -- 0/1 from the master_id is_main_release attribute
);
```

### Artist relations (self-referential graph)

```sql
CREATE TABLE artist_namevariation (
  artist_id INTEGER NOT NULL,
  seq       INTEGER NOT NULL,
  name      TEXT NOT NULL,
  PRIMARY KEY (artist_id, seq)
);

CREATE TABLE artist_url (
  artist_id INTEGER NOT NULL,
  seq       INTEGER NOT NULL,
  url       TEXT NOT NULL,
  PRIMARY KEY (artist_id, seq)
);

CREATE TABLE artist_alias (
  artist_id       INTEGER NOT NULL,  -- -> artist.id
  alias_artist_id INTEGER NOT NULL,  -- -> artist.id
  name            TEXT,              -- alias display name from dump
  PRIMARY KEY (artist_id, alias_artist_id)
);

-- group <members> and artist <groups> are inverse views of the same relation;
-- both populate this table (PK dedupes).
CREATE TABLE artist_membership (
  group_id  INTEGER NOT NULL,  -- -> artist.id (the group)
  member_id INTEGER NOT NULL,  -- -> artist.id (the member)
  name      TEXT,              -- member display name from dump
  PRIMARY KEY (group_id, member_id)
);
```

### Label relations

```sql
CREATE TABLE label_url (
  label_id INTEGER NOT NULL,
  seq      INTEGER NOT NULL,
  url      TEXT NOT NULL,
  PRIMARY KEY (label_id, seq)
);

-- <parentLabel> and <sublabels> are inverse views of the same parent/child relation.
CREATE TABLE label_relationship (
  parent_label_id INTEGER NOT NULL,  -- -> label.id
  sublabel_id     INTEGER NOT NULL,  -- -> label.id
  PRIMARY KEY (parent_label_id, sublabel_id)
);
```

### Master relations

```sql
CREATE TABLE master_artist (
  master_id INTEGER NOT NULL,
  seq       INTEGER NOT NULL,
  artist_id INTEGER NOT NULL,  -- -> artist.id
  anv       TEXT,              -- artist name variation
  join_str  TEXT,
  role      TEXT,
  PRIMARY KEY (master_id, seq)
);

CREATE TABLE master_genre (
  master_id INTEGER NOT NULL,
  seq       INTEGER NOT NULL,
  genre     TEXT NOT NULL,
  PRIMARY KEY (master_id, seq)
);

CREATE TABLE master_style (
  master_id INTEGER NOT NULL,
  seq       INTEGER NOT NULL,
  style     TEXT NOT NULL,
  PRIMARY KEY (master_id, seq)
);

CREATE TABLE master_video (
  master_id   INTEGER NOT NULL,
  seq         INTEGER NOT NULL,
  uri         TEXT,
  title       TEXT,
  description TEXT,
  duration    INTEGER,  -- seconds; NULL if blank
  PRIMARY KEY (master_id, seq)
);
```

### Release relations

```sql
-- <artists> (kind='main') and <extraartists> (kind='extra') folded into one table.
CREATE TABLE release_artist (
  id         INTEGER PRIMARY KEY,  -- surrogate
  release_id INTEGER NOT NULL,     -- -> release.id
  seq        INTEGER NOT NULL,
  artist_id  INTEGER NOT NULL,     -- -> artist.id
  anv        TEXT,
  join_str   TEXT,
  role       TEXT,
  tracks     TEXT,
  kind       TEXT NOT NULL         -- 'main' | 'extra'
);

CREATE TABLE release_label (
  release_id INTEGER NOT NULL,
  seq        INTEGER NOT NULL,
  label_id   INTEGER,   -- -> label.id (may be absent on some rows)
  name       TEXT,      -- label name as printed on the release
  catno      TEXT,      -- catalog number
  PRIMARY KEY (release_id, seq)
);

CREATE TABLE release_format (
  id         INTEGER PRIMARY KEY,  -- surrogate
  release_id INTEGER NOT NULL,     -- -> release.id
  seq        INTEGER NOT NULL,
  name       TEXT,                 -- 'CD', 'Vinyl', ...
  qty        TEXT,                 -- dump value is a string
  text       TEXT                  -- free-text qualifier
);

CREATE TABLE release_format_description (
  format_id   INTEGER NOT NULL,    -- -> release_format.id
  seq         INTEGER NOT NULL,
  description TEXT NOT NULL,        -- 'Album', 'Compilation', 'LP', ...
  PRIMARY KEY (format_id, seq)
);

CREATE TABLE release_genre (
  release_id INTEGER NOT NULL,
  seq        INTEGER NOT NULL,
  genre      TEXT NOT NULL,
  PRIMARY KEY (release_id, seq)
);

CREATE TABLE release_style (
  release_id INTEGER NOT NULL,
  seq        INTEGER NOT NULL,
  style      TEXT NOT NULL,
  PRIMARY KEY (release_id, seq)
);

CREATE TABLE release_identifier (
  release_id  INTEGER NOT NULL,
  seq         INTEGER NOT NULL,
  type        TEXT,   -- 'Barcode', 'Matrix / Runout', 'Label Code', ...
  value       TEXT,
  description TEXT,
  PRIMARY KEY (release_id, seq)
);

CREATE TABLE release_company (
  release_id       INTEGER NOT NULL,
  seq              INTEGER NOT NULL,
  company_label_id INTEGER,  -- -> label.id (companies reference label/entity ids)
  name             TEXT,
  catno            TEXT,
  entity_type      TEXT,     -- numeric code, as string
  entity_type_name TEXT,     -- 'Manufactured By', 'Pressed By', ...
  PRIMARY KEY (release_id, seq)
);

CREATE TABLE release_video (
  release_id  INTEGER NOT NULL,
  seq         INTEGER NOT NULL,
  uri         TEXT,
  title       TEXT,
  description TEXT,
  duration    INTEGER,  -- seconds; NULL if blank
  PRIMARY KEY (release_id, seq)
);

-- Tracklist with recursive <sub_tracks> via parent_track_id self-reference.
CREATE TABLE track (
  id              INTEGER PRIMARY KEY,  -- surrogate
  release_id      INTEGER NOT NULL,     -- -> release.id
  parent_track_id INTEGER,              -- -> track.id (NULL at top level)
  seq             INTEGER NOT NULL,     -- order within parent/release
  position        TEXT,
  title           TEXT,
  duration        TEXT                  -- dump value is 'm:ss' free text
);

-- Per-track <artists> (kind='main') and <extraartists> (kind='extra').
CREATE TABLE track_artist (
  id        INTEGER PRIMARY KEY,  -- surrogate
  track_id  INTEGER NOT NULL,     -- -> track.id
  seq       INTEGER NOT NULL,
  artist_id INTEGER NOT NULL,     -- -> artist.id
  anv       TEXT,
  join_str  TEXT,
  role      TEXT,
  kind      TEXT NOT NULL         -- 'main' | 'extra'
);
```

### FTS5 search indexes (derived; required)

**Contentless** FTS5 tables (`content=''`), with `rowid` set to the Discogs id, so
`MATCH` returns the id directly. Contentless is chosen because (a) one column,
`artist_names`, is a *denormalized* concatenation of a release's / master's credited
artist names that does not exist on any base table, and (b) the database is rebuilt
in full each month, so the contentless limitation (no rebuild/column-readback) is
irrelevant. Populated by explicit `INSERT` after the bulk row load.

```sql
CREATE VIRTUAL TABLE release_fts USING fts5(
  title, artist_names,
  content='', tokenize='unicode61 remove_diacritics 2'
);
CREATE VIRTUAL TABLE master_fts USING fts5(
  title, artist_names,
  content='', tokenize='unicode61 remove_diacritics 2'
);
CREATE VIRTUAL TABLE artist_fts USING fts5(
  name,
  content='', tokenize='unicode61 remove_diacritics 2'
);
CREATE VIRTUAL TABLE label_fts USING fts5(
  name,
  content='', tokenize='unicode61 remove_diacritics 2'
);
```

Population, e.g. for releases (artist_names assembled from `release_artist`):

```sql
INSERT INTO release_fts(rowid, title, artist_names)
SELECT r.id, r.title,
       (SELECT group_concat(ra.anv_or_name, ' ')
          FROM (SELECT COALESCE(NULLIF(ra.anv,''), a.name) AS anv_or_name
                  FROM release_artist ra JOIN artist a ON a.id = ra.artist_id
                 WHERE ra.release_id = r.id AND ra.kind = 'main'
                 ORDER BY ra.seq) ra)
  FROM release r;
```

The exact `artist_names` assembly query is an implementation detail; the requirement
is: `MATCH` on name/title returns the Discogs id, with diacritic-insensitive matching.

### Secondary indexes

Created after bulk load, before serving. At minimum, the FK columns used for joins:
`release_artist(release_id)`, `release_artist(artist_id)`, `track(release_id)`,
`track(parent_track_id)`, `track_artist(track_id)`, `release_label(release_id)`,
`release_format(release_id)`, `release_format_description(format_id)`,
`release_identifier(release_id)`, `release_company(release_id)`,
`master_artist(master_id)`, `release.master_id`, `master.main_release_id`.

## Import pipeline

1. **Fetch & verify** the four `.gz` and `CHECKSUM.txt` for the chosen date; abort on
   SHA-256 mismatch.
2. **Create schema** in a fresh `discogs-YYYYMMDD.db`. Set import-time PRAGMAs:
   `journal_mode=OFF` (or `MEMORY`), `synchronous=OFF`, large `cache_size`,
   `foreign_keys=OFF`, `temp_store=MEMORY`.
3. **Stream-parse each dump** with `gzip.NewReader` → `encoding/xml` token decoder,
   one top-level element at a time; never build a full DOM. Parser structs may be
   cribbed from `lukasaron/data-discogs`; the schema cross-checked against
   `discogs-xml2db`'s SQLite schema.
4. **Batch inserts** in transactions (e.g. a few thousand top-level records per tx).
   Malformed / unexpected records are logged-and-counted, not fatal (`data_quality`
   varies across Discogs records).
5. **Build FTS5 indexes** and **secondary indexes** after the bulk row load.
6. **Switch to serving PRAGMAs:** `journal_mode=WAL`, `synchronous=NORMAL`.
7. **Write `meta`** rows (dump date, checksums, counts, importer version, timestamp).
8. **Integrity report** (see below). Print summary (per-table counts, orphan counts).

Load order to minimize forward references: `artists`, `labels`, `masters`,
`releases`. The `master`↔`release` cycle means at least one direction will reference
not-yet-loaded rows regardless; this is handled by deferring FK enforcement.

## Referential integrity

`master.main_release_id` ↔ `release.master_id` is circular, and real Discogs data
contains dangling references (credits to deleted artists, etc.). Therefore:

- Import with `foreign_keys=OFF`.
- After load, run a **referential-integrity report**: for each documented FK
  relationship, count rows whose referent is absent.
- Decide per relationship whether to add an enforced `FOREIGN KEY` constraint to the
  served schema (only for relationships that come back clean) or leave it advisory
  (documented but unenforced). Default posture: advisory unless proven clean, since a
  fresh rebuild each month means constraints would otherwise have to hold every time.

## MusicBrainz-readiness (future)

The future MusicBrainz SQLite DB will be `ATTACH`ed alongside this one and joined on
Discogs IDs (MB carries Discogs keys extracted from its URL relationships). The only
obligations on this project: Discogs integer IDs as primary keys (met), SQLite as the
engine (met), and snake_case naming conventions consistent enough to keep cross-DB
queries readable. No MB work happens here.

## Consumer query patterns (documented to validate the schema)

These tools are refactored downstream, not in this project; listed to confirm the
schema and FTS indexes are sufficient.

- **classical-tagger** (Go, native bindings):
  - Search: `release_fts MATCH 'artist_names:<artist> title:<album>'`, optionally
    filtered to CD via join on `release_format.name='CD'`, ranked by `bm25`, returning
    candidate ids/titles for the user to pick.
  - By-id: assemble a full release from `release` + `release_artist` +
    `release_label` + `release_format`(+descriptions) + `track`(+`track_artist`,
    recursive sub_tracks) + `release_identifier`.
- **tidalist** (Python, `sqlite3`):
  - Search `release_fts` and `master_fts` by combined `<artist> <title>`, take top-N
    (~25); read year, format descriptions ("Live"), and credited artist names.

The schema serves both: FTS for recall into a candidate set; normalized tables for
detail assembly; format/description tables for filtering.

## Project layout & tech

- Module `github.com/cehbz/discogs`; cobra CLI under `cmd/discogs` with subcommands
  `fetch`, `import`, `verify`.
- `internal/dumps` — download + SHA-256 verification.
- `internal/parse` — per-type streaming XML parsers.
- `internal/store` — schema DDL, insert layer, FTS/index build, integrity report.
- SQLite driver: `modernc.org/sqlite` (cgo-free, FTS5 built in). Note `mattn/go-sqlite3`
  needs the `fts5` build tag; Python `sqlite3` needs an FTS5-enabled build (true for
  Homebrew / python.org builds — verify the one `tidalist` uses).

## Testing strategy

- **Parser unit tests** per dump type on small XML fixtures covering edge cases:
  nested `sub_tracks`, missing/blank `year` and `released`, multi-artist credits,
  `extraartists`, multiple identifiers, sublabels/parentLabel, aliases, members/groups.
- **Round-trip test:** one known `<release>` populates every expected row across all
  child tables with correct `seq` ordering.
- **FTS test:** diacritic-insensitive `MATCH` on name/title returns the right ids.
- **Integrity-report test:** a fixture set with deliberate orphans produces correct
  orphan counts.
- **Determinism/isolation:** tests build throwaway temp DBs; no network in unit tests
  (fetcher tested against a local fixture server / recorded responses).

## First real import — measured results (2026-06-01 dump, run 2026-06-24)

- **DB:** `/Volumes/Crucial X10/discogs/discogs-20260601.db` — **46 GB** (X10: 6.1 TB free). Stable symlink `discogs.db → discogs-20260601.db`.
- **Wall-clock:** ~2h45m total — fetch ~6 min; import ~2h40m (incl. ~17 min for the final integrity report).
- **Row counts:** artists 10,081,427 · labels 2,383,990 · masters 2,560,991 · releases 19,192,301 · tracks 178,224,810 · release_artist 93,727,607.
- **Per-FK enforcement decision:** no relationship returned 0 orphans, so **all FKs stay advisory (none enforced).** Orphans — release_artist→artist 2,317,526; track_artist→artist 1,643,197; master_artist→artist 173,661; release.master_id→master 159; master.main_release_id→release 128; release_label.label_id→label 109. (Credits referencing artists absent from the artists dump are normal Discogs data.)
- **FTS5 functional check:** column-scoped MATCH (`title:"kind of blue" AND artist_names:miles`) returns the correct Miles Davis editions in ~0.1s.
- **Perf follow-up:** the ~2h40m import (≈300M+ inserts via per-row `tx.Exec`) justified implementing the deferred prepared-statement optimization.

## Still open

- FTS5 availability in the Python `sqlite3` build `tidalist` uses (tidalist's environment; untested here).
