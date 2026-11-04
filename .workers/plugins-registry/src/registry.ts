/**
 * Registry loaders: Cache API, then the KV copy POST /api/sync wrote, then GitHub
 * (see synced-store.ts). Only the sync (bypass = true) writes KV.
 */
import { loadSynced } from "./synced-store.ts";
import { normaliseToArray, decodeGitHubContent } from "./registry-types.ts";
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
  const data = await loadSynced(env, ctx, KV_FREE, bypass, () => freeFromGitHub(env));
  return data === null ? [] : normaliseToArray(data, "free");
}

async function freeFromGitHub(env: Env): Promise<RegistryWireFormat> {
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
  return data;
}

export async function fetchProRegistry(
  env: Env,
  ctx: ExecutionContext,
  bypass = false,
): Promise<PluginEntry[] | null> {
  // Cache first (as the retired index.js did): a copy synced by /api/sync serves without a token.
  const data = await loadSynced(env, ctx, KV_PRO, bypass, () => proFromGitHub(env));
  return data === null ? null : normaliseToArray(data, "pro");
}

async function proFromGitHub(env: Env): Promise<RegistryWireFormat | null> {
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
  return data;
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
