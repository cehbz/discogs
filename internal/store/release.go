package store

import (
	"database/sql"
	"fmt"

	"github.com/cehbz/discogs/internal/parse"
)

func InsertRelease(tx *sql.Tx, r *parse.Release) error {
	var masterID any
	isMain := 0
	if r.Master != nil && r.Master.ID != 0 {
		masterID = r.Master.ID
		if r.Master.IsMainRelease {
			isMain = 1
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO release(id,status,title,country,released_raw,notes,data_quality,master_id,is_main_release)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Status, r.Title, r.Country, r.Released, r.Notes, r.DataQuality, masterID, isMain); err != nil {
		return fmt.Errorf("insert release %d: %w", r.ID, err)
	}
	if err := insertReleaseCredits(tx, r.ID, "main", r.Artists); err != nil {
		return err
	}
	if err := insertReleaseCredits(tx, r.ID, "extra", r.ExtraArtists); err != nil {
		return err
	}
	for i, l := range r.Labels {
		if _, err := tx.Exec(
			`INSERT INTO release_label(release_id,seq,label_id,name,catno) VALUES(?,?,?,?,?)`,
			r.ID, i, nullIfZero(l.ID), l.Name, l.Catno); err != nil {
			return fmt.Errorf("insert release_label %d/%d: %w", r.ID, i, err)
		}
	}
	for i, f := range r.Formats {
		res, err := tx.Exec(
			`INSERT INTO release_format(release_id,seq,name,qty,text) VALUES(?,?,?,?,?)`,
			r.ID, i, f.Name, f.Qty, f.Text)
		if err != nil {
			return fmt.Errorf("insert release_format %d/%d: %w", r.ID, i, err)
		}
		fid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for j, d := range f.Descriptions {
			if _, err := tx.Exec(
				`INSERT INTO release_format_description(format_id,seq,description) VALUES(?,?,?)`,
				fid, j, d); err != nil {
				return fmt.Errorf("insert format_desc %d/%d: %w", r.ID, j, err)
			}
		}
	}
	for i, g := range r.Genres {
		if _, err := tx.Exec(`INSERT INTO release_genre(release_id,seq,genre) VALUES(?,?,?)`, r.ID, i, g); err != nil {
			return fmt.Errorf("insert release_genre %d/%d: %w", r.ID, i, err)
		}
	}
	for i, s := range r.Styles {
		if _, err := tx.Exec(`INSERT INTO release_style(release_id,seq,style) VALUES(?,?,?)`, r.ID, i, s); err != nil {
			return fmt.Errorf("insert release_style %d/%d: %w", r.ID, i, err)
		}
	}
	for i, id := range r.Identifiers {
		if _, err := tx.Exec(
			`INSERT INTO release_identifier(release_id,seq,type,value,description) VALUES(?,?,?,?,?)`,
			r.ID, i, id.Type, id.Value, id.Description); err != nil {
			return fmt.Errorf("insert release_identifier %d/%d: %w", r.ID, i, err)
		}
	}
	for i, c := range r.Companies {
		if _, err := tx.Exec(
			`INSERT INTO release_company(release_id,seq,company_label_id,name,catno,entity_type,entity_type_name)
			 VALUES(?,?,?,?,?,?,?)`,
			r.ID, i, nullIfZero(c.ID), c.Name, c.Catno, c.EntityType, c.EntityTypeName); err != nil {
			return fmt.Errorf("insert release_company %d/%d: %w", r.ID, i, err)
		}
	}
	for i, v := range r.Videos {
		if _, err := tx.Exec(
			`INSERT INTO release_video(release_id,seq,uri,title,description,duration) VALUES(?,?,?,?,?,?)`,
			r.ID, i, v.URI, v.Title, v.Description, nullIfZero(v.Duration)); err != nil {
			return fmt.Errorf("insert release_video %d/%d: %w", r.ID, i, err)
		}
	}
	return nil
}

func insertReleaseCredits(tx *sql.Tx, releaseID int, kind string, credits []parse.ArtistCredit) error {
	for i, c := range credits {
		if _, err := tx.Exec(
			`INSERT INTO release_artist(release_id,seq,artist_id,anv,join_str,role,tracks,kind)
			 VALUES(?,?,?,?,?,?,?,?)`,
			releaseID, i, c.ID, c.ANV, c.Join, c.Role, c.Tracks, kind); err != nil {
			return fmt.Errorf("insert release_artist %d/%d: %w", releaseID, i, err)
		}
	}
	return nil
}
