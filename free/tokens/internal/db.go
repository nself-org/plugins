package internal

import (
	"context"
	"time"

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
