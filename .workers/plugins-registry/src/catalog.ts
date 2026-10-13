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
}

async function sha256Hex(text: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

/** True for a plausible catalog: a JSON object that carries a plugins collection. */
function looksLikeCatalog(data: unknown): data is Record<string, unknown> {
  if (typeof data !== "object" || data === null || Array.isArray(data)) return false;
  const plugins = (data as Record<string, unknown>)["plugins"];
  return typeof plugins === "object" && plugins !== null;
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
  if (!bypass && cached && usable && isFresh(cached, ttl)) {
    return { catalog: cached.data, cache: "HIT" };
  }

  let data: unknown = null;
  try {
    if (env.GH_ACCESS_TOKEN) {
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
    }
    if (!looksLikeCatalog(data)) {
      const resp = await fetch(CATALOG_RAW_URL, {
        headers: { "User-Agent": "nself-plugin-registry/2.0" },
      });
      data = resp.ok ? await resp.json() : null;
      if (!resp.ok) console.warn(`catalog.json raw fetch failed: ${resp.status}`);
    }
  } catch (e) {
    console.error("catalog.json fetch failed:", (e as Error).message);
    data = null;
  }

  if (looksLikeCatalog(data)) {
    const catalog = await buildCatalogBody(data);
    if (kv) ctx.waitUntil(kvPutWrapped(kv, KV_CATALOG, catalog));
    return { catalog, cache: "MISS" };
  }

  if (cached && usable) {
    return { catalog: cached.data, cache: "STALE" };
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
// is dropped by POST /api/sync like the registry keys.
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
  const maxAge = result.cache === "STALE" ? 60 : cacheTtl(env);
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
