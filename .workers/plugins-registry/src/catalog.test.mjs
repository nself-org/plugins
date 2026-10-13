/**
 * catalog.test.mjs — GET /catalog.json (P7-PLUG-08)
 *
 * Compact body, strong ETag (quoted sha256 hex of the served bytes), 304 on
 * If-None-Match, Cache-Control, KV cache, stale fallback, bad upstream data.
 * Runs against dist-test/index.mjs (esbuild bundle of src/index.ts).
 *
 * Run: pnpm test  (from .workers/plugins-registry)
 */

import { test, describe, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import worker from '../dist-test/index.mjs';
import {
  fakeKv, fakeCtx, envelope, sha256, contentsResponse, sampleCatalog, req,
} from './test-helpers.mjs';

const originalFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = originalFetch;
});

/** Stub fetch; records URLs; answers the Contents API and the raw URL from `doc`. */
function stubUpstream(doc, { apiStatus = 200, rawStatus = 200 } = {}) {
  const calls = [];
  globalThis.fetch = async (url) => {
    calls.push(String(url));
    if (String(url).startsWith('https://api.github.com/')) {
      return apiStatus === 200 ? contentsResponse(doc) : new Response('nope', { status: apiStatus });
    }
    return rawStatus === 200 ? new Response(JSON.stringify(doc, null, 2), { status: 200 }) : new Response('nope', { status: rawStatus });
  };
  return calls;
}

async function get(path, env, headers = {}) {
  const ctx = fakeCtx();
  const res = await worker.fetch(req(path, { headers }), env, ctx);
  await ctx.settle();
  return res;
}

describe('GET /catalog.json', () => {
  test('200: compact body, quoted sha256 ETag of the exact bytes, Cache-Control, JSON type', async () => {
    const doc = sampleCatalog();
    stubUpstream(doc);
    const res = await get('/catalog.json', { PLUGINS_KV: fakeKv(), GH_ACCESS_TOKEN: 'fake-token' });
    assert.equal(res.status, 200);
    const body = await res.text();
    assert.equal(body, JSON.stringify(doc), 'body is the compact serialisation');
    assert.ok(!body.includes('\n'), 'no pretty-printing');
    const etag = res.headers.get('ETag');
    assert.match(etag, /^"[0-9a-f]{64}"$/, 'strong (no W/), quoted, sha256 hex');
    assert.equal(etag, `"${sha256(body)}"`);
    assert.equal(res.headers.get('Cache-Control'), 'public, max-age=300');
    assert.match(res.headers.get('Content-Type'), /^application\/json/);
    assert.equal(res.headers.get('X-Cache'), 'MISS');
  });

  test('uses the Contents API with a token and the raw URL without one', async () => {
    let calls = stubUpstream(sampleCatalog());
    await get('/catalog.json', { PLUGINS_KV: fakeKv(), GH_ACCESS_TOKEN: 'fake-token' });
    assert.deepEqual(calls, ['https://api.github.com/repos/nself-org/plugins/contents/catalog.json']);

    calls = stubUpstream(sampleCatalog());
    const res = await get('/catalog.json', { PLUGINS_KV: fakeKv() });
    assert.deepEqual(calls, ['https://raw.githubusercontent.com/nself-org/plugins/main/catalog.json']);
    assert.equal(res.status, 200);
  });

  test('non-ASCII survives the Contents API (UTF-8 base64): same body and ETag as the raw URL', async () => {
    const doc = sampleCatalog({ bundles: { task: { display: '\u0273Task Bundle \u2192 \u2014', plugins: ['jobs'], tier: 'free' } } });
    stubUpstream(doc);
    const viaApi = await get('/catalog.json', { PLUGINS_KV: fakeKv(), GH_ACCESS_TOKEN: 'fake-token' });
    const viaRaw = await get('/catalog.json', { PLUGINS_KV: fakeKv() });
    const body = await viaApi.text();
    assert.ok(body.includes('\u0273Task Bundle \u2192 \u2014'), 'no mojibake');
    assert.equal(body, await viaRaw.text());
    assert.equal(viaApi.headers.get('ETag'), viaRaw.headers.get('ETag'));
  });

  test('falls back to the raw URL when the Contents API fails', async () => {
    const calls = stubUpstream(sampleCatalog(), { apiStatus: 403 });
    const res = await get('/catalog.json', { PLUGINS_KV: fakeKv(), GH_ACCESS_TOKEN: 'fake-token' });
    assert.equal(res.status, 200);
    assert.equal(calls.length, 2);
  });

  test('If-None-Match with the current ETag -> 304, empty body, ETag and Cache-Control kept', async () => {
    stubUpstream(sampleCatalog());
    const env = { PLUGINS_KV: fakeKv() };
    const first = await get('/catalog.json', env);
    const etag = first.headers.get('ETag');
    const res = await get('/catalog.json', env, { 'If-None-Match': etag });
    assert.equal(res.status, 304);
    assert.equal(await res.text(), '');
    assert.equal(res.headers.get('ETag'), etag);
    assert.equal(res.headers.get('Cache-Control'), 'public, max-age=300');
  });

  test('If-None-Match: list, weak form and * all match; a different tag does not', async () => {
    stubUpstream(sampleCatalog());
    const env = { PLUGINS_KV: fakeKv() };
    const etag = (await get('/catalog.json', env)).headers.get('ETag');
    const other = `"${'0'.repeat(64)}"`;
    assert.equal((await get('/catalog.json', env, { 'If-None-Match': `${other}, ${etag}` })).status, 304);
    assert.equal((await get('/catalog.json', env, { 'If-None-Match': `W/${etag}` })).status, 304);
    assert.equal((await get('/catalog.json', env, { 'If-None-Match': '*' })).status, 304);
    const miss = await get('/catalog.json', env, { 'If-None-Match': other });
    assert.equal(miss.status, 200);
    assert.ok((await miss.text()).length > 0);
    // an unquoted tag is not the ETag
    assert.equal((await get('/catalog.json', env, { 'If-None-Match': etag.slice(1, -1) })).status, 200);
  });

  test('a changed catalog changes the ETag; an unchanged one does not', async () => {
    stubUpstream(sampleCatalog());
    const a = (await get('/catalog.json', { PLUGINS_KV: fakeKv() })).headers.get('ETag');
    stubUpstream(sampleCatalog());
    const a2 = (await get('/catalog.json', { PLUGINS_KV: fakeKv() })).headers.get('ETag');
    stubUpstream(sampleCatalog({ counts: { free: 2, licensed: 1 } }));
    const b = (await get('/catalog.json', { PLUGINS_KV: fakeKv() })).headers.get('ETag');
    assert.equal(a, a2);
    assert.notEqual(a, b);
  });

  test('a stale If-None-Match after the catalog changed gets the new body, not a 304', async () => {
    stubUpstream(sampleCatalog());
    const oldTag = (await get('/catalog.json', { PLUGINS_KV: fakeKv() })).headers.get('ETag');
    stubUpstream(sampleCatalog({ counts: { free: 9, licensed: 9 } }));
    const res = await get('/catalog.json', { PLUGINS_KV: fakeKv() }, { 'If-None-Match': oldTag });
    assert.equal(res.status, 200);
    assert.notEqual(res.headers.get('ETag'), oldTag);
  });

  test('a fresh KV copy is served without touching the network (X-Cache: HIT, same ETag)', async () => {
    const doc = sampleCatalog();
    const body = JSON.stringify(doc);
    const kv = fakeKv({ 'catalog:json': envelope({ body, etag: sha256(body) }) });
    globalThis.fetch = async () => { throw new Error('network must not be used'); };
    const res = await get('/catalog.json', { PLUGINS_KV: kv });
    assert.equal(res.status, 200);
    assert.equal(res.headers.get('X-Cache'), 'HIT');
    assert.equal(res.headers.get('ETag'), `"${sha256(body)}"`);
    assert.equal(await res.text(), body);
  });

  test('the first fetch is written to KV so the second request is a HIT', async () => {
    stubUpstream(sampleCatalog());
    const kv = fakeKv();
    const env = { PLUGINS_KV: kv };
    await get('/catalog.json', env);
    assert.ok(kv.store.has('catalog:json'));
    globalThis.fetch = async () => { throw new Error('network must not be used'); };
    assert.equal((await get('/catalog.json', env)).headers.get('X-Cache'), 'HIT');
  });

  test('expired KV copy + upstream down -> 200 STALE with a short max-age', async () => {
    const body = JSON.stringify(sampleCatalog());
    const kv = fakeKv({ 'catalog:json': envelope({ body, etag: sha256(body) }, 10 * 60 * 1000) });
    stubUpstream(null, { apiStatus: 500, rawStatus: 500 });
    const res = await get('/catalog.json', { PLUGINS_KV: kv });
    assert.equal(res.status, 200);
    assert.equal(res.headers.get('X-Cache'), 'STALE');
    assert.equal(res.headers.get('Cache-Control'), 'public, max-age=60');
  });

  test('no cache and upstream down -> 502', async () => {
    stubUpstream(null, { apiStatus: 500, rawStatus: 500 });
    const res = await get('/catalog.json', { PLUGINS_KV: fakeKv() });
    assert.equal(res.status, 502);
    assert.ok(!res.headers.get('ETag'));
  });

  test('upstream bodies that are not a catalog are never served or cached', async () => {
    for (const bad of [[], 'text', 42, {}, { plugins: null }, { schema_version: '1.0.0' }]) {
      stubUpstream(bad);
      const kv = fakeKv();
      const res = await get('/catalog.json', { PLUGINS_KV: kv });
      assert.equal(res.status, 502, `bad body ${JSON.stringify(bad)}`);
      assert.equal(kv.store.has('catalog:json'), false);
    }
  });

  test('a corrupt KV envelope is ignored and refetched', async () => {
    stubUpstream(sampleCatalog());
    const kv = fakeKv({ 'catalog:json': envelope({ body: 5, etag: null }) });
    const res = await get('/catalog.json', { PLUGINS_KV: kv });
    assert.equal(res.status, 200);
    assert.equal(res.headers.get('X-Cache'), 'MISS');
  });

  test('works without any KV binding, under /v1 and with a trailing slash', async () => {
    stubUpstream(sampleCatalog());
    assert.equal((await get('/catalog.json', {})).status, 200);
    assert.equal((await get('/v1/catalog.json', {})).status, 200);
    assert.equal((await get('/catalog.json/', {})).status, 200);
  });

  test('only GET serves it: POST is 404, OPTIONS is 204', async () => {
    stubUpstream(sampleCatalog());
    const ctx = fakeCtx();
    assert.equal((await worker.fetch(req('/catalog.json', { method: 'POST' }), {}, ctx)).status, 404);
    assert.equal((await worker.fetch(req('/catalog.json', { method: 'OPTIONS' }), {}, ctx)).status, 204);
  });

  test('Contents API returning an empty body (a file over 1 MB) falls back to the raw URL', async () => {
    const doc = sampleCatalog();
    const calls = [];
    globalThis.fetch = async (url) => {
      calls.push(String(url));
      return String(url).startsWith('https://api.github.com/')
        ? new Response(JSON.stringify({ content: '', encoding: 'none' }), { status: 200 })
        : new Response(JSON.stringify(doc), { status: 200 });
    };
    const res = await get('/catalog.json', { PLUGINS_KV: fakeKv(), GH_ACCESS_TOKEN: 'fake-token' });
    assert.equal(res.status, 200);
    assert.equal(calls.length, 2);
    assert.equal(await res.text(), JSON.stringify(doc));
  });

  test('a network error on the Contents API also falls back to the raw URL', async () => {
    const doc = sampleCatalog();
    globalThis.fetch = async (url) => {
      if (String(url).startsWith('https://api.github.com/')) throw new Error('boom');
      return new Response(JSON.stringify(doc), { status: 200 });
    };
    const res = await get('/catalog.json', { PLUGINS_KV: fakeKv(), GH_ACCESS_TOKEN: 'fake-token' });
    assert.equal(res.status, 200);
  });

  test('a HIT promises only the time the KV copy has left (max-age shrinks with age)', async () => {
    const body = JSON.stringify(sampleCatalog());
    const kv = fakeKv({ 'catalog:json': envelope({ body, etag: sha256(body) }, 100 * 1000) });
    globalThis.fetch = async () => { throw new Error('network must not be used'); };
    const res = await get('/catalog.json', { PLUGINS_KV: kv });
    const maxAge = Number(/max-age=(\d+)/.exec(res.headers.get('Cache-Control'))[1]);
    assert.ok(maxAge >= 198 && maxAge <= 200, `max-age ${maxAge}`);
  });

  test('an expired KV copy with a changed upstream gets the new body and a new ETag', async () => {
    const oldDoc = sampleCatalog();
    const oldBody = JSON.stringify(oldDoc);
    const kv = fakeKv({ 'catalog:json': envelope({ body: oldBody, etag: sha256(oldBody) }, 10 * 60 * 1000) });
    const newDoc = sampleCatalog({ counts: { free: 5, licensed: 5 } });
    stubUpstream(newDoc);
    const res = await get('/catalog.json', { PLUGINS_KV: kv }, { 'If-None-Match': `"${sha256(oldBody)}"` });
    assert.equal(res.status, 200);
    assert.equal(res.headers.get('X-Cache'), 'MISS');
    assert.equal(res.headers.get('ETag'), `"${sha256(JSON.stringify(newDoc))}"`);
  });

  test('STALE: the old copy is re-stamped so GitHub is retried in about 60 s, not on every request', async () => {
    const body = JSON.stringify(sampleCatalog());
    const kv = fakeKv({ 'catalog:json': envelope({ body, etag: sha256(body) }, 10 * 60 * 1000) });
    stubUpstream(null, { apiStatus: 500, rawStatus: 500 });
    await get('/catalog.json', { PLUGINS_KV: kv });
    const stored = JSON.parse(kv.store.get('catalog:json'));
    assert.equal(stored.data.body, body);
    const remaining = 300 - (Date.now() - stored.timestamp) / 1000;
    assert.ok(remaining > 50 && remaining <= 61, `retry in ${remaining}s`);
    // the next request is a HIT on the re-stamped copy and does not touch the network
    globalThis.fetch = async () => { throw new Error('network must not be used'); };
    assert.equal((await get('/catalog.json', { PLUGINS_KV: kv })).headers.get('X-Cache'), 'HIT');
  });

  test('POST /api/sync with the upstream down keeps the old catalog copy (served STALE)', async () => {
    const body = JSON.stringify(sampleCatalog());
    const kv = fakeKv({ 'catalog:json': envelope({ body, etag: sha256(body) }) });
    stubUpstream(null, { apiStatus: 500, rawStatus: 500 });
    const ctx = fakeCtx();
    const res = await worker.fetch(
      req('/api/sync', { method: 'POST', headers: { Authorization: 'Bearer fake-sync-token' } }),
      { PLUGINS_KV: kv, GITHUB_SYNC_TOKEN: 'fake-sync-token' },
      ctx,
    );
    await ctx.settle();
    assert.equal(res.status, 200);
    assert.equal((await res.json()).catalogSynced, false);
    assert.equal(JSON.parse(kv.store.get('catalog:json')).data.body, body);
    assert.equal((await get('/catalog.json', { PLUGINS_KV: kv })).status, 200);
  });

  test('POST /api/sync refetches the catalog and replaces the KV copy', async () => {
    const kv = fakeKv({ 'catalog:json': envelope({ body: '{"plugins":{}}', etag: 'a'.repeat(64) }) });
    const calls = stubUpstream(sampleCatalog());
    const ctx = fakeCtx();
    const res = await worker.fetch(
      req('/api/sync', { method: 'POST', headers: { Authorization: 'Bearer fake-sync-token' } }),
      { PLUGINS_KV: kv, GITHUB_SYNC_TOKEN: 'fake-sync-token' },
      ctx,
    );
    await ctx.settle();
    assert.equal(res.status, 200);
    assert.equal((await res.json()).catalogSynced, true);
    assert.ok(calls.some((u) => u.endsWith('/catalog.json')));
    assert.equal(JSON.parse(kv.store.get('catalog:json')).data.body, JSON.stringify(sampleCatalog()));
  });
});
