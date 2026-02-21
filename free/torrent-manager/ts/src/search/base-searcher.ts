/**
 * Base Torrent Searcher
 * Abstract base class for all torrent search providers
 */

export interface ParsedTorrentInfo {
  title: string;
  year?: number;
  season?: number;
  episode?: number;
  quality?: string;  // '1080p', '720p', '2160p', etc.
  source?: string;   // 'BluRay', 'WEB-DL', 'HDTV', 'DVD'
  codec?: string;    // 'x264', 'x265', 'H.264', 'HEVC'
  audio?: string;    // 'AAC', 'DTS', 'AC3', etc.
  releaseGroup?: string;
  language?: string;
  isProper?: boolean;
  isRepack?: boolean;
  type?: 'movie' | 'tv' | 'unknown';
}

export interface TorrentSearchResult {
  title: string;
  normalizedTitle: string;
  magnetUri: string;
  infoHash?: string;
  size: string;         // Human readable (e.g., "1.4 GB")
  sizeBytes: number;
  seeders: number;
  leechers: number;
  uploadDate: string;
  uploadDateUnix: number;
  source: string;       // Search provider name
  sourceUrl: string;    // Detail page URL
  parsedInfo: ParsedTorrentInfo;
  score?: number;       // Populated by smart matcher
  scoreBreakdown?: Record<string, number>;
}

export interface SearchOptions {
  query: string;
  type?: 'movie' | 'tv';
  quality?: string;
  minSeeders?: number;
  maxResults?: number;
  timeout?: number;
}

export abstract class BaseTorrentSearcher {
  abstract readonly name: string;
  abstract readonly baseUrl: string;

  /**
   * Search for torrents
   */
  abstract search(options: SearchOptions): Promise<TorrentSearchResult[]>;

  /**
   * Normalize title for deduplication
   */
  protected normalizeTitle(title: string): string {
    return title
      .toLowerCase()
      .replace(/[^a-z0-9\s]/g, '')
      .replace(/\s+/g, ' ')
      .trim();
  }

  /**
   * Parse size string to bytes
   */
  protected parseSize(sizeStr: string): number {
    const match = sizeStr.match(/([\d.]+)\s*(GB|MB|KB|TB)/i);
    if (!match) return 0;

    const value = parseFloat(match[1]);
    const unit = match[2].toUpperCase();

    const multipliers: Record<string, number> = {
      'KB': 1024,
      'MB': 1024 * 1024,
      'GB': 1024 * 1024 * 1024,
      'TB': 1024 * 1024 * 1024 * 1024
    };

    return value * (multipliers[unit] || 0);
  }

  /**
   * Format bytes to human readable size
   */
  protected formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
    if (bytes < 1024 * 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
    return `${(bytes / (1024 * 1024 * 1024 * 1024)).toFixed(2)} TB`;
  }
}
