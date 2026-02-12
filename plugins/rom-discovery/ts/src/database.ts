/**
 * ROM Discovery Database Operations
 * Complete CRUD for ROM metadata, download queue, scrapers, and popularity tracking
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  RomMetadataRecord,
  DownloadQueueRecord,
  ScraperJobRecord,
  PopularityTrackingRecord,
  PlatformStats,
  FeaturedRoms,
  RomDiscoveryStats,
  AuditLogRecord,
} from './types.js';

const logger = createLogger('rom-discovery:db');

export class RomDiscoveryDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): RomDiscoveryDatabase {
    return new RomDiscoveryDatabase(this.db, sourceAccountId);
  }

  getCurrentSourceAccountId(): string {
    return this.sourceAccountId;
  }

  private normalizeSourceAccountId(value: string): string {
    const normalized = value
      .toLowerCase()
      .replace(/[^a-z0-9_-]+/g, '-')
      .replace(/^-+|-+$/g, '');

    return normalized.length > 0 ? normalized : 'primary';
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  async query<T extends Record<string, unknown>>(sql: string, params?: unknown[]): Promise<{ rows: T[]; rowCount: number | null }> {
    return this.db.query<T>(sql, params);
  }

  async execute(sql: string, params?: unknown[]): Promise<number> {
    return this.db.execute(sql, params);
  }

  // =========================================================================
  // Schema Management
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing ROM Discovery schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
      CREATE EXTENSION IF NOT EXISTS "pg_trgm";

      -- =====================================================================
      -- ROM Metadata
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_romdisc_metadata (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        rom_title VARCHAR(1000) NOT NULL,
        rom_title_normalized VARCHAR(1000) NOT NULL,
        platform VARCHAR(100) NOT NULL,
        region VARCHAR(50),
        file_name VARCHAR(500) NOT NULL,
        file_size_bytes BIGINT,
        file_hash_md5 VARCHAR(32),
        file_hash_sha256 VARCHAR(64),
        file_hash_crc32 VARCHAR(8),
        download_url TEXT,
        download_source VARCHAR(200),
        download_url_verified_at TIMESTAMPTZ,
        download_url_dead BOOLEAN DEFAULT false,
        release_year INTEGER,
        release_month INTEGER,
        release_day INTEGER,
        version VARCHAR(50),
        quality_score INTEGER DEFAULT 0,
        popularity_score INTEGER DEFAULT 0,
        release_group VARCHAR(200),
        is_verified_dump BOOLEAN DEFAULT false,
        is_hack BOOLEAN DEFAULT false,
        is_translation BOOLEAN DEFAULT false,
        is_homebrew BOOLEAN DEFAULT false,
        is_public_domain BOOLEAN DEFAULT false,
        game_title VARCHAR(1000),
        genre VARCHAR(200),
        publisher VARCHAR(500),
        developer VARCHAR(500),
        description TEXT,
        igdb_id INTEGER,
        mobygames_id INTEGER,
        box_art_url TEXT,
        screenshot_urls TEXT[] DEFAULT '{}',
        is_community_rom BOOLEAN DEFAULT false,
        community_source_url TEXT,
        community_update_year INTEGER,
        scraped_from VARCHAR(200),
        scraped_at TIMESTAMPTZ,
        search_vector tsvector,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, file_hash_sha256)
      );

      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_source_app
        ON np_romdisc_metadata(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_platform
        ON np_romdisc_metadata(source_account_id, platform);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_region
        ON np_romdisc_metadata(source_account_id, region);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_quality
        ON np_romdisc_metadata(source_account_id, quality_score DESC);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_popularity
        ON np_romdisc_metadata(source_account_id, popularity_score DESC);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_title_norm
        ON np_romdisc_metadata(source_account_id, rom_title_normalized);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_hash_sha256
        ON np_romdisc_metadata(file_hash_sha256);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_hash_md5
        ON np_romdisc_metadata(file_hash_md5);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_hash_crc32
        ON np_romdisc_metadata(file_hash_crc32);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_search
        ON np_romdisc_metadata USING GIN(search_vector);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_verified
        ON np_romdisc_metadata(source_account_id, is_verified_dump) WHERE is_verified_dump = true;
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_homebrew
        ON np_romdisc_metadata(source_account_id, is_homebrew) WHERE is_homebrew = true;
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_community
        ON np_romdisc_metadata(source_account_id, is_community_rom) WHERE is_community_rom = true;
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_metadata_trgm_title
        ON np_romdisc_metadata USING GIN(rom_title_normalized gin_trgm_ops);

      -- Full-text search trigger
      CREATE OR REPLACE FUNCTION np_romdisc_metadata_search_trigger() RETURNS trigger AS $$
      BEGIN
        NEW.search_vector := to_tsvector('english',
          COALESCE(NEW.rom_title, '') || ' ' ||
          COALESCE(NEW.game_title, '') || ' ' ||
          COALESCE(NEW.platform, '') || ' ' ||
          COALESCE(NEW.publisher, '') || ' ' ||
          COALESCE(NEW.developer, '') || ' ' ||
          COALESCE(NEW.genre, '') || ' ' ||
          COALESCE(NEW.description, '')
        );
        RETURN NEW;
      END;
      $$ LANGUAGE plpgsql;

      DROP TRIGGER IF EXISTS np_romdisc_metadata_search_update ON np_romdisc_metadata;
      CREATE TRIGGER np_romdisc_metadata_search_update
        BEFORE INSERT OR UPDATE ON np_romdisc_metadata
        FOR EACH ROW EXECUTE FUNCTION np_romdisc_metadata_search_trigger();

      -- =====================================================================
      -- Download Queue
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_romdisc_download_queue (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255),
        rom_metadata_id UUID NOT NULL REFERENCES np_romdisc_metadata(id) ON DELETE CASCADE,
        job_id VARCHAR(255),
        status VARCHAR(20) NOT NULL DEFAULT 'pending'
          CHECK (status IN ('pending', 'downloading', 'verifying', 'completed', 'failed', 'cancelled')),
        download_started_at TIMESTAMPTZ,
        download_completed_at TIMESTAMPTZ,
        download_progress_percent INTEGER DEFAULT 0,
        downloaded_bytes BIGINT DEFAULT 0,
        total_bytes BIGINT DEFAULT 0,
        object_storage_path TEXT,
        checksum_verified BOOLEAN DEFAULT false,
        error_message TEXT,
        retry_count INTEGER DEFAULT 0,
        max_retries INTEGER DEFAULT 3,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_romdisc_download_queue_source_app
        ON np_romdisc_download_queue(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_download_queue_status
        ON np_romdisc_download_queue(source_account_id, status);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_download_queue_rom
        ON np_romdisc_download_queue(rom_metadata_id);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_download_queue_created
        ON np_romdisc_download_queue(created_at DESC);

      -- =====================================================================
      -- Scraper Jobs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_romdisc_scraper_jobs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        scraper_name VARCHAR(100) NOT NULL UNIQUE,
        scraper_type VARCHAR(50) NOT NULL,
        scraper_source_url TEXT NOT NULL,
        enabled BOOLEAN DEFAULT true,
        last_run_at TIMESTAMPTZ,
        last_run_status VARCHAR(20),
        last_run_duration_seconds INTEGER,
        roms_found INTEGER DEFAULT 0,
        roms_added INTEGER DEFAULT 0,
        roms_updated INTEGER DEFAULT 0,
        roms_removed INTEGER DEFAULT 0,
        errors TEXT[] DEFAULT '{}',
        cron_schedule VARCHAR(100) NOT NULL DEFAULT '0 3 * * *',
        next_run_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_romdisc_scraper_jobs_name
        ON np_romdisc_scraper_jobs(scraper_name);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_scraper_jobs_enabled
        ON np_romdisc_scraper_jobs(enabled) WHERE enabled = true;

      -- =====================================================================
      -- Popularity Tracking
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_romdisc_popularity (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        rom_metadata_id UUID NOT NULL REFERENCES np_romdisc_metadata(id) ON DELETE CASCADE,
        download_count INTEGER DEFAULT 0,
        search_count INTEGER DEFAULT 0,
        play_count INTEGER DEFAULT 0,
        archive_org_downloads INTEGER DEFAULT 0,
        computed_popularity_score INTEGER DEFAULT 0,
        last_score_update_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(rom_metadata_id)
      );

      CREATE INDEX IF NOT EXISTS idx_np_romdisc_popularity_rom
        ON np_romdisc_popularity(rom_metadata_id);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_popularity_score
        ON np_romdisc_popularity(computed_popularity_score DESC);

      -- =====================================================================
      -- Audit Log
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_romdisc_audit_log (
        id SERIAL PRIMARY KEY,
        source_account_id TEXT NOT NULL,
        user_id TEXT NOT NULL,
        action TEXT NOT NULL,
        rom_metadata_id UUID REFERENCES np_romdisc_metadata(id),
        rom_name TEXT,
        rom_platform TEXT,
        rom_source TEXT,
        ip_address TEXT,
        user_agent TEXT,
        details JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_romdisc_audit_account
        ON np_romdisc_audit_log(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_audit_user
        ON np_romdisc_audit_log(user_id);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_audit_action
        ON np_romdisc_audit_log(action);
      CREATE INDEX IF NOT EXISTS idx_np_romdisc_audit_created
        ON np_romdisc_audit_log(created_at);

      -- =====================================================================
      -- Legal Acceptance
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_romdisc_legal_acceptance (
        id SERIAL PRIMARY KEY,
        source_account_id TEXT NOT NULL,
        user_id TEXT NOT NULL,
        disclaimer_version TEXT NOT NULL DEFAULT '1.0',
        accepted_at TIMESTAMPTZ DEFAULT NOW(),
        ip_address TEXT,
        user_agent TEXT,
        UNIQUE(source_account_id, user_id, disclaimer_version)
      );
    `;

    await this.execute(schema);
    await this.seedScraperJobs();
    logger.info('ROM Discovery schema initialized successfully');
  }

  private async seedScraperJobs(): Promise<void> {
    const scrapers = [
      {
        name: 'no-intro-nes',
        type: 'no-intro',
        url: 'https://datomatic.no-intro.org/index.php?page=download&s=64&op=daily',
        schedule: '0 3 * * 0',
      },
      {
        name: 'no-intro-snes',
        type: 'no-intro',
        url: 'https://datomatic.no-intro.org/index.php?page=download&s=65&op=daily',
        schedule: '0 3 * * 0',
      },
      {
        name: 'no-intro-gba',
        type: 'no-intro',
        url: 'https://datomatic.no-intro.org/index.php?page=download&s=23&op=daily',
        schedule: '0 3 * * 0',
      },
      {
        name: 'no-intro-genesis',
        type: 'no-intro',
        url: 'https://datomatic.no-intro.org/index.php?page=download&s=1&op=daily',
        schedule: '0 3 * * 0',
      },
      {
        name: 'no-intro-n64',
        type: 'no-intro',
        url: 'https://datomatic.no-intro.org/index.php?page=download&s=43&op=daily',
        schedule: '0 3 * * 0',
      },
      {
        name: 'redump-ps1',
        type: 'redump',
        url: 'http://redump.org/downloads/',
        schedule: '0 4 * * 0',
      },
      {
        name: 'archive-org',
        type: 'archive-org',
        url: 'https://archive.org/advancedsearch.php',
        schedule: '0 2 * * *',
      },
      {
        name: 'tecmobowl',
        type: 'web-scraper',
        url: 'https://tecmobowl.org',
        schedule: '0 5 * * 1',
      },
    ];

    for (const scraper of scrapers) {
      await this.execute(
        `INSERT INTO np_romdisc_scraper_jobs (scraper_name, scraper_type, scraper_source_url, cron_schedule, enabled)
         VALUES ($1, $2, $3, $4, true)
         ON CONFLICT (scraper_name) DO NOTHING`,
        [scraper.name, scraper.type, scraper.url, scraper.schedule]
      );
    }

    logger.info('Scraper jobs seeded');
  }

  // =========================================================================
  // ROM Metadata Operations
  // =========================================================================

  async searchRoms(filters: {
    query?: string;
    platform?: string;
    region?: string;
    quality_min?: number;
    popularity_min?: number;
    verified_only?: boolean;
    homebrew_only?: boolean;
    community_only?: boolean;
    show_hacks?: boolean;
    show_translations?: boolean;
    genre?: string;
    sort?: 'popularity' | 'quality' | 'title' | 'year';
    order?: 'asc' | 'desc';
    limit?: number;
    offset?: number;
  }): Promise<{ roms: RomMetadataRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.query) {
      conditions.push(`(search_vector @@ plainto_tsquery('english', $${paramIndex}) OR rom_title_normalized ILIKE $${paramIndex + 1})`);
      values.push(filters.query, `%${filters.query.toLowerCase()}%`);
      paramIndex += 2;
    }

    if (filters.platform) {
      conditions.push(`platform = $${paramIndex}`);
      values.push(filters.platform);
      paramIndex++;
    }

    if (filters.region) {
      conditions.push(`region = $${paramIndex}`);
      values.push(filters.region);
      paramIndex++;
    }

    if (filters.quality_min !== undefined) {
      conditions.push(`quality_score >= $${paramIndex}`);
      values.push(filters.quality_min);
      paramIndex++;
    }

    if (filters.popularity_min !== undefined) {
      conditions.push(`popularity_score >= $${paramIndex}`);
      values.push(filters.popularity_min);
      paramIndex++;
    }

    if (filters.verified_only) {
      conditions.push('is_verified_dump = true');
    }

    if (filters.homebrew_only) {
      conditions.push('is_homebrew = true');
    }

    if (filters.community_only) {
      conditions.push('is_community_rom = true');
    }

    if (!filters.show_hacks) {
      conditions.push('is_hack = false');
    }

    if (!filters.show_translations) {
      conditions.push('is_translation = false');
    }

    if (filters.genre) {
      conditions.push(`genre = $${paramIndex}`);
      values.push(filters.genre);
      paramIndex++;
    }

    conditions.push('download_url_dead = false');

    const whereClause = conditions.join(' AND ');

    // Get total count
    const countResult = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_romdisc_metadata WHERE ${whereClause}`,
      values
    );
    const total = parseInt(countResult.rows[0]?.count ?? '0', 10);

    // Build ORDER BY
    let orderBy: string;
    const direction = filters.order === 'asc' ? 'ASC' : 'DESC';
    switch (filters.sort) {
      case 'quality':
        orderBy = `quality_score ${direction}`;
        break;
      case 'title':
        orderBy = `rom_title_normalized ${direction}`;
        break;
      case 'year':
        orderBy = `release_year ${direction} NULLS LAST`;
        break;
      case 'popularity':
      default:
        orderBy = `popularity_score ${direction}`;
        break;
    }

    const limit = Math.min(filters.limit ?? 50, 200);
    const offset = filters.offset ?? 0;

    const result = await this.query<RomMetadataRecord>(
      `SELECT * FROM np_romdisc_metadata
       WHERE ${whereClause}
       ORDER BY ${orderBy}, rom_title_normalized ASC
       LIMIT ${limit} OFFSET ${offset}`,
      values
    );

    return { roms: result.rows, total };
  }

  async getRomById(id: string): Promise<RomMetadataRecord | null> {
    const result = await this.query<RomMetadataRecord>(
      `SELECT * FROM np_romdisc_metadata WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async upsertRomMetadata(data: Omit<RomMetadataRecord, 'id' | 'created_at' | 'updated_at' | 'search_vector'>): Promise<RomMetadataRecord> {
    const result = await this.query<RomMetadataRecord>(
      `INSERT INTO np_romdisc_metadata (
        source_account_id, rom_title, rom_title_normalized, platform, region,
        file_name, file_size_bytes, file_hash_md5, file_hash_sha256, file_hash_crc32,
        download_url, download_source, download_url_verified_at, download_url_dead,
        release_year, release_month, release_day, version,
        quality_score, popularity_score, release_group,
        is_verified_dump, is_hack, is_translation, is_homebrew, is_public_domain,
        game_title, genre, publisher, developer, description,
        igdb_id, mobygames_id, box_art_url, screenshot_urls,
        is_community_rom, community_source_url, community_update_year,
        scraped_from, scraped_at
      ) VALUES (
        $1, $2, $3, $4, $5,
        $6, $7, $8, $9, $10,
        $11, $12, $13, $14,
        $15, $16, $17, $18,
        $19, $20, $21,
        $22, $23, $24, $25, $26,
        $27, $28, $29, $30, $31,
        $32, $33, $34, $35,
        $36, $37, $38,
        $39, $40
      )
      ON CONFLICT (source_account_id, file_hash_sha256) DO UPDATE SET
        rom_title = EXCLUDED.rom_title,
        rom_title_normalized = EXCLUDED.rom_title_normalized,
        platform = EXCLUDED.platform,
        region = EXCLUDED.region,
        file_name = EXCLUDED.file_name,
        file_size_bytes = COALESCE(EXCLUDED.file_size_bytes, np_romdisc_metadata.file_size_bytes),
        file_hash_md5 = COALESCE(EXCLUDED.file_hash_md5, np_romdisc_metadata.file_hash_md5),
        file_hash_crc32 = COALESCE(EXCLUDED.file_hash_crc32, np_romdisc_metadata.file_hash_crc32),
        download_url = COALESCE(EXCLUDED.download_url, np_romdisc_metadata.download_url),
        download_source = COALESCE(EXCLUDED.download_source, np_romdisc_metadata.download_source),
        download_url_verified_at = COALESCE(EXCLUDED.download_url_verified_at, np_romdisc_metadata.download_url_verified_at),
        download_url_dead = EXCLUDED.download_url_dead,
        release_year = COALESCE(EXCLUDED.release_year, np_romdisc_metadata.release_year),
        version = COALESCE(EXCLUDED.version, np_romdisc_metadata.version),
        quality_score = GREATEST(EXCLUDED.quality_score, np_romdisc_metadata.quality_score),
        popularity_score = GREATEST(EXCLUDED.popularity_score, np_romdisc_metadata.popularity_score),
        release_group = COALESCE(EXCLUDED.release_group, np_romdisc_metadata.release_group),
        is_verified_dump = EXCLUDED.is_verified_dump OR np_romdisc_metadata.is_verified_dump,
        game_title = COALESCE(EXCLUDED.game_title, np_romdisc_metadata.game_title),
        genre = COALESCE(EXCLUDED.genre, np_romdisc_metadata.genre),
        publisher = COALESCE(EXCLUDED.publisher, np_romdisc_metadata.publisher),
        developer = COALESCE(EXCLUDED.developer, np_romdisc_metadata.developer),
        description = COALESCE(EXCLUDED.description, np_romdisc_metadata.description),
        igdb_id = COALESCE(EXCLUDED.igdb_id, np_romdisc_metadata.igdb_id),
        mobygames_id = COALESCE(EXCLUDED.mobygames_id, np_romdisc_metadata.mobygames_id),
        box_art_url = COALESCE(EXCLUDED.box_art_url, np_romdisc_metadata.box_art_url),
        scraped_from = EXCLUDED.scraped_from,
        scraped_at = EXCLUDED.scraped_at,
        updated_at = NOW()
      RETURNING *`,
      [
        data.source_account_id, data.rom_title, data.rom_title_normalized,
        data.platform, data.region,
        data.file_name, data.file_size_bytes, data.file_hash_md5,
        data.file_hash_sha256, data.file_hash_crc32,
        data.download_url, data.download_source, data.download_url_verified_at,
        data.download_url_dead,
        data.release_year, data.release_month, data.release_day, data.version,
        data.quality_score, data.popularity_score, data.release_group,
        data.is_verified_dump, data.is_hack, data.is_translation,
        data.is_homebrew, data.is_public_domain,
        data.game_title, data.genre, data.publisher, data.developer, data.description,
        data.igdb_id, data.mobygames_id, data.box_art_url, data.screenshot_urls,
        data.is_community_rom, data.community_source_url, data.community_update_year,
        data.scraped_from, data.scraped_at,
      ]
    );

    return result.rows[0];
  }

  async getPlatformStats(): Promise<PlatformStats[]> {
    const result = await this.query<{
      platform: string;
      rom_count: string;
      verified_count: string;
      homebrew_count: string;
      avg_quality: string;
    }>(
      `SELECT
        platform,
        COUNT(*) as rom_count,
        COUNT(*) FILTER (WHERE is_verified_dump = true) as verified_count,
        COUNT(*) FILTER (WHERE is_homebrew = true) as homebrew_count,
        ROUND(AVG(quality_score)) as avg_quality
       FROM np_romdisc_metadata
       WHERE source_account_id = $1 AND download_url_dead = false
       GROUP BY platform
       ORDER BY rom_count DESC`,
      [this.sourceAccountId]
    );

    return result.rows.map(row => ({
      platform: row.platform,
      rom_count: parseInt(row.rom_count, 10),
      verified_count: parseInt(row.verified_count, 10),
      homebrew_count: parseInt(row.homebrew_count, 10),
      avg_quality: parseFloat(row.avg_quality) || 0,
    }));
  }

  async getFeaturedRoms(): Promise<FeaturedRoms> {
    const popularResult = await this.query<RomMetadataRecord>(
      `SELECT * FROM np_romdisc_metadata
       WHERE source_account_id = $1 AND download_url_dead = false
       ORDER BY popularity_score DESC, quality_score DESC
       LIMIT 10`,
      [this.sourceAccountId]
    );

    const verifiedResult = await this.query<RomMetadataRecord>(
      `SELECT * FROM np_romdisc_metadata
       WHERE source_account_id = $1 AND is_verified_dump = true AND download_url_dead = false
       ORDER BY quality_score DESC, popularity_score DESC
       LIMIT 10`,
      [this.sourceAccountId]
    );

    const communityResult = await this.query<RomMetadataRecord>(
      `SELECT * FROM np_romdisc_metadata
       WHERE source_account_id = $1 AND is_community_rom = true AND download_url_dead = false
       ORDER BY community_update_year DESC NULLS LAST, quality_score DESC
       LIMIT 10`,
      [this.sourceAccountId]
    );

    const homebrewResult = await this.query<RomMetadataRecord>(
      `SELECT * FROM np_romdisc_metadata
       WHERE source_account_id = $1
         AND (is_homebrew = true OR is_public_domain = true)
         AND download_url_dead = false
       ORDER BY quality_score DESC, popularity_score DESC
       LIMIT 10`,
      [this.sourceAccountId]
    );

    return {
      most_popular: popularResult.rows,
      verified_dumps: verifiedResult.rows,
      community_updated: communityResult.rows,
      legal_homebrew: homebrewResult.rows,
    };
  }

  // =========================================================================
  // Download Queue Operations
  // =========================================================================

  async createDownloadQueueEntry(data: {
    source_account_id: string;
    user_id?: string;
    rom_metadata_id: string;
  }): Promise<DownloadQueueRecord> {
    // Check if there is already a pending/active download for this ROM
    const existing = await this.query<DownloadQueueRecord>(
      `SELECT * FROM np_romdisc_download_queue
       WHERE source_account_id = $1 AND rom_metadata_id = $2
         AND status IN ('pending', 'downloading', 'verifying')
       LIMIT 1`,
      [data.source_account_id, data.rom_metadata_id]
    );

    if (existing.rows.length > 0) {
      return existing.rows[0];
    }

    // Get ROM metadata to know total size
    const rom = await this.query<RomMetadataRecord>(
      `SELECT * FROM np_romdisc_metadata WHERE id = $1`,
      [data.rom_metadata_id]
    );

    const totalBytes = rom.rows[0]?.file_size_bytes ?? 0;

    const result = await this.query<DownloadQueueRecord>(
      `INSERT INTO np_romdisc_download_queue (
        source_account_id, user_id, rom_metadata_id, status, total_bytes
      ) VALUES ($1, $2, $3, 'pending', $4)
      RETURNING *`,
      [data.source_account_id, data.user_id ?? null, data.rom_metadata_id, totalBytes]
    );

    return result.rows[0];
  }

  async updateDownloadQueue(id: string, updates: Partial<DownloadQueueRecord>): Promise<DownloadQueueRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = [
      'status', 'download_started_at', 'download_completed_at',
      'download_progress_percent', 'downloaded_bytes', 'total_bytes',
      'object_storage_path', 'checksum_verified', 'error_message',
      'retry_count', 'job_id',
    ];

    for (const [key, value] of Object.entries(updates)) {
      if (allowedFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (fields.length === 0) {
      const result = await this.query<DownloadQueueRecord>(
        `SELECT * FROM np_romdisc_download_queue WHERE id = $1`,
        [id]
      );
      return result.rows[0] ?? null;
    }

    fields.push('updated_at = NOW()');
    values.push(id);

    const result = await this.query<DownloadQueueRecord>(
      `UPDATE np_romdisc_download_queue
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async getDownloadQueue(sourceAccountId?: string): Promise<DownloadQueueRecord[]> {
    const accountId = sourceAccountId ?? this.sourceAccountId;
    const result = await this.query<DownloadQueueRecord>(
      `SELECT dq.*, rm.rom_title, rm.platform, rm.file_name
       FROM np_romdisc_download_queue dq
       LEFT JOIN np_romdisc_metadata rm ON dq.rom_metadata_id = rm.id
       WHERE dq.source_account_id = $1
       ORDER BY
         CASE dq.status
           WHEN 'downloading' THEN 1
           WHEN 'verifying' THEN 2
           WHEN 'pending' THEN 3
           WHEN 'failed' THEN 4
           WHEN 'completed' THEN 5
           WHEN 'cancelled' THEN 6
         END,
         dq.created_at DESC
       LIMIT 100`,
      [accountId]
    );

    return result.rows;
  }

  async getDownloadById(id: string): Promise<DownloadQueueRecord | null> {
    const result = await this.query<DownloadQueueRecord>(
      `SELECT dq.*, rm.rom_title, rm.platform, rm.file_name
       FROM np_romdisc_download_queue dq
       LEFT JOIN np_romdisc_metadata rm ON dq.rom_metadata_id = rm.id
       WHERE dq.id = $1`,
      [id]
    );
    return result.rows[0] ?? null;
  }

  async cancelDownload(id: string): Promise<boolean> {
    const result = await this.execute(
      `UPDATE np_romdisc_download_queue
       SET status = 'cancelled', updated_at = NOW()
       WHERE id = $1 AND status IN ('pending', 'downloading')`,
      [id]
    );
    return result > 0;
  }

  async getActiveDownloadCount(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_romdisc_download_queue
       WHERE status IN ('downloading', 'verifying')`,
      []
    );
    return parseInt(result.rows[0]?.count ?? '0', 10);
  }

  // =========================================================================
  // Scraper Operations
  // =========================================================================

  async getScrapers(): Promise<ScraperJobRecord[]> {
    const result = await this.query<ScraperJobRecord>(
      `SELECT * FROM np_romdisc_scraper_jobs ORDER BY scraper_name ASC`,
      []
    );
    return result.rows;
  }

  async getScraperByName(name: string): Promise<ScraperJobRecord | null> {
    const result = await this.query<ScraperJobRecord>(
      `SELECT * FROM np_romdisc_scraper_jobs WHERE scraper_name = $1`,
      [name]
    );
    return result.rows[0] ?? null;
  }

  async updateScraper(name: string, updates: {
    enabled?: boolean;
    cron_schedule?: string;
  }): Promise<ScraperJobRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    if (updates.enabled !== undefined) {
      fields.push(`enabled = $${paramIndex}`);
      values.push(updates.enabled);
      paramIndex++;
    }

    if (updates.cron_schedule !== undefined) {
      fields.push(`cron_schedule = $${paramIndex}`);
      values.push(updates.cron_schedule);
      paramIndex++;
    }

    if (fields.length === 0) {
      return this.getScraperByName(name);
    }

    fields.push('updated_at = NOW()');
    values.push(name);

    const result = await this.query<ScraperJobRecord>(
      `UPDATE np_romdisc_scraper_jobs
       SET ${fields.join(', ')}
       WHERE scraper_name = $${paramIndex}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async updateScraperResults(name: string, results: {
    status: string;
    duration_seconds: number;
    roms_found: number;
    roms_added: number;
    roms_updated: number;
    roms_removed: number;
    errors: string[];
  }): Promise<void> {
    await this.execute(
      `UPDATE np_romdisc_scraper_jobs SET
        last_run_at = NOW(),
        last_run_status = $2,
        last_run_duration_seconds = $3,
        roms_found = $4,
        roms_added = $5,
        roms_updated = $6,
        roms_removed = $7,
        errors = $8,
        updated_at = NOW()
       WHERE scraper_name = $1`,
      [
        name, results.status, results.duration_seconds,
        results.roms_found, results.roms_added, results.roms_updated,
        results.roms_removed, results.errors,
      ]
    );
  }

  // =========================================================================
  // Popularity Operations
  // =========================================================================

  async getPopularity(romMetadataId: string): Promise<PopularityTrackingRecord | null> {
    const result = await this.query<PopularityTrackingRecord>(
      `SELECT * FROM np_romdisc_popularity WHERE rom_metadata_id = $1`,
      [romMetadataId]
    );
    return result.rows[0] ?? null;
  }

  async updatePopularity(romMetadataId: string, updates: Partial<PopularityTrackingRecord>): Promise<PopularityTrackingRecord> {
    const result = await this.query<PopularityTrackingRecord>(
      `INSERT INTO np_romdisc_popularity (rom_metadata_id, download_count, search_count, play_count, archive_org_downloads, computed_popularity_score, last_score_update_at)
       VALUES ($1, $2, $3, $4, $5, $6, NOW())
       ON CONFLICT (rom_metadata_id) DO UPDATE SET
         download_count = COALESCE($2, np_romdisc_popularity.download_count),
         search_count = COALESCE($3, np_romdisc_popularity.search_count),
         play_count = COALESCE($4, np_romdisc_popularity.play_count),
         archive_org_downloads = COALESCE($5, np_romdisc_popularity.archive_org_downloads),
         computed_popularity_score = COALESCE($6, np_romdisc_popularity.computed_popularity_score),
         last_score_update_at = NOW(),
         updated_at = NOW()
       RETURNING *`,
      [
        romMetadataId,
        updates.download_count ?? 0,
        updates.search_count ?? 0,
        updates.play_count ?? 0,
        updates.archive_org_downloads ?? 0,
        updates.computed_popularity_score ?? 0,
      ]
    );

    return result.rows[0];
  }

  async incrementDownloadCount(romMetadataId: string): Promise<void> {
    await this.execute(
      `INSERT INTO np_romdisc_popularity (rom_metadata_id, download_count)
       VALUES ($1, 1)
       ON CONFLICT (rom_metadata_id) DO UPDATE SET
         download_count = np_romdisc_popularity.download_count + 1,
         updated_at = NOW()`,
      [romMetadataId]
    );

    // Also update the main metadata popularity score
    await this.execute(
      `UPDATE np_romdisc_metadata SET
        popularity_score = COALESCE(
          (SELECT computed_popularity_score FROM np_romdisc_popularity WHERE rom_metadata_id = $1),
          popularity_score
        ),
        updated_at = NOW()
       WHERE id = $1`,
      [romMetadataId]
    );
  }

  async incrementSearchCount(romMetadataId: string): Promise<void> {
    await this.execute(
      `INSERT INTO np_romdisc_popularity (rom_metadata_id, search_count)
       VALUES ($1, 1)
       ON CONFLICT (rom_metadata_id) DO UPDATE SET
         search_count = np_romdisc_popularity.search_count + 1,
         updated_at = NOW()`,
      [romMetadataId]
    );
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<RomDiscoveryStats> {
    const result = await this.query<{
      total_roms: string;
      total_platforms: string;
      total_verified: string;
      total_homebrew: string;
      total_community: string;
      total_downloads_queued: string;
      total_downloads_completed: string;
      active_scrapers: string;
      avg_quality_score: string;
      avg_popularity_score: string;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM np_romdisc_metadata WHERE source_account_id = $1) as total_roms,
        (SELECT COUNT(DISTINCT platform) FROM np_romdisc_metadata WHERE source_account_id = $1) as total_platforms,
        (SELECT COUNT(*) FROM np_romdisc_metadata WHERE source_account_id = $1 AND is_verified_dump = true) as total_verified,
        (SELECT COUNT(*) FROM np_romdisc_metadata WHERE source_account_id = $1 AND is_homebrew = true) as total_homebrew,
        (SELECT COUNT(*) FROM np_romdisc_metadata WHERE source_account_id = $1 AND is_community_rom = true) as total_community,
        (SELECT COUNT(*) FROM np_romdisc_download_queue WHERE source_account_id = $1) as total_downloads_queued,
        (SELECT COUNT(*) FROM np_romdisc_download_queue WHERE source_account_id = $1 AND status = 'completed') as total_downloads_completed,
        (SELECT COUNT(*) FROM np_romdisc_scraper_jobs WHERE enabled = true) as active_scrapers,
        (SELECT COALESCE(ROUND(AVG(quality_score)), 0) FROM np_romdisc_metadata WHERE source_account_id = $1) as avg_quality_score,
        (SELECT COALESCE(ROUND(AVG(popularity_score)), 0) FROM np_romdisc_metadata WHERE source_account_id = $1) as avg_popularity_score`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      total_roms: parseInt(row.total_roms, 10),
      total_platforms: parseInt(row.total_platforms, 10),
      total_verified: parseInt(row.total_verified, 10),
      total_homebrew: parseInt(row.total_homebrew, 10),
      total_community: parseInt(row.total_community, 10),
      total_downloads_queued: parseInt(row.total_downloads_queued, 10),
      total_downloads_completed: parseInt(row.total_downloads_completed, 10),
      active_scrapers: parseInt(row.active_scrapers, 10),
      avg_quality_score: parseFloat(row.avg_quality_score) || 0,
      avg_popularity_score: parseFloat(row.avg_popularity_score) || 0,
    };
  }

  // =========================================================================
  // Legal Acceptance Operations
  // =========================================================================

  async hasAcceptedDisclaimer(userId: string, disclaimerVersion: string): Promise<boolean> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_romdisc_legal_acceptance
       WHERE source_account_id = $1 AND user_id = $2 AND disclaimer_version = $3`,
      [this.sourceAccountId, userId, disclaimerVersion]
    );
    return parseInt(result.rows[0]?.count ?? '0', 10) > 0;
  }

  async recordLegalAcceptance(data: {
    source_account_id: string;
    user_id: string;
    disclaimer_version: string;
    ip_address?: string;
    user_agent?: string;
  }): Promise<void> {
    await this.execute(
      `INSERT INTO np_romdisc_legal_acceptance (source_account_id, user_id, disclaimer_version, ip_address, user_agent)
       VALUES ($1, $2, $3, $4, $5)
       ON CONFLICT (source_account_id, user_id, disclaimer_version) DO NOTHING`,
      [data.source_account_id, data.user_id, data.disclaimer_version, data.ip_address ?? null, data.user_agent ?? null]
    );
  }

  // =========================================================================
  // Audit Log Operations
  // =========================================================================

  async addAuditLog(data: {
    source_account_id: string;
    user_id: string;
    action: string;
    rom_metadata_id?: string;
    rom_name?: string;
    rom_platform?: string;
    rom_source?: string;
    ip_address?: string;
    user_agent?: string;
    details?: Record<string, unknown>;
  }): Promise<void> {
    await this.execute(
      `INSERT INTO np_romdisc_audit_log (
        source_account_id, user_id, action, rom_metadata_id,
        rom_name, rom_platform, rom_source,
        ip_address, user_agent, details
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
      [
        data.source_account_id,
        data.user_id,
        data.action,
        data.rom_metadata_id ?? null,
        data.rom_name ?? null,
        data.rom_platform ?? null,
        data.rom_source ?? null,
        data.ip_address ?? null,
        data.user_agent ?? null,
        JSON.stringify(data.details ?? {}),
      ]
    );
  }

  async getAuditLog(filters: {
    user_id?: string;
    action?: string;
    from_date?: Date;
    to_date?: Date;
    limit?: number;
    offset?: number;
  }): Promise<{ entries: AuditLogRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.user_id) {
      conditions.push(`user_id = $${paramIndex}`);
      values.push(filters.user_id);
      paramIndex++;
    }

    if (filters.action) {
      conditions.push(`action = $${paramIndex}`);
      values.push(filters.action);
      paramIndex++;
    }

    if (filters.from_date) {
      conditions.push(`created_at >= $${paramIndex}`);
      values.push(filters.from_date);
      paramIndex++;
    }

    if (filters.to_date) {
      conditions.push(`created_at <= $${paramIndex}`);
      values.push(filters.to_date);
      paramIndex++;
    }

    const whereClause = conditions.join(' AND ');

    const countResult = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_romdisc_audit_log WHERE ${whereClause}`,
      values
    );
    const total = parseInt(countResult.rows[0]?.count ?? '0', 10);

    const limit = Math.min(filters.limit ?? 50, 500);
    const offset = filters.offset ?? 0;

    const result = await this.query<AuditLogRecord>(
      `SELECT * FROM np_romdisc_audit_log
       WHERE ${whereClause}
       ORDER BY created_at DESC
       LIMIT ${limit} OFFSET ${offset}`,
      values
    );

    return { entries: result.rows, total };
  }

  async exportAuditLog(filters: {
    from_date?: Date;
    to_date?: Date;
    user_id?: string;
    limit?: number;
  }): Promise<AuditLogRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.from_date) {
      conditions.push(`created_at >= $${paramIndex}`);
      values.push(filters.from_date);
      paramIndex++;
    }

    if (filters.to_date) {
      conditions.push(`created_at <= $${paramIndex}`);
      values.push(filters.to_date);
      paramIndex++;
    }

    if (filters.user_id) {
      conditions.push(`user_id = $${paramIndex}`);
      values.push(filters.user_id);
      paramIndex++;
    }

    const whereClause = conditions.join(' AND ');
    const limitClause = filters.limit ? ` LIMIT ${filters.limit}` : '';

    const result = await this.query<AuditLogRecord>(
      `SELECT * FROM np_romdisc_audit_log
       WHERE ${whereClause}
       ORDER BY created_at DESC${limitClause}`,
      values
    );

    return result.rows;
  }
}
