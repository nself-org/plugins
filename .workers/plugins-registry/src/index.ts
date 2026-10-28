/** nself Plugin Registry worker: routes public registry and marketplace requests. */
import { corsHeaders, jsonResponse } from "./http.ts";
import { handleBundlesJson, handleBundlesSchema } from "./bundles.ts";
import { handleCatalog } from "./catalog.ts";
import { handleRevocations, handleRevokedAuthors } from "./revocations.ts";
import { validateBundlesJson } from "./registry.ts";
import type { Env } from "./registry.ts";
import { handlePluginList, handlePlugin, isDirectDownloadTier, handlePluginTarball } from "./plugin-routes.ts";
import { handleRegistry, handleCategories, handleManifest } from "./registry-routes.ts";
import { handleHealth, handleStats, handleSync } from "./admin-routes.ts";
import { handleMarketplaceRoute } from "./marketplace-routes.ts";
import { resolveApiVersion, addApiVersionHeader } from "./middleware/api-version.ts";

const router = {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    const url = new URL(request.url);
    const path = url.pathname.replace(/\/$/, "") || "/";
    const method = request.method.toUpperCase();

    if (method === "OPTIONS") {
      return new Response(null, { status: 204, headers: corsHeaders() });
    }

    // API versioning — X-API-Version header (S77-T04)
    // Missing header defaults to v1 forever (LTS commitment through 2027-04-17)
    const { version: apiVersion, errorResponse: versionError } = resolveApiVersion(request);
    if (versionError) {
      return versionError;
    }

    // URL versioning: /v1/* → same as /* (unversioned stays v1 forever)
    const effectivePath = path.startsWith("/v1/") ? path.slice(3) : path;

    // For response headers, we'll add X-API-Version via addApiVersionHeader when building responses.
    // The variable apiVersion is available throughout the handler.
    void apiVersion; // suppress unused-variable warning until used in a future version
    void addApiVersionHeader; // imported for use in future version routing

    // resolvedPath is the canonical path after stripping /v1 prefix
    const resolvedPath = effectivePath;

    try {
      // Route matching uses resolvedPath (strips /v1 prefix if present)
      // Health
      if (method === "GET" && resolvedPath === "/health") {
        return await handleHealth(env);
      }

      // Registry (legacy compat)
      if (method === "GET" && (resolvedPath === "/registry.json" || resolvedPath === "/registry" || resolvedPath === "/")) {
        return await handleRegistry(url, env, ctx);
      }

      // Plugin list — must appear before /plugins/:name to prevent shadowing
      // S67-T02: /registry/list is an alias with explicit 60s CF edge cache header
      if (method === "GET" && (resolvedPath === "/plugins" || resolvedPath === "/registry/list")) {
        return await handlePluginList(url, env, ctx);
      }

      // Revocations — must appear before /plugins/:name to prevent "revocations" being
      // treated as a plugin name
      if (method === "GET" && resolvedPath === "/plugins/revocations") {
        return await handleRevocations(env);
      }

      // S58-T09: Author CRL endpoint.
      // CLI checks this on every plugin install and via daily cron.
      // Append-only: once revoked, entries are never removed.
      if (method === "GET" && resolvedPath === "/.well-known/revoked-authors.json") {
        return await handleRevokedAuthors(env);
      }

      // Single plugin (and sub-resources: /tarball, /signature, /:version)
      if (method === "GET" && resolvedPath.startsWith("/plugins/")) {
        return await handlePlugin(resolvedPath, env, ctx);
      }

      // Categories
      if (method === "GET" && resolvedPath === "/categories") {
        return await handleCategories(env, ctx);
      }

      // Manifest (CLI compat)
      if (method === "GET" && resolvedPath === "/manifest.json") {
        return await handleManifest(env, ctx);
      }

      // Catalog — P7-PLUG-08 (strong ETag, 304)
      if (method === "GET" && resolvedPath === "/catalog.json") {
        return await handleCatalog(request, env, ctx);
      }

      // Bundles — P6-E4-W3-S3-T8 / ADR-P6-03
      if (method === "GET" && resolvedPath === "/bundles.json") {
        return await handleBundlesJson(env, ctx);
      }
      if (method === "GET" && resolvedPath === "/bundles-schema.json") {
        return await handleBundlesSchema(env, ctx);
      }

      // Stats
      if (method === "GET" && resolvedPath === "/stats") {
        return await handleStats(env);
      }

      const marketplace = await handleMarketplaceRoute(request, url, resolvedPath, method, env, ctx);
      if (marketplace) return marketplace;

      // Sync webhook
      if (method === "POST" && resolvedPath === "/api/sync") {
        return await handleSync(request, env, ctx);
      }

      return jsonResponse(
        {
          error: "not found",
          endpoints: [
            "GET /health",
            "GET /plugins",
            "GET /plugins/:name",
            "GET /plugins/:name/tarball",
            "GET /plugins/:name/signature",
            "GET /plugins/:name/:version",
            "GET /plugins/revocations",
            "GET /registry.json",
            "GET /registry/list",
            "GET /.well-known/revoked-authors.json",
            "GET /categories",
            "GET /manifest.json",
            "GET /catalog.json",
            "GET /bundles.json",
            "GET /bundles-schema.json",
            "GET /marketplace",
            "GET /marketplace/ratings/:name",
            "POST /marketplace/ratings/:name",
            "POST /plugins/:name/install-event",
            "GET /stats",
            "POST /api/sync",
          ],
        },
        404,
      );
    } catch (err) {
      console.error("Unhandled error:", (err as Error).message);
      return jsonResponse({ error: "internal server error", message: (err as Error).message }, 500);
    }
  },
} satisfies ExportedHandler<Env>;

/** Route HEAD through GET, preserving response metadata while omitting its body. */
export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    if (request.method !== "HEAD") return router.fetch(request, env, ctx);
    const response = await router.fetch(new Request(request, { method: "GET" }), env, ctx);
    return new Response(null, {
      status: response.status,
      statusText: response.statusText,
      headers: response.headers,
    });
  },
} satisfies ExportedHandler<Env>;

export type { Env, PluginEntry, PluginListResponse, KVEnvelope } from "./registry.ts";
export { isDirectDownloadTier, handlePluginTarball, validateBundlesJson, handleBundlesJson, handleCatalog };
