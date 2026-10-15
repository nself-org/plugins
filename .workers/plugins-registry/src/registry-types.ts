/** Shared registry types, normalization and read-only KV access for durable state. */
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

export type RegistryWireFormat = FreeRegistryWireFormat | ProRegistryWireFormat | PluginEntry[];

// ---------------------------------------------------------------------------
// Normalise various wire formats to a typed flat array
// ---------------------------------------------------------------------------

export function normaliseToArray(data: RegistryWireFormat, expectedTier: PluginTier): PluginEntry[] {
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
export const DEFAULT_CACHE_TTL = 300;

export function cacheTtl(env: Env): number {
  const parsed = parseInt(env.CACHE_TTL ?? String(DEFAULT_CACHE_TTL), 10);
  return isNaN(parsed) ? DEFAULT_CACHE_TTL : parsed;
}

export function isFresh<T>(entry: KVEnvelope<T>, ttlSeconds: number): boolean {
  return (Date.now() - entry.timestamp) / 1000 < ttlSeconds;
}

export async function kvGet<T>(kv: KVNamespace, key: string): Promise<KVEnvelope<T> | null> {
  try {
    return await kv.get<KVEnvelope<T>>(key, "json");
  } catch {
    return null;
  }
}

export function decodeGitHubContent(raw: string): unknown {
  const binary = atob(raw.replace(/\n/g, ""));
  const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0));
  return JSON.parse(new TextDecoder("utf-8").decode(bytes));
}

// ---------------------------------------------------------------------------
// Public registry loader functions
// ---------------------------------------------------------------------------

/**
 * Namespace for ratings and install counters. The deployed wrangler.toml
 * binds only PLUGINS_KV, so RATINGS_KV is optional and PLUGINS_KV serves when
 * it is absent (every key is prefixed, none collide with the registry keys).
 */
export function ratingsKv(env: Env): KVNamespace {
  return (env.RATINGS_KV ?? env.PLUGINS_KV) as KVNamespace;
}
