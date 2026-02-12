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

export class VPNDatabase {
  private pool: Pool;

  constructor(connectionString: string) {
    this.pool = new Pool({
      connectionString,
      max: 20,
      idleTimeoutMillis: 30000,
      connectionTimeoutMillis: 2000,
    });

    this.pool.on('error', (err) => {
      logger.error('Unexpected database error', { error: err instanceof Error ? err.message : String(err) });
    });
  }

  /**
   * Execute a raw query against the pool (for ad-hoc queries not covered by typed methods)
   */
  async query<T extends Record<string, any> = any>(text: string, values?: any[]): Promise<QueryResult<T>> {
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
        CREATE TABLE IF NOT EXISTS vpn_providers (
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
          created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
        )
      `);

      // VPN Credentials table (encrypted)
      await client.query(`
        CREATE TABLE IF NOT EXISTS vpn_credentials (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          provider_id VARCHAR(255) NOT NULL REFERENCES vpn_providers(id) ON DELETE CASCADE,
          username VARCHAR(255),
          password_encrypted TEXT,
          api_key_encrypted TEXT,
          api_token_encrypted TEXT,
          account_number VARCHAR(255),
          private_key_encrypted TEXT,
          additional_data JSONB DEFAULT '{}',
          expires_at TIMESTAMP WITH TIME ZONE,
          created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          UNIQUE(provider_id)
        )
      `);

      // VPN Servers table
      await client.query(`
        CREATE TABLE IF NOT EXISTS vpn_servers (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          provider_id VARCHAR(255) NOT NULL REFERENCES vpn_providers(id) ON DELETE CASCADE,
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
          last_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          UNIQUE(provider_id, hostname)
        )
      `);

      // Create indexes for servers
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_vpn_servers_provider ON vpn_servers(provider_id);
        CREATE INDEX IF NOT EXISTS idx_vpn_servers_country ON vpn_servers(country_code);
        CREATE INDEX IF NOT EXISTS idx_vpn_servers_p2p ON vpn_servers(p2p_supported) WHERE p2p_supported = true;
        CREATE INDEX IF NOT EXISTS idx_vpn_servers_status ON vpn_servers(status);
        CREATE INDEX IF NOT EXISTS idx_vpn_servers_load ON vpn_servers(load);
      `);

      // VPN Connections table
      await client.query(`
        CREATE TABLE IF NOT EXISTS vpn_connections (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          provider_id VARCHAR(255) NOT NULL REFERENCES vpn_providers(id),
          server_id UUID REFERENCES vpn_servers(id),
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
          created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
        )
      `);

      // Create indexes for connections
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_vpn_connections_status ON vpn_connections(status);
        CREATE INDEX IF NOT EXISTS idx_vpn_connections_provider ON vpn_connections(provider_id);
        CREATE INDEX IF NOT EXISTS idx_vpn_connections_created ON vpn_connections(created_at DESC);
      `);

      // VPN Downloads table
      await client.query(`
        CREATE TABLE IF NOT EXISTS vpn_downloads (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          connection_id UUID REFERENCES vpn_connections(id),
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
          provider_id VARCHAR(255) NOT NULL REFERENCES vpn_providers(id),
          server_id UUID REFERENCES vpn_servers(id),
          started_at TIMESTAMP WITH TIME ZONE,
          completed_at TIMESTAMP WITH TIME ZONE,
          error_message TEXT,
          metadata JSONB DEFAULT '{}',
          created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
        )
      `);

      // Create indexes for downloads
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_vpn_downloads_status ON vpn_downloads(status);
        CREATE INDEX IF NOT EXISTS idx_vpn_downloads_info_hash ON vpn_downloads(info_hash);
        CREATE INDEX IF NOT EXISTS idx_vpn_downloads_created ON vpn_downloads(created_at DESC);
      `);

      // VPN Connection Logs table
      await client.query(`
        CREATE TABLE IF NOT EXISTS vpn_connection_logs (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          connection_id UUID NOT NULL REFERENCES vpn_connections(id) ON DELETE CASCADE,
          timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          event_type VARCHAR(50) NOT NULL,
          message TEXT NOT NULL,
          details JSONB DEFAULT '{}'
        )
      `);

      // Create index for logs
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_vpn_connection_logs_connection ON vpn_connection_logs(connection_id);
        CREATE INDEX IF NOT EXISTS idx_vpn_connection_logs_timestamp ON vpn_connection_logs(timestamp DESC);
      `);

      // VPN Server Performance table
      await client.query(`
        CREATE TABLE IF NOT EXISTS vpn_server_performance (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          server_id UUID NOT NULL REFERENCES vpn_servers(id) ON DELETE CASCADE,
          timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          ping_ms INTEGER,
          download_speed_mbps DECIMAL(10, 2),
          upload_speed_mbps DECIMAL(10, 2),
          load_percentage INTEGER,
          success_rate DECIMAL(5, 4),
          avg_connection_time_ms INTEGER
        )
      `);

      // Create indexes for performance
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_vpn_server_performance_server ON vpn_server_performance(server_id);
        CREATE INDEX IF NOT EXISTS idx_vpn_server_performance_timestamp ON vpn_server_performance(timestamp DESC);
      `);

      // VPN Leak Tests table
      await client.query(`
        CREATE TABLE IF NOT EXISTS vpn_leak_tests (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          connection_id UUID NOT NULL REFERENCES vpn_connections(id) ON DELETE CASCADE,
          test_type VARCHAR(50) NOT NULL,
          passed BOOLEAN NOT NULL,
          expected_value VARCHAR(255),
          actual_value VARCHAR(255),
          details JSONB DEFAULT '{}',
          tested_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
        )
      `);

      // Create index for leak tests
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_vpn_leak_tests_connection ON vpn_leak_tests(connection_id);
        CREATE INDEX IF NOT EXISTS idx_vpn_leak_tests_passed ON vpn_leak_tests(passed);
      `);

      // Create analytics views
      await client.query(`
        CREATE OR REPLACE VIEW vpn_active_connections AS
        SELECT
          c.id,
          c.provider_id,
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
        FROM vpn_connections c
        JOIN vpn_providers p ON c.provider_id = p.id
        LEFT JOIN vpn_servers s ON c.server_id = s.id
        WHERE c.status = 'connected'
        ORDER BY c.connected_at DESC
      `);

      await client.query(`
        CREATE OR REPLACE VIEW vpn_server_stats AS
        SELECT
          s.id,
          s.provider_id,
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
        FROM vpn_servers s
        LEFT JOIN vpn_connections c ON s.id = c.server_id
        LEFT JOIN vpn_server_performance sp ON s.id = sp.server_id
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
          p.display_name AS provider_name,
          s.hostname AS server,
          s.country_code,
          d.started_at,
          d.completed_at,
          EXTRACT(EPOCH FROM (COALESCE(d.completed_at, NOW()) - d.started_at))::INTEGER AS duration_seconds,
          d.created_at
        FROM vpn_downloads d
        JOIN vpn_providers p ON d.provider_id = p.id
        LEFT JOIN vpn_servers s ON d.server_id = s.id
        ORDER BY d.created_at DESC
      `);

      await client.query(`
        CREATE OR REPLACE VIEW vpn_provider_uptime AS
        SELECT
          p.id,
          p.display_name,
          COUNT(DISTINCT c.id) AS total_connections,
          SUM(CASE WHEN c.status = 'connected' THEN 1 ELSE 0 END) AS active_connections,
          SUM(c.duration_seconds) AS total_uptime_seconds,
          AVG(c.duration_seconds) AS avg_session_duration_seconds,
          ROUND((COUNT(CASE WHEN c.error_message IS NULL THEN 1 END)::NUMERIC /
                 NULLIF(COUNT(c.id), 0) * 100), 2) AS success_rate_percent
        FROM vpn_providers p
        LEFT JOIN vpn_connections c ON p.id = c.provider_id
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
  // Provider Operations
  // ============================================================================

  async upsertProvider(provider: Partial<VPNProviderRecord>): Promise<VPNProviderRecord> {
    const result = await this.pool.query<VPNProviderRecord>(
      `INSERT INTO vpn_providers (
        id, name, display_name, cli_available, cli_command, api_available, api_endpoint,
        port_forwarding_supported, p2p_all_servers, p2p_server_count, total_servers,
        total_countries, wireguard_supported, openvpn_supported, kill_switch_available,
        split_tunneling_available, config
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
      ON CONFLICT (id) DO UPDATE SET
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
      ]
    );
    return result.rows[0];
  }

  async getProvider(id: string): Promise<VPNProviderRecord | null> {
    const result = await this.pool.query<VPNProviderRecord>('SELECT * FROM vpn_providers WHERE id = $1', [id]);
    return result.rows[0] || null;
  }

  async getAllProviders(): Promise<VPNProviderRecord[]> {
    const result = await this.pool.query<VPNProviderRecord>('SELECT * FROM vpn_providers ORDER BY name');
    return result.rows;
  }

  // ============================================================================
  // Credential Operations (with encryption)
  // ============================================================================

  async upsertCredentials(credentials: Partial<VPNCredentialRecord>, encryptionKey: string): Promise<VPNCredentialRecord> {
    const result = await this.pool.query<VPNCredentialRecord>(
      `INSERT INTO vpn_credentials (
        provider_id, username, password_encrypted, api_key_encrypted, api_token_encrypted,
        account_number, private_key_encrypted, additional_data, expires_at
      ) VALUES (
        $1, $2,
        pgp_sym_encrypt($3::text, $4),
        pgp_sym_encrypt($5::text, $4),
        pgp_sym_encrypt($6::text, $4),
        $7,
        pgp_sym_encrypt($8::text, $4),
        $9, $10
      )
      ON CONFLICT (provider_id) DO UPDATE SET
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
        id, provider_id, username, account_number, additional_data, expires_at, created_at, updated_at`,
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
      ]
    );
    return result.rows[0];
  }

  async getCredentials(providerId: string, encryptionKey: string): Promise<VPNCredentialRecord | null> {
    const result = await this.pool.query<any>(
      `SELECT
        id, provider_id, username, account_number, additional_data, expires_at, created_at, updated_at,
        pgp_sym_decrypt(password_encrypted, $2) AS password_encrypted,
        pgp_sym_decrypt(api_key_encrypted, $2) AS api_key_encrypted,
        pgp_sym_decrypt(api_token_encrypted, $2) AS api_token_encrypted,
        pgp_sym_decrypt(private_key_encrypted, $2) AS private_key_encrypted
      FROM vpn_credentials
      WHERE provider_id = $1`,
      [providerId, encryptionKey]
    );
    return result.rows[0] || null;
  }

  async deleteCredentials(providerId: string): Promise<boolean> {
    const result = await this.pool.query('DELETE FROM vpn_credentials WHERE provider_id = $1', [providerId]);
    return (result.rowCount ?? 0) > 0;
  }

  // ============================================================================
  // Server Operations
  // ============================================================================

  async upsertServer(server: Partial<VPNServerRecord>): Promise<VPNServerRecord> {
    const result = await this.pool.query<VPNServerRecord>(
      `INSERT INTO vpn_servers (
        provider_id, hostname, ip_address, ipv6_address, country_code, country_name, city, region,
        latitude, longitude, p2p_supported, port_forwarding_supported, protocols, load, capacity,
        status, features, public_key, endpoint_port, owned, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
      ON CONFLICT (provider_id, hostname) DO UPDATE SET
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
    const conditions: string[] = ['1=1'];
    const values: any[] = [];
    let paramIndex = 1;

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
      SELECT * FROM vpn_servers
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
      `INSERT INTO vpn_connections (
        provider_id, server_id, protocol, status, local_ip, vpn_ip, interface_name, dns_servers,
        connected_at, kill_switch_enabled, requested_by, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
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
      ]
    );
    return result.rows[0];
  }

  async updateConnection(id: string, updates: Partial<VPNConnectionRecord>): Promise<VPNConnectionRecord> {
    const fields: string[] = [];
    const values: any[] = [];
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
    const result = await this.pool.query<VPNConnectionRecord>(
      `UPDATE vpn_connections SET ${fields.join(', ')} WHERE id = $${paramIndex} RETURNING *`,
      values
    );

    if (result.rows.length === 0) {
      throw new Error(`Connection ${id} not found`);
    }

    return result.rows[0];
  }

  async getActiveConnection(): Promise<VPNConnectionRecord | null> {
    const result = await this.pool.query<VPNConnectionRecord>(
      `SELECT * FROM vpn_connections WHERE status = 'connected' ORDER BY connected_at DESC LIMIT 1`
    );
    return result.rows[0] || null;
  }

  async getConnection(id: string): Promise<VPNConnectionRecord | null> {
    const result = await this.pool.query<VPNConnectionRecord>('SELECT * FROM vpn_connections WHERE id = $1', [id]);
    return result.rows[0] || null;
  }

  // ============================================================================
  // Download Operations
  // ============================================================================

  async createDownload(download: Partial<VPNDownloadRecord>): Promise<VPNDownloadRecord> {
    const result = await this.pool.query<VPNDownloadRecord>(
      `INSERT INTO vpn_downloads (
        connection_id, magnet_link, info_hash, name, destination_path, status, progress,
        bytes_downloaded, bytes_total, requested_by, provider_id, server_id, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
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
      ]
    );
    return result.rows[0];
  }

  async updateDownload(id: string, updates: Partial<VPNDownloadRecord>): Promise<VPNDownloadRecord> {
    const fields: string[] = [];
    const values: any[] = [];
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
    const result = await this.pool.query<VPNDownloadRecord>(
      `UPDATE vpn_downloads SET ${fields.join(', ')} WHERE id = $${paramIndex} RETURNING *`,
      values
    );

    if (result.rows.length === 0) {
      throw new Error(`Download ${id} not found`);
    }

    return result.rows[0];
  }

  async getDownload(id: string): Promise<VPNDownloadRecord | null> {
    const result = await this.pool.query<VPNDownloadRecord>('SELECT * FROM vpn_downloads WHERE id = $1', [id]);
    return result.rows[0] || null;
  }

  async getActiveDownloads(): Promise<VPNDownloadRecord[]> {
    const result = await this.pool.query<VPNDownloadRecord>(
      `SELECT * FROM vpn_downloads
       WHERE status IN ('queued', 'connecting_vpn', 'downloading', 'paused')
       ORDER BY created_at ASC`
    );
    return result.rows;
  }

  async getAllDownloads(limit: number = 100): Promise<VPNDownloadRecord[]> {
    const result = await this.pool.query<VPNDownloadRecord>(
      'SELECT * FROM vpn_downloads ORDER BY created_at DESC LIMIT $1',
      [limit]
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
      FROM vpn_connections`
    );

    // Get total and active downloads
    const downloadsResult = await this.pool.query<{ total: number; active: number; bytes: string }>(
      `SELECT
        COUNT(*) as total,
        COUNT(CASE WHEN status IN ('downloading', 'queued', 'connecting_vpn') THEN 1 END) as active,
        SUM(bytes_downloaded) as bytes
      FROM vpn_downloads`
    );

    // Get provider stats
    const providersResult = await this.pool.query(
      `SELECT * FROM vpn_provider_uptime ORDER BY total_connections DESC LIMIT 10`
    );

    // Get top servers
    const serversResult = await this.pool.query(
      `SELECT * FROM vpn_server_stats ORDER BY total_connections DESC, avg_download_speed DESC LIMIT 10`
    );

    return {
      total_connections: parseInt(String(connectionsResult.rows[0]?.total ?? '0')),
      active_connections: parseInt(String(connectionsResult.rows[0]?.active ?? '0')),
      total_downloads: parseInt(String(downloadsResult.rows[0]?.total ?? '0')),
      active_downloads: parseInt(String(downloadsResult.rows[0]?.active ?? '0')),
      total_bytes_downloaded: downloadsResult.rows[0]?.bytes || '0',
      providers: providersResult.rows.map((row: any) => ({
        provider: row.display_name,
        connections: parseInt(row.total_connections),
        uptime_percentage: parseFloat(row.success_rate_percent || '0'),
        avg_speed_mbps: 0, // Would need performance data
      })),
      top_servers: serversResult.rows.map((row: any) => ({
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
