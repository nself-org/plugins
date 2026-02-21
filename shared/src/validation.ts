/**
 * Input Validation Utilities
 * Shared validation helpers for all plugins
 */

/**
 * Validates and clamps pagination parameters
 */
export function validatePagination(
  limit?: number | string,
  offset?: number | string,
  maxLimit = 1000
): { limit: number; offset: number } {
  let parsedLimit = typeof limit === 'string' ? parseInt(limit, 10) : (limit ?? 100);
  let parsedOffset = typeof offset === 'string' ? parseInt(offset, 10) : (offset ?? 0);

  // Handle NaN
  if (isNaN(parsedLimit)) {
    parsedLimit = 100;
  }
  if (isNaN(parsedOffset)) {
    parsedOffset = 0;
  }

  // Clamp values
  return {
    limit: Math.min(Math.max(parsedLimit, 1), maxLimit),
    offset: Math.max(parsedOffset, 0),
  };
}

/**
 * Validates a port number
 */
export function validatePort(value: string | number | undefined, defaultPort: number): number {
  const port = typeof value === 'string' ? parseInt(value, 10) : (value ?? defaultPort);
  if (isNaN(port) || port < 1 || port > 65535) {
    return defaultPort;
  }
  return port;
}

/**
 * Validates a positive integer
 */
export function validatePositiveInt(
  value: string | number | undefined,
  defaultValue: number
): number {
  const num = typeof value === 'string' ? parseInt(value, 10) : (value ?? defaultValue);
  if (isNaN(num) || num < 1) {
    return defaultValue;
  }
  return num;
}

/**
 * Validates a string against an allowed list
 */
export function validateEnum<T extends string>(
  value: string | undefined,
  allowed: readonly T[],
  defaultValue?: T
): T | undefined {
  if (!value) {
    return defaultValue;
  }
  if (allowed.includes(value as T)) {
    return value as T;
  }
  return defaultValue;
}

/**
 * Validates an API key format
 */
export function validateApiKeyFormat(key: string, prefixes: string[]): boolean {
  return prefixes.some((prefix) => key.startsWith(prefix));
}

/**
 * Validates a database URL format
 */
export function validateDatabaseUrl(url: string): boolean {
  if (!url) {
    return false;
  }
  try {
    // Replace postgresql:// with https:// for URL parsing
    const normalized = url.replace(/^postgres(ql)?:\/\//, 'https://');
    new URL(normalized);
    return true;
  } catch {
    return false;
  }
}

/**
 * Validates integer ID from path parameter
 */
export function validateId(id: string | number): number | null {
  const parsed = typeof id === 'string' ? parseInt(id, 10) : id;
  if (isNaN(parsed) || parsed <= 0) {
    return null;
  }
  return parsed;
}

/**
 * Common Stripe subscription statuses
 */
export const STRIPE_SUBSCRIPTION_STATUSES = [
  'active',
  'canceled',
  'incomplete',
  'incomplete_expired',
  'past_due',
  'paused',
  'trialing',
  'unpaid',
] as const;

/**
 * Common Stripe invoice statuses
 */
export const STRIPE_INVOICE_STATUSES = [
  'draft',
  'open',
  'paid',
  'uncollectible',
  'void',
] as const;

/**
 * Common GitHub issue/PR states
 */
export const GITHUB_STATES = ['open', 'closed', 'all'] as const;

/**
 * Common Shopify order financial statuses
 */
export const SHOPIFY_FINANCIAL_STATUSES = [
  'pending',
  'authorized',
  'partially_paid',
  'paid',
  'partially_refunded',
  'refunded',
  'voided',
] as const;
