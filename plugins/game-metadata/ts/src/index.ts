/**
 * Game Metadata Plugin for nself
 * Game metadata service with IGDB integration, ROM hash matching, tier requirements, and artwork management
 */

export * from './types.js';
export { loadConfig } from './config.js';
export { GameMetadataDatabase } from './database.js';
export { createServer, startServer } from './server.js';
