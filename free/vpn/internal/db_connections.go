package internal

import (
	"context"
	"encoding/json"
	"time"
	pgx "github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Connection operations
// ---------------------------------------------------------------------------

// GetActiveConnection returns the most recent connected connection, or nil.
func (d *DB) GetActiveConnection(ctx context.Context) (*Connection, error) {
	var c Connection
	var metaJSON []byte
	err := d.pool.QueryRow(ctx,
		`SELECT id, provider_id, server_id, protocol, status, local_ip, vpn_ip,
			interface_name, dns_servers, connected_at, disconnected_at, duration_seconds,
			bytes_sent, bytes_received, error_message, kill_switch_enabled, port_forwarded,
			requested_by, metadata, source_account_id, created_at
		FROM np_vpn_connections
		WHERE status = 'connected' AND source_account_id = $1
		ORDER BY connected_at DESC LIMIT 1`,
		d.sourceAccountID,
	).Scan(
		&c.ID, &c.ProviderID, &c.ServerID, &c.Protocol, &c.Status,
		&c.LocalIP, &c.VPNIP, &c.InterfaceName, &c.DNSServers,
		&c.ConnectedAt, &c.DisconnectedAt, &c.DurationSeconds,
		&c.BytesSent, &c.BytesReceived, &c.ErrorMessage,
		&c.KillSwitchEnabled, &c.PortForwarded, &c.RequestedBy,
		&metaJSON, &c.SourceAccountID, &c.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Metadata = make(map[string]interface{})
	if len(metaJSON) > 0 {
		_ = json.Unmarshal(metaJSON, &c.Metadata)
	}
	if c.DNSServers == nil {
		c.DNSServers = []string{}
	}
	return &c, nil
}

// CreateConnection inserts a new connection record.
func (d *DB) CreateConnection(ctx context.Context, c *Connection) error {
	metaJSON, _ := json.Marshal(c.Metadata)
	return d.pool.QueryRow(ctx,
		`INSERT INTO np_vpn_connections (
			provider_id, server_id, protocol, status, local_ip, vpn_ip, interface_name,
			dns_servers, connected_at, kill_switch_enabled, requested_by, metadata, source_account_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at`,
		c.ProviderID, c.ServerID, c.Protocol,
		coalesceStr(c.Status, "connecting"), c.LocalIP, c.VPNIP, c.InterfaceName,
		c.DNSServers, c.ConnectedAt, c.KillSwitchEnabled, c.RequestedBy,
		metaJSON, d.sourceAccountID,
	).Scan(&c.ID, &c.CreatedAt)
}

// UpdateConnectionStatus sets the status and optional disconnect fields.
func (d *DB) UpdateConnectionStatus(ctx context.Context, id, status string, disconnectedAt *time.Time, durationSec *int) error {
	_, err := d.pool.Exec(ctx,
		`UPDATE np_vpn_connections SET status = $1, disconnected_at = $2, duration_seconds = $3
		WHERE id = $4 AND source_account_id = $5`,
		status, disconnectedAt, durationSec, id, d.sourceAccountID)
	return err
}

