/**
 * No-Intro DAT File Scraper
 * Reads No-Intro DAT XML files from a local directory and imports verified ROM metadata.
 * No-Intro provides curated checksums for cartridge-based ROMs (~50,000 entries).
 * DAT files must be manually downloaded from datomatic.no-intro.org.
 */

import { createHash } from 'node:crypto';
import { readdir, readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { createLogger } from '@nself/plugin-utils';
import { parseDatXml, extractRegion, extractRevision, extractLanguages } from './dat-parser.js';
import { calculateQualityScore } from '../scoring.js';
import type { ScraperResult } from '../types.js';
import type { RomDiscoveryDatabase } from '../database.js';

const logger = createLogger('rom-discovery:no-intro');

// =============================================================================
// Platform Mappings
// =============================================================================

/**
 * Maps our internal platform identifiers to No-Intro DAT file name patterns.
 * No-Intro uses standardized naming like "Nintendo - Nintendo Entertainment System (20240101-123456).dat"
 */
const PLATFORM_DAT_PATTERNS: Record<string, string> = {
  nes: 'Nintendo - Nintendo Entertainment System',
  snes: 'Nintendo - Super Nintendo Entertainment System',
  gba: 'Nintendo - Game Boy Advance',
  genesis: 'Sega - Mega Drive - Genesis',
  n64: 'Nintendo - Nintendo 64',
  gb: 'Nintendo - Game Boy',
  gbc: 'Nintendo - Game Boy Color',
};

/**
 * Default directory where DAT files are stored.
 * Configurable via ROM_DISCOVERY_DAT_DIR environment variable.
 */
function getDatDirectory(): string {
  return process.env.ROM_DISCOVERY_DAT_DIR ?? '/data/rom-discovery/dats/no-intro/';
}

// =============================================================================
// Title Normalization
// =============================================================================

/**
 * Normalize a ROM title for consistent matching and deduplication.
 * Removes parenthetical tags, bracket tags, lowercases, and trims.
 */
function normalizeTitle(title: string): string {
  return title
    .replace(/\([^)]*\)/g, '')   // Remove parenthetical tags: (USA), (Rev A), etc.
    .replace(/\[[^\]]*\]/g, '')  // Remove bracket tags: [!], [b], etc.
    .replace(/[_-]+/g, ' ')     // Replace underscores/hyphens with spaces
    .replace(/\s+/g, ' ')       // Collapse whitespace
    .trim()
    .toLowerCase();
}

// =============================================================================
// Hash Generation
// =============================================================================

/**
 * Generate a deterministic SHA-256 hash for ROM deduplication.
 * The database has a UNIQUE constraint on (source_account_id, file_hash_sha256).
 */
function generateHash(platform: string, gameName: string, romName: string): string {
  return createHash('sha256').update(`no-intro:${platform}:${gameName}:${romName}`).digest('hex');
}

// =============================================================================
// DAT File Discovery
// =============================================================================

/**
 * Find the most recent DAT file matching a platform pattern in the DAT directory.
 * Returns the full path to the file, or null if not found.
 */
async function findDatFile(datDir: string, platformPattern: string): Promise<string | null> {
  try {
    const files = await readdir(datDir);

    // Build a regex from the platform pattern:
    // "Nintendo - Nintendo Entertainment System" -> matches files containing that string
    // Escape special regex characters and replace spaces with flexible whitespace
    const escaped = platformPattern.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const pattern = new RegExp(escaped.replace(/\s+/g, '\\s+'), 'i');

    const matches = files
      .filter(f => f.endsWith('.dat') && pattern.test(f))
      .sort(); // Sort alphabetically; latest date-stamped file will be last

    if (matches.length === 0) {
      return null;
    }

    // Return the last match (most recent by filename convention)
    return join(datDir, matches[matches.length - 1]);
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    logger.warn('Failed to scan DAT directory', { datDir, error: msg });
    return null;
  }
}

// =============================================================================
// Main Scraper
// =============================================================================

/**
 * Run the No-Intro scraper for a specific platform.
 * Reads the corresponding DAT XML file from the configured directory,
 * parses it using the shared dat-parser, and upserts ROM metadata records.
 *
 * @param db - Database instance for upserting records
 * @param sourceAccountId - Multi-app isolation column value
 * @param platform - Platform identifier (nes, snes, gba, genesis, n64, gb, gbc)
 * @returns ScraperResult with counts and any errors
 */
export async function runNoIntroScraper(
  db: RomDiscoveryDatabase,
  sourceAccountId: string,
  platform: string,
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

  const datPattern = PLATFORM_DAT_PATTERNS[platform];
  if (!datPattern) {
    result.errors.push(`Unknown No-Intro platform: ${platform}`);
    result.duration_seconds = Math.round((Date.now() - startTime) / 1000);
    return result;
  }

  const datDir = getDatDirectory();
  logger.info(`No-Intro scraper starting for platform "${platform}"`, {
    datDir,
    datPattern,
  });

  // Find the DAT file
  const datFilePath = await findDatFile(datDir, datPattern);
  if (!datFilePath) {
    const msg = `No DAT file found for pattern "${datPattern}" in directory "${datDir}". ` +
      'Download DAT files from datomatic.no-intro.org and place them in the configured directory.';
    result.errors.push(msg);
    logger.warn(msg);
    result.duration_seconds = Math.round((Date.now() - startTime) / 1000);
    return result;
  }

  logger.info(`Found DAT file: ${datFilePath}`);

  // Read and parse the DAT file
  let xmlContent: string;
  try {
    xmlContent = await readFile(datFilePath, 'utf-8');
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    result.errors.push(`Failed to read DAT file "${datFilePath}": ${msg}`);
    result.duration_seconds = Math.round((Date.now() - startTime) / 1000);
    return result;
  }

  const datFile = parseDatXml(xmlContent);

  logger.info(`Parsed No-Intro DAT: ${datFile.header.name}`, {
    version: datFile.header.version,
    gameCount: datFile.games.length,
  });

  // Process each game entry
  for (const game of datFile.games) {
    for (const rom of game.roms) {
      try {
        const region = extractRegion(game.name);
        const revision = extractRevision(game.name);
        const languages = extractLanguages(game.name);
        const titleNormalized = normalizeTitle(game.name);
        const hash = generateHash(platform, game.name, rom.name);

        // Build description with language data from DAT file
        const description = languages.length > 0 ? `Languages: ${languages.join(', ')}` : null;

        const romData = {
          source_account_id: sourceAccountId,
          rom_title: game.name,
          rom_title_normalized: titleNormalized,
          platform,
          region,
          file_name: rom.name,
          file_size_bytes: rom.size ?? null,
          file_hash_md5: rom.md5 ?? null,
          file_hash_sha256: hash,
          file_hash_crc32: rom.crc32 ?? null,
          download_url: null,
          download_source: 'no-intro',
          download_url_verified_at: null,
          download_url_dead: false,
          release_year: game.year ? parseInt(game.year, 10) : null,
          release_month: null,
          release_day: null,
          version: revision,
          quality_score: 0, // Calculated below
          popularity_score: 0,
          release_group: 'No-Intro',
          is_verified_dump: true,
          is_hack: false,
          is_translation: false,
          is_homebrew: false,
          is_public_domain: false,
          game_title: game.description ?? titleNormalized,
          genre: null,
          publisher: game.manufacturer ?? null,
          developer: null,
          description,
          igdb_id: null,
          mobygames_id: null,
          box_art_url: null,
          screenshot_urls: [] as string[],
          is_community_rom: false,
          community_source_url: null,
          community_update_year: null,
          scraped_from: 'no-intro',
          scraped_at: new Date(),
        };

        // Calculate quality score using the existing scoring module
        romData.quality_score = calculateQualityScore(romData);

        result.roms_found++;

        const upserted = await db.upsertRomMetadata(romData);
        if (upserted) {
          const created = new Date(upserted.created_at).getTime();
          const updated = new Date(upserted.updated_at).getTime();
          if (Math.abs(updated - created) < 1000) {
            result.roms_added++;
          } else {
            result.roms_updated++;
          }
        }
      } catch (error) {
        const msg = error instanceof Error ? error.message : 'Unknown error';
        result.errors.push(`Failed to upsert No-Intro ROM "${rom.name}" from game "${game.name}": ${msg}`);
      }
    }
  }

  result.duration_seconds = Math.round((Date.now() - startTime) / 1000);

  logger.info('No-Intro scraper completed', {
    platform,
    found: result.roms_found,
    added: result.roms_added,
    updated: result.roms_updated,
    errors: result.errors.length,
    duration: result.duration_seconds,
  });

  return result;
}
