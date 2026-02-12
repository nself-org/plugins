/**
 * tecmobowl.org Community ROM Scraper
 * Scrapes tecmobowl.org for community-updated Tecmo Bowl ROMs with yearly roster updates.
 * These are ROM hacks (not homebrew) with updated rosters and gameplay modifications.
 * Typically ~100-200 ROMs spanning NES and SNES platforms.
 */

import { createHash } from 'node:crypto';
import { createLogger } from '@nself/plugin-utils';
import { calculateQualityScore } from '../scoring.js';
import type { ScraperResult } from '../types.js';
import type { RomDiscoveryDatabase } from '../database.js';

const logger = createLogger('rom-discovery:tecmobowl');

// =============================================================================
// Configuration
// =============================================================================

/**
 * CSS selectors used for scraping. Configurable for future-proofing
 * in case the site structure changes.
 */
const DEFAULT_SELECTORS = {
  /** Selector for ROM download link elements */
  romLinks: 'a[href]',
  /** Selector for ROM title/description containers */
  romContainers: '.download-item, .rom-entry, article, .post, li',
  /** Selector for page content area */
  contentArea: 'main, .content, #content, .site-content, body',
};

/**
 * ROM file extensions we consider valid for Tecmo Bowl ROMs.
 */
const ROM_EXTENSIONS = ['.nes', '.smc', '.sfc', '.zip', '.7z'];

/**
 * Delay between HTTP requests in milliseconds (1-2 seconds).
 */
const REQUEST_DELAY_MS = 1500;

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Generate a deterministic SHA-256 hash for ROM deduplication.
 */
function generateHash(scraperName: string, url: string): string {
  return createHash('sha256').update(`${scraperName}:${url}`).digest('hex');
}

/**
 * Normalize a ROM title for consistent matching.
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

/**
 * Extract the year from a Tecmo Bowl ROM name.
 * E.g., "Tecmo Super Bowl 2025" -> 2025
 *        "TSB 2024 Roster Update" -> 2024
 */
function extractYear(name: string): number | null {
  // Look for 4-digit years from 1990-2099
  const yearMatch = name.match(/\b(19[9]\d|20[0-9]\d)\b/);
  if (yearMatch) {
    return parseInt(yearMatch[1], 10);
  }
  return null;
}

/**
 * Guess the platform from a filename or URL.
 * Tecmo Bowl ROMs are primarily NES and SNES.
 */
function guessPlatform(filename: string): string {
  const lower = filename.toLowerCase();

  if (lower.includes('.smc') || lower.includes('.sfc') || lower.includes('snes') || lower.includes('super')) {
    return 'snes';
  }

  if (lower.includes('.nes') || lower.includes('nes')) {
    return 'nes';
  }

  // Default to NES for original Tecmo Bowl
  return 'nes';
}

/**
 * Check if a URL points to a downloadable ROM file.
 */
function isRomUrl(href: string): boolean {
  const lower = href.toLowerCase();
  return ROM_EXTENSIONS.some(ext => lower.endsWith(ext));
}

/**
 * Resolve a potentially relative URL to an absolute URL.
 */
function resolveUrl(href: string, baseUrl: string): string {
  if (href.startsWith('http://') || href.startsWith('https://')) {
    return href;
  }
  try {
    return new URL(href, baseUrl).toString();
  } catch {
    return href;
  }
}

/**
 * Sleep for a specified number of milliseconds.
 */
function delay(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

// =============================================================================
// Page Fetching and Parsing
// =============================================================================

interface RomLink {
  title: string;
  url: string;
  description: string | null;
  platform: string;
  year: number | null;
  author: string | null;
}

/**
 * Fetch a page and extract ROM links using cheerio.
 */
async function fetchAndParseRomLinks(
  pageUrl: string,
  selectors: typeof DEFAULT_SELECTORS,
): Promise<RomLink[]> {
  const { load } = await import('cheerio');
  const axios = (await import('axios')).default;

  logger.info(`Fetching page: ${pageUrl}`);

  const response = await axios.get(pageUrl, {
    timeout: 30000,
    headers: {
      'User-Agent': 'nself-rom-discovery/1.0 (metadata indexer)',
      'Accept': 'text/html,application/xhtml+xml',
    },
  });

  const $ = load(response.data as string);
  const romLinks: RomLink[] = [];

  // Strategy 1: Find direct download links to ROM files
  $(selectors.romLinks).each((_index, element) => {
    const href = $(element).attr('href') ?? '';
    const text = $(element).text().trim();

    if (!isRomUrl(href) || text.length === 0) return;

    const fullUrl = resolveUrl(href, pageUrl);
    const fileName = href.split('/').pop() ?? text;
    const platform = guessPlatform(fileName);
    const year = extractYear(text) ?? extractYear(fileName);

    // Try to get surrounding context for description and author
    const parentText = $(element).parent().text().trim();
    const description = parentText.length > text.length ? parentText : null;

    // Try to find author information in nearby elements
    const authorElement = $(element).closest(selectors.romContainers).find('.author, .creator, [rel="author"]');
    const author = authorElement.length > 0 ? authorElement.first().text().trim() : null;

    romLinks.push({
      title: text,
      url: fullUrl,
      description,
      platform,
      year,
      author,
    });
  });

  // Strategy 2: Look for links within content containers that point to ROM files
  if (romLinks.length === 0) {
    $(selectors.contentArea).find('a[href]').each((_index, element) => {
      const href = $(element).attr('href') ?? '';
      const text = $(element).text().trim();

      if (!isRomUrl(href) || text.length === 0) return;

      const fullUrl = resolveUrl(href, pageUrl);
      const fileName = href.split('/').pop() ?? text;
      const platform = guessPlatform(fileName);
      const year = extractYear(text) ?? extractYear(fileName);

      // Avoid duplicates
      if (romLinks.some(r => r.url === fullUrl)) return;

      romLinks.push({
        title: text,
        url: fullUrl,
        description: null,
        platform,
        year,
        author: null,
      });
    });
  }

  return romLinks;
}

/**
 * Discover additional pages to scrape from the main page.
 * Looks for navigation links, pagination, and category links.
 */
async function discoverSubPages(
  mainUrl: string,
): Promise<string[]> {
  const { load } = await import('cheerio');
  const axios = (await import('axios')).default;

  const subPages: string[] = [];

  try {
    const response = await axios.get(mainUrl, {
      timeout: 30000,
      headers: {
        'User-Agent': 'nself-rom-discovery/1.0 (metadata indexer)',
        'Accept': 'text/html,application/xhtml+xml',
      },
    });

    const $ = load(response.data as string);

    // Look for links that might lead to download/ROM pages
    const downloadPatterns = [
      /download/i,
      /rom/i,
      /patch/i,
      /file/i,
      /tecmo.*bowl/i,
      /tsb/i,
    ];

    $('a[href]').each((_index, element) => {
      const href = $(element).attr('href') ?? '';
      const text = $(element).text().trim();

      // Only follow internal links
      const fullUrl = resolveUrl(href, mainUrl);
      try {
        const linkHost = new URL(fullUrl).hostname;
        const baseHost = new URL(mainUrl).hostname;
        if (linkHost !== baseHost) return;
      } catch {
        return;
      }

      // Check if this looks like a relevant sub-page
      const isRelevant = downloadPatterns.some(p => p.test(href) || p.test(text));
      if (isRelevant && !subPages.includes(fullUrl) && fullUrl !== mainUrl) {
        subPages.push(fullUrl);
      }
    });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    logger.warn('Failed to discover sub-pages', { url: mainUrl, error: msg });
  }

  // Limit to a reasonable number of sub-pages
  return subPages.slice(0, 10);
}

// =============================================================================
// Main Scraper
// =============================================================================

/**
 * Run the tecmobowl.org scraper.
 * Fetches the site's pages, extracts ROM download links, and upserts metadata.
 *
 * Key behaviors:
 * - Sets is_community_rom = true (these are community ROM hacks)
 * - Sets is_homebrew = false (ROM hacks, not original homebrew)
 * - Extracts year from ROM names for community_update_year
 * - Delays 1-2 seconds between HTTP requests
 * - Uses configurable CSS selectors for future-proofing
 *
 * @param db - Database instance for upserting records
 * @param sourceAccountId - Multi-app isolation column value
 * @param sourceUrl - Base URL to scrape (e.g., "https://tecmobowl.org")
 * @returns ScraperResult with counts and any errors
 */
export async function runTecmobowlScraper(
  db: RomDiscoveryDatabase,
  sourceAccountId: string,
  sourceUrl: string,
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

  const selectors = DEFAULT_SELECTORS;

  logger.info(`Tecmobowl scraper starting`, { sourceUrl });

  try {
    // Phase 1: Discover pages to scrape
    const subPages = await discoverSubPages(sourceUrl);
    const pagesToScrape = [sourceUrl, ...subPages];

    logger.info(`Found ${pagesToScrape.length} pages to scrape`, {
      pages: pagesToScrape,
    });

    // Phase 2: Scrape each page for ROM links
    const allRomLinks: RomLink[] = [];
    const seenUrls = new Set<string>();

    for (const pageUrl of pagesToScrape) {
      try {
        const romLinks = await fetchAndParseRomLinks(pageUrl, selectors);

        for (const link of romLinks) {
          if (!seenUrls.has(link.url)) {
            seenUrls.add(link.url);
            allRomLinks.push(link);
          }
        }

        logger.info(`Scraped page`, {
          url: pageUrl,
          romLinksFound: romLinks.length,
          totalUnique: allRomLinks.length,
        });

        // Respectful delay between requests
        await delay(REQUEST_DELAY_MS);
      } catch (error) {
        const msg = error instanceof Error ? error.message : 'Unknown error';
        result.errors.push(`Failed to scrape page "${pageUrl}": ${msg}`);
        logger.warn('Failed to scrape page', { url: pageUrl, error: msg });
      }
    }

    result.roms_found = allRomLinks.length;
    logger.info(`Total ROM links found: ${allRomLinks.length}`);

    // Phase 3: Upsert ROM metadata
    for (const link of allRomLinks) {
      try {
        const hash = generateHash('tecmobowl', link.url);
        const titleNormalized = normalizeTitle(link.title);
        const fileName = link.url.split('/').pop() ?? link.title;

        const romData = {
          source_account_id: sourceAccountId,
          rom_title: link.title,
          rom_title_normalized: titleNormalized,
          platform: link.platform,
          region: null,
          file_name: fileName,
          file_size_bytes: null,
          file_hash_md5: null,
          file_hash_sha256: hash,
          file_hash_crc32: null,
          download_url: link.url,
          download_source: 'tecmobowl',
          download_url_verified_at: new Date(),
          download_url_dead: false,
          release_year: link.year,
          release_month: null,
          release_day: null,
          version: null,
          quality_score: 0, // Calculated below
          popularity_score: 0,
          release_group: 'Community',
          is_verified_dump: false,
          is_hack: false,
          is_translation: false,
          is_homebrew: false,
          is_public_domain: false,
          game_title: link.title,
          genre: 'Sports',
          publisher: link.author,
          developer: null,
          description: link.description,
          igdb_id: null,
          mobygames_id: null,
          box_art_url: null,
          screenshot_urls: [] as string[],
          is_community_rom: true,
          community_source_url: sourceUrl,
          community_update_year: link.year,
          scraped_from: 'tecmobowl',
          scraped_at: new Date(),
        };

        // Calculate quality score (community source base = 35 via scoring.ts)
        romData.quality_score = calculateQualityScore(romData);

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
        result.errors.push(`Failed to upsert tecmobowl ROM "${link.title}": ${msg}`);
      }
    }
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    result.errors.push(`Tecmobowl scraper failed: ${msg}`);
    logger.error('Tecmobowl scraper failed', { error: msg });
  }

  result.duration_seconds = Math.round((Date.now() - startTime) / 1000);

  logger.info('Tecmobowl scraper completed', {
    found: result.roms_found,
    added: result.roms_added,
    updated: result.roms_updated,
    errors: result.errors.length,
    duration: result.duration_seconds,
  });

  return result;
}
