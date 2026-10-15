import { ratingsKv } from "./registry.ts";
import { legacyRating } from "./legacy-ratings.ts";
import type { MarketplaceEnv, RatingAggregate } from "./marketplace-types.ts";

const KV_INSTALL_PREFIX = "installs:";

export function sanitisePluginName(name: string): string {
  return name.replace(/[^a-z0-9-]/gi, "").slice(0, 64);
}

// ---------------------------------------------------------------------------
// Ratings KV helpers
// ---------------------------------------------------------------------------

export async function readRatingAggregate(
  env: MarketplaceEnv,
  pluginName: string,
): Promise<RatingAggregate> {
  const key = `rating:${sanitisePluginName(pluginName)}`;
  try {
    const raw = await ratingsKv(env).get<RatingAggregate>(key, "json");
    if (raw) return raw;
  } catch {
    // fall through to the legacy record, then the default
  }
  return (await legacyRating(env, pluginName)) ?? { rating: 0, reviewCount: 0, reviews: [] };
}

export async function writeRatingAggregate(
  env: MarketplaceEnv,
  pluginName: string,
  aggregate: RatingAggregate,
): Promise<void> {
  const key = `rating:${sanitisePluginName(pluginName)}`;
  await ratingsKv(env).put(key, JSON.stringify(aggregate));
}

export async function readInstallCount(
  env: MarketplaceEnv,
  pluginName: string,
): Promise<number> {
  const key = `${KV_INSTALL_PREFIX}${sanitisePluginName(pluginName)}`;
  try {
    const raw = await ratingsKv(env).get<{ count: number }>(key, "json");
    return raw?.count ?? 0;
  } catch {
    return 0;
  }
}
