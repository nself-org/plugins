/**
 * TMDB Plugin
 * Media metadata enrichment from TMDB/IMDb with auto-matching and manual review queue
 */

export * from './types.js';
export * from './config.js';
export * from './database.js';
export { fastify, start } from './server.js';
