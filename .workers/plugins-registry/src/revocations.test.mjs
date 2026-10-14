/**
 * revocations.test.mjs — GET /.well-known/revoked-authors.json (D-0867)
 *
 * The author CRL must fail closed: a KV read error returns 503 with no-store
 * and no revokedAuthors payload, never a cacheable empty list. A good read and
 * an absent key keep the public 200 response. Runs against dist-test/index.mjs
 * (esbuild bundle of src/index.ts), like routes.test.mjs.
 *
 * Run: pnpm test (from .workers/plugins-registry)
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import worker from '../dist-test/index.mjs';
import { fakeCtx, req } from './test-helpers.mjs';

const PATH = '/.well-known/revoked-authors.json';

async function call(env) {
  const ctx = fakeCtx();
  const res = await worker.fetch(req(PATH), env, ctx);
  await ctx.settle();
  return res;
}

test('author revocation KV failure fails closed without caching an empty list', async () => {
  for (const binding of ['REGISTRY', 'PLUGINS_KV']) {
    const res = await call({
      [binding]: {
        get: async () => {
          throw new Error('KV unavailable');
        },
      },
    });
    assert.equal(res.status, 503, binding);
    assert.equal(res.headers.get('Cache-Control'), 'no-store', binding);
    const body = await res.json();
    assert.equal(typeof body.error, 'string');
    assert.equal('revokedAuthors' in body, false);
    assert.equal('count' in body, false);
  }
});

test('author revocation successful and absent reads keep the public response', async () => {
  const author = { authorKey: 'revoked-author', revokedAt: '2026-10-14T00:00:00Z' };
  for (const raw of [JSON.stringify([author]), null]) {
    const res = await call({
      REGISTRY: {
        get: async (key, type) => {
          assert.equal(key, 'revocations:authors');
          assert.equal(type, 'text');
          return raw;
        },
      },
    });
    assert.equal(res.status, 200);
    assert.equal(res.headers.get('Cache-Control'), 'public, max-age=60');
    const body = await res.json();
    assert.deepEqual(body.revokedAuthors, raw ? [author] : []);
    assert.equal(body.count, raw ? 1 : 0);
  }
});
