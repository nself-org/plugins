package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ============================================================================
// Encryption Keys CRUD
// ============================================================================

// CreateEncryptionKey inserts a new HLS encryption key.
func (d *DB) CreateEncryptionKey(contentID, keyMaterial, keyIV, keyURI string) (*EncryptionKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var k EncryptionKey
	err := d.pool.QueryRow(ctx,
		`INSERT INTO np_tokens_encryption_keys (source_account_id, content_id, key_material_encrypted, key_iv, key_uri)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, source_account_id, content_id, key_material_encrypted, key_iv, key_uri, rotation_generation, is_active, created_at, rotated_at, expires_at`,
		d.sourceAccountID, contentID, keyMaterial, keyIV, keyURI,
	).Scan(&k.ID, &k.SourceAccountID, &k.ContentID, &k.KeyMaterialEncrypted, &k.KeyIV,
		&k.KeyURI, &k.RotationGeneration, &k.IsActive, &k.CreatedAt, &k.RotatedAt, &k.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("create encryption key: %w", err)
	}
	return &k, nil
}

// GetEncryptionKeyByID returns an encryption key by its UUID.
func (d *DB) GetEncryptionKeyByID(id string) (*EncryptionKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var k EncryptionKey
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, content_id, key_material_encrypted, key_iv, key_uri, rotation_generation, is_active, created_at, rotated_at, expires_at
		 FROM np_tokens_encryption_keys WHERE id = $1 AND source_account_id = $2`,
		id, d.sourceAccountID,
	).Scan(&k.ID, &k.SourceAccountID, &k.ContentID, &k.KeyMaterialEncrypted, &k.KeyIV,
		&k.KeyURI, &k.RotationGeneration, &k.IsActive, &k.CreatedAt, &k.RotatedAt, &k.ExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get encryption key: %w", err)
	}
	return &k, nil
}

// GetActiveEncryptionKey returns the active encryption key for a content ID.
func (d *DB) GetActiveEncryptionKey(contentID string) (*EncryptionKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var k EncryptionKey
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, content_id, key_material_encrypted, key_iv, key_uri, rotation_generation, is_active, created_at, rotated_at, expires_at
		 FROM np_tokens_encryption_keys
		 WHERE source_account_id = $1 AND content_id = $2 AND is_active = true
		 ORDER BY rotation_generation DESC LIMIT 1`,
		d.sourceAccountID, contentID,
	).Scan(&k.ID, &k.SourceAccountID, &k.ContentID, &k.KeyMaterialEncrypted, &k.KeyIV,
		&k.KeyURI, &k.RotationGeneration, &k.IsActive, &k.CreatedAt, &k.RotatedAt, &k.ExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active encryption key: %w", err)
	}
	return &k, nil
}

// RotateEncryptionKey creates a new encryption key for a content ID and sets old keys to expire.
func (d *DB) RotateEncryptionKey(contentID, newKeyMaterial, newKeyIV, newKeyURI string, expireOldAfterHours int) (*EncryptionKey, error) {
	current, err := d.GetActiveEncryptionKey(contentID)
	if err != nil {
		return nil, err
	}
	nextGeneration := 1
	if current != nil {
		nextGeneration = current.RotationGeneration + 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Set old keys to expire
	expiresAt := time.Now().Add(time.Duration(expireOldAfterHours) * time.Hour)
	_, err = d.pool.Exec(ctx,
		`UPDATE np_tokens_encryption_keys SET rotated_at = NOW(), expires_at = $3
		 WHERE source_account_id = $1 AND content_id = $2 AND is_active = true`,
		d.sourceAccountID, contentID, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("expire old encryption keys: %w", err)
	}

	var k EncryptionKey
	err = d.pool.QueryRow(ctx,
		`INSERT INTO np_tokens_encryption_keys (source_account_id, content_id, key_material_encrypted, key_iv, key_uri, rotation_generation)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, source_account_id, content_id, key_material_encrypted, key_iv, key_uri, rotation_generation, is_active, created_at, rotated_at, expires_at`,
		d.sourceAccountID, contentID, newKeyMaterial, newKeyIV, newKeyURI, nextGeneration,
	).Scan(&k.ID, &k.SourceAccountID, &k.ContentID, &k.KeyMaterialEncrypted, &k.KeyIV,
		&k.KeyURI, &k.RotationGeneration, &k.IsActive, &k.CreatedAt, &k.RotatedAt, &k.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("insert rotated encryption key: %w", err)
	}

	return &k, nil
}

