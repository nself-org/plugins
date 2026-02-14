/**
 * Podcast Discovery
 * Search for podcasts via iTunes Search API and Podcast Index API
 */

import { createHash } from 'node:crypto';
import { createLogger } from '@nself/plugin-utils';
import type { Config } from './config.js';
import type {
  SearchResult,
  ITunesSearchResult,
  PodcastIndexSearchResult,
} from './types.js';

const logger = createLogger('podcast:discovery');

/**
 * Search for podcasts using iTunes Search API
 */
export async function searchItunes(
  query: string,
  config: Config,
  limit = 25
): Promise<SearchResult[]> {
  const url = new URL(config.itunesSearchUrl);
  url.searchParams.set('term', query);
  url.searchParams.set('media', 'podcast');
  url.searchParams.set('entity', 'podcast');
  url.searchParams.set('limit', String(Math.min(limit, 200)));

  logger.debug('Searching iTunes', { query, limit });

  const response = await fetch(url.toString(), {
    headers: {
      'User-Agent': 'nself-podcast/1.0.0',
    },
    signal: AbortSignal.timeout(15000),
  });

  if (!response.ok) {
    throw new Error(`iTunes search failed: HTTP ${response.status}`);
  }

  const data = (await response.json()) as ITunesSearchResult;

  return data.results
    .filter(result => result.feedUrl)
    .map(result => ({
      title: result.collectionName || result.trackName,
      author: result.artistName,
      feedUrl: result.feedUrl,
      artworkUrl: result.artworkUrl600 || result.artworkUrl100 || result.artworkUrl60,
      genre: result.primaryGenreName,
      episodeCount: result.trackCount ?? null,
      description: null,
      source: 'itunes' as const,
    }));
}

/**
 * Search for podcasts using Podcast Index API
 * Requires API key and secret to be configured
 */
export async function searchPodcastIndex(
  query: string,
  config: Config,
  limit = 25
): Promise<SearchResult[]> {
  if (!config.podcastIndexApiKey || !config.podcastIndexApiSecret) {
    logger.debug('Podcast Index API not configured, skipping');
    return [];
  }

  const apiHeaderTime = Math.floor(Date.now() / 1000);
  const hash4Value = createHash('sha1')
    .update(`${config.podcastIndexApiKey}${config.podcastIndexApiSecret}${apiHeaderTime}`)
    .digest('hex');

  const url = new URL('https://api.podcastindex.org/api/1.0/search/byterm');
  url.searchParams.set('q', query);
  url.searchParams.set('max', String(Math.min(limit, 100)));

  logger.debug('Searching Podcast Index', { query, limit });

  const response = await fetch(url.toString(), {
    headers: {
      'User-Agent': 'nself-podcast/1.0.0',
      'X-Auth-Date': String(apiHeaderTime),
      'X-Auth-Key': config.podcastIndexApiKey,
      'Authorization': hash4Value,
    },
    signal: AbortSignal.timeout(15000),
  });

  if (!response.ok) {
    throw new Error(`Podcast Index search failed: HTTP ${response.status}`);
  }

  const data = (await response.json()) as PodcastIndexSearchResult;

  return data.feeds.map(feed => ({
    title: feed.title,
    author: feed.author || feed.ownerName,
    feedUrl: feed.url || feed.originalUrl,
    artworkUrl: feed.artwork || feed.image,
    genre: Object.values(feed.categories || {}).join(', '),
    episodeCount: null,
    description: feed.description || null,
    source: 'podcastindex' as const,
  }));
}

/**
 * Search across all configured discovery sources
 * Returns deduplicated results (by feedUrl)
 */
export async function discoverPodcasts(
  query: string,
  config: Config,
  limit = 25
): Promise<SearchResult[]> {
  const results: SearchResult[] = [];
  const errors: string[] = [];

  // Search iTunes
  try {
    const itunesResults = await searchItunes(query, config, limit);
    results.push(...itunesResults);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.warn('iTunes search failed', { error: message });
    errors.push(`iTunes: ${message}`);
  }

  // Search Podcast Index (if configured)
  if (config.podcastIndexApiKey && config.podcastIndexApiSecret) {
    try {
      const podcastIndexResults = await searchPodcastIndex(query, config, limit);
      results.push(...podcastIndexResults);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.warn('Podcast Index search failed', { error: message });
      errors.push(`PodcastIndex: ${message}`);
    }
  }

  if (results.length === 0 && errors.length > 0) {
    throw new Error(`All search providers failed: ${errors.join('; ')}`);
  }

  // Deduplicate by feedUrl
  const seen = new Set<string>();
  const deduplicated: SearchResult[] = [];
  for (const result of results) {
    const normalized = result.feedUrl.toLowerCase().replace(/\/+$/, '');
    if (!seen.has(normalized)) {
      seen.add(normalized);
      deduplicated.push(result);
    }
  }

  return deduplicated.slice(0, limit);
}

/**
 * Generate HMAC authentication header for Podcast Index API
 * Exported for testing purposes
 */
export function generatePodcastIndexAuth(
  apiKey: string,
  apiSecret: string,
  timestamp?: number
): { authDate: string; authorization: string } {
  const authDate = String(timestamp ?? Math.floor(Date.now() / 1000));
  const authorization = createHash('sha1')
    .update(`${apiKey}${apiSecret}${authDate}`)
    .digest('hex');

  return { authDate, authorization };
}
