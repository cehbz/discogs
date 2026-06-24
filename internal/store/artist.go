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
