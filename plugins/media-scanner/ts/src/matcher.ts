/**
 * Media Scanner - TMDB Metadata Matcher
 * Levenshtein distance scoring and TMDB API integration
 */

import { createLogger, HttpClient } from '@nself/plugin-utils';
import type { MatchResult, TmdbSearchResponse, TmdbSearchResult } from './types.js';

const logger = createLogger('media-scanner:matcher');

const TMDB_BASE_URL = 'https://api.themoviedb.org/3';

/** Auto-accept threshold: matches above this confidence are accepted automatically */
export const AUTO_ACCEPT_THRESHOLD = 0.8;
/** Suggestion threshold: matches between this and auto-accept are suggested for review */
export const SUGGEST_THRESHOLD = 0.5;

export class TmdbMatcher {
  private http: HttpClient;
  private apiKey: string;

  constructor(apiKey: string) {
    this.apiKey = apiKey;
    this.http = new HttpClient({
      baseUrl: TMDB_BASE_URL,
      headers: {
        'Accept': 'application/json',
      },
      timeout: 10_000,
    });
  }

  /**
   * Match a title against TMDB and return ranked results.
   */
  async match(title: string, year: number | null, type: 'movie' | 'tv'): Promise<MatchResult[]> {
    if (!this.apiKey) {
      logger.warn('TMDB API key not configured, skipping match');
      return [];
    }

    const endpoint = type === 'movie' ? '/search/movie' : '/search/tv';
    const params: Record<string, string> = {
      api_key: this.apiKey,
      query: title,
      include_adult: 'false',
      language: 'en-US',
      page: '1',
    };

    if (year) {
      if (type === 'movie') {
        params.year = String(year);
      } else {
        params.first_air_date_year = String(year);
      }
    }

    try {
      const response = await this.http.get<TmdbSearchResponse>(endpoint, params);

      if (!response.results || response.results.length === 0) {
        // Retry without year constraint if no results
        if (year) {
          return this.match(title, null, type);
        }
        return [];
      }

      const results = response.results
        .map(result => this.scoreResult(result, title, year, type))
        .sort((a, b) => b.confidence - a.confidence)
        .slice(0, 10);

      return results;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('TMDB search failed', { title, type, error: message });
      return [];
    }
  }

  /**
   * Score a TMDB search result against the query title.
   */
  private scoreResult(
    result: TmdbSearchResult,
    queryTitle: string,
    queryYear: number | null,
    type: 'movie' | 'tv'
  ): MatchResult {
    const resultTitle = type === 'movie'
      ? (result.title ?? result.original_title ?? '')
      : (result.name ?? result.original_name ?? '');

    const resultYear = extractYear(
      type === 'movie' ? result.release_date : result.first_air_date
    );

    // Normalize both titles for comparison
    const normalizedQuery = normalizeForComparison(queryTitle);
    const normalizedResult = normalizeForComparison(resultTitle);

    // Title similarity (0-1)
    const titleSimilarity = computeSimilarity(normalizedQuery, normalizedResult);

    // Year bonus/penalty
    let yearScore = 0;
    if (queryYear && resultYear) {
      const yearDiff = Math.abs(queryYear - resultYear);
      if (yearDiff === 0) {
        yearScore = 0.15;
      } else if (yearDiff === 1) {
        yearScore = 0.05;
      } else if (yearDiff > 2) {
        yearScore = -0.1;
      }
    }

    // Popularity bonus (small, to break ties)
    const popularityBonus = Math.min((result.popularity ?? 0) / 10000, 0.05);

    const confidence = Math.min(1.0, Math.max(0, titleSimilarity + yearScore + popularityBonus));

    return {
      provider: 'tmdb',
      id: String(result.id),
      title: resultTitle,
      year: resultYear,
      confidence: Math.round(confidence * 1000) / 1000,
    };
  }
}

// ─── Levenshtein Distance ───────────────────────────────────────────────────

/**
 * Compute the Levenshtein edit distance between two strings.
 * Uses the Wagner-Fischer algorithm with O(min(m,n)) space.
 */
export function levenshteinDistance(a: string, b: string): number {
  if (a === b) return 0;
  if (a.length === 0) return b.length;
  if (b.length === 0) return a.length;

  // Ensure a is the shorter string for space optimization
  if (a.length > b.length) {
    [a, b] = [b, a];
  }

  const aLen = a.length;
  const bLen = b.length;

  // Previous and current rows of the distance matrix
  let prev = new Array<number>(aLen + 1);
  let curr = new Array<number>(aLen + 1);

  // Initialize the previous row
  for (let i = 0; i <= aLen; i++) {
    prev[i] = i;
  }

  for (let j = 1; j <= bLen; j++) {
    curr[0] = j;
    for (let i = 1; i <= aLen; i++) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1;
      curr[i] = Math.min(
        curr[i - 1] + 1,       // insertion
        prev[i] + 1,            // deletion
        prev[i - 1] + cost      // substitution
      );
    }
    // Swap rows
    [prev, curr] = [curr, prev];
  }

  return prev[aLen];
}

/**
 * Compute similarity score (0-1) based on Levenshtein distance.
 * 1.0 = identical, 0.0 = completely different.
 */
export function computeSimilarity(a: string, b: string): number {
  if (a === b) return 1.0;
  const maxLen = Math.max(a.length, b.length);
  if (maxLen === 0) return 1.0;
  const distance = levenshteinDistance(a, b);
  return 1.0 - (distance / maxLen);
}

// ─── Helpers ────────────────────────────────────────────────────────────────

/**
 * Normalize a title for comparison: lowercase, strip non-alphanumeric,
 * collapse whitespace.
 */
function normalizeForComparison(title: string): string {
  return title
    .toLowerCase()
    .replace(/[^\w\s]/g, '')
    .replace(/\s+/g, ' ')
    .trim();
}

/**
 * Extract year from a TMDB date string (YYYY-MM-DD).
 */
function extractYear(date: string | undefined): number | null {
  if (!date) return null;
  const year = parseInt(date.substring(0, 4), 10);
  if (isNaN(year) || year < 1900) return null;
  return year;
}
