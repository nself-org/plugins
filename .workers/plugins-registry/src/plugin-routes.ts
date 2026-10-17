import { signPlugin, canonicalPluginString } from "./sign.ts";
import { jsonResponse } from "./http.ts";
import { fetchAllPlugins, cacheTtl } from "./registry.ts";
import type { Env, PluginEntry, PluginListResponse, PluginTier } from "./registry.ts";
import { isRevoked } from "./revocations.ts";

/** Ed25519 public key hex: PUBLIC_KEY_HEX, or the retired index.js name SIGNING_PUBLIC_KEY. */
function publicKeyHex(env: Env): string | undefined {
  return env.PUBLIC_KEY_HEX ?? env.SIGNING_PUBLIC_KEY;
}

export async function handlePluginList(url: URL, env: Env, ctx: ExecutionContext): Promise<Response> {
  const tierFilter = url.searchParams.get("tier");
  const categoryFilter = url.searchParams.get("category");
  const ttl = cacheTtl(env);

  const { all } = await fetchAllPlugins(env, ctx);

  let filtered = all;
  if (tierFilter === "free" || tierFilter === "pro") {
    filtered = filtered.filter((p) => p.tier === tierFilter);
  }
  if (categoryFilter) {
    filtered = filtered.filter(
      (p) => p.category.toLowerCase() === categoryFilter.toLowerCase(),
    );
  }

  const response: PluginListResponse = {
    plugins: filtered,
    total: filtered.length,
    // free/pro count the filtered list, as the retired index.js did
    free: filtered.filter((p) => p.tier === "free").length,
    pro: filtered.filter((p) => p.tier === "pro").length,
    generatedAt: new Date().toISOString(),
  };

  // S67-T02: emit s-maxage for CF edge cache (60s TTL, 300s stale-while-revalidate)
  return jsonResponse(response, 200, {
    "Cache-Control": `public, s-maxage=60, stale-while-revalidate=300, max-age=${ttl}`,
  });
}

// ---------------------------------------------------------------------------
// GET /plugins/:name  and sub-resources
// ---------------------------------------------------------------------------

export async function handlePlugin(path: string, env: Env, ctx: ExecutionContext): Promise<Response> {
  const parts = path.split("/").filter(Boolean);
  // parts[0] = "plugins", parts[1] = name, parts[2] = sub-resource or version
  const pluginName = parts[1];
  const subResource = parts[2] ?? "latest";

  if (!pluginName) {
    return jsonResponse({ error: "plugin name required" }, 400);
  }

  const { all } = await fetchAllPlugins(env, ctx);
  const plugin = all.find((p) => p.name === pluginName);

  if (!plugin) {
    return jsonResponse(
      {
        error: "plugin not found",
        name: pluginName,
        available: all.map((p) => p.name).sort(),
      },
      404,
    );
  }

  if (subResource === "tarball") {
    return handlePluginTarball(plugin, env);
  }

  if (subResource === "signature") {
    return handlePluginSignature(plugin, env);
  }

  if (subResource !== "latest" && subResource !== plugin.version) {
    return jsonResponse(
      {
        error: "version not found",
        requestedVersion: subResource,
        latestVersion: plugin.version,
      },
      404,
    );
  }

  return jsonResponse(plugin, 200, { "Cache-Control": "public, max-age=60" });
}

// ---------------------------------------------------------------------------
// GET /plugins/:name/tarball — 302 to R2 (primary) or GitHub Releases (fallback)
//
// S67-T03: R2 bucket `nself-plugin-tarballs` is the primary CDN (free egress).
// Worker checks R2 object existence via HEAD before redirecting.
// Falls back to GitHub Releases on R2 not-found or any R2 error.
// Pro plugins always use the ping_api gated download — never this path.
// ---------------------------------------------------------------------------

// qa/bugs/paid-tarball-gated-by-repo-visibility-not-licence.md / P6 siege HIGH:
// PR #66 first added a guard here, but it only checked `tier === "pro"`.
// PluginTier is "free" | "pro" | "max" (a live plugin, "ai", ships at "max"),
// so a max-tier tarball still fell through to the unauthenticated redirect
// below. This switch is exhaustive over PluginTier: adding a 4th tier
// without adding a case here is a TypeScript compile error (via the `never`
// assignment in `default`), so the guard cannot silently miss a new paid
// tier the way it missed "max". It is also fail-safe at runtime: registry
// data is not type-checked, so any value that is not the literal "free"
// (a future tier, a typo, undefined) is refused. This is the only source
// (src/index.js was collapsed into it in P7-PLUG-08), and it is what
// wrangler.toml deploys.
export function isDirectDownloadTier(tier: PluginTier): boolean {
  switch (tier) {
    case "free":
      return true;
    case "pro":
    case "max":
      return false;
    default: {
      const _exhaustive: never = tier;
      void _exhaustive;
      return false;
    }
  }
}

export async function handlePluginTarball(plugin: PluginEntry, env: Env): Promise<Response> {
  const { name, version, tier } = plugin;

  // Refuse every non-free tier and point callers at the real, licence-gated
  // download route. See isDirectDownloadTier() above.
  if (!isDirectDownloadTier(tier)) {
    return jsonResponse(
      {
        error: "paid plugin — use the licence-gated download endpoint",
        plugin: name,
        download_url: `https://ping.nself.org/plugins/${name}/download`,
      },
      401,
    );
  }

  const revoked = await isRevoked(env, name, version);
  if (revoked) {
    return jsonResponse({ error: "plugin version revoked", plugin: name, version }, 410);
  }

  const repo = tier === "pro" ? "bundles" : "plugins";
  // GitHub Releases URL (canonical fallback, always valid)
  const githubURL =
    `https://github.com/nself-org/${repo}/releases/download/v${version}/${name}-${version}.tar.gz`;

  // S67-T03: R2 primary for free plugins only
  let tarballURL = githubURL;
  let tarballSource = "github";
  if (tier !== "pro" && env.PLUGIN_TARBALLS && env.R2_PUBLIC_DOMAIN) {
    const r2Key = `plugins/${name}/${name}-${version}.tar.gz`;
    try {
      const obj = await env.PLUGIN_TARBALLS.head(r2Key);
      if (obj !== null) {
        tarballURL = `https://${env.R2_PUBLIC_DOMAIN}/${r2Key}`;
        tarballSource = "r2";
      }
    } catch {
      // R2 unavailable or error — fall through to GitHub fallback
    }
  }

  const headers: Record<string, string> = {
    Location: tarballURL,
    "Access-Control-Allow-Origin": "*",
    "Cache-Control": "public, max-age=300",
    "X-Tarball-Source": tarballSource,
  };

  if (env.SIGNING_PRIVATE_KEY) {
    const signature = await signPlugin(name, version, tarballURL, env.SIGNING_PRIVATE_KEY);
    if (signature) {
      headers["X-Signature"] = signature;
      headers["X-Signed-For"] = canonicalPluginString(name, version, tarballURL);
    }
  }

  return new Response(null, { status: 302, headers });
}

// ---------------------------------------------------------------------------
// GET /plugins/:name/signature — Ed25519 signature metadata
// ---------------------------------------------------------------------------

async function handlePluginSignature(plugin: PluginEntry, env: Env): Promise<Response> {
  const { name, version, tier } = plugin;

  const repo = tier === "pro" ? "bundles" : "plugins";
  const tarballURL =
    `https://github.com/nself-org/${repo}/releases/download/v${version}/${name}-${version}.tar.gz`;

  if (!env.SIGNING_PRIVATE_KEY) {
    return jsonResponse(
      { name, version, tarballURL, signature: null },
      200,
      { "Cache-Control": "public, max-age=300" },
    );
  }

  const signature = await signPlugin(name, version, tarballURL, env.SIGNING_PRIVATE_KEY);

  return jsonResponse(
    {
      name,
      version,
      tarballURL,
      signature,
      algorithm: "ed25519",
      publicKey: publicKeyHex(env) ?? null,
      signedFor: canonicalPluginString(name, version, tarballURL),
    },
    200,
    { "Cache-Control": "public, max-age=300" },
  );
}
