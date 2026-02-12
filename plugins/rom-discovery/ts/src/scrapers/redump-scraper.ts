/**
 * Redump DAT File Scraper
 * Reads Redump DAT XML files from a local directory and imports verified disc-based ROM metadata.
 * Redump provides curated checksums for disc-based systems (PS1, PS2, Saturn, Dreamcast, GameCube).
 * DAT files must be manually downloaded from redump.org/downloads/.
 */

import { createHash } from 'node:crypto';
import { readdir, readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { createLogger } from '@nself/plugin-utils';
import { parseDatXml, extractRegion, extractRevision, extractLanguages } from './dat-parser.js';
import { calculateQualityScore } from '../scoring.js';
import type { ScraperResult } from '../types.js';
import type { RomDiscoveryDatabase } from '../database.js';

const logger = createLogger('rom-discovery:redump');

// =============================================================================
// Platform Mappings
// =============================================================================

/**
 * Maps internal platform identifiers to Redump DAT file name patterns.
 * Redump uses naming like "Sony - PlayStation (20240101 12-34-56).dat"
 */
const PLATFORM_DAT_PATTERNS: Record<string, string> = {
  ps1: 'Sony - PlayStation',
  ps2: 'Sony - PlayStation 2',
  saturn: 'Sega - Saturn',
  dreamcast: 'Sega - Dreamcast',
  gamecube: 'Nintendo - GameCube',
};

/**
 * Get the Redump DAT directory. Stored under a 'redump/' subdirectory
 * within the main DAT directory.
 */
function getDatDirectory(): string {
  const baseDir = process.env.ROM_DISCOVERY_DAT_DIR ?? '/data/rom-discovery/dats/';
  return join(baseDir, 'redump');
}

// =============================================================================
// Title Normalization
// =============================================================================

/**
 * Normalize a ROM title for consistent matching and deduplication.
 */
function normalizeTitle(title: string): string {
  return title
    .replace(/\([^)]*\)/g, '')
    .replace(/\[[^\]]*\]/g, '')
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .toLowerCase();
}

// =============================================================================
// Multi-Disc Detection
// =============================================================================

/**
 * Detect disc number from a game name or ROM name.
 * Redump games can have multiple discs: "Game (Disc 1)", "Game (Disc 2)", etc.
 * Also handles Track entries within a disc (Track 01, Track 02 for bin/cue).
 *
 * @returns Object with disc count info, or null if single disc
 */
function detectMultiDisc(gameName: string, roms: Array<{ name: string }>): {
  discNumber: number | null;
  totalDiscs: number | null;
  trackCount: number;
} {
  // Check for disc number in game name
  const discMatch = gameName.match(/\(Disc\s+(\d+)\)/i);
  const discNumber = discMatch ? parseInt(discMatch[1], 10) : null;

  // Count tracks (bin/cue files have multiple tracks)
  const trackCount = roms.filter(r =>
    /track\s*\d+/i.test(r.name) || r.name.toLowerCase().endsWith('.bin')
  ).length;

  return {
    discNumber,
    totalDiscs: null, // Total can only be determined by looking at all entries
    trackCount: Math.max(trackCount, 1),
  };
}

// =============================================================================
// Hash Generation
// =============================================================================

/**
 * Generate a deterministic SHA-256 hash for ROM deduplication.
 * The database has a UNIQUE constraint on (source_account_id, file_hash_sha256).
 */
function generateHash(platform: string, gameName: string, romName: string): string {
  return createHash('sha256').update(`redump:${platform}:${gameName}:${romName}`).digest('hex');
}

// =============================================================================
// DAT File Discovery
// =============================================================================

/**
 * Find the most recent DAT file matching a platform pattern in the Redump DAT directory.
 */
async function findDatFile(datDir: string, platformPattern: string): Promise<string | null> {
  try {
    const files = await readdir(datDir);
    const escaped = platformPattern.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const pattern = new RegExp(escaped.replace(/\s+/g, '\\s+'), 'i');

    const matches = files
      .filter(f => f.endsWith('.dat') && pattern.test(f))
      .sort();

    if (matches.length === 0) {
      return null;
    }

    return join(datDir, matches[matches.length - 1]);
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    logger.warn('Failed to scan Redump DAT directory', { datDir, error: msg });
    return null;
  }
}

// =============================================================================
// Main Scraper
// =============================================================================

/**
 * Run the Redump scraper for a specific platform.
 * Reads the corresponding DAT XML file from the configured redump/ subdirectory,
 * parses it using the shared dat-parser, and upserts ROM metadata records.
 *
 * Handles multi-disc games by storing disc info in the metadata and creating
 * individual entries per ROM file (tracks within a disc image).
 *
 * @param db - Database instance for upserting records
 * @param sourceAccountId - Multi-app isolation column value
 * @param platform - Platform identifier (ps1, ps2, saturn, dreamcast, gamecube)
 * @returns ScraperResult with counts and any errors
 */
export async function runRedumpScraper(
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
    result.errors.push(`Unknown Redump platform: ${platform}`);
    result.duration_seconds = Math.round((Date.now() - startTime) / 1000);
    return result;
  }

  const datDir = getDatDirectory();
  logger.info(`Redump scraper starting for platform "${platform}"`, {
    datDir,
    datPattern,
  });

  // Find the DAT file
  const datFilePath = await findDatFile(datDir, datPattern);
  if (!datFilePath) {
    const msg = `No DAT file found for pattern "${datPattern}" in directory "${datDir}". ` +
      'Download DAT files from redump.org/downloads/ and place them in the configured redump/ subdirectory.';
    result.errors.push(msg);
    logger.warn(msg);
    result.duration_seconds = Math.round((Date.now() - startTime) / 1000);
    return result;
  }

  logger.info(`Found Redump DAT file: ${datFilePath}`);

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

  logger.info(`Parsed Redump DAT: ${datFile.header.name}`, {
    version: datFile.header.version,
    gameCount: datFile.games.length,
  });

  // Process each game entry
  for (const game of datFile.games) {
    const discInfo = detectMultiDisc(game.name, game.roms);
    const region = extractRegion(game.name);
    const revision = extractRevision(game.name);
    const languages = extractLanguages(game.name);
    const titleNormalized = normalizeTitle(game.name);

    for (const rom of game.roms) {
      try {
        const hash = generateHash(platform, game.name, rom.name);

        // Build description with disc/track metadata and language data
        const descParts: string[] = [];
        if (discInfo.discNumber !== null) {
          descParts.push(`Disc ${discInfo.discNumber}`);
        }
        if (discInfo.trackCount > 1) {
          descParts.push(`${discInfo.trackCount} tracks`);
        }
        if (rom.serial) {
          descParts.push(`Serial: ${rom.serial}`);
        }
        if (languages.length > 0) {
          descParts.push(`Languages: ${languages.join(', ')}`);
        }
        const description = descParts.length > 0 ? descParts.join(', ') : null;

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
          download_source: 'redump',
          download_url_verified_at: null,
          download_url_dead: false,
          release_year: game.year ? parseInt(game.year, 10) : null,
          release_month: null,
          release_day: null,
          version: revision,
          quality_score: 0, // Calculated below
          popularity_score: 0,
          release_group: 'Redump',
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
          scraped_from: 'redump',
          scraped_at: new Date(),
        };

        // Calculate quality score
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
        result.errors.push(`Failed to upsert Redump ROM "${rom.name}" from game "${game.name}": ${msg}`);
      }
    }
  }

  result.duration_seconds = Math.round((Date.now() - startTime) / 1000);

  logger.info('Redump scraper completed', {
    platform,
    found: result.roms_found,
    added: result.roms_added,
    updated: result.roms_updated,
    errors: result.errors.length,
    duration: result.duration_seconds,
  });

  return result;
}
