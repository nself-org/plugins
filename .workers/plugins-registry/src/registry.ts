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
    raw = Object.values(data.plugins as Record<string, Omit<PluginEntry, "tier"> & { tier?: PluginTier }>);
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

export { cacheTtl, kvGet, kvPutWrapped, isFresh, DEFAULT_CACHE_TTL };
