/**
 * Photos Plugin Database
 * Schema initialization and CRUD operations for photo album management
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  PhotosAlbumRecord,
  PhotosItemRecord,
  PhotosTagRecord,
  PhotosFaceRecord,
  PhotosStats,
  TimelinePeriod,
} from './types.js';

const logger = createLogger('photos:database');

export class PhotosDatabase {
  private db: Database;
  private sourceAccountId: string = 'primary';

  constructor(db: Database) {
    this.db = db;
  }

  forSourceAccount(sourceAccountId: string): PhotosDatabase {
    const scoped = new PhotosDatabase(this.db);
    scoped.sourceAccountId = sourceAccountId;
    return scoped;
  }

  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  // ============================================================================
  // Schema Initialization
  // ============================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing photos database schema...');

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS photos_albums (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        owner_id VARCHAR(255) NOT NULL,
        name VARCHAR(255) NOT NULL,
        description TEXT,
        cover_photo_id UUID,
        visibility VARCHAR(20) DEFAULT 'private',
        visibility_user_ids TEXT[],
        photo_count INTEGER DEFAULT 0,
        sort_order VARCHAR(20) DEFAULT 'date_desc',
        date_range_start DATE,
        date_range_end DATE,
        location_name VARCHAR(255),
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      )
    `);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_albums_source_app ON photos_albums(source_account_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_albums_owner ON photos_albums(source_account_id, owner_id)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS photos_items (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        album_id UUID REFERENCES photos_albums(id) ON DELETE SET NULL,
        uploader_id VARCHAR(255) NOT NULL,
        file_id VARCHAR(255),
        original_url TEXT NOT NULL,
        thumbnail_small_url TEXT,
        thumbnail_medium_url TEXT,
        thumbnail_large_url TEXT,
        width INTEGER,
        height INTEGER,
        file_size_bytes BIGINT,
        mime_type VARCHAR(50),
        original_filename VARCHAR(500),
        caption TEXT,
        visibility VARCHAR(20) DEFAULT 'album',
        taken_at TIMESTAMPTZ,
        location_latitude DOUBLE PRECISION,
        location_longitude DOUBLE PRECISION,
        location_name VARCHAR(255),
        camera_make VARCHAR(100),
        camera_model VARCHAR(100),
        focal_length VARCHAR(20),
        aperture VARCHAR(20),
        shutter_speed VARCHAR(20),
        iso INTEGER,
        orientation INTEGER DEFAULT 1,
        processing_status VARCHAR(20) DEFAULT 'pending',
        search_vector tsvector,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      )
    `);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_items_source_app ON photos_items(source_account_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_items_album ON photos_items(album_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_items_uploader ON photos_items(source_account_id, uploader_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_items_taken ON photos_items(source_account_id, taken_at DESC)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_items_location ON photos_items(location_latitude, location_longitude) WHERE location_latitude IS NOT NULL`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_items_search ON photos_items USING GIN(search_vector)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS photos_tags (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        photo_id UUID NOT NULL REFERENCES photos_items(id) ON DELETE CASCADE,
        tag_type VARCHAR(20) NOT NULL DEFAULT 'keyword',
        tag_value VARCHAR(255) NOT NULL,
        tagged_user_id VARCHAR(255),
        face_region JSONB,
        confidence DOUBLE PRECISION,
        created_by VARCHAR(255),
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, photo_id, tag_type, tag_value)
      )
    `);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_tags_source_app ON photos_tags(source_account_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_tags_photo ON photos_tags(photo_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_tags_value ON photos_tags(source_account_id, tag_type, tag_value)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_tags_user ON photos_tags(source_account_id, tagged_user_id)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS photos_faces (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255),
        user_id VARCHAR(255),
        representative_photo_id UUID REFERENCES photos_items(id),
        photo_count INTEGER DEFAULT 0,
        confirmed BOOLEAN DEFAULT false,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      )
    `);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_faces_source_app ON photos_faces(source_account_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_faces_user ON photos_faces(source_account_id, user_id)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS photos_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMPTZ,
        error TEXT,
        retry_count INTEGER DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW()
      )
    `);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_photos_webhook_events_source_app ON photos_webhook_events(source_account_id)`);

    logger.success('Photos database schema initialized');
  }

  // ============================================================================
  // Albums CRUD
  // ============================================================================

  async createAlbum(album: {
    owner_id: string;
    name: string;
    description?: string;
    visibility?: string;
    visibility_user_ids?: string[];
    sort_order?: string;
    metadata?: Record<string, unknown>;
  }): Promise<PhotosAlbumRecord> {
    const result = await this.db.query<PhotosAlbumRecord>(`
      INSERT INTO photos_albums (
        source_account_id, owner_id, name, description, visibility,
        visibility_user_ids, sort_order, metadata
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
      RETURNING *
    `, [
      this.sourceAccountId, album.owner_id, album.name, album.description || null,
      album.visibility || 'private', album.visibility_user_ids || null,
      album.sort_order || 'date_desc', JSON.stringify(album.metadata || {}),
    ]);
    return result.rows[0];
  }

  async getAlbum(id: string): Promise<PhotosAlbumRecord | null> {
    return this.db.queryOne<PhotosAlbumRecord>(
      `SELECT * FROM photos_albums WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async listAlbums(ownerId?: string, visibility?: string, limit: number = 50, offset: number = 0): Promise<{ albums: PhotosAlbumRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (ownerId) {
      conditions.push(`owner_id = $${paramIndex++}`);
      params.push(ownerId);
    }
    if (visibility) {
      conditions.push(`visibility = $${paramIndex++}`);
      params.push(visibility);
    }

    const whereClause = conditions.join(' AND ');

    const countResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM photos_albums WHERE ${whereClause}`, params
    );
    const total = parseInt(countResult?.count ?? '0', 10);

    const result = await this.db.query<PhotosAlbumRecord>(
      `SELECT * FROM photos_albums WHERE ${whereClause} ORDER BY updated_at DESC LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`,
      [...params, limit, offset]
    );

    return { albums: result.rows, total };
  }

  async updateAlbum(id: string, updates: {
    name?: string;
    description?: string;
    cover_photo_id?: string;
    visibility?: string;
    visibility_user_ids?: string[];
    sort_order?: string;
  }): Promise<PhotosAlbumRecord | null> {
    const setClauses: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (updates.name !== undefined) { setClauses.push(`name = $${paramIndex++}`); params.push(updates.name); }
    if (updates.description !== undefined) { setClauses.push(`description = $${paramIndex++}`); params.push(updates.description); }
    if (updates.cover_photo_id !== undefined) { setClauses.push(`cover_photo_id = $${paramIndex++}`); params.push(updates.cover_photo_id); }
    if (updates.visibility !== undefined) { setClauses.push(`visibility = $${paramIndex++}`); params.push(updates.visibility); }
    if (updates.visibility_user_ids !== undefined) { setClauses.push(`visibility_user_ids = $${paramIndex++}`); params.push(updates.visibility_user_ids); }
    if (updates.sort_order !== undefined) { setClauses.push(`sort_order = $${paramIndex++}`); params.push(updates.sort_order); }

    if (setClauses.length === 0) return this.getAlbum(id);

    setClauses.push('updated_at = NOW()');

    const result = await this.db.query<PhotosAlbumRecord>(
      `UPDATE photos_albums SET ${setClauses.join(', ')} WHERE id = $1 AND source_account_id = $2 RETURNING *`,
      params
    );
    return result.rows[0] || null;
  }

  async deleteAlbum(id: string): Promise<void> {
    await this.db.execute(
      `DELETE FROM photos_albums WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async updateAlbumPhotoCount(albumId: string): Promise<void> {
    await this.db.execute(`
      UPDATE photos_albums SET photo_count = (
        SELECT COUNT(*) FROM photos_items WHERE album_id = $1 AND source_account_id = $2
      ), updated_at = NOW()
      WHERE id = $1 AND source_account_id = $2
    `, [albumId, this.sourceAccountId]);
  }

  // ============================================================================
  // Photos CRUD
  // ============================================================================

  async registerPhoto(photo: {
    album_id?: string;
    uploader_id: string;
    file_id?: string;
    original_url: string;
    original_filename?: string;
    caption?: string;
    visibility?: string;
  }): Promise<PhotosItemRecord> {
    const result = await this.db.query<PhotosItemRecord>(`
      INSERT INTO photos_items (
        source_account_id, album_id, uploader_id, file_id, original_url,
        original_filename, caption, visibility
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
      RETURNING *
    `, [
      this.sourceAccountId, photo.album_id || null, photo.uploader_id,
      photo.file_id || null, photo.original_url, photo.original_filename || null,
      photo.caption || null, photo.visibility || 'album',
    ]);

    // Update album photo count if assigned
    if (photo.album_id) {
      await this.updateAlbumPhotoCount(photo.album_id);
    }

    return result.rows[0];
  }

  async getPhoto(id: string): Promise<PhotosItemRecord | null> {
    return this.db.queryOne<PhotosItemRecord>(
      `SELECT * FROM photos_items WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async listPhotos(filters: {
    albumId?: string;
    uploaderId?: string;
    takenFrom?: string;
    takenTo?: string;
    limit?: number;
    offset?: number;
  }): Promise<{ photos: PhotosItemRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.albumId) { conditions.push(`album_id = $${paramIndex++}`); params.push(filters.albumId); }
    if (filters.uploaderId) { conditions.push(`uploader_id = $${paramIndex++}`); params.push(filters.uploaderId); }
    if (filters.takenFrom) { conditions.push(`taken_at >= $${paramIndex++}`); params.push(filters.takenFrom); }
    if (filters.takenTo) { conditions.push(`taken_at <= $${paramIndex++}`); params.push(filters.takenTo); }

    const whereClause = conditions.join(' AND ');
    const limit = filters.limit ?? 50;
    const offset = filters.offset ?? 0;

    const countResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM photos_items WHERE ${whereClause}`, params
    );
    const total = parseInt(countResult?.count ?? '0', 10);

    const result = await this.db.query<PhotosItemRecord>(
      `SELECT * FROM photos_items WHERE ${whereClause} ORDER BY COALESCE(taken_at, created_at) DESC LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`,
      [...params, limit, offset]
    );

    return { photos: result.rows, total };
  }

  async updatePhoto(id: string, updates: {
    caption?: string;
    album_id?: string;
    visibility?: string;
  }): Promise<PhotosItemRecord | null> {
    const setClauses: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (updates.caption !== undefined) { setClauses.push(`caption = $${paramIndex++}`); params.push(updates.caption); }
    if (updates.album_id !== undefined) { setClauses.push(`album_id = $${paramIndex++}`); params.push(updates.album_id); }
    if (updates.visibility !== undefined) { setClauses.push(`visibility = $${paramIndex++}`); params.push(updates.visibility); }

    if (setClauses.length === 0) return this.getPhoto(id);

    setClauses.push('updated_at = NOW()');

    const result = await this.db.query<PhotosItemRecord>(
      `UPDATE photos_items SET ${setClauses.join(', ')} WHERE id = $1 AND source_account_id = $2 RETURNING *`,
      params
    );
    return result.rows[0] || null;
  }

  async deletePhoto(id: string): Promise<void> {
    const photo = await this.getPhoto(id);
    await this.db.execute(
      `DELETE FROM photos_items WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    if (photo?.album_id) {
      await this.updateAlbumPhotoCount(photo.album_id);
    }
  }

  async movePhoto(id: string, albumId: string): Promise<PhotosItemRecord | null> {
    const photo = await this.getPhoto(id);
    const oldAlbumId = photo?.album_id;

    const result = await this.db.query<PhotosItemRecord>(
      `UPDATE photos_items SET album_id = $3, updated_at = NOW() WHERE id = $1 AND source_account_id = $2 RETURNING *`,
      [id, this.sourceAccountId, albumId]
    );

    if (oldAlbumId) await this.updateAlbumPhotoCount(oldAlbumId);
    await this.updateAlbumPhotoCount(albumId);

    return result.rows[0] || null;
  }

  async updatePhotoProcessingStatus(id: string, status: string, exifData?: Record<string, unknown>): Promise<void> {
    const setClauses = ['processing_status = $3', 'updated_at = NOW()'];
    const params: unknown[] = [id, this.sourceAccountId, status];
    let paramIndex = 4;

    if (exifData) {
      if (exifData.width) { setClauses.push(`width = $${paramIndex++}`); params.push(exifData.width); }
      if (exifData.height) { setClauses.push(`height = $${paramIndex++}`); params.push(exifData.height); }
      if (exifData.taken_at) { setClauses.push(`taken_at = $${paramIndex++}`); params.push(exifData.taken_at); }
      if (exifData.camera_make) { setClauses.push(`camera_make = $${paramIndex++}`); params.push(exifData.camera_make); }
      if (exifData.camera_model) { setClauses.push(`camera_model = $${paramIndex++}`); params.push(exifData.camera_model); }
      if (exifData.focal_length) { setClauses.push(`focal_length = $${paramIndex++}`); params.push(exifData.focal_length); }
      if (exifData.aperture) { setClauses.push(`aperture = $${paramIndex++}`); params.push(exifData.aperture); }
      if (exifData.shutter_speed) { setClauses.push(`shutter_speed = $${paramIndex++}`); params.push(exifData.shutter_speed); }
      if (exifData.iso) { setClauses.push(`iso = $${paramIndex++}`); params.push(exifData.iso); }
      if (exifData.orientation) { setClauses.push(`orientation = $${paramIndex++}`); params.push(exifData.orientation); }
      if (exifData.location_latitude) { setClauses.push(`location_latitude = $${paramIndex++}`); params.push(exifData.location_latitude); }
      if (exifData.location_longitude) { setClauses.push(`location_longitude = $${paramIndex++}`); params.push(exifData.location_longitude); }
      if (exifData.location_name) { setClauses.push(`location_name = $${paramIndex++}`); params.push(exifData.location_name); }
    }

    // Update search vector
    setClauses.push(`search_vector = to_tsvector('english', COALESCE(caption, '') || ' ' || COALESCE(original_filename, '') || ' ' || COALESCE(location_name, ''))`);

    await this.db.execute(
      `UPDATE photos_items SET ${setClauses.join(', ')} WHERE id = $1 AND source_account_id = $2`,
      params
    );
  }

  async getPendingPhotos(limit: number = 100): Promise<PhotosItemRecord[]> {
    const result = await this.db.query<PhotosItemRecord>(
      `SELECT * FROM photos_items WHERE source_account_id = $1 AND processing_status = 'pending' ORDER BY created_at ASC LIMIT $2`,
      [this.sourceAccountId, limit]
    );
    return result.rows;
  }

  async setPhotoThumbnails(id: string, small: string, medium: string, large: string): Promise<void> {
    await this.db.execute(`
      UPDATE photos_items SET
        thumbnail_small_url = $3,
        thumbnail_medium_url = $4,
        thumbnail_large_url = $5,
        updated_at = NOW()
      WHERE id = $1 AND source_account_id = $2
    `, [id, this.sourceAccountId, small, medium, large]);
  }

  // ============================================================================
  // Tags CRUD
  // ============================================================================

  async addTag(photoId: string, tag: {
    tag_type: string;
    tag_value: string;
    tagged_user_id?: string;
    face_region?: Record<string, unknown>;
    confidence?: number;
    created_by?: string;
  }): Promise<PhotosTagRecord> {
    const result = await this.db.query<PhotosTagRecord>(`
      INSERT INTO photos_tags (
        source_account_id, photo_id, tag_type, tag_value,
        tagged_user_id, face_region, confidence, created_by
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
      ON CONFLICT (source_account_id, photo_id, tag_type, tag_value) DO UPDATE SET
        tagged_user_id = EXCLUDED.tagged_user_id,
        face_region = EXCLUDED.face_region,
        confidence = EXCLUDED.confidence,
        created_by = EXCLUDED.created_by
      RETURNING *
    `, [
      this.sourceAccountId, photoId, tag.tag_type, tag.tag_value,
      tag.tagged_user_id || null, tag.face_region ? JSON.stringify(tag.face_region) : null,
      tag.confidence || null, tag.created_by || null,
    ]);
    return result.rows[0];
  }

  async removeTag(tagId: string): Promise<void> {
    await this.db.execute(
      `DELETE FROM photos_tags WHERE id = $1 AND source_account_id = $2`,
      [tagId, this.sourceAccountId]
    );
  }

  async getPhotoTags(photoId: string): Promise<PhotosTagRecord[]> {
    const result = await this.db.query<PhotosTagRecord>(
      `SELECT * FROM photos_tags WHERE photo_id = $1 AND source_account_id = $2 ORDER BY created_at`,
      [photoId, this.sourceAccountId]
    );
    return result.rows;
  }

  async listTags(tagType?: string, limit: number = 100): Promise<Array<{ value: string; count: number }>> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (tagType) {
      conditions.push(`tag_type = $${paramIndex++}`);
      params.push(tagType);
    }

    const result = await this.db.query<{ tag_value: string; count: string }>(
      `SELECT tag_value, COUNT(*) as count FROM photos_tags WHERE ${conditions.join(' AND ')}
       GROUP BY tag_value ORDER BY count DESC LIMIT $${paramIndex}`,
      [...params, limit]
    );

    return result.rows.map(r => ({ value: r.tag_value, count: parseInt(r.count, 10) }));
  }

  async getPhotosWithTag(tagValue: string, limit: number = 50, offset: number = 0): Promise<{ photos: PhotosItemRecord[]; total: number }> {
    const countResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(DISTINCT pi.id) as count FROM photos_items pi
       JOIN photos_tags pt ON pt.photo_id = pi.id
       WHERE pt.source_account_id = $1 AND pt.tag_value = $2`,
      [this.sourceAccountId, tagValue]
    );
    const total = parseInt(countResult?.count ?? '0', 10);

    const result = await this.db.query<PhotosItemRecord>(
      `SELECT DISTINCT pi.* FROM photos_items pi
       JOIN photos_tags pt ON pt.photo_id = pi.id
       WHERE pt.source_account_id = $1 AND pt.tag_value = $2
       ORDER BY pi.created_at DESC LIMIT $3 OFFSET $4`,
      [this.sourceAccountId, tagValue, limit, offset]
    );

    return { photos: result.rows, total };
  }

  // ============================================================================
  // Faces CRUD
  // ============================================================================

  async listFaces(limit: number = 50, offset: number = 0): Promise<{ faces: PhotosFaceRecord[]; total: number }> {
    const countResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM photos_faces WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
    const total = parseInt(countResult?.count ?? '0', 10);

    const result = await this.db.query<PhotosFaceRecord>(
      `SELECT * FROM photos_faces WHERE source_account_id = $1 ORDER BY photo_count DESC LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return { faces: result.rows, total };
  }

  async updateFace(id: string, updates: { name?: string; user_id?: string }): Promise<PhotosFaceRecord | null> {
    const setClauses: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (updates.name !== undefined) { setClauses.push(`name = $${paramIndex++}`); params.push(updates.name); }
    if (updates.user_id !== undefined) { setClauses.push(`user_id = $${paramIndex++}`); params.push(updates.user_id); }

    const result = await this.db.query<PhotosFaceRecord>(
      `UPDATE photos_faces SET ${setClauses.join(', ')} WHERE id = $1 AND source_account_id = $2 RETURNING *`,
      params
    );
    return result.rows[0] || null;
  }

  async mergeFaces(id: string, mergeWithId: string): Promise<PhotosFaceRecord | null> {
    // Move all tags referencing the merged face to the target face
    const mergedFace = await this.db.queryOne<PhotosFaceRecord>(
      `SELECT * FROM photos_faces WHERE id = $1 AND source_account_id = $2`,
      [mergeWithId, this.sourceAccountId]
    );

    if (!mergedFace) return null;

    // Update photo count on target
    await this.db.execute(`
      UPDATE photos_faces SET
        photo_count = photo_count + $3,
        updated_at = NOW()
      WHERE id = $1 AND source_account_id = $2
    `, [id, this.sourceAccountId, mergedFace.photo_count]);

    // Delete merged face
    await this.db.execute(
      `DELETE FROM photos_faces WHERE id = $1 AND source_account_id = $2`,
      [mergeWithId, this.sourceAccountId]
    );

    return this.db.queryOne<PhotosFaceRecord>(
      `SELECT * FROM photos_faces WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  // ============================================================================
  // Timeline
  // ============================================================================

  async getTimeline(granularity: string = 'month', from?: string, to?: string, userId?: string): Promise<TimelinePeriod[]> {
    const conditions: string[] = ['source_account_id = $1', 'taken_at IS NOT NULL'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (from) { conditions.push(`taken_at >= $${paramIndex++}`); params.push(from); }
    if (to) { conditions.push(`taken_at <= $${paramIndex++}`); params.push(to); }
    if (userId) { conditions.push(`uploader_id = $${paramIndex++}`); params.push(userId); }

    const dateFormat = granularity === 'day' ? 'YYYY-MM-DD'
      : granularity === 'week' ? 'IYYY-"W"IW'
      : granularity === 'year' ? 'YYYY'
      : 'YYYY-MM';

    const result = await this.db.query<{
      period: string;
      count: string;
      cover_url: string | null;
      location: string | null;
    }>(`
      SELECT
        TO_CHAR(taken_at, '${dateFormat}') as period,
        COUNT(*) as count,
        (SELECT thumbnail_medium_url FROM photos_items pi2
         WHERE pi2.source_account_id = photos_items.source_account_id
           AND TO_CHAR(pi2.taken_at, '${dateFormat}') = TO_CHAR(photos_items.taken_at, '${dateFormat}')
         ORDER BY pi2.taken_at ASC LIMIT 1) as cover_url,
        MODE() WITHIN GROUP (ORDER BY location_name) as location
      FROM photos_items
      WHERE ${conditions.join(' AND ')}
      GROUP BY period, source_account_id
      ORDER BY period DESC
    `, params);

    return result.rows.map(r => ({
      period: r.period,
      count: parseInt(r.count, 10),
      coverPhotoUrl: r.cover_url,
      location: r.location,
    }));
  }

  // ============================================================================
  // Search
  // ============================================================================

  async searchPhotos(query: string, filters: {
    tags?: string[];
    location?: string;
    dateFrom?: string;
    dateTo?: string;
    uploaderId?: string;
    albumId?: string;
    limit?: number;
    offset?: number;
  }): Promise<{ photos: PhotosItemRecord[]; total: number }> {
    const conditions: string[] = ['pi.source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (query) {
      conditions.push(`pi.search_vector @@ plainto_tsquery('english', $${paramIndex++})`);
      params.push(query);
    }

    if (filters.location) {
      conditions.push(`pi.location_name ILIKE $${paramIndex++}`);
      params.push(`%${filters.location}%`);
    }

    if (filters.dateFrom) { conditions.push(`pi.taken_at >= $${paramIndex++}`); params.push(filters.dateFrom); }
    if (filters.dateTo) { conditions.push(`pi.taken_at <= $${paramIndex++}`); params.push(filters.dateTo); }
    if (filters.uploaderId) { conditions.push(`pi.uploader_id = $${paramIndex++}`); params.push(filters.uploaderId); }
    if (filters.albumId) { conditions.push(`pi.album_id = $${paramIndex++}`); params.push(filters.albumId); }

    let joinClause = '';
    if (filters.tags && filters.tags.length > 0) {
      joinClause = `JOIN photos_tags pt ON pt.photo_id = pi.id AND pt.source_account_id = pi.source_account_id`;
      conditions.push(`pt.tag_value = ANY($${paramIndex++})`);
      params.push(filters.tags);
    }

    const whereClause = conditions.join(' AND ');
    const limit = filters.limit ?? 50;
    const offset = filters.offset ?? 0;

    const countResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(DISTINCT pi.id) as count FROM photos_items pi ${joinClause} WHERE ${whereClause}`,
      params
    );
    const total = parseInt(countResult?.count ?? '0', 10);

    const result = await this.db.query<PhotosItemRecord>(
      `SELECT DISTINCT pi.* FROM photos_items pi ${joinClause} WHERE ${whereClause}
       ORDER BY pi.created_at DESC LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`,
      [...params, limit, offset]
    );

    return { photos: result.rows, total };
  }

  // ============================================================================
  // Webhook Events
  // ============================================================================

  async insertWebhookEvent(eventId: string, eventType: string, payload: Record<string, unknown>): Promise<void> {
    await this.db.execute(`
      INSERT INTO photos_webhook_events (id, source_account_id, event_type, payload)
      VALUES ($1, $2, $3, $4)
      ON CONFLICT (id) DO NOTHING
    `, [eventId, this.sourceAccountId, eventType, JSON.stringify(payload)]);
  }

  // ============================================================================
  // Statistics
  // ============================================================================

  async getStats(): Promise<PhotosStats> {
    const totalAlbums = await this.db.countScoped('photos_albums', this.sourceAccountId);
    const totalPhotos = await this.db.countScoped('photos_items', this.sourceAccountId);
    const totalTags = await this.db.countScoped('photos_tags', this.sourceAccountId);
    const totalFaces = await this.db.countScoped('photos_faces', this.sourceAccountId);

    const pending = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM photos_items WHERE source_account_id = $1 AND processing_status = 'pending'`,
      [this.sourceAccountId]
    );

    const processed = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM photos_items WHERE source_account_id = $1 AND processing_status = 'completed'`,
      [this.sourceAccountId]
    );

    const storage = await this.db.queryOne<{ total: string }>(
      `SELECT COALESCE(SUM(file_size_bytes), 0) as total FROM photos_items WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    return {
      totalAlbums,
      totalPhotos,
      totalTags,
      totalFaces,
      pendingProcessing: parseInt(pending?.count ?? '0', 10),
      processedPhotos: parseInt(processed?.count ?? '0', 10),
      totalStorageBytes: parseInt(storage?.total ?? '0', 10),
    };
  }
}

export async function createPhotosDatabase(config: {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl?: boolean;
}): Promise<PhotosDatabase> {
  const db = createDatabase(config);
  await db.connect();
  return new PhotosDatabase(db);
}
