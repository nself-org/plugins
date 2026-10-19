// Package tenantdb runs byok database work under one tenant's row-level security.
//
// The byok tables force RLS with policies keyed on the app.current_tenant_id setting
// (migrations 002 and 005). A role without BYPASSRLS sees no rows until that setting is
// present, so every statement runs inside a transaction that sets it first. The setting is
// transaction-local (set_config(..., true)): a pooled connection never carries one tenant's
// id into the next request.
package tenantdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Setting is the custom GUC the RLS policies read.
const Setting = "app.current_tenant_id"

// ErrNoTenant is returned when the tenant id is the zero UUID: running unscoped would
// silently return nothing under RLS, so the caller is told instead.
var ErrNoTenant = errors.New("tenantdb: tenant id required")

// Beginner is the part of *pgxpool.Pool that With needs.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// With runs fn in a transaction whose app.current_tenant_id is tenant. It commits when fn
// returns nil and rolls back otherwise. fn must finish reading its rows before it returns.
func With(ctx context.Context, db Beginner, tenant uuid.UUID, fn func(tx pgx.Tx) error) error {
	if tenant == uuid.Nil {
		return ErrNoTenant
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tenantdb: begin: %w", err)
	}
	// A rollback after a successful commit is a no-op; use a fresh context so a cancelled
	// request still releases the connection.
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if _, err := tx.Exec(ctx, "SELECT set_config($1, $2, true)", Setting, tenant.String()); err != nil {
		return fmt.Errorf("tenantdb: set %s: %w", Setting, err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("tenantdb: commit: %w", err)
	}
	return nil
}
