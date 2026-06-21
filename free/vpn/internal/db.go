package internal

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool and scopes all queries to a source_account_id.
type DB struct {
	pool            *pgxpool.Pool
	sourceAccountID string
}

// NewDB creates a DB handle scoped to source_account_id "primary".
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool, sourceAccountID: "primary"}
}

// ForSourceAccount returns a new DB handle scoped to the given account,
// sharing the same underlying connection pool.
func (d *DB) ForSourceAccount(accountID string) *DB {
	return &DB{pool: d.pool, sourceAccountID: accountID}
}

// SourceAccountID returns the current scoped account.
func (d *DB) SourceAccountID() string {
	return d.sourceAccountID
}

// Pool returns the underlying pgxpool.Pool.
func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}

