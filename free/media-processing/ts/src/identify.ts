/**
 * Content Identification (UPGRADE 1d)
 * Parses filenames using regex to extract media metadata
 * Optionally searches TMDb via metadata-enrichment plugin
 */

import { createLogger } from '@nself/plugin-utils';
import type { Config } from './config.js';
import type { ParsedMediaInfo, TmdbSearchResult, ContentIdentification } from './types.js';

const logger = createLogger('media-processing:identify');

// Regex patterns for parsing media filenames
const YEAR_PATTERN = /[.\s_(-](\d{4})[.\s_)-]/;
const SEASON_EPISODE_PATTERN = /[Ss](\d{1,2})[Ee](\d{1,3})/;
const SEASON_ONLY_PATTERN = /[Ss]eason[\s._-]?(\d{1,2})/i;
const EPISODE_ONLY_PATTERN = /[Ee]pisode[\s._-]?(\d{1,3})/i;
const RESOLUTION_PATTERN = /\b(2160p|1080p|720p|480p|360p|240p|4[Kk])\b/;
const SOURCE_PATTERN = /\b(BluRay|Blu-Ray|BDRip|BRRip|WEB-DL|WEBRip|WEBDL|WEB|HDRip|DVDRip|HDTV|PDTV|SDTV|CAM|TS|TC|SCR|R5|DVDScr)\b/i;
const CODEC_PATTERN = /\b(x264|x265|H\.?264|H\.?265|HEVC|AVC|VP9|AV1|XviD|DivX|AAC|AC3|DTS|FLAC|MP3|Atmos|TrueHD)\b/i;
const RELEASE_GROUP_PATTERN = /-([A-Za-z0-9]+)(?:\.[a-z]{2,4})?$/;

/** Common noise words to strip from titles */
const NOISE_TOKENS = new Set([
  'extended', 'unrated', 'directors', 'cut', 'remastered', 'proper',
  'internal', 'limited', 'complete', 'dual', 'audio', 'multi',
  'subs', 'dubbed', 'subbed', 'imax', 'hdr', 'hdr10', 'dolby',
  'vision', 'dv', 'remux',
]);

export class ContentIdentifier {
  constructor(private config: Config) {}

  /**
   * Parse a filename to extract structured media information
   */
  parseFilename(filename: string): ParsedMediaInfo {
    // Remove file extension
    const raw = filename;
    let name = filename.replace(/\.[a-zA-Z0-9]{2,4}$/, '');

    // Extract year
    const yearMatch = name.match(YEAR_PATTERN);
    const year = yearMatch ? parseInt(yearMatch[1], 10) : undefined;

    // Validate year is reasonable (1900-2099)
    const validYear = year && year >= 1900 && year <= 2099 ? year : undefined;

    // Extract season/episode
    const seMatch = name.match(SEASON_EPISODE_PATTERN);
    let season: number | undefined;
    let episode: number | undefined;

    if (seMatch) {
      season = parseInt(seMatch[1], 10);
      episode = parseInt(seMatch[2], 10);
    } else {
      const seasonMatch = name.match(SEASON_ONLY_PATTERN);
      const episodeMatch = name.match(EPISODE_ONLY_PATTERN);
      if (seasonMatch) season = parseInt(seasonMatch[1], 10);
      if (episodeMatch) episode = parseInt(episodeMatch[1], 10);
    }

    // Extract resolution
    const resolutionMatch = name.match(RESOLUTION_PATTERN);
    const resolution = resolutionMatch ? resolutionMatch[1] : undefined;

    // Extract source
    const sourceMatch = name.match(SOURCE_PATTERN);
    const source = sourceMatch ? sourceMatch[1] : undefined;

    // Extract codec
    const codecMatch = name.match(CODEC_PATTERN);
    const codec = codecMatch ? codecMatch[1] : undefined;

    // Extract release group
    const groupMatch = name.match(RELEASE_GROUP_PATTERN);
    const releaseGroup = groupMatch ? groupMatch[1] : undefined;

    // Extract title - everything before the year or S00E00 pattern
    let title = name;

    // Find the earliest "marker" position to truncate title
    const markers: number[] = [];
    if (yearMatch && yearMatch.index !== undefined) markers.push(yearMatch.index);
    if (seMatch && seMatch.index !== undefined) markers.push(seMatch.index);
    if (resolutionMatch && resolutionMatch.index !== undefined) markers.push(resolutionMatch.index);
    if (sourceMatch && sourceMatch.index !== undefined) markers.push(sourceMatch.index);
    if (codecMatch && codecMatch.index !== undefined) markers.push(codecMatch.index);

    if (markers.length > 0) {
      const cutoff = Math.min(...markers);
      title = name.substring(0, cutoff);
    }

    // Clean up title: replace dots, underscores, hyphens with spaces
    title = title
      .replace(/[._]/g, ' ')
      .replace(/-/g, ' ')
      .replace(/\s+/g, ' ')
      .trim();

    // Remove noise tokens from end of title
    const titleWords = title.split(' ');
    while (titleWords.length > 0 && NOISE_TOKENS.has(titleWords[titleWords.length - 1].toLowerCase())) {
      titleWords.pop();
    }
    title = titleWords.join(' ').trim();

    // If title is empty, use the original filename
    if (!title) {
      title = name.replace(/[._-]/g, ' ').trim();
    }

    return {
      title,
      year: validYear,
      season,
      episode,
      resolution,
      source,
      codec,
      releaseGroup,
      raw,
    };
  }

  /**
   * Identify content by parsing filename and optionally searching TMDb
   */
  async identifyContent(filename: string, _duration?: number): Promise<ContentIdentification> {
    const parsed = this.parseFilename(filename);

    logger.info('Parsed filename', {
      title: parsed.title,
      year: parsed.year,
      season: parsed.season,
      episode: parsed.episode,
    });

    // Try to search TMDb via metadata-enrichment plugin
    try {
      const candidates = await this.searchTmdb(parsed);
      if (candidates.length > 0) {
        const best = this.matchTmdb(parsed, candidates);
        if (best) {
          const tmdbYear = best.release_date ? parseInt(best.release_date.substring(0, 4), 10) : undefined;
          return {
            ...parsed,
            tmdb_id: best.id,
            tmdb_title: best.title,
            tmdb_year: tmdbYear,
            confidence: this.calculateConfidence(parsed, best),
          };
        }
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.warn('TMDb search failed (non-fatal)', { error: message });
    }

    // Return parsed info without TMDb match
    return {
      ...parsed,
      confidence: 0,
    };
  }

  /**
   * Search TMDb via the metadata-enrichment plugin
   */
  private async searchTmdb(parsed: ParsedMediaInfo): Promise<TmdbSearchResult[]> {
    const mediaType = parsed.season !== undefined ? 'tv' : 'movie';
    const query = encodeURIComponent(parsed.title);
    const yearParam = parsed.year ? `&year=${parsed.year}` : '';

    const url = `${this.config.metadataEnrichmentUrl}/v1/search?query=${query}&type=${mediaType}${yearParam}`;

    logger.debug('Searching TMDb', { url });

    const response = await fetch(url, {
      headers: { 'Accept': 'application/json' },
      signal: AbortSignal.timeout(5000),
    });

    if (!response.ok) {
      throw new Error(`TMDb search failed: ${response.statusText}`);
    }

    const body = await response.json() as { success: boolean; data?: TmdbSearchResult[] };

    if (!body.success || !body.data) {
      return [];
    }

    return body.data;
  }

  /**
   * Match parsed info against TMDb candidates, return best match
   */
  matchTmdb(parsed: ParsedMediaInfo, candidates: TmdbSearchResult[]): TmdbSearchResult | null {
    if (candidates.length === 0) return null;

    let bestScore = -1;
    let bestMatch: TmdbSearchResult | null = null;

    for (const candidate of candidates) {
      let score = 0;

      // Title similarity (simple normalized comparison)
      const similarity = this.titleSimilarity(parsed.title, candidate.title);
      score += similarity * 100;

      // Year match
      if (parsed.year && candidate.release_date) {
        const candidateYear = parseInt(candidate.release_date.substring(0, 4), 10);
        if (candidateYear === parsed.year) {
          score += 50;
        } else if (Math.abs(candidateYear - parsed.year) <= 1) {
          score += 25;
        }
      }

      // Popularity boost (vote_average as a proxy)
      if (candidate.vote_average) {
        score += candidate.vote_average;
      }

      if (score > bestScore) {
        bestScore = score;
        bestMatch = candidate;
      }
    }

    return bestMatch;
  }

  /**
   * Calculate confidence score for a match (0-1)
   */
  private calculateConfidence(parsed: ParsedMediaInfo, match: TmdbSearchResult): number {
    let confidence = 0;

    // Title similarity contributes up to 0.6
    const similarity = this.titleSimilarity(parsed.title, match.title);
    confidence += similarity * 0.6;

    // Year match contributes up to 0.3
    if (parsed.year && match.release_date) {
      const matchYear = parseInt(match.release_date.substring(0, 4), 10);
      if (matchYear === parsed.year) {
        confidence += 0.3;
      } else if (Math.abs(matchYear - parsed.year) <= 1) {
        confidence += 0.15;
      }
    }

    // Having a TMDb ID at all contributes 0.1
    confidence += 0.1;

    return Math.min(confidence, 1.0);
  }

  /**
   * Simple title similarity using normalized comparison
   */
  private titleSimilarity(a: string, b: string): number {
    const normalize = (s: string) => s.toLowerCase().replace(/[^a-z0-9]/g, '');
    const na = normalize(a);
    const nb = normalize(b);

    if (na === nb) return 1.0;
    if (na.length === 0 || nb.length === 0) return 0;

    // Check if one contains the other
    if (na.includes(nb) || nb.includes(na)) {
      const shorter = Math.min(na.length, nb.length);
      const longer = Math.max(na.length, nb.length);
      return shorter / longer;
    }

    // Character overlap ratio
    const aChars = new Set(na.split(''));
    const bChars = new Set(nb.split(''));
    let overlap = 0;
    for (const c of aChars) {
      if (bChars.has(c)) overlap++;
    }
    const union = new Set([...aChars, ...bChars]).size;
    return union > 0 ? overlap / union : 0;
  }
}
