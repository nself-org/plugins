package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with tokens table operations.
type DB struct {
	pool            *pgxpool.Pool
	sourceAccountID string
}

// NewDB creates a new DB wrapper with the default "primary" source account.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool, sourceAccountID: "primary"}
}

// ForSourceAccount returns a new DB scoped to a specific source account.
func (d *DB) ForSourceAccount(sourceAccountID string) *DB {
	return &DB{pool: d.pool, sourceAccountID: sourceAccountID}
}

// InitSchema creates all tokens tables and indexes if they do not exist.
func (d *DB) InitSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	schema := `
CREATE TABLE IF NOT EXISTS np_tokens_signing_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    name VARCHAR(255) NOT NULL,
    algorithm VARCHAR(20) NOT NULL DEFAULT 'hmac-sha256',
    key_material_encrypted TEXT NOT NULL,
    is_active BOOLEAN DEFAULT true,
    rotated_from UUID REFERENCES np_tokens_signing_keys(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    rotated_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    UNIQUE(source_account_id, name)
);
CREATE INDEX IF NOT EXISTS idx_np_tokens_signing_keys_source_app ON np_tokens_signing_keys(source_account_id);

CREATE TABLE IF NOT EXISTS np_tokens_issued (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    token_hash VARCHAR(128) NOT NULL,
    token_type VARCHAR(50) NOT NULL DEFAULT 'playback',
    signing_key_id UUID REFERENCES np_tokens_signing_keys(id),
    user_id VARCHAR(255) NOT NULL,
    device_id VARCHAR(255),
    content_id VARCHAR(255) NOT NULL,
    content_type VARCHAR(50),
    permissions JSONB DEFAULT '{}',
    ip_address VARCHAR(45),
    issued_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked BOOLEAN DEFAULT false,
    revoked_at TIMESTAMPTZ,
    revoked_reason VARCHAR(255),
    last_used_at TIMESTAMPTZ,
    use_count INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_np_tokens_issued_source_app ON np_tokens_issued(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_tokens_issued_hash ON np_tokens_issued(token_hash);
CREATE INDEX IF NOT EXISTS idx_np_tokens_issued_user ON np_tokens_issued(source_account_id, user_id);
CREATE INDEX IF NOT EXISTS idx_np_tokens_issued_content ON np_tokens_issued(source_account_id, content_id);
CREATE INDEX IF NOT EXISTS idx_np_tokens_issued_active ON np_tokens_issued(source_account_id, revoked, expires_at);

CREATE TABLE IF NOT EXISTS np_tokens_encryption_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    content_id VARCHAR(255) NOT NULL,
    key_material_encrypted TEXT NOT NULL,
    key_iv VARCHAR(64) NOT NULL,
    key_uri TEXT NOT NULL,
    rotation_generation INTEGER DEFAULT 1,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    rotated_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_tokens_enc_keys_source_app ON np_tokens_encryption_keys(source_account_id);
CREATE INDEX IF NOT EXISTS idx_tokens_enc_keys_content ON np_tokens_encryption_keys(source_account_id, content_id, is_active);

CREATE TABLE IF NOT EXISTS np_tokens_entitlements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    user_id VARCHAR(255) NOT NULL,
    content_id VARCHAR(255) NOT NULL,
    content_type VARCHAR(50),
    entitlement_type VARCHAR(50) NOT NULL DEFAULT 'stream',
    granted_by VARCHAR(50) DEFAULT 'system',
    granted_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    revoked BOOLEAN DEFAULT false,
    metadata JSONB DEFAULT '{}',
    UNIQUE(source_account_id, user_id, content_id, entitlement_type)
);
CREATE INDEX IF NOT EXISTS idx_np_tokens_entitlements_source_app ON np_tokens_entitlements(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_tokens_entitlements_user ON np_tokens_entitlements(source_account_id, user_id);

CREATE TABLE IF NOT EXISTS np_tokens_webhook_events (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    event_type VARCHAR(128) NOT NULL,
    payload JSONB NOT NULL,
    processed BOOLEAN DEFAULT false,
    processed_at TIMESTAMPTZ,
    error TEXT,
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_np_tokens_webhook_events_source_app ON np_tokens_webhook_events(source_account_id);
`
	_, err := d.pool.Exec(ctx, schema)
	return err
}

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

// ============================================================================
// Issued Tokens CRUD
// ============================================================================

// InsertIssuedToken records a newly issued token.
func (d *DB) InsertIssuedToken(p InsertTokenParams) (*IssuedToken, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	permJSON, err := json.Marshal(p.Permissions)
	if err != nil {
		permJSON = []byte("{}")
	}

	var t IssuedToken
	err = d.pool.QueryRow(ctx,
		`INSERT INTO np_tokens_issued (
			source_account_id, token_hash, token_type, signing_key_id, user_id,
			device_id, content_id, content_type, permissions, ip_address, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, source_account_id, token_hash, token_type, signing_key_id, user_id,
			device_id, content_id, content_type, permissions, ip_address, issued_at,
			expires_at, revoked, revoked_at, revoked_reason, last_used_at, use_count`,
		d.sourceAccountID, p.TokenHash, p.TokenType, p.SigningKeyID, p.UserID,
		p.DeviceID, p.ContentID, p.ContentType, permJSON, p.IPAddress, p.ExpiresAt,
	).Scan(&t.ID, &t.SourceAccountID, &t.TokenHash, &t.TokenType, &t.SigningKeyID,
		&t.UserID, &t.DeviceID, &t.ContentID, &t.ContentType, &t.Permissions,
		&t.IPAddress, &t.IssuedAt, &t.ExpiresAt, &t.Revoked, &t.RevokedAt,
		&t.RevokedReason, &t.LastUsedAt, &t.UseCount)
	if err != nil {
		return nil, fmt.Errorf("insert issued token: %w", err)
	}
	return &t, nil
}

// GetIssuedTokenByHash looks up an issued token by its hash.
func (d *DB) GetIssuedTokenByHash(tokenHash string) (*IssuedToken, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var t IssuedToken
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, token_hash, token_type, signing_key_id, user_id,
			device_id, content_id, content_type, permissions, ip_address, issued_at,
			expires_at, revoked, revoked_at, revoked_reason, last_used_at, use_count
		 FROM np_tokens_issued WHERE token_hash = $1`,
		tokenHash,
	).Scan(&t.ID, &t.SourceAccountID, &t.TokenHash, &t.TokenType, &t.SigningKeyID,
		&t.UserID, &t.DeviceID, &t.ContentID, &t.ContentType, &t.Permissions,
		&t.IPAddress, &t.IssuedAt, &t.ExpiresAt, &t.Revoked, &t.RevokedAt,
		&t.RevokedReason, &t.LastUsedAt, &t.UseCount)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get issued token by hash: %w", err)
	}
	return &t, nil
}

// UpdateTokenLastUsed bumps the use count and last_used_at timestamp.
func (d *DB) UpdateTokenLastUsed(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := d.pool.Exec(ctx,
		`UPDATE np_tokens_issued SET last_used_at = NOW(), use_count = use_count + 1
		 WHERE id = $1`,
		id,
	)
	return err
}

// RevokeToken marks a single token as revoked.
func (d *DB) RevokeToken(id, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	_, err := d.pool.Exec(ctx,
		`UPDATE np_tokens_issued SET revoked = true, revoked_at = NOW(), revoked_reason = $3
		 WHERE id = $1 AND source_account_id = $2`,
		id, d.sourceAccountID, reasonPtr,
	)
	return err
}

// RevokeUserTokens revokes all active tokens for a user. Returns the count revoked.
func (d *DB) RevokeUserTokens(userID, reason string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	tag, err := d.pool.Exec(ctx,
		`UPDATE np_tokens_issued SET revoked = true, revoked_at = NOW(), revoked_reason = $3
		 WHERE source_account_id = $1 AND user_id = $2 AND revoked = false`,
		d.sourceAccountID, userID, reasonPtr,
	)
	if err != nil {
		return 0, fmt.Errorf("revoke user tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RevokeContentTokens revokes all active tokens for a content ID. Returns the count revoked.
func (d *DB) RevokeContentTokens(contentID, reason string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	tag, err := d.pool.Exec(ctx,
		`UPDATE np_tokens_issued SET revoked = true, revoked_at = NOW(), revoked_reason = $3
		 WHERE source_account_id = $1 AND content_id = $2 AND revoked = false`,
		d.sourceAccountID, contentID, reasonPtr,
	)
	if err != nil {
		return 0, fmt.Errorf("revoke content tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}

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

// ============================================================================
// Webhook Events
// ============================================================================

// InsertWebhookEvent records an event for webhook delivery.
func (d *DB) InsertWebhookEvent(eventID, eventType string, payload map[string]interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		payloadJSON = []byte("{}")
	}

	_, err = d.pool.Exec(ctx,
		`INSERT INTO np_tokens_webhook_events (id, source_account_id, event_type, payload)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO NOTHING`,
		eventID, d.sourceAccountID, eventType, payloadJSON,
	)
	return err
}

// ============================================================================
// Statistics
// ============================================================================

// GetStats returns aggregate statistics for the source account.
func (d *DB) GetStats() (*TokensStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var stats TokensStats

	err := d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tokens_signing_keys WHERE source_account_id = $1`,
		d.sourceAccountID,
	).Scan(&stats.TotalSigningKeys)
	if err != nil {
		return nil, fmt.Errorf("count signing keys: %w", err)
	}

	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tokens_signing_keys WHERE source_account_id = $1 AND is_active = true`,
		d.sourceAccountID,
	).Scan(&stats.ActiveSigningKeys)
	if err != nil {
		return nil, fmt.Errorf("count active signing keys: %w", err)
	}

	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tokens_issued WHERE source_account_id = $1`,
		d.sourceAccountID,
	).Scan(&stats.TotalTokensIssued)
	if err != nil {
		return nil, fmt.Errorf("count issued tokens: %w", err)
	}

	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tokens_issued WHERE source_account_id = $1 AND revoked = false AND expires_at > NOW()`,
		d.sourceAccountID,
	).Scan(&stats.ActiveTokens)
	if err != nil {
		return nil, fmt.Errorf("count active tokens: %w", err)
	}

	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tokens_issued WHERE source_account_id = $1 AND revoked = true`,
		d.sourceAccountID,
	).Scan(&stats.RevokedTokens)
	if err != nil {
		return nil, fmt.Errorf("count revoked tokens: %w", err)
	}

	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tokens_issued WHERE source_account_id = $1 AND revoked = false AND expires_at <= NOW()`,
		d.sourceAccountID,
	).Scan(&stats.ExpiredTokens)
	if err != nil {
		return nil, fmt.Errorf("count expired tokens: %w", err)
	}

	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tokens_encryption_keys WHERE source_account_id = $1`,
		d.sourceAccountID,
	).Scan(&stats.TotalEncryptionKeys)
	if err != nil {
		return nil, fmt.Errorf("count encryption keys: %w", err)
	}

	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tokens_entitlements WHERE source_account_id = $1`,
		d.sourceAccountID,
	).Scan(&stats.TotalEntitlements)
	if err != nil {
		return nil, fmt.Errorf("count entitlements: %w", err)
	}

	err = d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tokens_entitlements WHERE source_account_id = $1 AND revoked = false AND (expires_at IS NULL OR expires_at > NOW())`,
		d.sourceAccountID,
	).Scan(&stats.ActiveEntitlements)
	if err != nil {
		return nil, fmt.Errorf("count active entitlements: %w", err)
	}

	return &stats, nil
}
