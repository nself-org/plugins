package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

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

