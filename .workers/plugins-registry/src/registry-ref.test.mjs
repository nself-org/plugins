/**
 * registry-ref.test.mjs — licensed rows are read at the pinned bundles ref (P7-WIRE-16, D-1483).
 *
 * The CLI takes a licensed plugin's checksum from this worker and its bytes from ping.
 * Ping builds the bytes at PLUGINS_PRO_REF, so the worker must read bundles registry.json
 * at that same ref (LICENSED_REF), never at bundles main.
 *
 * Driven through the real entry: the worker's default export handles GET /registry.json
 * with a recording fetch stub. Node strips the TypeScript types of src/index.ts itself.
 *
 * Run: node --test src/registry-ref.test.mjs (from .workers/plugins-registry)
 */

import { test, describe, beforeEach, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import worker from './index.ts';
import { fakeKv, installCache, fakeCtx } from './test-helpers.mjs';

const BUNDLES_API = 'https://api.github.com/repos/nself-org/bundles/contents/registry.json';
const PINNED_SUM = 'a'.repeat(64);
const MAIN_SUM = 'b'.repeat(64);

function contents(obj) {
  return { content: Buffer.from(JSON.stringify(obj)).toString('base64'), encoding: 'base64' };
}

function bundlesAt(checksum) {
  return { plugins: { ai: { name: 'ai', version: '1.2.9', tier: 'max', checksum } } };
}

const freeRegistry = { plugins: { notify: { name: 'notify', version: '1.0.0', tier: 'free' } } };

let calls;
let realFetch;

beforeEach(() => {
  calls = [];
  realFetch = globalThis.fetch;
  installCache();
  globalThis.fetch = async (input) => {
    const url = typeof input === 'string' ? input : input.url;
    calls.push(url);
    if (url.startsWith(BUNDLES_API)) {
      // A pinned read sees the pinned checksum; an unpinned read follows main.
      const pinned = new URL(url).searchParams.get('ref') === 'v1.2.9';
      return Response.json(contents(bundlesAt(pinned ? PINNED_SUM : MAIN_SUM)));
    }
    if (url.includes('nself-org/plugins/contents/registry.json')) {
      return Response.json(contents(freeRegistry));
    }
    return new Response('not stubbed', { status: 404 });
  };
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

async function getRegistry(vars) {
  const env = { PLUGINS_KV: fakeKv(), CACHE_TTL: '300', GH_ACCESS_TOKEN: 'test-token-not-real', ...vars };
  const ctx = fakeCtx();
  const resp = await worker.fetch(new Request('https://plugins.nself.org/registry.json?nocache=1'), env, ctx);
  await ctx.settle();
  return resp;
}

const bundleCalls = () => calls.filter((u) => u.startsWith(BUNDLES_API));

describe('TestWire16RegistryFetchUsesRef', () => {
  test('the licensed fetch URL carries ?ref=<LICENSED_REF> and serves that ref\'s checksum', async () => {
    const resp = await getRegistry({ LICENSED_REF: 'v1.2.9' });
    assert.equal(resp.status, 200);
    assert.deepEqual(bundleCalls(), [`${BUNDLES_API}?ref=v1.2.9`]);
    const body = await resp.json();
    const ai = body.plugins.find((p) => p.name === 'ai');
    assert.ok(ai, 'licensed row served');
    assert.equal(ai.checksum, PINNED_SUM);
  });

  test('fails closed when LICENSED_REF is unset: no bundles fetch, no licensed rows', async () => {
    const resp = await getRegistry({});
    assert.deepEqual(bundleCalls(), []);
    assert.equal(resp.status, 200, 'free rows still serve');
    const body = await resp.json();
    assert.equal(body.proAvailable, false);
    assert.equal(body.plugins.find((p) => p.name === 'ai'), undefined);
  });

  for (const bad of ['', '   ', 'main?ref=evil', '../main', 'v1#x', 'a b', '-flag']) {
    test(`fails closed on a malformed LICENSED_REF ${JSON.stringify(bad)}`, async () => {
      const resp = await getRegistry({ LICENSED_REF: bad });
      assert.deepEqual(bundleCalls(), []);
      const body = await resp.json();
      assert.equal(body.proAvailable, false);
    });
  }
});
