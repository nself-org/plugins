import { jsonResponse } from "./http.ts";
import { fetchAllPlugins, cacheTtl } from "./registry.ts";
import type { Env } from "./registry.ts";
import { handleMarketplace, handleGetRating, handlePostRating, handleInstallEvent, checkGetRateLimit } from "./marketplace.ts";
import type { MarketplaceEnv } from "./marketplace.ts";

export async function handleMarketplaceRoute(request: Request, url: URL, resolvedPath: string, method: string, env: Env, ctx: ExecutionContext): Promise<Response | null> {
      // Marketplace — GET /marketplace/ratings/:name (must appear before /marketplace)
      if (method === "GET" && resolvedPath.startsWith("/marketplace/ratings/")) {
        // T-RATE-01: GET rate limit — 60 req/min per IP
        const rlGet = await checkGetRateLimit(request, env as MarketplaceEnv);
        if (rlGet.limited) {
          return jsonResponse(
            { error: `Rate limit exceeded. Retry after ${rlGet.retryAfter}s.` },
            429,
            { "Retry-After": String(rlGet.retryAfter) },
          );
        }
        const pluginName = resolvedPath.slice("/marketplace/ratings/".length);
        if (!pluginName) {
          return jsonResponse({ error: "plugin name required" }, 400);
        }
        const result = await handleGetRating(pluginName, env as MarketplaceEnv);
        // ratingCount is the retired index.js spelling of reviewCount.
        return jsonResponse({ ...result, ratingCount: result.reviewCount }, 200, {
          "Cache-Control": "public, max-age=60",
        });
      }

      // Marketplace — POST /plugins/:name/install-event (S68-T02)
      // Silent fire-and-forget from CLI after successful plugin install.
      // Body: { instanceId: string } — opaque SHA-256 hash, no PII.
      if (method === "POST" && resolvedPath.startsWith("/plugins/") && resolvedPath.endsWith("/install-event")) {
        const parts = resolvedPath.split("/").filter(Boolean);
        // /plugins/:name/install-event → parts = ["plugins", name, "install-event"]
        const pluginName = parts[1];
        if (!pluginName) {
          return jsonResponse({ error: "plugin name required" }, 400);
        }
        // Only count plugins that exist: an unknown name must not create KV keys.
        const { all: known } = await fetchAllPlugins(env, ctx);
        if (!known.some((p) => p.name === pluginName)) {
          return jsonResponse({ error: "plugin not found", name: pluginName }, 404);
        }
        const result = await handleInstallEvent(pluginName, request, env as MarketplaceEnv);
        return jsonResponse(
          result.ok
            ? { name: result.name, downloads: result.downloads, incremented: result.incremented }
            : { error: result.error },
          result.status,
        );
      }

      // Marketplace — POST /marketplace/ratings/:name
      if (method === "POST" && resolvedPath.startsWith("/marketplace/ratings/")) {
        const pluginName = resolvedPath.slice("/marketplace/ratings/".length);
        if (!pluginName) {
          return jsonResponse({ error: "plugin name required" }, 400);
        }
        const result = await handlePostRating(pluginName, request, env as MarketplaceEnv);

        // T05: Emit Retry-After header on all 429 responses
        const extraHeaders: Record<string, string> = {};
        if (result.status === 429) {
          // Extract Retry-After seconds from error message if present
          const match = (result.error ?? "").match(/Retry-After:\s*(\d+)s/);
          extraHeaders["Retry-After"] = match?.[1] ?? "60";
        }

        return jsonResponse(
          result.ok
            ? { name: result.name, rating: result.rating, reviewCount: result.reviewCount }
            : { error: result.error },
          result.status,
          extraHeaders,
        );
      }

      // Marketplace — GET /marketplace
      if (method === "GET" && resolvedPath === "/marketplace") {
        // T-RATE-01: GET rate limit — 60 req/min per IP
        const rlGet = await checkGetRateLimit(request, env as MarketplaceEnv);
        if (rlGet.limited) {
          return jsonResponse(
            { error: `Rate limit exceeded. Retry after ${rlGet.retryAfter}s.` },
            429,
            { "Retry-After": String(rlGet.retryAfter) },
          );
        }
        const { all, free, pro } = await fetchAllPlugins(env, ctx);
        const payload = await handleMarketplace(url, env as MarketplaceEnv, all, ctx);
        // Override stats counts with fresh registry counts for accuracy
        payload.stats.free = free.length;
        payload.stats.pro = pro.length;
        payload.stats.total = all.length;
        // version, fetchedAt and total (the filtered count, as in index.js) are the retired envelope fields, kept additively.
        return jsonResponse({ ...payload, version: "1.0.0", fetchedAt: payload.stats.updatedAt, total: payload.plugins.length }, 200, {
          "Cache-Control": `public, max-age=${cacheTtl(env)}`,
        });
      }

  return null;
}
