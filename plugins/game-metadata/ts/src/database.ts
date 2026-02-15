/**
 * Game Metadata Database Operations
 * Complete CRUD operations for game catalog, metadata, artwork, platforms, and genres
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  GameCatalogRecord,
  GameMetadataRecord,
  GameArtworkRecord,
  GamePlatformRecord,
  GameGenreRecord,
  GameMetadataStats,
} from './types.js';

const logger = createLogger('game-metadata:db');

export class GameMetadataDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): GameMetadataDatabase {
    return new GameMetadataDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing game metadata schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Platforms
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_game_platforms (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        abbreviation VARCHAR(20),
        slug VARCHAR(255) NOT NULL,
        igdb_id INTEGER,
        generation INTEGER,
        manufacturer VARCHAR(255),
        platform_family VARCHAR(255),
        category VARCHAR(50),
        release_date DATE,
        summary TEXT,
        is_active BOOLEAN DEFAULT true,
        sort_order INTEGER DEFAULT 0,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, slug)
      );

      CREATE INDEX IF NOT EXISTS idx_np_game_platforms_source_app
        ON np_game_platforms(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_game_platforms_slug
        ON np_game_platforms(source_account_id, slug);
      CREATE UNIQUE INDEX IF NOT EXISTS idx_np_game_platforms_igdb
        ON np_game_platforms(source_account_id, igdb_id) WHERE igdb_id IS NOT NULL;

      -- =====================================================================
      -- Genres
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_game_genres (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        slug VARCHAR(255) NOT NULL,
        igdb_id INTEGER,
        description TEXT,
        parent_id UUID REFERENCES np_game_genres(id) ON DELETE SET NULL,
        is_active BOOLEAN DEFAULT true,
        sort_order INTEGER DEFAULT 0,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, slug)
      );

      CREATE INDEX IF NOT EXISTS idx_np_game_genres_source_app
        ON np_game_genres(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_game_genres_slug
        ON np_game_genres(source_account_id, slug);
      CREATE UNIQUE INDEX IF NOT EXISTS idx_np_game_genres_igdb
        ON np_game_genres(source_account_id, igdb_id) WHERE igdb_id IS NOT NULL;

      -- =====================================================================
      -- Game Catalog
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_game_catalog (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        title VARCHAR(500) NOT NULL,
        slug VARCHAR(500) NOT NULL,
        platform_id UUID REFERENCES np_game_platforms(id) ON DELETE SET NULL,
        genre_id UUID REFERENCES np_game_genres(id) ON DELETE SET NULL,
        release_date DATE,
        developer VARCHAR(255),
        publisher VARCHAR(255),
        description TEXT,
        igdb_id INTEGER,
        rom_hash_md5 VARCHAR(32),
        rom_hash_sha1 VARCHAR(40),
        rom_hash_sha256 VARCHAR(64),
        rom_hash_crc32 VARCHAR(8),
        rom_filename VARCHAR(500),
        rom_size_bytes BIGINT,
        tier VARCHAR(50),
        rating DOUBLE PRECISION,
        players_min INTEGER DEFAULT 1,
        players_max INTEGER DEFAULT 1,
        is_verified BOOLEAN DEFAULT false,
        search_vector tsvector,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, slug, platform_id)
      );

      CREATE INDEX IF NOT EXISTS idx_np_game_catalog_source_app
        ON np_game_catalog(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_game_catalog_title
        ON np_game_catalog(source_account_id, title);
      CREATE INDEX IF NOT EXISTS idx_np_game_catalog_platform
        ON np_game_catalog(source_account_id, platform_id);
      CREATE INDEX IF NOT EXISTS idx_np_game_catalog_genre
        ON np_game_catalog(source_account_id, genre_id);
      CREATE INDEX IF NOT EXISTS idx_np_game_catalog_tier
        ON np_game_catalog(source_account_id, tier);
      CREATE INDEX IF NOT EXISTS idx_np_game_catalog_search
        ON np_game_catalog USING GIN(search_vector);
      CREATE UNIQUE INDEX IF NOT EXISTS idx_np_game_catalog_igdb
        ON np_game_catalog(source_account_id, igdb_id) WHERE igdb_id IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_np_game_catalog_hash_md5
        ON np_game_catalog(rom_hash_md5) WHERE rom_hash_md5 IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_np_game_catalog_hash_sha1
        ON np_game_catalog(rom_hash_sha1) WHERE rom_hash_sha1 IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_np_game_catalog_hash_crc32
        ON np_game_catalog(rom_hash_crc32) WHERE rom_hash_crc32 IS NOT NULL;

      -- =====================================================================
      -- Game Metadata (IGDB enrichment data)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_game_metadata (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        game_id UUID NOT NULL REFERENCES np_game_catalog(id) ON DELETE CASCADE,
        source VARCHAR(50) NOT NULL DEFAULT 'igdb',
        igdb_id INTEGER,
        igdb_url TEXT,
        summary TEXT,
        storyline TEXT,
        total_rating DOUBLE PRECISION,
        total_rating_count INTEGER,
        aggregated_rating DOUBLE PRECISION,
        aggregated_rating_count INTEGER,
        first_release_date DATE,
        genres TEXT[] DEFAULT '{}',
        themes TEXT[] DEFAULT '{}',
        keywords TEXT[] DEFAULT '{}',
        game_modes TEXT[] DEFAULT '{}',
        franchises TEXT[] DEFAULT '{}',
        alternative_names TEXT[] DEFAULT '{}',
        websites JSONB DEFAULT '{}',
        age_ratings JSONB DEFAULT '{}',
        involved_companies JSONB DEFAULT '[]',
        raw_data JSONB DEFAULT '{}',
        fetched_at TIMESTAMPTZ DEFAULT NOW(),
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, game_id, source)
      );

      CREATE INDEX IF NOT EXISTS idx_np_game_metadata_source_app
        ON np_game_metadata(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_game_metadata_game
        ON np_game_metadata(game_id);
      CREATE UNIQUE INDEX IF NOT EXISTS idx_np_game_metadata_igdb
        ON np_game_metadata(source_account_id, igdb_id) WHERE igdb_id IS NOT NULL;

      -- =====================================================================
      -- Game Artwork
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_game_artwork (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        game_id UUID NOT NULL REFERENCES np_game_catalog(id) ON DELETE CASCADE,
        artwork_type VARCHAR(50) NOT NULL,
        url TEXT,
        local_path TEXT,
        width INTEGER,
        height INTEGER,
        mime_type VARCHAR(50),
        file_size_bytes BIGINT,
        source VARCHAR(50) NOT NULL DEFAULT 'igdb',
        igdb_image_id VARCHAR(50),
        is_primary BOOLEAN DEFAULT false,
        sort_order INTEGER DEFAULT 0,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_game_artwork_source_app
        ON np_game_artwork(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_game_artwork_game
        ON np_game_artwork(game_id);
      CREATE INDEX IF NOT EXISTS idx_np_game_artwork_type
        ON np_game_artwork(game_id, artwork_type);
    `;

    await this.execute(schema);
    logger.info('Game metadata schema initialized successfully');
  }

  // =========================================================================
  // Platform Operations
  // =========================================================================

  async createPlatform(platform: Omit<GamePlatformRecord, 'id' | 'created_at' | 'updated_at'>): Promise<GamePlatformRecord> {
    const result = await this.query<GamePlatformRecord>(
      `INSERT INTO np_game_platforms (
        source_account_id, name, abbreviation, slug, igdb_id,
        generation, manufacturer, platform_family, category,
        release_date, summary, is_active, sort_order, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      RETURNING *`,
      [
        this.sourceAccountId, platform.name, platform.abbreviation,
        platform.slug, platform.igdb_id, platform.generation,
        platform.manufacturer, platform.platform_family, platform.category,
        platform.release_date, platform.summary, platform.is_active,
        platform.sort_order, JSON.stringify(platform.metadata),
      ]
    );

    return result.rows[0];
  }

  async getPlatform(id: string): Promise<GamePlatformRecord | null> {
    const result = await this.query<GamePlatformRecord>(
      `SELECT * FROM np_game_platforms WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getPlatformBySlug(slug: string): Promise<GamePlatformRecord | null> {
    const result = await this.query<GamePlatformRecord>(
      `SELECT * FROM np_game_platforms WHERE slug = $1 AND source_account_id = $2`,
      [slug, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listPlatforms(filters?: { isActive?: boolean; limit?: number; offset?: number }): Promise<GamePlatformRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.isActive !== undefined) {
      conditions.push(`is_active = $${paramIndex}`);
      values.push(filters.isActive);
      paramIndex++;
    }

    // Suppress unused variable warning
    void paramIndex;

    let sql = `
      SELECT * FROM np_game_platforms
      WHERE ${conditions.join(' AND ')}
      ORDER BY sort_order ASC, name ASC
    `;

    if (filters?.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters?.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<GamePlatformRecord>(sql, values);
    return result.rows;
  }

  async deletePlatform(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_game_platforms WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  // =========================================================================
  // Genre Operations
  // =========================================================================

  async createGenre(genre: Omit<GameGenreRecord, 'id' | 'created_at' | 'updated_at'>): Promise<GameGenreRecord> {
    const result = await this.query<GameGenreRecord>(
      `INSERT INTO np_game_genres (
        source_account_id, name, slug, igdb_id, description,
        parent_id, is_active, sort_order, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
      RETURNING *`,
      [
        this.sourceAccountId, genre.name, genre.slug, genre.igdb_id,
        genre.description, genre.parent_id, genre.is_active,
        genre.sort_order, JSON.stringify(genre.metadata),
      ]
    );

    return result.rows[0];
  }

  async getGenre(id: string): Promise<GameGenreRecord | null> {
    const result = await this.query<GameGenreRecord>(
      `SELECT * FROM np_game_genres WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listGenres(filters?: { isActive?: boolean; limit?: number; offset?: number }): Promise<GameGenreRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.isActive !== undefined) {
      conditions.push(`is_active = $${paramIndex}`);
      values.push(filters.isActive);
      paramIndex++;
    }

    // Suppress unused variable warning
    void paramIndex;

    let sql = `
      SELECT * FROM np_game_genres
      WHERE ${conditions.join(' AND ')}
      ORDER BY sort_order ASC, name ASC
    `;

    if (filters?.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters?.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<GameGenreRecord>(sql, values);
    return result.rows;
  }

  async deleteGenre(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_game_genres WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  // =========================================================================
  // Game Catalog Operations
  // =========================================================================

  async createGame(game: Omit<GameCatalogRecord, 'id' | 'search_vector' | 'created_at' | 'updated_at'>): Promise<GameCatalogRecord> {
    const result = await this.query<GameCatalogRecord>(
      `INSERT INTO np_game_catalog (
        source_account_id, title, slug, platform_id, genre_id,
        release_date, developer, publisher, description, igdb_id,
        rom_hash_md5, rom_hash_sha1, rom_hash_sha256, rom_hash_crc32,
        rom_filename, rom_size_bytes, tier, rating,
        players_min, players_max, is_verified, metadata,
        search_vector
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22,
        to_tsvector('english', $2 || ' ' || COALESCE($7, '') || ' ' || COALESCE($8, '') || ' ' || COALESCE($9, ''))
      )
      RETURNING *`,
      [
        this.sourceAccountId, game.title, game.slug, game.platform_id,
        game.genre_id, game.release_date, game.developer, game.publisher,
        game.description, game.igdb_id, game.rom_hash_md5, game.rom_hash_sha1,
        game.rom_hash_sha256, game.rom_hash_crc32, game.rom_filename,
        game.rom_size_bytes, game.tier, game.rating, game.players_min,
        game.players_max, game.is_verified, JSON.stringify(game.metadata),
      ]
    );

    return result.rows[0];
  }

  async getGame(id: string): Promise<GameCatalogRecord | null> {
    const result = await this.query<GameCatalogRecord>(
      `SELECT * FROM np_game_catalog WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getGameBySlug(slug: string, platformId?: string): Promise<GameCatalogRecord | null> {
    const conditions: string[] = ['slug = $1', 'source_account_id = $2'];
    const values: unknown[] = [slug, this.sourceAccountId];

    if (platformId) {
      conditions.push('platform_id = $3');
      values.push(platformId);
    }

    const result = await this.query<GameCatalogRecord>(
      `SELECT * FROM np_game_catalog WHERE ${conditions.join(' AND ')}`,
      values
    );
    return result.rows[0] ?? null;
  }

  async lookupByHash(hash: string, hashType: 'md5' | 'sha1' | 'sha256' | 'crc32'): Promise<GameCatalogRecord | null> {
    const columnMap: Record<string, string> = {
      md5: 'rom_hash_md5',
      sha1: 'rom_hash_sha1',
      sha256: 'rom_hash_sha256',
      crc32: 'rom_hash_crc32',
    };

    const column = columnMap[hashType];
    if (!column) return null;

    const result = await this.query<GameCatalogRecord>(
      `SELECT * FROM np_game_catalog WHERE ${column} = $1 AND source_account_id = $2`,
      [hash.toLowerCase(), this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listGames(filters: {
    platformId?: string; genreId?: string; tier?: string;
    isVerified?: boolean; limit?: number; offset?: number;
  }): Promise<GameCatalogRecord[]> {
    const conditions: string[] = ['g.source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.platformId) {
      conditions.push(`g.platform_id = $${paramIndex}`);
      values.push(filters.platformId);
      paramIndex++;
    }

    if (filters.genreId) {
      conditions.push(`g.genre_id = $${paramIndex}`);
      values.push(filters.genreId);
      paramIndex++;
    }

    if (filters.tier) {
      conditions.push(`g.tier = $${paramIndex}`);
      values.push(filters.tier);
      paramIndex++;
    }

    if (filters.isVerified !== undefined) {
      conditions.push(`g.is_verified = $${paramIndex}`);
      values.push(filters.isVerified);
      paramIndex++;
    }

    // Suppress unused variable warning
    void paramIndex;

    let sql = `
      SELECT g.* FROM np_game_catalog g
      WHERE ${conditions.join(' AND ')}
      ORDER BY g.title ASC
    `;

    if (filters.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<GameCatalogRecord>(sql, values);
    return result.rows;
  }

  async searchGames(filters: {
    query: string; platformId?: string; genreId?: string;
    tier?: string; isVerified?: boolean; limit?: number;
  }): Promise<GameCatalogRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.query) {
      conditions.push(`search_vector @@ plainto_tsquery('english', $${paramIndex})`);
      values.push(filters.query);
      paramIndex++;
    }

    if (filters.platformId) {
      conditions.push(`platform_id = $${paramIndex}`);
      values.push(filters.platformId);
      paramIndex++;
    }

    if (filters.genreId) {
      conditions.push(`genre_id = $${paramIndex}`);
      values.push(filters.genreId);
      paramIndex++;
    }

    if (filters.tier) {
      conditions.push(`tier = $${paramIndex}`);
      values.push(filters.tier);
      paramIndex++;
    }

    if (filters.isVerified !== undefined) {
      conditions.push(`is_verified = $${paramIndex}`);
      values.push(filters.isVerified);
      paramIndex++;
    }

    // Suppress unused variable warning
    void paramIndex;

    const limit = filters.limit ?? 50;

    const result = await this.query<GameCatalogRecord>(
      `SELECT * FROM np_game_catalog
       WHERE ${conditions.join(' AND ')}
       ORDER BY title ASC
       LIMIT ${limit}`,
      values
    );

    return result.rows;
  }

  async updateGame(id: string, updates: Partial<GameCatalogRecord>): Promise<GameCatalogRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = [
      'title', 'slug', 'platform_id', 'genre_id', 'release_date',
      'developer', 'publisher', 'description', 'igdb_id',
      'rom_hash_md5', 'rom_hash_sha1', 'rom_hash_sha256', 'rom_hash_crc32',
      'rom_filename', 'rom_size_bytes', 'tier', 'rating',
      'players_min', 'players_max', 'is_verified',
    ];

    for (const [key, value] of Object.entries(updates)) {
      if (allowedFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (updates.metadata !== undefined) {
      fields.push(`metadata = $${paramIndex}::jsonb`);
      values.push(JSON.stringify(updates.metadata));
      paramIndex++;
    }

    if (fields.length === 0) {
      return this.getGame(id);
    }

    // Update search_vector if title or description changed
    if (updates.title || updates.description || updates.developer || updates.publisher) {
      fields.push(`search_vector = to_tsvector('english',
        COALESCE($${paramIndex}, title) || ' ' ||
        COALESCE(developer, '') || ' ' ||
        COALESCE(publisher, '') || ' ' ||
        COALESCE(description, '')
      )`);
      values.push(updates.title ?? null);
      paramIndex++;
    }

    fields.push(`updated_at = NOW()`);
    values.push(id, this.sourceAccountId);

    const result = await this.query<GameCatalogRecord>(
      `UPDATE np_game_catalog
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteGame(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_game_catalog WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  // =========================================================================
  // Game Metadata Operations
  // =========================================================================

  async upsertGameMetadata(metadata: Omit<GameMetadataRecord, 'id' | 'created_at' | 'updated_at'>): Promise<GameMetadataRecord> {
    const result = await this.query<GameMetadataRecord>(
      `INSERT INTO np_game_metadata (
        source_account_id, game_id, source, igdb_id, igdb_url,
        summary, storyline, total_rating, total_rating_count,
        aggregated_rating, aggregated_rating_count, first_release_date,
        genres, themes, keywords, game_modes, franchises,
        alternative_names, websites, age_ratings, involved_companies,
        raw_data, fetched_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
      ON CONFLICT (source_account_id, game_id, source) DO UPDATE SET
        igdb_id = EXCLUDED.igdb_id,
        igdb_url = EXCLUDED.igdb_url,
        summary = EXCLUDED.summary,
        storyline = EXCLUDED.storyline,
        total_rating = EXCLUDED.total_rating,
        total_rating_count = EXCLUDED.total_rating_count,
        aggregated_rating = EXCLUDED.aggregated_rating,
        aggregated_rating_count = EXCLUDED.aggregated_rating_count,
        first_release_date = EXCLUDED.first_release_date,
        genres = EXCLUDED.genres,
        themes = EXCLUDED.themes,
        keywords = EXCLUDED.keywords,
        game_modes = EXCLUDED.game_modes,
        franchises = EXCLUDED.franchises,
        alternative_names = EXCLUDED.alternative_names,
        websites = EXCLUDED.websites,
        age_ratings = EXCLUDED.age_ratings,
        involved_companies = EXCLUDED.involved_companies,
        raw_data = EXCLUDED.raw_data,
        fetched_at = EXCLUDED.fetched_at,
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, metadata.game_id, metadata.source,
        metadata.igdb_id, metadata.igdb_url, metadata.summary,
        metadata.storyline, metadata.total_rating, metadata.total_rating_count,
        metadata.aggregated_rating, metadata.aggregated_rating_count,
        metadata.first_release_date, metadata.genres, metadata.themes,
        metadata.keywords, metadata.game_modes, metadata.franchises,
        metadata.alternative_names, JSON.stringify(metadata.websites),
        JSON.stringify(metadata.age_ratings), JSON.stringify(metadata.involved_companies),
        JSON.stringify(metadata.raw_data), metadata.fetched_at,
      ]
    );

    return result.rows[0];
  }

  async getGameMetadata(gameId: string, source = 'igdb'): Promise<GameMetadataRecord | null> {
    const result = await this.query<GameMetadataRecord>(
      `SELECT * FROM np_game_metadata
       WHERE game_id = $1 AND source = $2 AND source_account_id = $3`,
      [gameId, source, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  // =========================================================================
  // Artwork Operations
  // =========================================================================

  async createArtwork(artwork: Omit<GameArtworkRecord, 'id' | 'created_at' | 'updated_at'>): Promise<GameArtworkRecord> {
    const result = await this.query<GameArtworkRecord>(
      `INSERT INTO np_game_artwork (
        source_account_id, game_id, artwork_type, url, local_path,
        width, height, mime_type, file_size_bytes, source,
        igdb_image_id, is_primary, sort_order, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      RETURNING *`,
      [
        this.sourceAccountId, artwork.game_id, artwork.artwork_type,
        artwork.url, artwork.local_path, artwork.width, artwork.height,
        artwork.mime_type, artwork.file_size_bytes, artwork.source,
        artwork.igdb_image_id, artwork.is_primary, artwork.sort_order,
        JSON.stringify(artwork.metadata),
      ]
    );

    return result.rows[0];
  }

  async listArtwork(gameId: string, artworkType?: string): Promise<GameArtworkRecord[]> {
    const conditions: string[] = ['game_id = $1', 'source_account_id = $2'];
    const values: unknown[] = [gameId, this.sourceAccountId];

    if (artworkType) {
      conditions.push('artwork_type = $3');
      values.push(artworkType);
    }

    const result = await this.query<GameArtworkRecord>(
      `SELECT * FROM np_game_artwork
       WHERE ${conditions.join(' AND ')}
       ORDER BY is_primary DESC, sort_order ASC`,
      values
    );

    return result.rows;
  }

  async deleteArtwork(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_game_artwork WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  async deleteArtworkForGame(gameId: string): Promise<number> {
    const count = await this.execute(
      `DELETE FROM np_game_artwork WHERE game_id = $1 AND source_account_id = $2`,
      [gameId, this.sourceAccountId]
    );
    return count;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<GameMetadataStats> {
    const result = await this.query<{
      total_games: string;
      verified_games: string;
      total_platforms: string;
      total_genres: string;
      total_artwork: string;
      total_metadata: string;
      games_with_igdb: string;
      games_with_hashes: string;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM np_game_catalog WHERE source_account_id = $1) as total_games,
        (SELECT COUNT(*) FROM np_game_catalog WHERE source_account_id = $1 AND is_verified = true) as verified_games,
        (SELECT COUNT(*) FROM np_game_platforms WHERE source_account_id = $1) as total_platforms,
        (SELECT COUNT(*) FROM np_game_genres WHERE source_account_id = $1) as total_genres,
        (SELECT COUNT(*) FROM np_game_artwork WHERE source_account_id = $1) as total_artwork,
        (SELECT COUNT(*) FROM np_game_metadata WHERE source_account_id = $1) as total_metadata,
        (SELECT COUNT(*) FROM np_game_catalog WHERE source_account_id = $1 AND igdb_id IS NOT NULL) as games_with_igdb,
        (SELECT COUNT(*) FROM np_game_catalog WHERE source_account_id = $1 AND (rom_hash_md5 IS NOT NULL OR rom_hash_sha1 IS NOT NULL OR rom_hash_crc32 IS NOT NULL)) as games_with_hashes`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];

    // Get tier breakdown
    const tierResult = await this.query<{ tier: string; count: string }>(
      `SELECT COALESCE(tier, 'untiered') as tier, COUNT(*) as count
       FROM np_game_catalog
       WHERE source_account_id = $1
       GROUP BY tier
       ORDER BY tier ASC`,
      [this.sourceAccountId]
    );

    const tierBreakdown: Record<string, number> = {};
    for (const tierRow of tierResult.rows) {
      tierBreakdown[tierRow.tier] = parseInt(tierRow.count, 10);
    }

    return {
      total_games: parseInt(row.total_games, 10),
      verified_games: parseInt(row.verified_games, 10),
      total_platforms: parseInt(row.total_platforms, 10),
      total_genres: parseInt(row.total_genres, 10),
      total_artwork: parseInt(row.total_artwork, 10),
      total_metadata: parseInt(row.total_metadata, 10),
      games_with_igdb: parseInt(row.games_with_igdb, 10),
      games_with_hashes: parseInt(row.games_with_hashes, 10),
      tier_breakdown: tierBreakdown,
    };
  }
}
