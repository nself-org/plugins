package internal

import (
	"context"
)

// ---------------------------------------------------------------------------
// Credential operations (pgcrypto encrypted)
// ---------------------------------------------------------------------------

// UpsertCredentials stores or updates encrypted credentials for a provider.
func (d *DB) UpsertCredentials(ctx context.Context, providerID string, username, password, apiToken, accountNumber, apiKey, encryptionKey string) error {
	_, err := d.pool.Exec(ctx,
		`INSERT INTO np_vpn_credentials (
			provider_id, username, password_encrypted, api_key_encrypted, api_token_encrypted,
			account_number, source_account_id
		) VALUES (
			$1, $2,
			pgp_sym_encrypt($3::text, $7),
			pgp_sym_encrypt($4::text, $7),
			pgp_sym_encrypt($5::text, $7),
			$6, $8
		)
		ON CONFLICT (provider_id, source_account_id) DO UPDATE SET
			username = EXCLUDED.username,
			password_encrypted = EXCLUDED.password_encrypted,
			api_key_encrypted = EXCLUDED.api_key_encrypted,
			api_token_encrypted = EXCLUDED.api_token_encrypted,
			account_number = EXCLUDED.account_number,
			updated_at = NOW()`,
		providerID, username, password, apiKey, apiToken, accountNumber, encryptionKey, d.sourceAccountID)
	return err
}

// HasCredentials checks whether credentials exist for the given provider.
func (d *DB) HasCredentials(ctx context.Context, providerID, encryptionKey string) (bool, error) {
	var count int
	err := d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_vpn_credentials
		WHERE provider_id = $1 AND source_account_id = $2`,
		providerID, d.sourceAccountID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

