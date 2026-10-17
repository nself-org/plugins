import { edgeGet, edgePut } from "./edge-cache.ts";
import type { MarketplaceEnv } from "./marketplace-types.ts";

const KV_GET_RL_PREFIX = "rl:get:";
const DEFAULT_GET_RATE_LIMIT = 60;

// Cache API counters are best-effort and local to each Cloudflare data center.
// Read/increment/write is non-atomic across concurrent requests and isolates.

export async function checkGetRateLimit(
  request: Request,
  env: MarketplaceEnv,
): Promise<{ limited: boolean; retryAfter: number }> {
  const ip = getConnectingIP(request);
  return isGetRateLimited(env, ip);
}

export function getConnectingIP(request: Request): string {
  // CF-Connecting-IP is set by Cloudflare and cannot be spoofed from outside.
  // Never trust X-Forwarded-For — it is trivially spoofable.
  return request.headers.get("CF-Connecting-IP") ?? "unknown";
}

/** Rate-limit key: ip:<ip>:<windowKey> (POST ratings) */
function ipRateLimitKey(ip: string, windowMinute: string): string {
  return `rl:ip:${ip}:${windowMinute}`;
}

/** Rate-limit key for GET marketplace endpoints: rl:get:<ip>:<windowMinute> */
function getIpRateLimitKey(ip: string, windowMinute: string): string {
  return `${KV_GET_RL_PREFIX}${ip}:${windowMinute}`;
}

/**
 * T-RATE-01: Check + increment IP rate limit for marketplace GET endpoints.
 * Limit: MARKETPLACE_GET_RATE_LIMIT (default 60) requests per 60s window per IP.
 * Returns { limited: true, retryAfter: N } when limit exceeded.
 * Uses the edge cache with 90s TTL to cover current + next minute window overlap.
 */
async function isGetRateLimited(
  env: MarketplaceEnv,
  ip: string,
): Promise<{ limited: boolean; retryAfter: number }> {
  const maxRequests = parseInt(env.MARKETPLACE_GET_RATE_LIMIT ?? String(DEFAULT_GET_RATE_LIMIT), 10);
  const window = currentMinuteWindow();
  const key = getIpRateLimitKey(ip, window);

  let count = 0;
  try {
    const raw = await edgeGet<{ count: number }>(key);
    count = raw?.count ?? 0;
  } catch {
    return { limited: false, retryAfter: 60 }; // Cache read failure — allow through
  }

  if (count >= maxRequests) {
    const retryAfter = Math.max(1, 60 - new Date().getSeconds());
    return { limited: true, retryAfter };
  }

  // Increment counter with 90s TTL (covers window overlap)
  try {
    await edgePut(key, { count: count + 1 }, 90);
  } catch {
    // Best-effort counter increment — not fatal
  }

  return { limited: false, retryAfter: 0 };
}

/** Current minute window (YYYY-MM-DDTHH:MM) */
function currentMinuteWindow(): string {
  return new Date().toISOString().slice(0, 16); // "2026-04-20T14:23"
}

/**
 * Check + increment IP rate limit (5 POSTs per IP per minute for rating endpoint).
 * Returns true if the request should be rejected (limit exceeded).
 * Uses an edge-cache counter with 90s TTL (covers current + next minute window).
 */
export async function isRateLimited(env: MarketplaceEnv, ip: string): Promise<{ limited: boolean; retryAfter: number }> {
  const window = currentMinuteWindow();
  const key = ipRateLimitKey(ip, window);

  let count = 0;
  try {
    const raw = await edgeGet<{ count: number }>(key);
    count = raw?.count ?? 0;
  } catch {
    return { limited: false, retryAfter: 60 }; // Cache read failure — allow through
  }

  if (count >= 5) {
    // Calculate seconds until next minute window
    const retryAfter = Math.max(1, 60 - new Date().getSeconds());
    return { limited: true, retryAfter };
  }

  // Increment counter with 90s TTL
  try {
    await edgePut(key, { count: count + 1 }, 90);
  } catch {
    // Best-effort counter increment — not fatal
  }

  return { limited: false, retryAfter: 0 };
}
