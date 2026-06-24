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
