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
