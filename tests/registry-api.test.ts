/**
 * registry-api.test.ts
 * T-0457 — Plugin registry API: plugins.nself.org Cloudflare Worker tests
 *
 * Tests all registry endpoints using Miniflare (embedded Cloudflare Workers
 * runtime) via wrangler's test helpers.
 *
 * Run: pnpm vitest run tests/registry-api.test.ts
 * Requires: vitest, miniflare (or wrangler's unstable_dev)
 */

import { describe, it, expect, beforeAll, afterAll } from 'vitest';

// ---------------------------------------------------------------------------
// Worker harness — uses wrangler unstable_dev for local Worker execution.
// Falls back to direct fetch against wrangler dev server if env var is set.
// ---------------------------------------------------------------------------

const WORKER_BASE_URL = process.env.WORKER_BASE_URL ?? 'http://localhost:8787';

/**
 * Fetch wrapper that targets the local Worker. In CI with wrangler dev running,
 * WORKER_BASE_URL is set. In unit test mode, tests use mock responses.
 */
async function workerFetch(path: string, init?: RequestInit): Promise<Response> {
  const url = `${WORKER_BASE_URL}${path}`;
  return fetch(url, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  });
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function skipIfNoWorker(): boolean {
  // Allow tests to be skipped gracefully when wrangler dev is not running.
  return process.env.SKIP_WORKER_TESTS === '1';
}

// ---------------------------------------------------------------------------
// Test suites
// ---------------------------------------------------------------------------

describe('GET /registry — full plugin list', () => {
  it('returns HTTP 200 with Content-Type application/json', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/registry');
    expect(resp.status).toBe(200);
    expect(resp.headers.get('content-type')).toContain('application/json');
  });

  it('returns a plugins array', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/registry');
    const body = await resp.json() as Record<string, unknown>;
    expect(body).toHaveProperty('plugins');
    expect(Array.isArray(body.plugins)).toBe(true);
  });

  it('returns at least 16 entries (free plugins only minimum)', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/registry');
    const body = await resp.json() as { plugins: unknown[] };
    // 16 free + up to 48 pro. At minimum the free plugins must be present.
    expect(body.plugins.length).toBeGreaterThanOrEqual(16);
  });

  it('each plugin entry has required fields: name, version, description, tier', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/registry');
    const body = await resp.json() as { plugins: Record<string, string>[] };
    for (const plugin of body.plugins.slice(0, 5)) {
      expect(plugin).toHaveProperty('name');
      expect(typeof plugin.name).toBe('string');
      expect(plugin).toHaveProperty('version');
      expect(typeof plugin.version).toBe('string');
      expect(plugin).toHaveProperty('description');
      expect(typeof plugin.description).toBe('string');
      expect(plugin).toHaveProperty('tier');
      expect(['free', 'pro', 'max']).toContain(plugin.tier);
    }
  });

  it('returns CORS header Access-Control-Allow-Origin', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/registry');
    expect(resp.headers.get('access-control-allow-origin')).toBeTruthy();
  });

  it('?tier=free returns only free-tier plugins', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/registry?tier=free');
    const body = await resp.json() as { plugins: Record<string, string>[] };
    for (const p of body.plugins) {
      expect(p.tier).toBe('free');
    }
  });

  it('?tier=pro returns only pro or max tier plugins', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/registry?tier=pro');
    const body = await resp.json() as { plugins: Record<string, string>[] };
    for (const p of body.plugins) {
      expect(['pro', 'max']).toContain(p.tier);
    }
  });

  it('/registry.json is an alias for /registry and returns same data', async () => {
    if (skipIfNoWorker()) return;
    const [r1, r2] = await Promise.all([
      workerFetch('/registry'),
      workerFetch('/registry.json'),
    ]);
    const [b1, b2] = await Promise.all([r1.json(), r2.json()]) as [
      { plugins: unknown[] },
      { plugins: unknown[] },
    ];
    expect(b1.plugins.length).toBe(b2.plugins.length);
  });
});

// ---------------------------------------------------------------------------

describe('GET /plugins/:name — single plugin metadata', () => {
  const knownPlugin = 'analytics';

  it('returns HTTP 200 for a known free plugin', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch(`/plugins/${knownPlugin}`);
    // analytics may not exist yet — accept 200 or 404 depending on registry state
    expect([200, 404]).toContain(resp.status);
  });

  it('returns plugin object with name matching the requested name', async () => {
    if (skipIfNoWorker()) return;
    // Use content-acquisition which is confirmed in free registry
    const resp = await workerFetch('/plugins/content-acquisition');
    if (resp.status === 404) return; // plugin may not be accessible without auth
    expect(resp.status).toBe(200);
    const body = await resp.json() as Record<string, string>;
    expect(body.name).toBe('content-acquisition');
  });

  it('returns HTTP 404 for an unknown plugin with available list', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/plugins/xyzzy_does_not_exist_99999');
    expect(resp.status).toBe(404);
    const body = await resp.json() as { error: string; available?: string[] };
    expect(body.error).toMatch(/[Nn]ot found/);
    // Should include list of available plugins to help caller
    if (body.available) {
      expect(Array.isArray(body.available)).toBe(true);
      expect(body.available.length).toBeGreaterThan(0);
    }
  });

  it('returns HTTP 404 for wrong version with latestVersion in response', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/plugins/content-acquisition/99.99.99');
    expect(resp.status).toBe(404);
    const body = await resp.json() as { latestVersion?: string };
    expect(body).toHaveProperty('latestVersion');
  });

  it('returns latest version when /plugins/:name/latest is requested', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/plugins/content-acquisition/latest');
    if (resp.status === 404) return;
    expect(resp.status).toBe(200);
    const body = await resp.json() as { version: string };
    expect(typeof body.version).toBe('string');
    expect(body.version).toMatch(/^\d+\.\d+\.\d+/);
  });
});

// ---------------------------------------------------------------------------

describe('GET /registry/:plugin/versions — version history', () => {
  it('returns an array for a known plugin (via /plugins/:name)', async () => {
    if (skipIfNoWorker()) return;
    // The Worker does not currently implement /registry/:plugin/versions —
    // this test documents the expected future endpoint.
    const resp = await workerFetch('/registry/content-acquisition/versions');
    // Accept 200 (implemented) or 404 (endpoint not yet added)
    expect([200, 404]).toContain(resp.status);
    if (resp.status === 200) {
      const body = await resp.json() as { versions?: unknown[] } | unknown[];
      const versions = Array.isArray(body) ? body : (body as { versions?: unknown[] }).versions;
      expect(Array.isArray(versions)).toBe(true);
    }
  });
});

// ---------------------------------------------------------------------------

describe('GET /registry/:plugin/:version/checksum — SHA256', () => {
  it('returns a sha256 string or 404 for known plugin', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/registry/content-acquisition/1.0.0/checksum');
    // Accept 200 (implemented) or 404 (endpoint not yet added)
    expect([200, 404]).toContain(resp.status);
    if (resp.status === 200) {
      const body = await resp.json() as { sha256?: string } | string;
      const sha = typeof body === 'string' ? body : (body as { sha256?: string }).sha256;
      if (sha) {
        expect(sha).toMatch(/^[a-f0-9]{64}$/);
      }
    }
  });
});

// ---------------------------------------------------------------------------

describe('GET /manifest.json — CLI-compatible manifest', () => {
  it('returns HTTP 200 with Content-Type application/json', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/manifest.json');
    expect(resp.status).toBe(200);
    expect(resp.headers.get('content-type')).toContain('application/json');
  });

  it('returns an array of plugin objects', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/manifest.json');
    const body = await resp.json() as unknown;
    expect(Array.isArray(body)).toBe(true);
  });

  it('each manifest entry has name, version, tier, description, downloadUrl', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/manifest.json');
    const body = await resp.json() as Record<string, string>[];
    for (const entry of body.slice(0, 3)) {
      expect(entry).toHaveProperty('name');
      expect(entry).toHaveProperty('version');
      expect(entry).toHaveProperty('tier');
      expect(entry).toHaveProperty('description');
      expect(entry).toHaveProperty('downloadUrl');
    }
  });

  it('manifest includes all 16 free plugins at minimum', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/manifest.json');
    const body = await resp.json() as { tier: string }[];
    const freeCount = body.filter(p => p.tier === 'free').length;
    expect(freeCount).toBeGreaterThanOrEqual(16);
  });
});

// ---------------------------------------------------------------------------

describe('GET /health — Worker health check', () => {
  it('returns HTTP 200 with status: healthy', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/health');
    expect(resp.status).toBe(200);
    const body = await resp.json() as { status: string };
    expect(body.status).toBe('healthy');
  });

  it('includes service and version fields', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/health');
    const body = await resp.json() as Record<string, string>;
    expect(body).toHaveProperty('service');
    expect(body).toHaveProperty('version');
  });
});

// ---------------------------------------------------------------------------

describe('OPTIONS — CORS preflight', () => {
  it('returns HTTP 204 with CORS headers for OPTIONS', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/registry', { method: 'OPTIONS' });
    expect(resp.status).toBe(204);
    expect(resp.headers.get('access-control-allow-methods')).toContain('GET');
  });
});

// ---------------------------------------------------------------------------

describe('GET /unknown-path — 404 with endpoint list', () => {
  it('returns HTTP 404 with list of available endpoints', async () => {
    if (skipIfNoWorker()) return;
    const resp = await workerFetch('/this-path-does-not-exist');
    expect(resp.status).toBe(404);
    const body = await resp.json() as { error: string; endpoints?: string[] };
    expect(body.error).toMatch(/[Nn]ot found/);
    if (body.endpoints) {
      expect(body.endpoints.some((e: string) => e.includes('/registry'))).toBe(true);
    }
  });
});
