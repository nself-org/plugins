package internal

import (
	"testing"
)

// newTestHandler creates a Handler with a dummy databaseURL for testing pure
// helper methods that do not require a live DB or store.
func newTestHandler(dbURL string) *Handler {
	return &Handler{
		databaseURL: dbURL,
		storagePath: "/tmp/backups",
		pgDumpPath:  "pg_dump",
	}
}

// TestBuildPgDumpArgs_FullBackup verifies that a "full" backup type does not add
// --schema-only or --data-only flags.
func TestBuildPgDumpArgs_FullBackup(t *testing.T) {
	h := newTestHandler("postgres://user:pass@host/db")
	args := h.buildPgDumpArgs("full")
	for _, a := range args {
		if a == "--schema-only" || a == "--data-only" {
			t.Errorf("unexpected flag %q in full backup args: %v", a, args)
		}
	}
	// --format=custom must always be present
	found := false
	for _, a := range args {
		if a == "--format=custom" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing --format=custom in args: %v", args)
	}
}

// TestBuildPgDumpArgs_SchemaOnly verifies that schema_only adds --schema-only.
func TestBuildPgDumpArgs_SchemaOnly(t *testing.T) {
	h := newTestHandler("postgres://user:pass@host/db")
	args := h.buildPgDumpArgs("schema_only")
	found := false
	for _, a := range args {
		if a == "--schema-only" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --schema-only in args: %v", args)
	}
}

// TestBuildPgDumpArgs_DataOnly verifies that data_only adds --data-only.
func TestBuildPgDumpArgs_DataOnly(t *testing.T) {
	h := newTestHandler("postgres://user:pass@host/db")
	args := h.buildPgDumpArgs("data_only")
	found := false
	for _, a := range args {
		if a == "--data-only" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --data-only in args: %v", args)
	}
}

// TestBuildPgDumpArgs_ContainsDatabaseURL verifies that the database URL is passed
// via --dbname so pg_dump resolves connection details from it.
func TestBuildPgDumpArgs_ContainsDatabaseURL(t *testing.T) {
	dbURL := "postgres://user:pass@localhost:5432/mydb"
	h := newTestHandler(dbURL)
	args := h.buildPgDumpArgs("full")

	// Find the --dbname arg followed by the URL
	foundDBName := false
	for i, a := range args {
		if a == "--dbname" && i+1 < len(args) && args[i+1] == dbURL {
			foundDBName = true
			break
		}
	}
	if !foundDBName {
		t.Errorf("--dbname %q not found in args: %v", dbURL, args)
	}
}

// TestMarshalJSON_SimpleStruct verifies that MarshalJSON returns valid JSON bytes.
func TestMarshalJSON_SimpleStruct(t *testing.T) {
	type simple struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	b := MarshalJSON(simple{Name: "test", Age: 42})
	if len(b) == 0 {
		t.Error("MarshalJSON returned empty bytes")
	}
	s := string(b)
	if s[0] != '{' {
		t.Errorf("expected JSON object, got: %s", s)
	}
}
