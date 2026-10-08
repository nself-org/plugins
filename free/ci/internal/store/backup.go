package store

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Backup writes a consistent SQLite snapshot through VACUUM INTO.
func (s *Store) Backup(ctx context.Context, w io.Writer) error {
	dir, err := os.MkdirTemp(filepath.Dir(s.path), "backup-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	snap := filepath.Join(dir, "state.db")
	quote := "'" + strings.ReplaceAll(snap, "'", "''") + "'"
	if _, err = s.writer.ExecContext(ctx, "VACUUM INTO "+quote); err != nil {
		return err
	}
	f, err := os.Open(snap)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(w, f)
	return err
}

// Restore replaces the local database from a snapshot and fences old coordinators.
// The caller must close this Store and open the returned Store for subsequent work.
func (s *Store) Restore(ctx context.Context, r io.Reader) (*Store, error) {
	dir := filepath.Dir(s.path)
	temp, err := os.CreateTemp(dir, "restore-*.db")
	if err != nil {
		return nil, err
	}
	name := temp.Name()
	defer func() { _ = os.Remove(name) }()
	if err = temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return nil, err
	}
	if _, err = io.Copy(temp, r); err != nil {
		_ = temp.Close()
		return nil, err
	}
	if err = temp.Close(); err != nil {
		return nil, err
	}
	probe, err := openDB(name, 1)
	if err != nil {
		return nil, err
	}
	var ok string
	err = probe.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&ok)
	_ = probe.Close()
	if err != nil || ok != "ok" {
		return nil, coded("E607", "invalid restore image")
	}
	if err = s.Close(); err != nil {
		return nil, err
	}
	if err = os.Rename(name, s.path); err != nil {
		return nil, err
	}
	restored, err := Open(s.path, Options{TTL: s.ttl, Clock: s.now})
	if err != nil {
		return nil, err
	}
	err = restored.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE coordinator SET incarnation=incarnation+1")
		return err
	})
	if err != nil {
		_ = restored.Close()
		return nil, err
	}
	return restored, nil
}
