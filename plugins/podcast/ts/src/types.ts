/**
 * Podcast Plugin Types
 * All TypeScript interfaces for podcast feeds, episodes, and related records
 */

// =========================================================================
// Database Record Types
// =========================================================================

export interface FeedRecord {
  id: string;
  source_account_id: string;
  url: string;
  title: string | null;
  description: string | null;
  author: string | null;
  image_url: string | null;
  language: string | null;
  categories: string[] | null;
  last_fetched_at: Date | null;
  last_episode_at: Date | null;
  fetch_interval_minutes: number;
  error_count: number;
  last_error: string | null;
  status: FeedStatus;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
}

export interface EpisodeRecord {
  id: string;
  source_account_id: string;
  feed_id: string;
  guid: string;
  title: string;
  description: string | null;
  pub_date: Date | null;
  duration_seconds: number | null;
  enclosure_url: string | null;
  enclosure_type: string | null;
  enclosure_length: number | null;
  season_number: number | null;
  episode_number: number | null;
  episode_type: EpisodeType;
  chapters_url: string | null;
  transcript_url: string | null;
  image_url: string | null;
  played: boolean;
  play_position_seconds: number;
  downloaded: boolean;
  download_path: string | null;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
}

export type FeedStatus = 'active' | 'paused' | 'error';
export type EpisodeType = 'full' | 'trailer' | 'bonus';

// =========================================================================
// RSS/Atom Parsed Types
// =========================================================================

export interface ParsedFeed {
  title: string;
  description: string | null;
  author: string | null;
  imageUrl: string | null;
  language: string | null;
  categories: string[];
  link: string | null;
  lastBuildDate: Date | null;
  episodes: ParsedEpisode[];
}

export interface ParsedEpisode {
  guid: string;
  title: string;
  description: string | null;
  pubDate: Date | null;
  durationSeconds: number | null;
  enclosureUrl: string | null;
  enclosureType: string | null;
  enclosureLength: number | null;
  seasonNumber: number | null;
  episodeNumber: number | null;
  episodeType: EpisodeType;
  chaptersUrl: string | null;
  transcriptUrl: string | null;
  imageUrl: string | null;
}

// =========================================================================
// iTunes / Podcast Index API Types
// =========================================================================

export interface ITunesSearchResult {
  resultCount: number;
  results: ITunesPodcast[];
}

export interface ITunesPodcast {
  collectionId: number;
  trackId: number;
  artistName: string;
  collectionName: string;
  trackName: string;
  feedUrl: string;
  artworkUrl30: string;
  artworkUrl60: string;
  artworkUrl100: string;
  artworkUrl600: string;
  collectionViewUrl: string;
  trackViewUrl: string;
  primaryGenreName: string;
  genreIds: string[];
  genres: string[];
  trackCount: number;
  releaseDate: string;
  country: string;
}

export interface PodcastIndexSearchResult {
  status: string;
  feeds: PodcastIndexFeed[];
  count: number;
}

export interface PodcastIndexFeed {
  id: number;
  title: string;
  url: string;
  originalUrl: string;
  link: string;
  description: string;
  author: string;
  ownerName: string;
  image: string;
  artwork: string;
  lastUpdateTime: number;
  language: string;
  categories: Record<string, string>;
}

// =========================================================================
// Discovery Search Result (Unified)
// =========================================================================

export interface SearchResult {
  title: string;
  author: string;
  feedUrl: string;
  artworkUrl: string;
  genre: string;
  episodeCount: number | null;
  description: string | null;
  source: 'itunes' | 'podcastindex';
}

// =========================================================================
// OPML Types
// =========================================================================

export interface OpmlOutline {
  text: string;
  title?: string;
  type?: string;
  xmlUrl?: string;
  htmlUrl?: string;
  children?: OpmlOutline[];
}

export interface OpmlDocument {
  title: string;
  dateCreated: string;
  outlines: OpmlOutline[];
}

// =========================================================================
// API Request/Response Types
// =========================================================================

export interface SubscribeFeedRequest {
  url: string;
  title?: string;
}

export interface SubscribeFeedResponse {
  id: string;
  title: string | null;
  url: string;
  episode_count: number;
}

export interface RefreshFeedResponse {
  id: string;
  title: string | null;
  new_episodes: number;
  total_episodes: number;
}

export interface DiscoverRequest {
  query: string;
  limit?: number;
}

export interface ImportOpmlRequest {
  opml_content: string;
}

export interface ImportOpmlResponse {
  imported: number;
  errors: string[];
}

export interface ExportOpmlResponse {
  opml: string;
}

export interface DownloadEpisodeResponse {
  download_path: string;
}

export interface PodcastStats {
  feed_count: number;
  episode_count: number;
  total_duration_hours: number;
  unplayed_count: number;
  downloaded_count: number;
}

// =========================================================================
// Download Tracking
// =========================================================================

export interface DownloadProgress {
  episodeId: string;
  bytesDownloaded: number;
  totalBytes: number | null;
  status: 'pending' | 'downloading' | 'complete' | 'failed';
  error?: string;
}

// =========================================================================
// Scheduler Types
// =========================================================================

export interface FeedRefreshJob {
  feedId: string;
  url: string;
  nextRefreshAt: Date;
  intervalMinutes: number;
}
