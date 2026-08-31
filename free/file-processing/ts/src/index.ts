/**
 * File Processing Plugin - Main Entry Point
 */

export * from './types.js';
export * from './config.js';
export { createStorageAdapter } from './storage.js';
export { FileProcessor } from './processor.js';
export { Database } from './database.js';
export { getWebhookInfo } from './webhooks.js';
export { getSyncInfo } from './sync.js';
export { generatePosters, generateSpriteSheet, optimizeImage } from './image-processor.js';
