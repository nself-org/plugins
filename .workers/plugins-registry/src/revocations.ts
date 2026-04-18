/**
 * revocations.ts — Plugin revocation list module
 *
 * KV key: "revocations:list"
 * Value: JSON array of RevocationEntry objects
 *
 * GET /plugins/revocations returns the full list.
 * Cache-Control is short (60s) — revocations are security-critical and must
 * propagate quickly to the CLI, which polls this endpoint hourly.
 *
 * isRevoked() is called by the tarball download handler to block serving
 * revoked plugin versions (returns HTTP 410 Gone).
 */

import type { Env, RevocationEntry, RevocationListResponse } from "./registry.ts";

const KV_REVOCATIONS = "revocations:list";

// ---------------------------------------------------------------------------
// Internal helper — fetch and parse the revocation list from KV
// ---------------------------------------------------------------------------

async function getRevocationList(env: Env): Promise<RevocationEntry[]> {
  const kv = env.REGISTRY ?? env.PLUGINS_KV;
  if (!kv) return [];

  try {
    const raw = await kv.get(KV_REVOCATIONS, "text");
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed) ? (parsed as RevocationEntry[]) : [];
  } catch {
    return [];
  }
}

// ---------------------------------------------------------------------------
// GET /plugins/revocations handler
// Returns the full revocation list with count and fetchedAt.
// ---------------------------------------------------------------------------

export async function handleRevocations(env: Env): Promise<Response> {
  const list = await getRevocationList(env);

  const body: RevocationListResponse = {
    revoked: list,
    fetchedAt: new Date().toISOString(),
    count: list.length,
  };

  return new Response(JSON.stringify(body, null, 2), {
    status: 200,
    headers: {
      "Content-Type": "application/json",
      "Cache-Control": "public, max-age=60",
      "Access-Control-Allow-Origin": "*",
    },
  });
}

// ---------------------------------------------------------------------------
// Check whether a specific plugin version is revoked.
// ---------------------------------------------------------------------------

export async function isRevoked(
  env: Env,
  pluginName: string,
  version: string,
): Promise<boolean> {
  const list = await getRevocationList(env);
  return list.some((r) => r.name === pluginName && r.version === version);
}

// ---------------------------------------------------------------------------
// Add or update a revocation entry.
// Idempotent: updating an existing name@version replaces the record.
// ---------------------------------------------------------------------------

export async function addRevocation(
  env: Env,
  pluginName: string,
  version: string,
  reason?: string,
): Promise<void> {
  const kv = env.REGISTRY ?? env.PLUGINS_KV;
  if (!kv) return;

  const list = await getRevocationList(env);
  const idx = list.findIndex((r) => r.name === pluginName && r.version === version);

  const entry: RevocationEntry = {
    name: pluginName,
    version,
    revokedAt: new Date().toISOString(),
    ...(reason !== undefined && { reason }),
  };

  if (idx >= 0) {
    list[idx] = entry;
  } else {
    list.push(entry);
  }

  await kv.put(KV_REVOCATIONS, JSON.stringify(list));
}
