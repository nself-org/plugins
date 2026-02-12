/**
 * Scraper Scheduler
 * Cron-based scheduling for ROM scraper jobs
 */

import { createHash } from 'node:crypto';
import cron from 'node-cron';
import { createLogger } from '@nself/plugin-utils';
import { runArchiveOrgScraper } from './archive-org-scraper.js';
import { runNoIntroScraper } from './no-intro-scraper.js';
import { runRedumpScraper } from './redump-scraper.js';
import { runTecmobowlScraper } from './tecmobowl-scraper.js';
import type { RomDiscoveryDatabase } from '../database.js';
import type { ScraperResult } from '../types.js';

const logger = createLogger('rom-discovery:scheduler');

const SCRAPER_TIMEOUT_MS = parseInt(process.env.ROM_DISCOVERY_SCRAPER_TIMEOUT_MS ?? '300000', 10); // 5 minutes default

interface ScheduledTask {
  scraperName: string;
  task: cron.ScheduledTask;
}

export class ScraperScheduler {
  private db: RomDiscoveryDatabase;
  private sourceAccountId: string;
  private tasks: ScheduledTask[] = [];
  private running = false;

  constructor(db: RomDiscoveryDatabase, sourceAccountId = 'primary') {
    this.db = db;
    this.sourceAccountId = sourceAccountId;
  }

  /**
   * Start the scheduler: reads all enabled scrapers from the database
   * and creates cron jobs for each one
   */
  async start(): Promise<void> {
    if (this.running) {
      logger.warn('Scheduler is already running');
      return;
    }

    logger.info('Starting scraper scheduler...');

    const scrapers = await this.db.getScrapers();
    const enabledScrapers = scrapers.filter(s => s.enabled);

    for (const scraper of enabledScrapers) {
      if (!cron.validate(scraper.cron_schedule)) {
        logger.error(`Invalid cron schedule for scraper "${scraper.scraper_name}": ${scraper.cron_schedule}`);
        continue;
      }

      const task = cron.schedule(scraper.cron_schedule, async () => {
        await this.runScraper(scraper.scraper_name);
      });

      this.tasks.push({ scraperName: scraper.scraper_name, task });
      logger.info(`Scheduled scraper "${scraper.scraper_name}" with cron: ${scraper.cron_schedule}`);
    }

    this.running = true;
    logger.info(`Scheduler started with ${this.tasks.length} scheduled scrapers`);
  }

  /**
   * Stop the scheduler and destroy all cron jobs
   */
  stop(): void {
    logger.info('Stopping scraper scheduler...');

    for (const scheduled of this.tasks) {
      scheduled.task.stop();
    }

    this.tasks = [];
    this.running = false;
    logger.info('Scheduler stopped');
  }

  /**
   * Run a specific scraper by name
   */
  async runScraper(scraperName: string): Promise<ScraperResult> {
    logger.info(`Running scraper: ${scraperName}`);

    const scraper = await this.db.getScraperByName(scraperName);
    if (!scraper) {
      const errorResult: ScraperResult = {
        roms_found: 0,
        roms_added: 0,
        roms_updated: 0,
        roms_removed: 0,
        errors: [`Scraper "${scraperName}" not found`],
        duration_seconds: 0,
      };
      return errorResult;
    }

    const startTime = Date.now();
    let result: ScraperResult;

    try {
      // Wrap scraper execution with a timeout to prevent indefinite hangs
      const timeoutPromise = new Promise<never>((_, reject) =>
        setTimeout(() => reject(new Error(`Scraper "${scraperName}" timed out after ${SCRAPER_TIMEOUT_MS}ms`)), SCRAPER_TIMEOUT_MS)
      );

      let scraperPromise: Promise<ScraperResult>;

      switch (scraper.scraper_type) {
        case 'archive-org':
          scraperPromise = runArchiveOrgScraper(this.db, this.sourceAccountId, {
            maxItemsPerPlatform: 20,
            maxFilesPerItem: 100,
          });
          break;

        case 'no-intro':
          scraperPromise = this.runNoIntroScraper_(scraperName);
          break;

        case 'redump':
          scraperPromise = this.runRedumpScraper_(scraperName);
          break;

        case 'web-scraper':
          scraperPromise = this.runWebScraper_(scraperName, scraper.scraper_source_url);
          break;

        default:
          scraperPromise = Promise.resolve({
            roms_found: 0,
            roms_added: 0,
            roms_updated: 0,
            roms_removed: 0,
            errors: [`Unknown scraper type: ${scraper.scraper_type}`],
            duration_seconds: 0,
          });
      }

      result = await Promise.race([scraperPromise, timeoutPromise]);

      // Update scraper job record with results
      await this.db.updateScraperResults(scraperName, {
        status: result.errors.length > 0 ? 'completed_with_errors' : 'success',
        duration_seconds: Math.round((Date.now() - startTime) / 1000),
        roms_found: result.roms_found,
        roms_added: result.roms_added,
        roms_updated: result.roms_updated,
        roms_removed: result.roms_removed,
        errors: result.errors.slice(0, 50), // Limit stored errors
      });

      logger.info(`Scraper "${scraperName}" completed`, {
        found: result.roms_found,
        added: result.roms_added,
        updated: result.roms_updated,
        errors: result.errors.length,
      });

    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`Scraper "${scraperName}" failed`, { error: message });

      result = {
        roms_found: 0,
        roms_added: 0,
        roms_updated: 0,
        roms_removed: 0,
        errors: [message],
        duration_seconds: Math.round((Date.now() - startTime) / 1000),
      };

      await this.db.updateScraperResults(scraperName, {
        status: 'failed',
        duration_seconds: result.duration_seconds,
        roms_found: 0,
        roms_added: 0,
        roms_updated: 0,
        roms_removed: 0,
        errors: [message],
      });
    }

    return result;
  }

  /**
   * No-Intro scraper: reads DAT XML files from a local directory and parses ROM entries.
   * No-Intro provides curated checksums for verified ROM dumps.
   * DAT files must be manually downloaded from datomatic.no-intro.org and placed
   * in the configured DAT directory (ROM_DISCOVERY_DAT_DIR env var).
   */
  private async runNoIntroScraper_(scraperName: string): Promise<ScraperResult> {
    // Map scraper names to platform identifiers
    const platformMap: Record<string, string> = {
      'no-intro-nes': 'nes',
      'no-intro-snes': 'snes',
      'no-intro-gba': 'gba',
      'no-intro-genesis': 'genesis',
      'no-intro-n64': 'n64',
      'no-intro-gb': 'gb',
      'no-intro-gbc': 'gbc',
    };

    const platform = platformMap[scraperName];
    if (!platform) {
      return {
        roms_found: 0,
        roms_added: 0,
        roms_updated: 0,
        roms_removed: 0,
        errors: [`Unknown No-Intro scraper: ${scraperName}`],
        duration_seconds: 0,
      };
    }

    return runNoIntroScraper(this.db, this.sourceAccountId, platform);
  }

  /**
   * Redump scraper: reads DAT XML files from a local directory for disc-based systems.
   * DAT files must be manually downloaded from redump.org/downloads/ and placed
   * in the configured DAT directory under a 'redump/' subdirectory.
   */
  private async runRedumpScraper_(scraperName: string): Promise<ScraperResult> {
    // Map scraper names to platform identifiers
    const platformMap: Record<string, string> = {
      'redump-ps1': 'ps1',
      'redump-ps2': 'ps2',
      'redump-saturn': 'saturn',
      'redump-dreamcast': 'dreamcast',
      'redump-gamecube': 'gamecube',
    };

    const platform = platformMap[scraperName];
    if (!platform) {
      return {
        roms_found: 0,
        roms_added: 0,
        roms_updated: 0,
        roms_removed: 0,
        errors: [`Unknown Redump scraper: ${scraperName}`],
        duration_seconds: 0,
      };
    }

    return runRedumpScraper(this.db, this.sourceAccountId, platform);
  }

  /**
   * Web scraper for community sites.
   * Delegates to the dedicated tecmobowl scraper module for tecmobowl.org,
   * or falls back to generic link extraction for other community sites.
   */
  private async runWebScraper_(scraperName: string, sourceUrl: string): Promise<ScraperResult> {
    // Route to the tecmobowl scraper for tecmobowl.org
    if (scraperName === 'tecmobowl' || sourceUrl.includes('tecmobowl.org')) {
      return runTecmobowlScraper(this.db, this.sourceAccountId, sourceUrl);
    }

    // Generic web scraper fallback for other community sites
    return this.runGenericWebScraper(scraperName, sourceUrl);
  }

  /**
   * Generic web scraper fallback for community sites without a dedicated scraper.
   * Extracts ROM download links from HTML pages.
   */
  private async runGenericWebScraper(scraperName: string, sourceUrl: string): Promise<ScraperResult> {
    const result: ScraperResult = {
      roms_found: 0,
      roms_added: 0,
      roms_updated: 0,
      roms_removed: 0,
      errors: [],
      duration_seconds: 0,
    };

    const startTime = Date.now();

    try {
      const { load } = await import('cheerio');
      const axios_ = (await import('axios')).default;

      logger.info(`Generic web scraper "${scraperName}" fetching ${sourceUrl}`);

      const response = await axios_.get(sourceUrl, {
        timeout: 30000,
        headers: {
          'User-Agent': 'nself-rom-discovery/1.0 (metadata indexer)',
        },
      });

      const $ = load(response.data as string);
      const links: Array<{ title: string; url: string }> = [];

      $('a[href]').each((_index, element) => {
        const href = $(element).attr('href') ?? '';
        const text = $(element).text().trim();
        const romExtensions = ['.nes', '.smc', '.sfc', '.gba', '.md', '.gen', '.z64', '.n64', '.gb', '.gbc', '.zip', '.7z'];
        const isRomLink = romExtensions.some(ext => href.toLowerCase().endsWith(ext));

        if (isRomLink && text.length > 0) {
          const fullUrl = href.startsWith('http') ? href : new URL(href, sourceUrl).toString();
          links.push({ title: text, url: fullUrl });
        }
      });

      result.roms_found = links.length;
      logger.info(`Found ${links.length} ROM links on ${sourceUrl}`);

      for (const link of links) {
        try {
          const pseudoHash = generateWebScraperHash(scraperName, link.url);

          const romData = {
            source_account_id: this.sourceAccountId,
            rom_title: link.title,
            rom_title_normalized: link.title.toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim(),
            platform: guessPlatformFromUrl(link.url),
            region: null,
            file_name: link.url.split('/').pop() ?? link.title,
            file_size_bytes: null,
            file_hash_md5: null,
            file_hash_sha256: pseudoHash,
            file_hash_crc32: null,
            download_url: link.url,
            download_source: scraperName,
            download_url_verified_at: new Date(),
            download_url_dead: false,
            release_year: null,
            release_month: null,
            release_day: null,
            version: null,
            quality_score: 35,
            popularity_score: 0,
            release_group: 'Community',
            is_verified_dump: false,
            is_hack: false,
            is_translation: false,
            is_homebrew: false,
            is_public_domain: false,
            game_title: link.title,
            genre: null,
            publisher: null,
            developer: null,
            description: null,
            igdb_id: null,
            mobygames_id: null,
            box_art_url: null,
            screenshot_urls: [] as string[],
            is_community_rom: true,
            community_source_url: sourceUrl,
            community_update_year: new Date().getFullYear(),
            scraped_from: scraperName,
            scraped_at: new Date(),
          };

          await this.db.upsertRomMetadata(romData);
          result.roms_added++;
        } catch (upsertError) {
          const msg = upsertError instanceof Error ? upsertError.message : 'Unknown error';
          result.errors.push(`Failed to upsert from ${scraperName}: ${msg}`);
        }
      }
    } catch (fetchError) {
      const msg = fetchError instanceof Error ? fetchError.message : 'Unknown error';
      result.errors.push(`Web scraper "${scraperName}" failed: ${msg}`);
      logger.error(`Web scraper failed`, { scraper: scraperName, error: msg });
    }

    result.duration_seconds = Math.round((Date.now() - startTime) / 1000);
    return result;
  }

  isRunning(): boolean {
    return this.running;
  }

  getScheduledScrapers(): string[] {
    return this.tasks.map(t => t.scraperName);
  }
}

/**
 * Generate a deterministic SHA-256 hash for web-scraped ROM entries
 */
function generateWebScraperHash(scraperName: string, url: string): string {
  return createHash('sha256').update(`${scraperName}:${url}`).digest('hex');
}

/**
 * Guess the platform from a URL path or filename
 */
function guessPlatformFromUrl(url: string): string {
  const lower = url.toLowerCase();

  const platformPatterns: Record<string, string[]> = {
    nes: ['.nes', 'nintendo-entertainment', 'nes-rom'],
    snes: ['.smc', '.sfc', 'super-nintendo', 'snes-rom'],
    gba: ['.gba', 'game-boy-advance', 'gba-rom'],
    genesis: ['.md', '.gen', 'genesis', 'mega-drive', 'megadrive'],
    n64: ['.z64', '.n64', '.v64', 'nintendo-64', 'n64-rom'],
    gb: ['.gb', 'game-boy'],
    gbc: ['.gbc', 'game-boy-color'],
    ps1: ['.bin', '.cue', 'playstation', 'psx'],
  };

  for (const [platform, patterns] of Object.entries(platformPatterns)) {
    if (patterns.some(p => lower.includes(p))) {
      return platform;
    }
  }

  return 'unknown';
}
