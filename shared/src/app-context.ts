/**
 * Multi-app context utilities for nself plugins
 *
 * Provides standardized app context resolution, source account normalization,
 * and CSV multi-account config parsing used across all plugins.
 */

import type { FastifyRequest, FastifyInstance } from 'fastify';
import { createLogger } from './logger.js';

const logger = createLogger('app-context');

// =========================================================================
// Types
// =========================================================================

export interface AppContext {
  sourceAccountId: string;
}

export interface AccountConfig {
  /** Normalized account identifier (used as source_account_id in DB) */
  id: string;
  /** Human-readable label */
  label?: string;
}

export interface MultiAppConfig {
  /** Default source account ID for single-app mode */
  defaultAccountId: string;
  /** Configured accounts (empty = single-app mode with defaultAccountId) */
  accounts: AccountConfig[];
}

// =========================================================================
// Normalization
// =========================================================================

/**
 * Normalize a source account ID to a safe, consistent format.
 * - Lowercases the value
 * - Strips characters except a-z, 0-9, hyphens, underscores
 * - Trims leading/trailing hyphens/underscores
 * - Returns 'primary' for empty or invalid input
 */
export function normalizeSourceAccountId(value: string | undefined | null): string {
  if (!value || typeof value !== 'string') {
    return 'primary';
  }

  const normalized = value
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/^[-_]+|[-_]+$/g, '');

  return normalized.length > 0 ? normalized : 'primary';
}

// =========================================================================
// App Context Resolution
// =========================================================================

/**
 * Resolve the app context from an incoming HTTP request.
 *
 * Resolution priority:
 * 1. X-App-Name header (set by nginx per-app routes)
 * 2. `app` query parameter
 * 3. Falls back to 'primary'
 */
export function getAppContext(request: FastifyRequest): AppContext {
  const headerValue = request.headers['x-app-name'];
  const header = Array.isArray(headerValue) ? headerValue[0] : headerValue;

  if (header) {
    return { sourceAccountId: normalizeSourceAccountId(header) };
  }

  const query = request.query as Record<string, string | undefined>;
  if (query?.app) {
    return { sourceAccountId: normalizeSourceAccountId(query.app) };
  }

  return { sourceAccountId: 'primary' };
}

/**
 * Create a Fastify onRequest hook that decorates each request with app context.
 *
 * Usage:
 * ```typescript
 * import { registerAppContext } from '@nself/plugin-utils';
 * registerAppContext(app);
 * // Then in route handlers:
 * const { sourceAccountId } = (request as any).appContext;
 * ```
 */
export function registerAppContext(app: FastifyInstance): void {
  app.decorateRequest('appContext', null);
  app.addHook('onRequest', async (request) => {
    (request as unknown as Record<string, unknown>).appContext = getAppContext(request);
  });
}

// =========================================================================
// CSV Multi-Account Parsing
// =========================================================================

/**
 * Parse a comma-separated string into a trimmed, non-empty list.
 */
export function parseCsvList(value: string | undefined): string[] {
  if (!value) return [];
  return value.split(',').map(item => item.trim()).filter(Boolean);
}

/**
 * Build account configs from CSV environment variables.
 *
 * @param keys - Comma-separated primary values (API keys, tokens, etc.)
 * @param labels - Optional comma-separated account labels (must match keys length)
 * @param fallbackSingleKey - Single-key env var value for backward compat
 * @param fallbackSingleLabel - Single-label env var value for backward compat
 * @returns Array of AccountConfig with normalized IDs
 *
 * Usage:
 * ```typescript
 * const accounts = buildAccountConfigs(
 *   process.env.STRIPE_API_KEYS,
 *   process.env.STRIPE_ACCOUNT_LABELS,
 *   process.env.STRIPE_API_KEY,
 *   process.env.STRIPE_ACCOUNT_ID,
 * );
 * ```
 */
export function buildAccountConfigs(
  keys: string | undefined,
  labels: string | undefined,
  _fallbackSingleKey?: string,
  fallbackSingleLabel?: string,
): AccountConfig[] {
  const multiKeys = parseCsvList(keys);
  const multiLabels = parseCsvList(labels);

  if (multiLabels.length > 0 && multiLabels.length !== multiKeys.length) {
    throw new Error('Account labels count must match keys count');
  }

  if (multiKeys.length > 0) {
    return multiKeys.map((_, index) => {
      const label = multiLabels[index] ?? `account-${index + 1}`;
      return {
        id: normalizeSourceAccountId(label),
        label,
      };
    });
  }

  // Single-key fallback
  const label = fallbackSingleLabel ?? 'primary';
  return [{
    id: normalizeSourceAccountId(label),
    label,
  }];
}

logger.debug('App context module loaded');
