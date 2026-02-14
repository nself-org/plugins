/**
 * Content-based filtering implementation with TF-IDF
 *
 * Builds TF-IDF vectors from item metadata (genres, cast, director, description),
 * computes pairwise cosine similarity between items, and recommends items
 * similar to what the user has watched or rated highly.
 */

import { createLogger } from '@nself/plugin-utils';
import type { ItemProfileRecord, ScoredItem, SparseVector } from './types.js';

const logger = createLogger('recommendation:content-based');

// =============================================================================
// Text Processing
// =============================================================================

/** Simple stop words to filter out of descriptions */
const STOP_WORDS = new Set([
  'a', 'an', 'the', 'and', 'or', 'but', 'in', 'on', 'at', 'to', 'for',
  'of', 'with', 'by', 'from', 'is', 'it', 'this', 'that', 'was', 'are',
  'be', 'been', 'being', 'have', 'has', 'had', 'do', 'does', 'did',
  'will', 'would', 'could', 'should', 'may', 'might', 'can', 'not',
  'so', 'if', 'then', 'than', 'too', 'very', 'just', 'about', 'up',
  'out', 'no', 'as', 'all', 'each', 'every', 'both', 'few', 'more',
  'most', 'other', 'some', 'such', 'only', 'own', 'same', 'its',
  'he', 'she', 'her', 'his', 'they', 'them', 'their', 'we', 'us',
  'who', 'which', 'what', 'when', 'where', 'how', 'why', 'while',
  'after', 'before', 'during', 'between', 'through', 'into', 'over',
]);

/**
 * Tokenize text into lowercase words, filtering stop words and short tokens.
 */
function tokenize(text: string): string[] {
  return text
    .toLowerCase()
    .replace(/[^a-z0-9\s]/g, ' ')
    .split(/\s+/)
    .filter(word => word.length > 2 && !STOP_WORDS.has(word));
}

// =============================================================================
// TF-IDF Implementation
// =============================================================================

export class ContentBasedFilter {
  /** media_id -> TF-IDF sparse vector */
  private itemVectors: Map<string, SparseVector> = new Map();

  /** Precomputed pairwise similarity: media_id -> [(similar_media_id, score)] */
  private similarityCache: Map<string, Array<{ mediaId: string; score: number }>> = new Map();

  /** Document frequency: term -> number of documents containing the term */
  private documentFrequency: Map<string, number> = new Map();

  /** Total number of documents (items) */
  private totalDocuments = 0;

  /** Maximum similar items to cache per item */
  private readonly maxSimilarPerItem = 30;

  /** Minimum similarity to store */
  private readonly minSimilarity = 0.01;

  /**
   * Build TF-IDF vectors for all items.
   *
   * Each item's "document" is constructed from:
   * - Genres (weighted 3x via repetition)
   * - Cast members (weighted 2x)
   * - Director (weighted 2x)
   * - Description (weighted 1x)
   *
   * TF = term count in document / total terms in document
   * IDF = log(N / (1 + df)) where N = total docs, df = docs containing term
   * TF-IDF = TF * IDF
   */
  buildVectors(items: ItemProfileRecord[]): void {
    const start = Date.now();
    this.itemVectors.clear();
    this.documentFrequency.clear();
    this.similarityCache.clear();
    this.totalDocuments = items.length;

    if (items.length === 0) {
      logger.warn('No items to build TF-IDF vectors from');
      return;
    }

    // Step 1: Build term frequency for each document and compute document frequencies
    const documentTermFrequencies: Map<string, Map<string, number>> = new Map();

    for (const item of items) {
      const terms = this.extractTerms(item);
      const termCounts = new Map<string, number>();

      for (const term of terms) {
        termCounts.set(term, (termCounts.get(term) ?? 0) + 1);
      }

      documentTermFrequencies.set(item.media_id, termCounts);

      // Count document frequency (each unique term in this document)
      for (const term of termCounts.keys()) {
        this.documentFrequency.set(term, (this.documentFrequency.get(term) ?? 0) + 1);
      }
    }

    // Step 2: Compute TF-IDF vectors
    for (const item of items) {
      const termCounts = documentTermFrequencies.get(item.media_id)!;
      const totalTerms = [...termCounts.values()].reduce((sum, count) => sum + count, 0);

      if (totalTerms === 0) continue;

      const tfidfVector: SparseVector = new Map();

      for (const [term, count] of termCounts) {
        const tf = count / totalTerms;
        const df = this.documentFrequency.get(term) ?? 0;
        const idf = Math.log(this.totalDocuments / (1 + df));
        const tfidf = tf * idf;

        if (tfidf > 0) {
          tfidfVector.set(term, tfidf);
        }
      }

      this.itemVectors.set(item.media_id, tfidfVector);
    }

    const duration = Date.now() - start;
    logger.info('TF-IDF vectors built', {
      items: items.length,
      uniqueTerms: this.documentFrequency.size,
      duration,
    });
  }

  /**
   * Extract weighted terms from an item profile.
   * Genres and key metadata are weighted more heavily by repeating their tokens.
   */
  private extractTerms(item: ItemProfileRecord): string[] {
    const terms: string[] = [];

    // Genres: weight 3x by prepending genre: prefix and repeating
    if (item.genres && item.genres.length > 0) {
      for (const genre of item.genres) {
        const genreToken = `genre_${genre.toLowerCase().replace(/[^a-z0-9]/g, '_')}`;
        terms.push(genreToken, genreToken, genreToken);
      }
    }

    // Cast members: weight 2x
    if (item.cast_members && item.cast_members.length > 0) {
      for (const member of item.cast_members) {
        const memberToken = `cast_${member.toLowerCase().replace(/[^a-z0-9]/g, '_')}`;
        terms.push(memberToken, memberToken);
      }
    }

    // Director: weight 2x
    if (item.director) {
      const directorToken = `director_${item.director.toLowerCase().replace(/[^a-z0-9]/g, '_')}`;
      terms.push(directorToken, directorToken);
    }

    // Media type: weight 1x
    if (item.media_type) {
      terms.push(`type_${item.media_type.toLowerCase()}`);
    }

    // Description: weight 1x, tokenized naturally
    if (item.description) {
      terms.push(...tokenize(item.description));
    }

    // Title: weight 1x
    if (item.title) {
      terms.push(...tokenize(item.title));
    }

    return terms;
  }

  /**
   * Precompute pairwise similarities between all items.
   * Uses cosine similarity on TF-IDF vectors.
   */
  precomputeSimilarities(): void {
    const start = Date.now();
    this.similarityCache.clear();

    const mediaIds = [...this.itemVectors.keys()];
    const count = mediaIds.length;

    for (let i = 0; i < count; i++) {
      const mediaIdA = mediaIds[i];
      const vectorA = this.itemVectors.get(mediaIdA)!;
      const magA = vectorMagnitude(vectorA);

      if (magA === 0) continue;

      const similarities: Array<{ mediaId: string; score: number }> = [];

      for (let j = 0; j < count; j++) {
        if (i === j) continue;
        const mediaIdB = mediaIds[j];
        const vectorB = this.itemVectors.get(mediaIdB)!;
        const magB = vectorMagnitude(vectorB);

        if (magB === 0) continue;

        const dot = vectorDotProduct(vectorA, vectorB);
        const sim = dot / (magA * magB);

        if (sim >= this.minSimilarity) {
          similarities.push({ mediaId: mediaIdB, score: sim });
        }
      }

      // Sort descending, keep top N
      similarities.sort((a, b) => b.score - a.score);
      this.similarityCache.set(mediaIdA, similarities.slice(0, this.maxSimilarPerItem));
    }

    const duration = Date.now() - start;
    logger.info('Content similarities precomputed', {
      items: count,
      duration,
    });
  }

  /**
   * Get the most similar items to a given media item.
   */
  getSimilarItems(mediaId: string, limit: number): Array<{ mediaId: string; score: number }> {
    const cached = this.similarityCache.get(mediaId);
    if (!cached) return [];
    return cached.slice(0, limit);
  }

  /**
   * Generate content-based recommendations for a user.
   *
   * Algorithm:
   * 1. Get the user's top-rated / most-watched items
   * 2. For each of those items, find the most similar items
   * 3. Aggregate similarity scores across all seed items
   * 4. Exclude already-seen items
   * 5. Normalize scores to 0-1
   */
  recommend(
    userLikedItems: Array<{ mediaId: string; rating: number }>,
    limit: number,
    excludeMediaIds?: Set<string>
  ): ScoredItem[] {
    if (userLikedItems.length === 0) {
      return [];
    }

    const seenItems = new Set(userLikedItems.map(i => i.mediaId));
    if (excludeMediaIds) {
      for (const id of excludeMediaIds) {
        seenItems.add(id);
      }
    }

    // Aggregate scores from similar items to user's liked items
    const candidateScores = new Map<string, { totalScore: number; count: number; bestReason: string }>();

    for (const liked of userLikedItems) {
      const similarItems = this.similarityCache.get(liked.mediaId);
      if (!similarItems) continue;

      // Weight contribution by the user's rating of the seed item
      const ratingWeight = liked.rating / 5.0;

      for (const { mediaId: simMediaId, score: similarity } of similarItems) {
        if (seenItems.has(simMediaId)) continue;

        const weightedScore = similarity * ratingWeight;
        let candidate = candidateScores.get(simMediaId);
        if (!candidate) {
          candidate = { totalScore: 0, count: 0, bestReason: '' };
          candidateScores.set(simMediaId, candidate);
        }

        candidate.totalScore += weightedScore;
        candidate.count++;
        if (similarity > 0.3) {
          candidate.bestReason = `Similar to content you enjoyed`;
        }
      }
    }

    // Compute final scores
    const predictions: ScoredItem[] = [];
    for (const [mediaId, { totalScore, count, bestReason }] of candidateScores) {
      // Average weighted score, slightly boost items recommended by multiple seeds
      const avgScore = totalScore / count;
      const diversityBoost = Math.min(count / userLikedItems.length, 1.0) * 0.1;
      const finalScore = avgScore + diversityBoost;

      predictions.push({
        media_id: mediaId,
        score: finalScore,
        reason: bestReason || 'Matches your content preferences',
        algorithm: 'content-based',
      });
    }

    // Sort descending
    predictions.sort((a, b) => b.score - a.score);

    // Normalize to 0-1
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
        for (const item of topItems) {
          item.score = 0.5;
        }
      }
    }

    return topItems;
  }

  /**
   * Get the TF-IDF vector for an item (for storage in DB).
   */
  getItemVector(mediaId: string): Record<string, number> | null {
    const vector = this.itemVectors.get(mediaId);
    if (!vector) return null;
    const obj: Record<string, number> = {};
    for (const [key, val] of vector) {
      obj[key] = val;
    }
    return obj;
  }

  /**
   * Get all item similarity pairs (for bulk storage in DB).
   */
  getAllSimilarityPairs(): Array<{ media_id: string; similar_media_id: string; similarity_score: number }> {
    const pairs: Array<{ media_id: string; similar_media_id: string; similarity_score: number }> = [];
    for (const [mediaId, similarities] of this.similarityCache) {
      for (const { mediaId: simMediaId, score } of similarities) {
        pairs.push({
          media_id: mediaId,
          similar_media_id: simMediaId,
          similarity_score: score,
        });
      }
    }
    return pairs;
  }

  /**
   * Check if the model has been built.
   */
  isReady(): boolean {
    return this.itemVectors.size > 0;
  }

  /**
   * Get the total number of items with vectors.
   */
  getItemCount(): number {
    return this.itemVectors.size;
  }
}

// =============================================================================
// Vector Math Helpers (module-level for reuse)
// =============================================================================

function vectorDotProduct(a: SparseVector, b: SparseVector): number {
  let result = 0;
  const [smaller, larger] = a.size <= b.size ? [a, b] : [b, a];
  for (const [key, valA] of smaller) {
    const valB = larger.get(key);
    if (valB !== undefined) {
      result += valA * valB;
    }
  }
  return result;
}

function vectorMagnitude(v: SparseVector): number {
  let sumSq = 0;
  for (const val of v.values()) {
    sumSq += val * val;
  }
  return Math.sqrt(sumSq);
}
