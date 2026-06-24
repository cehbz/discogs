package importer

import (
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/cehbz/discogs/internal/dumps"
	"github.com/cehbz/discogs/internal/parse"
	"github.com/cehbz/discogs/internal/store"
	_ "modernc.org/sqlite"
)

const batchSize = 50000

type Report struct {
	Counts    map[string]int64
	Integrity *store.IntegrityReport
}

// Import builds a fresh SQLite DB at dbPath from the four dumps in dir for date.
func Import(dbPath, dir, date string) (*Report, error) {
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove existing db: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1) // single writer; keeps PRAGMAs and tx on one connection

	for _, p := range []string{
		`PRAGMA journal_mode=OFF`, `PRAGMA synchronous=OFF`,
		`PRAGMA temp_store=MEMORY`, `PRAGMA cache_size=-200000`, `PRAGMA foreign_keys=OFF`,
	} {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}
	if err := store.CreateSchema(db); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	counts := map[string]int64{}
	path := func(typ string) string { return filepath.Join(dir, dumps.FileName(date, typ)) }

	if counts["artists"], err = importStream(db, path("artists"), parse.ParseArtists, store.InsertArtist); err != nil {
		return nil, fmt.Errorf("import artists: %w", err)
	}
	if counts["labels"], err = importStream(db, path("labels"), parse.ParseLabels, store.InsertLabel); err != nil {
		return nil, fmt.Errorf("import labels: %w", err)
	}
	if counts["masters"], err = importStream(db, path("masters"), parse.ParseMasters, store.InsertMaster); err != nil {
		return nil, fmt.Errorf("import masters: %w", err)
	}
	if counts["releases"], err = importStream(db, path("releases"), parse.ParseReleases, store.InsertRelease); err != nil {
		return nil, fmt.Errorf("import releases: %w", err)
	}

	if err := store.BuildFTS(db); err != nil {
		return nil, fmt.Errorf("build fts: %w", err)
	}
	if err := store.BuildIndexes(db); err != nil {
		return nil, fmt.Errorf("build indexes: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	meta := map[string]string{"dump_date": date, "importer_version": "1"}
	for typ, n := range counts {
		meta["count_"+typ] = strconv.FormatInt(n, 10)
	}
	if err := store.SetMeta(tx, meta); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	for _, p := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA synchronous=NORMAL`} {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}

	rep, err := store.RunIntegrityReport(db)
	if err != nil {
		return nil, fmt.Errorf("integrity report: %w", err)
	}
	return &Report{Counts: counts, Integrity: rep}, nil
}

// importStream streams one gzip dump file through parseFn, inserting each record via
// insert, committing every batchSize records. T is the parsed record type.
func importStream[T any](
	db *sql.DB,
	path string,
	parseFn func(io.Reader, func(*T) error) error,
	insert func(*sql.Tx, *T) error,
) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	defer gz.Close()

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	var n int64
	perr := parseFn(gz, func(rec *T) error {
		if err := insert(tx, rec); err != nil {
			return err
		}
		n++
		if n%batchSize == 0 {
			if err := tx.Commit(); err != nil {
				return err
			}
			tx, err = db.Begin()
			return err
		}
		return nil
	})
	if perr != nil {
		_ = tx.Rollback()
		return n, perr
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}
