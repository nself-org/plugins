/**
 * Torrent Source Registry
 * Built-in registry of known torrent sources with lifecycle and trust metadata
 */

import type { SourceRegistryEntry } from '../types.js';

export const SOURCE_REGISTRY: SourceRegistryEntry[] = [
  {
    name: '1337x',
    active_from: '2007-01-01',
    retired_at: null,
    category: 'public',
    trust_score: 80,
    strengths: ['tv', 'movies', 'general'],
  },
  {
    name: 'ThePirateBay',
    active_from: '2003-11-25',
    retired_at: null,
    category: 'public',
    trust_score: 70,
    strengths: ['general', 'large-catalog'],
  },
  {
    name: 'RuTracker',
    active_from: '2004-01-01',
    retired_at: null,
    category: 'semi-private',
    trust_score: 90,
    strengths: ['high-quality', 'lossless', 'remux'],
  },
  {
    name: 'TorrentGalaxy',
    active_from: '2018-01-01',
    retired_at: null,
    category: 'public',
    trust_score: 75,
    strengths: ['movies', 'tv'],
  },
  {
    name: 'EZTV',
    active_from: '2015-05-01',
    retired_at: null,
    category: 'public',
    trust_score: 65,
    strengths: ['tv'],
  },
  {
    name: 'RARBG',
    active_from: '2012-01-01',
    retired_at: '2023-05-31',
    category: 'public',
    trust_score: 95,
    strengths: ['high-quality', 'verified', 'movies'],
  },
  {
    name: 'YTS-original',
    active_from: '2011-01-01',
    retired_at: '2015-10-20',
    category: 'public',
    trust_score: 60,
    strengths: ['movies', 'small-size'],
  },
  {
    name: 'YTS-mx',
    active_from: '2015-11-01',
    retired_at: null,
    category: 'public',
    trust_score: 50,
    strengths: ['movies', 'small-size'],
  },
  {
    name: 'KickassTorrents',
    active_from: '2008-11-01',
    retired_at: '2016-07-20',
    category: 'public',
    trust_score: 85,
    strengths: ['general'],
  },
];

/**
 * Get all sources from the registry
 */
export function getAllSources(): SourceRegistryEntry[] {
  return SOURCE_REGISTRY;
}

/**
 * Get only currently active sources
 */
export function getActiveSources(): SourceRegistryEntry[] {
  return SOURCE_REGISTRY.filter((s) => s.retired_at === null);
}

/**
 * Get a specific source by name (case-insensitive)
 */
export function getSourceByName(name: string): SourceRegistryEntry | undefined {
  return SOURCE_REGISTRY.find(
    (s) => s.name.toLowerCase() === name.toLowerCase()
  );
}
