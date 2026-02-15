/**
 * Admin API Plugin for nself
 * Admin API service providing aggregated metrics, system health, session counts,
 * storage breakdown, and real-time dashboard endpoints
 */

export * from './types.js';
export { loadConfig } from './config.js';
export { AdminApiDatabase } from './database.js';
export { createServer, startServer } from './server.js';
