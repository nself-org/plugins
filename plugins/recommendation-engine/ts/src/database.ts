/**
 * Database client for recommendation-engine operations
 * Multi-app aware: all queries are scoped by source_account_id
 */

import { Pool, PoolClient } from 'pg';
import { config } from './config.js';
import {
  UserProfileRecord,
  ItemProfileRecord,
  CachedRecommendationRecord,
  SimilarItemRecord,
  ModelStateRecord,
  UserInteraction,
  UpsertUserProfileInput,
  UpsertItemProfileInput,
} from './types.js';

export class RecommendationDatabase {
  private pool: Pool;
  private sourceAccountId: string;

  constructor(sourceAccountId = 'primary') {
    this.pool = new Pool({
      host: config.database.host,
      port: config.database.port,
      database: config.database.database,
      user: config.database.user,
      password: config.database.password,
      ssl: config.database.ssl ? { rejectUnauthorized: false } : false,
    });
    this.sourceAccountId = sourceAccountId;
  }

  forSourceAccount(accountId: string): RecommendationDatabase {
    const scoped = Object.create(RecommendationDatabase.prototype) as RecommendationDatabase;
    scoped.pool = this.pool;
    scoped.sourceAccountId = accountId;
    return scoped;
  }

  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  async getClient(): Promise<PoolClient> {
    return this.pool.connect();
  }

  async close(): Promise<void> {
    await this.pool.end();
  }

  // =============================================================================
  // Schema Initialization
  // =============================================================================

  async initializeSchema(): Promise<void> {
    const client = await this.getClient();
    try {
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_recom_user_profiles (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          user_id VARCHAR(255) NOT NULL,
          interaction_count INTEGER DEFAULT 0,
          preferred_genres TEXT[],
          avg_rating REAL,
          last_interaction_at TIMESTAMPTZ,
          profile_vector JSONB,
          created_at TIMESTAMPTZ DEFAULT NOW(),
          updated_at TIMESTAMPTZ DEFAULT NOW(),
          synced_at TIMESTAMPTZ DEFAULT NOW(),
          UNIQUE(source_account_id, user_id)
        );
        CREATE INDEX IF NOT EXISTS idx_np_recom_user_profiles_user ON np_recom_user_profiles(source_account_id, user_id);
        CREATE INDEX IF NOT EXISTS idx_np_recom_user_profiles_interaction ON np_recom_user_profiles(source_account_id, interaction_count DESC);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS np_recom_item_profiles (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          media_id VARCHAR(255) NOT NULL,
          title TEXT NOT NULL,
          media_type VARCHAR(50),
          genres TEXT[],
          cast_members TEXT[],
          director TEXT,
          description TEXT,
          tfidf_vector JSONB,
          view_count INTEGER DEFAULT 0,
          avg_rating REAL DEFAULT 0,
          created_at TIMESTAMPTZ DEFAULT NOW(),
          updated_at TIMESTAMPTZ DEFAULT NOW(),
          synced_at TIMESTAMPTZ DEFAULT NOW(),
          UNIQUE(source_account_id, media_id)
        );
        CREATE INDEX IF NOT EXISTS idx_np_recom_item_profiles_media ON np_recom_item_profiles(source_account_id, media_id);
        CREATE INDEX IF NOT EXISTS idx_np_recom_item_profiles_type ON np_recom_item_profiles(source_account_id, media_type);
        CREATE INDEX IF NOT EXISTS idx_np_recom_item_profiles_rating ON np_recom_item_profiles(source_account_id, avg_rating DESC);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS np_recom_cached_recommendations (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          user_id VARCHAR(255) NOT NULL,
          media_id VARCHAR(255) NOT NULL,
          score REAL NOT NULL,
          reason TEXT,
          algorithm VARCHAR(50) NOT NULL,
          created_at TIMESTAMPTZ DEFAULT NOW(),
          expires_at TIMESTAMPTZ NOT NULL,
          UNIQUE(source_account_id, user_id, media_id)
        );
        CREATE INDEX IF NOT EXISTS idx_np_recom_cached_user ON np_recom_cached_recommendations(source_account_id, user_id, expires_at);
        CREATE INDEX IF NOT EXISTS idx_np_recom_cached_score ON np_recom_cached_recommendations(source_account_id, user_id, score DESC);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS np_recom_similar_items (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          media_id VARCHAR(255) NOT NULL,
          similar_media_id VARCHAR(255) NOT NULL,
          similarity_score REAL NOT NULL,
          created_at TIMESTAMPTZ DEFAULT NOW(),
          UNIQUE(source_account_id, media_id, similar_media_id)
        );
        CREATE INDEX IF NOT EXISTS idx_np_recom_similar ON np_recom_similar_items(source_account_id, media_id, similarity_score DESC);
      `);

      await client.query(`
        CREATE TABLE IF NOT EXISTS np_recom_model_state (
          id VARCHAR(50) PRIMARY KEY DEFAULT 'current',
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          last_rebuild TIMESTAMPTZ,
          item_count INTEGER DEFAULT 0,
          user_count INTEGER DEFAULT 0,
          model_ready BOOLEAN DEFAULT FALSE,
          rebuild_duration_seconds REAL,
          created_at TIMESTAMPTZ DEFAULT NOW(),
          updated_at TIMESTAMPTZ DEFAULT NOW()
        );
      `);
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // User Profiles
  // =============================================================================

  async upsertUserProfile(input: UpsertUserProfileInput): Promise<UserProfileRecord> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO np_recom_user_profiles (
          source_account_id, user_id, interaction_count, preferred_genres,
          avg_rating, last_interaction_at, profile_vector, updated_at, synced_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
        ON CONFLICT (source_account_id, user_id) DO UPDATE SET
          interaction_count = EXCLUDED.interaction_count,
          preferred_genres = EXCLUDED.preferred_genres,
          avg_rating = EXCLUDED.avg_rating,
          last_interaction_at = EXCLUDED.last_interaction_at,
          profile_vector = EXCLUDED.profile_vector,
          updated_at = NOW(),
          synced_at = NOW()
        RETURNING *`,
        [
          this.sourceAccountId, input.user_id, input.interaction_count,
          input.preferred_genres, input.avg_rating, input.last_interaction_at,
          input.profile_vector ? JSON.stringify(input.profile_vector) : null,
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getUserProfile(userId: string): Promise<UserProfileRecord | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_recom_user_profiles WHERE user_id = $1 AND source_account_id = $2',
        [userId, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async getAllUserProfiles(): Promise<UserProfileRecord[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_recom_user_profiles WHERE source_account_id = $1 ORDER BY interaction_count DESC',
        [this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Item Profiles
  // =============================================================================

  async upsertItemProfile(input: UpsertItemProfileInput): Promise<ItemProfileRecord> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO np_recom_item_profiles (
          source_account_id, media_id, title, media_type, genres, cast_members,
          director, description, tfidf_vector, view_count, avg_rating, updated_at, synced_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
        ON CONFLICT (source_account_id, media_id) DO UPDATE SET
          title = EXCLUDED.title,
          media_type = EXCLUDED.media_type,
          genres = EXCLUDED.genres,
          cast_members = EXCLUDED.cast_members,
          director = EXCLUDED.director,
          description = EXCLUDED.description,
          tfidf_vector = EXCLUDED.tfidf_vector,
          view_count = EXCLUDED.view_count,
          avg_rating = EXCLUDED.avg_rating,
          updated_at = NOW(),
          synced_at = NOW()
        RETURNING *`,
        [
          this.sourceAccountId, input.media_id, input.title, input.media_type,
          input.genres, input.cast_members, input.director, input.description,
          input.tfidf_vector ? JSON.stringify(input.tfidf_vector) : null,
          input.view_count, input.avg_rating,
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getItemProfile(mediaId: string): Promise<ItemProfileRecord | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_recom_item_profiles WHERE media_id = $1 AND source_account_id = $2',
        [mediaId, this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async getAllItemProfiles(): Promise<ItemProfileRecord[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_recom_item_profiles WHERE source_account_id = $1 ORDER BY avg_rating DESC',
        [this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async getItemProfilesByType(mediaType: string): Promise<ItemProfileRecord[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_recom_item_profiles WHERE source_account_id = $1 AND media_type = $2 ORDER BY avg_rating DESC',
        [this.sourceAccountId, mediaType]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async getItemProfilesByIds(mediaIds: string[]): Promise<ItemProfileRecord[]> {
    if (mediaIds.length === 0) return [];
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_recom_item_profiles WHERE source_account_id = $1 AND media_id = ANY($2)',
        [this.sourceAccountId, mediaIds]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // User Interactions (read from external tables or API)
  // =============================================================================

  /**
   * Fetch user interactions from the watch history / ratings data.
   * This queries the nself-tv content-progress tables if available,
   * or can be populated via the item profile data.
   * For standalone mode, we derive interactions from user_profiles + item_profiles.
   */
  async getUserInteractions(): Promise<UserInteraction[]> {
    const client = await this.getClient();
    try {
      // Derive implicit interactions from cached recommendations that were previously computed
      // In production, this would join with a watch_history or ratings table
      // For now, generate interactions from user profiles that have profile_vectors
      const result = await client.query(
        `SELECT
          u.user_id,
          i.media_id,
          COALESCE(i.avg_rating, 3.0) AS rating,
          0.5 AS watch_time_pct
        FROM np_recom_user_profiles u
        CROSS JOIN LATERAL (
          SELECT media_id, avg_rating
          FROM np_recom_item_profiles
          WHERE source_account_id = $1
          AND media_type = ANY(
            SELECT unnest(u.preferred_genres)
            WHERE u.preferred_genres IS NOT NULL
          )
          LIMIT 100
        ) i
        WHERE u.source_account_id = $1
        UNION ALL
        SELECT
          cr.user_id,
          cr.media_id,
          cr.score * 5.0 AS rating,
          cr.score AS watch_time_pct
        FROM np_recom_cached_recommendations cr
        WHERE cr.source_account_id = $1
          AND cr.algorithm != 'popular'`,
        [this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Cached Recommendations
  // =============================================================================

  async getCachedRecommendations(
    userId: string,
    limit: number,
    mediaType?: string
  ): Promise<CachedRecommendationRecord[]> {
    const client = await this.getClient();
    try {
      if (mediaType) {
        const result = await client.query(
          `SELECT cr.* FROM np_recom_cached_recommendations cr
           JOIN np_recom_item_profiles ip ON ip.media_id = cr.media_id AND ip.source_account_id = cr.source_account_id
           WHERE cr.user_id = $1 AND cr.source_account_id = $2
             AND cr.expires_at > NOW()
             AND ip.media_type = $3
           ORDER BY cr.score DESC LIMIT $4`,
          [userId, this.sourceAccountId, mediaType, limit]
        );
        return result.rows;
      }
      const result = await client.query(
        `SELECT * FROM np_recom_cached_recommendations
         WHERE user_id = $1 AND source_account_id = $2 AND expires_at > NOW()
         ORDER BY score DESC LIMIT $3`,
        [userId, this.sourceAccountId, limit]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async upsertCachedRecommendation(
    userId: string,
    mediaId: string,
    score: number,
    reason: string,
    algorithm: string,
    ttlSeconds: number
  ): Promise<void> {
    const client = await this.getClient();
    try {
      await client.query(
        `INSERT INTO np_recom_cached_recommendations (
          source_account_id, user_id, media_id, score, reason, algorithm, expires_at
        ) VALUES ($1, $2, $3, $4, $5, $6, NOW() + INTERVAL '1 second' * $7)
        ON CONFLICT (source_account_id, user_id, media_id) DO UPDATE SET
          score = EXCLUDED.score,
          reason = EXCLUDED.reason,
          algorithm = EXCLUDED.algorithm,
          created_at = NOW(),
          expires_at = EXCLUDED.expires_at`,
        [this.sourceAccountId, userId, mediaId, score, reason, algorithm, ttlSeconds]
      );
    } finally {
      client.release();
    }
  }

  async bulkUpsertCachedRecommendations(
    userId: string,
    items: Array<{ media_id: string; score: number; reason: string; algorithm: string }>,
    ttlSeconds: number
  ): Promise<void> {
    if (items.length === 0) return;
    const client = await this.getClient();
    try {
      // Clear existing cached recommendations for user first
      await client.query(
        'DELETE FROM np_recom_cached_recommendations WHERE user_id = $1 AND source_account_id = $2',
        [userId, this.sourceAccountId]
      );

      // Build bulk insert
      const values: unknown[] = [];
      const placeholders: string[] = [];
      let paramIdx = 1;

      for (const item of items) {
        placeholders.push(
          `($${paramIdx}, $${paramIdx + 1}, $${paramIdx + 2}, $${paramIdx + 3}, $${paramIdx + 4}, $${paramIdx + 5}, NOW() + INTERVAL '1 second' * $${paramIdx + 6})`
        );
        values.push(
          this.sourceAccountId, userId, item.media_id,
          item.score, item.reason, item.algorithm, ttlSeconds
        );
        paramIdx += 7;
      }

      await client.query(
        `INSERT INTO np_recom_cached_recommendations (
          source_account_id, user_id, media_id, score, reason, algorithm, expires_at
        ) VALUES ${placeholders.join(', ')}
        ON CONFLICT (source_account_id, user_id, media_id) DO UPDATE SET
          score = EXCLUDED.score,
          reason = EXCLUDED.reason,
          algorithm = EXCLUDED.algorithm,
          created_at = NOW(),
          expires_at = EXCLUDED.expires_at`,
        values
      );
    } finally {
      client.release();
    }
  }

  async clearExpiredRecommendations(): Promise<number> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'DELETE FROM np_recom_cached_recommendations WHERE expires_at < NOW() AND source_account_id = $1',
        [this.sourceAccountId]
      );
      return result.rowCount ?? 0;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Similar Items
  // =============================================================================

  async getSimilarItems(mediaId: string, limit: number): Promise<SimilarItemRecord[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `SELECT * FROM np_recom_similar_items
         WHERE media_id = $1 AND source_account_id = $2
         ORDER BY similarity_score DESC LIMIT $3`,
        [mediaId, this.sourceAccountId, limit]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async upsertSimilarItem(
    mediaId: string,
    similarMediaId: string,
    similarityScore: number
  ): Promise<void> {
    const client = await this.getClient();
    try {
      await client.query(
        `INSERT INTO np_recom_similar_items (
          source_account_id, media_id, similar_media_id, similarity_score
        ) VALUES ($1, $2, $3, $4)
        ON CONFLICT (source_account_id, media_id, similar_media_id) DO UPDATE SET
          similarity_score = EXCLUDED.similarity_score,
          created_at = NOW()`,
        [this.sourceAccountId, mediaId, similarMediaId, similarityScore]
      );
    } finally {
      client.release();
    }
  }

  async bulkUpsertSimilarItems(
    items: Array<{ media_id: string; similar_media_id: string; similarity_score: number }>
  ): Promise<void> {
    if (items.length === 0) return;
    const client = await this.getClient();
    try {
      // Clear existing similar items
      const uniqueMediaIds = [...new Set(items.map(i => i.media_id))];
      await client.query(
        'DELETE FROM np_recom_similar_items WHERE source_account_id = $1 AND media_id = ANY($2)',
        [this.sourceAccountId, uniqueMediaIds]
      );

      // Batch insert in chunks of 500 to avoid parameter limits
      const chunkSize = 500;
      for (let i = 0; i < items.length; i += chunkSize) {
        const chunk = items.slice(i, i + chunkSize);
        const values: unknown[] = [];
        const placeholders: string[] = [];
        let paramIdx = 1;

        for (const item of chunk) {
          placeholders.push(
            `($${paramIdx}, $${paramIdx + 1}, $${paramIdx + 2}, $${paramIdx + 3})`
          );
          values.push(this.sourceAccountId, item.media_id, item.similar_media_id, item.similarity_score);
          paramIdx += 4;
        }

        await client.query(
          `INSERT INTO np_recom_similar_items (
            source_account_id, media_id, similar_media_id, similarity_score
          ) VALUES ${placeholders.join(', ')}
          ON CONFLICT (source_account_id, media_id, similar_media_id) DO UPDATE SET
            similarity_score = EXCLUDED.similarity_score,
            created_at = NOW()`,
          values
        );
      }
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Model State
  // =============================================================================

  async getModelState(): Promise<ModelStateRecord | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        "SELECT * FROM np_recom_model_state WHERE id = 'current' AND source_account_id = $1",
        [this.sourceAccountId]
      );
      return result.rows[0] ?? null;
    } finally {
      client.release();
    }
  }

  async upsertModelState(
    itemCount: number,
    userCount: number,
    modelReady: boolean,
    rebuildDurationSeconds: number | null
  ): Promise<ModelStateRecord> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO np_recom_model_state (
          id, source_account_id, last_rebuild, item_count, user_count,
          model_ready, rebuild_duration_seconds, updated_at
        ) VALUES ('current', $1, NOW(), $2, $3, $4, $5, NOW())
        ON CONFLICT (id) DO UPDATE SET
          source_account_id = EXCLUDED.source_account_id,
          last_rebuild = NOW(),
          item_count = EXCLUDED.item_count,
          user_count = EXCLUDED.user_count,
          model_ready = EXCLUDED.model_ready,
          rebuild_duration_seconds = EXCLUDED.rebuild_duration_seconds,
          updated_at = NOW()
        RETURNING *`,
        [this.sourceAccountId, itemCount, userCount, modelReady, rebuildDurationSeconds]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Popular Items (fallback for cold-start)
  // =============================================================================

  async getPopularItems(limit: number, mediaType?: string): Promise<ItemProfileRecord[]> {
    const client = await this.getClient();
    try {
      if (mediaType) {
        const result = await client.query(
          `SELECT * FROM np_recom_item_profiles
           WHERE source_account_id = $1 AND media_type = $2
           ORDER BY (view_count * 0.3 + avg_rating * 0.7) DESC
           LIMIT $3`,
          [this.sourceAccountId, mediaType, limit]
        );
        return result.rows;
      }
      const result = await client.query(
        `SELECT * FROM np_recom_item_profiles
         WHERE source_account_id = $1
         ORDER BY (view_count * 0.3 + avg_rating * 0.7) DESC
         LIMIT $2`,
        [this.sourceAccountId, limit]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Statistics
  // =============================================================================

  async getStats(): Promise<Record<string, number>> {
    const client = await this.getClient();
    try {
      const users = await client.query(
        'SELECT COUNT(*) FROM np_recom_user_profiles WHERE source_account_id = $1',
        [this.sourceAccountId]
      );
      const items = await client.query(
        'SELECT COUNT(*) FROM np_recom_item_profiles WHERE source_account_id = $1',
        [this.sourceAccountId]
      );
      const cached = await client.query(
        'SELECT COUNT(*) FROM np_recom_cached_recommendations WHERE source_account_id = $1 AND expires_at > NOW()',
        [this.sourceAccountId]
      );
      const similar = await client.query(
        'SELECT COUNT(*) FROM np_recom_similar_items WHERE source_account_id = $1',
        [this.sourceAccountId]
      );
      return {
        total_users: parseInt(users.rows[0].count, 10),
        total_items: parseInt(items.rows[0].count, 10),
        active_cached_recommendations: parseInt(cached.rows[0].count, 10),
        total_similar_pairs: parseInt(similar.rows[0].count, 10),
      };
    } finally {
      client.release();
    }
  }
}

export const db = new RecommendationDatabase();
