/**
 * Search Plugin for nself
 * Full-text search with PostgreSQL FTS and MeiliSearch support
 */

export { SearchDatabase } from './database.js';
export { createServer, startServer } from './server.js';
export { loadConfig } from './config.js';
export * from './types.js';
