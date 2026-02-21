/**
 * Security Middleware and Utilities
 * Authentication, rate limiting, and security helpers for nself plugins
 */

import { createLogger } from './logger.js';

const logger = createLogger('security');

/**
 * Simple in-memory rate limiter
 * For production, consider using Redis-backed rate limiting
 */
export class ApiRateLimiter {
  private requests: Map<string, { count: number; resetAt: number }> = new Map();
  private readonly maxRequests: number;
  private readonly windowMs: number;

  constructor(maxRequests = 100, windowMs = 60000) {
    this.maxRequests = maxRequests;
    this.windowMs = windowMs;

    // Cleanup old entries every minute
    setInterval(() => this.cleanup(), 60000);
  }

  /**
   * Check if a request should be allowed
   * @returns true if allowed, false if rate limited
   */
  check(key: string): boolean {
    const now = Date.now();
    const record = this.requests.get(key);

    if (!record || now > record.resetAt) {
      this.requests.set(key, { count: 1, resetAt: now + this.windowMs });
      return true;
    }

    if (record.count >= this.maxRequests) {
      return false;
    }

    record.count++;
    return true;
  }

  /**
   * Get remaining requests for a key
   */
  getRemaining(key: string): number {
    const record = this.requests.get(key);
    if (!record || Date.now() > record.resetAt) {
      return this.maxRequests;
    }
    return Math.max(0, this.maxRequests - record.count);
  }

  /**
   * Get reset time for a key
   */
  getResetTime(key: string): number {
    const record = this.requests.get(key);
    if (!record || Date.now() > record.resetAt) {
      return Date.now() + this.windowMs;
    }
    return record.resetAt;
  }

  private cleanup(): void {
    const now = Date.now();
    for (const [key, record] of this.requests) {
      if (now > record.resetAt) {
        this.requests.delete(key);
      }
    }
  }
}

/**
 * API Key authentication result
 */
export interface AuthResult {
  authenticated: boolean;
  error?: string;
}

/**
 * Validate an API key
 * @param providedKey - The key from the request
 * @param validKey - The configured valid key
 * @returns Authentication result
 */
export function validateApiKey(providedKey: string | undefined, validKey: string | undefined): AuthResult {
  // If no API key is configured, allow all requests (dev mode)
  if (!validKey) {
    return { authenticated: true };
  }

  if (!providedKey) {
    return { authenticated: false, error: 'API key required' };
  }

  // Constant-time comparison to prevent timing attacks
  if (providedKey.length !== validKey.length) {
    return { authenticated: false, error: 'Invalid API key' };
  }

  let result = 0;
  for (let i = 0; i < providedKey.length; i++) {
    result |= providedKey.charCodeAt(i) ^ validKey.charCodeAt(i);
  }

  if (result !== 0) {
    return { authenticated: false, error: 'Invalid API key' };
  }

  return { authenticated: true };
}

/**
 * Extract API key from request headers
 * Supports: Authorization: Bearer <key>, X-API-Key: <key>
 */
export function extractApiKey(headers: Record<string, string | string[] | undefined>): string | undefined {
  // Check Authorization header first
  const authHeader = headers['authorization'] || headers['Authorization'];
  if (authHeader) {
    const auth = Array.isArray(authHeader) ? authHeader[0] : authHeader;
    if (auth.startsWith('Bearer ')) {
      return auth.slice(7);
    }
  }

  // Check X-API-Key header
  const apiKeyHeader = headers['x-api-key'] || headers['X-API-Key'];
  if (apiKeyHeader) {
    return Array.isArray(apiKeyHeader) ? apiKeyHeader[0] : apiKeyHeader;
  }

  return undefined;
}

/**
 * Check if running in production mode
 */
export function isProduction(): boolean {
  return process.env.NODE_ENV === 'production';
}

/**
 * Validate that webhook secret is configured in production
 * @throws Error if secret is missing in production
 */
export function requireWebhookSecret(secret: string | undefined, serviceName: string): void {
  if (isProduction() && !secret) {
    throw new Error(
      `${serviceName.toUpperCase()}_WEBHOOK_SECRET is required in production. ` +
      `Set NODE_ENV to something other than 'production' to bypass this check.`
    );
  }

  if (!secret) {
    logger.warn(
      `${serviceName} webhook secret not configured. ` +
      `Webhook signature verification is disabled. ` +
      `This is a security risk in production.`
    );
  }
}

/**
 * Security configuration for plugins
 */
export interface SecurityConfig {
  /** API key for authenticating requests (optional - if not set, no auth required) */
  apiKey?: string;
  /** Rate limit: max requests per window */
  rateLimitMax?: number;
  /** Rate limit: window in milliseconds */
  rateLimitWindowMs?: number;
  /** Whether to require webhook secret */
  requireWebhookSecret?: boolean;
}

/**
 * Load security configuration from environment variables
 */
export function loadSecurityConfig(prefix: string): SecurityConfig {
  const envPrefix = prefix.toUpperCase();
  return {
    apiKey: process.env[`${envPrefix}_API_KEY`] || process.env.NSELF_API_KEY,
    rateLimitMax: parseInt(process.env[`${envPrefix}_RATE_LIMIT_MAX`] || process.env.RATE_LIMIT_MAX || '100', 10),
    rateLimitWindowMs: parseInt(process.env[`${envPrefix}_RATE_LIMIT_WINDOW_MS`] || process.env.RATE_LIMIT_WINDOW_MS || '60000', 10),
    requireWebhookSecret: isProduction(),
  };
}

/**
 * Create Fastify authentication hook
 * Usage: app.addHook('preHandler', createAuthHook(config.apiKey))
 */
export function createAuthHook(apiKey: string | undefined) {
  return async (request: { headers: Record<string, string | string[] | undefined> }, reply: { status: (code: number) => { send: (body: unknown) => void } }) => {
    // Skip auth for health check endpoints
    const url = (request as unknown as { url: string }).url;
    if (url === '/health' || url === '/ready' || url === '/live') {
      return;
    }

    const providedKey = extractApiKey(request.headers);
    const result = validateApiKey(providedKey, apiKey);

    if (!result.authenticated) {
      logger.warn('Authentication failed', { error: result.error });
      return reply.status(401).send({ error: result.error });
    }
  };
}

/**
 * Create Fastify rate limiting hook
 * Usage: app.addHook('preHandler', createRateLimitHook(limiter))
 */
export function createRateLimitHook(limiter: ApiRateLimiter) {
  return async (
    request: { ip: string; headers: Record<string, string | string[] | undefined> },
    reply: {
      status: (code: number) => { send: (body: unknown) => void };
      header: (name: string, value: string) => void;
    }
  ) => {
    // Use IP address as rate limit key
    const key = request.ip || 'unknown';

    // Add rate limit headers
    reply.header('X-RateLimit-Limit', limiter['maxRequests'].toString());
    reply.header('X-RateLimit-Remaining', limiter.getRemaining(key).toString());
    reply.header('X-RateLimit-Reset', Math.ceil(limiter.getResetTime(key) / 1000).toString());

    if (!limiter.check(key)) {
      logger.warn('Rate limit exceeded', { ip: key });
      reply.header('Retry-After', '60');
      return reply.status(429).send({ error: 'Too many requests' });
    }
  };
}
