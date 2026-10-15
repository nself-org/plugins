/** Registry loaders: cache locally, fetch upstream on misses; no edge cache mutations. */
import { edgeGetWrapped, edgePutWrapped } from "./edge-cache.ts";
import { normaliseToArray, cacheTtl, isFresh, decodeGitHubContent } from "./registry-types.ts";
import type { Env, PluginEntry, RegistryWireFormat, GitHubContentsResponse } from "./registry-types.ts";
export * from "./registry-types.ts";
export * from "./bundles-data.ts";

const FREE_REGISTRY_API_URL =
  "https://api.github.com/repos/nself-org/plugins/contents/registry.json";
const PRO_REGISTRY_API_URL =
  "https://api.github.com/repos/nself-org/bundles/contents/registry.json";
const FREE_REGISTRY_RAW_URL =
  "https://raw.githubusercontent.com/nself-org/plugins/main/registry.json";

const KV_FREE = "registry:free";
const KV_PRO = "registry:pro";

export async function fetchFreeRegistry(
  env: Env,
  ctx: ExecutionContext,
  bypass = false,
): Promise<PluginEntry[]> {
  const ttl = cacheTtl(env);

  if (!bypass) {
    const cached = await edgeGetWrapped<RegistryWireFormat>(KV_FREE);
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

  ctx.waitUntil(edgePutWrapped(KV_FREE, data));
  return normaliseToArray(data, "free");
}

export async function fetchProRegistry(
  env: Env,
  ctx: ExecutionContext,
  bypass = false,
): Promise<PluginEntry[] | null> {
  const ttl = cacheTtl(env);

  // Cache first (as the retired index.js did): a edge copy synced by /api/sync serves without a token.
  if (!bypass) {
    const cached = await edgeGetWrapped<RegistryWireFormat>(KV_PRO);
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

  ctx.waitUntil(edgePutWrapped(KV_PRO, data));
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
