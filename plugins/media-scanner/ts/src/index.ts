/**
 * Media Scanner Plugin for nself
 * Library scanning, filename parsing, FFprobe analysis, TMDB matching, and MeiliSearch indexing
 */

export { MediaScannerDatabase } from './database.js';
export { scanDirectories, countMediaFiles } from './scanner.js';
export { parseFilename, normalizeTitle } from './parser.js';
export { probeFile, checkFFprobeAvailable } from './probe.js';
export { TmdbMatcher, levenshteinDistance, computeSimilarity, AUTO_ACCEPT_THRESHOLD, SUGGEST_THRESHOLD } from './matcher.js';
export { MediaSearchService } from './search.js';
export { createServer } from './server.js';
export { loadConfig } from './config.js';
export * from './types.js';
