/**
 * marketplace.ts — Public plugin marketplace discovery endpoint (TypeScript).
 *
 * Spec: S43-T06 (P93 Wave 5). Browse is fully open: no license key, no auth
 * header, no cookie. Anyone can GET the marketplace to compare tiers, read
 * bundle pricing, and drill into a plugin's details.
 *
 * Install is gated separately by the CLI via /license/validate + signed
 * tarball download (see S43-T07). Discovery stays open so prospective users
 * can browse before buying.
 *
 * Endpoints covered:
 *   GET /marketplace                       — full catalog (all tiers)
 *   GET /marketplace?tier=free|pro         — filter by tier
 *   GET /marketplace?category=<name>       — filter by category
 *   GET /marketplace?bundle=<slug>         — filter by bundle membership
 *   GET /marketplace?q=<search>            — free-text search
 *   GET /marketplace/plugin/:name          — single plugin card
 *
 * The implementation delegates to the existing marketplace.js module for
 * catalog assembly; this TypeScript layer enforces "no auth required" and
 * returns consistent CORS + cache headers.
 */

export interface MarketplaceEnv {
  // KV + secrets reused from the main worker — optional on purpose so this
  // module can be loaded standalone for testing.
  PLUGINS_KV?: KVNamespace;
  GH_ACCESS_TOKEN?: string;
  SIGNING_PUBLIC_KEY?: string;
}

// Minimal KVNamespace shim for type-checking outside a Workers context.
interface KVNamespace {
  get(key: string, options?: { type?: 'text' | 'json' }): Promise<unknown>;
  put(
    key: string,
    value: string,
    options?: { expirationTtl?: number },
  ): Promise<void>;
}

export interface MarketplaceQuery {
  tier?: 'free' | 'pro' | 'all';
  category?: string;
  bundle?: string;
  q?: string;
}

export interface PluginCard {
  name: string;
  displayName: string;
  version: string;
  description: string;
  tier: string;
  category: string;
  author?: string;
  homepage?: string;
  repository?: string;
  tags: string[];
  bundle?: string;
  price: string;
  licenseRequired: boolean;
  rating?: number;
  downloads?: number;
  related: string[];
}

export interface MarketplaceResponse {
  version: string;
  fetchedAt: string;
  total: number;
  categories: Array<{ name: string; slug: string; count: number; plugins: PluginCard[] }>;
  plugins: PluginCard[];
  bundles: Array<{ slug: string; name: string; price: string; plugins: string[] }>;
  filters: { tiers: string[]; categories: string[]; bundles: string[] };
  public: true;
  authRequired: false;
}

// ---------------------------------------------------------------------------
// Header builders — every marketplace response is public + CDN-cacheable.
// ---------------------------------------------------------------------------

const PUBLIC_HEADERS: Record<string, string> = {
  'Content-Type':                'application/json; charset=utf-8',
  'Cache-Control':               'public, max-age=120, s-maxage=300',
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type',
  'X-Marketplace-Public':        'true',
  'X-Marketplace-Auth':          'none',
};

// ---------------------------------------------------------------------------
// Parse query params from a Request URL.
// ---------------------------------------------------------------------------

export function parseMarketplaceQuery(url: URL): MarketplaceQuery {
  const tier = url.searchParams.get('tier');
  const category = url.searchParams.get('category');
  const bundle = url.searchParams.get('bundle');
  const q = url.searchParams.get('q');
  const out: MarketplaceQuery = {};
  if (tier === 'free' || tier === 'pro' || tier === 'all') out.tier = tier;
  if (category) out.category = category;
  if (bundle) out.bundle = bundle;
  if (q) out.q = q;
  return out;
}

// ---------------------------------------------------------------------------
// Filter helpers.
// ---------------------------------------------------------------------------

export function filterCards(
  cards: PluginCard[],
  query: MarketplaceQuery,
): PluginCard[] {
  let out = cards;
  if (query.tier && query.tier !== 'all') {
    out = out.filter((c) => c.tier === query.tier);
  }
  if (query.category) {
    out = out.filter((c) => c.category === query.category);
  }
  if (query.bundle) {
    out = out.filter((c) => c.bundle === query.bundle);
  }
  if (query.q) {
    const needle = query.q.toLowerCase();
    out = out.filter(
      (c) =>
        c.name.toLowerCase().includes(needle) ||
        c.description.toLowerCase().includes(needle) ||
        c.tags.some((t) => t.toLowerCase().includes(needle)),
    );
  }
  return out;
}

// ---------------------------------------------------------------------------
// Public handler. NO license key check. NO auth header check. Always open.
// ---------------------------------------------------------------------------

/**
 * handleMarketplaceRequest is the public Worker entry. It builds a response
 * from the supplied full plugin catalog (the main worker passes in the already-
 * assembled catalog so this module stays testable without fetching).
 */
export function handleMarketplaceRequest(
  request: Request,
  catalog: MarketplaceResponse,
): Response {
  // CORS preflight — always allowed.
  if (request.method === 'OPTIONS') {
    return new Response(null, { status: 204, headers: PUBLIC_HEADERS });
  }

  if (request.method !== 'GET') {
    return new Response(
      JSON.stringify({ error: 'Only GET is supported on /marketplace' }),
      { status: 405, headers: PUBLIC_HEADERS },
    );
  }

  const url = new URL(request.url);
  const query = parseMarketplaceQuery(url);

  const filteredPlugins = filterCards(catalog.plugins, query);
  const filteredCategories = catalog.categories
    .map((cat) => {
      const plugins = filterCards(cat.plugins, query);
      return { ...cat, plugins, count: plugins.length };
    })
    .filter((cat) => cat.count > 0);

  const response: MarketplaceResponse = {
    ...catalog,
    total: filteredPlugins.length,
    plugins: filteredPlugins,
    categories: filteredCategories,
    public: true,
    authRequired: false,
  };

  return new Response(JSON.stringify(response, null, 2), {
    status: 200,
    headers: PUBLIC_HEADERS,
  });
}

/**
 * assertPublic is a runtime guard called by the main worker right before
 * dispatching the marketplace request. It short-circuits any accidentally-
 * added auth middleware and returns true when the request can proceed.
 */
export function assertPublic(request: Request): boolean {
  // Never reject a marketplace request for lack of credentials.
  // Presence of Authorization / Cookie is acceptable but never required.
  const method = request.method;
  return method === 'GET' || method === 'HEAD' || method === 'OPTIONS';
}
