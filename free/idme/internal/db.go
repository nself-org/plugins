package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxIface is the subset of *pgxpool.Pool used by the handlers. Declaring it
// as an interface (rather than depending on the concrete *pgxpool.Pool type)
// lets tests substitute a pgxmock.PgxPoolIface without a real database.
type PgxIface interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// DB wraps a pgxpool.Pool.
type DB struct {
	Pool PgxIface

	closer interface{ Close() }
}

// NewDB creates a new DB from the given connection string.
func NewDB(ctx context.Context, connStr string) (*DB, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return &DB{Pool: pool, closer: pool}, nil
}

// Close closes the pool.
func (d *DB) Close() {
	if d.closer != nil {
		d.closer.Close()
	}
}
