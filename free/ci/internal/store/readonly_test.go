package store

// Purpose: OpenReadOnly (D-1094) opens an existing database for reads only. It
// never creates a file, never migrates, and refuses a schema it cannot read.

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func seededDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	seed(t, s, "ro1")
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func rawExec(t *testing.T, path, q string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec(q); err != nil {
		t.Fatal(err)
	}
}

func schemaVersion(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var v int
	if err = db.QueryRow("SELECT schema_version FROM schema_meta WHERE id=1").Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestOpenReadOnlyReadsAndRefusesWrites(t *testing.T) {
	path := seededDB(t)
	s, err := OpenReadOnly(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, ok, err := s.PipelineByID(context.Background(), "pro1"); err != nil || !ok {
		t.Fatalf("read failed: ok=%v err=%v", ok, err)
	}
	if err = s.AppendAudit(context.Background(), "a", "b", "c", "d"); err == nil {
		t.Fatal("a read-only store accepted a write")
	}
}

func TestOpenReadOnlyCreatesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	if s, err := OpenReadOnly(filepath.Join(dir, "state.db"), Options{}); err == nil {
		_ = s.Close()
		t.Fatal("opened a database that does not exist")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("OpenReadOnly created the directory: %v", err)
	}
	empty := filepath.Join(t.TempDir(), "state.db")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := OpenReadOnly(empty, Options{}); err == nil {
		_ = s.Close()
		t.Fatal("opened an empty file as a store")
	}
	if fi, err := os.Stat(empty); err != nil || fi.Size() != 0 {
		t.Fatalf("an empty file was written to: %v", err)
	}
}

func TestOpenReadOnlyDoesNotMigrate(t *testing.T) {
	path := seededDB(t)
	rawExec(t, path, "DELETE FROM migration_log WHERE rowid=(SELECT max(rowid) FROM migration_log); UPDATE schema_meta SET schema_version=schema_version-1")
	before := schemaVersion(t, path)
	s, err := OpenReadOnly(path, Options{})
	if err == nil {
		_ = s.Close()
		t.Fatal("opened a database with a migration missing")
	}
	if !codeIs(err, "E607") || !strings.Contains(err.Error(), "older") {
		t.Fatalf("unexpected refusal: %v", err)
	}
	if after := schemaVersion(t, path); after != before {
		t.Fatalf("schema version moved %d -> %d", before, after)
	}
}

func TestOpenReadOnlyRefusesNewerSchema(t *testing.T) {
	path := seededDB(t)
	rawExec(t, path, "UPDATE schema_meta SET min_reader_version=9")
	s, err := OpenReadOnly(path, Options{})
	if err == nil {
		_ = s.Close()
		t.Fatal("opened a database that needs a newer reader")
	}
	if !codeIs(err, "E606") {
		t.Fatalf("want E606, got %v", err)
	}
}
