package internal

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with mDNS table operations.
type DB struct {
	pool            *pgxpool.Pool
	sourceAccountID string
}

// NewDB creates a new DB wrapper with source_account_id defaulting to "primary".
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool, sourceAccountID: "primary"}
}

// ForSourceAccount returns a new DB scoped to a specific source_account_id.
func (d *DB) ForSourceAccount(id string) *DB {
	return &DB{pool: d.pool, sourceAccountID: id}
}

// Ping checks database connectivity.
func (d *DB) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return d.pool.Ping(ctx)
}

// --- Service CRUD ---

// CreateService inserts a new mDNS service record.
