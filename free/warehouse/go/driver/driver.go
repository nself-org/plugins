// Package driver defines the warehouse target interface.
package driver

import "context"

// Driver writes batches of rows to a warehouse target and can synchronise
// the mirror schema on first run.
type Driver interface {
	// WriteBatch inserts or upserts a slice of rows into the named table.
	// The table name mirrors the source np_* Postgres table name.
	WriteBatch(ctx context.Context, table string, rows []map[string]interface{}) error

	// EnsureTable creates the target table if it does not exist, based on
	// the provided column map (col name → warehouse type string).
	EnsureTable(ctx context.Context, table string, columns map[string]string) error

	// Close releases any held connections.
	Close() error
}
