/**
 * nself Plugin Registry — Marketplace endpoint
 *
 * GET /marketplace[?tier=free|pro][&category=X][&bundle=Y][&q=search-term]
 * GET /marketplace/ratings/:name
 * POST /marketplace/ratings/:name
 *
 * Returns a full marketplace view consumed by Admin UI and web/cloud.
 */

import type { Env, PluginEntry } from "./registry.ts";

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

export interface MarketplacePlugin {
  name: string;
  displayName: string;
  version: string;
  description: string;
  category: string;
  tier: "free" | "pro";
  bundle: string | null;
  author: string;
  icon: string;
  tags: string[];
  downloads: number;
  rating: number;
  reviewCount: number;
  licenseRequired: boolean;
  price: string | null;
  related: string[];
  homepage: string | null;
  repository: string | null;
}

export interface BundleInfo {
  slug: string;
  name: string;
  price: string;
  plugins: string[];
  description: string;
}

export interface MarketplaceResponse {
  plugins: MarketplacePlugin[];
  bundles: BundleInfo[];
  categories: string[];
  stats: { total: number; free: number; pro: number; updatedAt: string };
}

export interface RatingAggregate {
  rating: number;
  reviewCount: number;
  reviews: RatingReview[];
}

export interface RatingReview {
  user: string;
  rating: number;
  comment: string;
  createdAt: string;
}

export interface RatingResponse {
  name: string;
  rating: number;
  reviewCount: number;
  reviews: RatingReview[];
}

// ---------------------------------------------------------------------------
// Bundle data — canonical from SPORT F06-BUNDLE-INVENTORY
// ---------------------------------------------------------------------------

interface BundleDefinition {
  name: string;
  price: string;
  plugins: string[];
  description: string;
}

const BUNDLES: Record<string, BundleDefinition> = {
  nclaw: {
    name: "ɳClaw Bundle",
    price: "$0.99/mo",
    plugins: ["ai", "claw", "claw-web", "mux", "voice", "browser", "google", "notify", "cron"],
    description: "Full AI assistant: email, calendar, news, budget, tools",
  },
  clawde: {
    name: "ClawDE+ Bundle",
    price: "$1.99/mo",
    plugins: ["realtime", "auth", "cms", "notify"],
    description: "Cloud sync, premium models, team features",
  },
  ntv: {
    name: "nTV Bundle",
    price: "$0.99/mo",
    plugins: ["media-processing", "streaming", "epg", "tmdb", "torrent-manager", "content-acquisition"],
    description: "Media downloading, encoding, streaming, metadata",
  },
  nfamily: {
    name: "nFamily Bundle",
    price: "$0.99/mo",
    plugins: ["social", "photos", "activity-feed", "moderation", "realtime", "cms", "chat"],
    description: "Private family social, photo sharing, activity feeds",
  },
  nchat: {
    name: "nChat Bundle",
    price: "$0.99/mo",
    plugins: ["chat", "livekit", "recording", "moderation", "bots", "realtime", "auth"],
    description: "Messaging with video calls, bots, moderation",
  },
};

// ---------------------------------------------------------------------------
// Category default icons
// ---------------------------------------------------------------------------

const CATEGORY_ICONS: Record<string, string> = {
  authentication: "🔐",
  automation: "⚙️",
  commerce: "🛒",
  communication: "💬",
  content: "📝",
  data: "🗄️",
  development: "🔧",
  infrastructure: "🏗️",
  integrations: "🔌",
  media: "🎬",
  streaming: "📡",
  ai: "🤖",
  compliance: "✅",
};

// ---------------------------------------------------------------------------
// KV key constants
// ---------------------------------------------------------------------------

const KV_MARKETPLACE_CACHE = "marketplace:enriched";
const MARKETPLACE_CACHE_TTL_MS = 5 * 60 * 1000; // 5 minutes

// ---------------------------------------------------------------------------
// MarketplaceEnv — alias for Env (RATINGS_KV is defined there)
// ---------------------------------------------------------------------------

export type MarketplaceEnv = Env;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function bundleForPlugin(pluginName: string): { slug: string; def: BundleDefinition } | null {
  for (const [slug, def] of Object.entries(BUNDLES)) {
    if (def.plugins.includes(pluginName)) {
      return { slug, def };
    }
  }
  return null;
}

function toDisplayName(plugin: PluginEntry): string {
  if (plugin.displayName) return plugin.displayName;
  return (plugin.name ?? "")
    .split(/[-_]/)
    .map((w: string) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}

function categoryIcon(category: string): string {
  return CATEGORY_ICONS[category.toLowerCase()] ?? "🔌";
}

function normaliseTier(tier: string): "free" | "pro" {
  return tier === "pro" ? "pro" : "free";
}

/** Validate a userHash — must be a 64-char lowercase hex string (SHA-256). */
function isValidUserHash(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{64}$/.test(value);
}

/** Sanitise a plugin name for use as a KV key component. Only alphanumeric + hyphens. */
function sanitisePluginName(name: string): string {
  return name.replace(/[^a-z0-9-]/gi, "").slice(0, 64);
}

// ---------------------------------------------------------------------------
// Ratings KV helpers
// ---------------------------------------------------------------------------

async function readRatingAggregate(
  env: MarketplaceEnv,
  pluginName: string,
): Promise<RatingAggregate> {
  const key = `rating:${sanitisePluginName(pluginName)}`;
  try {
    const raw = await env.RATINGS_KV.get<RatingAggregate>(key, "json");
    if (raw) return raw;
  } catch {
    // fall through to default
  }
  return { rating: 0, reviewCount: 0, reviews: [] };
}

async function writeRatingAggregate(
  env: MarketplaceEnv,
  pluginName: string,
  aggregate: RatingAggregate,
): Promise<void> {
  const key = `rating:${sanitisePluginName(pluginName)}`;
  await env.RATINGS_KV.put(key, JSON.stringify(aggregate));
}

// ---------------------------------------------------------------------------
// Build a MarketplacePlugin card
// ---------------------------------------------------------------------------

function buildCard(plugin: PluginEntry, aggregate: RatingAggregate): MarketplacePlugin {
  const bundleMatch = bundleForPlugin(plugin.name);
  const tier = normaliseTier(plugin.tier);

  let price: string | null = null;
  if (bundleMatch) {
    price = bundleMatch.def.price;
  } else if (tier === "pro") {
    price = "$0.99/mo";
  }

  const rawCategory = (plugin.category ?? "other").toLowerCase();
  const icon = categoryIcon(rawCategory);

  return {
    name: plugin.name,
    displayName: toDisplayName(plugin),
    version: plugin.version ?? "0.0.0",
    description: plugin.description ?? "",
    category: rawCategory,
    tier,
    bundle: bundleMatch ? bundleMatch.slug : null,
    author: plugin.author ?? "nself",
    icon,
    tags: Array.isArray(plugin.tags) ? plugin.tags : [],
    downloads: plugin.downloads ?? 0,
    rating: aggregate.rating,
    reviewCount: aggregate.reviewCount,
    licenseRequired: tier === "pro",
    price,
    related: bundleMatch
      ? bundleMatch.def.plugins.filter((n) => n !== plugin.name)
      : [],
    homepage: plugin.homepage ?? null,
    repository: plugin.repository ?? null,
  };
}

// ---------------------------------------------------------------------------
// Build bundle list
// ---------------------------------------------------------------------------

function buildBundles(): BundleInfo[] {
  return Object.entries(BUNDLES).map(([slug, def]) => ({
    slug,
    name: def.name,
    price: def.price,
    plugins: def.plugins,
    description: def.description,
  }));
}

// ---------------------------------------------------------------------------
// Marketplace cache (5-minute TTL in KV)
// ---------------------------------------------------------------------------

interface MarketplaceCacheEnvelope {
  data: MarketplaceResponse;
  timestamp: number;
}

async function getCachedMarketplace(
  env: MarketplaceEnv,
): Promise<MarketplaceResponse | null> {
  try {
    const raw = await env.PLUGINS_KV.get<MarketplaceCacheEnvelope>(KV_MARKETPLACE_CACHE, "json");
    if (!raw) return null;
    if (Date.now() - raw.timestamp < MARKETPLACE_CACHE_TTL_MS) {
      return raw.data;
    }
  } catch {
    // ignore, treat as miss
  }
  return null;
}

async function setCachedMarketplace(
  env: MarketplaceEnv,
  data: MarketplaceResponse,
): Promise<void> {
  const envelope: MarketplaceCacheEnvelope = { data, timestamp: Date.now() };
  try {
    await env.PLUGINS_KV.put(KV_MARKETPLACE_CACHE, JSON.stringify(envelope), {
      expirationTtl: 300,
    });
  } catch {
    // best-effort cache write
  }
}

// ---------------------------------------------------------------------------
// Build full MarketplaceResponse (unfiltered — for cache)
// ---------------------------------------------------------------------------

async function buildFullMarketplace(
  plugins: PluginEntry[],
  env: MarketplaceEnv,
): Promise<MarketplaceResponse> {
  // Fetch all rating aggregates in parallel (one per plugin)
  const aggregates = await Promise.all(
    plugins.map((p) => readRatingAggregate(env, p.name)),
  );

  const cards: MarketplacePlugin[] = plugins.map((p, i) => {
    const agg = aggregates[i] ?? { rating: 0, reviewCount: 0, reviews: [] };
    return buildCard(p, agg);
  });

  const freeCount = cards.filter((c) => c.tier === "free").length;
  const proCount = cards.filter((c) => c.tier === "pro").length;

  const categorySet = new Set(cards.map((c) => c.category));
  const categories = Array.from(categorySet).sort();

  return {
    plugins: cards,
    bundles: buildBundles(),
    categories,
    stats: {
      total: cards.length,
      free: freeCount,
      pro: proCount,
      updatedAt: new Date().toISOString(),
    },
  };
}

// ---------------------------------------------------------------------------
// Apply query filters to a MarketplaceResponse
// ---------------------------------------------------------------------------

function applyFilters(
  response: MarketplaceResponse,
  params: { tier: string | null; bundle: string | null; category: string | null; q: string | null },
): MarketplaceResponse {
  let filtered = response.plugins;

  if (params.tier === "free" || params.tier === "pro") {
    filtered = filtered.filter((p) => p.tier === params.tier);
  }
  if (params.bundle && params.bundle in BUNDLES) {
    const members = new Set(BUNDLES[params.bundle]?.plugins ?? []);
    filtered = filtered.filter((p) => members.has(p.name));
  }
  if (params.category) {
    const cat = params.category.toLowerCase();
    filtered = filtered.filter((p) => p.category === cat);
  }
  if (params.q) {
    const needle = params.q.toLowerCase();
    filtered = filtered.filter(
      (p) =>
        p.name.toLowerCase().includes(needle) ||
        p.displayName.toLowerCase().includes(needle) ||
        p.description.toLowerCase().includes(needle) ||
        p.tags.some((t) => t.toLowerCase().includes(needle)),
    );
  }

  const categorySet = new Set(filtered.map((p) => p.category));

  return {
    plugins: filtered,
    bundles: response.bundles,
    categories: Array.from(categorySet).sort(),
    stats: {
      total: filtered.length,
      free: filtered.filter((p) => p.tier === "free").length,
      pro: filtered.filter((p) => p.tier === "pro").length,
      updatedAt: response.stats.updatedAt,
    },
  };
}

// ---------------------------------------------------------------------------
// GET /marketplace
// ---------------------------------------------------------------------------

export async function handleMarketplace(
  url: URL,
  env: MarketplaceEnv,
  plugins: PluginEntry[],
  ctx: ExecutionContext,
): Promise<MarketplaceResponse> {
  const tier = url.searchParams.get("tier");
  const bundle = url.searchParams.get("bundle");
  const category = url.searchParams.get("category");
  const q = url.searchParams.get("q");

  const isFiltered = Boolean(tier || bundle || category || q);

  // Attempt cache hit for unfiltered requests
  if (!isFiltered) {
    const cached = await getCachedMarketplace(env);
    if (cached) return cached;
  }

  const full = await buildFullMarketplace(plugins, env);

  // Cache the unfiltered full response
  if (!isFiltered) {
    ctx.waitUntil(setCachedMarketplace(env, full));
  }

  return applyFilters(full, { tier, bundle, category, q });
}

// ---------------------------------------------------------------------------
// GET /marketplace/ratings/:name
// ---------------------------------------------------------------------------

export async function handleGetRating(
  pluginName: string,
  env: MarketplaceEnv,
): Promise<RatingResponse> {
  const aggregate = await readRatingAggregate(env, pluginName);
  return {
    name: pluginName,
    rating: aggregate.rating,
    reviewCount: aggregate.reviewCount,
    reviews: aggregate.reviews,
  };
}

// ---------------------------------------------------------------------------
// POST /marketplace/ratings/:name
// Body: { rating: 1-5, comment?: string, userHash: string }
// ---------------------------------------------------------------------------

export interface RatingSubmitResult {
  ok: boolean;
  status: number;
  error?: string;
  name?: string;
  rating?: number;
  reviewCount?: number;
}

export async function handlePostRating(
  pluginName: string,
  request: Request,
  env: MarketplaceEnv,
): Promise<RatingSubmitResult> {
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return { ok: false, status: 400, error: "Invalid JSON body" };
  }

  if (
    body === null ||
    typeof body !== "object" ||
    Array.isArray(body)
  ) {
    return { ok: false, status: 400, error: "Body must be a JSON object" };
  }

  const raw = body as Record<string, unknown>;

  const ratingValue = raw["rating"];
  const commentValue = raw["comment"];
  const userHashValue = raw["userHash"];

  // Validate rating
  if (
    typeof ratingValue !== "number" ||
    !Number.isInteger(ratingValue) ||
    ratingValue < 1 ||
    ratingValue > 5
  ) {
    return { ok: false, status: 400, error: "rating must be an integer 1-5" };
  }

  // Validate userHash — required, must be 64-char lowercase hex (SHA-256)
  if (!isValidUserHash(userHashValue)) {
    return {
      ok: false,
      status: 400,
      error: "userHash must be a 64-character lowercase hex string (SHA-256 of license key)",
    };
  }

  const comment =
    typeof commentValue === "string" ? commentValue.slice(0, 500) : "";

  const existing = await readRatingAggregate(env, pluginName);

  // Check for duplicate submission from this userHash
  const isDuplicate = existing.reviews.some((r) => r.user === userHashValue);
  if (isDuplicate) {
    return {
      ok: false,
      status: 409,
      error: "A rating from this user already exists for this plugin",
    };
  }

  // Update aggregate
  const nextCount = existing.reviewCount + 1;
  const nextRating =
    Math.round(
      ((existing.rating * existing.reviewCount + ratingValue) / nextCount) * 100,
    ) / 100;

  const newReview: RatingReview = {
    user: userHashValue,
    rating: ratingValue,
    comment,
    createdAt: new Date().toISOString(),
  };

  const updatedReviews: RatingReview[] = [newReview, ...existing.reviews].slice(0, 100);

  const updated: RatingAggregate = {
    rating: nextRating,
    reviewCount: nextCount,
    reviews: updatedReviews,
  };

  try {
    await writeRatingAggregate(env, pluginName, updated);
  } catch {
    return { ok: false, status: 500, error: "Failed to persist rating" };
  }

  // Invalidate marketplace cache so next fetch reflects new rating
  try {
    await env.PLUGINS_KV.delete(KV_MARKETPLACE_CACHE);
  } catch {
    // best-effort
  }

  return {
    ok: true,
    status: 200,
    name: pluginName,
    rating: updated.rating,
    reviewCount: updated.reviewCount,
  };
}
