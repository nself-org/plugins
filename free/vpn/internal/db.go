package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool and scopes all queries to a source_account_id.
type DB struct {
	pool            *pgxpool.Pool
	sourceAccountID string
}

// NewDB creates a DB handle scoped to source_account_id "primary".
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool, sourceAccountID: "primary"}
}

// ForSourceAccount returns a new DB handle scoped to the given account,
// sharing the same underlying connection pool.
func (d *DB) ForSourceAccount(accountID string) *DB {
	return &DB{pool: d.pool, sourceAccountID: accountID}
}

// SourceAccountID returns the current scoped account.
func (d *DB) SourceAccountID() string {
	return d.sourceAccountID
}

// Pool returns the underlying pgxpool.Pool.
func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}

// ---------------------------------------------------------------------------
// Schema initialisation
// ---------------------------------------------------------------------------

// InitSchema creates all tables, indexes, and views if they do not exist.
func (d *DB) InitSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	stmts := []string{
		// pgcrypto for credential encryption
		`CREATE EXTENSION IF NOT EXISTS pgcrypto`,

		// np_vpn_providers
		`CREATE TABLE IF NOT EXISTS np_vpn_providers (
			id VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			display_name VARCHAR(255) NOT NULL,
			cli_available BOOLEAN DEFAULT FALSE,
			cli_command VARCHAR(255),
			api_available BOOLEAN DEFAULT FALSE,
			api_endpoint VARCHAR(255),
			port_forwarding_supported BOOLEAN DEFAULT FALSE,
			p2p_all_servers BOOLEAN DEFAULT FALSE,
			p2p_server_count INTEGER DEFAULT 0,
			total_servers INTEGER DEFAULT 0,
			total_countries INTEGER DEFAULT 0,
			wireguard_supported BOOLEAN DEFAULT TRUE,
			openvpn_supported BOOLEAN DEFAULT TRUE,
			kill_switch_available BOOLEAN DEFAULT TRUE,
			split_tunneling_available BOOLEAN DEFAULT FALSE,
			config JSONB DEFAULT '{}',
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_vpn_providers_account ON np_vpn_providers(source_account_id)`,

		// np_vpn_credentials
		`CREATE TABLE IF NOT EXISTS np_vpn_credentials (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			provider_id VARCHAR(255) NOT NULL REFERENCES np_vpn_providers(id) ON DELETE CASCADE,
			username VARCHAR(255),
			password_encrypted TEXT,
			api_key_encrypted TEXT,
			api_token_encrypted TEXT,
			account_number VARCHAR(255),
			private_key_encrypted TEXT,
			additional_data JSONB DEFAULT '{}',
			expires_at TIMESTAMPTZ,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(provider_id, source_account_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_vpn_credentials_account ON np_vpn_credentials(source_account_id)`,

		// np_vpn_servers
		`CREATE TABLE IF NOT EXISTS np_vpn_servers (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			provider_id VARCHAR(255) NOT NULL REFERENCES np_vpn_providers(id) ON DELETE CASCADE,
			hostname VARCHAR(255) NOT NULL,
			ip_address VARCHAR(45) NOT NULL,
			ipv6_address VARCHAR(45),
			country_code VARCHAR(2) NOT NULL,
			country_name VARCHAR(255) NOT NULL,
			city VARCHAR(255),
			region VARCHAR(255),
			latitude DECIMAL(10, 7),
			longitude DECIMAL(10, 7),
			p2p_supported BOOLEAN DEFAULT FALSE,
			port_forwarding_supported BOOLEAN DEFAULT FALSE,
			protocols TEXT[] DEFAULT '{}',
			load INTEGER,
			capacity INTEGER,
			status VARCHAR(50) DEFAULT 'online',
			features TEXT[] DEFAULT '{}',
			public_key VARCHAR(255),
			endpoint_port INTEGER,
			owned BOOLEAN DEFAULT FALSE,
			metadata JSONB DEFAULT '{}',
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			last_seen TIMESTAMPTZ DEFAULT NOW(),
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(provider_id, hostname, source_account_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_servers_provider ON np_vpn_servers(provider_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_servers_country ON np_vpn_servers(country_code)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_servers_p2p ON np_vpn_servers(p2p_supported) WHERE p2p_supported = true`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_servers_status ON np_vpn_servers(status)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_servers_load ON np_vpn_servers(load)`,
		`CREATE INDEX IF NOT EXISTS idx_np_vpn_servers_account ON np_vpn_servers(source_account_id)`,

		// np_vpn_connections
		`CREATE TABLE IF NOT EXISTS np_vpn_connections (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			provider_id VARCHAR(255) NOT NULL REFERENCES np_vpn_providers(id),
			server_id UUID REFERENCES np_vpn_servers(id),
			protocol VARCHAR(50) NOT NULL,
			status VARCHAR(50) NOT NULL DEFAULT 'disconnected',
			local_ip VARCHAR(45),
			vpn_ip VARCHAR(45),
			interface_name VARCHAR(50),
			dns_servers TEXT[],
			connected_at TIMESTAMPTZ,
			disconnected_at TIMESTAMPTZ,
			duration_seconds INTEGER,
			bytes_sent BIGINT DEFAULT 0,
			bytes_received BIGINT DEFAULT 0,
			error_message TEXT,
			kill_switch_enabled BOOLEAN DEFAULT TRUE,
			port_forwarded INTEGER,
			requested_by VARCHAR(255),
			metadata JSONB DEFAULT '{}',
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_connections_status ON np_vpn_connections(status)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_connections_provider ON np_vpn_connections(provider_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_connections_created ON np_vpn_connections(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_np_vpn_connections_account ON np_vpn_connections(source_account_id)`,

		// np_vpn_downloads
		`CREATE TABLE IF NOT EXISTS np_vpn_downloads (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			connection_id UUID REFERENCES np_vpn_connections(id),
			magnet_link TEXT NOT NULL,
			info_hash VARCHAR(40) NOT NULL,
			name VARCHAR(512),
			destination_path TEXT NOT NULL,
			status VARCHAR(50) NOT NULL DEFAULT 'queued',
			progress DECIMAL(5, 2) DEFAULT 0,
			bytes_downloaded BIGINT DEFAULT 0,
			bytes_total BIGINT,
			download_speed BIGINT DEFAULT 0,
			upload_speed BIGINT DEFAULT 0,
			peers INTEGER DEFAULT 0,
			seeds INTEGER DEFAULT 0,
			eta_seconds INTEGER,
			requested_by VARCHAR(255) NOT NULL,
			provider_id VARCHAR(255) NOT NULL REFERENCES np_vpn_providers(id),
			server_id UUID REFERENCES np_vpn_servers(id),
			started_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ,
			error_message TEXT,
			metadata JSONB DEFAULT '{}',
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_downloads_status ON np_vpn_downloads(status)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_downloads_info_hash ON np_vpn_downloads(info_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_downloads_created ON np_vpn_downloads(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_np_vpn_downloads_account ON np_vpn_downloads(source_account_id)`,

		// np_vpn_connection_logs
		`CREATE TABLE IF NOT EXISTS np_vpn_connection_logs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			connection_id UUID NOT NULL REFERENCES np_vpn_connections(id) ON DELETE CASCADE,
			timestamp TIMESTAMPTZ DEFAULT NOW(),
			event_type VARCHAR(50) NOT NULL,
			message TEXT NOT NULL,
			details JSONB DEFAULT '{}',
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_connection_logs_connection ON np_vpn_connection_logs(connection_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_connection_logs_timestamp ON np_vpn_connection_logs(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_np_vpn_connection_logs_account ON np_vpn_connection_logs(source_account_id)`,

		// np_vpn_server_performance
		`CREATE TABLE IF NOT EXISTS np_vpn_server_performance (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			server_id UUID NOT NULL REFERENCES np_vpn_servers(id) ON DELETE CASCADE,
			timestamp TIMESTAMPTZ DEFAULT NOW(),
			ping_ms INTEGER,
			download_speed_mbps DECIMAL(10, 2),
			upload_speed_mbps DECIMAL(10, 2),
			load_percentage INTEGER,
			success_rate DECIMAL(5, 4),
			avg_connection_time_ms INTEGER,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_server_performance_server ON np_vpn_server_performance(server_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_server_performance_timestamp ON np_vpn_server_performance(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_np_vpn_server_performance_account ON np_vpn_server_performance(source_account_id)`,

		// np_vpn_leak_tests
		`CREATE TABLE IF NOT EXISTS np_vpn_leak_tests (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			connection_id UUID NOT NULL REFERENCES np_vpn_connections(id) ON DELETE CASCADE,
			test_type VARCHAR(50) NOT NULL,
			passed BOOLEAN NOT NULL,
			expected_value VARCHAR(255),
			actual_value VARCHAR(255),
			details JSONB DEFAULT '{}',
			tested_at TIMESTAMPTZ DEFAULT NOW(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_leak_tests_connection ON np_vpn_leak_tests(connection_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vpn_leak_tests_passed ON np_vpn_leak_tests(passed)`,
		`CREATE INDEX IF NOT EXISTS idx_np_vpn_leak_tests_account ON np_vpn_leak_tests(source_account_id)`,

		// Views
		`CREATE OR REPLACE VIEW vpn_active_connections AS
		SELECT
			c.id,
			c.provider_id,
			c.source_account_id,
			p.display_name AS provider_name,
			s.hostname AS server,
			s.country_code,
			s.city,
			c.vpn_ip,
			c.interface_name,
			c.protocol,
			c.connected_at,
			EXTRACT(EPOCH FROM (NOW() - c.connected_at))::INTEGER AS uptime_seconds,
			c.bytes_sent,
			c.bytes_received,
			c.port_forwarded,
			c.kill_switch_enabled
		FROM np_vpn_connections c
		JOIN np_vpn_providers p ON c.provider_id = p.id AND c.source_account_id = p.source_account_id
		LEFT JOIN np_vpn_servers s ON c.server_id = s.id AND c.source_account_id = s.source_account_id
		WHERE c.status = 'connected'
		ORDER BY c.connected_at DESC`,

		`CREATE OR REPLACE VIEW vpn_server_stats AS
		SELECT
			s.id,
			s.provider_id,
			s.source_account_id,
			s.hostname,
			s.country_code,
			s.city,
			s.p2p_supported,
			s.port_forwarding_supported,
			s.load,
			s.status,
			COUNT(DISTINCT c.id) AS total_connections,
			AVG(sp.download_speed_mbps) AS avg_download_speed,
			AVG(sp.ping_ms) AS avg_ping,
			AVG(sp.success_rate) AS avg_success_rate,
			MAX(c.connected_at) AS last_used
		FROM np_vpn_servers s
		LEFT JOIN np_vpn_connections c ON s.id = c.server_id AND s.source_account_id = c.source_account_id
		LEFT JOIN np_vpn_server_performance sp ON s.id = sp.server_id AND s.source_account_id = sp.source_account_id
		GROUP BY s.id
		ORDER BY total_connections DESC, avg_download_speed DESC`,

		`CREATE OR REPLACE VIEW vpn_download_history AS
		SELECT
			d.id,
			d.name,
			d.info_hash,
			d.status,
			d.progress,
			d.bytes_downloaded,
			d.bytes_total,
			d.requested_by,
			d.source_account_id,
			p.display_name AS provider_name,
			s.hostname AS server,
			s.country_code,
			d.started_at,
			d.completed_at,
			EXTRACT(EPOCH FROM (COALESCE(d.completed_at, NOW()) - d.started_at))::INTEGER AS duration_seconds,
			d.created_at
		FROM np_vpn_downloads d
		JOIN np_vpn_providers p ON d.provider_id = p.id AND d.source_account_id = p.source_account_id
		LEFT JOIN np_vpn_servers s ON d.server_id = s.id AND d.source_account_id = s.source_account_id
		ORDER BY d.created_at DESC`,

		`CREATE OR REPLACE VIEW vpn_provider_uptime AS
		SELECT
			p.id,
			p.source_account_id,
			p.display_name,
			COUNT(DISTINCT c.id) AS total_connections,
			SUM(CASE WHEN c.status = 'connected' THEN 1 ELSE 0 END) AS active_connections,
			SUM(c.duration_seconds) AS total_uptime_seconds,
			AVG(c.duration_seconds) AS avg_session_duration_seconds,
			ROUND((COUNT(CASE WHEN c.error_message IS NULL THEN 1 END)::NUMERIC /
				   NULLIF(COUNT(c.id), 0) * 100), 2) AS success_rate_percent
		FROM np_vpn_providers p
		LEFT JOIN np_vpn_connections c ON p.id = c.provider_id AND p.source_account_id = c.source_account_id
		GROUP BY p.id
		ORDER BY total_connections DESC`,
	}

	for _, stmt := range stmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:minLen(len(stmt), 60)], err)
		}
	}

	return tx.Commit(ctx)
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// Provider operations
// ---------------------------------------------------------------------------

// GetAllProviders returns all providers scoped to the current account.
func (d *DB) GetAllProviders(ctx context.Context) ([]Provider, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, name, display_name, cli_available, cli_command, api_available, api_endpoint,
			port_forwarding_supported, p2p_all_servers, p2p_server_count, total_servers,
			total_countries, wireguard_supported, openvpn_supported, kill_switch_available,
			split_tunneling_available, config, source_account_id, created_at, updated_at
		FROM np_vpn_providers WHERE source_account_id = $1 ORDER BY name`,
		d.sourceAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Provider
	for rows.Next() {
		var p Provider
		var configJSON []byte
		if err := rows.Scan(
			&p.ID, &p.Name, &p.DisplayName, &p.CLIAvailable, &p.CLICommand,
			&p.APIAvailable, &p.APIEndpoint, &p.PortForwardingSupported,
			&p.P2PAllServers, &p.P2PServerCount, &p.TotalServers, &p.TotalCountries,
			&p.WireguardSupported, &p.OpenVPNSupported, &p.KillSwitchAvailable,
			&p.SplitTunnelingAvailable, &configJSON, &p.SourceAccountID,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		p.Config = make(map[string]interface{})
		if len(configJSON) > 0 {
			_ = json.Unmarshal(configJSON, &p.Config)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// GetProvider returns a single provider by ID, scoped to the current account.
func (d *DB) GetProvider(ctx context.Context, id string) (*Provider, error) {
	var p Provider
	var configJSON []byte
	err := d.pool.QueryRow(ctx,
		`SELECT id, name, display_name, cli_available, cli_command, api_available, api_endpoint,
			port_forwarding_supported, p2p_all_servers, p2p_server_count, total_servers,
			total_countries, wireguard_supported, openvpn_supported, kill_switch_available,
			split_tunneling_available, config, source_account_id, created_at, updated_at
		FROM np_vpn_providers WHERE id = $1 AND source_account_id = $2`,
		id, d.sourceAccountID,
	).Scan(
		&p.ID, &p.Name, &p.DisplayName, &p.CLIAvailable, &p.CLICommand,
		&p.APIAvailable, &p.APIEndpoint, &p.PortForwardingSupported,
		&p.P2PAllServers, &p.P2PServerCount, &p.TotalServers, &p.TotalCountries,
		&p.WireguardSupported, &p.OpenVPNSupported, &p.KillSwitchAvailable,
		&p.SplitTunnelingAvailable, &configJSON, &p.SourceAccountID,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.Config = make(map[string]interface{})
	if len(configJSON) > 0 {
		_ = json.Unmarshal(configJSON, &p.Config)
	}
	return &p, nil
}

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

// ---------------------------------------------------------------------------
// Server operations
// ---------------------------------------------------------------------------

// GetServers returns servers matching the given filters, scoped to the current account.
func (d *DB) GetServers(ctx context.Context, f ServerFilter) ([]Server, error) {
	conditions := []string{"source_account_id = $1"}
	args := []interface{}{d.sourceAccountID}
	argIdx := 2

	if f.Provider != "" {
		conditions = append(conditions, fmt.Sprintf("provider_id = $%d", argIdx))
		args = append(args, f.Provider)
		argIdx++
	}
	if f.Country != "" {
		conditions = append(conditions, fmt.Sprintf("country_code = $%d", argIdx))
		args = append(args, f.Country)
		argIdx++
	}
	if f.P2POnly {
		conditions = append(conditions, "p2p_supported = true")
	}
	if f.PortForwarding {
		conditions = append(conditions, "port_forwarding_supported = true")
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}

	query := fmt.Sprintf(`
		SELECT id, provider_id, hostname, ip_address, ipv6_address, country_code, country_name,
			city, region, latitude, longitude, p2p_supported, port_forwarding_supported,
			protocols, load, capacity, status, features, public_key, endpoint_port,
			owned, metadata, source_account_id, last_seen, created_at, updated_at
		FROM np_vpn_servers
		WHERE %s
		ORDER BY load ASC NULLS LAST, created_at DESC
		LIMIT $%d`,
		joinAnd(conditions), argIdx)
	args = append(args, limit)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Server
	for rows.Next() {
		s, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func scanServer(rows pgx.Rows) (Server, error) {
	var s Server
	var metaJSON []byte
	err := rows.Scan(
		&s.ID, &s.ProviderID, &s.Hostname, &s.IPAddress, &s.IPv6Address,
		&s.CountryCode, &s.CountryName, &s.City, &s.Region,
		&s.Latitude, &s.Longitude, &s.P2PSupported, &s.PortForwardingSupported,
		&s.Protocols, &s.Load, &s.Capacity, &s.Status, &s.Features,
		&s.PublicKey, &s.EndpointPort, &s.Owned, &metaJSON,
		&s.SourceAccountID, &s.LastSeen, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return Server{}, err
	}
	s.Metadata = make(map[string]interface{})
	if len(metaJSON) > 0 {
		_ = json.Unmarshal(metaJSON, &s.Metadata)
	}
	if s.Protocols == nil {
		s.Protocols = []string{}
	}
	if s.Features == nil {
		s.Features = []string{}
	}
	return s, nil
}

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

// ---------------------------------------------------------------------------
// Download operations
// ---------------------------------------------------------------------------

// CreateDownload inserts a new download record.
func (d *DB) CreateDownload(ctx context.Context, dl *Download) error {
	metaJSON, _ := json.Marshal(dl.Metadata)
	return d.pool.QueryRow(ctx,
		`INSERT INTO np_vpn_downloads (
			connection_id, magnet_link, info_hash, name, destination_path, status, progress,
			bytes_downloaded, bytes_total, requested_by, provider_id, server_id, metadata, source_account_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, created_at`,
		dl.ConnectionID, dl.MagnetLink, dl.InfoHash, dl.Name, dl.DestinationPath,
		coalesceStr(dl.Status, "queued"), dl.Progress, dl.BytesDownloaded, dl.BytesTotal,
		dl.RequestedBy, dl.ProviderID, dl.ServerID, metaJSON, d.sourceAccountID,
	).Scan(&dl.ID, &dl.CreatedAt)
}

// GetDownload returns a single download by ID.
func (d *DB) GetDownload(ctx context.Context, id string) (*Download, error) {
	var dl Download
	var metaJSON []byte
	err := d.pool.QueryRow(ctx,
		`SELECT id, connection_id, magnet_link, info_hash, name, destination_path, status,
			progress, bytes_downloaded, bytes_total, download_speed, upload_speed, peers, seeds,
			eta_seconds, requested_by, provider_id, server_id, started_at, completed_at,
			error_message, metadata, source_account_id, created_at
		FROM np_vpn_downloads WHERE id = $1 AND source_account_id = $2`,
		id, d.sourceAccountID,
	).Scan(
		&dl.ID, &dl.ConnectionID, &dl.MagnetLink, &dl.InfoHash, &dl.Name,
		&dl.DestinationPath, &dl.Status, &dl.Progress, &dl.BytesDownloaded,
		&dl.BytesTotal, &dl.DownloadSpeed, &dl.UploadSpeed, &dl.Peers, &dl.Seeds,
		&dl.ETASeconds, &dl.RequestedBy, &dl.ProviderID, &dl.ServerID,
		&dl.StartedAt, &dl.CompletedAt, &dl.ErrorMessage, &metaJSON,
		&dl.SourceAccountID, &dl.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dl.Metadata = make(map[string]interface{})
	if len(metaJSON) > 0 {
		_ = json.Unmarshal(metaJSON, &dl.Metadata)
	}
	return &dl, nil
}

// GetAllDownloads returns downloads scoped to the current account.
func (d *DB) GetAllDownloads(ctx context.Context, limit int) ([]Download, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.pool.Query(ctx,
		`SELECT id, connection_id, magnet_link, info_hash, name, destination_path, status,
			progress, bytes_downloaded, bytes_total, download_speed, upload_speed, peers, seeds,
			eta_seconds, requested_by, provider_id, server_id, started_at, completed_at,
			error_message, metadata, source_account_id, created_at
		FROM np_vpn_downloads WHERE source_account_id = $1 ORDER BY created_at DESC LIMIT $2`,
		d.sourceAccountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Download
	for rows.Next() {
		var dl Download
		var metaJSON []byte
		if err := rows.Scan(
			&dl.ID, &dl.ConnectionID, &dl.MagnetLink, &dl.InfoHash, &dl.Name,
			&dl.DestinationPath, &dl.Status, &dl.Progress, &dl.BytesDownloaded,
			&dl.BytesTotal, &dl.DownloadSpeed, &dl.UploadSpeed, &dl.Peers, &dl.Seeds,
			&dl.ETASeconds, &dl.RequestedBy, &dl.ProviderID, &dl.ServerID,
			&dl.StartedAt, &dl.CompletedAt, &dl.ErrorMessage, &metaJSON,
			&dl.SourceAccountID, &dl.CreatedAt,
		); err != nil {
			return nil, err
		}
		dl.Metadata = make(map[string]interface{})
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &dl.Metadata)
		}
		result = append(result, dl)
	}
	return result, rows.Err()
}

// UpdateDownloadStatus sets the status and optional error message on a download.
func (d *DB) UpdateDownloadStatus(ctx context.Context, id, status string, errMsg *string) error {
	_, err := d.pool.Exec(ctx,
		`UPDATE np_vpn_downloads SET status = $1, error_message = $2
		WHERE id = $3 AND source_account_id = $4`,
		status, errMsg, id, d.sourceAccountID)
	return err
}

// ---------------------------------------------------------------------------
// Leak test operations
// ---------------------------------------------------------------------------

// GetLatestLeakTest returns the most recent leak test for a connection.
func (d *DB) GetLatestLeakTest(ctx context.Context, connectionID string) (*LeakTest, error) {
	var lt LeakTest
	var detailsJSON []byte
	err := d.pool.QueryRow(ctx,
		`SELECT id, connection_id, test_type, passed, expected_value, actual_value,
			details, tested_at, source_account_id
		FROM np_vpn_leak_tests
		WHERE connection_id = $1 AND source_account_id = $2
		ORDER BY tested_at DESC LIMIT 1`,
		connectionID, d.sourceAccountID,
	).Scan(
		&lt.ID, &lt.ConnectionID, &lt.TestType, &lt.Passed,
		&lt.ExpectedValue, &lt.ActualValue, &detailsJSON,
		&lt.TestedAt, &lt.SourceAccountID,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lt.Details = make(map[string]interface{})
	if len(detailsJSON) > 0 {
		_ = json.Unmarshal(detailsJSON, &lt.Details)
	}
	return &lt, nil
}

// InsertLeakTest stores a leak test result.
func (d *DB) InsertLeakTest(ctx context.Context, connectionID, testType string, passed bool, expected, actual string, details map[string]interface{}) error {
	detailsJSON, _ := json.Marshal(details)
	_, err := d.pool.Exec(ctx,
		`INSERT INTO np_vpn_leak_tests (connection_id, test_type, passed, expected_value, actual_value, details, source_account_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		connectionID, testType, passed, expected, actual, detailsJSON, d.sourceAccountID)
	return err
}

// ---------------------------------------------------------------------------
// Statistics
// ---------------------------------------------------------------------------

// GetStatistics returns aggregate usage statistics.
func (d *DB) GetStatistics(ctx context.Context) (*Statistics, error) {
	stats := &Statistics{
		Providers:  []ProviderStat{},
		TopServers: []ServerStat{},
	}

	// Connections summary
	var totalConn, activeConn int
	err := d.pool.QueryRow(ctx,
		`SELECT
			COUNT(*),
			COUNT(CASE WHEN status = 'connected' THEN 1 END)
		FROM np_vpn_connections
		WHERE source_account_id = $1`,
		d.sourceAccountID).Scan(&totalConn, &activeConn)
	if err != nil {
		return nil, err
	}
	stats.TotalConnections = totalConn
	stats.ActiveConnections = activeConn

	// Downloads summary
	var totalDL, activeDL int
	var totalBytes *int64
	err = d.pool.QueryRow(ctx,
		`SELECT
			COUNT(*),
			COUNT(CASE WHEN status IN ('downloading', 'queued', 'connecting_vpn') THEN 1 END),
			SUM(bytes_downloaded)
		FROM np_vpn_downloads
		WHERE source_account_id = $1`,
		d.sourceAccountID).Scan(&totalDL, &activeDL, &totalBytes)
	if err != nil {
		return nil, err
	}
	stats.TotalDownloads = totalDL
	stats.ActiveDownloads = activeDL
	if totalBytes != nil {
		stats.TotalBytesDownloaded = strconv.FormatInt(*totalBytes, 10)
	} else {
		stats.TotalBytesDownloaded = "0"
	}

	// Provider stats
	provRows, err := d.pool.Query(ctx,
		`SELECT p.display_name, COUNT(c.id) AS total_connections,
			ROUND((COUNT(CASE WHEN c.error_message IS NULL THEN 1 END)::NUMERIC /
				   NULLIF(COUNT(c.id), 0) * 100), 2) AS success_rate_percent
		FROM np_vpn_providers p
		LEFT JOIN np_vpn_connections c ON p.id = c.provider_id AND c.source_account_id = $1
		WHERE p.source_account_id = $1
		GROUP BY p.id, p.display_name
		ORDER BY COUNT(c.id) DESC
		LIMIT 10`,
		d.sourceAccountID)
	if err != nil {
		return nil, err
	}
	defer provRows.Close()

	for provRows.Next() {
		var ps ProviderStat
		var successRate *float64
		if err := provRows.Scan(&ps.Provider, &ps.Connections, &successRate); err != nil {
			return nil, err
		}
		if successRate != nil {
			ps.UptimePercentage = *successRate
		}
		stats.Providers = append(stats.Providers, ps)
	}
	if err := provRows.Err(); err != nil {
		return nil, err
	}

	// Top servers
	srvRows, err := d.pool.Query(ctx,
		`SELECT s.hostname, s.provider_id, s.country_code,
			COUNT(DISTINCT c.id) AS total_connections,
			AVG(sp.download_speed_mbps) AS avg_download_speed
		FROM np_vpn_servers s
		LEFT JOIN np_vpn_connections c ON s.id = c.server_id AND c.source_account_id = $1
		LEFT JOIN np_vpn_server_performance sp ON s.id = sp.server_id AND sp.source_account_id = $1
		WHERE s.source_account_id = $1
		GROUP BY s.id, s.hostname, s.provider_id, s.country_code
		ORDER BY COUNT(DISTINCT c.id) DESC, AVG(sp.download_speed_mbps) DESC
		LIMIT 10`,
		d.sourceAccountID)
	if err != nil {
		return nil, err
	}
	defer srvRows.Close()

	for srvRows.Next() {
		var ss ServerStat
		var avgSpeed *float64
		if err := srvRows.Scan(&ss.Server, &ss.Provider, &ss.Country, &ss.Connections, &avgSpeed); err != nil {
			return nil, err
		}
		if avgSpeed != nil {
			ss.AvgSpeedMbps = *avgSpeed
		}
		stats.TopServers = append(stats.TopServers, ss)
	}
	if err := srvRows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func joinAnd(parts []string) string {
	if len(parts) == 0 {
		return "TRUE"
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += " AND " + p
	}
	return result
}

func coalesceStr(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}
