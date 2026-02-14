/**
 * Hybrid recommendation engine
 *
 * Blends collaborative filtering and content-based filtering results,
 * handles cold-start scenarios, manages Redis caching, and orchestrates
 * model rebuilds on a configurable schedule.
 */

import { Redis } from 'ioredis';
import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { RecommendationDatabase } from './database.js';
import { CollaborativeFilter } from './collaborative.js';
import { ContentBasedFilter } from './content-based.js';
import type {
  RecommendationItem,
  SimilarItem,
  ModelStatus,
  RebuildResult,
  ScoredItem,
  ItemProfileRecord,
} from './types.js';

const logger = createLogger('recommendation:engine');

export class RecommendationEngine {
  private db: RecommendationDatabase;
  private collaborative: CollaborativeFilter;
  private contentBased: ContentBasedFilter;
  private redis: Redis | null = null;
  private rebuildTimer: ReturnType<typeof setInterval> | null = null;
  private isRebuilding = false;

  constructor(db: RecommendationDatabase) {
    this.db = db;
    this.collaborative = new CollaborativeFilter();
    this.contentBased = new ContentBasedFilter();
  }

  /**
   * Initialize the engine: connect Redis (if configured) and start the rebuild timer.
   */
  async initialize(): Promise<void> {
    // Connect Redis if available
    if (config.redis.enabled) {
      try {
        this.redis = new Redis(config.redis.url, {
          maxRetriesPerRequest: 3,
          lazyConnect: true,
          retryStrategy: (times: number) => {
            if (times > 3) return null;
            return Math.min(times * 200, 2000);
          },
        });
        await this.redis.connect();
        logger.info('Redis connected for recommendation caching');
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.warn('Redis connection failed, falling back to DB-only caching', { error: message });
        this.redis = null;
      }
    }

    // Start periodic rebuild
    const intervalMs = config.engine.rebuildIntervalHours * 60 * 60 * 1000;
    this.rebuildTimer = setInterval(() => {
      this.rebuild().catch(err => {
        const message = err instanceof Error ? err.message : 'Unknown error';
        logger.error('Scheduled rebuild failed', { error: message });
      });
    }, intervalMs);

    logger.info('Recommendation engine initialized', {
      collaborativeWeight: config.engine.collaborativeWeight,
      contentWeight: config.engine.contentWeight,
      rebuildIntervalHours: config.engine.rebuildIntervalHours,
      redisEnabled: this.redis !== null,
    });
  }

  /**
   * Shut down the engine: stop timers, disconnect Redis.
   */
  async shutdown(): Promise<void> {
    if (this.rebuildTimer) {
      clearInterval(this.rebuildTimer);
      this.rebuildTimer = null;
    }
    if (this.redis) {
      await this.redis.quit();
      this.redis = null;
    }
    logger.info('Recommendation engine shut down');
  }

  /**
   * Rebuild the entire recommendation model.
   *
   * Steps:
   * 1. Load all item profiles from DB
   * 2. Build content-based TF-IDF vectors and similarities
   * 3. Load user interactions and build collaborative matrix
   * 4. Precompute user-user similarities
   * 5. Store computed similarity data back to DB
   * 6. Update model state
   * 7. Flush Redis cache
   */
  async rebuild(): Promise<RebuildResult> {
    if (this.isRebuilding) {
      logger.warn('Rebuild already in progress, skipping');
      return { started: false, estimated_time_seconds: 0 };
    }

    this.isRebuilding = true;
    const startTime = Date.now();
    logger.info('Starting model rebuild...');

    try {
      // Step 1: Load item profiles
      const items = await this.db.getAllItemProfiles();
      logger.info('Loaded item profiles', { count: items.length });

      if (items.length === 0) {
        logger.warn('No items found, model will be empty');
        await this.db.upsertModelState(0, 0, false, 0);
        return { started: true, estimated_time_seconds: 0 };
      }

      // Step 2: Build content-based model
      this.contentBased.buildVectors(items);
      this.contentBased.precomputeSimilarities();

      // Store TF-IDF vectors back to item profiles
      for (const item of items) {
        const vector = this.contentBased.getItemVector(item.media_id);
        if (vector) {
          await this.db.upsertItemProfile({
            media_id: item.media_id,
            title: item.title,
            media_type: item.media_type,
            genres: item.genres ?? [],
            cast_members: item.cast_members ?? [],
            director: item.director,
            description: item.description,
            tfidf_vector: vector,
            view_count: item.view_count,
            avg_rating: item.avg_rating,
          });
        }
      }

      // Store similarity pairs
      const similarityPairs = this.contentBased.getAllSimilarityPairs();
      if (similarityPairs.length > 0) {
        await this.db.bulkUpsertSimilarItems(similarityPairs);
        logger.info('Stored similarity pairs', { count: similarityPairs.length });
      }

      // Step 3: Load interactions and build collaborative model
      const interactions = await this.db.getUserInteractions();
      logger.info('Loaded user interactions', { count: interactions.length });

      if (interactions.length > 0) {
        this.collaborative.buildMatrix(interactions);
        this.collaborative.precomputeSimilarities();
      }

      // Step 4: Compute user profile vectors and store
      const userProfiles = await this.db.getAllUserProfiles();
      for (const profile of userProfiles) {
        // Build a simple user preference vector from their genre preferences
        const profileVector: Record<string, number> = {};
        if (profile.preferred_genres) {
          for (const genre of profile.preferred_genres) {
            profileVector[`genre_${genre.toLowerCase()}`] = 1.0;
          }
        }
        await this.db.upsertUserProfile({
          user_id: profile.user_id,
          interaction_count: profile.interaction_count,
          preferred_genres: profile.preferred_genres ?? [],
          avg_rating: profile.avg_rating,
          last_interaction_at: profile.last_interaction_at,
          profile_vector: Object.keys(profileVector).length > 0 ? profileVector : null,
        });
      }

      // Step 5: Update model state
      const durationSeconds = (Date.now() - startTime) / 1000;
      const userCount = this.collaborative.getUserCount() || userProfiles.length;
      await this.db.upsertModelState(items.length, userCount, true, durationSeconds);

      // Step 6: Flush Redis cache
      if (this.redis) {
        const keys = await this.redis.keys('recom:*');
        if (keys.length > 0) {
          await this.redis.del(...keys);
        }
        logger.info('Redis cache flushed', { keys: keys.length });
      }

      this.isRebuilding = false;

      logger.success('Model rebuild complete', {
        items: items.length,
        users: userCount,
        similarityPairs: similarityPairs.length,
        durationSeconds: Math.round(durationSeconds * 100) / 100,
      });

      return {
        started: true,
        estimated_time_seconds: Math.round(durationSeconds),
      };
    } catch (error) {
      this.isRebuilding = false;
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Model rebuild failed', { error: message });

      // Mark model as not ready on failure
      await this.db.upsertModelState(0, 0, false, null).catch(() => {
        // Ignore secondary failure
      });

      throw error;
    }
  }

  /**
   * Get personalized recommendations for a user.
   *
   * Flow:
   * 1. Check Redis cache
   * 2. Check DB cache (if Redis miss)
   * 3. If cache miss, compute fresh recommendations:
   *    a. If user has enough interactions -> hybrid (collaborative + content)
   *    b. If user has some interactions -> content-based only
   *    c. If new user -> popular items fallback
   * 4. Cache results and return
   */
  async getRecommendations(
    userId: string,
    limit: number,
    mediaType?: string
  ): Promise<RecommendationItem[]> {
    // Step 1: Check Redis cache
    const cacheKey = `recom:user:${this.db.getSourceAccountId()}:${userId}:${mediaType ?? 'all'}:${limit}`;
    if (this.redis) {
      try {
        const cached = await this.redis.get(cacheKey);
        if (cached) {
          logger.debug('Redis cache hit', { userId });
          return JSON.parse(cached) as RecommendationItem[];
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.warn('Redis read error', { error: message });
      }
    }

    // Step 2: Check DB cache
    const dbCached = await this.db.getCachedRecommendations(userId, limit, mediaType);
    if (dbCached.length > 0) {
      const items = await this.enrichRecommendations(dbCached.map(r => ({
        media_id: r.media_id,
        score: r.score,
        reason: r.reason ?? 'Recommended for you',
      })));

      // Store in Redis for faster subsequent access
      await this.cacheInRedis(cacheKey, items);
      return items.slice(0, limit);
    }

    // Step 3: Compute fresh recommendations
    const recommendations = await this.computeRecommendations(userId, limit, mediaType);

    // Step 4: Cache results
    if (recommendations.length > 0) {
      await this.db.bulkUpsertCachedRecommendations(
        userId,
        recommendations.map(r => ({
          media_id: r.id,
          score: r.score,
          reason: r.reason,
          algorithm: 'hybrid',
        })),
        config.engine.cacheTtlSeconds
      );
      await this.cacheInRedis(cacheKey, recommendations);
    }

    return recommendations;
  }

  /**
   * Compute fresh recommendations using the hybrid approach.
   */
  private async computeRecommendations(
    userId: string,
    limit: number,
    mediaType?: string
  ): Promise<RecommendationItem[]> {
    const userProfile = await this.db.getUserProfile(userId);
    const interactionCount = userProfile?.interaction_count ?? 0;

    // Determine which algorithms to use based on user's interaction history
    const useCollaborative = interactionCount >= config.engine.minInteractionsForCollaborative
      && this.collaborative.isReady();
    const useContentBased = this.contentBased.isReady();

    let scoredItems: ScoredItem[] = [];

    if (useCollaborative && useContentBased) {
      // Hybrid: blend both algorithms
      scoredItems = this.blendRecommendations(userId, userProfile, limit * 3);
    } else if (useContentBased && interactionCount > 0) {
      // Content-based only (cold-start for collaborative)
      scoredItems = await this.getContentBasedRecommendations(userId, userProfile, limit * 2);
    } else {
      // Full cold-start: return popular items
      return this.getPopularRecommendations(limit, mediaType);
    }

    // Filter by media type if requested
    if (mediaType) {
      const typeItems = await this.filterByMediaType(scoredItems, mediaType);
      scoredItems = typeItems;
    }

    // Deduplicate and take top N
    const seen = new Set<string>();
    const deduped: ScoredItem[] = [];
    for (const item of scoredItems) {
      if (!seen.has(item.media_id)) {
        seen.add(item.media_id);
        deduped.push(item);
      }
    }

    const topItems = deduped.slice(0, limit);
    return this.enrichScoredItems(topItems);
  }

  /**
   * Blend collaborative and content-based results using configured weights.
   *
   * 1. Get collaborative recommendations (60% default)
   * 2. Get content-based recommendations (40% default)
   * 3. Merge: for overlapping items, combine scores with weights
   * 4. For non-overlapping items, use weighted single score
   * 5. Re-normalize to 0-1
   */
  private blendRecommendations(
    userId: string,
    userProfile: { preferred_genres?: string[] | null; profile_vector?: Record<string, number> | null } | null,
    candidateCount: number
  ): ScoredItem[] {
    const collabWeight = config.engine.collaborativeWeight;
    const contentWeight = config.engine.contentWeight;

    // Get collaborative recommendations
    const collabItems = this.collaborative.recommend(userId, candidateCount);

    // Get content-based recommendations
    const userLiked = this.getUserLikedItemsFromProfile(userProfile);
    const contentItems = this.contentBased.recommend(userLiked, candidateCount);

    // Build lookup maps
    const collabMap = new Map<string, ScoredItem>();
    for (const item of collabItems) {
      collabMap.set(item.media_id, item);
    }

    const contentMap = new Map<string, ScoredItem>();
    for (const item of contentItems) {
      contentMap.set(item.media_id, item);
    }

    // Merge
    const allMediaIds = new Set([...collabMap.keys(), ...contentMap.keys()]);
    const merged: ScoredItem[] = [];

    for (const mediaId of allMediaIds) {
      const collabItem = collabMap.get(mediaId);
      const contentItem = contentMap.get(mediaId);

      let finalScore: number;
      let reason: string;

      if (collabItem && contentItem) {
        // Both algorithms recommend this item -- strong signal
        finalScore = collabItem.score * collabWeight + contentItem.score * contentWeight;
        reason = 'Highly recommended based on similar users and content match';
      } else if (collabItem) {
        finalScore = collabItem.score * collabWeight;
        reason = collabItem.reason;
      } else {
        finalScore = contentItem!.score * contentWeight;
        reason = contentItem!.reason;
      }

      merged.push({
        media_id: mediaId,
        score: finalScore,
        reason,
        algorithm: 'hybrid',
      });
    }

    // Sort descending
    merged.sort((a, b) => b.score - a.score);

    // Normalize to 0-1
    if (merged.length > 0) {
      const maxScore = merged[0].score;
      const minScore = merged[merged.length - 1].score;
      const range = maxScore - minScore;

      if (range > 0) {
        for (const item of merged) {
          item.score = (item.score - minScore) / range;
        }
      } else {
        for (const item of merged) {
          item.score = 0.5;
        }
      }
    }

    return merged;
  }

  /**
   * Get content-based recommendations only (for users with some interactions
   * but not enough for collaborative filtering).
   */
  private async getContentBasedRecommendations(
    _userId: string,
    userProfile: { preferred_genres?: string[] | null; profile_vector?: Record<string, number> | null } | null,
    limit: number
  ): Promise<ScoredItem[]> {
    const userLiked = this.getUserLikedItemsFromProfile(userProfile);
    return this.contentBased.recommend(userLiked, limit);
  }

  /**
   * Build a list of "liked items" from user profile data.
   * Uses the profile vector and preferred genres to construct seed items.
   */
  private getUserLikedItemsFromProfile(
    userProfile: { preferred_genres?: string[] | null; profile_vector?: Record<string, number> | null } | null
  ): Array<{ mediaId: string; rating: number }> {
    if (!userProfile) return [];

    // If the user profile has a vector, we can reconstruct their preferences
    // For now, use the preferred genres to find representative items
    const likedItems: Array<{ mediaId: string; rating: number }> = [];

    // Use profile_vector keys that look like media IDs
    if (userProfile.profile_vector) {
      for (const [key, value] of Object.entries(userProfile.profile_vector)) {
        if (!key.startsWith('genre_')) {
          likedItems.push({ mediaId: key, rating: value * 5 });
        }
      }
    }

    return likedItems;
  }

  /**
   * Fallback: return popular items for cold-start users.
   */
  private async getPopularRecommendations(
    limit: number,
    mediaType?: string
  ): Promise<RecommendationItem[]> {
    const popularItems = await this.db.getPopularItems(limit, mediaType);
    return popularItems.map((item, index) => ({
      id: item.media_id,
      title: item.title,
      type: item.media_type,
      score: Math.max(0, 1 - index * 0.05), // Decreasing score by rank
      reason: 'Popular among all users',
    }));
  }

  /**
   * Filter scored items by media type using DB lookup.
   */
  private async filterByMediaType(items: ScoredItem[], mediaType: string): Promise<ScoredItem[]> {
    const mediaIds = items.map(i => i.media_id);
    const profiles = await this.db.getItemProfilesByIds(mediaIds);
    const typeSet = new Set(
      profiles.filter(p => p.media_type === mediaType).map(p => p.media_id)
    );
    return items.filter(i => typeSet.has(i.media_id));
  }

  /**
   * Enrich scored items with title and type from item profiles.
   */
  private async enrichScoredItems(items: ScoredItem[]): Promise<RecommendationItem[]> {
    if (items.length === 0) return [];

    const mediaIds = items.map(i => i.media_id);
    const profiles = await this.db.getItemProfilesByIds(mediaIds);
    const profileMap = new Map<string, ItemProfileRecord>();
    for (const p of profiles) {
      profileMap.set(p.media_id, p);
    }

    return items.map(item => {
      const profile = profileMap.get(item.media_id);
      return {
        id: item.media_id,
        title: profile?.title ?? 'Unknown',
        type: profile?.media_type ?? null,
        score: Math.round(item.score * 1000) / 1000,
        reason: item.reason,
      };
    });
  }

  /**
   * Enrich cached recommendation records with title and type.
   */
  private async enrichRecommendations(
    items: Array<{ media_id: string; score: number; reason: string }>
  ): Promise<RecommendationItem[]> {
    const mediaIds = items.map(i => i.media_id);
    const profiles = await this.db.getItemProfilesByIds(mediaIds);
    const profileMap = new Map<string, ItemProfileRecord>();
    for (const p of profiles) {
      profileMap.set(p.media_id, p);
    }

    return items.map(item => {
      const profile = profileMap.get(item.media_id);
      return {
        id: item.media_id,
        title: profile?.title ?? 'Unknown',
        type: profile?.media_type ?? null,
        score: Math.round(item.score * 1000) / 1000,
        reason: item.reason,
      };
    });
  }

  /**
   * Get similar content for a given media item.
   */
  async getSimilarItems(mediaId: string, limit: number): Promise<SimilarItem[]> {
    // Check Redis cache
    const cacheKey = `recom:similar:${this.db.getSourceAccountId()}:${mediaId}:${limit}`;
    if (this.redis) {
      try {
        const cached = await this.redis.get(cacheKey);
        if (cached) {
          return JSON.parse(cached) as SimilarItem[];
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.warn('Redis read error for similar items', { error: message });
      }
    }

    // Try in-memory model first
    let similarPairs: Array<{ mediaId: string; score: number }> = [];
    if (this.contentBased.isReady()) {
      similarPairs = this.contentBased.getSimilarItems(mediaId, limit);
    }

    // Fall back to DB if in-memory model not available
    if (similarPairs.length === 0) {
      const dbSimilar = await this.db.getSimilarItems(mediaId, limit);
      similarPairs = dbSimilar.map(s => ({
        mediaId: s.similar_media_id,
        score: s.similarity_score,
      }));
    }

    // Enrich with item details
    const mediaIds = similarPairs.map(p => p.mediaId);
    const profiles = await this.db.getItemProfilesByIds(mediaIds);
    const profileMap = new Map<string, ItemProfileRecord>();
    for (const p of profiles) {
      profileMap.set(p.media_id, p);
    }

    const results: SimilarItem[] = similarPairs.map(pair => {
      const profile = profileMap.get(pair.mediaId);
      return {
        id: pair.mediaId,
        title: profile?.title ?? 'Unknown',
        type: profile?.media_type ?? null,
        similarity_score: Math.round(pair.score * 1000) / 1000,
      };
    });

    // Cache in Redis
    await this.cacheInRedis(cacheKey, results);

    return results;
  }

  /**
   * Get the current model status.
   */
  async getStatus(): Promise<ModelStatus> {
    const state = await this.db.getModelState();
    if (!state) {
      return {
        last_rebuild: null,
        item_count: 0,
        user_count: 0,
        model_ready: false,
        rebuild_duration_seconds: null,
      };
    }
    return {
      last_rebuild: state.last_rebuild,
      item_count: state.item_count,
      user_count: state.user_count,
      model_ready: state.model_ready,
      rebuild_duration_seconds: state.rebuild_duration_seconds,
    };
  }

  /**
   * Cache data in Redis with configured TTL.
   */
  private async cacheInRedis(key: string, data: unknown): Promise<void> {
    if (!this.redis) return;
    try {
      await this.redis.set(key, JSON.stringify(data), 'EX', config.engine.cacheTtlSeconds);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.warn('Redis write error', { error: message });
    }
  }

  /**
   * Check if the engine is ready to serve recommendations.
   */
  isModelReady(): boolean {
    return this.collaborative.isReady() || this.contentBased.isReady();
  }
}
