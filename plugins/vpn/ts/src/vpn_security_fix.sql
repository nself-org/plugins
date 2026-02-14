-- VPN Multi-App Security Fix
-- Add source_account_id to all 8 VPN tables

ALTER TABLE np_vpn_providers ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE np_vpn_credentials ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE np_vpn_servers ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE np_vpn_connections ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE np_vpn_downloads ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE np_vpn_connection_logs ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE np_vpn_server_performance ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE np_vpn_leak_tests ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';

-- Add indexes
CREATE INDEX IF NOT EXISTS idx_np_vpn_providers_account ON np_vpn_providers(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_vpn_credentials_account ON np_vpn_credentials(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_vpn_servers_account ON np_vpn_servers(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_vpn_connections_account ON np_vpn_connections(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_vpn_downloads_account ON np_vpn_downloads(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_vpn_connection_logs_account ON np_vpn_connection_logs(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_vpn_server_performance_account ON np_vpn_server_performance(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_vpn_leak_tests_account ON np_vpn_leak_tests(source_account_id);
