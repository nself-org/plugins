package internal

import "github.com/jackc/pgx/v5/pgxpool"

// DB wraps a pgxpool.Pool with donorbox-specific database operations.
type DB struct {
	pool            *pgxpool.Pool
	sourceAccountID string
}

// NewDB creates a new DB instance bound to the "primary" source account.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool, sourceAccountID: "primary"}
}

// ForSourceAccount returns a new DB scoped to the given source account ID.
func (db *DB) ForSourceAccount(accountID string) *DB {
	return &DB{pool: db.pool, sourceAccountID: accountID}
}

// Pool returns the underlying connection pool.
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}
