/**
 * bundles.test.mjs — GET /bundles.json route + schema validation (P6-E4-W3-S3-T8, ADR-P6-03)
 *
 * Covers validateBundlesJson() (structural check against bundles-schema.json's
 * constraints, without a JSON-Schema library dependency) and handleBundlesJson()
 * (KV cache hit/miss, upstream fetch failure -> 502, schema failure -> 502).
 * Follows index.test.mjs's convention: Node's built-in test runner against
 * index.js directly (this package's real, deployed source — no vitest/jest).
 *
 * Run: node --test src/*.test.mjs
 * (from .workers/plugins-registry)
 */

import { test, describe, beforeEach, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import { validateBundlesJson, handleBundlesJson } from './index.js';

// ---------------------------------------------------------------------------
// validateBundlesJson() — structural schema check
// ---------------------------------------------------------------------------

function validBundle(overrides = {}) {
  return {
    display: 'Task Bundle',
    tier: 'free',
    price_monthly: 0,
    price_yearly: 0,
    saas: 'task.nself.org',
    page: 'nself.org/task',
    plugins: ['notifications', 'jobs'],
    ...overrides,
  };
}

function validBundlesFile() {
  return {
    schema_version: '2.0.0',
    bundles: {
      task: validBundle(),
      chat: validBundle({ tier: 'paid', price_monthly: 0.99, price_yearly: 9.99, saas: 'chat.nself.org', page: 'nself.org/chat' }),
      claw: validBundle({ tier: 'paid', price_monthly: 0.99, price_yearly: 9.99, saas: 'claw.nself.org', page: 'nself.org/claw' }),
      family: validBundle({ tier: 'paid', price_monthly: 0.99, price_yearly: 9.99, saas: 'family.nself.org', page: 'nself.org/family' }),
      sentry: validBundle({ tier: 'paid', price_monthly: 0.99, price_yearly: 9.99, saas: 'sentry.nself.org', page: 'nself.org/sentry' }),
      clawde: validBundle({ tier: 'paid', price_monthly: 0.99, price_yearly: 9.99, saas: null, page: 'clawde.io' }),
    },
  };
}

describe('validateBundlesJson()', () => {
  test('a well-formed bundles.json (6 canonical slugs) passes', () => {
    const { valid, errors } = validateBundlesJson(validBundlesFile());
    assert.equal(valid, true);
    assert.deepEqual(errors, []);
  });

  test('non-object input fails', () => {
    assert.equal(validateBundlesJson(null).valid, false);
    assert.equal(validateBundlesJson('not json').valid, false);
    assert.equal(validateBundlesJson(42).valid, false);
  });

  test('missing schema_version fails', () => {
    const data = validBundlesFile();
    delete data.schema_version;
    const { valid, errors } = validateBundlesJson(data);
    assert.equal(valid, false);
    assert.ok(errors.some(e => e.includes('schema_version')));
  });

  test('a 7th slug (e.g. stale "tv") fails — TV Bundle retired 2026-08-31, schema is 6 slugs only', () => {
    const data = validBundlesFile();
    data.bundles.tv = validBundle({ tier: 'paid' });
    const { valid, errors } = validateBundlesJson(data);
    assert.equal(valid, false);
    assert.ok(errors.some(e => e.includes('unexpected bundle slug') && e.includes('tv')));
  });

  test('a missing canonical slug fails', () => {
    const data = validBundlesFile();
    delete data.bundles.clawde;
    const { valid, errors } = validateBundlesJson(data);
    assert.equal(valid, false);
    assert.ok(errors.some(e => e.includes('missing bundle slug') && e.includes('clawde')));
  });

  test('a bundle entry missing a required field fails', () => {
    const data = validBundlesFile();
    delete data.bundles.task.plugins;
    const { valid, errors } = validateBundlesJson(data);
    assert.equal(valid, false);
    assert.ok(errors.some(e => e.includes('task') && e.includes('plugins')));
  });

  test('an invalid tier value fails', () => {
    const data = validBundlesFile();
    data.bundles.task.tier = 'premium';
    const { valid, errors } = validateBundlesJson(data);
    assert.equal(valid, false);
    assert.ok(errors.some(e => e.includes('invalid tier')));
  });
});

// ---------------------------------------------------------------------------
// handleBundlesJson() — route behaviour (KV hit/miss, fetch failure, schema failure)
// ---------------------------------------------------------------------------

const CTX = { waitUntil: () => {} };
const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
});

describe('handleBundlesJson()', () => {
  test('no GH_ACCESS_TOKEN and no KV cache -> 502, never calls fetch', async () => {
    let fetchCalled = false;
    globalThis.fetch = async () => { fetchCalled = true; throw new Error('should not be called'); };

    const env = {}; // no PLUGINS_KV, no GH_ACCESS_TOKEN
    const res = await handleBundlesJson(env, CTX);
    assert.equal(res.status, 502);
    assert.equal(fetchCalled, false);
  });

  test('fresh KV cache -> 200 with cached data, X-Cache: HIT, never calls fetch', async () => {
    let fetchCalled = false;
    globalThis.fetch = async () => { fetchCalled = true; throw new Error('should not be called'); };

    const cachedData = validBundlesFile();
    const env = {
      PLUGINS_KV: { get: async () => ({ data: cachedData, timestamp: Date.now() }) },
    };
    const res = await handleBundlesJson(env, CTX);
    assert.equal(res.status, 200);
    assert.equal(res.headers.get('X-Cache'), 'HIT');
    assert.equal(res.headers.get('Cache-Control'), 'public, s-maxage=60, stale-while-revalidate=300');
    const body = await res.json();
    assert.deepEqual(Object.keys(body.bundles), Object.keys(cachedData.bundles));
    assert.equal(fetchCalled, false);
  });

  test('GH API fetch failure -> 502', async () => {
    globalThis.fetch = async () => new Response('rate limited', { status: 403 });
    const env = { GH_ACCESS_TOKEN: 'fake-token' };
    const res = await handleBundlesJson(env, CTX);
    assert.equal(res.status, 502);
  });

  test('GH API returns content that fails schema validation -> 502 with details', async () => {
    const invalid = validBundlesFile();
    delete invalid.bundles.task; // now missing a canonical slug
    globalThis.fetch = async () => new Response(
      JSON.stringify({ content: Buffer.from(JSON.stringify(invalid)).toString('base64') }),
      { status: 200 },
    );
    const env = { GH_ACCESS_TOKEN: 'fake-token' };
    const res = await handleBundlesJson(env, CTX);
    assert.equal(res.status, 502);
    const body = await res.json();
    assert.match(body.error, /schema validation/);
    assert.ok(Array.isArray(body.details) && body.details.length > 0);
  });

  test('GH API returns valid bundles.json -> 200, X-Cache: MISS, correct Cache-Control', async () => {
    const valid = validBundlesFile();
    globalThis.fetch = async () => new Response(
      JSON.stringify({ content: Buffer.from(JSON.stringify(valid)).toString('base64') }),
      { status: 200 },
    );
    const env = { GH_ACCESS_TOKEN: 'fake-token' }; // no PLUGINS_KV -> cache read is a no-op miss
    const res = await handleBundlesJson(env, CTX);
    assert.equal(res.status, 200);
    assert.equal(res.headers.get('X-Cache'), 'MISS');
    assert.equal(res.headers.get('Cache-Control'), 'public, s-maxage=60, stale-while-revalidate=300');
    const body = await res.json();
    assert.deepEqual(Object.keys(body.bundles).sort(), ['chat', 'claw', 'clawde', 'family', 'sentry', 'task']);
  });
});
