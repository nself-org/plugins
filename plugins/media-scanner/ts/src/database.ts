/**
 * Media Scanner Database
 * Schema initialization, CRUD operations, and statistics
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  ScanRecord,
  ScanState,
  ScanError,
  MediaFileRecord,
  DiscoveredFile,
  ParsedFilename,
  MediaInfo,
  MatchResult,
  LibraryStats,
} from './types.js';

const logger = createLogger('media-scanner:database');

const SCHEMA_SQL = `
  CREATE TABLE IF NOT EXISTS np_mscan_scans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
    paths TEXT[] NOT NULL,
    recursive BOOLEAN DEFAULT TRUE,
    state VARCHAR(50) NOT NULL DEFAULT 'pending',
    files_found INTEGER DEFAULT 0,
    files_processed INTEGER DEFAULT 0,
    errors JSONB DEFAULT '[]',
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
  );

  CREATE INDEX IF NOT EXISTS idx_np_mscan_scans_state
    ON np_mscan_scans(state);
  CREATE INDEX IF NOT EXISTS idx_np_mscan_scans_source_account
    ON np_mscan_scans(source_account_id);
  CREATE INDEX IF NOT EXISTS idx_np_mscan_scans_created
    ON np_mscan_scans(created_at DESC);

  CREATE TABLE IF NOT EXISTS np_mscan_media_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
    scan_id UUID REFERENCES np_mscan_scans(id) ON DELETE SET NULL,
    file_path TEXT NOT NULL UNIQUE,
    filename TEXT NOT NULL,
    file_size BIGINT NOT NULL,
    modified_at TIMESTAMPTZ,
    parsed_title TEXT,
    parsed_year INTEGER,
    parsed_season INTEGER,
    parsed_episode INTEGER,
    parsed_quality VARCHAR(50),
    parsed_resolution VARCHAR(20),
    parsed_codec VARCHAR(50),
    parsed_group VARCHAR(100),
    duration_seconds REAL,
    video_codec VARCHAR(50),
    video_resolution VARCHAR(20),
    video_bitrate INTEGER,
    audio_tracks INTEGER DEFAULT 0,
    audio_languages TEXT[] DEFAULT '{}',
    subtitle_tracks INTEGER DEFAULT 0,
    subtitle_languages TEXT[] DEFAULT '{}',
    match_provider VARCHAR(50),
    match_id VARCHAR(255),
    match_title TEXT,
    match_confidence REAL,
    indexed BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    synced_at TIMESTAMPTZ DEFAULT NOW()
  );

  CREATE INDEX IF NOT EXISTS idx_np_mscan_media_files_scan_id
    ON np_mscan_media_files(scan_id);
  CREATE INDEX IF NOT EXISTS idx_np_mscan_media_files_source_account
    ON np_mscan_media_files(source_account_id);
  CREATE INDEX IF NOT EXISTS idx_np_mscan_media_files_file_path
    ON np_mscan_media_files(file_path);
  CREATE INDEX IF NOT EXISTS idx_np_mscan_media_files_parsed_title
    ON np_mscan_media_files(parsed_title);
  CREATE INDEX IF NOT EXISTS idx_np_mscan_media_files_indexed
    ON np_mscan_media_files(indexed);
  CREATE INDEX IF NOT EXISTS idx_np_mscan_media_files_match_confidence
    ON np_mscan_media_files(match_confidence);
  CREATE INDEX IF NOT EXISTS idx_np_mscan_media_files_created
    ON np_mscan_media_files(created_at DESC);
`;

export class MediaScannerDatabase {
  private db: Database;
  private sourceAccountId: string;

  constructor(sourceAccountId = 'primary') {
    this.db = createDatabase();
    this.sourceAccountId = sourceAccountId;
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  async query<T extends Record<string, unknown>>(text: string, params?: unknown[]) {
    return this.db.query<T>(text, params);
  }

  async initializeSchema(): Promise<void> {
    await this.db.query(SCHEMA_SQL);
    logger.info('Database schema initialized');
  }

  /**
   * Return a new instance scoped to a different source_account_id,
   * reusing the same underlying connection pool.
   */
  forSourceAccount(accountId: string): MediaScannerDatabase {
    const instance = Object.create(this) as MediaScannerDatabase;
    instance.sourceAccountId = accountId;
    return instance;
  }

  // ─── Scan CRUD ──────────────────────────────────────────────────────────

  async createScan(paths: string[], recursive: boolean): Promise<ScanRecord> {
    const result = await this.db.queryOne<ScanRecord>(
      `INSERT INTO np_mscan_scans (source_account_id, paths, recursive, state)
       VALUES ($1, $2, $3, 'pending')
       RETURNING *`,
      [this.sourceAccountId, paths, recursive]
    );
    if (!result) {
      throw new Error('Failed to create scan record');
    }
    return result;
  }

  async getScan(id: string): Promise<ScanRecord | null> {
    return this.db.queryOne<ScanRecord>(
      `SELECT * FROM np_mscan_scans WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async updateScanState(
    id: string,
    state: ScanState,
    updates?: {
      files_found?: number;
      files_processed?: number;
      errors?: ScanError[];
      started_at?: Date;
      completed_at?: Date;
    }
  ): Promise<void> {
    const setClauses = ['state = $3', 'updated_at = NOW()'];
    const params: unknown[] = [id, this.sourceAccountId, state];
    let paramIndex = 4;

    if (updates?.files_found !== undefined) {
      setClauses.push(`files_found = $${paramIndex}`);
      params.push(updates.files_found);
      paramIndex++;
    }
    if (updates?.files_processed !== undefined) {
      setClauses.push(`files_processed = $${paramIndex}`);
      params.push(updates.files_processed);
      paramIndex++;
    }
    if (updates?.errors !== undefined) {
      setClauses.push(`errors = $${paramIndex}`);
      params.push(JSON.stringify(updates.errors));
      paramIndex++;
    }
    if (updates?.started_at !== undefined) {
      setClauses.push(`started_at = $${paramIndex}`);
      params.push(updates.started_at);
      paramIndex++;
    }
    if (updates?.completed_at !== undefined) {
      setClauses.push(`completed_at = $${paramIndex}`);
      params.push(updates.completed_at);
      paramIndex++;
    }

    await this.db.execute(
      `UPDATE np_mscan_scans SET ${setClauses.join(', ')} WHERE id = $1 AND source_account_id = $2`,
      params
    );
  }

  async incrementScanProcessed(id: string): Promise<void> {
    await this.db.execute(
      `UPDATE np_mscan_scans
       SET files_processed = files_processed + 1, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async appendScanError(id: string, error: ScanError): Promise<void> {
    await this.db.execute(
      `UPDATE np_mscan_scans
       SET errors = errors || $3::jsonb, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId, JSON.stringify(error)]
    );
  }

  async listScans(limit = 20, offset = 0): Promise<ScanRecord[]> {
    const result = await this.db.query<ScanRecord>(
      `SELECT * FROM np_mscan_scans
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  // ─── Media File CRUD ────────────────────────────────────────────────────

  async upsertMediaFile(
    scanId: string | null,
    file: DiscoveredFile,
    parsed: ParsedFilename
  ): Promise<MediaFileRecord> {
    const result = await this.db.queryOne<MediaFileRecord>(
      `INSERT INTO np_mscan_media_files (
        source_account_id, scan_id, file_path, filename, file_size, modified_at,
        parsed_title, parsed_year, parsed_season, parsed_episode,
        parsed_quality, parsed_resolution, parsed_codec, parsed_group
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      ON CONFLICT (file_path) DO UPDATE SET
        scan_id = EXCLUDED.scan_id,
        filename = EXCLUDED.filename,
        file_size = EXCLUDED.file_size,
        modified_at = EXCLUDED.modified_at,
        parsed_title = EXCLUDED.parsed_title,
        parsed_year = EXCLUDED.parsed_year,
        parsed_season = EXCLUDED.parsed_season,
        parsed_episode = EXCLUDED.parsed_episode,
        parsed_quality = EXCLUDED.parsed_quality,
        parsed_resolution = EXCLUDED.parsed_resolution,
        parsed_codec = EXCLUDED.parsed_codec,
        parsed_group = EXCLUDED.parsed_group,
        updated_at = NOW(),
        synced_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        scanId,
        file.path,
        file.filename,
        file.size,
        file.modified_at,
        parsed.title,
        parsed.year,
        parsed.season,
        parsed.episode,
        parsed.quality,
        parsed.resolution,
        parsed.codec,
        parsed.group,
      ]
    );
    if (!result) {
      throw new Error(`Failed to upsert media file: ${file.path}`);
    }
    return result;
  }

  async updateMediaProbe(fileId: string, info: MediaInfo): Promise<void> {
    await this.db.execute(
      `UPDATE np_mscan_media_files SET
        duration_seconds = $2,
        video_codec = $3,
        video_resolution = $4,
        video_bitrate = $5,
        audio_tracks = $6,
        audio_languages = $7,
        subtitle_tracks = $8,
        subtitle_languages = $9,
        updated_at = NOW(),
        synced_at = NOW()
      WHERE id = $1`,
      [
        fileId,
        info.duration_seconds,
        info.video_codec,
        info.video_resolution,
        info.video_bitrate,
        info.audio_tracks,
        info.audio_languages,
        info.subtitle_tracks,
        info.subtitle_languages,
      ]
    );
  }

  async updateMediaMatch(fileId: string, match: MatchResult): Promise<void> {
    await this.db.execute(
      `UPDATE np_mscan_media_files SET
        match_provider = $2,
        match_id = $3,
        match_title = $4,
        match_confidence = $5,
        updated_at = NOW(),
        synced_at = NOW()
      WHERE id = $1`,
      [
        fileId,
        match.provider,
        match.id,
        match.title,
        match.confidence,
      ]
    );
  }

  async setMediaIndexed(fileId: string, indexed: boolean): Promise<void> {
    await this.db.execute(
      `UPDATE np_mscan_media_files SET indexed = $2, updated_at = NOW(), synced_at = NOW() WHERE id = $1`,
      [fileId, indexed]
    );
  }

  async getMediaFile(id: string): Promise<MediaFileRecord | null> {
    return this.db.queryOne<MediaFileRecord>(
      `SELECT * FROM np_mscan_media_files WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async getMediaFileByPath(filePath: string): Promise<MediaFileRecord | null> {
    return this.db.queryOne<MediaFileRecord>(
      `SELECT * FROM np_mscan_media_files WHERE file_path = $1 AND source_account_id = $2`,
      [filePath, this.sourceAccountId]
    );
  }

  async listMediaFiles(limit = 100, offset = 0): Promise<MediaFileRecord[]> {
    const result = await this.db.query<MediaFileRecord>(
      `SELECT * FROM np_mscan_media_files
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async listUnprobed(limit = 100): Promise<MediaFileRecord[]> {
    const result = await this.db.query<MediaFileRecord>(
      `SELECT * FROM np_mscan_media_files
       WHERE source_account_id = $1 AND duration_seconds IS NULL
       ORDER BY created_at ASC
       LIMIT $2`,
      [this.sourceAccountId, limit]
    );
    return result.rows;
  }

  async listUnmatched(limit = 100): Promise<MediaFileRecord[]> {
    const result = await this.db.query<MediaFileRecord>(
      `SELECT * FROM np_mscan_media_files
       WHERE source_account_id = $1 AND match_provider IS NULL AND parsed_title IS NOT NULL
       ORDER BY created_at ASC
       LIMIT $2`,
      [this.sourceAccountId, limit]
    );
    return result.rows;
  }

  async listUnindexed(limit = 100): Promise<MediaFileRecord[]> {
    const result = await this.db.query<MediaFileRecord>(
      `SELECT * FROM np_mscan_media_files
       WHERE source_account_id = $1 AND indexed = FALSE AND match_provider IS NOT NULL
       ORDER BY created_at ASC
       LIMIT $2`,
      [this.sourceAccountId, limit]
    );
    return result.rows;
  }

  async countMediaFiles(): Promise<number> {
    return this.db.countScoped('np_mscan_media_files', this.sourceAccountId);
  }

  // ─── Statistics ─────────────────────────────────────────────────────────

  async getStats(): Promise<LibraryStats> {
    const totalResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_mscan_media_files WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const moviesResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_mscan_media_files
       WHERE source_account_id = $1 AND parsed_season IS NULL`,
      [this.sourceAccountId]
    );

    const tvResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_mscan_media_files
       WHERE source_account_id = $1 AND parsed_season IS NOT NULL`,
      [this.sourceAccountId]
    );

    const sizeResult = await this.db.queryOne<{ total: string | null }>(
      `SELECT SUM(file_size) as total FROM np_mscan_media_files WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const lastScanResult = await this.db.queryOne<{ last: Date | null }>(
      `SELECT MAX(completed_at) as last FROM np_mscan_scans
       WHERE source_account_id = $1 AND state = 'completed'`,
      [this.sourceAccountId]
    );

    const indexedResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_mscan_media_files
       WHERE source_account_id = $1 AND indexed = TRUE`,
      [this.sourceAccountId]
    );

    const matchedResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_mscan_media_files
       WHERE source_account_id = $1 AND match_provider IS NOT NULL`,
      [this.sourceAccountId]
    );

    const unmatchedResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_mscan_media_files
       WHERE source_account_id = $1 AND match_provider IS NULL AND parsed_title IS NOT NULL`,
      [this.sourceAccountId]
    );

    const totalBytes = parseInt(sizeResult?.total ?? '0', 10);

    return {
      total_items: parseInt(totalResult?.count ?? '0', 10),
      movies: parseInt(moviesResult?.count ?? '0', 10),
      tv_shows: parseInt(tvResult?.count ?? '0', 10),
      total_size_gb: parseFloat((totalBytes / (1024 * 1024 * 1024)).toFixed(2)),
      last_scan: lastScanResult?.last ?? null,
      indexed_count: parseInt(indexedResult?.count ?? '0', 10),
      matched_count: parseInt(matchedResult?.count ?? '0', 10),
      unmatched_count: parseInt(unmatchedResult?.count ?? '0', 10),
    };
  }
}
