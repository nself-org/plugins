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

func migrate(ctx context.Context, db *sql.DB, readerVersion int) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_meta (id INTEGER PRIMARY KEY CHECK(id=1), schema_version INTEGER NOT NULL, min_reader_version INTEGER NOT NULL); INSERT OR IGNORE INTO schema_meta VALUES (1,0,1); CREATE TABLE IF NOT EXISTS migration_log (set_name TEXT NOT NULL, filename TEXT NOT NULL, PRIMARY KEY(set_name,filename));`); err != nil {
		return err
	}
	var minimum int
	if err := db.QueryRowContext(ctx, "SELECT min_reader_version FROM schema_meta WHERE id=1").Scan(&minimum); err != nil {
		return err
	}
	if minimum > readerVersion {
		return coded("E606", fmt.Sprintf("database requires reader %d; upgrade nself-ci", minimum))
	}
	steps, err := migrationList()
	if err != nil {
		return err
	}
	for _, step := range steps {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		var exists int
		err = tx.QueryRowContext(ctx, "SELECT count(*) FROM migration_log WHERE set_name=? AND filename=?", step.set, step.name).Scan(&exists)
		if err == nil && exists == 0 {
			_, err = tx.ExecContext(ctx, step.sql)
		}
		if err == nil && exists == 0 {
			_, err = tx.ExecContext(ctx, "INSERT INTO migration_log VALUES (?,?)", step.set, step.name)
		}
		if err == nil && exists == 0 {
			_, err = tx.ExecContext(ctx, "UPDATE schema_meta SET schema_version=schema_version+1 WHERE id=1")
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s/%s: %w", step.set, step.name, err)
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
