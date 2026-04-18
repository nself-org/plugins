/**
 * nself Plugin Registry — Marketplace endpoint
 *
 * GET /marketplace[?tier=free|pro][&category=X][&bundle=Y][&q=search-term]
 *
 * Returns a categorized marketplace view of the plugin catalog that the Admin UI,
 * the CLI `nself plugin marketplace list`, and web/cloud consume.
 *
 * Response envelope:
 *   {
 *     version:     "1.0.0",
 *     fetchedAt:   ISO-8601,
 *     total:       N,
 *     categories:  [{ name, slug, count, plugins: [ PluginCard ] }],
 *     plugins:     [ PluginCard ],   // flat view, for search/filter clients
 *     bundles:     [{ slug, name, price, plugins: [names] }],
 *     filters:     { tiers, categories, bundles }
 *   }
 *
 * PluginCard shape (augments raw registry plugin):
 *   {
 *     name, displayName, version, description, tier, category,
 *     author, homepage, repository, icon, tags,
 *     rating, downloads,            // placeholders when KV ratings unavailable
 *     bundle,                       // bundle membership (e.g. "nclaw", "nchat")
 *     price,                        // "$0.99/mo" for bundle-priced plugins, "free" otherwise
 *     related,                      // list of plugin names in same bundle
 *     licenseRequired,              // true for pro tier
 *   }
 */

// ---------------------------------------------------------------------------
// Bundle membership (canonical — keep in sync with SPORT F06-BUNDLE-INVENTORY)
// ---------------------------------------------------------------------------

const BUNDLES = {
  nclaw: {
    name: 'ɳClaw Bundle',
    price: '$0.99/mo',
    plugins: ['ai', 'claw', 'claw-web', 'mux', 'voice', 'browser', 'google', 'notify', 'cron'],
  },
  clawde: {
    name: 'ClawDE+ Bundle',
    price: '$1.99/mo',
    plugins: ['realtime', 'auth', 'cms', 'notify'],
  },
  ntv: {
    name: 'nTV Bundle',
    price: '$0.99/mo',
    plugins: ['media-processing', 'streaming', 'epg', 'tmdb', 'torrent-manager', 'content-acquisition'],
  },
  nfamily: {
    name: 'nFamily Bundle',
    price: '$0.99/mo',
    plugins: ['social', 'photos', 'activity-feed', 'moderation', 'realtime', 'cms', 'chat'],
  },
  nchat: {
    name: 'nChat Bundle',
    price: '$0.99/mo',
    plugins: ['chat', 'livekit', 'recording', 'moderation', 'bots', 'realtime', 'auth'],
  },
};

// ---------------------------------------------------------------------------
// Category display helpers
// ---------------------------------------------------------------------------

const CATEGORY_DISPLAY = {
  authentication: 'Authentication',
  automation: 'Automation',
  commerce: 'Commerce',
  communication: 'Communication',
  content: 'Content',
  data: 'Data',
  development: 'Development',
  infrastructure: 'Infrastructure',
  integrations: 'Integrations',
  media: 'Media',
  streaming: 'Streaming',
  sports: 'Sports',
  compliance: 'Compliance',
  ai: 'AI',
  other: 'Other',
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function bundleFor(pluginName) {
  for (const [slug, bundle] of Object.entries(BUNDLES)) {
    if (bundle.plugins.includes(pluginName)) {
      return { slug, ...bundle };
    }
  }
  return null;
}

function relatedPlugins(pluginName) {
  const bundle = bundleFor(pluginName);
  if (!bundle) return [];
  return bundle.plugins.filter((n) => n !== pluginName);
}

function toDisplayName(plugin) {
  if (plugin.displayName) return plugin.displayName;
  const parts = (plugin.name || '').split(/[-_]/);
  return parts.map((p) => p.charAt(0).toUpperCase() + p.slice(1)).join(' ');
}

function categorize(plugins) {
  const cats = {};
  for (const p of plugins) {
    const slug = (p.category || 'other').toLowerCase();
    if (!cats[slug]) {
      cats[slug] = {
        name: CATEGORY_DISPLAY[slug] || slug.charAt(0).toUpperCase() + slug.slice(1),
        slug,
        count: 0,
        plugins: [],
      };
    }
    cats[slug].plugins.push(p);
    cats[slug].count += 1;
  }
  return Object.values(cats).sort((a, b) => a.name.localeCompare(b.name));
}

function buildCard(plugin, ratings) {
  const bundle = bundleFor(plugin.name);
  const rating = ratings && ratings[plugin.name];
  return {
    name: plugin.name,
    displayName: toDisplayName(plugin),
    version: plugin.version || '0.0.0',
    description: plugin.description || '',
    tier: plugin.tier || 'free',
    category: plugin.category || 'other',
    author: plugin.author || 'nself',
    homepage: plugin.homepage || '',
    repository: plugin.repository || '',
    icon: plugin.icon || '',
    tags: Array.isArray(plugin.tags) ? plugin.tags : [],
    rating: (rating && typeof rating.average === 'number') ? rating.average : 0,
    ratingCount: (rating && typeof rating.count === 'number') ? rating.count : 0,
    downloads: plugin.downloads || 0,
    bundle: bundle ? bundle.slug : null,
    bundleName: bundle ? bundle.name : null,
    price: bundle ? bundle.price : ((plugin.tier === 'pro') ? 'pro' : 'free'),
    related: relatedPlugins(plugin.name),
    licenseRequired: plugin.tier === 'pro',
  };
}

function matchesSearch(plugin, q) {
  if (!q) return true;
  const needle = q.toLowerCase();
  return (
    plugin.name.toLowerCase().includes(needle) ||
    (plugin.description || '').toLowerCase().includes(needle) ||
    (plugin.displayName || '').toLowerCase().includes(needle) ||
    (plugin.tags || []).some((tag) => String(tag).toLowerCase().includes(needle))
  );
}

// ---------------------------------------------------------------------------
// Build the marketplace payload
// ---------------------------------------------------------------------------

export function buildMarketplacePayload(plugins, ratings, filters) {
  const { tier, category, bundle, q } = filters;

  let filtered = plugins.slice();
  if (tier === 'free' || tier === 'pro') {
    filtered = filtered.filter((p) => (p.tier || 'free') === tier);
  }
  if (category) {
    filtered = filtered.filter((p) => (p.category || '').toLowerCase() === category.toLowerCase());
  }
  if (bundle && BUNDLES[bundle]) {
    const members = new Set(BUNDLES[bundle].plugins);
    filtered = filtered.filter((p) => members.has(p.name));
  }
  if (q) {
    filtered = filtered.filter((p) => matchesSearch(p, q));
  }

  const cards = filtered.map((p) => buildCard(p, ratings));
  const categorized = categorize(cards);

  const tiersPresent = Array.from(new Set(cards.map((c) => c.tier))).sort();
  const categoriesPresent = Array.from(new Set(cards.map((c) => c.category))).sort();
  const bundlesPresent = Array.from(
    new Set(cards.map((c) => c.bundle).filter(Boolean)),
  ).sort();

  return {
    version: '1.0.0',
    fetchedAt: new Date().toISOString(),
    total: cards.length,
    categories: categorized,
    plugins: cards,
    bundles: Object.entries(BUNDLES).map(([slug, b]) => ({
      slug,
      name: b.name,
      price: b.price,
      plugins: b.plugins,
    })),
    filters: {
      tiers: tiersPresent,
      categories: categoriesPresent,
      bundles: bundlesPresent,
    },
  };
}

// ---------------------------------------------------------------------------
// KV ratings helper — reads a single JSON object keyed by plugin name:
//   { "<plugin>": { average: 4.5, count: 123, reviews: [...] } }
// ---------------------------------------------------------------------------

const KV_RATINGS = 'ratings:all';

export async function readRatings(env) {
  if (!env.PLUGINS_KV) return {};
  try {
    const raw = await env.PLUGINS_KV.get(KV_RATINGS, 'json');
    if (!raw) return {};
    // Accept either the flat map or the timestamp envelope used elsewhere.
    return raw.data || raw;
  } catch {
    return {};
  }
}

export async function writeRating(env, pluginName, stars, review) {
  if (!env.PLUGINS_KV) return { ok: false, error: 'KV not configured' };
  if (!pluginName || typeof stars !== 'number' || stars < 1 || stars > 5) {
    return { ok: false, error: 'pluginName and stars (1-5) required' };
  }
  try {
    const all = await readRatings(env);
    const entry = all[pluginName] || { average: 0, count: 0, reviews: [] };
    const nextCount = entry.count + 1;
    const nextAverage = ((entry.average * entry.count) + stars) / nextCount;
    entry.average = Math.round(nextAverage * 100) / 100;
    entry.count = nextCount;
    if (review && typeof review === 'string') {
      entry.reviews = [
        { stars, review: review.slice(0, 500), at: new Date().toISOString() },
        ...(entry.reviews || []).slice(0, 49),
      ];
    }
    all[pluginName] = entry;
    await env.PLUGINS_KV.put(KV_RATINGS, JSON.stringify(all));
    return { ok: true, rating: entry };
  } catch (err) {
    return { ok: false, error: err.message || 'KV write failed' };
  }
}

// ---------------------------------------------------------------------------
// Request handlers
// ---------------------------------------------------------------------------

export async function handleMarketplace(url, env, fetchPluginsFn) {
  const tier = url.searchParams.get('tier');
  const category = url.searchParams.get('category');
  const bundle = url.searchParams.get('bundle');
  const q = url.searchParams.get('q');

  const plugins = await fetchPluginsFn();
  const ratings = await readRatings(env);

  const payload = buildMarketplacePayload(plugins, ratings, { tier, category, bundle, q });

  return payload;
}

export async function handleRatingPost(request, env) {
  let body;
  try {
    body = await request.json();
  } catch {
    return { ok: false, status: 400, error: 'Invalid JSON body' };
  }
  const { plugin, stars, review } = body || {};
  const result = await writeRating(env, plugin, Number(stars), review);
  return {
    ok: result.ok,
    status: result.ok ? 200 : 400,
    error: result.error,
    rating: result.rating,
  };
}
