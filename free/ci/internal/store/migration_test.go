package store

import (
	"context"
	"embed"
	"path/filepath"
	"testing"
)

//go:embed migrations/testextra/*.sql
var testMigrations embed.FS

func TestRegisteredMigrationsAfterCore(t *testing.T) {
	if err := Register("testextra", testMigrations); err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	var order int
	err = s.readers.QueryRowContext(context.Background(), "SELECT count(*) FROM migration_log WHERE set_name='core' AND filename='0001_core.sql'").Scan(&order)
	if err != nil || order != 1 {
		t.Fatalf("core migration absent: %d %v", order, err)
	}
	err = s.readers.QueryRowContext(context.Background(), "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='extra_test'").Scan(&order)
	if err != nil || order != 1 {
		t.Fatalf("extra migration absent: %d %v", order, err)
	}
}
