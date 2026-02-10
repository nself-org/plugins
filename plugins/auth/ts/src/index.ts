/**
 * Auth Plugin
 * Main entry point and exports
 */

export * from './types.js';
export * from './config.js';
export * from './database.js';
export * from './server.js';
export { config } from './config.js';
export { createAuthDatabase, AuthDatabase } from './database.js';
export { createAuthServer, AuthServer } from './server.js';
