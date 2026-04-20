/**
 * nself Plugin Registry — Cloudflare Worker (TypeScript)
 *
 * Serves the nself plugin registry at plugins.nself.org.
 * Combines the free (public) and pro (private) registries, adds Ed25519
 * tarball signatures, and maintains a revocation list for security.
 *
 * Endpoints:
 *   GET /health                          — Health check
 *   GET /plugins                         — All plugins (PluginListResponse)
 *   GET /plugins/:name                   — Single plugin detail
 *   GET /plugins/:name/tarball           — 302 redirect to GitHub tarball
 *   GET /plugins/:name/signature         — Ed25519 signature metadata
 *   GET /plugins/revocations             — Revocation list (polled hourly by CLI)
 *   GET /registry.json                   — Combined registry (legacy CLI compat)
 *   GET /registry                        — Alias for /registry.json
 *   GET /categories                      — Category list
 *   GET /manifest.json                   — Flat CLI manifest for nself plugin outdated
 *   GET /marketplace                     — Enriched marketplace view
 *   GET /stats                           — Cache statistics
 *   GET /.well-known/revoked-authors.json — Author CRL (S58-T09, polled daily by CLI)
 *   POST /api/sync                       — Force-refresh KV cache (GitHub Actions)
 *
 * KV keys:
 *   registry:free      — free registry (timestamp envelope)
 *   registry:pro       — pro registry  (timestamp envelope)
 *   registry:combined  — merged output (timestamp envelope)
 *   revocations:list   — JSON array of RevocationEntry
 *   revocations:authors — JSON array of RevokedAuthorEntry (S58-T09)
 *   stats:global       — request counters
 *
 * Secrets (set via `wrangler secret put`):
 *   SIGNING_PRIVATE_KEY  — Ed25519 seed, 32 bytes as 64 lowercase hex chars
 *   PUBLIC_KEY_HEX       — Ed25519 public key, 32 bytes as 64 lowercase hex chars
 *   GH_ACCESS_TOKEN      — Fine-grained PAT (contents:read on plugins + plugins-pro)
 *   GITHUB_SYNC_TOKEN    — Bearer token for POST /api/sync
 */

import { signPlugin, canonicalPluginString } from "./sign.ts";
import { handleRevocations, isRevoked } from "./revocations.ts";
import {
  fetchFreeRegistry,
  fetchProRegistry,
  fetchAllPlugins,
  cacheTtl,
  kvGet,
  kvPutWrapped,
  isFresh,
  DEFAULT_CACHE_TTL,
} from "./registry.ts";
import type {
  Env,
  PluginEntry,
  PluginListResponse,
  KVEnvelope,
  RevokedAuthorEntry,
  RevokedAuthorListResponse,
} from "./registry.ts";
import {
  handleMarketplace,
  handleGetRating,
  handlePostRating,
  handleInstallEvent,
  checkGetRateLimit,
} from "./marketplace.ts";
import type { MarketplaceEnv } from "./marketplace.ts";
import { resolveApiVersion, addApiVersionHeader } from "./middleware/api-version.ts";

// ---------------------------------------------------------------------------
// CORS helpers
// ---------------------------------------------------------------------------

function corsHeaders(): Record<string, string> {
  return {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type, Authorization",
  };
}

function jsonResponse(
  body: unknown,
  status = 200,
  extra: Record<string, string> = {},
): Response {
  return new Response(JSON.stringify(body, null, 2), {
    status,
    headers: {
      "Content-Type": "application/json",
      ...corsHeaders(),
      ...extra,
    },
  });
}

// ---------------------------------------------------------------------------
// Main fetch handler
// ---------------------------------------------------------------------------

export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    const url = new URL(request.url);
    const path = url.pathname.replace(/\/$/, "") || "/";
    const method = request.method.toUpperCase();

    if (method === "OPTIONS") {
      return new Response(null, { status: 204, headers: corsHeaders() });
    }

    // API versioning — X-API-Version header (S77-T04)
    // Missing header defaults to v1 forever (LTS commitment through 2027-04-17)
    const { version: apiVersion, errorResponse: versionError } = resolveApiVersion(request);
    if (versionError) {
      return versionError;
    }

    // URL versioning: /v1/* → same as /* (unversioned stays v1 forever)
    const effectivePath = path.startsWith("/v1/") ? path.slice(3) : path;

    // For response headers, we'll add X-API-Version via addApiVersionHeader when building responses.
    // The variable apiVersion is available throughout the handler.
    void apiVersion; // suppress unused-variable warning until used in a future version
    void addApiVersionHeader; // imported for use in future version routing

    // resolvedPath is the canonical path after stripping /v1 prefix
    const resolvedPath = effectivePath;

    try {
      // Route matching uses resolvedPath (strips /v1 prefix if present)
      // Health
      if (method === "GET" && resolvedPath === "/health") {
        return handleHealth(env);
      }

      // Registry (legacy compat)
      if (method === "GET" && (resolvedPath === "/registry.json" || resolvedPath === "/registry" || resolvedPath === "/")) {
        return handleRegistry(url, env, ctx);
      }

      // Plugin list — must appear before /plugins/:name to prevent shadowing
      // S67-T02: /registry/list is an alias with explicit 60s CF edge cache header
      if (method === "GET" && (resolvedPath === "/plugins" || resolvedPath === "/registry/list")) {
        return handlePluginList(url, env, ctx);
      }

      // Revocations — must appear before /plugins/:name to prevent "revocations" being
      // treated as a plugin name
      if (method === "GET" && resolvedPath === "/plugins/revocations") {
        return handleRevocations(env);
      }

      // S58-T09: Author CRL endpoint.
      // CLI checks this on every plugin install and via daily cron.
      // Append-only: once revoked, entries are never removed.
      if (method === "GET" && resolvedPath === "/.well-known/revoked-authors.json") {
        return handleRevokedAuthors(env);
      }

      // Single plugin (and sub-resources: /tarball, /signature, /:version)
      if (method === "GET" && resolvedPath.startsWith("/plugins/")) {
        return handlePlugin(resolvedPath, env, ctx);
      }

      // Categories
      if (method === "GET" && resolvedPath === "/categories") {
        return handleCategories(env, ctx);
      }

      // Manifest (CLI compat)
      if (method === "GET" && resolvedPath === "/manifest.json") {
        return handleManifest(env, ctx);
      }

      // Stats
      if (method === "GET" && resolvedPath === "/stats") {
        return handleStats(env);
      }

      // Marketplace — GET /marketplace/ratings/:name (must appear before /marketplace)
      if (method === "GET" && resolvedPath.startsWith("/marketplace/ratings/")) {
        // T-RATE-01: GET rate limit — 60 req/min per IP
        const rlGet = await checkGetRateLimit(request, env as MarketplaceEnv);
        if (rlGet.limited) {
          return jsonResponse(
            { error: `Rate limit exceeded. Retry after ${rlGet.retryAfter}s.` },
            429,
            { "Retry-After": String(rlGet.retryAfter) },
          );
        }
        const pluginName = resolvedPath.slice("/marketplace/ratings/".length);
        if (!pluginName) {
          return jsonResponse({ error: "plugin name required" }, 400);
        }
        const result = await handleGetRating(pluginName, env as MarketplaceEnv);
        return jsonResponse(result, 200, { "Cache-Control": "public, max-age=60" });
      }

      // Marketplace — POST /plugins/:name/install-event (S68-T02)
      // Silent fire-and-forget from CLI after successful plugin install.
      // Body: { instanceId: string } — opaque SHA-256 hash, no PII.
      if (method === "POST" && resolvedPath.startsWith("/plugins/") && resolvedPath.endsWith("/install-event")) {
        const parts = resolvedPath.split("/").filter(Boolean);
        // /plugins/:name/install-event → parts = ["plugins", name, "install-event"]
        const pluginName = parts[1];
        if (!pluginName) {
          return jsonResponse({ error: "plugin name required" }, 400);
        }
        const result = await handleInstallEvent(pluginName, request, env as MarketplaceEnv);
        return jsonResponse(
          result.ok
            ? { name: result.name, downloads: result.downloads, incremented: result.incremented }
            : { error: result.error },
          result.status,
        );
      }

      // Marketplace — POST /marketplace/ratings/:name
      if (method === "POST" && resolvedPath.startsWith("/marketplace/ratings/")) {
        const pluginName = resolvedPath.slice("/marketplace/ratings/".length);
        if (!pluginName) {
          return jsonResponse({ error: "plugin name required" }, 400);
        }
        const result = await handlePostRating(pluginName, request, env as MarketplaceEnv);

        // T05: Emit Retry-After header on all 429 responses
        const extraHeaders: Record<string, string> = {};
        if (result.status === 429) {
          // Extract Retry-After seconds from error message if present
          const match = (result.error ?? "").match(/Retry-After:\s*(\d+)s/);
          extraHeaders["Retry-After"] = match?.[1] ?? "60";
        }

        return jsonResponse(
          result.ok
            ? { name: result.name, rating: result.rating, reviewCount: result.reviewCount }
            : { error: result.error },
          result.status,
          extraHeaders,
        );
      }

      // Marketplace — GET /marketplace
      if (method === "GET" && resolvedPath === "/marketplace") {
        // T-RATE-01: GET rate limit — 60 req/min per IP
        const rlGet = await checkGetRateLimit(request, env as MarketplaceEnv);
        if (rlGet.limited) {
          return jsonResponse(
            { error: `Rate limit exceeded. Retry after ${rlGet.retryAfter}s.` },
            429,
            { "Retry-After": String(rlGet.retryAfter) },
          );
        }
        const { all, free, pro } = await fetchAllPlugins(env, ctx);
        const payload = await handleMarketplace(url, env as MarketplaceEnv, all, ctx);
        // Override stats counts with fresh registry counts for accuracy
        payload.stats.free = free.length;
        payload.stats.pro = pro.length;
        payload.stats.total = all.length;
        return jsonResponse(payload, 200, {
          "Cache-Control": `public, max-age=${cacheTtl(env)}`,
        });
      }

      // Sync webhook
      if (method === "POST" && resolvedPath === "/api/sync") {
        return handleSync(request, env, ctx);
      }

      return jsonResponse(
        {
          error: "not found",
          endpoints: [
            "GET /health",
            "GET /plugins",
            "GET /plugins/:name",
            "GET /plugins/:name/tarball",
            "GET /plugins/:name/signature",
            "GET /plugins/:name/:version",
            "GET /plugins/revocations",
            "GET /registry.json",
            "GET /categories",
            "GET /manifest.json",
            "GET /marketplace",
            "GET /marketplace/ratings/:name",
            "POST /marketplace/ratings/:name",
            "POST /plugins/:name/install-event",
            "GET /stats",
            "POST /api/sync",
          ],
        },
        404,
      );
    } catch (err) {
      console.error("Unhandled error:", (err as Error).message);
      return jsonResponse({ error: "internal server error", message: (err as Error).message }, 500);
    }
  },
} satisfies ExportedHandler<Env>;

// ---------------------------------------------------------------------------
// GET /health
// ---------------------------------------------------------------------------

function handleHealth(env: Env): Response {
  return jsonResponse({
    status: "ok",
    service: "nself-plugin-registry",
    version: env.PLUGIN_REGISTRY_VERSION ?? env.REGISTRY_VERSION ?? "1.0.0",
    ts: new Date().toISOString(),
    signing: {
      configured: Boolean(env.SIGNING_PRIVATE_KEY && env.PUBLIC_KEY_HEX),
    },
  });
}

// ---------------------------------------------------------------------------
// GET /plugins — PluginListResponse
// ---------------------------------------------------------------------------

async function handlePluginList(url: URL, env: Env, ctx: ExecutionContext): Promise<Response> {
  const tierFilter = url.searchParams.get("tier");
  const categoryFilter = url.searchParams.get("category");
  const ttl = cacheTtl(env);

  const { free, pro, all } = await fetchAllPlugins(env, ctx);

  let filtered = all;
  if (tierFilter === "free" || tierFilter === "pro") {
    filtered = filtered.filter((p) => p.tier === tierFilter);
  }
  if (categoryFilter) {
    filtered = filtered.filter(
      (p) => p.category.toLowerCase() === categoryFilter.toLowerCase(),
    );
  }

  const response: PluginListResponse = {
    plugins: filtered,
    total: filtered.length,
    free: free.length,
    pro: pro.length,
    generatedAt: new Date().toISOString(),
  };

  // S67-T02: emit s-maxage for CF edge cache (60s TTL, 300s stale-while-revalidate)
  return jsonResponse(response, 200, {
    "Cache-Control": `public, s-maxage=60, stale-while-revalidate=300, max-age=${ttl}`,
  });
}

// ---------------------------------------------------------------------------
// GET /plugins/:name  and sub-resources
// ---------------------------------------------------------------------------

async function handlePlugin(path: string, env: Env, ctx: ExecutionContext): Promise<Response> {
  const parts = path.split("/").filter(Boolean);
  // parts[0] = "plugins", parts[1] = name, parts[2] = sub-resource or version
  const pluginName = parts[1];
  const subResource = parts[2] ?? "latest";

  if (!pluginName) {
    return jsonResponse({ error: "plugin name required" }, 400);
  }

  const { all } = await fetchAllPlugins(env, ctx);
  const plugin = all.find((p) => p.name === pluginName);

  if (!plugin) {
    return jsonResponse(
      {
        error: "plugin not found",
        name: pluginName,
        available: all.map((p) => p.name).sort(),
      },
      404,
    );
  }

  if (subResource === "tarball") {
    return handlePluginTarball(plugin, env);
  }

  if (subResource === "signature") {
    return handlePluginSignature(plugin, env);
  }

  if (subResource !== "latest" && subResource !== plugin.version) {
    return jsonResponse(
      {
        error: "version not found",
        requestedVersion: subResource,
        latestVersion: plugin.version,
      },
      404,
    );
  }

  return jsonResponse(plugin, 200, { "Cache-Control": "public, max-age=60" });
}

// ---------------------------------------------------------------------------
// GET /plugins/:name/tarball — 302 to R2 (primary) or GitHub Releases (fallback)
//
// S67-T03: R2 bucket `nself-plugin-tarballs` is the primary CDN (free egress).
// Worker checks R2 object existence via HEAD before redirecting.
// Falls back to GitHub Releases on R2 not-found or any R2 error.
// Pro plugins always use the ping_api gated download — never this path.
// ---------------------------------------------------------------------------

async function handlePluginTarball(plugin: PluginEntry, env: Env): Promise<Response> {
  const { name, version, tier } = plugin;

  const revoked = await isRevoked(env, name, version);
  if (revoked) {
    return jsonResponse({ error: "plugin version revoked", plugin: name, version }, 410);
  }

  const repo = tier === "pro" ? "plugins-pro" : "plugins";
  // GitHub Releases URL (canonical fallback, always valid)
  const githubURL =
    `https://github.com/nself-org/${repo}/releases/download/v${version}/${name}-${version}.tar.gz`;

  // S67-T03: R2 primary for free plugins only
  let tarballURL = githubURL;
  let tarballSource = "github";
  if (tier !== "pro" && env.PLUGIN_TARBALLS && env.R2_PUBLIC_DOMAIN) {
    const r2Key = `plugins/${name}/${name}-${version}.tar.gz`;
    try {
      const obj = await env.PLUGIN_TARBALLS.head(r2Key);
      if (obj !== null) {
        tarballURL = `https://${env.R2_PUBLIC_DOMAIN}/${r2Key}`;
        tarballSource = "r2";
      }
    } catch {
      // R2 unavailable or error — fall through to GitHub fallback
    }
  }

  const headers: Record<string, string> = {
    Location: tarballURL,
    "Access-Control-Allow-Origin": "*",
    "Cache-Control": "public, max-age=300",
    "X-Tarball-Source": tarballSource,
  };

  if (env.SIGNING_PRIVATE_KEY) {
    const signature = await signPlugin(name, version, tarballURL, env.SIGNING_PRIVATE_KEY);
    if (signature) {
      headers["X-Signature"] = signature;
      headers["X-Signed-For"] = canonicalPluginString(name, version, tarballURL);
    }
  }

  return new Response(null, { status: 302, headers });
}

// ---------------------------------------------------------------------------
// GET /plugins/:name/signature — Ed25519 signature metadata
// ---------------------------------------------------------------------------

async function handlePluginSignature(plugin: PluginEntry, env: Env): Promise<Response> {
  const { name, version, tier } = plugin;

  const repo = tier === "pro" ? "plugins-pro" : "plugins";
  const tarballURL =
    `https://github.com/nself-org/${repo}/releases/download/v${version}/${name}-${version}.tar.gz`;

  if (!env.SIGNING_PRIVATE_KEY) {
    return jsonResponse(
      { name, version, tarballURL, signature: null },
      200,
      { "Cache-Control": "public, max-age=300" },
    );
  }

  const signature = await signPlugin(name, version, tarballURL, env.SIGNING_PRIVATE_KEY);

  return jsonResponse(
    {
      name,
      version,
      tarballURL,
      signature,
      algorithm: "ed25519",
      publicKey: env.PUBLIC_KEY_HEX ?? null,
      signedFor: canonicalPluginString(name, version, tarballURL),
    },
    200,
    { "Cache-Control": "public, max-age=300" },
  );
}

// ---------------------------------------------------------------------------
// GET /registry.json — combined registry (legacy CLI compatibility)
// ---------------------------------------------------------------------------

const KV_COMBINED = "registry:combined";

interface CombinedRegistry {
  version: string;
  fetchedAt: string;
  pluginCount: { free: number; pro: number; total: number };
  proAvailable: boolean;
  plugins: PluginEntry[];
  categories: Record<string, { name: string; count: number; tiers: string[] }>;
}

async function handleRegistry(url: URL, env: Env, ctx: ExecutionContext): Promise<Response> {
  const tierFilter = url.searchParams.get("tier");
  const bypass = url.searchParams.get("nocache") === "1";
  const ttl = cacheTtl(env);

  const kv = env.REGISTRY ?? env.PLUGINS_KV;

  if (!tierFilter && !bypass && kv) {
    const cached = await kvGet<CombinedRegistry>(kv, KV_COMBINED);
    if (cached && isFresh(cached, ttl)) {
      return jsonResponse(cached.data, 200, {
        "Cache-Control": `public, max-age=${ttl}`,
        "X-Cache": "HIT",
      });
    }
  }

  const { free, pro, all } = await fetchAllPlugins(env, ctx, bypass);

  const combined: CombinedRegistry = {
    version: "1.0.0",
    fetchedAt: new Date().toISOString(),
    pluginCount: { free: free.length, pro: pro.length, total: all.length },
    proAvailable: pro.length > 0,
    plugins: tierFilter ? all.filter((p) => p.tier === tierFilter) : all,
    categories: buildCategoryMap(all),
  };

  if (!tierFilter && !bypass && kv) {
    ctx.waitUntil(kvPutWrapped(kv, KV_COMBINED, combined));
  }

  return jsonResponse(combined, 200, {
    "Cache-Control": `public, max-age=${ttl}`,
    "X-Cache": "MISS",
  });
}

// ---------------------------------------------------------------------------
// GET /categories
// ---------------------------------------------------------------------------

async function handleCategories(env: Env, ctx: ExecutionContext): Promise<Response> {
  const ttl = cacheTtl(env);
  const { all } = await fetchAllPlugins(env, ctx);
  return jsonResponse({ categories: buildCategoryMap(all) }, 200, {
    "Cache-Control": `public, max-age=${ttl}`,
  });
}

// ---------------------------------------------------------------------------
// GET /manifest.json — flat array for nself plugin outdated
// ---------------------------------------------------------------------------

const KV_MANIFEST = "registry:manifest";

async function handleManifest(env: Env, ctx: ExecutionContext): Promise<Response> {
  const kv = env.REGISTRY ?? env.PLUGINS_KV;
  const ttl = cacheTtl(env);

  if (kv) {
    const cached = await kvGet<Array<Record<string, string>>>(kv, KV_MANIFEST);
    if (cached && isFresh(cached, ttl)) {
      return jsonResponse(cached.data, 200, {
        "Cache-Control": `public, max-age=${ttl}`,
        "X-Cache": "HIT",
      });
    }
  }

  const { all } = await fetchAllPlugins(env, ctx);
  const manifest = all.map((p) => ({
    name: p.name,
    version: p.version,
    tier: p.tier,
    description: p.description,
    downloadUrl: p.tarballURL || p.homepage || `https://github.com/nself-org/plugins/tree/main/${p.name}`,
  }));

  if (kv) ctx.waitUntil(kvPutWrapped(kv, KV_MANIFEST, manifest));

  return jsonResponse(manifest, 200, {
    "Cache-Control": `public, max-age=${ttl}`,
    "X-Cache": "MISS",
  });
}

// ---------------------------------------------------------------------------
// GET /.well-known/revoked-authors.json — Author Certificate Revocation List
// (S58-T09)
//
// CLI checks this endpoint on every `nself plugin install` and via daily cron
// to detect plugins whose author keys have been revoked (abuse, security
// breach, license fraud). Short max-age (60 s) because revocations are
// security-critical and must propagate quickly.
//
// KV key: revocations:authors  — JSON array of RevokedAuthorEntry
// The list is append-only: once revoked, entries must never be deleted.
// ---------------------------------------------------------------------------

const KV_REVOKED_AUTHORS = "revocations:authors";

async function handleRevokedAuthors(env: Env): Promise<Response> {
  const kv = env.REGISTRY ?? env.PLUGINS_KV;
  let revokedAuthors: RevokedAuthorEntry[] = [];

  if (kv) {
    try {
      const raw = await kv.get(KV_REVOKED_AUTHORS, "text");
      if (raw) {
        const parsed = JSON.parse(raw) as unknown;
        if (Array.isArray(parsed)) {
          revokedAuthors = parsed as RevokedAuthorEntry[];
        }
      }
    } catch {
      // KV read failure — return empty list so CLI is not falsely blocked.
      // Errors are surfaced via the /health endpoint instead.
      revokedAuthors = [];
    }
  }

  const body: RevokedAuthorListResponse = {
    revokedAuthors,
    fetchedAt: new Date().toISOString(),
    count: revokedAuthors.length,
  };

  // Short TTL: revocations are security-critical. CLI must see them promptly.
  return jsonResponse(body, 200, { "Cache-Control": "public, max-age=60" });
}

// ---------------------------------------------------------------------------
// GET /stats
// ---------------------------------------------------------------------------

const KV_STATS = "stats:global";

interface StatsRecord {
  registryHits?: number;
  registryFetches?: number;
  lastSync?: string | null;
}

async function handleStats(env: Env): Promise<Response> {
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
// POST /api/sync — bust KV cache and fetch fresh from GitHub
// ---------------------------------------------------------------------------

async function handleSync(
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

  if (kv) {
    await Promise.all([
      kv.delete("registry:free"),
      kv.delete("registry:pro"),
      kv.delete(KV_COMBINED),
      kv.delete(KV_MANIFEST),
    ]);
  }

  const [freeResult, proResult] = await Promise.allSettled([
    fetchFreeRegistry(env, ctx, true),
    fetchProRegistry(env, ctx, true),
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
        const s: StatsRecord = existing?.data ?? { registryHits: 0, registryFetches: 0 };
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
    timestamp: new Date().toISOString(),
  });
}

// ---------------------------------------------------------------------------
// Category helpers
// ---------------------------------------------------------------------------

function buildCategoryMap(
  plugins: PluginEntry[],
): Record<string, { name: string; count: number; tiers: string[] }> {
  const cats: Record<string, { name: string; count: number; tiers: string[] }> = {};
  for (const p of plugins) {
    const cat = p.category || "other";
    if (!cats[cat]) {
      cats[cat] = { name: toTitleCase(cat), count: 0, tiers: [] };
    }
    const entry = cats[cat];
    if (entry) {
      entry.count += 1;
      if (!entry.tiers.includes(p.tier)) {
        entry.tiers.push(p.tier);
      }
    }
  }
  return cats;
}

function toTitleCase(str: string): string {
  return str
    .split(/[-_]/)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}

// Re-export types for external consumers
export type { Env, PluginEntry, PluginListResponse, KVEnvelope };
