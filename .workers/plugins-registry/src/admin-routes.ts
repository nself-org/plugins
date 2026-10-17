import { jsonResponse } from "./http.ts";
import { fetchFreeRegistry, fetchProRegistry, kvGet, KV_BUNDLES_JSON, KV_BUNDLES_SCHEMA } from "./registry.ts";
import type { Env } from "./registry.ts";
import { fetchCatalog } from "./catalog.ts";
import { edgeDelete } from "./edge-cache.ts";
import { signingConfigured } from "./sign.ts";

/** Ed25519 public key hex: PUBLIC_KEY_HEX, or the retired index.js name SIGNING_PUBLIC_KEY. */
function publicKeyHex(env: Env): string | undefined {
  return env.PUBLIC_KEY_HEX ?? env.SIGNING_PUBLIC_KEY;
}

export async function handleHealth(env: Env): Promise<Response> {
  return jsonResponse({
    // "healthy" is what publish.yml and deploy.sh check after a deploy.
    status: "healthy",
    service: "nself-plugin-registry",
    version: env.PLUGIN_REGISTRY_VERSION ?? env.REGISTRY_VERSION ?? "1.0.0",
    ts: new Date().toISOString(),
    // `timestamp` and `features` are the retired index.js health fields.
    timestamp: new Date().toISOString(),
    features: [
      "dual-registry",
      "edge-cache",
      "tier-filter",
      "github-api-fetch",
      "plugin-list",
      "tarball-redirect",
      "ed25519-signatures",
      "revocations",
      "catalog-etag",
    ],
    signing: {
      configured: await signingConfigured(env.SIGNING_PRIVATE_KEY, publicKeyHex(env)),
    },
  });
}

// ---------------------------------------------------------------------------
const KV_STATS = "stats:global";

interface StatsRecord {
  registryHits?: number;
  registryFetches?: number;
  lastSync?: string | null;
}

export async function handleStats(env: Env): Promise<Response> {
  const kv = env.REGISTRY ?? env.PLUGINS_KV;
  let stats: StatsRecord = { registryHits: 0, registryFetches: 0, lastSync: null };

  if (kv) {
    const raw = await kvGet<StatsRecord>(kv, KV_STATS);
    if (raw) {
      stats = raw.data ?? (raw as unknown as StatsRecord);
    }
  }

  return jsonResponse(stats);
}

// ---------------------------------------------------------------------------
// POST /api/sync — bust edge cache and fetch fresh from GitHub
// ---------------------------------------------------------------------------

export async function handleSync(
  request: Request,
  env: Env,
  ctx: ExecutionContext,
): Promise<Response> {
  const authHeader = request.headers.get("Authorization") ?? "";
  if (!authHeader.startsWith("Bearer ")) {
    return jsonResponse({ error: "unauthorized" }, 401);
  }
  const token = authHeader.slice(7);
  if (!env.GITHUB_SYNC_TOKEN || token !== env.GITHUB_SYNC_TOKEN) {
    return jsonResponse({ error: "invalid token" }, 403);
  }

  const kv = env.REGISTRY ?? env.PLUGINS_KV;

  await Promise.all([
    "registry:free",
    "registry:pro",
    "registry:combined",
    "registry:manifest",
    KV_BUNDLES_JSON,
    KV_BUNDLES_SCHEMA,
    "marketplace:enriched",
  ].map(edgeDelete));

  const [freeResult, proResult, catalogResult] = await Promise.allSettled([
    fetchFreeRegistry(env, ctx, true),
    fetchProRegistry(env, ctx, true),
    fetchCatalog(env, ctx, true),
  ]);

  const freeCount = freeResult.status === "fulfilled" ? freeResult.value.length : 0;
  const proCount =
    proResult.status === "fulfilled" ? (proResult.value?.length ?? 0) : 0;
  const freeError =
    freeResult.status === "rejected" ? (freeResult.reason as Error).message : null;
  const proError =
    proResult.status === "rejected" ? (proResult.reason as Error).message : null;

  if (kv) {
    ctx.waitUntil(
      (async () => {
        const existing = await kvGet<StatsRecord>(kv, KV_STATS);
        const s: StatsRecord = existing
          ? (existing.data ?? (existing as unknown as StatsRecord))
          : { registryHits: 0, registryFetches: 0 };
        s.lastSync = new Date().toISOString();
        await kv.put(KV_STATS, JSON.stringify(s));
      })(),
    );
  }

  return jsonResponse({
    success: true,
    message: "registry caches refreshed",
    freeCount,
    proCount,
    totalCount: freeCount + proCount,
    freeError,
    proError,
    catalogSynced: catalogResult.status === "fulfilled" && catalogResult.value?.cache === "MISS",
    timestamp: new Date().toISOString(),
  });
}
