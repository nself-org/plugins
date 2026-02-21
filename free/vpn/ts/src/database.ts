/**
 * VPN Plugin Database Operations
 */

import { Pool, QueryResult } from 'pg';
import { createLogger } from '@nself/plugin-utils';
import type {
  VPNProviderRecord,
  VPNCredentialRecord,
  VPNServerRecord,
  VPNConnectionRecord,
  VPNDownloadRecord,
  VPNConnectionLogRecord,
  VPNServerPerformanceRecord,
  VPNLeakTestRecord,
  VPNStatistics,
} from './types.js';

const logger = createLogger('vpn:database');

// Internal types for database query results
interface ProviderStatsRow {
  display_name: string;
  total_connections: string;
  success_rate_percent: string | null;
}

interface ServerStatsRow {
  hostname: string;
  provider_id: string;
  country_code: string;
  total_connections: string | null;
  avg_download_speed: string | null;
}

// Tables that belong to this plugin
const ALL_TABLES = [
  'np_vpn_providers',
  'np_vpn_credentials',
  'np_vpn_servers',
  'np_vpn_connections',
  'np_vpn_downloads',
  'np_vpn_connection_logs',
  'np_vpn_server_performance',
  'np_vpn_leak_tests',
] as const;

export class VPNDatabase {
  private pool: Pool;
  private sourceAccountId: string;

  constructor(connectionString: string, sourceAccountId = 'primary') {
    this.pool = new Pool({
      connectionString,
      max: 20,
      idleTimeoutMillis: 30000,
      connectionTimeoutMillis: 2000,
    });

    this.sourceAccountId = sourceAccountId;

    this.pool.on('error', (err) => {
      logger.error('Unexpected database error', { error: err instanceof Error ? err.message : String(err) });
    });
  }

  /** Return a new VPNDatabase handle scoped to a different source account, sharing the same pool. */
  forSourceAccount(accountId: string): VPNDatabase {
    const scoped = Object.create(VPNDatabase.prototype) as VPNDatabase;
    scoped.pool = this.pool;
    scoped.sourceAccountId = accountId;
    return scoped;
  }

  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  /**
   * Execute a raw query against the pool (for ad-hoc queries not covered by typed methods)
   */
  async query<T extends Record<string, unknown> = Record<string, unknown>>(text: string, values?: unknown[]): Promise<QueryResult<T>> {
    return this.pool.query<T>(text, values);
  }

  /**
   * Initialize database schema
   */
  async initializeSchema(): Promise<void> {
    logger.info('Initializing database schema');

    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');

      // Enable pgcrypto for encryption
      await client.query(`CREATE EXTENSION IF NOT EXISTS pgcrypto`);

      // VPN Providers table
      await client.query(`
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
          created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
        )
      `);

      // Index for source_account_id
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_np_vpn_providers_account ON np_vpn_providers(source_account_id)
      `);

      // VPN Credentials table (encrypted)
      await client.query(`
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
          expires_at TIMESTAMP WITH TIME ZONE,
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          UNIQUE(provider_id, source_account_id)
        )
      `);

      // Index for source_account_id
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_np_vpn_credentials_account ON np_vpn_credentials(source_account_id)
      `);

      // VPN Servers table
      await client.query(`
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
          last_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          UNIQUE(provider_id, hostname, source_account_id)
        )
      `);

      // Create indexes for servers
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_vpn_servers_provider ON np_vpn_servers(provider_id);
        CREATE INDEX IF NOT EXISTS idx_vpn_servers_country ON np_vpn_servers(country_code);
        CREATE INDEX IF NOT EXISTS idx_vpn_servers_p2p ON np_vpn_servers(p2p_supported) WHERE p2p_supported = true;
        CREATE INDEX IF NOT EXISTS idx_vpn_servers_status ON np_vpn_servers(status);
        CREATE INDEX IF NOT EXISTS idx_vpn_servers_load ON np_vpn_servers(load);
        CREATE INDEX IF NOT EXISTS idx_np_vpn_servers_account ON np_vpn_servers(source_account_id);
      `);

      // VPN Connections table
      await client.query(`
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
          connected_at TIMESTAMP WITH TIME ZONE,
          disconnected_at TIMESTAMP WITH TIME ZONE,
          duration_seconds INTEGER,
          bytes_sent BIGINT DEFAULT 0,
          bytes_received BIGINT DEFAULT 0,
          error_message TEXT,
          kill_switch_enabled BOOLEAN DEFAULT TRUE,
          port_forwarded INTEGER,
          requested_by VARCHAR(255),
          metadata JSONB DEFAULT '{}',
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
        )
      `);

      // Create indexes for connections
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_vpn_connections_status ON np_vpn_connections(status);
        CREATE INDEX IF NOT EXISTS idx_vpn_connections_provider ON np_vpn_connections(provider_id);
        CREATE INDEX IF NOT EXISTS idx_vpn_connections_created ON np_vpn_connections(created_at DESC);
        CREATE INDEX IF NOT EXISTS idx_np_vpn_connections_account ON np_vpn_connections(source_account_id);
      `);

      // VPN Downloads table
      await client.query(`
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
          started_at TIMESTAMP WITH TIME ZONE,
          completed_at TIMESTAMP WITH TIME ZONE,
          error_message TEXT,
          metadata JSONB DEFAULT '{}',
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
          created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
        )
      `);

      // Create indexes for downloads
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_vpn_downloads_status ON np_vpn_downloads(status);
        CREATE INDEX IF NOT EXISTS idx_vpn_downloads_info_hash ON np_vpn_downloads(info_hash);
        CREATE INDEX IF NOT EXISTS idx_vpn_downloads_created ON np_vpn_downloads(created_at DESC);
        CREATE INDEX IF NOT EXISTS idx_np_vpn_downloads_account ON np_vpn_downloads(source_account_id);
      `);

      // VPN Connection Logs table
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_vpn_connection_logs (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          connection_id UUID NOT NULL REFERENCES np_vpn_connections(id) ON DELETE CASCADE,
          timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          event_type VARCHAR(50) NOT NULL,
          message TEXT NOT NULL,
          details JSONB DEFAULT '{}',
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'
        )
      `);

      // Create index for logs
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_vpn_connection_logs_connection ON np_vpn_connection_logs(connection_id);
        CREATE INDEX IF NOT EXISTS idx_vpn_connection_logs_timestamp ON np_vpn_connection_logs(timestamp DESC);
        CREATE INDEX IF NOT EXISTS idx_np_vpn_connection_logs_account ON np_vpn_connection_logs(source_account_id);
      `);

      // VPN Server Performance table
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_vpn_server_performance (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          server_id UUID NOT NULL REFERENCES np_vpn_servers(id) ON DELETE CASCADE,
          timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          ping_ms INTEGER,
          download_speed_mbps DECIMAL(10, 2),
          upload_speed_mbps DECIMAL(10, 2),
          load_percentage INTEGER,
          success_rate DECIMAL(5, 4),
          avg_connection_time_ms INTEGER,
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'
        )
      `);

      // Create indexes for performance
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_vpn_server_performance_server ON np_vpn_server_performance(server_id);
        CREATE INDEX IF NOT EXISTS idx_vpn_server_performance_timestamp ON np_vpn_server_performance(timestamp DESC);
        CREATE INDEX IF NOT EXISTS idx_np_vpn_server_performance_account ON np_vpn_server_performance(source_account_id);
      `);

      // VPN Leak Tests table
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_vpn_leak_tests (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          connection_id UUID NOT NULL REFERENCES np_vpn_connections(id) ON DELETE CASCADE,
          test_type VARCHAR(50) NOT NULL,
          passed BOOLEAN NOT NULL,
          expected_value VARCHAR(255),
          actual_value VARCHAR(255),
          details JSONB DEFAULT '{}',
          tested_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'
        )
      `);

      // Create index for leak tests
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_vpn_leak_tests_connection ON np_vpn_leak_tests(connection_id);
        CREATE INDEX IF NOT EXISTS idx_vpn_leak_tests_passed ON np_vpn_leak_tests(passed);
        CREATE INDEX IF NOT EXISTS idx_np_vpn_leak_tests_account ON np_vpn_leak_tests(source_account_id);
      `);

      // Create analytics views
      await client.query(`
        CREATE OR REPLACE VIEW vpn_active_connections AS
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
        ORDER BY c.connected_at DESC
      `);

      await client.query(`
        CREATE OR REPLACE VIEW vpn_server_stats AS
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
        ORDER BY total_connections DESC, avg_download_speed DESC
      `);

      await client.query(`
        CREATE OR REPLACE VIEW vpn_download_history AS
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
        ORDER BY d.created_at DESC
      `);

      await client.query(`
        CREATE OR REPLACE VIEW vpn_provider_uptime AS
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
        ORDER BY total_connections DESC
      `);

      await client.query('COMMIT');
      logger.info('Database schema initialized successfully');
    } catch (error) {
      await client.query('ROLLBACK');
      logger.error('Failed to initialize database schema', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    } finally {
      client.release();
    }
  }

  // ============================================================================
  // Migration: add source_account_id to existing tables
  // ============================================================================

  async migrateMultiApp(): Promise<void> {
    for (const table of ALL_TABLES) {
      const colCheck = await this.pool.query(
        `SELECT 1 FROM information_schema.columns
         WHERE table_schema = 'public'
           AND table_name = $1
           AND column_name = 'source_account_id'`,
        [table],
      );

      if (colCheck.rowCount === 0) {
        logger.info(`Adding source_account_id to ${table}`);
        await this.pool.query(
          `ALTER TABLE ${table} ADD COLUMN source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'`,
        );
        await this.pool.query(
          `CREATE INDEX IF NOT EXISTS idx_${table}_account ON ${table}(source_account_id)`,
        );
      }
    }
  }

  // ============================================================================
  // Provider Operations
  // ============================================================================

  async upsertProvider(provider: Partial<VPNProviderRecord>): Promise<VPNProviderRecord> {
    const result = await this.pool.query<VPNProviderRecord>(
      `INSERT INTO np_vpn_providers (
        id, name, display_name, cli_available, cli_command, api_available, api_endpoint,
        port_forwarding_supported, p2p_all_servers, p2p_server_count, total_servers,
        total_countries, wireguard_supported, openvpn_supported, kill_switch_available,
        split_tunneling_available, config, source_account_id
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        display_name = EXCLUDED.display_name,
        cli_available = EXCLUDED.cli_available,
        cli_command = EXCLUDED.cli_command,
        api_available = EXCLUDED.api_available,
        api_endpoint = EXCLUDED.api_endpoint,
        port_forwarding_supported = EXCLUDED.port_forwarding_supported,
        p2p_all_servers = EXCLUDED.p2p_all_servers,
        p2p_server_count = EXCLUDED.p2p_server_count,
        total_servers = EXCLUDED.total_servers,
        total_countries = EXCLUDED.total_countries,
        wireguard_supported = EXCLUDED.wireguard_supported,
        openvpn_supported = EXCLUDED.openvpn_supported,
        kill_switch_available = EXCLUDED.kill_switch_available,
        split_tunneling_available = EXCLUDED.split_tunneling_available,
        config = EXCLUDED.config,
        updated_at = NOW()
      RETURNING *`,
      [
        provider.id,
        provider.name,
        provider.display_name,
        provider.cli_available,
        provider.cli_command,
        provider.api_available,
        provider.api_endpoint,
        provider.port_forwarding_supported,
        provider.p2p_all_servers,
        provider.p2p_server_count,
        provider.total_servers,
        provider.total_countries,
        provider.wireguard_supported,
        provider.openvpn_supported,
        provider.kill_switch_available,
        provider.split_tunneling_available,
        JSON.stringify(provider.config || {}),
        this.sourceAccountId,
      ]
    );
    return result.rows[0];
  }

  async getProvider(id: string): Promise<VPNProviderRecord | null> {
    const result = await this.pool.query<VPNProviderRecord>(
      'SELECT * FROM np_vpn_providers WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] || null;
  }

  async getAllProviders(): Promise<VPNProviderRecord[]> {
    const result = await this.pool.query<VPNProviderRecord>(
      'SELECT * FROM np_vpn_providers WHERE source_account_id = $1 ORDER BY name',
      [this.sourceAccountId]
    );
    return result.rows;
  }

  // ============================================================================
  // Credential Operations (with encryption)
  // ============================================================================

  async upsertCredentials(credentials: Partial<VPNCredentialRecord>, encryptionKey: string): Promise<VPNCredentialRecord> {
    const result = await this.pool.query<VPNCredentialRecord>(
      `INSERT INTO np_vpn_credentials (
        provider_id, username, password_encrypted, api_key_encrypted, api_token_encrypted,
        account_number, private_key_encrypted, additional_data, expires_at, source_account_id
      ) VALUES (
        $1, $2,
        pgp_sym_encrypt($3::text, $4),
        pgp_sym_encrypt($5::text, $4),
        pgp_sym_encrypt($6::text, $4),
        $7,
        pgp_sym_encrypt($8::text, $4),
        $9, $10, $11
      )
      ON CONFLICT (provider_id, source_account_id) DO UPDATE SET
        username = EXCLUDED.username,
        password_encrypted = EXCLUDED.password_encrypted,
        api_key_encrypted = EXCLUDED.api_key_encrypted,
        api_token_encrypted = EXCLUDED.api_token_encrypted,
        account_number = EXCLUDED.account_number,
        private_key_encrypted = EXCLUDED.private_key_encrypted,
        additional_data = EXCLUDED.additional_data,
        expires_at = EXCLUDED.expires_at,
        updated_at = NOW()
      RETURNING
        id, provider_id, username, account_number, additional_data, expires_at, created_at, updated_at, source_account_id`,
      [
        credentials.provider_id,
        credentials.username,
        credentials.password_encrypted || '',
        encryptionKey,
        credentials.api_key_encrypted || '',
        credentials.api_token_encrypted || '',
        credentials.account_number,
        credentials.private_key_encrypted || '',
        JSON.stringify(credentials.additional_data || {}),
        credentials.expires_at,
        this.sourceAccountId,
      ]
    );
    return result.rows[0];
  }

  async getCredentials(providerId: string, encryptionKey: string): Promise<VPNCredentialRecord | null> {
    const result = await this.pool.query<VPNCredentialRecord>(
      `SELECT
        id, provider_id, username, account_number, additional_data, expires_at, created_at, updated_at, source_account_id,
        pgp_sym_decrypt(password_encrypted, $2) AS password_encrypted,
        pgp_sym_decrypt(api_key_encrypted, $2) AS api_key_encrypted,
        pgp_sym_decrypt(api_token_encrypted, $2) AS api_token_encrypted,
        pgp_sym_decrypt(private_key_encrypted, $2) AS private_key_encrypted
      FROM np_vpn_credentials
      WHERE provider_id = $1 AND source_account_id = $3`,
      [providerId, encryptionKey, this.sourceAccountId]
    );
    return result.rows[0] || null;
  }

  async deleteCredentials(providerId: string): Promise<boolean> {
    const result = await this.pool.query(
      'DELETE FROM np_vpn_credentials WHERE provider_id = $1 AND source_account_id = $2',
      [providerId, this.sourceAccountId]
    );
    return (result.rowCount ?? 0) > 0;
  }

  // ============================================================================
  // Server Operations
  // ============================================================================

  async upsertServer(server: Partial<VPNServerRecord>): Promise<VPNServerRecord> {
    const result = await this.pool.query<VPNServerRecord>(
      `INSERT INTO np_vpn_servers (
        provider_id, hostname, ip_address, ipv6_address, country_code, country_name, city, region,
        latitude, longitude, p2p_supported, port_forwarding_supported, protocols, load, capacity,
        status, features, public_key, endpoint_port, owned, metadata, source_account_id
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
      ON CONFLICT (provider_id, hostname, source_account_id) DO UPDATE SET
        ip_address = EXCLUDED.ip_address,
        ipv6_address = EXCLUDED.ipv6_address,
        country_code = EXCLUDED.country_code,
        country_name = EXCLUDED.country_name,
        city = EXCLUDED.city,
        region = EXCLUDED.region,
        latitude = EXCLUDED.latitude,
        longitude = EXCLUDED.longitude,
        p2p_supported = EXCLUDED.p2p_supported,
        port_forwarding_supported = EXCLUDED.port_forwarding_supported,
        protocols = EXCLUDED.protocols,
        load = EXCLUDED.load,
        capacity = EXCLUDED.capacity,
        status = EXCLUDED.status,
        features = EXCLUDED.features,
        public_key = EXCLUDED.public_key,
        endpoint_port = EXCLUDED.endpoint_port,
        owned = EXCLUDED.owned,
        metadata = EXCLUDED.metadata,
        last_seen = NOW(),
        updated_at = NOW()
      RETURNING *`,
      [
        server.provider_id,
        server.hostname,
        server.ip_address,
        server.ipv6_address,
        server.country_code,
        server.country_name,
        server.city,
        server.region,
        server.latitude,
        server.longitude,
        server.p2p_supported,
        server.port_forwarding_supported,
        server.protocols,
        server.load,
        server.capacity,
        server.status || 'online',
        server.features || [],
        server.public_key,
        server.endpoint_port,
        server.owned || false,
        JSON.stringify(server.metadata || {}),
        this.sourceAccountId,
      ]
    );
    return result.rows[0];
  }

  async getServers(filters: {
    provider?: string;
    country?: string;
    p2p_only?: boolean;
    port_forwarding?: boolean;
    status?: string;
    limit?: number;
  }): Promise<VPNServerRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.provider) {
      conditions.push(`provider_id = $${paramIndex++}`);
      values.push(filters.provider);
    }

    if (filters.country) {
      conditions.push(`country_code = $${paramIndex++}`);
      values.push(filters.country);
    }

    if (filters.p2p_only) {
      conditions.push('p2p_supported = true');
    }

    if (filters.port_forwarding) {
      conditions.push('port_forwarding_supported = true');
    }

    if (filters.status) {
      conditions.push(`status = $${paramIndex++}`);
      values.push(filters.status);
    }

    const limit = filters.limit || 100;
    const query = `
      SELECT * FROM np_vpn_servers
      WHERE ${conditions.join(' AND ')}
      ORDER BY load ASC NULLS LAST, created_at DESC
      LIMIT $${paramIndex}
    `;
    values.push(limit);

    const result = await this.pool.query<VPNServerRecord>(query, values);
    return result.rows;
  }

  // ============================================================================
  // Connection Operations
  // ============================================================================

  async createConnection(connection: Partial<VPNConnectionRecord>): Promise<VPNConnectionRecord> {
    const result = await this.pool.query<VPNConnectionRecord>(
      `INSERT INTO np_vpn_connections (
        provider_id, server_id, protocol, status, local_ip, vpn_ip, interface_name, dns_servers,
        connected_at, kill_switch_enabled, requested_by, metadata, source_account_id
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
      RETURNING *`,
      [
        connection.provider_id,
        connection.server_id,
        connection.protocol,
        connection.status || 'connecting',
        connection.local_ip,
        connection.vpn_ip,
        connection.interface_name,
        connection.dns_servers || [],
        connection.connected_at || new Date(),
        connection.kill_switch_enabled !== false,
        connection.requested_by,
        JSON.stringify(connection.metadata || {}),
        this.sourceAccountId,
      ]
    );
    return result.rows[0];
  }

  async updateConnection(id: string, updates: Partial<VPNConnectionRecord>): Promise<VPNConnectionRecord> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const updateableFields: (keyof VPNConnectionRecord)[] = [
      'status',
      'vpn_ip',
      'interface_name',
      'dns_servers',
      'disconnected_at',
      'duration_seconds',
      'bytes_sent',
      'bytes_received',
      'error_message',
      'port_forwarded',
    ];

    for (const field of updateableFields) {
      if (updates[field] !== undefined) {
        fields.push(`${field} = $${paramIndex++}`);
        values.push(updates[field]);
      }
    }

    if (fields.length === 0) {
      throw new Error('No fields to update');
    }

    values.push(id);
    values.push(this.sourceAccountId);
    const result = await this.pool.query<VPNConnectionRecord>(
      `UPDATE np_vpn_connections SET ${fields.join(', ')} WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1} RETURNING *`,
      values
    );

    if (result.rows.length === 0) {
      throw new Error(`Connection ${id} not found`);
    }

    return result.rows[0];
  }

  async getActiveConnection(): Promise<VPNConnectionRecord | null> {
    const result = await this.pool.query<VPNConnectionRecord>(
      `SELECT * FROM np_vpn_connections WHERE status = 'connected' AND source_account_id = $1 ORDER BY connected_at DESC LIMIT 1`,
      [this.sourceAccountId]
    );
    return result.rows[0] || null;
  }

  async getConnection(id: string): Promise<VPNConnectionRecord | null> {
    const result = await this.pool.query<VPNConnectionRecord>(
      'SELECT * FROM np_vpn_connections WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] || null;
  }

  // ============================================================================
  // Download Operations
  // ============================================================================

  async createDownload(download: Partial<VPNDownloadRecord>): Promise<VPNDownloadRecord> {
    const result = await this.pool.query<VPNDownloadRecord>(
      `INSERT INTO np_vpn_downloads (
        connection_id, magnet_link, info_hash, name, destination_path, status, progress,
        bytes_downloaded, bytes_total, requested_by, provider_id, server_id, metadata, source_account_id
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      RETURNING *`,
      [
        download.connection_id,
        download.magnet_link,
        download.info_hash,
        download.name,
        download.destination_path,
        download.status || 'queued',
        download.progress || 0,
        download.bytes_downloaded || 0,
        download.bytes_total,
        download.requested_by,
        download.provider_id,
        download.server_id,
        JSON.stringify(download.metadata || {}),
        this.sourceAccountId,
      ]
    );
    return result.rows[0];
  }

  async updateDownload(id: string, updates: Partial<VPNDownloadRecord>): Promise<VPNDownloadRecord> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const updateableFields: (keyof VPNDownloadRecord)[] = [
      'status',
      'progress',
      'bytes_downloaded',
      'bytes_total',
      'download_speed',
      'upload_speed',
      'peers',
      'seeds',
      'eta_seconds',
      'started_at',
      'completed_at',
      'error_message',
      'name',
    ];

    for (const field of updateableFields) {
      if (updates[field] !== undefined) {
        fields.push(`${field} = $${paramIndex++}`);
        values.push(updates[field]);
      }
    }

    if (fields.length === 0) {
      throw new Error('No fields to update');
    }

    values.push(id);
    values.push(this.sourceAccountId);
    const result = await this.pool.query<VPNDownloadRecord>(
      `UPDATE np_vpn_downloads SET ${fields.join(', ')} WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1} RETURNING *`,
      values
    );

    if (result.rows.length === 0) {
      throw new Error(`Download ${id} not found`);
    }

    return result.rows[0];
  }

  async getDownload(id: string): Promise<VPNDownloadRecord | null> {
    const result = await this.pool.query<VPNDownloadRecord>(
      'SELECT * FROM np_vpn_downloads WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] || null;
  }

  async getActiveDownloads(): Promise<VPNDownloadRecord[]> {
    const result = await this.pool.query<VPNDownloadRecord>(
      `SELECT * FROM np_vpn_downloads
       WHERE status IN ('queued', 'connecting_vpn', 'downloading', 'paused')
         AND source_account_id = $1
       ORDER BY created_at ASC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async getAllDownloads(limit: number = 100): Promise<VPNDownloadRecord[]> {
    const result = await this.pool.query<VPNDownloadRecord>(
      'SELECT * FROM np_vpn_downloads WHERE source_account_id = $1 ORDER BY created_at DESC LIMIT $2',
      [this.sourceAccountId, limit]
    );
    return result.rows;
  }

  // ============================================================================
  // Statistics
  // ============================================================================

  async getStatistics(): Promise<VPNStatistics> {
    // Get total and active connections
    const connectionsResult = await this.pool.query<{ total: number; active: number }>(
      `SELECT
        COUNT(*) as total,
        COUNT(CASE WHEN status = 'connected' THEN 1 END) as active
      FROM np_vpn_connections
      WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    // Get total and active downloads
    const downloadsResult = await this.pool.query<{ total: number; active: number; bytes: string }>(
      `SELECT
        COUNT(*) as total,
        COUNT(CASE WHEN status IN ('downloading', 'queued', 'connecting_vpn') THEN 1 END) as active,
        SUM(bytes_downloaded) as bytes
      FROM np_vpn_downloads
      WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    // Get provider stats
    const providersResult = await this.pool.query(
      `SELECT p.display_name, COUNT(c.id)::text as total_connections,
        ROUND((COUNT(CASE WHEN c.error_message IS NULL THEN 1 END)::NUMERIC /
               NULLIF(COUNT(c.id), 0) * 100), 2)::text AS success_rate_percent
      FROM np_vpn_providers p
      LEFT JOIN np_vpn_connections c ON p.id = c.provider_id AND c.source_account_id = $1
      WHERE p.source_account_id = $1
      GROUP BY p.id, p.display_name
      ORDER BY COUNT(c.id) DESC
      LIMIT 10`,
      [this.sourceAccountId]
    );

    // Get top servers
    const serversResult = await this.pool.query(
      `SELECT s.hostname, s.provider_id, s.country_code,
        COUNT(DISTINCT c.id)::text AS total_connections,
        AVG(sp.download_speed_mbps)::text AS avg_download_speed
      FROM np_vpn_servers s
      LEFT JOIN np_vpn_connections c ON s.id = c.server_id AND c.source_account_id = $1
      LEFT JOIN np_vpn_server_performance sp ON s.id = sp.server_id AND sp.source_account_id = $1
      WHERE s.source_account_id = $1
      GROUP BY s.id, s.hostname, s.provider_id, s.country_code
      ORDER BY COUNT(DISTINCT c.id) DESC, AVG(sp.download_speed_mbps) DESC
      LIMIT 10`,
      [this.sourceAccountId]
    );

    return {
      total_connections: parseInt(String(connectionsResult.rows[0]?.total ?? '0')),
      active_connections: parseInt(String(connectionsResult.rows[0]?.active ?? '0')),
      total_downloads: parseInt(String(downloadsResult.rows[0]?.total ?? '0')),
      active_downloads: parseInt(String(downloadsResult.rows[0]?.active ?? '0')),
      total_bytes_downloaded: downloadsResult.rows[0]?.bytes || '0',
      providers: providersResult.rows.map((row: ProviderStatsRow) => ({
        provider: row.display_name,
        connections: parseInt(row.total_connections),
        uptime_percentage: parseFloat(row.success_rate_percent || '0'),
        avg_speed_mbps: 0, // Would need performance data
      })),
      top_servers: serversResult.rows.map((row: ServerStatsRow) => ({
        server: row.hostname,
        provider: row.provider_id,
        country: row.country_code,
        connections: parseInt(row.total_connections || '0'),
        avg_speed_mbps: parseFloat(row.avg_download_speed || '0'),
      })),
    };
  }

  // ============================================================================
  // Cleanup
  // ============================================================================

  async close(): Promise<void> {
    await this.pool.end();
    logger.info('Database connection pool closed');
  }
}
