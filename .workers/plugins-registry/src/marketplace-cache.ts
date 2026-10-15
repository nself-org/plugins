import { edgeGet, edgePut, edgeDelete } from "./edge-cache.ts";
import type { MarketplaceEnv, MarketplaceResponse } from "./marketplace-types.ts";

const KV_MARKETPLACE_CACHE = "marketplace:enriched";
const MARKETPLACE_CACHE_TTL_MS = 5 * 60 * 1000;

interface MarketplaceCacheEnvelope {
  data: MarketplaceResponse;
  timestamp: number;
}

export async function getCachedMarketplace(
  env: MarketplaceEnv,
): Promise<MarketplaceResponse | null> {
  try {
    const raw = await edgeGet<MarketplaceCacheEnvelope>(KV_MARKETPLACE_CACHE);
    if (!raw) return null;
    if (Date.now() - raw.timestamp < MARKETPLACE_CACHE_TTL_MS) {
      return raw.data;
    }
  } catch {
    // ignore, treat as miss
  }
  return null;
}

export async function setCachedMarketplace(
  env: MarketplaceEnv,
  data: MarketplaceResponse,
): Promise<void> {
  const envelope: MarketplaceCacheEnvelope = { data, timestamp: Date.now() };
  try {
    await edgePut(KV_MARKETPLACE_CACHE, envelope, 300);
  } catch {
    // best-effort cache write
  }
}

export async function invalidateMarketplace(): Promise<void> {
  await edgeDelete(KV_MARKETPLACE_CACHE);
}
