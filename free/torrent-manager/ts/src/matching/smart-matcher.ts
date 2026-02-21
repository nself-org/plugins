/**
 * Smart Torrent Matcher
 * Scores and selects the best torrent based on multiple criteria
 */

import { TorrentSearchResult } from '../search/base-searcher.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('torrent-manager:matcher');

export interface MatchOptions {
  // Content identification
  title: string;
  year?: number;
  season?: number;
  episode?: number;

  // Quality preferences
  preferredQualities?: string[];    // ['1080p', '720p']
  preferredSources?: string[];      // ['BluRay', 'WEB-DL']
  preferredCodecs?: string[];       // ['x265', 'x264']
  preferredGroups?: string[];       // ['YIFY', 'RARBG', 'FGT']

  // Size constraints
  minSizeGB?: number;
  maxSizeGB?: number;

  // Seeder requirements
  minSeeders?: number;

  // Exclusions
  excludeLanguages?: string[];      // ['CAM', 'TS', 'KOREAN']
  excludeKeywords?: string[];       // ['KORSUB', 'HC', 'BLURRED']
}

interface ScoreBreakdown {
  [key: string]: number;
  qualityScore: number;
  sourceScore: number;
  seederScore: number;
  sizeScore: number;
  releaseGroupScore: number;
}

export class SmartMatcher {
  /**
   * Find the best matching torrent from search results
   */
  findBestMatch(
    results: TorrentSearchResult[],
    options: MatchOptions
  ): TorrentSearchResult | null {
    if (results.length === 0) {
      logger.warn('No results to match');
      return null;
    }

    logger.info(`Matching ${results.length} results for: ${options.title}`);

    // Step 1: Filter by title match
    const titleMatches = this.filterByTitle(results, options);
    logger.info(`Title matches: ${titleMatches.length}/${results.length}`);

    if (titleMatches.length === 0) {
      return null;
    }

    // Step 2: Apply hard filters (exclusions, size limits, seeders)
    const filtered = this.applyHardFilters(titleMatches, options);
    logger.info(`After hard filters: ${filtered.length}/${titleMatches.length}`);

    if (filtered.length === 0) {
      return null;
    }

    // Step 3: Score each result
    const scored = filtered.map(result => ({
      result,
      score: this.scoreResult(result, options),
      breakdown: this.getScoreBreakdown(result, options)
    }));

    // Step 4: Sort by score (descending)
    scored.sort((a, b) => b.score - a.score);

    // Step 5: Return best match
    const best = scored[0];
    logger.info(`Best match: ${best.result.title} (score: ${best.score.toFixed(2)}/100)`);
    logger.debug('Score breakdown:', best.breakdown);

    // Attach score to result
    best.result.score = best.score;
    best.result.scoreBreakdown = best.breakdown;

    return best.result;
  }

  /**
   * Filter results by title match
   */
  private filterByTitle(
    results: TorrentSearchResult[],
    options: MatchOptions
  ): TorrentSearchResult[] {
    const normalizedQuery = this.normalizeTitle(options.title);

    return results.filter(result => {
      const parsed = result.parsedInfo;

      // Match title
      const normalizedResultTitle = this.normalizeTitle(parsed.title);
      const titleMatch = this.calculateTitleSimilarity(normalizedQuery, normalizedResultTitle) >= 0.8;

      if (!titleMatch) return false;

      // Match year (for movies)
      if (options.year && parsed.year) {
        // Allow +/- 1 year tolerance
        if (Math.abs(parsed.year - options.year) > 1) {
          return false;
        }
      }

      // Match season/episode (for TV)
      if (options.season !== undefined && parsed.season !== undefined) {
        if (parsed.season !== options.season) return false;
      }

      if (options.episode !== undefined && parsed.episode !== undefined) {
        if (parsed.episode !== options.episode) return false;
      }

      return true;
    });
  }

  /**
   * Apply hard filters (must pass)
   */
  private applyHardFilters(
    results: TorrentSearchResult[],
    options: MatchOptions
  ): TorrentSearchResult[] {
    return results.filter(result => {
      const parsed = result.parsedInfo;

      // Minimum seeders
      if (options.minSeeders && result.seeders < options.minSeeders) {
        return false;
      }

      // Size constraints
      const sizeGB = result.sizeBytes / (1024 * 1024 * 1024);
      if (options.minSizeGB && sizeGB < options.minSizeGB) {
        return false;
      }
      if (options.maxSizeGB && sizeGB > options.maxSizeGB) {
        return false;
      }

      // Exclude bad sources (CAM, TS, TC)
      const badSources = ['CAM', 'TS', 'TC', 'R5', 'SCREENER'];
      if (parsed.source && badSources.includes(parsed.source)) {
        return false;
      }

      // Exclude specific languages
      if (options.excludeLanguages && parsed.language) {
        if (options.excludeLanguages.includes(parsed.language)) {
          return false;
        }
      }

      // Exclude keywords in title
      if (options.excludeKeywords) {
        const titleLower = result.title.toLowerCase();
        for (const keyword of options.excludeKeywords) {
          if (titleLower.includes(keyword.toLowerCase())) {
            return false;
          }
        }
      }

      return true;
    });
  }

  /**
   * Score a result (0-100)
   */
  private scoreResult(
    result: TorrentSearchResult,
    options: MatchOptions
  ): number {
    const breakdown = this.getScoreBreakdown(result, options);
    return Object.values(breakdown).reduce((sum, score) => sum + score, 0);
  }

  /**
   * Get detailed score breakdown
   */
  private getScoreBreakdown(
    result: TorrentSearchResult,
    options: MatchOptions
  ): ScoreBreakdown {
    const parsed = result.parsedInfo;

    // Quality score (0-30 points)
    const qualityScore = this.scoreQuality(parsed.quality, options);

    // Source score (0-25 points)
    const sourceScore = this.scoreSource(parsed.source, options);

    // Seeder score (0-20 points)
    const seederScore = this.scoreSeeders(result.seeders);

    // Size score (0-15 points)
    const sizeScore = this.scoreSize(result.sizeBytes, parsed.quality);

    // Release group score (0-10 points)
    const releaseGroupScore = this.scoreReleaseGroup(parsed.releaseGroup, options);

    return {
      qualityScore,
      sourceScore,
      seederScore,
      sizeScore,
      releaseGroupScore
    };
  }

  private scoreQuality(quality: string | undefined, options: MatchOptions): number {
    if (!quality) return 0;

    const qualityRanking: Record<string, number> = {
      '2160p': 30,
      '4K': 30,
      '1080p': 25,
      '720p': 20,
      '480p': 10,
      '360p': 5
    };

    const baseScore = qualityRanking[quality] || 0;

    // Bonus if matches preferred quality
    if (options.preferredQualities?.includes(quality)) {
      return Math.min(30, baseScore + 5);
    }

    return baseScore;
  }

  private scoreSource(source: string | undefined, options: MatchOptions): number {
    if (!source) return 0;

    const sourceRanking: Record<string, number> = {
      'BluRay': 25,
      'WEB-DL': 20,
      'WEBRip': 18,
      'HDTV': 15,
      'DVD': 10
    };

    const baseScore = sourceRanking[source] || 0;

    // Bonus if matches preferred source
    if (options.preferredSources?.includes(source)) {
      return Math.min(25, baseScore + 5);
    }

    return baseScore;
  }

  private scoreSeeders(seeders: number): number {
    // Logarithmic scoring: more seeders is better, but diminishing returns
    if (seeders < 1) return 0;
    if (seeders >= 1000) return 20;

    // 1-10 seeders: 5-10 points
    // 10-100 seeders: 10-15 points
    // 100-1000 seeders: 15-20 points
    return Math.min(20, 5 + Math.log10(seeders) * 5);
  }

  private scoreSize(sizeBytes: number, quality: string | undefined): number {
    const sizeGB = sizeBytes / (1024 * 1024 * 1024);

    // Expected sizes for different qualities
    const expectedSizes: Record<string, { min: number; ideal: number; max: number }> = {
      '2160p': { min: 15, ideal: 40, max: 100 },
      '4K': { min: 15, ideal: 40, max: 100 },
      '1080p': { min: 1.5, ideal: 8, max: 25 },
      '720p': { min: 0.7, ideal: 4, max: 15 },
      '480p': { min: 0.3, ideal: 1.5, max: 5 }
    };

    const expected = expectedSizes[quality || '1080p'] || expectedSizes['1080p'];

    // Too small: likely poor quality
    if (sizeGB < expected.min) return 0;

    // Too large: bloated
    if (sizeGB > expected.max) return 5;

    // Ideal range
    if (sizeGB >= expected.min && sizeGB <= expected.ideal) {
      return 15;
    }

    // Between ideal and max: linearly decrease
    const ratio = (expected.max - sizeGB) / (expected.max - expected.ideal);
    return 5 + (ratio * 10);
  }

  private scoreReleaseGroup(group: string | undefined, options: MatchOptions): number {
    if (!group) return 5; // Neutral score

    // Trusted groups (scene/p2p groups known for quality)
    const trustedGroups = ['YIFY', 'YTS', 'RARBG', 'FGT', 'EVO', 'SPARKS', 'NTb', 'TOMMY'];

    if (trustedGroups.includes(group.toUpperCase())) {
      return 10;
    }

    // Preferred groups from options
    if (options.preferredGroups?.includes(group)) {
      return 10;
    }

    return 5; // Unknown group: neutral
  }

  private normalizeTitle(title: string): string {
    return title
      .toLowerCase()
      .replace(/[^a-z0-9\s]/g, '')
      .replace(/\s+/g, ' ')
      .trim();
  }

  /**
   * Calculate title similarity using Levenshtein distance
   */
  private calculateTitleSimilarity(title1: string, title2: string): number {
    const len1 = title1.length;
    const len2 = title2.length;

    const matrix: number[][] = [];

    for (let i = 0; i <= len1; i++) {
      matrix[i] = [i];
    }

    for (let j = 0; j <= len2; j++) {
      matrix[0][j] = j;
    }

    for (let i = 1; i <= len1; i++) {
      for (let j = 1; j <= len2; j++) {
        const cost = title1[i - 1] === title2[j - 1] ? 0 : 1;
        matrix[i][j] = Math.min(
          matrix[i - 1][j] + 1,
          matrix[i][j - 1] + 1,
          matrix[i - 1][j - 1] + cost
        );
      }
    }

    const distance = matrix[len1][len2];
    const maxLen = Math.max(len1, len2);

    return 1 - (distance / maxLen);
  }
}
