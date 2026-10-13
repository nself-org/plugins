/**
 * legacy-ratings.ts: read-only bridge to the ratings the retired src/index.js stored (P7-PLUG-08).
 *
 * Imports: registry.ts (ratingsKv, Env).
 */

import { ratingsKv } from "./registry.ts";
import type { Env } from "./registry.ts";

// ---------------------------------------------------------------------------
// Legacy ratings. The retired index.js kept every rating in one flat KV record,
// ratings:all = { "<plugin>": { average, count, reviews: [{ stars, review, at }] } }.
// A per-plugin rating:<name> key wins; when it is absent the legacy aggregate is
// served (and becomes the base the next rating is added to), so no rating that
// was ever stored disappears at the deploy. The record is read once a minute.
// ---------------------------------------------------------------------------

interface LegacyRating {
  average?: number;
  count?: number;
  reviews?: Array<{ stars?: number; review?: string; at?: string }>;
}

let legacyMemo: { kv: KVNamespace; at: number; all: Record<string, LegacyRating> } | null = null;

export async function legacyRating(
  env: Env,
  name: string,
): Promise<{ rating: number; reviewCount: number; reviews: Array<{ user: string; rating: number; comment: string; createdAt: string }> } | null> {
  const kv = ratingsKv(env);
  if (!legacyMemo || legacyMemo.kv !== kv || Date.now() - legacyMemo.at > 60_000) {
    let all: Record<string, LegacyRating> = {};
    try {
      const raw = await kv.get<Record<string, unknown>>("ratings:all", "json");
      // flat map, or the timestamp envelope some writers used
      all = ((raw && typeof raw["data"] === "object" && raw["data"] !== null ? raw["data"] : raw) ?? {}) as Record<string, LegacyRating>;
    } catch {
      // unreadable legacy record: treat as empty
    }
    legacyMemo = { kv, at: Date.now(), all };
  }
  const entry = legacyMemo.all[name];
  if (!entry || typeof entry.average !== "number" || typeof entry.count !== "number") return null;
  return {
    rating: entry.average,
    reviewCount: entry.count,
    reviews: (entry.reviews ?? []).map((r) => ({
      user: "legacy",
      rating: r.stars ?? 0,
      comment: r.review ?? "",
      createdAt: r.at ?? "",
    })),
  };
}
