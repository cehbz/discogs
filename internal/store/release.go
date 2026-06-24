package store

import (
	"database/sql"
	"fmt"

	"github.com/cehbz/discogs/internal/parse"
)

// ReleaseInserter holds prepared statements for inserting a release and all its child rows.
type ReleaseInserter struct {
	release            *sql.Stmt
	releaseArtist      *sql.Stmt
	releaseLabel       *sql.Stmt
	releaseFormat      *sql.Stmt
	releaseFormatDesc  *sql.Stmt
	releaseGenre       *sql.Stmt
	releaseStyle       *sql.Stmt
	releaseIdentifier  *sql.Stmt
	releaseCompany     *sql.Stmt
	releaseVideo       *sql.Stmt
	track              *sql.Stmt
	trackArtist        *sql.Stmt
}

// NewReleaseInserter prepares all statements for release insertion on the given tx.
func NewReleaseInserter(tx *sql.Tx) (*ReleaseInserter, error) {
	ins := &ReleaseInserter{}
	var err error

	ins.release, err = tx.Prepare(
		`INSERT INTO release(id,status,title,country,released_raw,notes,data_quality,master_id,is_main_release)
		 VALUES(?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return nil, fmt.Errorf("prepare release: %w", err)
	}
	ins.releaseArtist, err = tx.Prepare(
		`INSERT INTO release_artist(release_id,seq,artist_id,anv,join_str,role,tracks,kind)
		 VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare release_artist: %w", err)
	}
	ins.releaseLabel, err = tx.Prepare(
		`INSERT INTO release_label(release_id,seq,label_id,name,catno) VALUES(?,?,?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare release_label: %w", err)
	}
	ins.releaseFormat, err = tx.Prepare(
		`INSERT INTO release_format(release_id,seq,name,qty,text) VALUES(?,?,?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare release_format: %w", err)
	}
	ins.releaseFormatDesc, err = tx.Prepare(
		`INSERT INTO release_format_description(format_id,seq,description) VALUES(?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare release_format_description: %w", err)
	}
	ins.releaseGenre, err = tx.Prepare(
		`INSERT INTO release_genre(release_id,seq,genre) VALUES(?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare release_genre: %w", err)
	}
	ins.releaseStyle, err = tx.Prepare(
		`INSERT INTO release_style(release_id,seq,style) VALUES(?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare release_style: %w", err)
	}
	ins.releaseIdentifier, err = tx.Prepare(
		`INSERT INTO release_identifier(release_id,seq,type,value,description) VALUES(?,?,?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare release_identifier: %w", err)
	}
	ins.releaseCompany, err = tx.Prepare(
		`INSERT INTO release_company(release_id,seq,company_label_id,name,catno,entity_type,entity_type_name)
		 VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare release_company: %w", err)
	}
	ins.releaseVideo, err = tx.Prepare(
		`INSERT INTO release_video(release_id,seq,uri,title,description,duration) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare release_video: %w", err)
	}
	ins.track, err = tx.Prepare(
		`INSERT INTO track(release_id,parent_track_id,seq,position,title,duration) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare track: %w", err)
	}
	ins.trackArtist, err = tx.Prepare(
		`INSERT INTO track_artist(track_id,seq,artist_id,anv,join_str,role,kind) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		ins.Close()
		return nil, fmt.Errorf("prepare track_artist: %w", err)
	}
	return ins, nil
}

// Insert inserts one release and all its child rows using prepared statements.
func (ins *ReleaseInserter) Insert(r *parse.Release) error {
	var masterID any
	isMain := 0
	if r.Master != nil && r.Master.ID != 0 {
		masterID = r.Master.ID
		if r.Master.IsMainRelease {
			isMain = 1
		}
	}
	if _, err := ins.release.Exec(
		r.ID, r.Status, r.Title, r.Country, r.Released, r.Notes, r.DataQuality, masterID, isMain); err != nil {
		return fmt.Errorf("insert release %d: %w", r.ID, err)
	}
	if err := ins.insertReleaseCredits(r.ID, "main", r.Artists); err != nil {
		return err
	}
	if err := ins.insertReleaseCredits(r.ID, "extra", r.ExtraArtists); err != nil {
		return err
	}
	for i, l := range r.Labels {
		if _, err := ins.releaseLabel.Exec(r.ID, i, nullIfZero(l.ID), l.Name, l.Catno); err != nil {
			return fmt.Errorf("insert release_label %d/%d: %w", r.ID, i, err)
		}
	}
	for i, f := range r.Formats {
		res, err := ins.releaseFormat.Exec(r.ID, i, f.Name, f.Qty, f.Text)
		if err != nil {
			return fmt.Errorf("insert release_format %d/%d: %w", r.ID, i, err)
		}
		fid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for j, d := range f.Descriptions {
			if _, err := ins.releaseFormatDesc.Exec(fid, j, d); err != nil {
				return fmt.Errorf("insert format_desc %d format %d desc %d: %w", r.ID, i, j, err)
			}
		}
	}
	for i, g := range r.Genres {
		if _, err := ins.releaseGenre.Exec(r.ID, i, g); err != nil {
			return fmt.Errorf("insert release_genre %d/%d: %w", r.ID, i, err)
		}
	}
	for i, s := range r.Styles {
		if _, err := ins.releaseStyle.Exec(r.ID, i, s); err != nil {
			return fmt.Errorf("insert release_style %d/%d: %w", r.ID, i, err)
		}
	}
	for i, id := range r.Identifiers {
		if _, err := ins.releaseIdentifier.Exec(r.ID, i, id.Type, id.Value, id.Description); err != nil {
			return fmt.Errorf("insert release_identifier %d/%d: %w", r.ID, i, err)
		}
	}
	for i, c := range r.Companies {
		if _, err := ins.releaseCompany.Exec(
			r.ID, i, nullIfZero(c.ID), c.Name, c.Catno, c.EntityType, c.EntityTypeName); err != nil {
			return fmt.Errorf("insert release_company %d/%d: %w", r.ID, i, err)
		}
	}
	for i, v := range r.Videos {
		if _, err := ins.releaseVideo.Exec(r.ID, i, v.URI, v.Title, v.Description, nullIfZero(v.Duration)); err != nil {
			return fmt.Errorf("insert release_video %d/%d: %w", r.ID, i, err)
		}
	}
	if err := ins.insertTracks(r.ID, nil, r.Tracklist); err != nil {
		return err
	}
	return nil
}

func (ins *ReleaseInserter) insertTracks(releaseID int, parentID any, tracks []parse.Track) error {
	for i, t := range tracks {
		res, err := ins.track.Exec(releaseID, parentID, i, t.Position, t.Title, t.Duration)
		if err != nil {
			return fmt.Errorf("insert track (release %d): %w", releaseID, err)
		}
		tid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if err := ins.insertTrackCredits(tid, "main", t.Artists); err != nil {
			return err
		}
		if err := ins.insertTrackCredits(tid, "extra", t.ExtraArtists); err != nil {
			return err
		}
		if len(t.SubTracks) > 0 {
			if err := ins.insertTracks(releaseID, tid, t.SubTracks); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ins *ReleaseInserter) insertTrackCredits(trackID int64, kind string, credits []parse.ArtistCredit) error {
	for i, c := range credits {
		if _, err := ins.trackArtist.Exec(trackID, i, c.ID, c.ANV, c.Join, c.Role, kind); err != nil {
			return fmt.Errorf("insert track_artist (track %d): %w", trackID, err)
		}
	}
	return nil
}

func (ins *ReleaseInserter) insertReleaseCredits(releaseID int, kind string, credits []parse.ArtistCredit) error {
	for i, c := range credits {
		if _, err := ins.releaseArtist.Exec(releaseID, i, c.ID, c.ANV, c.Join, c.Role, c.Tracks, kind); err != nil {
			return fmt.Errorf("insert release_artist %d/%d: %w", releaseID, i, err)
		}
	}
	return nil
}

// Close closes all prepared statements, returning the first error encountered.
func (ins *ReleaseInserter) Close() error {
	var first error
	for _, s := range []*sql.Stmt{
		ins.release, ins.releaseArtist, ins.releaseLabel,
		ins.releaseFormat, ins.releaseFormatDesc, ins.releaseGenre,
		ins.releaseStyle, ins.releaseIdentifier, ins.releaseCompany,
		ins.releaseVideo, ins.track, ins.trackArtist,
	} {
		if s == nil {
			continue
		}
		if err := s.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// InsertRelease is a thin wrapper that creates a ReleaseInserter, inserts one record, and closes it.
func InsertRelease(tx *sql.Tx, r *parse.Release) error {
	ins, err := NewReleaseInserter(tx)
	if err != nil {
		return err
	}
	defer ins.Close()
	return ins.Insert(r)
}
