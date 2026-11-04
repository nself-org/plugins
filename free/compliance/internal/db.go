package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxIface is the subset of *pgxpool.Pool used by handlers.
// Declaring it as an interface (rather than depending on the concrete
// *pgxpool.Pool type) lets tests substitute a pgxmock.PgxPoolIface without a
// real database. Mirrors the pattern established in paid/moderation.
type PgxIface interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Ping(ctx context.Context) error
}

// DB wraps a pgx connection pool. The schema comes from the boot migrations (NewDB), not Go DDL.
type DB struct {
	Pool PgxIface

	closer interface{ Close() }
}

// NewDB opens a connection pool using the provided DATABASE_URL.
func NewDB(ctx context.Context, databaseURL string) (*DB, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	mustBootMigrations(ctx, pool)

	return &DB{Pool: pool, closer: pool}, nil
}

// Close releases all pool connections.
func (d *DB) Close() {
	if d.closer != nil {
		d.closer.Close()
	}
}
