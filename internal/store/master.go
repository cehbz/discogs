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

// MasterInserter holds prepared statements for inserting a master and its child rows.
type MasterInserter struct {
	master *sql.Stmt
	artist *sql.Stmt
	genre  *sql.Stmt
	style  *sql.Stmt
	video  *sql.Stmt
}

// NewMasterInserter prepares all statements for master insertion on the given tx.
func NewMasterInserter(tx *sql.Tx) (*MasterInserter, error) {
	ins := &MasterInserter{}
	var err error

	ins.master, err = tx.Prepare(`INSERT INTO master(id,main_release_id,title,year,data_quality) VALUES(?,?,?,?,?)`)
	if err != nil {
		return nil, fmt.Errorf("prepare master: %w", err)
	}
	ins.artist, err = tx.Prepare(`INSERT INTO master_artist(master_id,seq,artist_id,anv,join_str,role) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare master_artist: %w", err)
	}
	ins.genre, err = tx.Prepare(`INSERT INTO master_genre(master_id,seq,genre) VALUES(?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare master_genre: %w", err)
	}
	ins.style, err = tx.Prepare(`INSERT INTO master_style(master_id,seq,style) VALUES(?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare master_style: %w", err)
	}
	ins.video, err = tx.Prepare(`INSERT INTO master_video(master_id,seq,uri,title,description,duration) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare master_video: %w", err)
	}
	return ins, nil
}

// Insert inserts one master and all its child rows using prepared statements.
func (ins *MasterInserter) Insert(m *parse.Master) error {
	if _, err := ins.master.Exec(m.ID, nullIfZero(m.MainRelease), m.Title, nullIfZero(m.Year), m.DataQuality); err != nil {
		return fmt.Errorf("insert master %d: %w", m.ID, err)
	}
	for i, c := range m.Artists {
		if _, err := ins.artist.Exec(m.ID, i, c.ID, c.ANV, c.Join, c.Role); err != nil {
			return fmt.Errorf("insert master_artist %d/%d: %w", m.ID, i, err)
		}
	}
	for i, g := range m.Genres {
		if _, err := ins.genre.Exec(m.ID, i, g); err != nil {
			return fmt.Errorf("insert master_genre %d/%d: %w", m.ID, i, err)
		}
	}
	for i, s := range m.Styles {
		if _, err := ins.style.Exec(m.ID, i, s); err != nil {
			return fmt.Errorf("insert master_style %d/%d: %w", m.ID, i, err)
		}
	}
	for i, v := range m.Videos {
		if _, err := ins.video.Exec(m.ID, i, v.URI, v.Title, v.Description, nullIfZero(v.Duration)); err != nil {
			return fmt.Errorf("insert master_video %d/%d: %w", m.ID, i, err)
		}
	}
	return nil
}

// Close closes all prepared statements, returning the first error encountered.
func (ins *MasterInserter) Close() error {
	var first error
	for _, s := range []*sql.Stmt{ins.master, ins.artist, ins.genre, ins.style, ins.video} {
		if s == nil {
			continue
		}
		if err := s.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// InsertMaster is a thin wrapper that creates a MasterInserter, inserts one record, and closes it.
func InsertMaster(tx *sql.Tx, m *parse.Master) error {
	ins, err := NewMasterInserter(tx)
	if err != nil {
		return err
	}
	defer ins.Close()
	return ins.Insert(m)
}
