import type { PluginEntry } from "./registry.ts";
import type { MarketplaceEnv, MarketplacePlugin, MarketplaceResponse } from "./marketplace-types.ts";
import { buildCard, buildBundles } from "./marketplace-catalog.ts";
import { readRatingAggregate, readInstallCount } from "./marketplace-data.ts";
import { applyFilters } from "./marketplace-filters.ts";
import { getCachedMarketplace, setCachedMarketplace } from "./marketplace-cache.ts";

export type * from "./marketplace-types.ts";
export { checkGetRateLimit } from "./marketplace-rate-limit.ts";
export { handleGetRating, handlePostRating, type RatingSubmitResult } from "./marketplace-ratings.ts";
export { handleInstallEvent, type InstallEventResult } from "./marketplace-installs.ts";

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
