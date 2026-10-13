/**
 * registry.ts — Plugin data types and static fallback registry loader
 *
 * Types shared across all worker modules. The static fallback is a minimal
 * seed that keeps the worker functional if KV is empty and the GitHub fetch
 * fails during cold-start or network partition.
 */

// ---------------------------------------------------------------------------
// Core data types
// ---------------------------------------------------------------------------

export type PluginTier = "free" | "pro" | "max";

export interface PluginEntry {
  name: string;
  version: string;
  tier: PluginTier;
  description: string;
  category: string;
  license: string;
  tarballURL: string;
  checksum?: string;
  signature?: string;
  author?: string;
  homepage?: string;
  repository?: string;
  tags?: string[];
  downloads?: number;
  displayName?: string;
}

export interface PluginListResponse {
  plugins: PluginEntry[];
  total: number;
  free: number;
  pro: number;
  generatedAt: string;
}

export interface RevocationEntry {
  name: string;
  version: string;
  revokedAt: string;
  reason?: string;
}

export interface RevocationListResponse {
  revoked: RevocationEntry[];
  fetchedAt: string;
  count: number;
}

// RevokedAuthorEntry — one entry in the author Certificate Revocation List. (S58-T09)
// CLI checks this list on every plugin install and daily via cron.
// Append-only: once an author is revoked, the entry must never be deleted.
export interface RevokedAuthorEntry {
  /** Plugin author identifier (matches plugin.json "author" field). */
  authorKey: string;
  /** ISO 8601 datetime of revocation. */
  revokedAt: string;
  /** Optional human-readable reason for revocation. */
  reason?: string;
}

export interface RevokedAuthorListResponse {
  revokedAuthors: RevokedAuthorEntry[];
  fetchedAt: string;
  count: number;
}

// ---------------------------------------------------------------------------
// Worker environment bindings
// ---------------------------------------------------------------------------

export interface Env {
  REGISTRY: KVNamespace;
  PLUGINS_KV: KVNamespace;
  /** Optional dedicated ratings namespace; falls back to PLUGINS_KV (see ratingsKv). */
  RATINGS_KV?: KVNamespace;
  SIGNING_PRIVATE_KEY: string;
  PUBLIC_KEY_HEX: string;
  /** Legacy name of PUBLIC_KEY_HEX read by the retired src/index.js; accepted as an alias. */
  SIGNING_PUBLIC_KEY?: string;
  PLUGIN_REGISTRY_VERSION: string;
  CACHE_TTL?: string;
  REGISTRY_VERSION?: string;
  GH_ACCESS_TOKEN?: string;
  GITHUB_SYNC_TOKEN?: string;
  // S67-T03: R2 plugin tarball CDN (primary; GitHub Releases = fallback on 5xx)
  PLUGIN_TARBALLS?: R2Bucket;
  R2_PUBLIC_DOMAIN?: string; // e.g. "pub-abc123.r2.dev" or custom domain
  // T-RATE-01: GET rate limit config for marketplace endpoints
  MARKETPLACE_GET_RATE_LIMIT?: string;    // default "60" req/min
  MARKETPLACE_GET_RATE_WINDOW_MS?: string; // default "60000" ms
}

// ---------------------------------------------------------------------------
// KV envelope — all values stored as { data, timestamp }
// ---------------------------------------------------------------------------

export interface KVEnvelope<T> {
  data: T;
  timestamp: number;
}

// ---------------------------------------------------------------------------
// GitHub Contents API response
// ---------------------------------------------------------------------------

export interface GitHubContentsResponse {
  content: string;
  encoding: string;
  name: string;
}

// ---------------------------------------------------------------------------
// Remote registry wire format (free registry uses object, pro uses array)
// ---------------------------------------------------------------------------

interface FreeRegistryWireFormat {
  plugins: Record<string, Omit<PluginEntry, "tier"> & { tier?: PluginTier }>;
  version?: string;
  lastUpdated?: string;
}

interface ProRegistryWireFormat {
  plugins: Array<Omit<PluginEntry, "tier"> & { tier?: PluginTier }>;
  version?: string;
}

type RegistryWireFormat = FreeRegistryWireFormat | ProRegistryWireFormat | PluginEntry[];

// ---------------------------------------------------------------------------
// Normalise various wire formats to a typed flat array
// ---------------------------------------------------------------------------

function normaliseToArray(data: RegistryWireFormat, expectedTier: PluginTier): PluginEntry[] {
  let raw: Array<Omit<PluginEntry, "tier"> & { tier?: PluginTier }>;

  if (Array.isArray(data)) {
    raw = data;
  } else if ("plugins" in data && Array.isArray(data.plugins)) {
    raw = data.plugins;
  } else if ("plugins" in data && typeof data.plugins === "object" && data.plugins !== null) {
    // Use Object.entries so the dict key becomes the fallback name when the entry
    // omits the "name" field (the Bundles repo registry.json uses this dict format).
    raw = Object.entries(data.plugins as Record<string, Omit<PluginEntry, "tier"> & { tier?: PluginTier }>)
      .map(([key, entry]) => (entry.name ? entry : { ...entry, name: key }));
  } else {
    return [];
  }

  // Spread first: the CLI reads checksums, download_url, dependencies, tables, ... straight
  // from this wire format, so unknown fields must pass through. Defaults fill the typed core.
  return raw.map((p) => ({
    ...p,
    name: p.name ?? "",
    version: p.version ?? "0.0.0",
    // Everything from the licensed (Bundles) registry is licensed whatever its
    // entry says: only "max" keeps its own tier, any other value becomes "pro".
    tier: expectedTier === "pro" ? (p.tier === "max" ? "max" : "pro") : (p.tier ?? expectedTier),
    description: p.description ?? "",
    category: p.category ?? "other",
    license: p.license ?? "MIT",
    tarballURL: p.tarballURL ?? "",
  }));
}

// ---------------------------------------------------------------------------
// GitHub URLs
// ---------------------------------------------------------------------------

const FREE_REGISTRY_API_URL =
  "https://api.github.com/repos/nself-org/plugins/contents/registry.json";
const PRO_REGISTRY_API_URL =
  "https://api.github.com/repos/nself-org/bundles/contents/registry.json";
const FREE_REGISTRY_RAW_URL =
  "https://raw.githubusercontent.com/nself-org/plugins/main/registry.json";

const KV_FREE = "registry:free";
const KV_PRO = "registry:pro";

const DEFAULT_CACHE_TTL = 300;

function cacheTtl(env: Env): number {
  const parsed = parseInt(env.CACHE_TTL ?? String(DEFAULT_CACHE_TTL), 10);
  return isNaN(parsed) ? DEFAULT_CACHE_TTL : parsed;
}

function isFresh<T>(entry: KVEnvelope<T>, ttlSeconds: number): boolean {
  return (Date.now() - entry.timestamp) / 1000 < ttlSeconds;
}

async function kvGet<T>(kv: KVNamespace, key: string): Promise<KVEnvelope<T> | null> {
  try {
    return await kv.get<KVEnvelope<T>>(key, "json");
  } catch {
    return null;
  }
}

async function kvPutWrapped<T>(kv: KVNamespace, key: string, data: T): Promise<void> {
  try {
    await kv.put(key, JSON.stringify({ data, timestamp: Date.now() } satisfies KVEnvelope<T>));
  } catch (err) {
    console.error(`KV put failed for key ${key}:`, (err as Error).message);
  }
}

// The Contents API returns base64 of the file's UTF-8 bytes. atob() yields one
// char per byte, so decode the bytes as UTF-8 or every non-ASCII character
// (the nSelf mark, dashes) turns into mojibake and the catalog ETag would
// differ from the one computed from the raw URL.
export function decodeGitHubContent(raw: string): unknown {
  const binary = atob(raw.replace(/\n/g, ""));
  const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0));
  return JSON.parse(new TextDecoder("utf-8").decode(bytes));
}

// ---------------------------------------------------------------------------
// Public registry loader functions
// ---------------------------------------------------------------------------

export async function fetchFreeRegistry(
  env: Env,
  ctx: ExecutionContext,
  bypass = false,
): Promise<PluginEntry[]> {
  const kv = env.REGISTRY ?? env.PLUGINS_KV;
  const ttl = cacheTtl(env);

  if (!bypass && kv) {
    const cached = await kvGet<RegistryWireFormat>(kv, KV_FREE);
    if (cached && isFresh(cached, ttl)) {
      return normaliseToArray(cached.data, "free");
    }
  }

  let data: RegistryWireFormat | null = null;

  if (env.GH_ACCESS_TOKEN) {
    try {
      const resp = await fetch(FREE_REGISTRY_API_URL, {
        headers: {
          Authorization: `token ${env.GH_ACCESS_TOKEN}`,
          Accept: "application/vnd.github.v3+json",
          "User-Agent": "nself-plugin-registry/2.0",
        },
      });
      if (resp.ok) {
        const envelope = (await resp.json()) as GitHubContentsResponse;
        data = decodeGitHubContent(envelope.content) as RegistryWireFormat;
      } else {
        console.warn(`Free registry GitHub API fetch failed: ${resp.status}`);
      }
    } catch (e) {
      // A network or decode error falls through to the raw URL, like a non-200.
      console.error("Free registry GitHub API fetch failed:", (e as Error).message);
    }
  }

  if (data === null) {
    const resp = await fetch(FREE_REGISTRY_RAW_URL, {
      headers: { "User-Agent": "nself-plugin-registry/2.0" },
    });
    if (!resp.ok) {
      // Fail loudly, as the retired index.js did: seed data would be cached and served as real.
      throw new Error(`Free registry raw fetch failed: ${resp.status}`);
    }
    data = (await resp.json()) as RegistryWireFormat;
  }

  if (kv) ctx.waitUntil(kvPutWrapped(kv, KV_FREE, data));
  return normaliseToArray(data, "free");
}

export async function fetchProRegistry(
  env: Env,
  ctx: ExecutionContext,
  bypass = false,
): Promise<PluginEntry[] | null> {
  const kv = env.REGISTRY ?? env.PLUGINS_KV;
  const ttl = cacheTtl(env);

  // Cache first (as the retired index.js did): a KV copy synced by /api/sync serves without a token.
  if (!bypass && kv) {
    const cached = await kvGet<RegistryWireFormat>(kv, KV_PRO);
    if (cached && isFresh(cached, ttl)) {
      return normaliseToArray(cached.data, "pro");
    }
  }

  if (!env.GH_ACCESS_TOKEN) {
    return null;
  }

  const resp = await fetch(PRO_REGISTRY_API_URL, {
    headers: {
      Authorization: `token ${env.GH_ACCESS_TOKEN}`,
      Accept: "application/vnd.github.v3+json",
      "User-Agent": "nself-plugin-registry/2.0",
    },
  });

  if (!resp.ok) {
    const body = await resp.text().catch(() => "");
    console.error(`Pro registry fetch failed: ${resp.status} — ${body.slice(0, 200)}`);
    return null;
  }

  let data: RegistryWireFormat;
  try {
    const envelope = (await resp.json()) as GitHubContentsResponse;
    data = decodeGitHubContent(envelope.content) as RegistryWireFormat;
  } catch (e) {
    console.error("Failed to decode pro registry content:", (e as Error).message);
    return null;
  }

  if (kv) ctx.waitUntil(kvPutWrapped(kv, KV_PRO, data));
  return normaliseToArray(data, "pro");
}

export async function fetchAllPlugins(
  env: Env,
  ctx: ExecutionContext,
  bypass = false,
): Promise<{ free: PluginEntry[]; pro: PluginEntry[]; all: PluginEntry[] }> {
  const [freeResult, proResult] = await Promise.allSettled([
    fetchFreeRegistry(env, ctx, bypass),
    fetchProRegistry(env, ctx, bypass),
  ]);

  const free = freeResult.status === "fulfilled" ? freeResult.value : [];
  const pro = proResult.status === "fulfilled" ? (proResult.value ?? []) : [];

  return { free, pro, all: [...free, ...pro] };
}

// ---------------------------------------------------------------------------
// bundles.json — P6-E4-W3-S3-T8 (ADR-P6-03: served by this worker at
// plugins.nself.org/bundles.json). Lives in the Bundles repo alongside
// registry.json today; the path becomes nself-org/bundles/contents/*.json
// the plugins-pro -> bundles rename (ADR-P6-01 / W3-S3-T6) landed 2026-09-16 and these are updated; it
// landed — update the two URL constants below then, nothing else here needs
// to change. Fetched the same way as the pro registry via the GitHub
// Contents API and cached under its own KV key so a bundles.json miss/
// refresh never invalidates the unrelated pro-registry cache.
// ---------------------------------------------------------------------------

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

/** A fetched JSON document and whether it came from the KV cache. */
export interface CachedJson {
  data: unknown;
  cache: "HIT" | "MISS";
}

/**
 * Cache-first fetch of one JSON file from the Bundles repo through the GitHub
 * Contents API (authenticated; base64 body). The KV copy is served while fresh
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
  const kv = env.REGISTRY ?? env.PLUGINS_KV;

  if (!bypass && kv) {
    const cached = await kvGet<unknown>(kv, kvKey);
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

  if (kv) ctx.waitUntil(kvPutWrapped(kv, kvKey, data));
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

/**
 * Namespace for ratings and install counters. The deployed wrangler.toml
 * binds only PLUGINS_KV, so RATINGS_KV is optional and PLUGINS_KV serves when
 * it is absent (every key is prefixed, none collide with the registry keys).
 */
export function ratingsKv(env: Env): KVNamespace {
  return (env.RATINGS_KV ?? env.PLUGINS_KV) as KVNamespace;
}

export { cacheTtl, kvGet, kvPutWrapped, isFresh, DEFAULT_CACHE_TTL };
