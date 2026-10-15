import { ratingsKv } from "./registry.ts";
import type { MarketplaceEnv } from "./marketplace-types.ts";
import { readInstallCount, sanitisePluginName } from "./marketplace-data.ts";
import { invalidateMarketplace } from "./marketplace-cache.ts";

const KV_INSTALL_PREFIX = "installs:";
const KV_DEDUP_PREFIX = "dedup:";

function isoWeek(date: Date): string {
  // Algorithm per ISO 8601: week starts on Monday
  const d = new Date(Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()));
  const day = d.getUTCDay() || 7; // make Sunday = 7
  d.setUTCDate(d.getUTCDate() + 4 - day);
  const yearStart = new Date(Date.UTC(d.getUTCFullYear(), 0, 1));
  const week = Math.ceil(((d.getTime() - yearStart.getTime()) / 86400000 + 1) / 7);
  return `${d.getUTCFullYear()}-W${String(week).padStart(2, "0")}`;
}

// ---------------------------------------------------------------------------
// POST /plugins/:name/install-event
// Body: { instanceId: string }
// instanceId MUST be an opaque hash (SHA-256 hex, no PII).
// Dedup: (instanceId, pluginName, week) — same install in same week is no-op.
// ---------------------------------------------------------------------------

export interface InstallEventResult {
  ok: boolean;
  status: number;
  error?: string;
  name?: string;
  downloads?: number;
  incremented?: boolean;
}

export async function handleInstallEvent(
  pluginName: string,
  request: Request,
  env: MarketplaceEnv,
): Promise<InstallEventResult> {
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return { ok: false, status: 400, error: "Invalid JSON body" };
  }

  if (body === null || typeof body !== "object" || Array.isArray(body)) {
    return { ok: false, status: 400, error: "Body must be a JSON object" };
  }

  const raw = body as Record<string, unknown>;
  const instanceId = raw["instanceId"];

  // instanceId must be a 64-char hex string (SHA-256, no PII)
  if (typeof instanceId !== "string" || !/^[0-9a-f]{64}$/.test(instanceId)) {
    return {
      ok: false,
      status: 400,
      error: "instanceId must be a 64-character lowercase hex string (SHA-256 hash, no PII)",
    };
  }

  const safePlugin = sanitisePluginName(pluginName);
  if (!safePlugin) {
    return { ok: false, status: 400, error: "Invalid plugin name" };
  }

  const week = isoWeek(new Date());
  const dedupKey = `${KV_DEDUP_PREFIX}${instanceId}:${safePlugin}:${week}`;

  // Check dedup — KV free tier: ~1K writes/day. Using presence check (get returns null on miss).
  try {
    const existing = await ratingsKv(env).get(dedupKey, "text");
    if (existing !== null) {
      // Already counted this week — no-op
      const count = await readInstallCount(env, safePlugin);
      return { ok: true, status: 200, name: pluginName, downloads: count, incremented: false };
    }
  } catch {
    // KV read failure — safe to proceed (worst case we count twice, not miss)
  }

  // Increment counter
  const countKey = `${KV_INSTALL_PREFIX}${safePlugin}`;
  let newCount = 1;
  try {
    const current = await ratingsKv(env).get<{ count: number }>(countKey, "json");
    newCount = (current?.count ?? 0) + 1;
    await ratingsKv(env).put(countKey, JSON.stringify({ count: newCount, lastUpdated: new Date().toISOString() }));
  } catch {
    return { ok: false, status: 500, error: "Failed to update install count" };
  }

  // Write dedup key with 8-day TTL (covers full week + buffer)
  try {
    await ratingsKv(env).put(dedupKey, "1", { expirationTtl: 8 * 24 * 60 * 60 });
  } catch {
    // Best-effort dedup — not fatal
  }

  // Invalidate marketplace cache so next fetch reflects updated count
  try {
    await invalidateMarketplace();
  } catch {
    // Best-effort
  }

  return { ok: true, status: 200, name: pluginName, downloads: newCount, incremented: true };
}
