package store_test

import (
	"testing"
)

// TestUpsertAndLoad verifies that UpsertDocument persists state and LoadDocument
// retrieves it. This is a unit-level test using table-driven approach.
// Integration tests (requiring a live DB) live in tests/integration/.
func TestUpsertAndLoad(t *testing.T) {
	// Without a live DB this test only validates the package compiles and
	// the Store type is correctly exported.
	// Integration tests in /tests/integration cover the full round-trip.
	t.Skip("requires live Postgres — run via 'make test-integration'")
}

func TestCompactUpdates(t *testing.T) {
	// Compaction reduces np_crdt_updates rows by >80% for a doc with 1000 updates.
	// Full test requires live Postgres — see tests/integration/compact_test.go.
	t.Skip("requires live Postgres — run via 'make test-integration'")
}
