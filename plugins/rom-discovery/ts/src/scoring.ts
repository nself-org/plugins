/**
 * ROM Discovery Quality and Popularity Scoring
 * Calculates quality and popularity scores for ROM metadata
 */

import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('rom-discovery:scoring');

// =============================================================================
// Quality Scoring
// =============================================================================

/**
 * Calculate a quality score for a ROM based on its metadata.
 *
 * Score breakdown:
 * - Base score: 50
 * - Release group bonus: +20 to +45
 * - Verification bonus: +5 to +10
 * - Penalties for hacks, translations, dead URLs
 *
 * @returns Quality score 0-100
 */
export function calculateQualityScore(rom: {
  release_group?: string | null;
  is_verified_dump?: boolean;
  is_hack?: boolean;
  is_translation?: boolean;
  is_homebrew?: boolean;
  is_public_domain?: boolean;
  download_url_dead?: boolean;
  download_url?: string | null;
  file_hash_sha256?: string | null;
  file_hash_md5?: string | null;
  file_size_bytes?: number | null;
  scraped_from?: string | null;
}): number {
  let score = 50; // Base score

  // Release group bonuses
  const releaseGroup = (rom.release_group ?? '').toLowerCase();
  if (releaseGroup.includes('no-intro')) {
    score += 45;
  } else if (releaseGroup.includes('redump')) {
    score += 45;
  } else if (releaseGroup.includes('tosec')) {
    score += 30;
  } else if (releaseGroup.includes('community') || rom.scraped_from === 'tecmobowl') {
    score += 35;
  } else if (releaseGroup.includes('archive.org') || rom.scraped_from === 'archive-org') {
    score += 20;
  }

  // Verification bonus
  if (rom.is_verified_dump) {
    score += 5;
  }

  // Homebrew bonus (legal, distributable)
  if (rom.is_homebrew) {
    score += 10;
  }

  // Public domain bonus
  if (rom.is_public_domain) {
    score += 10;
  }

  // Hash availability bonus
  if (rom.file_hash_sha256) {
    score += 3;
  }
  if (rom.file_hash_md5) {
    score += 2;
  }

  // File size known bonus
  if (rom.file_size_bytes && rom.file_size_bytes > 0) {
    score += 2;
  }

  // Download URL available bonus
  if (rom.download_url) {
    score += 3;
  }

  // Penalties
  if (rom.is_hack) {
    score -= 20;
  }

  if (rom.is_translation) {
    score -= 10;
  }

  if (rom.download_url_dead) {
    score -= 50;
  }

  // Clamp to 0-100
  const finalScore = Math.max(0, Math.min(100, score));

  logger.debug('Quality score calculated', {
    release_group: rom.release_group,
    is_verified_dump: rom.is_verified_dump,
    is_hack: rom.is_hack,
    score: finalScore,
  });

  return finalScore;
}

// =============================================================================
// Popularity Scoring
// =============================================================================

/**
 * Calculate a popularity score using a weighted average of various signals.
 *
 * Weights:
 * - download_count: 0.30
 * - play_count: 0.25
 * - external_downloads (archive.org): 0.25
 * - search_count: 0.20
 *
 * Uses log scale normalization to prevent extremely popular ROMs from
 * dominating the entire scale.
 *
 * @returns Popularity score 0-100
 */
export function calculatePopularityScore(metrics: {
  download_count: number;
  play_count: number;
  archive_org_downloads: number;
  search_count: number;
}): number {
  const WEIGHTS = {
    download_count: 0.30,
    play_count: 0.25,
    archive_org_downloads: 0.25,
    search_count: 0.20,
  };

  // Log scale normalization: log(1 + value) / log(1 + max_reasonable_value)
  // This prevents any single metric from overwhelming the score
  const MAX_DOWNLOADS = 100000;
  const MAX_PLAYS = 50000;
  const MAX_ARCHIVE_DOWNLOADS = 1000000;
  const MAX_SEARCHES = 10000;

  function logNormalize(value: number, maxValue: number): number {
    if (value <= 0) return 0;
    return Math.log(1 + value) / Math.log(1 + maxValue);
  }

  const normalizedDownloads = logNormalize(metrics.download_count, MAX_DOWNLOADS);
  const normalizedPlays = logNormalize(metrics.play_count, MAX_PLAYS);
  const normalizedArchive = logNormalize(metrics.archive_org_downloads, MAX_ARCHIVE_DOWNLOADS);
  const normalizedSearches = logNormalize(metrics.search_count, MAX_SEARCHES);

  const weightedScore =
    normalizedDownloads * WEIGHTS.download_count +
    normalizedPlays * WEIGHTS.play_count +
    normalizedArchive * WEIGHTS.archive_org_downloads +
    normalizedSearches * WEIGHTS.search_count;

  // Scale to 0-100
  const finalScore = Math.round(weightedScore * 100);

  logger.debug('Popularity score calculated', {
    metrics,
    normalizedDownloads,
    normalizedPlays,
    normalizedArchive,
    normalizedSearches,
    finalScore,
  });

  return Math.max(0, Math.min(100, finalScore));
}
