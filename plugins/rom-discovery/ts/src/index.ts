/**
 * ROM Discovery Plugin for nself
 * ROM metadata database, search, discovery, automated download orchestration,
 * and multi-source scraping for nself-tv
 */

export * from './types.js';
export { loadConfig } from './config.js';
export { RomDiscoveryDatabase } from './database.js';
export { createServer, startServer } from './server.js';
export { calculateQualityScore, calculatePopularityScore } from './scoring.js';
export { ScraperScheduler } from './scrapers/scraper-scheduler.js';
export { runArchiveOrgScraper, PLATFORM_MAPPINGS } from './scrapers/archive-org-scraper.js';
