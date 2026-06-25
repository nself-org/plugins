package internal

import (
	"context"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Schema initialisation
// ---------------------------------------------------------------------------

// InitSchema creates all tables, indexes, and views if they do not exist.
// Size-cap exception: SQL DDL migration — 299L of linear SQL statements; splitting across files adds no value and breaks transactional migration semantics.
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

