/**
 * nself Plugin Registry - Cloudflare Worker
 *
 * Provides a fast, cached API for the nself plugin registry.
 * Syncs from GitHub and serves plugin metadata to nself CLI.
 *
 * Endpoints:
 *   GET /registry.json          - Full registry (cached)
 *   GET /plugins/:name          - Plugin info (latest version)
 *   GET /plugins/:name/:version - Plugin info (specific version)
 *   GET /health                 - Health check
 *   GET /stats                  - Registry statistics
 *   POST /api/sync              - Webhook to sync from GitHub
 */

// CORS headers for browser access (if needed)
const corsHeaders = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, POST, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type, Authorization',
};

// Cache keys
const REGISTRY_CACHE_KEY = 'registry:latest';
const STATS_CACHE_KEY = 'stats:global';

/**
 * Main request handler
 */
export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);
    const path = url.pathname;

    // Handle CORS preflight
    if (request.method === 'OPTIONS') {
      return new Response(null, { headers: corsHeaders });
    }

    try {
      // Route requests
      if (path === '/registry.json' || path === '/') {
        return await handleRegistry(env, ctx);
      }

      if (path.startsWith('/plugins/')) {
        return await handlePluginInfo(path, env);
      }

      if (path === '/health') {
        return handleHealth(env);
      }

      if (path === '/stats') {
        return await handleStats(env);
      }

      if (path === '/api/sync' && request.method === 'POST') {
        return await handleSync(request, env, ctx);
      }

      if (path === '/categories') {
        return await handleCategories(env);
      }

      // 404 for unknown routes
      return new Response(JSON.stringify({
        error: 'Not found',
        endpoints: [
          'GET /registry.json',
          'GET /plugins/:name',
          'GET /plugins/:name/:version',
          'GET /health',
          'GET /stats',
          'GET /categories'
        ]
      }), {
        status: 404,
        headers: {
          'Content-Type': 'application/json',
          ...corsHeaders
        }
      });

    } catch (error) {
      console.error('Request error:', error);
      return new Response(JSON.stringify({
        error: 'Internal server error',
        message: error.message
      }), {
        status: 500,
        headers: {
          'Content-Type': 'application/json',
          ...corsHeaders
        }
      });
    }
  }
};

/**
 * Get full registry (with caching)
 */
async function handleRegistry(env, ctx) {
  const cacheTtl = parseInt(env.CACHE_TTL) || 300;

  // Try KV cache first
  if (env.PLUGINS_KV) {
    const cached = await env.PLUGINS_KV.get(REGISTRY_CACHE_KEY, 'json');
    if (cached && cached.timestamp) {
      const age = (Date.now() - cached.timestamp) / 1000;
      if (age < cacheTtl) {
        // Increment stats asynchronously
        ctx.waitUntil(incrementStats(env, 'registry_hits'));

        return new Response(JSON.stringify(cached.data), {
          headers: {
            'Content-Type': 'application/json',
            'Cache-Control': `public, max-age=${Math.floor(cacheTtl - age)}`,
            'X-Cache': 'HIT',
            'X-Cache-Age': Math.floor(age).toString(),
            ...corsHeaders
          }
        });
      }
    }
  }

  // Fetch from GitHub
  const registry = await fetchFromGitHub(env);

  if (!registry) {
    return new Response(JSON.stringify({ error: 'Failed to fetch registry' }), {
      status: 502,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }

  // Cache in KV
  if (env.PLUGINS_KV) {
    ctx.waitUntil(
      env.PLUGINS_KV.put(REGISTRY_CACHE_KEY, JSON.stringify({
        data: registry,
        timestamp: Date.now()
      }))
    );
  }

  // Increment stats asynchronously
  ctx.waitUntil(incrementStats(env, 'registry_fetches'));

  return new Response(JSON.stringify(registry), {
    headers: {
      'Content-Type': 'application/json',
      'Cache-Control': `public, max-age=${cacheTtl}`,
      'X-Cache': 'MISS',
      ...corsHeaders
    }
  });
}

/**
 * Get specific plugin info
 */
async function handlePluginInfo(path, env) {
  const parts = path.split('/').filter(Boolean);
  // parts = ['plugins', 'name'] or ['plugins', 'name', 'version']

  if (parts.length < 2) {
    return new Response(JSON.stringify({ error: 'Plugin name required' }), {
      status: 400,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }

  const pluginName = parts[1];
  const requestedVersion = parts[2] || 'latest';

  // Get registry
  let registry;
  if (env.PLUGINS_KV) {
    const cached = await env.PLUGINS_KV.get(REGISTRY_CACHE_KEY, 'json');
    if (cached && cached.data) {
      registry = cached.data;
    }
  }

  if (!registry) {
    registry = await fetchFromGitHub(env);
  }

  if (!registry || !registry.plugins) {
    return new Response(JSON.stringify({ error: 'Registry unavailable' }), {
      status: 502,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }

  // Find plugin (plugins is an object keyed by name)
  const plugin = registry.plugins[pluginName];

  if (!plugin) {
    return new Response(JSON.stringify({
      error: 'Plugin not found',
      available: Object.keys(registry.plugins)
    }), {
      status: 404,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }

  // Version resolution (for now, we only have latest)
  // In future, could support version history
  if (requestedVersion !== 'latest' && requestedVersion !== plugin.version) {
    return new Response(JSON.stringify({
      error: 'Version not found',
      requestedVersion,
      availableVersion: plugin.version
    }), {
      status: 404,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }

  return new Response(JSON.stringify(plugin), {
    headers: {
      'Content-Type': 'application/json',
      'Cache-Control': 'public, max-age=60',
      ...corsHeaders
    }
  });
}

/**
 * Health check endpoint
 */
function handleHealth(env) {
  return new Response(JSON.stringify({
    status: 'healthy',
    service: 'nself-plugin-registry',
    version: env.REGISTRY_VERSION || '1.0.0',
    timestamp: new Date().toISOString()
  }), {
    headers: { 'Content-Type': 'application/json', ...corsHeaders }
  });
}

/**
 * Get registry statistics
 */
async function handleStats(env) {
  let stats = {
    registryHits: 0,
    registryFetches: 0,
    pluginDownloads: {},
    lastSync: null
  };

  if (env.PLUGINS_KV) {
    const cached = await env.PLUGINS_KV.get(STATS_CACHE_KEY, 'json');
    if (cached) {
      stats = cached;
    }
  }

  return new Response(JSON.stringify(stats), {
    headers: { 'Content-Type': 'application/json', ...corsHeaders }
  });
}

/**
 * Get available categories
 */
async function handleCategories(env) {
  let registry;
  if (env.PLUGINS_KV) {
    const cached = await env.PLUGINS_KV.get(REGISTRY_CACHE_KEY, 'json');
    if (cached && cached.data) {
      registry = cached.data;
    }
  }

  if (!registry) {
    registry = await fetchFromGitHub(env);
  }

  const categories = registry?.categories || {};

  return new Response(JSON.stringify({ categories }), {
    headers: { 'Content-Type': 'application/json', ...corsHeaders }
  });
}

/**
 * Handle sync webhook from GitHub Actions
 */
async function handleSync(request, env, ctx) {
  // Verify authorization
  const authHeader = request.headers.get('Authorization');
  if (!authHeader || !authHeader.startsWith('Bearer ')) {
    return new Response(JSON.stringify({ error: 'Unauthorized' }), {
      status: 401,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }

  const token = authHeader.slice(7);
  if (token !== env.GITHUB_SYNC_TOKEN) {
    return new Response(JSON.stringify({ error: 'Invalid token' }), {
      status: 403,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }

  // Force refresh from GitHub
  const registry = await fetchFromGitHub(env);

  if (!registry) {
    return new Response(JSON.stringify({ error: 'Failed to fetch registry' }), {
      status: 502,
      headers: { 'Content-Type': 'application/json', ...corsHeaders }
    });
  }

  // Update cache
  if (env.PLUGINS_KV) {
    await env.PLUGINS_KV.put(REGISTRY_CACHE_KEY, JSON.stringify({
      data: registry,
      timestamp: Date.now()
    }));

    // Update sync timestamp in stats
    const stats = await env.PLUGINS_KV.get(STATS_CACHE_KEY, 'json') || {};
    stats.lastSync = new Date().toISOString();
    await env.PLUGINS_KV.put(STATS_CACHE_KEY, JSON.stringify(stats));
  }

  return new Response(JSON.stringify({
    success: true,
    message: 'Registry synced',
    pluginCount: Object.keys(registry.plugins || {}).length,
    timestamp: new Date().toISOString()
  }), {
    headers: { 'Content-Type': 'application/json', ...corsHeaders }
  });
}

/**
 * Fetch registry from GitHub
 */
async function fetchFromGitHub(env) {
  const repo = env.GITHUB_REPO || 'acamarata/nself-plugins';
  const branch = env.GITHUB_BRANCH || 'main';
  const url = `https://raw.githubusercontent.com/${repo}/${branch}/registry.json`;

  try {
    const response = await fetch(url, {
      headers: {
        'User-Agent': 'nself-plugin-registry/1.0'
      }
    });

    if (!response.ok) {
      console.error(`GitHub fetch failed: ${response.status}`);
      return null;
    }

    const registry = await response.json();

    // Add metadata
    registry.fetchedAt = new Date().toISOString();
    registry.source = 'github';

    return registry;

  } catch (error) {
    console.error('GitHub fetch error:', error);
    return null;
  }
}

/**
 * Increment statistics counter
 */
async function incrementStats(env, key) {
  if (!env.PLUGINS_KV) return;

  try {
    const stats = await env.PLUGINS_KV.get(STATS_CACHE_KEY, 'json') || {
      registryHits: 0,
      registryFetches: 0,
      pluginDownloads: {}
    };

    if (key === 'registry_hits') {
      stats.registryHits = (stats.registryHits || 0) + 1;
    } else if (key === 'registry_fetches') {
      stats.registryFetches = (stats.registryFetches || 0) + 1;
    } else if (key.startsWith('download:')) {
      const plugin = key.slice(9);
      stats.pluginDownloads[plugin] = (stats.pluginDownloads[plugin] || 0) + 1;
    }

    await env.PLUGINS_KV.put(STATS_CACHE_KEY, JSON.stringify(stats));
  } catch (error) {
    console.error('Stats update error:', error);
  }
}
