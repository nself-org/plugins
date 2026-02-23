/**
 * nself Plugin Registry — Cloudflare Worker
 *
 * Dual-registry combining architecture.
 *
 * Serves a unified plugin registry that merges:
 *   - Free registry  (public, plugins/registry.json on GitHub)
 *   - Pro registry   (private, plugins-pro/registry.json via GitHub API with token)
 *
 * Endpoints:
 *   GET /registry.json              Combined registry (free + pro), or ?tier=free|pro to filter
 *   GET /registry                   Alias for /registry.json
 *   GET /plugins/:name              Single plugin metadata (free or pro)
 *   GET /plugins/:name/:version     Single plugin at specific version
 *   GET /categories                 All categories (merged from both registries)
 *   GET /health                     Health check
 *   GET /stats                      Cache statistics
 *   POST /api/sync                  Force-refresh KV cache (webhook from GitHub Actions)
 *
 * KV cache keys:
 *   registry:free          — cached free registry raw JSON (with timestamp envelope)
 *   registry:pro           — cached pro registry raw JSON (with timestamp envelope)
 *   registry:combined      — cached merged output (with timestamp envelope)
 *   stats:global           — request statistics
 *
 * Secrets (set via wrangler secret put):
 *   GITHUB_TOKEN           — Fine-grained PAT with contents:read on nself-org/plugins-pro
 *                            Also used for authenticated free registry fetch (avoids raw CDN cache)
 *   GITHUB_SYNC_TOKEN      — Bearer token for POST /api/sync (from GitHub Actions)
 */

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

// GitHub Contents API for both registries — authenticated fetch avoids CDN cache lag
const FREE_REGISTRY_API_URL =
  'https://api.github.com/repos/nself-org/plugins/contents/registry.json';

const PRO_REGISTRY_API_URL =
  'https://api.github.com/repos/nself-org/plugins-pro/contents/registry.json';

// Fallback: unauthenticated raw URL for free registry (used when GITHUB_TOKEN absent)
const FREE_REGISTRY_RAW_URL =
  'https://raw.githubusercontent.com/nself-org/plugins/main/registry.json';

const KV_FREE     = 'registry:free';
const KV_PRO      = 'registry:pro';
const KV_COMBINED = 'registry:combined';
const KV_STATS    = 'stats:global';

const DEFAULT_CACHE_TTL = 300; // seconds

// ---------------------------------------------------------------------------
// CORS + response helpers
// ---------------------------------------------------------------------------

function corsHeaders() {
  return {
    'Access-Control-Allow-Origin':  '*',
    'Access-Control-Allow-Methods': 'GET, POST, OPTIONS',
    'Access-Control-Allow-Headers': 'Content-Type, Authorization',
  };
}

function jsonResponse(body, status = 200, extra = {}) {
  return new Response(JSON.stringify(body, null, 2), {
    status,
    headers: {
      'Content-Type': 'application/json',
      ...corsHeaders(),
      ...extra,
    },
  });
}

// ---------------------------------------------------------------------------
// Main fetch handler
// ---------------------------------------------------------------------------

export default {
  async fetch(request, env, ctx) {
    const url    = new URL(request.url);
    const path   = url.pathname.replace(/\/$/, '') || '/';
    const method = request.method.toUpperCase();

    if (method === 'OPTIONS') {
      return new Response(null, { status: 204, headers: corsHeaders() });
    }

    try {
      if (method === 'GET' && (path === '/registry.json' || path === '/registry' || path === '/')) {
        return await handleRegistry(url, env, ctx);
      }
      if (method === 'GET' && path.startsWith('/plugins/')) {
        return await handlePlugin(path, env, ctx);
      }
      if (method === 'GET' && path === '/categories') {
        return await handleCategories(env, ctx);
      }


      if (method === 'GET' && path === '/health') {
        return handleHealth(env);
      }
      if (method === 'GET' && path === '/stats') {
        return await handleStats(env);
      }
      if (method === 'POST' && path === '/api/sync') {
        return await handleSync(request, env, ctx);
      }

      return jsonResponse({
        error: 'Not found',
        endpoints: [
          'GET /registry.json[?tier=free|pro]',
          'GET /plugins/:name',
          'GET /plugins/:name/:version',
          'GET /categories',
          'GET /health',
          'GET /stats',
          'POST /api/sync',
        ],
      }, 404);

    } catch (err) {
      console.error('Unhandled error:', err);
      return jsonResponse({ error: 'Internal server error', message: err.message }, 500);
    }
  },
};

// ---------------------------------------------------------------------------
// Registry handler
// ---------------------------------------------------------------------------

async function handleRegistry(url, env, ctx) {
  const tierFilter = url.searchParams.get('tier');
  const cacheTtl   = parseInt(env.CACHE_TTL || DEFAULT_CACHE_TTL, 10);
  const bypassKv   = url.searchParams.get('nocache') === '1';

  // Serve combined from KV cache when no filter and not bypassing
  if (!tierFilter && !bypassKv) {
    const cached = await kvGet(env, KV_COMBINED);
    if (cached && isFresh(cached, cacheTtl)) {
      ctx.waitUntil(bumpStat(env, 'registry_hits'));
      return jsonResponse(cached.data, 200, {
        'Cache-Control': `public, max-age=${ttlRemaining(cached, cacheTtl)}`,
        'X-Cache':       'HIT',
        'X-Cache-Age':   String(cacheAge(cached)),
        'X-Tier':        'combined',
      });
    }
  }

  const [freeResult, proResult] = await Promise.allSettled([
    fetchFreeRegistry(env, ctx, cacheTtl, bypassKv),
    fetchProRegistry(env, ctx, cacheTtl, bypassKv),
  ]);

  const freePlugins = freeResult.status === 'fulfilled' ? freeResult.value : null;
  const proPlugins  = proResult.status  === 'fulfilled' ? proResult.value  : null;

  if (!freePlugins && !proPlugins) {
    return jsonResponse({ error: 'Registry unavailable — both sources failed' }, 502);
  }

  const allPlugins = [
    ...(freePlugins || []),
    ...(proPlugins  || []),
  ];

  const combined = {
    version:      '1.0.0',
    fetchedAt:    new Date().toISOString(),
    pluginCount:  { free: (freePlugins || []).length, pro: (proPlugins || []).length, total: allPlugins.length },
    proAvailable: proPlugins !== null,
    plugins:      allPlugins,
    categories:   mergeCategoriesFromPlugins(allPlugins),
  };

  // Cache combined result
  if (!tierFilter && !bypassKv) {
    ctx.waitUntil(kvPutWrapped(env, KV_COMBINED, combined));
    ctx.waitUntil(bumpStat(env, 'registry_fetches'));
  }

  const headers = {
    'Cache-Control': `public, max-age=${cacheTtl}`,
    'X-Cache':       'MISS',
    'X-Tier':        tierFilter || 'combined',
  };

  if (!proPlugins) {
    headers['X-Pro-Registry-Status'] = 'unavailable';
    headers['X-Warning'] = 'Pro registry unavailable — ensure GITHUB_TOKEN secret is set';
  }

  let outputPlugins = allPlugins;
  if (tierFilter === 'free' || tierFilter === 'pro') {
    outputPlugins = allPlugins.filter(p => p.tier === tierFilter);
  }

  return jsonResponse({ ...combined, plugins: outputPlugins }, 200, headers);
}

// ---------------------------------------------------------------------------
// Fetch free registry
// Uses GitHub Contents API when GITHUB_TOKEN is available (bypasses CDN cache),
// falls back to raw URL otherwise.
// ---------------------------------------------------------------------------

async function fetchFreeRegistry(env, ctx, cacheTtl, bypass = false) {
  if (!bypass) {
    const cached = await kvGet(env, KV_FREE);
    if (cached && isFresh(cached, cacheTtl)) {
      return normalizeToArray(cached.data, 'free');
    }
  }

  let data;

  if (env.GH_ACCESS_TOKEN) {
    // Authenticated GitHub API fetch — bypasses CDN, always returns latest commit
    const resp = await fetch(FREE_REGISTRY_API_URL, {
      headers: {
        'Authorization': `token ${env.GH_ACCESS_TOKEN}`,
        'Accept':        'application/vnd.github.v3+json',
        'User-Agent':    'nself-plugin-registry/2.0',
      },
    });

    if (resp.ok) {
      const envelope = await resp.json();
      try {
        const raw = atob(envelope.content.replace(/\n/g, ''));
        data = JSON.parse(raw);
      } catch (e) {
        console.error('Failed to decode free registry content:', e.message);
      }
    } else {
      console.warn(`Free registry GitHub API fetch failed: ${resp.status}, falling back to raw URL`);
    }
  }

  // Fallback: unauthenticated raw URL
  if (!data) {
    const resp = await fetch(FREE_REGISTRY_RAW_URL, {
      headers: { 'User-Agent': 'nself-plugin-registry/2.0' },
    });
    if (!resp.ok) {
      throw new Error(`Free registry raw fetch failed: ${resp.status}`);
    }
    data = await resp.json();
  }

  ctx.waitUntil(kvPutWrapped(env, KV_FREE, data));
  return normalizeToArray(data, 'free');
}

// ---------------------------------------------------------------------------
// Fetch pro registry (private GitHub Contents API)
// ---------------------------------------------------------------------------

async function fetchProRegistry(env, ctx, cacheTtl, bypass = false) {
  if (!bypass) {
    const cached = await kvGet(env, KV_PRO);
    if (cached && isFresh(cached, cacheTtl)) {
      return normalizeToArray(cached.data, 'pro');
    }
  }

  if (!env.GH_ACCESS_TOKEN) {
    console.warn('GITHUB_TOKEN not set — pro registry unavailable');
    return null;
  }

  const resp = await fetch(PRO_REGISTRY_API_URL, {
    headers: {
      'Authorization': `token ${env.GH_ACCESS_TOKEN}`,
      'Accept':        'application/vnd.github.v3+json',
      'User-Agent':    'nself-plugin-registry/2.0',
    },
  });

  if (!resp.ok) {
    const body = await resp.text().catch(() => '');
    console.error(`Pro registry fetch failed: ${resp.status} — ${body.slice(0, 200)}`);
    return null;
  }

  const envelope = await resp.json();
  let data;
  try {
    const raw = atob(envelope.content.replace(/\n/g, ''));
    data = JSON.parse(raw);
  } catch (e) {
    console.error('Failed to decode pro registry content:', e.message);
    return null;
  }

  ctx.waitUntil(kvPutWrapped(env, KV_PRO, data));
  return normalizeToArray(data, 'pro');
}

// ---------------------------------------------------------------------------
// Normalize registry formats → flat array with tier guaranteed
//
// Free registry format: { plugins: { "name": { ... } } }  (object keyed by name)
// Pro registry format:  { plugins: [ { name, tier, ... } ] }  (array)
// ---------------------------------------------------------------------------

function normalizeToArray(data, expectedTier) {
  let raw;

  if (Array.isArray(data)) {
    raw = data;
  } else if (data && Array.isArray(data.plugins)) {
    raw = data.plugins;
  } else if (data && data.plugins && typeof data.plugins === 'object') {
    raw = Object.values(data.plugins);
  } else {
    return [];
  }

  return raw.map(p => ({ ...p, tier: p.tier || expectedTier }));
}

// ---------------------------------------------------------------------------
// Single plugin endpoint
// ---------------------------------------------------------------------------

async function handlePlugin(path, env, ctx) {
  const parts        = path.split('/').filter(Boolean);
  const pluginName   = parts[1];
  const requestedVer = parts[2] || 'latest';
  const cacheTtl     = parseInt(env.CACHE_TTL || DEFAULT_CACHE_TTL, 10);

  if (!pluginName) {
    return jsonResponse({ error: 'Plugin name required' }, 400);
  }

  const [freeResult, proResult] = await Promise.allSettled([
    fetchFreeRegistry(env, ctx, cacheTtl),
    fetchProRegistry(env, ctx, cacheTtl),
  ]);

  const all = [
    ...(freeResult.status === 'fulfilled' ? (freeResult.value || []) : []),
    ...(proResult.status  === 'fulfilled' ? (proResult.value  || []) : []),
  ];

  const plugin = all.find(p => p.name === pluginName);
  if (!plugin) {
    return jsonResponse({ error: 'Plugin not found', name: pluginName, available: all.map(p => p.name).sort() }, 404);
  }

  if (requestedVer !== 'latest' && requestedVer !== plugin.version) {
    return jsonResponse({ error: 'Version not found', requestedVersion: requestedVer, latestVersion: plugin.version }, 404);
  }

  return jsonResponse(plugin, 200, { 'Cache-Control': 'public, max-age=60' });
}

// ---------------------------------------------------------------------------
// Categories endpoint
// ---------------------------------------------------------------------------

async function handleCategories(env, ctx) {
  const cacheTtl = parseInt(env.CACHE_TTL || DEFAULT_CACHE_TTL, 10);
  const [freeResult, proResult] = await Promise.allSettled([
    fetchFreeRegistry(env, ctx, cacheTtl),
    fetchProRegistry(env, ctx, cacheTtl),
  ]);
  const all = [
    ...(freeResult.status === 'fulfilled' ? (freeResult.value || []) : []),
    ...(proResult.status  === 'fulfilled' ? (proResult.value  || []) : []),
  ];
  return jsonResponse({ categories: mergeCategoriesFromPlugins(all) }, 200, {
    'Cache-Control': `public, max-age=${cacheTtl}`,
  });
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

function handleHealth(env) {
  return jsonResponse({
    status:    'healthy',
    service:   'nself-plugin-registry',
    version:   env.REGISTRY_VERSION || '2.0.0',
    timestamp: new Date().toISOString(),
    features:  ['dual-registry', 'kv-cache', 'tier-filter', 'github-api-fetch'],
  });
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

async function handleStats(env) {
  const stats = (await kvGet(env, KV_STATS)) || {
    registryHits:    0,
    registryFetches: 0,
    lastSync:        null,
  };
  // Unwrap if stats were stored in the timestamp envelope
  const data = stats.data || stats;
  return jsonResponse(data);
}

// ---------------------------------------------------------------------------
// Sync — bust KV cache and fetch fresh from GitHub
// Done in two steps: delete KV (async), then immediately fetch bypassing cache.
// ---------------------------------------------------------------------------

async function handleSync(request, env, ctx) {
  const authHeader = request.headers.get('Authorization') || '';
  if (!authHeader.startsWith('Bearer ')) {
    return jsonResponse({ error: 'Unauthorized' }, 401);
  }
  const token = authHeader.slice(7);
  if (!env.GITHUB_SYNC_TOKEN || token !== env.GITHUB_SYNC_TOKEN) {
    return jsonResponse({ error: 'Invalid token' }, 403);
  }

  // Delete KV cache entries
  if (env.PLUGINS_KV) {
    await Promise.all([
      env.PLUGINS_KV.delete(KV_FREE),
      env.PLUGINS_KV.delete(KV_PRO),
      env.PLUGINS_KV.delete(KV_COMBINED),
    ]);
  }

  // Fetch fresh — bypass=true so we skip KV read (already deleted)
  const [freeResult, proResult] = await Promise.allSettled([
    fetchFreeRegistry(env, ctx, 0, true),
    fetchProRegistry(env, ctx, 0, true),
  ]);

  const freeCount = freeResult.status === 'fulfilled' ? (freeResult.value || []).length : 0;
  const proCount  = proResult.status  === 'fulfilled' ? (proResult.value  || []).length : 0;
  const freeError = freeResult.status === 'rejected'  ? freeResult.reason?.message : null;
  const proError  = proResult.status  === 'rejected'  ? proResult.reason?.message  : null;

  // Update last sync timestamp
  ctx.waitUntil((async () => {
    if (env.PLUGINS_KV) {
      const existing = (await kvGet(env, KV_STATS)) || {};
      const stats = existing.data || existing;
      stats.lastSync = new Date().toISOString();
      await env.PLUGINS_KV.put(KV_STATS, JSON.stringify(stats));
    }
  })());

  return jsonResponse({
    success:    true,
    message:    'Registry caches refreshed',
    freeCount,
    proCount,
    totalCount: freeCount + proCount,
    freeError,
    proError,
    timestamp:  new Date().toISOString(),
  });
}

// ---------------------------------------------------------------------------
// KV helpers — all values stored as { data, timestamp } envelope
// ---------------------------------------------------------------------------

async function kvGet(env, key) {
  if (!env.PLUGINS_KV) return null;
  try {
    return await env.PLUGINS_KV.get(key, 'json');
  } catch {
    return null;
  }
}

async function kvPutWrapped(env, key, data) {
  if (!env.PLUGINS_KV) return;
  try {
    await env.PLUGINS_KV.put(key, JSON.stringify({ data, timestamp: Date.now() }));
  } catch (e) {
    console.error(`KV put failed for key ${key}:`, e.message);
  }
}

// ---------------------------------------------------------------------------
// Cache freshness helpers
// ---------------------------------------------------------------------------

function isFresh(entry, ttlSeconds) {
  if (!entry || !entry.timestamp) return false;
  return (Date.now() - entry.timestamp) / 1000 < ttlSeconds;
}

function cacheAge(entry) {
  return Math.floor((Date.now() - (entry.timestamp || 0)) / 1000);
}

function ttlRemaining(entry, ttlSeconds) {
  return Math.max(0, Math.floor(ttlSeconds - cacheAge(entry)));
}

// ---------------------------------------------------------------------------
// Stats helper
// ---------------------------------------------------------------------------

async function bumpStat(env, key) {
  if (!env.PLUGINS_KV) return;
  try {
    const entry   = (await kvGet(env, KV_STATS)) || {};
    const stats   = entry.data || entry;
    if (key === 'registry_hits')    stats.registryHits    = (stats.registryHits    || 0) + 1;
    if (key === 'registry_fetches') stats.registryFetches = (stats.registryFetches || 0) + 1;
    await env.PLUGINS_KV.put(KV_STATS, JSON.stringify(stats));
  } catch { /* best-effort */ }
}

// ---------------------------------------------------------------------------
// Merge categories from combined plugin list
// ---------------------------------------------------------------------------

function mergeCategoriesFromPlugins(plugins) {
  const cats = {};
  for (const p of plugins) {
    const cat = p.category || 'other';
    if (!cats[cat]) {
      cats[cat] = { name: toTitleCase(cat), count: 0, tiers: [] };
    }
    cats[cat].count += 1;
    if (p.tier && !cats[cat].tiers.includes(p.tier)) {
      cats[cat].tiers.push(p.tier);
    }
  }
  return cats;
}

function toTitleCase(str) {
  return str.split(/[-_]/).map(w => w.charAt(0).toUpperCase() + w.slice(1)).join(' ');
}
