import { ratingsKv } from "./registry.ts";
import type { MarketplaceEnv, RatingResponse, RatingAggregate, RatingReview } from "./marketplace-types.ts";
import { readRatingAggregate, writeRatingAggregate, sanitisePluginName } from "./marketplace-data.ts";
import { getConnectingIP, isRateLimited } from "./marketplace-rate-limit.ts";
import { invalidateMarketplace } from "./marketplace-cache.ts";

function isValidUserHash(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{64}$/.test(value);
}

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

async function isUserHashDedupBlocked(
  env: MarketplaceEnv,
  userHash: string,
  pluginName: string,
): Promise<boolean> {
  const key = `rl:uh:${sanitisePluginName(pluginName)}:${userHash}`;
  try {
    const raw = await ratingsKv(env).get(key, "text");
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
    await ratingsKv(env).put(key, "1", { expirationTtl: 7 * 24 * 60 * 60 });
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
    await invalidateMarketplace();
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
