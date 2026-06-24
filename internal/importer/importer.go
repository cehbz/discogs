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
func Import(dbPath, dir, date string) (_ *Report, err error) {
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove existing db: %w", err)
	}
	db, openErr := sql.Open("sqlite", dbPath)
	if openErr != nil {
		return nil, openErr
	}
	defer func() {
		db.Close()
		if err != nil {
			os.Remove(dbPath)
		}
	}()
	db.SetMaxOpenConns(1) // single writer; keeps PRAGMAs and tx on one connection

	for _, p := range []string{
		`PRAGMA journal_mode=OFF`, `PRAGMA synchronous=OFF`,
		`PRAGMA temp_store=MEMORY`, `PRAGMA cache_size=-200000`, `PRAGMA foreign_keys=OFF`,
	} {
		if _, err = db.Exec(p); err != nil {
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}
	if err = store.CreateSchema(db); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	counts := map[string]int64{}
	path := func(typ string) string { return filepath.Join(dir, dumps.FileName(date, typ)) }

	var streamErr error
	if counts["artists"], streamErr = importStream(db, path("artists"), parse.ParseArtists,
		func(tx *sql.Tx) (store.Inserter[parse.Artist], error) { return store.NewArtistInserter(tx) }); streamErr != nil {
		return nil, fmt.Errorf("import artists: %w", streamErr)
	}
	if counts["labels"], streamErr = importStream(db, path("labels"), parse.ParseLabels,
		func(tx *sql.Tx) (store.Inserter[parse.Label], error) { return store.NewLabelInserter(tx) }); streamErr != nil {
		return nil, fmt.Errorf("import labels: %w", streamErr)
	}
	if counts["masters"], streamErr = importStream(db, path("masters"), parse.ParseMasters,
		func(tx *sql.Tx) (store.Inserter[parse.Master], error) { return store.NewMasterInserter(tx) }); streamErr != nil {
		return nil, fmt.Errorf("import masters: %w", streamErr)
	}
	if counts["releases"], streamErr = importStream(db, path("releases"), parse.ParseReleases,
		func(tx *sql.Tx) (store.Inserter[parse.Release], error) { return store.NewReleaseInserter(tx) }); streamErr != nil {
		return nil, fmt.Errorf("import releases: %w", streamErr)
	}

	if err = store.BuildIndexes(db); err != nil {
		return nil, fmt.Errorf("build indexes: %w", err)
	}
	if err = store.BuildFTS(db); err != nil {
		return nil, fmt.Errorf("build fts: %w", err)
	}

	tx, txErr := db.Begin()
	if txErr != nil {
		err = txErr
		return nil, err
	}
	meta := map[string]string{"dump_date": date, "importer_version": "1"}
	for typ, n := range counts {
		meta["count_"+typ] = strconv.FormatInt(n, 10)
	}
	if metaErr := store.SetMeta(tx, meta); metaErr != nil {
		tx.Rollback()
		err = metaErr
		return nil, err
	}
	if commitErr := tx.Commit(); commitErr != nil {
		err = commitErr
		return nil, err
	}

	for _, p := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA synchronous=NORMAL`} {
		if _, err = db.Exec(p); err != nil {
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}

	rep, repErr := store.RunIntegrityReport(db)
	if repErr != nil {
		err = repErr
		return nil, fmt.Errorf("integrity report: %w", err)
	}
	return &Report{Counts: counts, Integrity: rep}, nil
}

// importStream streams one gzip dump file through parseFn, inserting each record via
// an Inserter created by newIns, committing every batchSize records. T is the parsed record type.
func importStream[T any](
	db *sql.DB,
	path string,
	parseFn func(io.Reader, func(*T) error) error,
	newIns func(*sql.Tx) (store.Inserter[T], error),
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
	ins, err := newIns(tx)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	var n int64
	perr := parseFn(gz, func(rec *T) error {
		if err := ins.Insert(rec); err != nil {
			return err
		}
		n++
		if n%batchSize == 0 {
			if err := ins.Close(); err != nil {
				ins = nil
				return err
			}
			ins = nil
			if err := tx.Commit(); err != nil {
				tx = nil
				return err
			}
			tx, err = db.Begin()
			if err != nil {
				tx = nil
				ins = nil
				return err
			}
			ins, err = newIns(tx)
			if err != nil {
				ins = nil
				return err
			}
		}
		return nil
	})
	if perr != nil {
		if ins != nil {
			_ = ins.Close()
		}
		if tx != nil {
			_ = tx.Rollback()
		}
		return n, perr
	}
	if ins != nil {
		if err := ins.Close(); err != nil {
			_ = tx.Rollback()
			return n, err
		}
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}
