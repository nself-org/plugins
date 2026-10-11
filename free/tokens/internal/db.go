package internal

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with tokens table operations.
type DB struct {
	pool            *pgxpool.Pool
	sourceAccountID string
}

// NewDB creates a new DB wrapper with the default "primary" source account.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool, sourceAccountID: "primary"}
}

// ForSourceAccount returns a new DB scoped to a specific source account.
func (d *DB) ForSourceAccount(sourceAccountID string) *DB {
	return &DB{pool: d.pool, sourceAccountID: sourceAccountID}
}

// ============================================================================
