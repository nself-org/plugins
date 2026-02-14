/**
 * DLNA Plugin Database
 * Schema initialization and CRUD operations for media items and renderers
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type { MediaItemRecord, RendererRecord, ObjectType } from './types.js';

const logger = createLogger('dlna:database');

const SCHEMA_SQL = `
  CREATE TABLE IF NOT EXISTS np_dlna_media_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
    parent_id UUID REFERENCES np_dlna_media_items(id) ON DELETE CASCADE,
    object_type VARCHAR(20) NOT NULL DEFAULT 'item',
    upnp_class TEXT NOT NULL,
    title TEXT NOT NULL,
    file_path TEXT,
    file_size BIGINT,
    mime_type VARCHAR(100),
    duration_seconds INTEGER,
    resolution VARCHAR(20),
    bitrate INTEGER,
    album TEXT,
    artist TEXT,
    genre TEXT,
    thumbnail_path TEXT,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    synced_at TIMESTAMPTZ DEFAULT NOW()
  );

  CREATE TABLE IF NOT EXISTS np_dlna_renderers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
    usn TEXT NOT NULL UNIQUE,
    friendly_name TEXT,
    location TEXT NOT NULL,
    ip_address VARCHAR(45),
    device_type TEXT,
    manufacturer TEXT,
    model_name TEXT,
    last_seen_at TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
  );

  CREATE INDEX IF NOT EXISTS idx_np_dlna_media_parent
    ON np_dlna_media_items(parent_id);

  CREATE INDEX IF NOT EXISTS idx_np_dlna_media_type
    ON np_dlna_media_items(source_account_id, object_type);

  CREATE UNIQUE INDEX IF NOT EXISTS idx_np_dlna_media_file_path
    ON np_dlna_media_items(file_path) WHERE file_path IS NOT NULL;

  CREATE INDEX IF NOT EXISTS idx_np_dlna_media_upnp_class
    ON np_dlna_media_items(upnp_class);

  CREATE INDEX IF NOT EXISTS idx_np_dlna_renderers_seen
    ON np_dlna_renderers(last_seen_at);

  CREATE INDEX IF NOT EXISTS idx_np_dlna_renderers_usn
    ON np_dlna_renderers(usn);
`;

export class DlnaDatabase {
  private db: Database;
  private sourceAccountId: string;

  constructor(sourceAccountId = 'primary') {
    this.db = createDatabase();
    this.sourceAccountId = sourceAccountId;
  }

  async connect(): Promise<void> {
    await this.db.connect();
    logger.info('Database connected');
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
    logger.info('Database disconnected');
  }

  async initializeSchema(): Promise<void> {
    await this.db.query(SCHEMA_SQL);
    logger.info('Database schema initialized');
  }

  /**
   * Create a scoped instance for a specific source account
   */
  forSourceAccount(sourceAccountId: string): DlnaDatabase {
    const scoped = new DlnaDatabase(sourceAccountId);
    scoped.db = this.db;
    return scoped;
  }

  // ---------------------------------------------------------------------------
  // Raw query access (for health checks etc.)
  // ---------------------------------------------------------------------------

  async query<T extends Record<string, unknown>>(text: string, params?: unknown[]): Promise<{ rows: T[]; rowCount: number | null }> {
    return this.db.query<T & Record<string, unknown>>(text, params);
  }

  // ---------------------------------------------------------------------------
  // Media Items - CRUD
  // ---------------------------------------------------------------------------

  /**
   * Upsert a media item by file_path (for scanned files) or by id (for containers)
   */
  async upsertMediaItem(item: Omit<MediaItemRecord, 'id' | 'created_at' | 'updated_at' | 'synced_at'> & { id?: string }): Promise<string> {
    // For items with file_path, use file_path as conflict target
    if (item.file_path) {
      const result = await this.db.query<{ id: string }>(
        `INSERT INTO np_dlna_media_items (
          source_account_id, parent_id, object_type, upnp_class, title,
          file_path, file_size, mime_type, duration_seconds, resolution,
          bitrate, album, artist, genre, thumbnail_path, sort_order, synced_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW())
        ON CONFLICT (file_path) WHERE file_path IS NOT NULL
        DO UPDATE SET
          parent_id = EXCLUDED.parent_id,
          object_type = EXCLUDED.object_type,
          upnp_class = EXCLUDED.upnp_class,
          title = EXCLUDED.title,
          file_size = EXCLUDED.file_size,
          mime_type = EXCLUDED.mime_type,
          duration_seconds = EXCLUDED.duration_seconds,
          resolution = EXCLUDED.resolution,
          bitrate = EXCLUDED.bitrate,
          album = EXCLUDED.album,
          artist = EXCLUDED.artist,
          genre = EXCLUDED.genre,
          thumbnail_path = EXCLUDED.thumbnail_path,
          sort_order = EXCLUDED.sort_order,
          updated_at = NOW(),
          synced_at = NOW()
        RETURNING id`,
        [
          item.source_account_id, item.parent_id, item.object_type, item.upnp_class,
          item.title, item.file_path, item.file_size, item.mime_type,
          item.duration_seconds, item.resolution, item.bitrate, item.album,
          item.artist, item.genre, item.thumbnail_path, item.sort_order,
        ]
      );
      return result.rows[0].id;
    }

    // For containers (no file_path), insert or update by id
    if (item.id) {
      const result = await this.db.query<{ id: string }>(
        `INSERT INTO np_dlna_media_items (
          id, source_account_id, parent_id, object_type, upnp_class, title,
          file_path, file_size, mime_type, sort_order, synced_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
        ON CONFLICT (id) DO UPDATE SET
          title = EXCLUDED.title,
          upnp_class = EXCLUDED.upnp_class,
          sort_order = EXCLUDED.sort_order,
          updated_at = NOW(),
          synced_at = NOW()
        RETURNING id`,
        [
          item.id, item.source_account_id, item.parent_id, item.object_type,
          item.upnp_class, item.title, item.file_path, item.file_size,
          item.mime_type, item.sort_order,
        ]
      );
      return result.rows[0].id;
    }

    // New container without predetermined id
    const result = await this.db.query<{ id: string }>(
      `INSERT INTO np_dlna_media_items (
        source_account_id, parent_id, object_type, upnp_class, title,
        file_path, file_size, mime_type, sort_order, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
      RETURNING id`,
      [
        item.source_account_id, item.parent_id, item.object_type,
        item.upnp_class, item.title, item.file_path, item.file_size,
        item.mime_type, item.sort_order,
      ]
    );
    return result.rows[0].id;
  }

  /**
   * Get a single media item by id
   */
  async getMediaItem(id: string): Promise<MediaItemRecord | null> {
    const result = await this.db.query<MediaItemRecord>(
      `SELECT * FROM np_dlna_media_items WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  /**
   * Get a media item by file path
   */
  async getMediaItemByPath(filePath: string): Promise<MediaItemRecord | null> {
    const result = await this.db.query<MediaItemRecord>(
      `SELECT * FROM np_dlna_media_items WHERE file_path = $1 AND source_account_id = $2`,
      [filePath, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  /**
   * List children of a container (for Browse action)
   */
  async listChildren(
    parentId: string | null,
    startingIndex: number,
    requestedCount: number
  ): Promise<{ items: MediaItemRecord[]; totalCount: number }> {
    const whereClause = parentId
      ? `parent_id = $1 AND source_account_id = $2`
      : `parent_id IS NULL AND source_account_id = $1`;
    const params = parentId
      ? [parentId, this.sourceAccountId]
      : [this.sourceAccountId];

    const countResult = await this.db.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_dlna_media_items WHERE ${whereClause}`,
      params
    );
    const totalCount = parseInt(countResult.rows[0].count, 10);

    const effectiveCount = requestedCount === 0 ? totalCount : requestedCount;

    const listParams = parentId
      ? [parentId, this.sourceAccountId, effectiveCount, startingIndex]
      : [this.sourceAccountId, effectiveCount, startingIndex];
    const offsetIndex = parentId ? 3 : 2;

    const result = await this.db.query<MediaItemRecord>(
      `SELECT * FROM np_dlna_media_items
       WHERE ${whereClause}
       ORDER BY object_type DESC, sort_order ASC, title ASC
       LIMIT $${offsetIndex} OFFSET $${offsetIndex + 1}`,
      listParams
    );

    return { items: result.rows, totalCount };
  }

  /**
   * Get the child count for a container
   */
  async getChildCount(parentId: string): Promise<number> {
    const result = await this.db.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_dlna_media_items
       WHERE parent_id = $1 AND source_account_id = $2`,
      [parentId, this.sourceAccountId]
    );
    return parseInt(result.rows[0].count, 10);
  }

  /**
   * Search media items by title/artist/album
   */
  async searchMediaItems(
    searchCriteria: string,
    startingIndex: number,
    requestedCount: number
  ): Promise<{ items: MediaItemRecord[]; totalCount: number }> {
    // Parse simple search criteria (UPnP search syntax subset)
    // Supports: dc:title contains "query", upnp:class derivedfrom "object.item.videoItem"
    const searchTerm = extractSearchTerm(searchCriteria);
    const classFilter = extractClassFilter(searchCriteria);

    let whereClause = 'source_account_id = $1';
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (searchTerm) {
      whereClause += ` AND (title ILIKE $${paramIndex} OR artist ILIKE $${paramIndex} OR album ILIKE $${paramIndex})`;
      params.push(`%${searchTerm}%`);
      paramIndex++;
    }

    if (classFilter) {
      whereClause += ` AND upnp_class LIKE $${paramIndex}`;
      params.push(`${classFilter}%`);
      paramIndex++;
    }

    const countResult = await this.db.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_dlna_media_items WHERE ${whereClause}`,
      params
    );
    const totalCount = parseInt(countResult.rows[0].count, 10);

    const effectiveCount = requestedCount === 0 ? totalCount : requestedCount;
    params.push(effectiveCount, startingIndex);

    const result = await this.db.query<MediaItemRecord>(
      `SELECT * FROM np_dlna_media_items
       WHERE ${whereClause}
       ORDER BY title ASC
       LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`,
      params
    );

    return { items: result.rows, totalCount };
  }

  /**
   * List all media items with pagination
   */
  async listMediaItems(limit = 100, offset = 0): Promise<MediaItemRecord[]> {
    const result = await this.db.query<MediaItemRecord>(
      `SELECT * FROM np_dlna_media_items
       WHERE source_account_id = $1
       ORDER BY object_type DESC, sort_order ASC, title ASC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  /**
   * Count media items
   */
  async countMediaItems(objectType?: ObjectType): Promise<number> {
    if (objectType) {
      return this.db.countScoped('np_dlna_media_items', this.sourceAccountId, 'object_type = $1', [objectType]);
    }
    return this.db.countScoped('np_dlna_media_items', this.sourceAccountId);
  }

  /**
   * Delete media items whose file_path is not in the given set.
   * Used during scan to remove files that no longer exist.
   */
  async removeStaleItems(validPaths: Set<string>): Promise<number> {
    if (validPaths.size === 0) {
      // Remove all file-backed items
      const result = await this.db.execute(
        `DELETE FROM np_dlna_media_items
         WHERE source_account_id = $1 AND file_path IS NOT NULL`,
        [this.sourceAccountId]
      );
      return result;
    }

    const pathArray = Array.from(validPaths);
    const placeholders = pathArray.map((_, i) => `$${i + 2}`).join(', ');

    const result = await this.db.execute(
      `DELETE FROM np_dlna_media_items
       WHERE source_account_id = $1
         AND file_path IS NOT NULL
         AND file_path NOT IN (${placeholders})`,
      [this.sourceAccountId, ...pathArray]
    );
    return result;
  }

  /**
   * Delete all containers for this source account (to rebuild)
   */
  async removeContainers(): Promise<number> {
    return this.db.execute(
      `DELETE FROM np_dlna_media_items
       WHERE source_account_id = $1 AND object_type = 'container'`,
      [this.sourceAccountId]
    );
  }

  // ---------------------------------------------------------------------------
  // Renderers - CRUD
  // ---------------------------------------------------------------------------

  /**
   * Upsert a discovered renderer by USN
   */
  async upsertRenderer(renderer: Omit<RendererRecord, 'id' | 'created_at' | 'updated_at'>): Promise<string> {
    const result = await this.db.query<{ id: string }>(
      `INSERT INTO np_dlna_renderers (
        source_account_id, usn, friendly_name, location, ip_address,
        device_type, manufacturer, model_name, last_seen_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
      ON CONFLICT (usn) DO UPDATE SET
        friendly_name = COALESCE(EXCLUDED.friendly_name, np_dlna_renderers.friendly_name),
        location = EXCLUDED.location,
        ip_address = EXCLUDED.ip_address,
        device_type = COALESCE(EXCLUDED.device_type, np_dlna_renderers.device_type),
        manufacturer = COALESCE(EXCLUDED.manufacturer, np_dlna_renderers.manufacturer),
        model_name = COALESCE(EXCLUDED.model_name, np_dlna_renderers.model_name),
        last_seen_at = EXCLUDED.last_seen_at,
        updated_at = NOW()
      RETURNING id`,
      [
        renderer.source_account_id, renderer.usn, renderer.friendly_name,
        renderer.location, renderer.ip_address, renderer.device_type,
        renderer.manufacturer, renderer.model_name, renderer.last_seen_at,
      ]
    );
    return result.rows[0].id;
  }

  /**
   * List all discovered renderers
   */
  async listRenderers(limit = 100, offset = 0): Promise<RendererRecord[]> {
    const result = await this.db.query<RendererRecord>(
      `SELECT * FROM np_dlna_renderers
       WHERE source_account_id = $1
       ORDER BY last_seen_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  /**
   * Count renderers
   */
  async countRenderers(): Promise<number> {
    return this.db.countScoped('np_dlna_renderers', this.sourceAccountId);
  }

  /**
   * Get recently seen renderers (within last N minutes)
   */
  async getActiveRenderers(minutesAgo = 5): Promise<RendererRecord[]> {
    const result = await this.db.query<RendererRecord>(
      `SELECT * FROM np_dlna_renderers
       WHERE source_account_id = $1
         AND last_seen_at > NOW() - INTERVAL '1 minute' * $2
       ORDER BY last_seen_at DESC`,
      [this.sourceAccountId, minutesAgo]
    );
    return result.rows;
  }

  /**
   * Remove renderers not seen for a long time
   */
  async pruneStaleRenderers(hoursAgo = 24): Promise<number> {
    return this.db.execute(
      `DELETE FROM np_dlna_renderers
       WHERE source_account_id = $1
         AND last_seen_at < NOW() - INTERVAL '1 hour' * $2`,
      [this.sourceAccountId, hoursAgo]
    );
  }

  // ---------------------------------------------------------------------------
  // Statistics
  // ---------------------------------------------------------------------------

  async getStats(): Promise<Record<string, unknown>> {
    const mediaItems = await this.countMediaItems();
    const containers = await this.countMediaItems('container');
    const items = await this.countMediaItems('item');
    const renderers = await this.countRenderers();

    const videoResult = await this.db.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_dlna_media_items
       WHERE source_account_id = $1 AND mime_type LIKE 'video/%'`,
      [this.sourceAccountId]
    );
    const audioResult = await this.db.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_dlna_media_items
       WHERE source_account_id = $1 AND mime_type LIKE 'audio/%'`,
      [this.sourceAccountId]
    );
    const imageResult = await this.db.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_dlna_media_items
       WHERE source_account_id = $1 AND mime_type LIKE 'image/%'`,
      [this.sourceAccountId]
    );

    const sizeResult = await this.db.query<{ total: string | null }>(
      `SELECT SUM(file_size) as total FROM np_dlna_media_items
       WHERE source_account_id = $1 AND file_size IS NOT NULL`,
      [this.sourceAccountId]
    );

    return {
      mediaItems,
      containers,
      items,
      videos: parseInt(videoResult.rows[0].count, 10),
      audio: parseInt(audioResult.rows[0].count, 10),
      images: parseInt(imageResult.rows[0].count, 10),
      renderers,
      totalSizeBytes: parseInt(sizeResult.rows[0].total ?? '0', 10),
      lastSyncedAt: await this.db.getLastSyncTimeScoped('np_dlna_media_items', this.sourceAccountId),
    };
  }
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

/**
 * Extract search term from UPnP search criteria
 * e.g. 'dc:title contains "Terminator"' -> 'Terminator'
 */
function extractSearchTerm(criteria: string): string | null {
  const match = criteria.match(/contains\s+"([^"]+)"/i);
  return match ? match[1] : null;
}

/**
 * Extract UPnP class filter from search criteria
 * e.g. 'upnp:class derivedfrom "object.item.videoItem"' -> 'object.item.videoItem'
 */
function extractClassFilter(criteria: string): string | null {
  const match = criteria.match(/upnp:class\s+derivedfrom\s+"([^"]+)"/i);
  return match ? match[1] : null;
}
