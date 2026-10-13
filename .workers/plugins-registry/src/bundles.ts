/**
 * bundles.ts: GET /bundles.json and GET /bundles-schema.json (P6-E4-W3-S3-T8, ADR-P6-03).
 *
 * Imports: registry.ts (cache-first fetch from the Bundles repo, schema check),
 * http.ts (response helpers).
 */

import { fetchBundlesJson, fetchBundlesSchema, validateBundlesJson } from "./registry.ts";
import type { Env } from "./registry.ts";
import { jsonResponse } from "./http.ts";

// ---------------------------------------------------------------------------
// GET /bundles.json — P6-E4-W3-S3-T8 (ADR-P6-03).
//
// Serves the bundle-to-plugin membership map from the Bundles repo (today; the
// nself-org/bundles repo after the rename lands), cached in PLUGINS_KV under
// its own key (bundles-json-v1, distinct from the registry:* keys) so a
// stale registry cache and a stale bundles.json cache invalidate
// independently. Cache-Control matches the ticket guide's stated convention
// (s-maxage + stale-while-revalidate) rather than this worker's usual plain
// max-age, since bundles.json is fetched by CDN edges more aggressively than
// the per-request registry endpoints.
// ---------------------------------------------------------------------------

const BUNDLES_CACHE_CONTROL = "public, s-maxage=60, stale-while-revalidate=300";

export async function handleBundlesJson(env: Env, ctx: ExecutionContext): Promise<Response> {
  const fetched = await fetchBundlesJson(env, ctx);
  if (fetched === null) {
    return jsonResponse(
      { error: "bundles.json unavailable — upstream fetch failed or GH_ACCESS_TOKEN unset" },
      502,
    );
  }

  const { valid, errors } = validateBundlesJson(fetched.data);
  if (!valid) {
    console.error("bundles.json failed schema validation:", errors.join("; "));
    return jsonResponse(
      { error: "bundles.json failed schema validation", details: errors },
      502,
    );
  }

  return jsonResponse(fetched.data, 200, {
    "Cache-Control": BUNDLES_CACHE_CONTROL,
    "X-Cache": fetched.cache,
  });
}

// ---------------------------------------------------------------------------
// GET /bundles-schema.json — the schema bundles.json validates against.
// Passthrough + cache only; not itself schema-validated.
// ---------------------------------------------------------------------------

export async function handleBundlesSchema(env: Env, ctx: ExecutionContext): Promise<Response> {
  const fetched = await fetchBundlesSchema(env, ctx);
  if (fetched === null) {
    return jsonResponse(
      { error: "bundles-schema.json unavailable — upstream fetch failed or GH_ACCESS_TOKEN unset" },
      502,
    );
  }
  return jsonResponse(fetched.data, 200, {
    "Cache-Control": BUNDLES_CACHE_CONTROL,
    "X-Cache": fetched.cache,
  });
}
