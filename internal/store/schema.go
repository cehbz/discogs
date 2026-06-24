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
