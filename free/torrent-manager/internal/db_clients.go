package internal

import (
	"context"
	"fmt"
	"time"
	pgx "github.com/jackc/pgx/v5"
)

// ============================================================================
// Client Operations
// ============================================================================

// ListClients returns all configured torrent clients.
func (d *DB) ListClients() ([]TorrentClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := d.pool.Query(ctx,
		`SELECT id, source_account_id, client_type, host, port, username, password_encrypted,
		        is_default, status, last_connected_at, last_error, created_at, updated_at
		 FROM np_torrentmanager_torrent_clients ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list clients: %w", err)
	}
	defer rows.Close()

	var clients []TorrentClient
	for rows.Next() {
		var c TorrentClient
		if err := rows.Scan(
			&c.ID, &c.SourceAccountID, &c.ClientType, &c.Host, &c.Port,
			&c.Username, &c.PasswordEncrypted, &c.IsDefault, &c.Status,
			&c.LastConnectedAt, &c.LastError, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan client: %w", err)
		}
		clients = append(clients, c)
	}
	return clients, rows.Err()
}

// GetDefaultClient returns the default torrent client, or nil if none.
func (d *DB) GetDefaultClient() (*TorrentClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var c TorrentClient
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, client_type, host, port, username, password_encrypted,
		        is_default, status, last_connected_at, last_error, created_at, updated_at
		 FROM np_torrentmanager_torrent_clients WHERE is_default = TRUE LIMIT 1`,
	).Scan(
		&c.ID, &c.SourceAccountID, &c.ClientType, &c.Host, &c.Port,
		&c.Username, &c.PasswordEncrypted, &c.IsDefault, &c.Status,
		&c.LastConnectedAt, &c.LastError, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get default client: %w", err)
	}
	return &c, nil
}

