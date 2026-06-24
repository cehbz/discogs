package store

import (
	"database/sql"
	"fmt"

	"github.com/cehbz/discogs/internal/parse"
)

// ArtistInserter holds prepared statements for inserting an artist and its child rows.
type ArtistInserter struct {
	artist     *sql.Stmt
	namevar    *sql.Stmt
	url        *sql.Stmt
	alias      *sql.Stmt
	membership *sql.Stmt
}

// NewArtistInserter prepares all statements for artist insertion on the given tx.
func NewArtistInserter(tx *sql.Tx) (*ArtistInserter, error) {
	ins := &ArtistInserter{}
	var err error

	ins.artist, err = tx.Prepare(`INSERT INTO artist(id,name,realname,profile,data_quality) VALUES(?,?,?,?,?)`)
	if err != nil {
		return nil, fmt.Errorf("prepare artist: %w", err)
	}
	ins.namevar, err = tx.Prepare(`INSERT INTO artist_namevariation(artist_id,seq,name) VALUES(?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare artist_namevariation: %w", err)
	}
	ins.url, err = tx.Prepare(`INSERT INTO artist_url(artist_id,seq,url) VALUES(?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare artist_url: %w", err)
	}
	ins.alias, err = tx.Prepare(`INSERT OR IGNORE INTO artist_alias(artist_id,alias_artist_id,name) VALUES(?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare artist_alias: %w", err)
	}
	ins.membership, err = tx.Prepare(`INSERT OR IGNORE INTO artist_membership(group_id,member_id,name) VALUES(?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare artist_membership: %w", err)
	}
	return ins, nil
}

// Insert inserts one artist and all its child rows using prepared statements.
func (ins *ArtistInserter) Insert(a *parse.Artist) error {
	if _, err := ins.artist.Exec(a.ID, a.Name, a.RealName, a.Profile, a.DataQuality); err != nil {
		return fmt.Errorf("insert artist %d: %w", a.ID, err)
	}
	for i, nv := range a.NameVariations {
		if _, err := ins.namevar.Exec(a.ID, i, nv); err != nil {
			return fmt.Errorf("insert namevar %d/%d: %w", a.ID, i, err)
		}
	}
	for i, u := range a.URLs {
		if _, err := ins.url.Exec(a.ID, i, u); err != nil {
			return fmt.Errorf("insert artist_url %d/%d: %w", a.ID, i, err)
		}
	}
	for _, al := range a.Aliases {
		if al.ID == 0 {
			continue
		}
		if _, err := ins.alias.Exec(a.ID, al.ID, al.Name); err != nil {
			return fmt.Errorf("insert alias %d: %w", a.ID, err)
		}
	}
	for _, m := range a.Members {
		if m.ID == 0 {
			continue
		}
		if _, err := ins.membership.Exec(a.ID, m.ID, m.Name); err != nil {
			return fmt.Errorf("insert member %d: %w", a.ID, err)
		}
	}
	for _, g := range a.Groups {
		if g.ID == 0 {
			continue
		}
		// inverse view: this artist is a member of group g
		if _, err := ins.membership.Exec(g.ID, a.ID, a.Name); err != nil {
			return fmt.Errorf("insert group %d: %w", a.ID, err)
		}
	}
	return nil
}

// Close closes all prepared statements, returning the first error encountered.
func (ins *ArtistInserter) Close() error {
	var first error
	for _, s := range []*sql.Stmt{ins.artist, ins.namevar, ins.url, ins.alias, ins.membership} {
		if s == nil {
			continue
		}
		if err := s.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// InsertArtist is a thin wrapper that creates an ArtistInserter, inserts one record, and closes it.
func InsertArtist(tx *sql.Tx, a *parse.Artist) error {
	ins, err := NewArtistInserter(tx)
	if err != nil {
		return err
	}
	defer ins.Close()
	return ins.Insert(a)
}
