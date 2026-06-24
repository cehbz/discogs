package store

import (
	"database/sql"
	"fmt"

	"github.com/cehbz/discogs/internal/parse"
)

// LabelInserter holds prepared statements for inserting a label and its child rows.
type LabelInserter struct {
	label        *sql.Stmt
	url          *sql.Stmt
	relationship *sql.Stmt
}

// NewLabelInserter prepares all statements for label insertion on the given tx.
func NewLabelInserter(tx *sql.Tx) (*LabelInserter, error) {
	ins := &LabelInserter{}
	var err error

	ins.label, err = tx.Prepare(`INSERT INTO label(id,name,contactinfo,profile,data_quality) VALUES(?,?,?,?,?)`)
	if err != nil {
		return nil, fmt.Errorf("prepare label: %w", err)
	}
	ins.url, err = tx.Prepare(`INSERT INTO label_url(label_id,seq,url) VALUES(?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare label_url: %w", err)
	}
	ins.relationship, err = tx.Prepare(`INSERT OR IGNORE INTO label_relationship(parent_label_id,sublabel_id) VALUES(?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare label_relationship: %w", err)
	}
	return ins, nil
}

// Insert inserts one label and all its child rows using prepared statements.
func (ins *LabelInserter) Insert(l *parse.Label) error {
	if _, err := ins.label.Exec(l.ID, l.Name, l.ContactInfo, l.Profile, l.DataQuality); err != nil {
		return fmt.Errorf("insert label %d: %w", l.ID, err)
	}
	for i, u := range l.URLs {
		if _, err := ins.url.Exec(l.ID, i, u); err != nil {
			return fmt.Errorf("insert label_url %d/%d: %w", l.ID, i, err)
		}
	}
	if l.ParentLabel != nil && l.ParentLabel.ID != 0 {
		if _, err := ins.relationship.Exec(l.ParentLabel.ID, l.ID); err != nil {
			return fmt.Errorf("insert parent rel %d: %w", l.ID, err)
		}
	}
	for _, sub := range l.SubLabels {
		if sub.ID == 0 {
			continue
		}
		if _, err := ins.relationship.Exec(l.ID, sub.ID); err != nil {
			return fmt.Errorf("insert sublabel rel %d: %w", l.ID, err)
		}
	}
	return nil
}

// Close closes all prepared statements, returning the first error encountered.
func (ins *LabelInserter) Close() error {
	var first error
	for _, s := range []*sql.Stmt{ins.label, ins.url, ins.relationship} {
		if s == nil {
			continue
		}
		if err := s.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// InsertLabel is a thin wrapper that creates a LabelInserter, inserts one record, and closes it.
func InsertLabel(tx *sql.Tx, l *parse.Label) error {
	ins, err := NewLabelInserter(tx)
	if err != nil {
		return err
	}
	defer ins.Close()
	return ins.Insert(l)
}
