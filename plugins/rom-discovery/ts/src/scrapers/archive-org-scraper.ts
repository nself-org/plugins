/**
 * Archive.org Scraper
 * Searches Archive.org for ROM collections and extracts metadata
 */

import { createHash } from 'node:crypto';
import axios from 'axios';
import { createLogger } from '@nself/plugin-utils';
import { calculateQualityScore } from '../scoring.js';
import type {
  RomMetadataRecord,
  ScraperResult,
  ArchiveOrgSearchResponse,
  ArchiveOrgMetadataResponse,
  PlatformMapping,
} from '../types.js';
import type { RomDiscoveryDatabase } from '../database.js';

const logger = createLogger('rom-discovery:archive-org');

const ARCHIVE_SEARCH_URL = 'https://archive.org/advancedsearch.php';
const ARCHIVE_METADATA_URL = 'https://archive.org/metadata';
const ARCHIVE_DOWNLOAD_URL = 'https://archive.org/download';

// Platform mappings to Archive.org collections
const PLATFORM_MAPPINGS: Record<string, PlatformMapping> = {
  nes: {
    displayName: 'Nintendo Entertainment System',
    archiveCollections: ['nes_roms', 'ni-roms', 'nintendo-entertainment-system-rom-set'],
    fileExtensions: ['.nes', '.unf', '.unif', '.fds'],
    noIntroName: 'Nintendo - Nintendo Entertainment System',
    redumpName: null,
  },
  snes: {
    displayName: 'Super Nintendo Entertainment System',
    archiveCollections: ['snes_roms', 'super-nintendo-rom-set'],
    fileExtensions: ['.smc', '.sfc', '.fig', '.swc'],
    noIntroName: 'Nintendo - Super Nintendo Entertainment System',
    redumpName: null,
  },
  gba: {
    displayName: 'Game Boy Advance',
    archiveCollections: ['gba_roms', 'game-boy-advance-rom-set'],
    fileExtensions: ['.gba', '.agb'],
    noIntroName: 'Nintendo - Game Boy Advance',
    redumpName: null,
  },
  genesis: {
    displayName: 'Sega Genesis / Mega Drive',
    archiveCollections: ['genesis_roms', 'sega-genesis-rom-set', 'sega-megadrive-roms'],
    fileExtensions: ['.md', '.gen', '.bin', '.smd'],
    noIntroName: 'Sega - Mega Drive - Genesis',
    redumpName: null,
  },
  n64: {
    displayName: 'Nintendo 64',
    archiveCollections: ['n64_roms', 'nintendo-64-rom-set'],
    fileExtensions: ['.z64', '.n64', '.v64'],
    noIntroName: 'Nintendo - Nintendo 64',
    redumpName: null,
  },
  gb: {
    displayName: 'Game Boy',
    archiveCollections: ['game-boy-roms'],
    fileExtensions: ['.gb'],
    noIntroName: 'Nintendo - Game Boy',
    redumpName: null,
  },
  gbc: {
    displayName: 'Game Boy Color',
    archiveCollections: ['game-boy-color-roms'],
    fileExtensions: ['.gbc'],
    noIntroName: 'Nintendo - Game Boy Color',
    redumpName: null,
  },
  ps1: {
    displayName: 'PlayStation',
    archiveCollections: ['psx_roms', 'playstation-roms'],
    fileExtensions: ['.bin', '.cue', '.iso', '.img'],
    noIntroName: null,
    redumpName: 'Sony - PlayStation',
  },
  'master-system': {
    displayName: 'Sega Master System',
    archiveCollections: ['sega-master-system-roms'],
    fileExtensions: ['.sms'],
    noIntroName: 'Sega - Master System - Mark III',
    redumpName: null,
  },
  'game-gear': {
    displayName: 'Sega Game Gear',
    archiveCollections: ['game-gear-roms'],
    fileExtensions: ['.gg'],
    noIntroName: 'Sega - Game Gear',
    redumpName: null,
  },
};

/**
 * Normalize a ROM title for consistent matching.
 * Removes tags like (USA), [!], (Rev A), etc., lowercases, trims.
 */
function normalizeTitle(title: string): string {
  return title
    .replace(/\([^)]*\)/g, '')  // Remove parenthetical tags
    .replace(/\[[^\]]*\]/g, '') // Remove bracket tags
    .replace(/\.(nes|smc|sfc|gba|md|gen|z64|n64|v64|gb|gbc|sms|gg|bin|zip|7z)$/i, '') // Remove extension
    .replace(/[_-]+/g, ' ')    // Replace underscores/hyphens with spaces
    .replace(/\s+/g, ' ')      // Collapse whitespace
    .trim()
    .toLowerCase();
}

/**
 * Extract region from ROM filename tags like (USA), (Europe), (Japan)
 */
function extractRegion(filename: string): string | null {
  const regionPatterns: Record<string, string | null> = {
    'USA': 'USA',
    'US': 'USA',
    'United States': 'USA',
    'Europe': 'Europe',
    'EU': 'Europe',
    'Japan': 'Japan',
    'JP': 'Japan',
    'World': 'World',
    'Asia': 'Asia',
    'Australia': 'Australia',
    'Brazil': 'Brazil',
    'Canada': 'Canada',
    'China': 'China',
    'France': 'France',
    'Germany': 'Germany',
    'Italy': 'Italy',
    'Korea': 'Korea',
    'Netherlands': 'Netherlands',
    'Spain': 'Spain',
    'Sweden': 'Sweden',
    'Unknown': null,
  };

  for (const [pattern, region] of Object.entries(regionPatterns)) {
    const regex = new RegExp(`\\(${pattern}\\)`, 'i');
    if (regex.test(filename)) {
      return region;
    }
  }

  return null;
}

/**
 * Detect if a ROM file is a hack, translation, homebrew, etc.
 */
function detectRomFlags(filename: string): {
  is_hack: boolean;
  is_translation: boolean;
  is_homebrew: boolean;
  is_public_domain: boolean;
  is_verified_dump: boolean;
  version: string | null;
} {
  const lower = filename.toLowerCase();
  return {
    is_hack: /\(hack\)|\[h\d?\]|\[hack\]/i.test(filename),
    is_translation: /\(translation\)|\[t\d?\]|\[t[-+]\w+\]/i.test(filename),
    is_homebrew: /\(homebrew\)|\(pd\)|\bhomebrew\b/i.test(filename) || lower.includes('homebrew'),
    is_public_domain: /\(pd\)|\(public domain\)/i.test(filename),
    is_verified_dump: /\[!\]/.test(filename),
    version: extractVersion(filename),
  };
}

/**
 * Extract version info from filename
 */
function extractVersion(filename: string): string | null {
  const versionMatch = filename.match(/\((?:Rev|v|Version)\s*([A-Za-z0-9.]+)\)/i);
  return versionMatch ? versionMatch[1] : null;
}

/**
 * Check if a file is a ROM based on its extension
 */
function isRomFile(filename: string, platform: string): boolean {
  const mapping = PLATFORM_MAPPINGS[platform];
  if (!mapping) return false;

  const lowerName = filename.toLowerCase();

  // Always accept zip/7z archives as they likely contain ROMs
  if (lowerName.endsWith('.zip') || lowerName.endsWith('.7z')) {
    return true;
  }

  return mapping.fileExtensions.some(ext => lowerName.endsWith(ext));
}

/**
 * Generate a deterministic SHA-256 hash for ROM deduplication.
 * Since Archive.org doesn't always provide SHA256, we derive one from identifier + filename.
 */
function generateHash(identifier: string, filename: string): string {
  return createHash('sha256').update(`archive-org:${identifier}/${filename}`).digest('hex');
}

/**
 * Search Archive.org for ROM items in a specific platform collection
 */
async function searchArchiveOrg(
  platform: string,
  collection: string,
  page = 1,
  rows = 50
): Promise<ArchiveOrgSearchResponse> {
  const params = {
    q: `collection:${collection} AND mediatype:software`,
    fl: ['identifier', 'title', 'description', 'mediatype', 'collection', 'downloads', 'date', 'creator', 'subject'].join(','),
    sort: ['downloads desc'],
    rows: rows.toString(),
    page: page.toString(),
    output: 'json',
  };

  logger.info(`Searching Archive.org for ${platform} ROMs in collection "${collection}"`, { page, rows });

  const response = await axios.get<ArchiveOrgSearchResponse>(ARCHIVE_SEARCH_URL, {
    params,
    timeout: 30000,
  });

  return response.data;
}

/**
 * Get detailed metadata for an Archive.org item including file list
 */
async function getItemMetadata(identifier: string): Promise<ArchiveOrgMetadataResponse> {
  logger.debug(`Fetching metadata for ${identifier}`);

  const response = await axios.get<ArchiveOrgMetadataResponse>(
    `${ARCHIVE_METADATA_URL}/${identifier}`,
    { timeout: 30000 }
  );

  return response.data;
}

/**
 * Process an Archive.org item into ROM metadata records
 */
function processArchiveItem(
  identifier: string,
  metadata: ArchiveOrgMetadataResponse,
  platform: string,
  sourceAccountId: string
): Array<Omit<RomMetadataRecord, 'id' | 'created_at' | 'updated_at' | 'search_vector'>> {
  const roms: Array<Omit<RomMetadataRecord, 'id' | 'created_at' | 'updated_at' | 'search_vector'>> = [];

  const files = metadata.files ?? [];
  const itemTitle = metadata.metadata?.title ?? identifier;
  const itemDescription = typeof metadata.metadata?.description === 'string'
    ? metadata.metadata.description
    : undefined;
  const itemCreator = typeof metadata.metadata?.creator === 'string'
    ? metadata.metadata.creator
    : undefined;

  for (const file of files) {
    if (!file.name) continue;
    if (!isRomFile(file.name, platform)) continue;

    // Skip metadata files, thumbnails, etc.
    if (file.source === 'metadata' || file.format === 'Metadata') continue;

    const flags = detectRomFlags(file.name);
    const region = extractRegion(file.name);
    const titleNormalized = normalizeTitle(file.name);
    const downloadUrl = `${ARCHIVE_DOWNLOAD_URL}/${identifier}/${encodeURIComponent(file.name)}`;
    const hash = generateHash(identifier, file.name);

    const romData: Omit<RomMetadataRecord, 'id' | 'created_at' | 'updated_at' | 'search_vector'> = {
      source_account_id: sourceAccountId,
      rom_title: file.name,
      rom_title_normalized: titleNormalized,
      platform: platform,
      region: region,
      file_name: file.name,
      file_size_bytes: file.size ? parseInt(file.size, 10) : null,
      file_hash_md5: file.md5 ?? null,
      file_hash_sha256: hash,
      file_hash_crc32: null,
      download_url: downloadUrl,
      download_source: 'archive.org',
      download_url_verified_at: new Date(),
      download_url_dead: false,
      release_year: null,
      release_month: null,
      release_day: null,
      version: flags.version,
      quality_score: 0, // Will be calculated below
      popularity_score: 0,
      release_group: 'Archive.org',
      is_verified_dump: flags.is_verified_dump,
      is_hack: flags.is_hack,
      is_translation: flags.is_translation,
      is_homebrew: flags.is_homebrew,
      is_public_domain: flags.is_public_domain,
      game_title: titleNormalized.length > 0 ? titleNormalized : itemTitle,
      genre: null,
      publisher: itemCreator ?? null,
      developer: null,
      description: itemDescription ?? null,
      igdb_id: null,
      mobygames_id: null,
      box_art_url: `https://archive.org/services/img/${identifier}`,
      screenshot_urls: [],
      is_community_rom: false,
      community_source_url: null,
      community_update_year: null,
      scraped_from: 'archive-org',
      scraped_at: new Date(),
    };

    // Calculate quality score
    romData.quality_score = calculateQualityScore(romData);

    roms.push(romData);
  }

  return roms;
}

/**
 * Run the Archive.org scraper for all configured platforms
 */
export async function runArchiveOrgScraper(
  db: RomDiscoveryDatabase,
  sourceAccountId: string,
  options?: {
    platforms?: string[];
    maxItemsPerPlatform?: number;
    maxFilesPerItem?: number;
  }
): Promise<ScraperResult> {
  const startTime = Date.now();
  const result: ScraperResult = {
    roms_found: 0,
    roms_added: 0,
    roms_updated: 0,
    roms_removed: 0,
    errors: [],
    duration_seconds: 0,
  };

  const platforms = options?.platforms ?? Object.keys(PLATFORM_MAPPINGS);
  const maxItemsPerPlatform = options?.maxItemsPerPlatform ?? 20;

  logger.info(`Starting Archive.org scraper for ${platforms.length} platforms`);

  for (const platform of platforms) {
    const mapping = PLATFORM_MAPPINGS[platform];
    if (!mapping) {
      result.errors.push(`Unknown platform: ${platform}`);
      continue;
    }

    for (const collection of mapping.archiveCollections) {
      try {
        // Search for items in this collection
        const searchResult = await searchArchiveOrg(platform, collection, 1, maxItemsPerPlatform);
        const docs = searchResult.response?.docs ?? [];

        logger.info(`Found ${docs.length} items in collection "${collection}" for ${platform}`);

        for (const doc of docs) {
          try {
            // Get detailed file metadata for each item
            const itemMetadata = await getItemMetadata(doc.identifier);

            // Process files into ROM records
            const romRecords = processArchiveItem(
              doc.identifier,
              itemMetadata,
              platform,
              sourceAccountId
            );

            // Limit files per item if specified
            const recordsToInsert = options?.maxFilesPerItem
              ? romRecords.slice(0, options.maxFilesPerItem)
              : romRecords;

            result.roms_found += recordsToInsert.length;

            // Upsert each ROM
            for (const romData of recordsToInsert) {
              try {
                const upserted = await db.upsertRomMetadata(romData);
                if (upserted) {
                  // Determine if it was an insert or update based on created_at vs updated_at
                  const created = new Date(upserted.created_at).getTime();
                  const updated = new Date(upserted.updated_at).getTime();
                  if (Math.abs(updated - created) < 1000) {
                    result.roms_added++;
                  } else {
                    result.roms_updated++;
                  }
                }
              } catch (upsertError) {
                const msg = upsertError instanceof Error ? upsertError.message : 'Unknown error';
                result.errors.push(`Failed to upsert ROM "${romData.file_name}": ${msg}`);
              }
            }

            // Small delay between items to be respectful to Archive.org
            await new Promise(resolve => setTimeout(resolve, 500));
          } catch (itemError) {
            const msg = itemError instanceof Error ? itemError.message : 'Unknown error';
            result.errors.push(`Failed to process item "${doc.identifier}": ${msg}`);
            logger.warn(`Failed to process Archive.org item`, { identifier: doc.identifier, error: msg });
          }
        }

        // Delay between collections
        await new Promise(resolve => setTimeout(resolve, 1000));
      } catch (searchError) {
        const msg = searchError instanceof Error ? searchError.message : 'Unknown error';
        result.errors.push(`Failed to search collection "${collection}": ${msg}`);
        logger.error(`Archive.org search failed for collection`, { collection, error: msg });
      }
    }
  }

  result.duration_seconds = Math.round((Date.now() - startTime) / 1000);

  logger.info('Archive.org scraper completed', {
    found: result.roms_found,
    added: result.roms_added,
    updated: result.roms_updated,
    errors: result.errors.length,
    duration: result.duration_seconds,
  });

  return result;
}

export { PLATFORM_MAPPINGS, normalizeTitle, extractRegion, detectRomFlags, isRomFile };
