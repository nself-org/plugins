-- vpn plugin: initial schema
-- CODE WINS: 8 tables from internal/db_schema.go (np_vpn_* prefix)
-- Tables: np_vpn_providers, np_vpn_credentials, np_vpn_servers, np_vpn_connections,
--         np_vpn_downloads, np_vpn_connection_logs, np_vpn_server_performance, np_vpn_leak_tests
-- All tables already have source_account_id in Go code.
-- SSRF: api_endpoint stored but validation in Go layer (provider URL validation required at write time)
-- SECURITY: np_vpn_credentials — encrypted columns (password_encrypted, api_key_encrypted, etc.) — admin-only in Hasura

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS np_vpn_providers (
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
);
CREATE INDEX IF NOT EXISTS idx_np_vpn_providers_account ON np_vpn_providers(source_account_id);

CREATE TABLE IF NOT EXISTS np_vpn_credentials (
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
);
CREATE INDEX IF NOT EXISTS idx_np_vpn_credentials_account ON np_vpn_credentials(source_account_id);

CREATE TABLE IF NOT EXISTS np_vpn_servers (
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
);
CREATE INDEX IF NOT EXISTS idx_vpn_servers_provider ON np_vpn_servers(provider_id);
CREATE INDEX IF NOT EXISTS idx_vpn_servers_country ON np_vpn_servers(country_code);
CREATE INDEX IF NOT EXISTS idx_vpn_servers_p2p ON np_vpn_servers(p2p_supported) WHERE p2p_supported = true;
CREATE INDEX IF NOT EXISTS idx_vpn_servers_status ON np_vpn_servers(status);
CREATE INDEX IF NOT EXISTS idx_vpn_servers_load ON np_vpn_servers(load);
CREATE INDEX IF NOT EXISTS idx_np_vpn_servers_account ON np_vpn_servers(source_account_id);

CREATE TABLE IF NOT EXISTS np_vpn_connections (
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
);
CREATE INDEX IF NOT EXISTS idx_vpn_connections_status ON np_vpn_connections(status);
CREATE INDEX IF NOT EXISTS idx_vpn_connections_provider ON np_vpn_connections(provider_id);
CREATE INDEX IF NOT EXISTS idx_vpn_connections_created ON np_vpn_connections(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_np_vpn_connections_account ON np_vpn_connections(source_account_id);

CREATE TABLE IF NOT EXISTS np_vpn_downloads (
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
);
CREATE INDEX IF NOT EXISTS idx_vpn_downloads_status ON np_vpn_downloads(status);
CREATE INDEX IF NOT EXISTS idx_vpn_downloads_info_hash ON np_vpn_downloads(info_hash);
CREATE INDEX IF NOT EXISTS idx_vpn_downloads_created ON np_vpn_downloads(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_np_vpn_downloads_account ON np_vpn_downloads(source_account_id);

CREATE TABLE IF NOT EXISTS np_vpn_connection_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    connection_id UUID NOT NULL REFERENCES np_vpn_connections(id) ON DELETE CASCADE,
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    event_type VARCHAR(50) NOT NULL,
    message TEXT NOT NULL,
    details JSONB DEFAULT '{}',
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'
);
CREATE INDEX IF NOT EXISTS idx_vpn_connection_logs_connection ON np_vpn_connection_logs(connection_id);
CREATE INDEX IF NOT EXISTS idx_vpn_connection_logs_timestamp ON np_vpn_connection_logs(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_np_vpn_connection_logs_account ON np_vpn_connection_logs(source_account_id);

CREATE TABLE IF NOT EXISTS np_vpn_server_performance (
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
);
CREATE INDEX IF NOT EXISTS idx_vpn_server_performance_server ON np_vpn_server_performance(server_id);
CREATE INDEX IF NOT EXISTS idx_vpn_server_performance_timestamp ON np_vpn_server_performance(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_np_vpn_server_performance_account ON np_vpn_server_performance(source_account_id);

CREATE TABLE IF NOT EXISTS np_vpn_leak_tests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    connection_id UUID NOT NULL REFERENCES np_vpn_connections(id) ON DELETE CASCADE,
    test_type VARCHAR(50) NOT NULL,
    passed BOOLEAN NOT NULL,
    expected_value VARCHAR(255),
    actual_value VARCHAR(255),
    details JSONB DEFAULT '{}',
    tested_at TIMESTAMPTZ DEFAULT NOW(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'
);
CREATE INDEX IF NOT EXISTS idx_vpn_leak_tests_connection ON np_vpn_leak_tests(connection_id);
CREATE INDEX IF NOT EXISTS idx_vpn_leak_tests_passed ON np_vpn_leak_tests(passed);
CREATE INDEX IF NOT EXISTS idx_np_vpn_leak_tests_account ON np_vpn_leak_tests(source_account_id);
