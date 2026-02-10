/**
 * nself Plugin Utilities
 * Shared utilities for building nself plugins
 */

export * from './types.js';
export * from './logger.js';
export * from './database.js';
export * from './webhook.js';
export * from './http.js';
export * from './validation.js';
export * from './security.js';
export * from './app-context.js';

// Re-export commonly used items at top level
export { createLogger, Logger } from './logger.js';
export { createDatabase, Database } from './database.js';
export { HttpClient, HttpError, RateLimiter } from './http.js';
export {
  verifyHmacSignature,
  verifyStripeSignature,
  verifyGitHubSignature,
  verifyShopifySignature,
  createWebhookRoute,
  WebhookProcessor,
  withRetry,
} from './webhook.js';
export {
  validatePagination,
  validatePort,
  validatePositiveInt,
  validateEnum,
  validateApiKeyFormat,
  validateDatabaseUrl,
  validateId,
} from './validation.js';
export {
  ApiRateLimiter,
  validateApiKey,
  extractApiKey,
  isProduction,
  requireWebhookSecret,
  loadSecurityConfig,
  createAuthHook,
  createRateLimitHook,
} from './security.js';
export {
  normalizeSourceAccountId,
  getAppContext,
  registerAppContext,
  parseCsvList,
  buildAccountConfigs,
} from './app-context.js';
