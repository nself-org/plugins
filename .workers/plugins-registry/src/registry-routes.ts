import { jsonResponse } from "./http.ts";
import { fetchAllPlugins, cacheTtl, isFresh } from "./registry.ts";
import type { Env, PluginEntry } from "./registry.ts";
import { edgeGetWrapped, edgePutWrapped } from "./edge-cache.ts";

const KV_COMBINED = "registry:combined";

interface CombinedRegistry {
  version: string;
  fetchedAt: string;
  pluginCount: { free: number; pro: number; total: number };
  proAvailable: boolean;
  plugins: PluginEntry[];
  categories: Record<string, { name: string; count: number; tiers: string[] }>;
}

export async function handleRegistry(url: URL, env: Env, ctx: ExecutionContext): Promise<Response> {
  // Only free|pro filter, as the retired index.js did; any other value means no filter.
  const tierParam = url.searchParams.get("tier");
  const tierFilter = tierParam === "free" || tierParam === "pro" ? tierParam : null;
  const bypass = url.searchParams.get("nocache") === "1";
  const ttl = cacheTtl(env);


  if (!tierFilter && !bypass) {
    const cached = await edgeGetWrapped<CombinedRegistry>(KV_COMBINED);
    if (cached && isFresh(cached, ttl)) {
      const age = Math.max(0, Math.floor((Date.now() - cached.timestamp) / 1000));
      return jsonResponse(cached.data, 200, {
        "Cache-Control": `public, max-age=${Math.max(0, ttl - age)}`,
        "X-Cache": "HIT",
        "X-Cache-Age": String(age),
        "X-Tier": "combined",
      });
    }
  }

  const { free, pro, all } = await fetchAllPlugins(env, ctx, bypass);
  if (all.length === 0) {
    // Both sources failed: say so (as index.js did) instead of caching an empty registry.
    return jsonResponse({ error: "Registry unavailable — both sources failed" }, 502);
  }

  const combined: CombinedRegistry = {
    version: "1.0.0",
    fetchedAt: new Date().toISOString(),
    pluginCount: { free: free.length, pro: pro.length, total: all.length },
    proAvailable: pro.length > 0,
    plugins: tierFilter ? all.filter((p) => p.tier === tierFilter) : all,
    categories: buildCategoryMap(all),
  };

  if (!tierFilter && !bypass) {
    ctx.waitUntil(edgePutWrapped(KV_COMBINED, combined));
  }

  return jsonResponse(combined, 200, {
    "Cache-Control": `public, max-age=${ttl}`,
    "X-Cache": "MISS",
    "X-Tier": tierFilter ?? "combined",
  });
}

// ---------------------------------------------------------------------------
// GET /categories
// ---------------------------------------------------------------------------

export async function handleCategories(env: Env, ctx: ExecutionContext): Promise<Response> {
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

export async function handleManifest(env: Env, ctx: ExecutionContext): Promise<Response> {
  const ttl = cacheTtl(env);

  {
    const cached = await edgeGetWrapped<Array<Record<string, string>>>(KV_MANIFEST);
    if (cached && isFresh(cached, ttl)) {
      const age = Math.max(0, Math.floor((Date.now() - cached.timestamp) / 1000));
      return jsonResponse(cached.data, 200, {
        "Cache-Control": `public, max-age=${Math.max(0, ttl - age)}`,
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
    // an explicit downloadUrl wins (index.js), then the tarball, then the homepage
    downloadUrl:
      (p as { downloadUrl?: string }).downloadUrl ||
      p.tarballURL ||
      p.homepage ||
      `https://github.com/nself-org/plugins/tree/main/${p.name}`,
  }));

  ctx.waitUntil(edgePutWrapped(KV_MANIFEST, manifest));

  return jsonResponse(manifest, 200, {
    "Cache-Control": `public, max-age=${ttl}`,
    "X-Cache": "MISS",
  });
}

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
