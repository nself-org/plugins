/**
 * Quality Profile Presets
 *
 * Three built-in quality profiles that cover common use-cases.
 * Users can select a profile by name when creating downloads or subscriptions.
 */

import type { QualityProfilePreset } from './types.js';

export const QUALITY_PRESETS: Record<string, QualityProfilePreset> = {
  minimal: {
    name: 'Minimal',
    description: 'Small file sizes for limited bandwidth or storage. 720p/480p max.',
    max_resolution: '720p',
    min_resolution: '480p',
    preferred_sources: ['WEB-DL', 'WEBRip', 'HDTV'],
    max_size_movie_gb: 2,
    max_size_episode_gb: 0.5,
  },

  balanced: {
    name: 'Balanced',
    description: 'Best balance of quality and size. 1080p preferred, WEB-DL and above.',
    max_resolution: '1080p',
    min_resolution: '720p',
    preferred_sources: ['WEB-DL', 'WEBRip', 'BluRay'],
    max_size_movie_gb: 8,
    max_size_episode_gb: 2,
  },

  '4k_premium': {
    name: '4K Premium',
    description: 'Maximum quality with 2160p/4K preferred. BluRay and Remux sources.',
    max_resolution: '2160p',
    min_resolution: '1080p',
    preferred_sources: ['BluRay', 'Remux', 'WEB-DL'],
    max_size_movie_gb: 40,
    max_size_episode_gb: 10,
  },
};

/**
 * Look up a quality profile preset by name (case-insensitive).
 * Returns `undefined` if no matching preset exists.
 */
export function getQualityPreset(name: string): QualityProfilePreset | undefined {
  const key = name.toLowerCase().replace(/\s+/g, '_');
  return QUALITY_PRESETS[key];
}

/**
 * List all available quality presets.
 */
export function listQualityPresets(): QualityProfilePreset[] {
  return Object.values(QUALITY_PRESETS);
}

/**
 * Evaluate whether a release matches a quality profile.
 *
 * @param profile  - The preset name (minimal, balanced, 4k_premium)
 * @param quality  - The detected resolution (e.g. "1080p", "2160p")
 * @param source   - The release source (e.g. "WEB-DL", "BluRay")
 * @param sizeGb   - The file size in GB
 * @param isMovie  - Whether this is a movie (true) or episode (false)
 * @returns `true` if the release satisfies the profile constraints.
 */
export function matchesQualityProfile(
  profile: string,
  quality?: string,
  source?: string,
  sizeGb?: number,
  isMovie?: boolean,
): boolean {
  const preset = getQualityPreset(profile);
  if (!preset) return true; // no profile = accept everything

  const resolutionOrder = ['480p', '720p', '1080p', '2160p'];

  // Check resolution bounds
  if (quality) {
    const idx = resolutionOrder.indexOf(quality);
    const minIdx = resolutionOrder.indexOf(preset.min_resolution);
    const maxIdx = resolutionOrder.indexOf(preset.max_resolution);

    if (idx !== -1) {
      if (idx < minIdx) return false; // below minimum
      if (idx > maxIdx) return false; // above maximum
    }
  }

  // Check preferred sources
  if (source && preset.preferred_sources.length > 0) {
    const normalizedSource = source.toLowerCase();
    const matches = preset.preferred_sources.some(s => normalizedSource.includes(s.toLowerCase()));
    if (!matches) return false;
  }

  // Check size limits
  if (sizeGb !== undefined && sizeGb > 0) {
    const maxSize = isMovie ? preset.max_size_movie_gb : preset.max_size_episode_gb;
    if (sizeGb > maxSize) return false;
  }

  return true;
}
