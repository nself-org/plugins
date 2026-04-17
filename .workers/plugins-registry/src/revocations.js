/**
 * revocations.js — Plugin revocation list endpoint
 *
 * KV key: "revocations:list"
 * Value: JSON array of { plugin, version, revokedAt, reason }
 *
 * GET /plugins/revocations returns the full list with metadata.
 * Cache-Control is short (60s) — revocations are security-critical and must
 * propagate quickly to clients.
 *
 * addRevocation() is called by internal admin flows (POST /api/revoke, not yet
 * exposed publicly). isRevoked() is used by the tarball download handler to
 * block serving revoked plugin versions.
 */

const KV_REVOCATIONS = 'revocations:list';

// ---------------------------------------------------------------------------
// GET /plugins/revocations handler
// Returns the full revocation list with count and fetchedAt timestamp.
// ---------------------------------------------------------------------------

export async function handleRevocations(env) {
  const list = await getRevocationList(env);
  return new Response(
    JSON.stringify(
      {
        revocations: list,
        fetchedAt:   new Date().toISOString(),
        count:       list.length,
      },
      null,
      2,
    ),
    {
      status:  200,
      headers: {
        'Content-Type':                'application/json',
        'Cache-Control':               'public, max-age=60',
        'Access-Control-Allow-Origin': '*',
      },
    },
  );
}

// ---------------------------------------------------------------------------
// Add a revocation entry to the KV list.
// Idempotent: if plugin@version is already revoked, updates the record.
// ---------------------------------------------------------------------------

export async function addRevocation(env, plugin, version, reason) {
  if (!env.PLUGINS_KV) return;

  const list = await getRevocationList(env);

  const existing = list.findIndex(r => r.plugin === plugin && r.version === version);
  const entry = {
    plugin,
    version,
    revokedAt: new Date().toISOString(),
    reason:    reason || 'unspecified',
  };

  if (existing >= 0) {
    list[existing] = entry;
  } else {
    list.push(entry);
  }

  await env.PLUGINS_KV.put(KV_REVOCATIONS, JSON.stringify(list));
}

// ---------------------------------------------------------------------------
// Check whether a specific plugin version is revoked.
// Returns true if revoked, false otherwise.
// ---------------------------------------------------------------------------

export async function isRevoked(env, pluginName, version) {
  const list = await getRevocationList(env);
  return list.some(r => r.plugin === pluginName && r.version === version);
}

// ---------------------------------------------------------------------------
// Internal helper — fetch and parse the revocation list from KV.
// Returns an empty array when KV is unavailable or the key is absent.
// ---------------------------------------------------------------------------

async function getRevocationList(env) {
  if (!env.PLUGINS_KV) return [];
  try {
    const raw = await env.PLUGINS_KV.get(KV_REVOCATIONS, 'text');
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}
