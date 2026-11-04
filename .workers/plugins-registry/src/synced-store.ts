/**
 * synced-store.ts: durable KV copies of the upstream registries (D-1543).
 *
 * Read order for registry:free, registry:pro and catalog:json:
 *   1. Cache API (this data center), fresh for at most CACHE_TTL.
 *   2. KV copy written by POST /api/sync (expires after SYNC_KV_TTL_SECONDS).
 *   3. GitHub. The result fills the Cache API only, never KV.
 *
 * KV is written only by the authenticated sync, and only when the content hash
 * differs from the stored one, so a repeat sync of identical content writes
 * nothing. No public request path writes or deletes KV here.
 */
import { edgeGetWrapped, edgePutWrapped } from "./edge-cache.ts";
import { cacheTtl, isFresh } from "./registry-types.ts";
import type { Env } from "./registry-types.ts";

/** How long a synced copy lives in KV without a newer sync (hours, per D-1543). */
export const SYNC_KV_TTL_SECONDS = 12 * 60 * 60;

/**
 * KV reads may be served from a per-location cache up to ~60s old. A Cache API
 * fill from KV is back-dated by this much so a sync still goes live everywhere
 * within CACHE_TTL, as it did when every colo refetched GitHub.
 */
const KV_READ_LAG_SECONDS = 60;

export interface SyncedEnvelope<T> {
  data: T;
  /** Epoch ms of the sync that wrote this copy. */
  timestamp: number;
  /** sha256 hex of JSON.stringify(data); a copy without one is ignored. */
  hash: string;
}

export function syncKv(env: Env): KVNamespace | undefined {
  return env.REGISTRY ?? env.PLUGINS_KV;
}

export async function sha256Hex(text: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

/** The synced KV copy, or null when absent, unreadable or written by older code (no hash). */
export async function readSynced<T>(env: Env, key: string): Promise<SyncedEnvelope<T> | null> {
  const kv = syncKv(env);
  if (!kv) return null;
  try {
    const raw = await kv.get<SyncedEnvelope<T>>(key, "json");
    if (!raw || typeof raw.hash !== "string" || typeof raw.timestamp !== "number" || raw.data == null) {
      return null;
    }
    return raw;
  } catch {
    return null;
  }
}

/**
 * Store `data` as the synced copy of `key` unless the stored hash already matches.
 * Returns "written", "unchanged" or "failed". Called only from POST /api/sync.
 */
export async function writeSynced<T>(
  env: Env,
  key: string,
  data: T,
  hash?: string,
): Promise<"written" | "unchanged" | "failed"> {
  const kv = syncKv(env);
  if (!kv) return "failed";
  try {
    const digest = hash ?? (await sha256Hex(JSON.stringify(data)));
    const existing = await readSynced<T>(env, key);
    if (existing?.hash === digest) return "unchanged";
    const envelope: SyncedEnvelope<T> = { data, timestamp: Date.now(), hash: digest };
    await kv.put(key, JSON.stringify(envelope), { expirationTtl: SYNC_KV_TTL_SECONDS });
    return "written";
  } catch (e) {
    console.error(`sync KV write failed for ${key}:`, (e as Error).message);
    return "failed";
  }
}

/** Seconds a Cache API fill from KV is back-dated (its effective age when served). */
export function kvReadLagSeconds(env: Env): number {
  return Math.min(KV_READ_LAG_SECONDS, Math.floor(cacheTtl(env) / 2));
}

/** Fresh Cache API copy of `key`, or null. */
export async function readEdgeFresh<T>(env: Env, key: string): Promise<T | null> {
  const cached = await edgeGetWrapped<T>(key);
  return cached && cached.data != null && isFresh(cached, cacheTtl(env)) ? cached.data : null;
}

/**
 * Synced KV copy of `key`; on a hit, refill this colo's Cache API, back-dated so
 * the copy is refreshed from KV before a newer sync can be more than CACHE_TTL old.
 */
export async function readSyncedIntoEdge<T>(
  env: Env,
  ctx: ExecutionContext,
  key: string,
): Promise<T | null> {
  const synced = await readSynced<T>(env, key);
  if (!synced) return null;
  const lag = kvReadLagSeconds(env);
  ctx.waitUntil(edgePutWrapped(key, synced.data, Date.now() - lag * 1000));
  return synced.data;
}

/**
 * Shared registry loader: Cache API, then synced KV, then `upstream`.
 * With `sync` set (POST /api/sync only) both caches are skipped, and a
 * successful upstream result is also written to KV when its hash changed.
 */
export async function loadSynced<T>(
  env: Env,
  ctx: ExecutionContext,
  key: string,
  sync: boolean,
  upstream: () => Promise<T | null>,
): Promise<T | null> {
  if (!sync) {
    const edge = await readEdgeFresh<T>(env, key);
    if (edge !== null) return edge;
    const synced = await readSyncedIntoEdge<T>(env, ctx, key);
    if (synced !== null) return synced;
  }
  const data = await upstream();
  if (data === null) return null;
  ctx.waitUntil(edgePutWrapped(key, data));
  if (sync) await writeSynced(env, key, data);
  return data;
}
