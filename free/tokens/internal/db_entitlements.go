package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ============================================================================
// Entitlements CRUD
// ============================================================================

// CheckEntitlement checks if a user has an active, non-expired entitlement for content.
func (d *DB) CheckEntitlement(userID, contentID, entitlementType string) (*Entitlement, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var e Entitlement
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, user_id, content_id, content_type, entitlement_type,
			granted_by, granted_at, expires_at, revoked, metadata
		 FROM np_tokens_entitlements
		 WHERE source_account_id = $1 AND user_id = $2 AND content_id = $3
		   AND entitlement_type = $4 AND revoked = false
		   AND (expires_at IS NULL OR expires_at > NOW())`,
		d.sourceAccountID, userID, contentID, entitlementType,
	).Scan(&e.ID, &e.SourceAccountID, &e.UserID, &e.ContentID, &e.ContentType,
		&e.EntitlementType, &e.GrantedBy, &e.GrantedAt, &e.ExpiresAt, &e.Revoked, &e.Metadata)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check entitlement: %w", err)
	}
	return &e, nil
}

// HasAnyEntitlements checks if a user has any entitlements at all (regardless of status).
func (d *DB) HasAnyEntitlements(userID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	err := d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tokens_entitlements WHERE source_account_id = $1 AND user_id = $2`,
		d.sourceAccountID, userID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("has any entitlements: %w", err)
	}
	return count > 0, nil
}

// GrantEntitlement creates or upserts an entitlement for a user/content pair.
func (d *DB) GrantEntitlement(p GrantEntitlementParams) (*Entitlement, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	metaJSON, err := json.Marshal(p.Metadata)
	if err != nil {
		metaJSON = []byte("{}")
	}

	var e Entitlement
	err = d.pool.QueryRow(ctx,
		`INSERT INTO np_tokens_entitlements (
			source_account_id, user_id, content_id, content_type,
			entitlement_type, expires_at, metadata, granted_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (source_account_id, user_id, content_id, entitlement_type) DO UPDATE SET
			content_type = EXCLUDED.content_type,
			expires_at = EXCLUDED.expires_at,
			metadata = EXCLUDED.metadata,
			granted_by = EXCLUDED.granted_by,
			revoked = false,
			granted_at = NOW()
		RETURNING id, source_account_id, user_id, content_id, content_type, entitlement_type,
			granted_by, granted_at, expires_at, revoked, metadata`,
		d.sourceAccountID, p.UserID, p.ContentID, p.ContentType,
		p.EntitlementType, p.ExpiresAt, metaJSON, p.GrantedBy,
	).Scan(&e.ID, &e.SourceAccountID, &e.UserID, &e.ContentID, &e.ContentType,
		&e.EntitlementType, &e.GrantedBy, &e.GrantedAt, &e.ExpiresAt, &e.Revoked, &e.Metadata)
	if err != nil {
		return nil, fmt.Errorf("grant entitlement: %w", err)
	}
	return &e, nil
}

// RevokeEntitlement marks a specific entitlement as revoked.
func (d *DB) RevokeEntitlement(userID, contentID, entitlementType string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := d.pool.Exec(ctx,
		`UPDATE np_tokens_entitlements SET revoked = true
		 WHERE source_account_id = $1 AND user_id = $2 AND content_id = $3 AND entitlement_type = $4`,
		d.sourceAccountID, userID, contentID, entitlementType,
	)
	return err
}

// ListUserEntitlements returns all entitlements for a user, optionally filtered.
func (d *DB) ListUserEntitlements(userID string, contentType *string, activeOnly bool) ([]Entitlement, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT id, source_account_id, user_id, content_id, content_type, entitlement_type,
		granted_by, granted_at, expires_at, revoked, metadata
		FROM np_tokens_entitlements
		WHERE source_account_id = $1 AND user_id = $2`
	args := []interface{}{d.sourceAccountID, userID}
	paramIdx := 3

	if contentType != nil {
		query += fmt.Sprintf(" AND content_type = $%d", paramIdx)
		args = append(args, *contentType)
		paramIdx++
	}

	if activeOnly {
		query += " AND revoked = false AND (expires_at IS NULL OR expires_at > NOW())"
	}

	query += " ORDER BY granted_at DESC"

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list user entitlements: %w", err)
	}
	defer rows.Close()

	var entitlements []Entitlement
	for rows.Next() {
		var e Entitlement
		if err := rows.Scan(&e.ID, &e.SourceAccountID, &e.UserID, &e.ContentID, &e.ContentType,
			&e.EntitlementType, &e.GrantedBy, &e.GrantedAt, &e.ExpiresAt, &e.Revoked, &e.Metadata); err != nil {
			return nil, fmt.Errorf("scan entitlement: %w", err)
		}
		entitlements = append(entitlements, e)
	}
	return entitlements, rows.Err()
}

// GetEntitlementByID returns a single entitlement by ID.
func (d *DB) GetEntitlementByID(id string) (*Entitlement, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var e Entitlement
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, user_id, content_id, content_type, entitlement_type,
			granted_by, granted_at, expires_at, revoked, metadata
		 FROM np_tokens_entitlements WHERE id = $1 AND source_account_id = $2`,
		id, d.sourceAccountID,
	).Scan(&e.ID, &e.SourceAccountID, &e.UserID, &e.ContentID, &e.ContentType,
		&e.EntitlementType, &e.GrantedBy, &e.GrantedAt, &e.ExpiresAt, &e.Revoked, &e.Metadata)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get entitlement: %w", err)
	}
	return &e, nil
}

// DeleteEntitlementByID deletes an entitlement by ID. Returns true if a row was deleted.
func (d *DB) DeleteEntitlementByID(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_tokens_entitlements WHERE id = $1 AND source_account_id = $2`,
		id, d.sourceAccountID,
	)
	if err != nil {
		return false, fmt.Errorf("delete entitlement: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

