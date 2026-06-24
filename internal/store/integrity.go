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
