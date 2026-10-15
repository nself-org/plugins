/** Bundles document fetching and structural validation. */
import { edgeGetWrapped, edgePutWrapped } from "./edge-cache.ts";
import { cacheTtl, isFresh, decodeGitHubContent } from "./registry-types.ts";
import type { Env, GitHubContentsResponse } from "./registry-types.ts";

const BUNDLES_JSON_API_URL =
  "https://api.github.com/repos/nself-org/bundles/contents/bundles.json";
const BUNDLES_SCHEMA_API_URL =
  "https://api.github.com/repos/nself-org/bundles/contents/bundles-schema.json";

export const KV_BUNDLES_JSON = "bundles-json-v1";
export const KV_BUNDLES_SCHEMA = "bundles-schema-v1";

// Canonical bundle slugs, in ordering-canon order (nSelf PPI "Ordering" rule:
// task → chat → claw → family → sentry → clawde). The TV Bundle was retired
// 2026-08-31 (owner directive) and bundles-schema.json's own propertyNames
// enum already excludes it — 6 slugs, not 7.
export const CANONICAL_BUNDLE_SLUGS = [
  "task",
  "chat",
  "claw",
  "family",
  "sentry",
  "clawde",
] as const;

export interface BundlesJsonFile {
  schema_version: string;
  bundles: Record<string, unknown>;
}

/**
 * Structural validation of a fetched bundles.json body against the shape
 * bundles-schema.json requires, without pulling in a JSON-Schema library
 * (this worker is dependency-light by design). Checks exactly the
 * constraints the schema encodes: schema_version present, bundles keyed by
 * exactly the 6 canonical slugs, and each bundle entry carries its required
 * fields.
 */
export function validateBundlesJson(data: unknown): { valid: boolean; errors: string[] } {
  const errors: string[] = [];

  if (typeof data !== "object" || data === null) {
    return { valid: false, errors: ["bundles.json is not an object"] };
  }
  const file = data as Partial<BundlesJsonFile>;

  if (typeof file.schema_version !== "string" || !/^\d+\.\d+\.\d+$/.test(file.schema_version)) {
    errors.push("schema_version missing or not a semver string");
  }
  if (typeof file.bundles !== "object" || file.bundles === null || Array.isArray(file.bundles)) {
    errors.push("bundles field missing or not an object");
    return { valid: false, errors };
  }

  const keys = Object.keys(file.bundles);
  const unexpected = keys.filter((k) => !(CANONICAL_BUNDLE_SLUGS as readonly string[]).includes(k));
  const missing = CANONICAL_BUNDLE_SLUGS.filter((k) => !keys.includes(k));
  if (unexpected.length > 0) errors.push(`unexpected bundle slug(s): ${unexpected.join(", ")}`);
  if (missing.length > 0) errors.push(`missing bundle slug(s): ${missing.join(", ")}`);

  const requiredFields = ["display", "tier", "price_monthly", "price_yearly", "saas", "page", "plugins"];
  for (const [slug, entry] of Object.entries(file.bundles)) {
    if (typeof entry !== "object" || entry === null) {
      errors.push(`bundle "${slug}" is not an object`);
      continue;
    }
    const rec = entry as Record<string, unknown>;
    for (const field of requiredFields) {
      if (!(field in rec)) errors.push(`bundle "${slug}" missing required field "${field}"`);
    }
    if ("tier" in rec && rec.tier !== "free" && rec.tier !== "paid") {
      errors.push(`bundle "${slug}" has invalid tier "${String(rec.tier)}"`);
    }
    if ("plugins" in rec && !Array.isArray(rec.plugins)) {
      errors.push(`bundle "${slug}" plugins field is not an array`);
    }
  }

  return { valid: errors.length === 0, errors };
}

/** A fetched JSON document and whether it came from the edge cache. */
export interface CachedJson {
  data: unknown;
  cache: "HIT" | "MISS";
}

/**
 * Cache-first fetch of one JSON file from the Bundles repo through the GitHub
 * Contents API (authenticated; base64 body). The edge copy is served while fresh
 * with no token, as the retired index.js did. Returns null on fetch/decode
 * failure. Callers validate the body separately so a schema failure reads
 * differently from a fetch failure.
 */
async function fetchBundlesRepoJson(
  env: Env,
  ctx: ExecutionContext,
  url: string,
  kvKey: string,
  bypass: boolean,
): Promise<CachedJson | null> {

  if (!bypass) {
    const cached = await edgeGetWrapped<unknown>(kvKey);
    if (cached && isFresh(cached, cacheTtl(env))) {
      return { data: cached.data, cache: "HIT" };
    }
  }

  if (!env.GH_ACCESS_TOKEN) {
    console.warn(`GH_ACCESS_TOKEN not set — cannot fetch ${url}`);
    return null;
  }

  const resp = await fetch(url, {
    headers: {
      Authorization: `token ${env.GH_ACCESS_TOKEN}`,
      Accept: "application/vnd.github.v3+json",
      "User-Agent": "nself-plugin-registry/2.0",
    },
  });

  if (!resp.ok) {
    const body = await resp.text().catch(() => "");
    console.error(`Fetch failed for ${url}: ${resp.status} — ${body.slice(0, 200)}`);
    return null;
  }

  let data: unknown;
  try {
    const envelope = (await resp.json()) as GitHubContentsResponse;
    data = decodeGitHubContent(envelope.content);
  } catch (e) {
    console.error(`Failed to decode content for ${url}:`, (e as Error).message);
    return null;
  }

  ctx.waitUntil(edgePutWrapped(kvKey, data));
  return { data, cache: "MISS" };
}

export function fetchBundlesJson(
  env: Env,
  ctx: ExecutionContext,
  bypass = false,
): Promise<CachedJson | null> {
  return fetchBundlesRepoJson(env, ctx, BUNDLES_JSON_API_URL, KV_BUNDLES_JSON, bypass);
}

/** bundles-schema.json: same repo and auth, its own KV key, served as-is. */
export function fetchBundlesSchema(
  env: Env,
  ctx: ExecutionContext,
  bypass = false,
): Promise<CachedJson | null> {
  return fetchBundlesRepoJson(env, ctx, BUNDLES_SCHEMA_API_URL, KV_BUNDLES_SCHEMA, bypass);
}
