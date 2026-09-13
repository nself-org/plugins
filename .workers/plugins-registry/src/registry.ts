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
  RATINGS_KV: KVNamespace;
  SIGNING_PRIVATE_KEY: string;
  PUBLIC_KEY_HEX: string;
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

interface GitHubContentsResponse {
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
// Static fallback registry — seed data so the worker never returns empty
// on first cold start. Contains only the stable free plugins.
// ---------------------------------------------------------------------------

const STATIC_FALLBACK: PluginEntry[] = [
  {
    name: "backup",
    version: "1.0.0",
    tier: "free",
    description: "Automated backup with pruning and cloud storage",
    category: "infrastructure",
    license: "MIT",
    tarballURL: "https://github.com/nself-org/plugins/releases/download/v1.0.0/backup-1.0.0.tar.gz",
    author: "nself",
    homepage: "https://github.com/nself-org/plugins/tree/main/backup",
  },
  {
    name: "cron",
    version: "1.0.0",
    tier: "free",
    description: "Scheduled job execution",
    category: "automation",
    license: "MIT",
    tarballURL: "https://github.com/nself-org/plugins/releases/download/v1.0.0/cron-1.0.0.tar.gz",
    author: "nself",
    homepage: "https://github.com/nself-org/plugins/tree/main/cron",
  },
  {
    name: "feature-flags",
    version: "1.0.0",
    tier: "free",
    description: "Feature flag management",
    category: "infrastructure",
    license: "MIT",
    tarballURL: "https://github.com/nself-org/plugins/releases/download/v1.0.0/feature-flags-1.0.0.tar.gz",
    author: "nself",
    homepage: "https://github.com/nself-org/plugins/tree/main/feature-flags",
  },
  {
    name: "jobs",
    version: "1.0.0",
    tier: "free",
    description: "BullMQ background job queue",
    category: "infrastructure",
    license: "MIT",
    tarballURL: "https://github.com/nself-org/plugins/releases/download/v1.0.0/jobs-1.0.0.tar.gz",
    author: "nself",
    homepage: "https://github.com/nself-org/plugins/tree/main/jobs",
  },
  {
    name: "monitoring",
    version: "1.0.0",
    tier: "free",
    description: "Prometheus, Grafana, Loki, and full observability stack",
    category: "infrastructure",
    license: "MIT",
    tarballURL: "https://github.com/nself-org/plugins/releases/download/v1.0.0/monitoring-1.0.0.tar.gz",
    author: "nself",
    homepage: "https://github.com/nself-org/plugins/tree/main/monitoring",
  },
  {
    name: "search",
    version: "1.0.0",
    tier: "free",
    description: "Full-text search with PostgreSQL and MeiliSearch",
    category: "infrastructure",
    license: "MIT",
    tarballURL: "https://github.com/nself-org/plugins/releases/download/v1.0.0/search-1.0.0.tar.gz",
    author: "nself",
    homepage: "https://github.com/nself-org/plugins/tree/main/search",
  },
  {
    name: "stripe",
    version: "1.0.0",
    tier: "free",
    description: "Stripe payment and billing integration",
    category: "commerce",
    license: "MIT",
    tarballURL: "https://github.com/nself-org/plugins/releases/download/v1.0.0/stripe-1.0.0.tar.gz",
    author: "nself",
    homepage: "https://github.com/nself-org/plugins/tree/main/stripe",
  },
  {
    name: "webhooks",
    version: "1.0.0",
    tier: "free",
    description: "Outbound webhook delivery with retry",
    category: "communication",
    license: "MIT",
    tarballURL: "https://github.com/nself-org/plugins/releases/download/v1.0.0/webhooks-1.0.0.tar.gz",
    author: "nself",
    homepage: "https://github.com/nself-org/plugins/tree/main/webhooks",
  },
];

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
    // omits the "name" field (plugins-pro/registry.json uses this dict format).
    raw = Object.entries(data.plugins as Record<string, Omit<PluginEntry, "tier"> & { tier?: PluginTier }>)
      .map(([key, entry]) => (entry.name ? entry : { ...entry, name: key }));
  } else {
    return [];
  }

  return raw.map((p) => ({
    name: p.name ?? "",
    version: p.version ?? "0.0.0",
    tier: p.tier ?? expectedTier,
    description: p.description ?? "",
    category: p.category ?? "other",
    license: p.license ?? "MIT",
    tarballURL: p.tarballURL ?? "",
    ...(p.checksum !== undefined && { checksum: p.checksum }),
    ...(p.signature !== undefined && { signature: p.signature }),
    ...(p.author !== undefined && { author: p.author }),
    ...(p.homepage !== undefined && { homepage: p.homepage }),
    ...(p.repository !== undefined && { repository: p.repository }),
    ...(p.tags !== undefined && { tags: p.tags }),
    ...(p.downloads !== undefined && { downloads: p.downloads }),
    ...(p.displayName !== undefined && { displayName: p.displayName }),
  }));
}

// ---------------------------------------------------------------------------
// GitHub URLs
// ---------------------------------------------------------------------------

const FREE_REGISTRY_API_URL =
  "https://api.github.com/repos/nself-org/plugins/contents/registry.json";
const PRO_REGISTRY_API_URL =
  "https://api.github.com/repos/nself-org/plugins-pro/contents/registry.json";
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

function decodeGitHubContent(raw: string): unknown {
  const decoded = atob(raw.replace(/\n/g, ""));
  return JSON.parse(decoded);
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
    const resp = await fetch(FREE_REGISTRY_API_URL, {
      headers: {
        Authorization: `token ${env.GH_ACCESS_TOKEN}`,
        Accept: "application/vnd.github.v3+json",
        "User-Agent": "nself-plugin-registry/2.0",
      },
    });
    if (resp.ok) {
      try {
        const envelope = (await resp.json()) as GitHubContentsResponse;
        data = decodeGitHubContent(envelope.content) as RegistryWireFormat;
      } catch (e) {
        console.error("Failed to decode free registry content:", (e as Error).message);
      }
    } else {
      console.warn(`Free registry GitHub API fetch failed: ${resp.status}`);
    }
  }

  if (data === null) {
    const resp = await fetch(FREE_REGISTRY_RAW_URL, {
      headers: { "User-Agent": "nself-plugin-registry/2.0" },
    });
    if (!resp.ok) {
      console.warn(`Free registry raw fetch failed: ${resp.status} — using static fallback`);
      return STATIC_FALLBACK;
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
  if (!env.GH_ACCESS_TOKEN) {
    return null;
  }

  const kv = env.REGISTRY ?? env.PLUGINS_KV;
  const ttl = cacheTtl(env);

  if (!bypass && kv) {
    const cached = await kvGet<RegistryWireFormat>(kv, KV_PRO);
    if (cached && isFresh(cached, ttl)) {
      return normaliseToArray(cached.data, "pro");
    }
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
// plugins.nself.org/bundles.json). Lives in plugins-pro alongside
// registry.json today; the path becomes nself-org/bundles/contents/*.json
// once the plugins-pro -> bundles repo rename (ADR-P6-01 / W3-S3-T6) has
// landed — update the two URL constants below then, nothing else here needs
// to change. Fetched the same way as the pro registry via the GitHub
// Contents API and cached under its own KV key so a bundles.json miss/
// refresh never invalidates the unrelated pro-registry cache.
// ---------------------------------------------------------------------------

const BUNDLES_JSON_API_URL =
  "https://api.github.com/repos/nself-org/plugins-pro/contents/bundles.json";
const BUNDLES_SCHEMA_API_URL =
  "https://api.github.com/repos/nself-org/plugins-pro/contents/bundles-schema.json";

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

/**
 * Fetches bundles.json from the plugins-pro repo via the GitHub Contents
 * API, mirroring fetchProRegistry's auth + KV-cache pattern above (reused
 * rather than re-implemented per DRY). Returns the raw parsed JSON
 * (unvalidated — callers run validateBundlesJson separately so a schema
 * failure can be reported distinctly from a fetch failure) or null on
 * fetch/decode failure.
 */
export async function fetchBundlesJson(
  env: Env,
  ctx: ExecutionContext,
  bypass = false,
): Promise<unknown | null> {
  if (!env.GH_ACCESS_TOKEN) {
    console.warn("GH_ACCESS_TOKEN not set — bundles.json unavailable");
    return null;
  }

  const kv = env.REGISTRY ?? env.PLUGINS_KV;
  const ttl = cacheTtl(env);

  if (!bypass && kv) {
    const cached = await kvGet<unknown>(kv, KV_BUNDLES_JSON);
    if (cached && isFresh(cached, ttl)) {
      return cached.data;
    }
  }

  const resp = await fetch(BUNDLES_JSON_API_URL, {
    headers: {
      Authorization: `token ${env.GH_ACCESS_TOKEN}`,
      Accept: "application/vnd.github.v3+json",
      "User-Agent": "nself-plugin-registry/2.0",
    },
  });

  if (!resp.ok) {
    const body = await resp.text().catch(() => "");
    console.error(`bundles.json fetch failed: ${resp.status} — ${body.slice(0, 200)}`);
    return null;
  }

  let data: unknown;
  try {
    const envelope = (await resp.json()) as GitHubContentsResponse;
    data = decodeGitHubContent(envelope.content);
  } catch (e) {
    console.error("Failed to decode bundles.json content:", (e as Error).message);
    return null;
  }

  if (kv) ctx.waitUntil(kvPutWrapped(kv, KV_BUNDLES_JSON, data));
  return data;
}

/**
 * Fetches bundles-schema.json — same repo, same auth pattern, its own KV key.
 * Served as-is (no validation of the schema against itself).
 */
export async function fetchBundlesSchema(
  env: Env,
  ctx: ExecutionContext,
  bypass = false,
): Promise<unknown | null> {
  if (!env.GH_ACCESS_TOKEN) {
    return null;
  }

  const kv = env.REGISTRY ?? env.PLUGINS_KV;
  const ttl = cacheTtl(env);

  if (!bypass && kv) {
    const cached = await kvGet<unknown>(kv, KV_BUNDLES_SCHEMA);
    if (cached && isFresh(cached, ttl)) {
      return cached.data;
    }
  }

  const resp = await fetch(BUNDLES_SCHEMA_API_URL, {
    headers: {
      Authorization: `token ${env.GH_ACCESS_TOKEN}`,
      Accept: "application/vnd.github.v3+json",
      "User-Agent": "nself-plugin-registry/2.0",
    },
  });

  if (!resp.ok) {
    console.error(`bundles-schema.json fetch failed: ${resp.status}`);
    return null;
  }

  let data: unknown;
  try {
    const envelope = (await resp.json()) as GitHubContentsResponse;
    data = decodeGitHubContent(envelope.content);
  } catch (e) {
    console.error("Failed to decode bundles-schema.json content:", (e as Error).message);
    return null;
  }

  if (kv) ctx.waitUntil(kvPutWrapped(kv, KV_BUNDLES_SCHEMA, data));
  return data;
}

export { cacheTtl, kvGet, kvPutWrapped, isFresh, DEFAULT_CACHE_TTL };
