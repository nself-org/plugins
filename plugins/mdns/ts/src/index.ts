/**
 * mDNS Plugin for nself
 * mDNS/Bonjour service discovery for zero-config LAN advertising
 */

export * from './types.js';
export { loadConfig } from './config.js';
export { MdnsDatabase } from './database.js';
export { createServer, startServer } from './server.js';
