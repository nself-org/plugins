/**
 * catalog.ts: GET /catalog.json (P7-PLUG-08).
 *
 * Fetches the generated catalog, keeps its compact serialisation and sha256
 * together in KV, and serves it with a strong ETag and 304 handling.
 *
 * Imports: registry.ts (KV helpers, GitHub decode), http.ts (response helpers).
 */

import {
  cacheTtl,
  decodeGitHubContent,
  isFresh,
  kvGet,
  kvPutWrapped,
} from "./registry.ts";
import type { Env, GitHubContentsResponse } from "./registry.ts";
import { corsHeaders, jsonResponse } from "./http.ts";

// ---------------------------------------------------------------------------
// catalog.json — P7-PLUG-08. The free + licensed catalog projection generated
// into the plugins repo by tools/catalog (P7-PLUG-36), synced into KV like
// registry.json. The worker stores the compact serialisation and its sha256
// together so a request never re-hashes, and serves it with a strong ETag.
// ---------------------------------------------------------------------------

const CATALOG_API_URL =
  "https://api.github.com/repos/nself-org/plugins/contents/catalog.json";
const CATALOG_RAW_URL =
  "https://raw.githubusercontent.com/nself-org/plugins/main/catalog.json";
export const KV_CATALOG = "catalog:json";

/** The catalog as served: compact JSON text plus the sha256 hex of exactly those bytes. */
export interface CatalogBody {
  body: string;
  etag: string;
}

export interface CatalogResult {
  catalog: CatalogBody;
  cache: "HIT" | "MISS" | "STALE";
  /** Seconds the served copy has been in KV (0 for a fresh fetch). */
  ageSeconds: number;
}

/** A STALE copy is retried against GitHub after this many seconds, not on every request. */
const STALE_RETRY_SECONDS = 60;

async function sha256Hex(text: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

/**
 * True for a plausible catalog: an object with a schema_version string and a
 * non-empty plugins collection. An empty or unversioned body is an upstream
 * failure, not a catalog, so it can never replace a good copy.
 */
function looksLikeCatalog(data: unknown): data is Record<string, unknown> {
  if (typeof data !== "object" || data === null || Array.isArray(data)) return false;
  const obj = data as Record<string, unknown>;
  if (typeof obj["schema_version"] !== "string") return false;
  const plugins = obj["plugins"];
  if (typeof plugins !== "object" || plugins === null) return false;
  return Object.keys(plugins).length > 0;
}

/** Compact-serialise a parsed catalog and hash the result. */
export async function buildCatalogBody(data: unknown): Promise<CatalogBody> {
  const body = JSON.stringify(data);
  return { body, etag: await sha256Hex(body) };
}

/**
 * Loads the catalog: fresh KV copy first, then GitHub (Contents API with the
 * token, else the raw URL; the repo is public). A body that is not a catalog
 * is never cached or served. When the upstream fails and an expired KV copy
 * exists it is served as STALE rather than failing. Returns null when there
 * is nothing to serve.
 */
export async function fetchCatalog(
  env: Env,
  ctx: ExecutionContext,
  bypass = false,
): Promise<CatalogResult | null> {
  const kv = env.REGISTRY ?? env.PLUGINS_KV;
  const ttl = cacheTtl(env);

  const cached = kv ? await kvGet<CatalogBody>(kv, KV_CATALOG) : null;
  const usable =
    cached !== null &&
    typeof cached.data?.body === "string" &&
    typeof cached.data?.etag === "string";
  const ageSeconds = cached ? Math.max(0, Math.floor((Date.now() - cached.timestamp) / 1000)) : 0;
  if (!bypass && cached && usable && isFresh(cached, ttl)) {
    return { catalog: cached.data, cache: "HIT", ageSeconds };
  }

  // Contents API first (token); any failure there, including a decode error or an
  // empty body for a file over 1 MB, falls through to the raw URL.
  let data: unknown = null;
  if (env.GH_ACCESS_TOKEN) {
    try {
      const resp = await fetch(CATALOG_API_URL, {
        headers: {
          Authorization: `token ${env.GH_ACCESS_TOKEN}`,
          Accept: "application/vnd.github.v3+json",
          "User-Agent": "nself-plugin-registry/2.0",
        },
      });
      if (resp.ok) {
        const envelope = (await resp.json()) as GitHubContentsResponse;
        data = decodeGitHubContent(envelope.content);
      } else {
        console.warn(`catalog.json GitHub API fetch failed: ${resp.status}`);
      }
    } catch (e) {
      console.warn("catalog.json GitHub API fetch failed:", (e as Error).message);
      data = null;
    }
  }
  if (!looksLikeCatalog(data)) {
    try {
      const resp = await fetch(CATALOG_RAW_URL, {
        headers: { "User-Agent": "nself-plugin-registry/2.0" },
      });
      data = resp.ok ? await resp.json() : null;
      if (!resp.ok) console.warn(`catalog.json raw fetch failed: ${resp.status}`);
    } catch (e) {
      console.error("catalog.json raw fetch failed:", (e as Error).message);
      data = null;
    }
  }

  if (looksLikeCatalog(data)) {
    const catalog = await buildCatalogBody(data);
    if (kv) ctx.waitUntil(kvPutWrapped(kv, KV_CATALOG, catalog));
    return { catalog, cache: "MISS", ageSeconds: 0 };
  }

  if (cached && usable) {
    // Upstream is down: serve the old copy and push its timestamp so it is
    // retried in STALE_RETRY_SECONDS rather than hitting GitHub on every request.
    if (kv) {
      const timestamp = Date.now() - Math.max(0, ttl - STALE_RETRY_SECONDS) * 1000;
      ctx.waitUntil(
        kv.put(KV_CATALOG, JSON.stringify({ data: cached.data, timestamp })).catch(() => {}),
      );
    }
    return { catalog: cached.data, cache: "STALE", ageSeconds };
  }
  return null;
}

/**
 * RFC 9110 If-None-Match: true when the header lists the current ETag (or *).
 * Comparison is weak for this header, so a W/ prefix on a listed tag is ignored.
 */
export function etagMatches(ifNoneMatch: string | null, etag: string): boolean {
  if (!ifNoneMatch) return false;
  const header = ifNoneMatch.trim();
  if (header === "*") return true;
  return header
    .split(",")
    .map((t) => t.trim().replace(/^W\//, ""))
    .some((t) => t === `"${etag}"`);
}

// ---------------------------------------------------------------------------
// GET /catalog.json
//
// Compact JSON with a strong ETag (the sha256 hex of the exact body, quoted).
// A request whose If-None-Match lists that tag (or *) gets 304 with no body.
// The ETag changes exactly when the catalog bytes change. KV key catalog:json
// is refreshed by POST /api/sync (a bypassing refetch; the old copy survives
// a failed refetch and is then served STALE).
// ---------------------------------------------------------------------------

export async function handleCatalog(
  request: Request,
  env: Env,
  ctx: ExecutionContext,
): Promise<Response> {
  const result = await fetchCatalog(env, ctx);
  if (result === null) {
    return jsonResponse(
      { error: "catalog.json unavailable — upstream fetch failed and no cached copy" },
      502,
    );
  }

  const { body, etag } = result.catalog;
  // Never promise downstream caches more than the KV copy has left to live.
  const ttl = cacheTtl(env);
  const maxAge =
    result.cache === "STALE"
      ? STALE_RETRY_SECONDS
      : Math.max(0, ttl - result.ageSeconds);
  const headers: Record<string, string> = {
    ...corsHeaders(),
    ETag: `"${etag}"`,
    "Cache-Control": `public, max-age=${maxAge}`,
    "X-Cache": result.cache,
  };

  if (etagMatches(request.headers.get("If-None-Match"), etag)) {
    return new Response(null, { status: 304, headers });
  }
  return new Response(body, {
    status: 200,
    headers: { "Content-Type": "application/json", ...headers },
  });
}
