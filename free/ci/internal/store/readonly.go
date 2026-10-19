package store

// Purpose: open an existing state.db for reads only (D-1094). The release
// signer reads a run's facts and must never create, upgrade or write the file.
// Inputs: a state.db path ("" = the default) and Options (ReaderVersion).
// Outputs: a Store with a read pool and no writer connection.
// Constraints: no file or directory is created, no migration runs, no pragma
// that writes is issued. A database that is missing, empty, older than this
// binary's migrations (E607) or needs a newer reader (E606) is refused.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// OpenReadOnly opens an existing database with SQLite mode=ro and verifies the
// schema without changing it.
func OpenReadOnly(path string, opts Options) (*Store, error) {
	var err error
	if path == "" {
		if path, err = defaultPath(); err != nil {
			return nil, err
		}
	}
	if path, err = filepath.Abs(path); err != nil {
		return nil, err
	}
	if _, err = os.Stat(path); err != nil {
		return nil, coded("E607", "the run store does not exist")
	}
	readers, err := openReadDB(path)
	if err != nil {
		return nil, err
	}
	version := opts.ReaderVersion
	if version == 0 {
		version = 1
	}
	if err = checkReadable(context.Background(), readers, version); err != nil {
		_ = readers.Close()
		return nil, err
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	now := opts.Clock
	if now == nil {
		now = time.Now
	}
	return &Store{readers: readers, path: path, ttl: ttl, now: now, lastBeat: map[string]time.Time{}}, nil
}

func openReadDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// checkReadable compares the database with what migrate would apply, reads only.
func checkReadable(ctx context.Context, db *sql.DB, readerVersion int) error {
	var minimum int
	if err := db.QueryRowContext(ctx, "SELECT min_reader_version FROM schema_meta WHERE id=1").Scan(&minimum); err != nil {
		return coded("E607", "the run store has no readable schema")
	}
	if minimum > readerVersion {
		return coded("E606", fmt.Sprintf("database requires reader %d; upgrade nself-ci", minimum))
	}
	steps, err := migrationList()
	if err != nil {
		return err
	}
	for _, step := range steps {
		var n int
		if err = db.QueryRowContext(ctx, "SELECT count(*) FROM migration_log WHERE set_name=? AND filename=?", step.set, step.name).Scan(&n); err != nil {
			return coded("E607", "the run store has no readable schema")
		}
		if n == 0 {
			return coded("E607", fmt.Sprintf("the run store schema is older than this binary (missing %s/%s); run any nself ci command to upgrade it", step.set, step.name))
		}
	}
	return nil
}
