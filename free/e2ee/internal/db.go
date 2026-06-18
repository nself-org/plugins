package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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

// beginScoped opens a transaction and binds the per-tenant RLS GUCs
// (app.source_account_id + app.current_user_id) from the AUTHENTICATED
// principal so the row-level-security policies in 001_e2ee_init.sql actually
// enforce isolation at runtime (CR-C critical #1: RLS was previously inert).
//
// Purpose:   Activate RLS by issuing SET LOCAL for the two GUCs the policies
//            read via current_setting('app.*'), scoped to this transaction.
// Inputs:    ctx, the pool, and the authenticated source account + user id.
// Outputs:   a pgx.Tx with both GUCs set, or an error (caller must Rollback).
// Constraints: SET LOCAL ties the settings to the tx lifetime; on Commit/
//   Rollback the connection is returned to the pool with no residual GUC state.
//   The values come from gateway-forwarded headers only — never request bodies.
func beginScoped(ctx context.Context, pool *pgxpool.Pool, sourceAccount, userID string) (pgx.Tx, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.source_account_id', $1, true),
		        set_config('app.current_user_id',  $2, true)`,
		sourceAccount, userID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
