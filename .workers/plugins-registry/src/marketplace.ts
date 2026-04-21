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
  /** T03: curated plugins are pinned in search results via curated_boost */
  curated?: boolean;
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
// T03: Ranking — curated plugin list (max 5 per spec; schema validates)
// Curated plugins get curated_boost = 20 in the score formula.
// This list is the authoritative source — see .claude/docs/operations/marketplace-curation.md
// ---------------------------------------------------------------------------

const CURATED_PLUGINS: ReadonlySet<string> = new Set(["ai", "claw", "mux", "voice", "browser"]);

// Ranking formula constants
const BUNDLE_BOOST_NONE = 0;
const BUNDLE_BOOST_STD = 15;
const BUNDLE_BOOST_AI = 30;   // nClaw bundle (AI suite) gets max boost
const TIER_BOOST_FREE = 0;
const TIER_BOOST_PRO = 5;
const CURATED_BOOST = 20;
const RECENCY_MAX = 15;
const RECENCY_DECAY_DAYS = 60; // half-life for recency boost

// AI-suite bundle slugs that get the maximum bundle boost
const AI_BUNDLE_SLUGS: ReadonlySet<string> = new Set(["nclaw"]);

function bundleBoost(bundle: string | null): number {
  if (!bundle) return BUNDLE_BOOST_NONE;
  if (AI_BUNDLE_SLUGS.has(bundle)) return BUNDLE_BOOST_AI;
  return BUNDLE_BOOST_STD;
}

function tierBoost(tier: "free" | "pro"): number {
  return tier === "pro" ? TIER_BOOST_PRO : TIER_BOOST_FREE;
}

function recencyBoost(version: string): number {
  // Use version as proxy for age — real updatedAt timestamps would be better
  // but plugin registry doesn't carry them yet. Curated plugins get max recency.
  // Until updatedAt is in the registry, use a neutral middle value.
  return RECENCY_MAX * 0.5;
}

function marketplaceScore(p: MarketplacePlugin): number {
  const bb = bundleBoost(p.bundle);
  const tb = tierBoost(p.tier);
  const cb = p.curated ? CURATED_BOOST : 0;
  const rb = recencyBoost(p.version);
  return bb + tb + cb + rb;
}

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

// Install tracking KV prefix
// Key format: installs:{pluginName} → { count: number, lastUpdated: string }
// Dedup key format: dedup:{instanceId}:{pluginName}:{week} — no value, used for existence check
// Week format: YYYY-Www (ISO week number) for consistent 7-day dedup windows
const KV_INSTALL_PREFIX = "installs:";
const KV_DEDUP_PREFIX = "dedup:";

// T-RATE-01: GET rate limit prefix
// Key format: rl:get:{ip}:{windowMinute} → { count: number }
// Default: 60 req/min per IP. Configurable via MARKETPLACE_GET_RATE_LIMIT env var.
const KV_GET_RL_PREFIX = "rl:get:";
const DEFAULT_GET_RATE_LIMIT = 60;

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
    curated: CURATED_PLUGINS.has(plugin.name),
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
  // Fetch all rating aggregates + install counts in parallel (one per plugin)
  const [aggregates, installCounts] = await Promise.all([
    Promise.all(plugins.map((p) => readRatingAggregate(env, p.name))),
    Promise.all(plugins.map((p) => readInstallCount(env, p.name))),
  ]);

  const cards: MarketplacePlugin[] = plugins.map((p, i) => {
    const agg = aggregates[i] ?? { rating: 0, reviewCount: 0, reviews: [] };
    const card = buildCard(p, agg);
    // Override downloads with real KV count (registry field may be 0 or stale)
    const kvCount = installCounts[i] ?? 0;
    if (kvCount > 0) card.downloads = kvCount;
    return card;
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

// ---------------------------------------------------------------------------
// T03: Diversification — no 3 consecutive same-bundle in first 12 results
// ---------------------------------------------------------------------------

function diversify(plugins: MarketplacePlugin[]): MarketplacePlugin[] {
  const result: MarketplacePlugin[] = [];
  const deferred: MarketplacePlugin[] = [];

  for (const p of plugins) {
    if (result.length >= 12) {
      result.push(p);
      continue;
    }

    // Count trailing consecutive same-bundle in result
    let consecutive = 0;
    for (let i = result.length - 1; i >= 0; i--) {
      if (result[i]?.bundle && result[i]?.bundle === p.bundle) {
        consecutive++;
      } else {
        break;
      }
    }

    if (consecutive >= 2 && p.bundle !== null) {
      deferred.push(p);
    } else {
      result.push(p);
      // Try to fill from deferred
      const idx = deferred.findIndex((d) => {
        const last = result[result.length - 1];
        return !last?.bundle || d.bundle !== last.bundle;
      });
      if (idx !== -1) {
        const fill = deferred.splice(idx, 1)[0];
        if (fill) result.push(fill);
      }
    }
  }

  // Append any remaining deferred items
  return [...result, ...deferred];
}

function applyFilters(
  response: MarketplaceResponse,
  params: {
    tier: string | null;
    bundle: string | null;
    category: string | null;
    q: string | null;
    sort: string | null;
  },
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

  // T03: Sort
  const sort = params.sort ?? "score";
  if (sort === "alpha") {
    filtered = [...filtered].sort((a, b) => a.name.localeCompare(b.name));
  } else if (sort === "newest") {
    // Curated plugins first (proxy for recency until registry carries updatedAt)
    filtered = [...filtered].sort((a, b) => {
      if (a.curated && !b.curated) return -1;
      if (!a.curated && b.curated) return 1;
      return a.name.localeCompare(b.name);
    });
  } else if (sort === "popular") {
    // Popular falls back to score until install counts mature (documented in curation.md)
    filtered = [...filtered].sort((a, b) => {
      const dDiff = (b.downloads ?? 0) - (a.downloads ?? 0);
      if (dDiff !== 0) return dDiff;
      return marketplaceScore(b) - marketplaceScore(a);
    });
  } else {
    // Default: score sort + diversification
    filtered = [...filtered].sort((a, b) => marketplaceScore(b) - marketplaceScore(a));
    filtered = diversify(filtered);
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
  const sort = url.searchParams.get("sort"); // T03: score|alpha|newest|popular

  const isFiltered = Boolean(tier || bundle || category || q || sort);

  // Attempt cache hit for unfiltered requests (no filters AND no sort)
  if (!isFiltered) {
    const cached = await getCachedMarketplace(env);
    if (cached) return cached;
  }

  const full = await buildFullMarketplace(plugins, env);

  // Cache the unfiltered full response
  if (!isFiltered) {
    ctx.waitUntil(setCachedMarketplace(env, full));
  }

  return applyFilters(full, { tier, bundle, category, q, sort });
}

// ---------------------------------------------------------------------------
// T-RATE-01: GET rate limit check — exported for use in index.ts route handlers
// Returns { limited: false } or { limited: true, retryAfter: N }
// ---------------------------------------------------------------------------

export async function checkGetRateLimit(
  request: Request,
  env: MarketplaceEnv,
): Promise<{ limited: boolean; retryAfter: number }> {
  const ip = getConnectingIP(request);
  return isGetRateLimited(env, ip);
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

// ---------------------------------------------------------------------------
// T05: Anti-abuse helpers
// ---------------------------------------------------------------------------

/** Get the real connecting IP from Cloudflare headers (CF-Connecting-IP wins, never X-Forwarded-For). */
function getConnectingIP(request: Request): string {
  // CF-Connecting-IP is set by Cloudflare and cannot be spoofed from outside.
  // Never trust X-Forwarded-For — it is trivially spoofable.
  return request.headers.get("CF-Connecting-IP") ?? "unknown";
}

/** Rate-limit key: ip:<ip>:<windowKey> (POST ratings) */
function ipRateLimitKey(ip: string, windowMinute: string): string {
  return `rl:ip:${ip}:${windowMinute}`;
}

/** Rate-limit key for GET marketplace endpoints: rl:get:<ip>:<windowMinute> */
function getIpRateLimitKey(ip: string, windowMinute: string): string {
  return `${KV_GET_RL_PREFIX}${ip}:${windowMinute}`;
}

/**
 * T-RATE-01: Check + increment IP rate limit for marketplace GET endpoints.
 * Limit: MARKETPLACE_GET_RATE_LIMIT (default 60) requests per 60s window per IP.
 * Returns { limited: true, retryAfter: N } when limit exceeded.
 * Uses RATINGS_KV with 90s TTL to cover current + next minute window overlap.
 */
async function isGetRateLimited(
  env: MarketplaceEnv,
  ip: string,
): Promise<{ limited: boolean; retryAfter: number }> {
  const maxRequests = parseInt(env.MARKETPLACE_GET_RATE_LIMIT ?? String(DEFAULT_GET_RATE_LIMIT), 10);
  const window = currentMinuteWindow();
  const key = getIpRateLimitKey(ip, window);

  let count = 0;
  try {
    const raw = await env.RATINGS_KV.get<{ count: number }>(key, "json");
    count = raw?.count ?? 0;
  } catch {
    return { limited: false, retryAfter: 60 }; // KV read failure — allow through
  }

  if (count >= maxRequests) {
    const now = new Date();
    const nextMinute = new Date(now);
    nextMinute.setSeconds(60 - now.getSeconds(), 0);
    const retryAfter = Math.max(1, Math.ceil((nextMinute.getTime() - now.getTime()) / 1000));
    return { limited: true, retryAfter };
  }

  // Increment counter with 90s TTL (covers window overlap)
  try {
    await env.RATINGS_KV.put(key, JSON.stringify({ count: count + 1 }), { expirationTtl: 90 });
  } catch {
    // Best-effort counter increment — not fatal
  }

  return { limited: false, retryAfter: 0 };
}

/** Current minute window (YYYY-MM-DDTHH:MM) */
function currentMinuteWindow(): string {
  return new Date().toISOString().slice(0, 16); // "2026-04-20T14:23"
}

/**
 * Check + increment IP rate limit (5 POSTs per IP per minute for rating endpoint).
 * Returns true if the request should be rejected (limit exceeded).
 * Uses KV counter with 90s TTL (covers current + next minute window).
 */
async function isRateLimited(env: MarketplaceEnv, ip: string): Promise<{ limited: boolean; retryAfter: number }> {
  const window = currentMinuteWindow();
  const key = ipRateLimitKey(ip, window);

  let count = 0;
  try {
    const raw = await env.RATINGS_KV.get<{ count: number }>(key, "json");
    count = raw?.count ?? 0;
  } catch {
    return { limited: false, retryAfter: 60 }; // KV read failure — allow through
  }

  if (count >= 5) {
    // Calculate seconds until next minute window
    const now = new Date();
    const nextMinute = new Date(now);
    nextMinute.setSeconds(60 - now.getSeconds(), 0);
    const retryAfter = Math.ceil((nextMinute.getTime() - now.getTime()) / 1000);
    return { limited: true, retryAfter };
  }

  // Increment counter with 90s TTL
  try {
    await env.RATINGS_KV.put(key, JSON.stringify({ count: count + 1 }), { expirationTtl: 90 });
  } catch {
    // Best-effort counter increment — not fatal
  }

  return { limited: false, retryAfter: 0 };
}

/**
 * Per-userHash dedup: reject if (userHash, plugin) updated within 7 days.
 * User CAN edit (no restriction on editing), just cannot spam.
 * Returns true if the submission should be rejected as a spam attempt.
 */
async function isUserHashDedupBlocked(
  env: MarketplaceEnv,
  userHash: string,
  pluginName: string,
): Promise<boolean> {
  const key = `rl:uh:${sanitisePluginName(pluginName)}:${userHash}`;
  try {
    const raw = await env.RATINGS_KV.get(key, "text");
    return raw !== null; // exists = submitted within 7 days
  } catch {
    return false; // KV read failure — allow through
  }
}

/**
 * Write per-userHash dedup entry with 7-day TTL after a successful rating.
 */
async function writeUserHashDedup(
  env: MarketplaceEnv,
  userHash: string,
  pluginName: string,
): Promise<void> {
  const key = `rl:uh:${sanitisePluginName(pluginName)}:${userHash}`;
  try {
    await env.RATINGS_KV.put(key, "1", { expirationTtl: 7 * 24 * 60 * 60 });
  } catch {
    // Best-effort
  }
}

export async function handlePostRating(
  pluginName: string,
  request: Request,
  env: MarketplaceEnv,
): Promise<RatingSubmitResult> {
  // T05: IP rate limit — 5 POSTs per IP per minute
  // Uses CF-Connecting-IP (not X-Forwarded-For) to prevent header-spoofing bypass.
  const ip = getConnectingIP(request);
  const { limited, retryAfter } = await isRateLimited(env, ip);
  if (limited) {
    return {
      ok: false,
      status: 429,
      error: `Rate limit exceeded. Maximum 5 rating submissions per minute per IP. Chain message: This limit exists to prevent automated spam submissions and protect the integrity of the plugin rating system. Retry-After: ${retryAfter}s.`,
    };
  }

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

  // T05: Validate comment length cap (500 chars)
  const rawComment = typeof commentValue === "string" ? commentValue : "";
  if (rawComment.length > 500) {
    return {
      ok: false,
      status: 400,
      error: "Comment exceeds the 500-character limit. Please shorten your review.",
    };
  }
  const comment = rawComment;

  // T05: Per-userHash dedup — reject if submitted within 7 days
  // Uses constant-time string comparison via simple KV presence check.
  const dedupBlocked = await isUserHashDedupBlocked(env, userHashValue, pluginName);
  if (dedupBlocked) {
    return {
      ok: false,
      status: 429,
      error: `You have already reviewed this plugin within the last 7 days. Chain message: This limit prevents rating manipulation. You may update your review after 7 days. Retry-After: 604800s.`,
    };
  }

  const existing = await readRatingAggregate(env, pluginName);

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

  // T05: Write per-userHash dedup entry (7-day TTL) after successful submission
  await writeUserHashDedup(env, userHashValue, pluginName);

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

// ---------------------------------------------------------------------------
// GET /plugins/:name — install count helper
// Returns the current install count for a plugin from KV.
// Called by buildFullMarketplace so downloads field reflects real counts.
// ---------------------------------------------------------------------------

async function readInstallCount(
  env: MarketplaceEnv,
  pluginName: string,
): Promise<number> {
  const key = `${KV_INSTALL_PREFIX}${sanitisePluginName(pluginName)}`;
  try {
    const raw = await env.RATINGS_KV.get<{ count: number }>(key, "json");
    return raw?.count ?? 0;
  } catch {
    return 0;
  }
}

// ---------------------------------------------------------------------------
// ISO week number for dedup (YYYY-Www format)
// ---------------------------------------------------------------------------

function isoWeek(date: Date): string {
  // Algorithm per ISO 8601: week starts on Monday
  const d = new Date(Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()));
  const day = d.getUTCDay() || 7; // make Sunday = 7
  d.setUTCDate(d.getUTCDate() + 4 - day);
  const yearStart = new Date(Date.UTC(d.getUTCFullYear(), 0, 1));
  const week = Math.ceil(((d.getTime() - yearStart.getTime()) / 86400000 + 1) / 7);
  return `${d.getUTCFullYear()}-W${String(week).padStart(2, "0")}`;
}

// ---------------------------------------------------------------------------
// POST /plugins/:name/install-event
// Body: { instanceId: string }
// instanceId MUST be an opaque hash (SHA-256 hex, no PII).
// Dedup: (instanceId, pluginName, week) — same install in same week is no-op.
// ---------------------------------------------------------------------------

export interface InstallEventResult {
  ok: boolean;
  status: number;
  error?: string;
  name?: string;
  downloads?: number;
  incremented?: boolean;
}

export async function handleInstallEvent(
  pluginName: string,
  request: Request,
  env: MarketplaceEnv,
): Promise<InstallEventResult> {
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return { ok: false, status: 400, error: "Invalid JSON body" };
  }

  if (body === null || typeof body !== "object" || Array.isArray(body)) {
    return { ok: false, status: 400, error: "Body must be a JSON object" };
  }

  const raw = body as Record<string, unknown>;
  const instanceId = raw["instanceId"];

  // instanceId must be a 64-char hex string (SHA-256, no PII)
  if (typeof instanceId !== "string" || !/^[0-9a-f]{64}$/.test(instanceId)) {
    return {
      ok: false,
      status: 400,
      error: "instanceId must be a 64-character lowercase hex string (SHA-256 hash, no PII)",
    };
  }

  const safePlugin = sanitisePluginName(pluginName);
  if (!safePlugin) {
    return { ok: false, status: 400, error: "Invalid plugin name" };
  }

  const week = isoWeek(new Date());
  const dedupKey = `${KV_DEDUP_PREFIX}${instanceId}:${safePlugin}:${week}`;

  // Check dedup — KV free tier: ~1K writes/day. Using presence check (get returns null on miss).
  try {
    const existing = await env.RATINGS_KV.get(dedupKey, "text");
    if (existing !== null) {
      // Already counted this week — no-op
      const count = await readInstallCount(env, safePlugin);
      return { ok: true, status: 200, name: pluginName, downloads: count, incremented: false };
    }
  } catch {
    // KV read failure — safe to proceed (worst case we count twice, not miss)
  }

  // Increment counter
  const countKey = `${KV_INSTALL_PREFIX}${safePlugin}`;
  let newCount = 1;
  try {
    const current = await env.RATINGS_KV.get<{ count: number }>(countKey, "json");
    newCount = (current?.count ?? 0) + 1;
    await env.RATINGS_KV.put(countKey, JSON.stringify({ count: newCount, lastUpdated: new Date().toISOString() }));
  } catch {
    return { ok: false, status: 500, error: "Failed to update install count" };
  }

  // Write dedup key with 8-day TTL (covers full week + buffer)
  try {
    await env.RATINGS_KV.put(dedupKey, "1", { expirationTtl: 8 * 24 * 60 * 60 });
  } catch {
    // Best-effort dedup — not fatal
  }

  // Invalidate marketplace cache so next fetch reflects updated count
  try {
    await env.PLUGINS_KV.delete(KV_MARKETPLACE_CACHE);
  } catch {
    // Best-effort
  }

  return { ok: true, status: 200, name: pluginName, downloads: newCount, incremented: true };
}
