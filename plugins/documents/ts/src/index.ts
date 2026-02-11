/**
 * Documents Plugin for nself
 * Document management and generation with templates, versioning, and sharing
 */

export * from './types.js';
export { loadConfig } from './config.js';
export { DocumentsDatabase } from './database.js';
export { createServer, startServer } from './server.js';
