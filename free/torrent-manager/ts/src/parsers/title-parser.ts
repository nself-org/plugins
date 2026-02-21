/**
 * Torrent Title Parser
 * Extracts metadata from torrent titles using battle-tested regex patterns
 * Based on patterns from Sonarr, Radarr, and Jackett
 */

import { ParsedTorrentInfo } from '../search/base-searcher.js';

export class TorrentTitleParser {
  // Quality patterns
  private static readonly QUALITY_PATTERNS: Array<{regex: RegExp; quality: string}> = [
    { regex: /\b(4K|2160p|UHD)\b/i, quality: '2160p' },
    { regex: /\b1080p\b/i, quality: '1080p' },
    { regex: /\b720p\b/i, quality: '720p' },
    { regex: /\b480p\b/i, quality: '480p' },
    { regex: /\b360p\b/i, quality: '360p' },
  ];

  // Source patterns
  private static readonly SOURCE_PATTERNS: Array<{regex: RegExp; source: string}> = [
    { regex: /\bBlu[\s.-]?Ray\b/i, source: 'BluRay' },
    { regex: /\bBRRip\b/i, source: 'BluRay' },
    { regex: /\bBDRip\b/i, source: 'BluRay' },
    { regex: /\bWEB[-\s.]?DL\b/i, source: 'WEB-DL' },
    { regex: /\bWEBRip\b/i, source: 'WEBRip' },
    { regex: /\bWEB\b/i, source: 'WEB-DL' },
    { regex: /\bHDTV\b/i, source: 'HDTV' },
    { regex: /\bDVDRip\b/i, source: 'DVD' },
    { regex: /\bDVD\b/i, source: 'DVD' },
    { regex: /\b(CAM|TS|TC|TELESYNC|HDCAM)\b/i, source: 'CAM' },
    { regex: /\bR5\b/i, source: 'R5' },
    { regex: /\bSCREENER\b/i, source: 'SCREENER' },
  ];

  // Codec patterns
  private static readonly CODEC_PATTERNS: Array<{regex: RegExp; codec: string}> = [
    { regex: /\b(x265|H\.?265|HEVC)\b/i, codec: 'x265' },
    { regex: /\b(x264|H\.?264|AVC)\b/i, codec: 'x264' },
    { regex: /\bXviD\b/i, codec: 'XviD' },
    { regex: /\bDivX\b/i, codec: 'DivX' },
  ];

  // Audio patterns
  private static readonly AUDIO_PATTERNS: Array<{regex: RegExp; audio: string}> = [
    { regex: /\b(DTS[-\s]?HD[-\s]?MA|DTS[-\s]?MA)\b/i, audio: 'DTS-HD MA' },
    { regex: /\bDTS\b/i, audio: 'DTS' },
    { regex: /\bDD5[\s.-]?1\b/i, audio: 'DD5.1' },
    { regex: /\bAC3\b/i, audio: 'AC3' },
    { regex: /\bAAC(\d\.\d)?\b/i, audio: 'AAC' },
    { regex: /\bMP3\b/i, audio: 'MP3' },
    { regex: /\bFLAC\b/i, audio: 'FLAC' },
  ];

  // Release group pattern (usually at the end)
  private static readonly RELEASE_GROUP_PATTERN = /[-\[]([A-Z0-9]+)\]?$/i;

  // TV show patterns
  private static readonly TV_PATTERNS = [
    /\bS(\d{1,2})E(\d{1,2})\b/i,                    // S01E01
    /\bS(\d{1,2})[\s.-]?E(\d{1,2})\b/i,             // S01 E01
    /\b(\d{1,2})x(\d{1,2})\b/i,                     // 1x01
    /\bSeason[\s.-]?(\d{1,2})[\s.-]?Episode[\s.-]?(\d{1,2})\b/i,  // Season 1 Episode 1
  ];

  // Year pattern
  private static readonly YEAR_PATTERN = /\b(19\d{2}|20\d{2})\b/;

  /**
   * Parse torrent title and extract metadata
   */
  static parse(title: string): ParsedTorrentInfo {
    const result: ParsedTorrentInfo = {
      title: '',
      type: 'unknown'
    };

    // Detect TV show or movie
    const tvMatch = this.matchTVPattern(title);
    if (tvMatch) {
      result.season = tvMatch.season;
      result.episode = tvMatch.episode;
      result.type = 'tv';
    } else {
      result.type = 'movie';
    }

    // Extract title (everything before quality/year markers)
    result.title = this.extractTitle(title, tvMatch);

    // Extract year
    const yearMatch = title.match(this.YEAR_PATTERN);
    if (yearMatch) {
      result.year = parseInt(yearMatch[1]);
    }

    // Extract quality
    for (const {regex, quality} of this.QUALITY_PATTERNS) {
      if (regex.test(title)) {
        result.quality = quality;
        break;
      }
    }

    // Extract source
    for (const {regex, source} of this.SOURCE_PATTERNS) {
      if (regex.test(title)) {
        result.source = source;
        break;
      }
    }

    // Extract codec
    for (const {regex, codec} of this.CODEC_PATTERNS) {
      if (regex.test(title)) {
        result.codec = codec;
        break;
      }
    }

    // Extract audio
    for (const {regex, audio} of this.AUDIO_PATTERNS) {
      if (regex.test(title)) {
        result.audio = audio;
        break;
      }
    }

    // Extract release group
    const groupMatch = title.match(this.RELEASE_GROUP_PATTERN);
    if (groupMatch) {
      result.releaseGroup = groupMatch[1].toUpperCase();
    }

    // Check for PROPER/REPACK
    result.isProper = /\bPROPER\b/i.test(title);
    result.isRepack = /\bREPACK\b/i.test(title);

    // Detect language (if not English)
    if (/\b(FRENCH|FR)\b/i.test(title)) result.language = 'French';
    else if (/\b(GERMAN|GER)\b/i.test(title)) result.language = 'German';
    else if (/\b(SPANISH|ES)\b/i.test(title)) result.language = 'Spanish';
    else if (/\b(ITALIAN|IT)\b/i.test(title)) result.language = 'Italian';
    else if (/\b(KOREAN|KOR)\b/i.test(title)) result.language = 'Korean';
    else if (/\b(JAPANESE|JAP)\b/i.test(title)) result.language = 'Japanese';
    else result.language = 'English';

    return result;
  }

  /**
   * Match TV show pattern
   */
  private static matchTVPattern(title: string): { season: number; episode: number } | null {
    for (const pattern of this.TV_PATTERNS) {
      const match = title.match(pattern);
      if (match) {
        return {
          season: parseInt(match[1]),
          episode: parseInt(match[2])
        };
      }
    }
    return null;
  }

  /**
   * Extract clean title from torrent name
   */
  private static extractTitle(title: string, tvMatch: { season: number; episode: number } | null): string {
    let cleanTitle = title;

    // Remove everything after quality/source markers
    const markers = [
      /\b(1080p|720p|480p|2160p|4K)\b/i,
      /\b(BluRay|WEB-DL|HDTV|DVDRip)\b/i,
      /\bS\d{1,2}E\d{1,2}\b/i,
      /\b\d{1,2}x\d{1,2}\b/i,
    ];

    for (const marker of markers) {
      const match = cleanTitle.match(marker);
      if (match && match.index !== undefined) {
        cleanTitle = cleanTitle.substring(0, match.index);
        break;
      }
    }

    // If TV show, remove season/episode
    if (tvMatch) {
      for (const pattern of this.TV_PATTERNS) {
        cleanTitle = cleanTitle.replace(pattern, '');
      }
    }

    // Remove year
    cleanTitle = cleanTitle.replace(this.YEAR_PATTERN, '');

    // Clean up
    cleanTitle = cleanTitle
      .replace(/[._\-]/g, ' ')  // Replace separators with spaces
      .replace(/\s+/g, ' ')     // Collapse multiple spaces
      .trim();

    return cleanTitle;
  }
}
