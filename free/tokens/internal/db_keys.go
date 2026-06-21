package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ============================================================================
// Signing Keys CRUD
// ============================================================================

// CreateSigningKey inserts a new signing key.
func (d *DB) CreateSigningKey(name, algorithm, keyMaterialEncrypted string) (*SigningKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var k SigningKey
	err := d.pool.QueryRow(ctx,
		`INSERT INTO np_tokens_signing_keys (source_account_id, name, algorithm, key_material_encrypted)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, source_account_id, name, algorithm, key_material_encrypted, is_active, rotated_from, created_at, rotated_at, expires_at`,
		d.sourceAccountID, name, algorithm, keyMaterialEncrypted,
	).Scan(&k.ID, &k.SourceAccountID, &k.Name, &k.Algorithm, &k.KeyMaterialEncrypted,
		&k.IsActive, &k.RotatedFrom, &k.CreatedAt, &k.RotatedAt, &k.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("create signing key: %w", err)
	}
	return &k, nil
}

// GetSigningKey returns a signing key by ID.
func (d *DB) GetSigningKey(id string) (*SigningKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var k SigningKey
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, name, algorithm, key_material_encrypted, is_active, rotated_from, created_at, rotated_at, expires_at
		 FROM np_tokens_signing_keys WHERE id = $1 AND source_account_id = $2`,
		id, d.sourceAccountID,
	).Scan(&k.ID, &k.SourceAccountID, &k.Name, &k.Algorithm, &k.KeyMaterialEncrypted,
		&k.IsActive, &k.RotatedFrom, &k.CreatedAt, &k.RotatedAt, &k.ExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get signing key: %w", err)
	}
	return &k, nil
}

// GetActiveSigningKey returns the most recently created active signing key.
func (d *DB) GetActiveSigningKey() (*SigningKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var k SigningKey
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, name, algorithm, key_material_encrypted, is_active, rotated_from, created_at, rotated_at, expires_at
		 FROM np_tokens_signing_keys
		 WHERE source_account_id = $1 AND is_active = true
		 ORDER BY created_at DESC LIMIT 1`,
		d.sourceAccountID,
	).Scan(&k.ID, &k.SourceAccountID, &k.Name, &k.Algorithm, &k.KeyMaterialEncrypted,
		&k.IsActive, &k.RotatedFrom, &k.CreatedAt, &k.RotatedAt, &k.ExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active signing key: %w", err)
	}
	return &k, nil
}

// ListSigningKeys returns all signing keys for the source account.
func (d *DB) ListSigningKeys() ([]SigningKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, name, algorithm, key_material_encrypted, is_active, rotated_from, created_at, rotated_at, expires_at
		 FROM np_tokens_signing_keys
		 WHERE source_account_id = $1
		 ORDER BY created_at DESC`,
		d.sourceAccountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list signing keys: %w", err)
	}
	defer rows.Close()

	var keys []SigningKey
	for rows.Next() {
		var k SigningKey
		if err := rows.Scan(&k.ID, &k.SourceAccountID, &k.Name, &k.Algorithm, &k.KeyMaterialEncrypted,
			&k.IsActive, &k.RotatedFrom, &k.CreatedAt, &k.RotatedAt, &k.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan signing key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// RotateSigningKey creates a new key rotated from an old one, and sets the old key to expire.
func (d *DB) RotateSigningKey(id, newKeyMaterial string, expireOldAfterHours int) (*SigningKey, error) {
	oldKey, err := d.GetSigningKey(id)
	if err != nil {
		return nil, err
	}
	if oldKey == nil {
		return nil, fmt.Errorf("signing key not found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var k SigningKey
	err = d.pool.QueryRow(ctx,
		`INSERT INTO np_tokens_signing_keys (source_account_id, name, algorithm, key_material_encrypted, rotated_from)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, source_account_id, name, algorithm, key_material_encrypted, is_active, rotated_from, created_at, rotated_at, expires_at`,
		d.sourceAccountID, oldKey.Name, oldKey.Algorithm, newKeyMaterial, id,
	).Scan(&k.ID, &k.SourceAccountID, &k.Name, &k.Algorithm, &k.KeyMaterialEncrypted,
		&k.IsActive, &k.RotatedFrom, &k.CreatedAt, &k.RotatedAt, &k.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("insert rotated signing key: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(expireOldAfterHours) * time.Hour)
	_, err = d.pool.Exec(ctx,
		`UPDATE np_tokens_signing_keys SET rotated_at = NOW(), expires_at = $3
		 WHERE id = $1 AND source_account_id = $2`,
		id, d.sourceAccountID, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("expire old signing key: %w", err)
	}

	return &k, nil
}

// DeactivateSigningKey marks a signing key as inactive.
func (d *DB) DeactivateSigningKey(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := d.pool.Exec(ctx,
		`UPDATE np_tokens_signing_keys SET is_active = false
		 WHERE id = $1 AND source_account_id = $2`,
		id, d.sourceAccountID,
	)
	return err
}

