import type { MarketplacePlugin, MarketplaceResponse } from "./marketplace-types.ts";
import { BUNDLES } from "./marketplace-catalog.ts";

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
const AI_BUNDLE_SLUGS: ReadonlySet<string> = new Set(["claw"]);

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

export function applyFilters(
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
