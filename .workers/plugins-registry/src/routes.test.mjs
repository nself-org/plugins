/**
 * routes.test.mjs — route inventory and licence-gate adversarial tests (P7-PLUG-08)
 *
 * 1. Route inventory: every route that src/index.js (the file wrangler used to
 *    deploy) or src/index.ts served at the P7-PLUG-08 basis still answers from
 *    the single remaining source, src/index.ts.
 * 2. Adversarial: path tricks on the tarball route (encoded slashes, dot
 *    segments, case changes, version aliases, doubled slashes, prefixes,
 *    methods, hostile registry tier values) never hand out a licensed tarball.
 *
 * Runs against dist-test/index.mjs (esbuild bundle of src/index.ts). All data is
 * synthetic and served from a fake KV, so nothing here touches the network.
 *
 * Run: pnpm test  (from .workers/plugins-registry)
 */

import { test, describe, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import worker from '../dist-test/index.mjs';
import { fakeKv, fakeCtx, envelope, registryKv, req } from './test-helpers.mjs';

const originalFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = originalFetch;
});

const GITHUB_RELEASES = /^https:\/\/github\.com\/nself-org\//;

async function call(path, init, env = { PLUGINS_KV: registryKv() }) {
  const ctx = fakeCtx();
  const res = await worker.fetch(req(path, init), env, ctx);
  await ctx.settle();
  return res;
}

// ---------------------------------------------------------------------------
// 1. Route inventory
// ---------------------------------------------------------------------------

// [source, method, path, expected status]. source: js = served by index.js at
// the basis, ts = served by index.ts at the basis, both = served by both.
const INVENTORY = [
  ['both', 'GET', '/registry.json', 200],
  ['both', 'GET', '/registry', 200],
  ['both', 'GET', '/', 200],
  ['ts', 'GET', '/registry/list', 200],
  ['both', 'GET', '/plugins', 200],
  ['both', 'GET', '/plugins/revocations', 200],
  ['both', 'GET', '/plugins/jobs', 200],
  ['both', 'GET', '/plugins/jobs/1.2.0', 200],
  ['both', 'GET', '/plugins/jobs/latest', 200],
  ['both', 'GET', '/plugins/jobs/tarball', 302],
  ['both', 'GET', '/plugins/jobs/signature', 200],
  ['both', 'GET', '/categories', 200],
  ['both', 'GET', '/manifest.json', 200],
  ['both', 'GET', '/health', 200],
  ['both', 'GET', '/stats', 200],
  ['ts', 'GET', '/.well-known/revoked-authors.json', 200],
  ['both', 'GET', '/marketplace', 200],
  ['both', 'GET', '/marketplace/ratings/jobs', 200],
  ['both', 'POST', '/marketplace/ratings/jobs', 400], // empty body: rejected, but the route exists
  ['ts', 'POST', '/plugins/jobs/install-event', 400], // no instanceId: rejected, but the route exists
  ['both', 'POST', '/api/sync', 401], // no bearer token
  ['new', 'GET', '/v1/plugins', 200], // ts /v1 prefix
];

describe('route inventory (every basis route still answers from src/index.ts)', () => {
  for (const [source, method, path, status] of INVENTORY) {
    test(`${method} ${path} -> ${status} (${source})`, async () => {
      // /plugins/jobs/signature without a signing key answers 200 with signature: null
      const res = await call(path, { method, body: method === 'POST' ? '{}' : undefined });
      assert.equal(res.status, status);
    });
  }

  test('GET /bundles.json and /bundles-schema.json answer 502 with no upstream (route exists, not 404)', async () => {
    for (const path of ['/bundles.json', '/bundles-schema.json']) {
      const res = await call(path, {}, { PLUGINS_KV: fakeKv() });
      assert.equal(res.status, 502, path);
    }
  });

  test('GET /catalog.json (new) answers', async () => {
    globalThis.fetch = async () => new Response(JSON.stringify({ plugins: {} }), { status: 200 });
    assert.equal((await call('/catalog.json', {}, { PLUGINS_KV: fakeKv() })).status, 200);
  });

  test('OPTIONS answers 204 with CORS on any path', async () => {
    const res = await call('/anything', { method: 'OPTIONS' });
    assert.equal(res.status, 204);
    assert.equal(res.headers.get('Access-Control-Allow-Origin'), '*');
  });

  test('an unknown path is a 404 that lists the endpoints, catalog.json included', async () => {
    const res = await call('/nope');
    assert.equal(res.status, 404);
    const body = await res.json();
    assert.ok(body.endpoints.includes('GET /catalog.json'));
    for (const e of ['GET /registry.json', 'GET /manifest.json', 'GET /bundles.json', 'POST /api/sync']) {
      assert.ok(body.endpoints.includes(e), e);
    }
  });

  test('/health reports signing configured with the legacy SIGNING_PUBLIC_KEY name too', async () => {
    const a = await (await call('/health', {}, { SIGNING_PRIVATE_KEY: 'x', PUBLIC_KEY_HEX: 'y' })).json();
    const b = await (await call('/health', {}, { SIGNING_PRIVATE_KEY: 'x', SIGNING_PUBLIC_KEY: 'y' })).json();
    const c = await (await call('/health', {}, { SIGNING_PRIVATE_KEY: 'x' })).json();
    assert.equal(a.signing.configured, true);
    assert.equal(b.signing.configured, true);
    assert.equal(c.signing.configured, false);
    assert.ok(Array.isArray(a.features) && a.timestamp);
    assert.equal(a.status, 'healthy', 'publish.yml and deploy.sh check for "healthy"');
  });

  test('/plugins/revocations serves `revoked` (CLI) and `revocations` (legacy), reading name or legacy plugin', async () => {
    const kv = registryKv({
      'revocations:list': JSON.stringify([
        { name: 'jobs', version: '1.0.0', revokedAt: 'x' },
        { plugin: 'notify', version: '2.0.0', revokedAt: 'x' },
      ]),
    });
    const body = await (await call('/plugins/revocations', {}, { PLUGINS_KV: kv })).json();
    assert.equal(body.count, 2);
    assert.deepEqual(body.revoked.map((r) => r.name), ['jobs', 'notify']);
    assert.deepEqual(body.revocations, body.revoked);
  });

  test('ratings and marketplace work with only PLUGINS_KV bound (the deployed wrangler.toml)', async () => {
    const kv = registryKv();
    const post = await call(
      '/marketplace/ratings/jobs',
      {
        method: 'POST',
        headers: { 'CF-Connecting-IP': '203.0.113.7' },
        body: JSON.stringify({ rating: 5, comment: 'ok', userHash: 'a'.repeat(64) }),
      },
      { PLUGINS_KV: kv },
    );
    assert.equal(post.status, 200);
    const got = await (await call('/marketplace/ratings/jobs', {}, { PLUGINS_KV: kv })).json();
    assert.equal(got.reviewCount, 1);
    assert.equal(got.rating, 5);
  });

  test('registry entries pass every field through (the CLI reads checksums, download_url, tables, ...)', async () => {
    const full = {
      version: '1.2.0', description: 'Jobs', category: 'infrastructure', tarballURL: 'https://example.invalid/jobs.tgz',
      checksums: { sha256: 'a'.repeat(64) }, download_url: 'https://example.invalid/dl', requires_license: false,
      implementation: { language: 'go', entry: 'cmd/main.go' }, dependencies: { required: ['database'], optional: [] },
      tables: ['np_jobs_queue'], port: 3201, tier_pair: false, bundles: ['task'], minNselfVersion: '1.0.0', tarballUrl: 'x',
    };
    const kv = fakeKv({ 'registry:free': envelope({ plugins: { jobs: full } }) });
    const one = await (await call('/plugins/jobs', {}, { PLUGINS_KV: kv })).json();
    for (const [k, v] of Object.entries(full)) assert.deepEqual(one[k], v, `/plugins/jobs ${k}`);
    const reg = await (await call('/registry.json', {}, { PLUGINS_KV: kv })).json();
    for (const [k, v] of Object.entries(full)) assert.deepEqual(reg.plugins[0][k], v, `/registry.json ${k}`);
    assert.equal(one.name, 'jobs');
    assert.equal(one.tier, 'free');
  });

  test('/registry.json: only tier=free|pro filters (tier=bogus lists everything), X-Tier header', async () => {
    const all = await (await call('/registry.json')).json();
    const bogus = await call('/registry.json?tier=bogus');
    assert.equal(bogus.headers.get('X-Tier'), 'combined');
    assert.equal((await bogus.json()).plugins.length, all.plugins.length);
    const pro = await call('/registry.json?tier=pro');
    assert.equal(pro.headers.get('X-Tier'), 'pro');
    assert.ok((await pro.json()).plugins.every((p) => p.tier === 'pro'));
  });

  test('/registry.json is a 502 and is not cached when both sources fail', async () => {
    globalThis.fetch = async () => new Response('down', { status: 500 });
    const kv = fakeKv();
    const res = await call('/registry.json', {}, { PLUGINS_KV: kv });
    assert.equal(res.status, 502);
    assert.equal(kv.store.has('registry:combined'), false);
    assert.equal(kv.store.has('registry:free'), false);
  });

  test('/stats counts registry fetches and hits (flat record, as index.js stored it)', async () => {
    const kv = registryKv();
    await call('/registry.json', {}, { PLUGINS_KV: kv });
    await call('/registry.json', {}, { PLUGINS_KV: kv });
    const stats = await (await call('/stats', {}, { PLUGINS_KV: kv })).json();
    assert.equal(stats.registryFetches, 1);
    assert.equal(stats.registryHits, 1);
  });

  test('GET ratings keeps the old ratingCount field next to reviewCount', async () => {
    const body = await (await call('/marketplace/ratings/jobs')).json();
    assert.ok('reviewCount' in body && 'ratingCount' in body);
    assert.equal(body.ratingCount, body.reviewCount);
  });

  test('a cached pro registry serves without GH_ACCESS_TOKEN (cache-first, as index.js did)', async () => {
    const body = await (await call('/plugins')).json();
    assert.ok(body.plugins.some((p) => p.name === 'ai'));
  });
});

// ---------------------------------------------------------------------------
// 2. Adversarial: the licence gate on /plugins/:name/tarball
// ---------------------------------------------------------------------------

/** A licensed tarball was handed out: a redirect, or any Location header. */
function assertNoTarball(res, label) {
  assert.notEqual(res.status, 302, `${label}: 302`);
  assert.equal(res.headers.get('Location'), null, `${label}: Location header`);
  assert.ok(![301, 303, 307, 308].includes(res.status), `${label}: redirect status ${res.status}`);
}

const LICENSED = ['ai', 'claw', 'untiered', 'sneaky'];

describe('licence gate: paid tiers never get a tarball redirect', () => {
  for (const name of LICENSED) {
    test(`/plugins/${name}/tarball -> 401`, async () => {
      const res = await call(`/plugins/${name}/tarball`);
      assert.equal(res.status, 401);
      assertNoTarball(res, name);
      assert.match((await res.json()).download_url, /^https:\/\/ping\.nself\.org\/plugins\/.+\/download$/);
    });
  }

  test('registry tier values that are not exactly "free" are refused ("FREE", "free ", "max")', async () => {
    for (const name of ['upper', 'spaced', 'sneaky']) {
      const res = await call(`/plugins/${name}/tarball`);
      assert.equal(res.status, 401, name);
      assertNoTarball(res, name);
    }
  });

  test('a registry entry with no tier inherits its registry: pro registry -> refused', async () => {
    assert.equal((await call('/plugins/untiered/tarball')).status, 401);
  });

  test('a licensed-registry entry that claims tier "free" is still licensed (pro registry forces pro)', async () => {
    const kv = fakeKv({
      'registry:free': envelope({ plugins: {} }),
      'registry:pro': envelope({ plugins: [{ name: 'liar', version: '1.0.0', tier: 'free' }, { name: 'liar2', version: '1.0.0', tier: 'enterprise' }] }),
    });
    for (const name of ['liar', 'liar2']) {
      const res = await call(`/plugins/${name}/tarball`, {}, { PLUGINS_KV: kv });
      assert.equal(res.status, 401, name);
      assertNoTarball(res, name);
    }
    const listed = await (await call('/plugins/liar', {}, { PLUGINS_KV: kv })).json();
    assert.equal(listed.tier, 'pro');
  });

  test('a name in both tiers resolves to the free entry and never to the licensed tarball', async () => {
    const res = await call('/plugins/notify/tarball');
    assert.equal(res.status, 302);
    assert.match(res.headers.get('Location'), /^https:\/\/github\.com\/nself-org\/plugins\/releases\/download\/v1\.0\.0\//);
    assert.doesNotMatch(res.headers.get('Location'), /7\.0\.0|bundles/);
  });

  const TRICKS = [
    // encoded slashes and dots
    '/plugins/ai%2Ftarball',
    '/plugins/ai%2ftarball',
    '/plugins/ai%2F..%2Ftarball',
    '/plugins/%61i/tarball',
    '/plugins/ai/%74arball',
    '/plugins/ai%00/tarball',
    '/plugins/ai%0d%0a/tarball',
    '/plugins/ai%5Ctarball',
    '/plugins/..%2Fai/tarball',
    // dot segments (the URL parser normalises these to the real path)
    '/plugins/jobs/../ai/tarball',
    '/plugins/jobs/%2e%2e/ai/tarball',
    '/plugins/jobs/.%2e/ai/tarball',
    '/plugins/./ai/tarball',
    '/plugins/ai/./tarball',
    '/plugins/ai/tarball/..',
    // case changes
    '/plugins/AI/tarball',
    '/plugins/Ai/Tarball',
    '/plugins/ai/TARBALL',
    '/PLUGINS/ai/tarball',
    // version aliases and extra segments
    '/plugins/ai/latest',
    '/plugins/ai/2.0.0',
    '/plugins/ai/v2.0.0',
    '/plugins/ai/2.0.0/tarball',
    '/plugins/ai/latest/tarball',
    '/plugins/ai/tarball/extra',
    '/plugins/ai/tarball/',
    '/plugins/ai/tarball?tier=free',
    '/plugins/ai/tarball?name=jobs',
    '/plugins/ai/tarball#jobs',
    '/plugins/ai/tarball%20',
    '/plugins/ai/tarball;jobs',
    // doubled slashes and prefixes
    '/plugins//ai/tarball',
    '//plugins/ai/tarball',
    '/v1/plugins/ai/tarball',
    '/v1//plugins/ai/tarball',
    '/v1/v1/plugins/ai/tarball',
    '/registry/list/../plugins/ai/tarball',
    '/marketplace/../plugins/ai/tarball',
    '/.well-known/../plugins/ai/tarball',
  ];

  for (const path of TRICKS) {
    test(`GET ${path} never returns a licensed tarball`, async () => {
      const res = await call(path);
      assertNoTarball(res, path);
      // whatever it returns, it must not be the licensed GitHub release
      assert.doesNotMatch(res.headers.get('Location') ?? '', GITHUB_RELEASES);
    });
  }

  test('methods other than GET never reach the tarball handler', async () => {
    for (const method of ['POST', 'PUT', 'DELETE', 'PATCH']) {
      const res = await call('/plugins/ai/tarball', { method, body: '{}' });
      assertNoTarball(res, method);
      assert.equal(res.status, 404, method);
    }
    // HEAD is not routed either
    assertNoTarball(await call('/plugins/ai/tarball', { method: 'HEAD' }), 'HEAD');
  });

  test('request headers cannot unlock it (Authorization, X-Forwarded-*, X-Tier, Host override)', async () => {
    const res = await call('/plugins/ai/tarball', {
      headers: {
        Authorization: 'Bearer fake-license-key',
        'X-Forwarded-For': '127.0.0.1',
        'X-Tier': 'free',
        'X-Original-URL': '/plugins/jobs/tarball',
        'X-Rewrite-URL': '/plugins/jobs/tarball',
        'X-API-Version': '1',
      },
    });
    assert.equal(res.status, 401);
    assertNoTarball(res, 'headers');
  });

  test('a revoked free version is 410 and a paid revoked version is still 401 (gate runs first)', async () => {
    const kv = registryKv({ 'revocations:list': JSON.stringify([
      { name: 'jobs', version: '1.2.0', revokedAt: 'x' },
      { plugin: 'ai', version: '2.0.0', revokedAt: 'x' },
    ]) });
    assert.equal((await call('/plugins/jobs/tarball', {}, { PLUGINS_KV: kv })).status, 410);
    assert.equal((await call('/plugins/ai/tarball', {}, { PLUGINS_KV: kv })).status, 401);
  });

  test('registry data cannot smuggle a tier through the wire format (array and keyed forms)', async () => {
    const kv = fakeKv({
      'registry:free': envelope([
        { name: 'arr-pro', version: '1.0.0', tier: 'pro' },
        { name: 'arr-null', version: '1.0.0', tier: null },
        { name: 'arr-num', version: '1.0.0', tier: 1 },
        { name: 'arr-obj', version: '1.0.0', tier: { free: true } },
      ]),
    });
    for (const name of ['arr-pro', 'arr-num', 'arr-obj']) {
      const res = await call(`/plugins/${name}/tarball`, {}, { PLUGINS_KV: kv });
      assert.equal(res.status, 401, name);
      assertNoTarball(res, name);
    }
    // tier: null falls back to the registry it came from (free), the same rule as an absent tier
    assert.equal((await call('/plugins/arr-null/tarball', {}, { PLUGINS_KV: kv })).status, 302);
  });

  test('signature and metadata routes for a licensed plugin never carry a download Location', async () => {
    for (const path of ['/plugins/ai', '/plugins/ai/signature', '/plugins/ai/latest']) {
      assertNoTarball(await call(path), path);
    }
  });
});
