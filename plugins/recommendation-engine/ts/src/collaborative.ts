/**
 * Collaborative filtering implementation
 *
 * Builds a user-item interaction matrix, computes user-user cosine similarity,
 * and recommends items that similar users liked. Uses sparse representations
 * for memory efficiency.
 */

import { createLogger } from '@nself/plugin-utils';
import type { UserInteraction, UserItemMatrix, ScoredItem, SparseVector } from './types.js';

const logger = createLogger('recommendation:collaborative');

// =============================================================================
// Sparse Vector Math
// =============================================================================

/**
 * Compute the dot product of two sparse vectors.
 */
function dotProduct(a: SparseVector, b: SparseVector): number {
  let result = 0;
  // Iterate over the smaller vector for efficiency
  const [smaller, larger] = a.size <= b.size ? [a, b] : [b, a];
  for (const [key, valA] of smaller) {
    const valB = larger.get(key);
    if (valB !== undefined) {
      result += valA * valB;
    }
  }
  return result;
}

/**
 * Compute the L2 norm (magnitude) of a sparse vector.
 */
function magnitude(v: SparseVector): number {
  let sumSq = 0;
  for (const val of v.values()) {
    sumSq += val * val;
  }
  return Math.sqrt(sumSq);
}

/**
 * Compute cosine similarity between two sparse vectors.
 * Returns 0 if either vector has zero magnitude.
 */
function cosineSimilarity(a: SparseVector, b: SparseVector): number {
  const magA = magnitude(a);
  const magB = magnitude(b);
  if (magA === 0 || magB === 0) return 0;
  return dotProduct(a, b) / (magA * magB);
}

// =============================================================================
// Collaborative Filtering Engine
// =============================================================================

export class CollaborativeFilter {
  /** user_id -> (media_id -> implicit_rating) */
  private userItemMatrix: UserItemMatrix = new Map();

  /** Precomputed user-user similarity cache: user_id -> [(similar_user_id, similarity)] */
  private userSimilarityCache: Map<string, Array<{ userId: string; similarity: number }>> = new Map();

  /** Set of all media_ids in the matrix */
  private allMediaIds: Set<string> = new Set();

  /** Maximum number of similar users to consider */
  private readonly maxSimilarUsers = 50;

  /** Minimum similarity threshold to include a user */
  private readonly minSimilarity = 0.05;

  /**
   * Build the user-item interaction matrix from raw interaction data.
   *
   * Each interaction contributes an implicit rating:
   *   implicit_score = (explicit_rating / 5.0) * 0.6 + watch_time_pct * 0.4
   *
   * This blends explicit ratings (if available) with engagement signals.
   */
  buildMatrix(interactions: UserInteraction[]): void {
    const start = Date.now();
    this.userItemMatrix.clear();
    this.userSimilarityCache.clear();
    this.allMediaIds.clear();

    for (const interaction of interactions) {
      const normalizedRating = Math.min(Math.max(interaction.rating / 5.0, 0), 1);
      const watchPct = Math.min(Math.max(interaction.watch_time_pct, 0), 1);
      const implicitScore = normalizedRating * 0.6 + watchPct * 0.4;

      let userRow = this.userItemMatrix.get(interaction.user_id);
      if (!userRow) {
        userRow = new Map();
        this.userItemMatrix.set(interaction.user_id, userRow);
      }

      // Keep the maximum score if there are duplicate interactions
      const existing = userRow.get(interaction.media_id) ?? 0;
      userRow.set(interaction.media_id, Math.max(existing, implicitScore));
      this.allMediaIds.add(interaction.media_id);
    }

    const duration = Date.now() - start;
    logger.info('User-item matrix built', {
      users: this.userItemMatrix.size,
      items: this.allMediaIds.size,
      interactions: interactions.length,
      duration,
    });
  }

  /**
   * Precompute similarity scores for all user pairs.
   * This is the most expensive step but only runs during model rebuild.
   */
  precomputeSimilarities(): void {
    const start = Date.now();
    this.userSimilarityCache.clear();

    const userIds = [...this.userItemMatrix.keys()];
    const userCount = userIds.length;

    for (let i = 0; i < userCount; i++) {
      const userId = userIds[i];
      const userVector = this.userItemMatrix.get(userId)!;
      const similarities: Array<{ userId: string; similarity: number }> = [];

      for (let j = 0; j < userCount; j++) {
        if (i === j) continue;
        const otherUserId = userIds[j];
        const otherVector = this.userItemMatrix.get(otherUserId)!;

        const sim = cosineSimilarity(userVector, otherVector);
        if (sim >= this.minSimilarity) {
          similarities.push({ userId: otherUserId, similarity: sim });
        }
      }

      // Sort by similarity descending, keep top N
      similarities.sort((a, b) => b.similarity - a.similarity);
      this.userSimilarityCache.set(userId, similarities.slice(0, this.maxSimilarUsers));
    }

    const duration = Date.now() - start;
    logger.info('User similarities precomputed', {
      users: userCount,
      duration,
    });
  }

  /**
   * Generate collaborative filtering recommendations for a user.
   *
   * Algorithm:
   * 1. Find the K most similar users (from precomputed cache)
   * 2. For each item those users interacted with (that target user hasn't seen):
   *    - Compute weighted average score: sum(similarity * rating) / sum(|similarity|)
   * 3. Sort by predicted score descending
   * 4. Normalize scores to 0-1 range
   */
  recommend(userId: string, limit: number, excludeMediaIds?: Set<string>): ScoredItem[] {
    const userVector = this.userItemMatrix.get(userId);
    if (!userVector || userVector.size === 0) {
      logger.debug('No interaction data for user', { userId });
      return [];
    }

    const similarUsers = this.userSimilarityCache.get(userId);
    if (!similarUsers || similarUsers.length === 0) {
      logger.debug('No similar users found', { userId });
      return [];
    }

    // Items the user has already interacted with
    const seenItems = new Set(userVector.keys());
    if (excludeMediaIds) {
      for (const id of excludeMediaIds) {
        seenItems.add(id);
      }
    }

    // Aggregate scores from similar users
    const candidateScores = new Map<string, { weightedSum: number; simSum: number }>();

    for (const { userId: simUserId, similarity } of similarUsers) {
      const simUserVector = this.userItemMatrix.get(simUserId);
      if (!simUserVector) continue;

      for (const [mediaId, rating] of simUserVector) {
        if (seenItems.has(mediaId)) continue;

        let candidate = candidateScores.get(mediaId);
        if (!candidate) {
          candidate = { weightedSum: 0, simSum: 0 };
          candidateScores.set(mediaId, candidate);
        }

        candidate.weightedSum += similarity * rating;
        candidate.simSum += Math.abs(similarity);
      }
    }

    // Compute predicted scores
    const predictions: ScoredItem[] = [];
    for (const [mediaId, { weightedSum, simSum }] of candidateScores) {
      if (simSum === 0) continue;
      const predictedScore = weightedSum / simSum;
      predictions.push({
        media_id: mediaId,
        score: predictedScore,
        reason: 'Users with similar taste enjoyed this',
        algorithm: 'collaborative',
      });
    }

    // Sort by score descending
    predictions.sort((a, b) => b.score - a.score);

    // Normalize to 0-1 range
    const topItems = predictions.slice(0, limit);
    if (topItems.length > 0) {
      const maxScore = topItems[0].score;
      const minScore = topItems[topItems.length - 1].score;
      const range = maxScore - minScore;

      if (range > 0) {
        for (const item of topItems) {
          item.score = (item.score - minScore) / range;
        }
      } else {
        // All scores the same -- normalize to 0.5
        for (const item of topItems) {
          item.score = 0.5;
        }
      }
    }

    return topItems;
  }

  /**
   * Get the number of interactions for a user.
   */
  getUserInteractionCount(userId: string): number {
    return this.userItemMatrix.get(userId)?.size ?? 0;
  }

  /**
   * Get the total number of users in the matrix.
   */
  getUserCount(): number {
    return this.userItemMatrix.size;
  }

  /**
   * Get the total number of items in the matrix.
   */
  getItemCount(): number {
    return this.allMediaIds.size;
  }

  /**
   * Check if the model has been built with data.
   */
  isReady(): boolean {
    return this.userItemMatrix.size > 0;
  }
}
