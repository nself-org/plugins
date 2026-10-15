/**
 * Purpose: keep disposable envelopes and counters off the daily KV write budget.
 * Inputs: internal cache keys and JSON values. Outputs: best-effort local cache.
 * Constraints: each data center has its own cache; eviction is a normal miss.
 */
import type { KVEnvelope } from "./registry-types.ts";

function cacheRequest(key: string): Request {
  return new Request(`https://plugins.nself.org/__worker-cache/v1/${encodeURIComponent(key)}`);
}

export async function edgeGet<T>(key: string): Promise<T | null> {
  try {
    const response = await caches.default.match(cacheRequest(key));
    return response ? await response.json<T>() : null;
  } catch {
    return null;
  }
}

export async function edgePut<T>(key: string, value: T, ttlSeconds = 86400): Promise<void> {
  try {
    await caches.default.put(cacheRequest(key), new Response(JSON.stringify(value), {
      headers: {
        "Content-Type": "application/json",
        "Cache-Control": `public, max-age=${ttlSeconds}`,
      },
    }));
  } catch {
    // Cache availability must not break registry reads or write durable KV.
  }
}

export async function edgeDelete(key: string): Promise<void> {
  try {
    await caches.default.delete(cacheRequest(key));
  } catch {
    // Best effort; stale data remains bounded by the envelope freshness check.
  }
}

export function edgeGetWrapped<T>(key: string): Promise<KVEnvelope<T> | null> {
  return edgeGet<KVEnvelope<T>>(key);
}

export function edgePutWrapped<T>(key: string, data: T, timestamp = Date.now()): Promise<void> {
  // Keep the physical entry beyond its logical TTL for catalog stale fallback.
  return edgePut(key, { data, timestamp });
}
