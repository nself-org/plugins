/**
 * Type definitions for the recommendation-engine plugin
 */

// =============================================================================
// Configuration
// =============================================================================

export interface RecommendationConfig {
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };
  server: {
    port: number;
    host: string;
  };
  redis: {
    url: string;
    enabled: boolean;
  };
  engine: {
    collaborativeWeight: number;
    contentWeight: number;
    cacheTtlSeconds: number;
    rebuildIntervalHours: number;
    minInteractionsForCollaborative: number;
  };
}

// =============================================================================
// Database Records
// =============================================================================

export interface UserProfileRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  interaction_count: number;
  preferred_genres: string[];
  avg_rating: number | null;
  last_interaction_at: Date | null;
  profile_vector: Record<string, number> | null;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
}

export interface ItemProfileRecord {
  id: string;
  source_account_id: string;
  media_id: string;
  title: string;
  media_type: string | null;
  genres: string[];
  cast_members: string[];
  director: string | null;
  description: string | null;
  tfidf_vector: Record<string, number> | null;
  view_count: number;
  avg_rating: number;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
}

export interface CachedRecommendationRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  media_id: string;
  score: number;
  reason: string | null;
  algorithm: string;
  created_at: Date;
  expires_at: Date;
}

export interface SimilarItemRecord {
  id: string;
  source_account_id: string;
  media_id: string;
  similar_media_id: string;
  similarity_score: number;
  created_at: Date;
}

export interface ModelStateRecord {
  id: string;
  source_account_id: string;
  last_rebuild: Date | null;
  item_count: number;
  user_count: number;
  model_ready: boolean;
  rebuild_duration_seconds: number | null;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// API Input/Output Types
// =============================================================================

export interface RecommendationItem {
  id: string;
  title: string;
  type: string | null;
  score: number;
  reason: string;
}

export interface SimilarItem {
  id: string;
  title: string;
  type: string | null;
  similarity_score: number;
}

export interface ModelStatus {
  last_rebuild: Date | null;
  item_count: number;
  user_count: number;
  model_ready: boolean;
  rebuild_duration_seconds: number | null;
}

export interface RebuildResult {
  started: boolean;
  estimated_time_seconds: number;
}

// =============================================================================
// Interaction Data (for building the user-item matrix)
// =============================================================================

export interface UserInteraction {
  user_id: string;
  media_id: string;
  rating: number;
  watch_time_pct: number;
}

// =============================================================================
// Sparse Vector Types
// =============================================================================

/** Sparse vector: maps dimension keys to float values */
export type SparseVector = Map<string, number>;

/** User-item interaction matrix: user_id -> (media_id -> score) */
export type UserItemMatrix = Map<string, Map<string, number>>;

/** Item-user matrix (transpose): media_id -> (user_id -> score) */
export type ItemUserMatrix = Map<string, Map<string, number>>;

// =============================================================================
// Algorithm Output
// =============================================================================

export interface ScoredItem {
  media_id: string;
  score: number;
  reason: string;
  algorithm: 'collaborative' | 'content-based' | 'hybrid' | 'popular';
}

// =============================================================================
// Query Types
// =============================================================================

export interface RecommendationQuery {
  limit?: string;
  type?: string;
}

export interface SimilarQuery {
  limit?: string;
}

// =============================================================================
// Upsert Input Types
// =============================================================================

export interface UpsertUserProfileInput {
  user_id: string;
  interaction_count: number;
  preferred_genres: string[];
  avg_rating: number | null;
  last_interaction_at: Date | null;
  profile_vector: Record<string, number> | null;
}

export interface UpsertItemProfileInput {
  media_id: string;
  title: string;
  media_type: string | null;
  genres: string[];
  cast_members: string[];
  director: string | null;
  description: string | null;
  tfidf_vector: Record<string, number> | null;
  view_count: number;
  avg_rating: number;
}
