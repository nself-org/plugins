/**
 * Torrent Manager Database Operations
 * Complete PostgreSQL schema and CRUD operations
 */

import pg from 'pg';
import { createLogger } from '@nself/plugin-utils';
import type {
  TorrentClient,
  TorrentDownload,
  TorrentSearchCache,
  TorrentSource,
  TorrentFile,
  TorrentTracker,
  SeedingPolicy,
  DownloadSeedingPolicy,
  TorrentStats,
} from './types.js';

const logger = createLogger('torrent-manager:database');

export class TorrentDatabase {
  private pool: pg.Pool;

  constructor(databaseUrl: string) {
    this.pool = new pg.Pool({ connectionString: databaseUrl });
    logger.info('Database pool created');
  }

  // ============================================================================
  // Schema Initialization
  // ============================================================================

  async initialize(): Promise<void> {
    logger.info('Initializing database schema');

    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');

      // Torrent Clients Table
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_torrentmanager_torrent_clients (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          client_type VARCHAR(50) NOT NULL,
          host VARCHAR(255) NOT NULL,
          port INT NOT NULL,
          username VARCHAR(255),
          password_encrypted TEXT,
          is_default BOOLEAN DEFAULT FALSE,
          status VARCHAR(50) NOT NULL DEFAULT 'disconnected',
          last_connected_at TIMESTAMPTZ,
          last_error TEXT,
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_torrent_clients_account
        ON np_torrentmanager_torrent_clients(source_account_id)
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_torrent_clients_type
        ON np_torrentmanager_torrent_clients(client_type)
      `);

      // Torrent Sources Table
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_torrentmanager_sources (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          source_name VARCHAR(50) NOT NULL,
          base_url VARCHAR(500) NOT NULL,
          is_active BOOLEAN DEFAULT TRUE,
          priority INT DEFAULT 50,
          requires_proxy BOOLEAN DEFAULT FALSE,
          last_success_at TIMESTAMPTZ,
          last_failure_at TIMESTAMPTZ,
          failure_count INT DEFAULT 0,
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_np_torrentmanager_sources_active
        ON np_torrentmanager_sources(is_active) WHERE is_active = TRUE
      `);

      // Torrent Downloads Table
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_torrentmanager_torrent_downloads (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          client_id UUID NOT NULL REFERENCES np_torrentmanager_torrent_clients(id) ON DELETE CASCADE,
          client_torrent_id VARCHAR(255) NOT NULL,

          name VARCHAR(500) NOT NULL,
          info_hash VARCHAR(40) NOT NULL,
          magnet_uri TEXT NOT NULL,

          status VARCHAR(50) NOT NULL DEFAULT 'queued',
          category VARCHAR(50) NOT NULL DEFAULT 'other',

          size_bytes BIGINT DEFAULT 0,
          downloaded_bytes BIGINT DEFAULT 0,
          uploaded_bytes BIGINT DEFAULT 0,
          progress_percent DECIMAL(5,2) DEFAULT 0,
          ratio DECIMAL(5,2) DEFAULT 0,

          download_speed_bytes BIGINT DEFAULT 0,
          upload_speed_bytes BIGINT DEFAULT 0,

          seeders INT DEFAULT 0,
          leechers INT DEFAULT 0,
          peers_connected INT DEFAULT 0,

          download_path VARCHAR(500),
          files_count INT DEFAULT 0,

          stop_at_ratio DECIMAL(5,2),
          stop_at_time_hours INT,

          vpn_ip VARCHAR(50),
          vpn_interface VARCHAR(50),

          error_message TEXT,

          content_id UUID,
          requested_by VARCHAR(255) NOT NULL,
          metadata JSONB DEFAULT '{}',

          added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          started_at TIMESTAMPTZ,
          completed_at TIMESTAMPTZ,
          stopped_at TIMESTAMPTZ,

          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_torrent_downloads_account
        ON np_torrentmanager_torrent_downloads(source_account_id)
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_torrent_downloads_status
        ON np_torrentmanager_torrent_downloads(status)
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_torrent_downloads_info_hash
        ON np_torrentmanager_torrent_downloads(info_hash)
      `);

      // Torrent Files Table
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_torrentmanager_torrent_files (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          download_id UUID NOT NULL REFERENCES np_torrentmanager_torrent_downloads(id) ON DELETE CASCADE,
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          file_index INT NOT NULL,
          file_name VARCHAR(500) NOT NULL,
          file_path VARCHAR(500) NOT NULL,
          size_bytes BIGINT NOT NULL,
          downloaded_bytes BIGINT DEFAULT 0,
          progress_percent DECIMAL(5,2) DEFAULT 0,
          priority INT DEFAULT 0,
          is_selected BOOLEAN DEFAULT TRUE,
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_torrent_files_download
        ON np_torrentmanager_torrent_files(download_id)
      `);

      // Torrent Trackers Table
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_torrentmanager_torrent_trackers (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          download_id UUID NOT NULL REFERENCES np_torrentmanager_torrent_downloads(id) ON DELETE CASCADE,
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          tracker_url VARCHAR(500) NOT NULL,
          tier INT NOT NULL,
          status VARCHAR(50) NOT NULL,
          seeders INT,
          leechers INT,
          last_announce_at TIMESTAMPTZ,
          last_scrape_at TIMESTAMPTZ,
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_torrent_trackers_download
        ON np_torrentmanager_torrent_trackers(download_id)
      `);

      // Torrent Search Cache Table
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_torrentmanager_search_cache (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          query_hash VARCHAR(64) NOT NULL,
          query TEXT NOT NULL,
          results JSONB NOT NULL DEFAULT '[]',
          results_count INT DEFAULT 0,
          sources_searched VARCHAR(50)[] DEFAULT '{}',
          search_duration_ms INT,
          cached_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          expires_at TIMESTAMPTZ NOT NULL,
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_np_torrentmanager_search_cache_hash
        ON np_torrentmanager_search_cache(query_hash)
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_np_torrentmanager_search_cache_expires
        ON np_torrentmanager_search_cache(expires_at)
      `);

      // Seeding Policy Table
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_torrentmanager_seeding_policy (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          policy_name VARCHAR(255) NOT NULL,
          description TEXT,
          ratio_limit DECIMAL(5,2),
          ratio_action VARCHAR(50) DEFAULT 'stop',
          time_limit_hours INT,
          time_action VARCHAR(50) DEFAULT 'stop',
          max_seeding_size_gb INT,
          applies_to_categories VARCHAR(50)[] DEFAULT '{}',
          priority INT DEFAULT 50,
          is_active BOOLEAN DEFAULT TRUE,
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )
      `);

      // Torrent Stats Table
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_torrentmanager_stats (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          total_downloads INT DEFAULT 0,
          active_downloads INT DEFAULT 0,
          completed_downloads INT DEFAULT 0,
          failed_downloads INT DEFAULT 0,
          seeding_torrents INT DEFAULT 0,
          total_downloaded_bytes BIGINT DEFAULT 0,
          total_uploaded_bytes BIGINT DEFAULT 0,
          overall_ratio DECIMAL(5,2) DEFAULT 0,
          download_speed_bytes BIGINT DEFAULT 0,
          upload_speed_bytes BIGINT DEFAULT 0,
          disk_space_used_bytes BIGINT DEFAULT 0,
          disk_space_available_bytes BIGINT DEFAULT 0,
          snapshot_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )
      `);

      // Per-Download Seeding Policies Table
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_torrentmanager_seeding_policies (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          download_id VARCHAR(255) NOT NULL,
          ratio_limit DECIMAL(5,2) DEFAULT 2.0,
          time_limit_hours INT DEFAULT 168,
          auto_remove BOOLEAN DEFAULT TRUE,
          keep_files BOOLEAN DEFAULT FALSE,
          favorite BOOLEAN DEFAULT FALSE,
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )
      `);

      await client.query(`
        CREATE UNIQUE INDEX IF NOT EXISTS idx_np_torrentmanager_seeding_policies_download
        ON np_torrentmanager_seeding_policies(download_id)
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_np_torrentmanager_seeding_policies_account
        ON np_torrentmanager_seeding_policies(source_account_id)
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_np_torrentmanager_seeding_policies_favorite
        ON np_torrentmanager_seeding_policies(favorite) WHERE favorite = TRUE
      `);

      // Create Views
      await client.query(`
        CREATE OR REPLACE VIEW torrent_active_downloads AS
        SELECT * FROM np_torrentmanager_torrent_downloads
        WHERE status IN ('downloading', 'paused')
        ORDER BY added_at DESC
      `);

      await client.query(`
        CREATE OR REPLACE VIEW torrent_completed_downloads AS
        SELECT * FROM np_torrentmanager_torrent_downloads
        WHERE status = 'completed'
        ORDER BY completed_at DESC
      `);

      await client.query(`
        CREATE OR REPLACE VIEW torrent_seeding_torrents AS
        SELECT * FROM np_torrentmanager_torrent_downloads
        WHERE status = 'seeding'
        ORDER BY completed_at DESC
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
  // Torrent Client Operations
  // ============================================================================

  async upsertClient(client: Partial<TorrentClient>): Promise<TorrentClient> {
    const result = await this.pool.query(
      `INSERT INTO np_torrentmanager_torrent_clients (
        source_account_id, client_type, host, port, username, password_encrypted,
        is_default, status, last_connected_at, last_error
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
      ON CONFLICT (id) DO UPDATE SET
        host = EXCLUDED.host,
        port = EXCLUDED.port,
        username = EXCLUDED.username,
        password_encrypted = EXCLUDED.password_encrypted,
        is_default = EXCLUDED.is_default,
        status = EXCLUDED.status,
        last_connected_at = EXCLUDED.last_connected_at,
        last_error = EXCLUDED.last_error,
        updated_at = NOW()
      RETURNING *`,
      [
        client.source_account_id || 'primary',
        client.client_type,
        client.host,
        client.port,
        client.username,
        client.password_encrypted,
        client.is_default || false,
        client.status || 'disconnected',
        client.last_connected_at,
        client.last_error,
      ]
    );
    return result.rows[0];
  }

  async getDefaultClient(): Promise<TorrentClient | null> {
    const result = await this.pool.query(
      'SELECT * FROM np_torrentmanager_torrent_clients WHERE is_default = TRUE LIMIT 1'
    );
    return result.rows[0] || null;
  }

  async listClients(): Promise<TorrentClient[]> {
    const result = await this.pool.query('SELECT * FROM np_torrentmanager_torrent_clients ORDER BY created_at DESC');
    return result.rows;
  }

  // ============================================================================
  // Torrent Download Operations
  // ============================================================================

  async createDownload(download: Partial<TorrentDownload>): Promise<TorrentDownload> {
    const result = await this.pool.query(
      `INSERT INTO np_torrentmanager_torrent_downloads (
        source_account_id, client_id, client_torrent_id, name, info_hash, magnet_uri,
        status, category, size_bytes, download_path, requested_by, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
      RETURNING *`,
      [
        download.source_account_id || 'primary',
        download.client_id,
        download.client_torrent_id,
        download.name,
        download.info_hash,
        download.magnet_uri,
        download.status || 'queued',
        download.category || 'other',
        download.size_bytes || 0,
        download.download_path,
        download.requested_by,
        JSON.stringify(download.metadata || {}),
      ]
    );
    return result.rows[0];
  }

  async updateDownload(id: string, updates: Partial<TorrentDownload>): Promise<TorrentDownload> {
    const fields = [];
    const values = [];
    let paramCount = 1;

    if (updates.status) {
      fields.push(`status = $${paramCount++}`);
      values.push(updates.status);
    }
    if (updates.progress_percent !== undefined) {
      fields.push(`progress_percent = $${paramCount++}`);
      values.push(updates.progress_percent);
    }
    if (updates.downloaded_bytes !== undefined) {
      fields.push(`downloaded_bytes = $${paramCount++}`);
      values.push(updates.downloaded_bytes);
    }
    if (updates.uploaded_bytes !== undefined) {
      fields.push(`uploaded_bytes = $${paramCount++}`);
      values.push(updates.uploaded_bytes);
    }
    if (updates.ratio !== undefined) {
      fields.push(`ratio = $${paramCount++}`);
      values.push(updates.ratio);
    }
    if (updates.seeders !== undefined) {
      fields.push(`seeders = $${paramCount++}`);
      values.push(updates.seeders);
    }
    if (updates.leechers !== undefined) {
      fields.push(`leechers = $${paramCount++}`);
      values.push(updates.leechers);
    }
    if (updates.error_message !== undefined) {
      fields.push(`error_message = $${paramCount++}`);
      values.push(updates.error_message);
    }

    fields.push(`updated_at = NOW()`);
    values.push(id);
    paramCount++;

    const result = await this.pool.query(
      `UPDATE np_torrentmanager_torrent_downloads SET ${fields.join(', ')} WHERE id = $${paramCount} RETURNING *`,
      values
    );
    return result.rows[0];
  }

  async getDownload(id: string): Promise<TorrentDownload | null> {
    const result = await this.pool.query('SELECT * FROM np_torrentmanager_torrent_downloads WHERE id = $1', [id]);
    return result.rows[0] || null;
  }

  async listDownloads(filter?: {
    status?: string;
    category?: string;
    limit?: number;
  }): Promise<TorrentDownload[]> {
    let query = 'SELECT * FROM np_torrentmanager_torrent_downloads WHERE 1=1';
    const values: unknown[] = [];
    let paramCount = 1;

    if (filter?.status) {
      query += ` AND status = $${paramCount++}`;
      values.push(filter.status);
    }
    if (filter?.category) {
      query += ` AND category = $${paramCount++}`;
      values.push(filter.category);
    }

    query += ` ORDER BY added_at DESC`;

    if (filter?.limit) {
      query += ` LIMIT $${paramCount++}`;
      values.push(filter.limit);
    }

    const result = await this.pool.query(query, values);
    return result.rows;
  }

  async deleteDownload(id: string): Promise<void> {
    await this.pool.query('DELETE FROM np_torrentmanager_torrent_downloads WHERE id = $1', [id]);
  }

  // ============================================================================
  // Per-Download Seeding Policy Operations
  // ============================================================================

  async upsertDownloadSeedingPolicy(
    downloadId: string,
    policy: Partial<DownloadSeedingPolicy>
  ): Promise<DownloadSeedingPolicy> {
    const result = await this.pool.query(
      `INSERT INTO np_torrentmanager_seeding_policies (
        source_account_id, download_id, ratio_limit, time_limit_hours,
        auto_remove, keep_files, favorite
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      ON CONFLICT (download_id) DO UPDATE SET
        ratio_limit = COALESCE(EXCLUDED.ratio_limit, np_torrentmanager_seeding_policies.ratio_limit),
        time_limit_hours = COALESCE(EXCLUDED.time_limit_hours, np_torrentmanager_seeding_policies.time_limit_hours),
        auto_remove = COALESCE(EXCLUDED.auto_remove, np_torrentmanager_seeding_policies.auto_remove),
        keep_files = COALESCE(EXCLUDED.keep_files, np_torrentmanager_seeding_policies.keep_files),
        favorite = COALESCE(EXCLUDED.favorite, np_torrentmanager_seeding_policies.favorite),
        updated_at = NOW()
      RETURNING *`,
      [
        policy.source_account_id || 'primary',
        downloadId,
        policy.ratio_limit ?? 2.0,
        policy.time_limit_hours ?? 168,
        policy.auto_remove ?? true,
        policy.keep_files ?? false,
        policy.favorite ?? false,
      ]
    );
    return result.rows[0];
  }

  async getDownloadSeedingPolicy(downloadId: string): Promise<DownloadSeedingPolicy | null> {
    const result = await this.pool.query(
      'SELECT * FROM np_torrentmanager_seeding_policies WHERE download_id = $1',
      [downloadId]
    );
    return result.rows[0] || null;
  }

  // ============================================================================
  // Search Cache Operations
  // ============================================================================

  async cacheSearchResults(cache: Partial<TorrentSearchCache>): Promise<void> {
    await this.pool.query(
      `INSERT INTO np_torrentmanager_search_cache (
        source_account_id, query_hash, query, results, results_count,
        sources_searched, search_duration_ms, expires_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
      [
        cache.source_account_id || 'primary',
        cache.query_hash,
        cache.query,
        JSON.stringify(cache.results || []),
        cache.results_count || 0,
        cache.sources_searched || [],
        cache.search_duration_ms,
        cache.expires_at,
      ]
    );
  }

  async getSearchCache(queryHash: string): Promise<TorrentSearchCache | null> {
    const result = await this.pool.query(
      `SELECT * FROM np_torrentmanager_search_cache
       WHERE query_hash = $1 AND expires_at > NOW()
       ORDER BY created_at DESC LIMIT 1`,
      [queryHash]
    );
    return result.rows[0] || null;
  }

  // ============================================================================
  // Statistics
  // ============================================================================

  async getStats(): Promise<TorrentStats> {
    const result = await this.pool.query(`
      SELECT
        COUNT(*) FILTER (WHERE status NOT IN ('removed')) as total_downloads,
        COUNT(*) FILTER (WHERE status IN ('downloading', 'paused')) as active_downloads,
        COUNT(*) FILTER (WHERE status = 'completed') as completed_downloads,
        COUNT(*) FILTER (WHERE status = 'failed') as failed_downloads,
        COUNT(*) FILTER (WHERE status = 'seeding') as seeding_torrents,
        COALESCE(SUM(downloaded_bytes), 0) as total_downloaded_bytes,
        COALESCE(SUM(uploaded_bytes), 0) as total_uploaded_bytes,
        COALESCE(AVG(ratio), 0) as overall_ratio,
        COALESCE(SUM(download_speed_bytes), 0) as download_speed_bytes,
        COALESCE(SUM(upload_speed_bytes), 0) as upload_speed_bytes
      FROM np_torrentmanager_torrent_downloads
    `);

    return {
      total_downloads: parseInt(result.rows[0].total_downloads, 10),
      active_downloads: parseInt(result.rows[0].active_downloads, 10),
      completed_downloads: parseInt(result.rows[0].completed_downloads, 10),
      failed_downloads: parseInt(result.rows[0].failed_downloads, 10),
      seeding_torrents: parseInt(result.rows[0].seeding_torrents, 10),
      total_downloaded_bytes: parseInt(result.rows[0].total_downloaded_bytes, 10),
      total_uploaded_bytes: parseInt(result.rows[0].total_uploaded_bytes, 10),
      overall_ratio: parseFloat(result.rows[0].overall_ratio),
      download_speed_bytes: parseInt(result.rows[0].download_speed_bytes, 10),
      upload_speed_bytes: parseInt(result.rows[0].upload_speed_bytes, 10),
      disk_space_used_bytes: 0, // Would need filesystem check
      disk_space_available_bytes: 0, // Would need filesystem check
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
