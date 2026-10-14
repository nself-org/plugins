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

import type {
  Env,
  RevocationEntry,
  RevocationListResponse,
  RevokedAuthorEntry,
  RevokedAuthorListResponse,
} from "./registry.ts";
import { jsonResponse } from "./http.ts";

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
    if (!Array.isArray(parsed)) return [];
    // The retired index.js wrote { plugin, version, ... }; read both spellings.
    return (parsed as Array<RevocationEntry & { plugin?: string }>).map((r) =>
      r.name === undefined && r.plugin !== undefined ? { ...r, name: r.plugin } : r,
    );
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

  // `revoked` is the key the CLI reads; `revocations` is the retired index.js
  // spelling, kept so any client written against the deployed worker still parses.
  const body: RevocationListResponse & { revocations: RevocationEntry[] } = {
    revoked: list,
    revocations: list,
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

// ---------------------------------------------------------------------------
// GET /.well-known/revoked-authors.json — Author Certificate Revocation List
// (S58-T09)
//
// CLI checks this endpoint on every `nself plugin install` and via daily cron
// to detect plugins whose author keys have been revoked (abuse, security
// breach, license fraud). Short max-age (60 s) because revocations are
// security-critical and must propagate quickly.
//
// KV key: revocations:authors  — JSON array of RevokedAuthorEntry
// The list is append-only: once revoked, entries must never be deleted.
// ---------------------------------------------------------------------------

const KV_REVOKED_AUTHORS = "revocations:authors";

export async function handleRevokedAuthors(env: Env): Promise<Response> {
  const kv = env.REGISTRY ?? env.PLUGINS_KV;
  let revokedAuthors: RevokedAuthorEntry[] = [];

  if (kv) {
    try {
      const raw = await kv.get(KV_REVOKED_AUTHORS, "text");
      if (raw) {
        const parsed = JSON.parse(raw) as unknown;
        if (Array.isArray(parsed)) {
          revokedAuthors = parsed as RevokedAuthorEntry[];
        }
      }
    } catch {
      // An unavailable revocation list must not become a cached all-clear.
      return jsonResponse(
        { error: "Author revocation list unavailable" },
        503,
        { "Cache-Control": "no-store" },
      );
    }
  }

  const body: RevokedAuthorListResponse = {
    revokedAuthors,
    fetchedAt: new Date().toISOString(),
    count: revokedAuthors.length,
  };

  // Short TTL: revocations are security-critical. CLI must see them promptly.
  return jsonResponse(body, 200, { "Cache-Control": "public, max-age=60" });
}
