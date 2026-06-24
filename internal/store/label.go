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
