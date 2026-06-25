package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

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

