package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a pgxpool connection pool from cfg.DatabaseURL.
//
// Purpose:   Establish the single shared Postgres pool for the plugin.
// Inputs:    ctx (for dial timeout), cfg.DatabaseURL.
// Outputs:   *pgxpool.Pool, or error if URL missing / unreachable.
// Constraints: DATABASE_URL must be set; the pool is pinged before return so a
//   misconfigured database fails fast at startup rather than on first request.
func NewPool(ctx context.Context, cfg *Config) (*pgxpool.Pool, error) {
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}
