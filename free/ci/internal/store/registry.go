package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
)

//go:embed migrations/core/*.sql
var coreMigrations embed.FS

type migration struct{ set, name, sql string }

var registryMu sync.Mutex
var registered = map[string]embed.FS{}

// Register adds an expand-only migration set. A set is immutable after first open.
func Register(set string, source embed.FS) error {
	if set == "" || set == "core" || strings.ContainsAny(set, "/\\.") {
		return coded("E607", "invalid migration set")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registered[set]; exists {
		return coded("E607", "duplicate migration set")
	}
	registered[set] = source
	return nil
}

func migrationList() ([]migration, error) {
	registryMu.Lock()
	defer registryMu.Unlock()
	sets := []string{"core"}
	for set := range registered {
		sets = append(sets, set)
	}
	sort.Strings(sets[1:])
	out := []migration{}
	for _, set := range sets {
		source := fs.FS(coreMigrations)
		dir := "migrations/core"
		if set != "core" {
			source = registered[set]
			dir = "migrations/" + set
		}
		entries, err := fs.ReadDir(source, dir)
		if err != nil {
			return nil, err
		}
		names := []string{}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		if len(names) == 0 {
			return nil, fmt.Errorf("migration set %s is empty", set)
		}
		for _, name := range names {
			b, err := fs.ReadFile(source, path.Join(dir, name))
			if err != nil {
				return nil, err
			}
			out = append(out, migration{set, name, string(b)})
		}
	}
	return out, nil
}

// migrate brings the database to the registered schema in ONE write transaction:
// the bootstrap tables, the reader-version check, and every step not yet in
// migration_log, applied in registry order. One transaction means one commit
// (one WAL sync) for a fresh database instead of one per step, and an upgrade is
// all-or-nothing: a failing step rolls back every step of this open, so a
// database is never left half-upgraded. The writer opens transactions with
// BEGIN IMMEDIATE, so a concurrent opener waits for the lock and then finds
// every step logged. A database already at the schema commits an empty
// transaction (no write).
func migrate(ctx context.Context, db *sql.DB, readerVersion int) error {
	steps, err := migrationList()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err = migrateTx(ctx, tx, readerVersion, steps); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func migrateTx(ctx context.Context, tx *sql.Tx, readerVersion int, steps []migration) error {
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_meta (id INTEGER PRIMARY KEY CHECK(id=1), schema_version INTEGER NOT NULL, min_reader_version INTEGER NOT NULL); INSERT OR IGNORE INTO schema_meta VALUES (1,0,1); CREATE TABLE IF NOT EXISTS migration_log (set_name TEXT NOT NULL, filename TEXT NOT NULL, PRIMARY KEY(set_name,filename));`); err != nil {
		return err
	}
	var minimum int
	if err := tx.QueryRowContext(ctx, "SELECT min_reader_version FROM schema_meta WHERE id=1").Scan(&minimum); err != nil {
		return err
	}
	if minimum > readerVersion {
		return coded("E606", fmt.Sprintf("database requires reader %d; upgrade nself-ci", minimum))
	}
	applied, err := appliedSteps(ctx, tx)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if applied[step.set+"/"+step.name] {
			continue
		}
		_, err = tx.ExecContext(ctx, step.sql)
		if err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO migration_log VALUES (?,?)", step.set, step.name)
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, "UPDATE schema_meta SET schema_version=schema_version+1 WHERE id=1")
		}
		if err != nil {
			return fmt.Errorf("migration %s/%s: %w", step.set, step.name, err)
		}
	}
	return nil
}

// appliedSteps reads migration_log once, keyed "set/filename".
func appliedSteps(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, "SELECT set_name, filename FROM migration_log")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var set, name string
		if err := rows.Scan(&set, &name); err != nil {
			return nil, err
		}
		out[set+"/"+name] = true
	}
	return out, rows.Err()
}
