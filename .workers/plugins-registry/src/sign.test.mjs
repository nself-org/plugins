import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import { webcrypto } from 'node:crypto';
import worker from '../dist-test/index.mjs';
import { fakeCtx, registryKv, req } from './test-helpers.mjs';

// RFC 8032 test vector 1: public key independently derived from this seed.
const seed = '9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60';
const publicKey = 'd75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a';
Object.defineProperty(globalThis, 'crypto', { configurable: true, value: webcrypto });
afterEach(() => Object.defineProperty(globalThis, 'crypto', { configurable: true, value: webcrypto }));

async function call(path, keys = {}) {
  return worker.fetch(req(path), { PLUGINS_KV: registryKv(), ...keys }, fakeCtx());
}

async function verify(signature, message) {
  const key = await webcrypto.subtle.importKey('raw', Buffer.from(publicKey, 'hex'), 'Ed25519', false, ['verify']);
  return webcrypto.subtle.verify('Ed25519', key, Buffer.from(signature, 'hex'), new TextEncoder().encode(message));
}

test('seed signs tarball redirect and signature metadata with verifiable canonical signatures', async () => {
  const keys = { SIGNING_PRIVATE_KEY: seed, PUBLIC_KEY_HEX: publicKey };
  const redirect = await call('/plugins/jobs/tarball', keys);
  assert.equal(redirect.status, 302);
  assert.match(redirect.headers.get('X-Signature') ?? '', /^[0-9a-f]{128}$/);
  assert.equal(await verify(redirect.headers.get('X-Signature'), redirect.headers.get('X-Signed-For')), true);
  const response = await call('/plugins/jobs/signature', keys);
  assert.equal(response.status, 200);
  const body = await response.json();
  assert.match(body.signature ?? '', /^[0-9a-f]{128}$/);
  assert.equal(await verify(body.signature, body.signedFor), true);
});

test('configured invalid seeds fail both signing endpoints with 500', async () => {
  for (const invalid of ['x', 'ab'.repeat(31), '0g'.repeat(32), 'ab'.repeat(33)]) {
    for (const path of ['/plugins/jobs/tarball', '/plugins/jobs/signature']) {
      const response = await call(path, { SIGNING_PRIVATE_KEY: invalid });
      assert.equal(response.status, 500, `${path}: ${invalid.length} characters`);
      assert.equal(response.headers.has('Location'), false);
      assert.equal((await response.json()).message, 'Plugin signing failed');
    }
  }
});

test('health verifies matching keys, including legacy public-key alias', async () => {
  for (const field of ['PUBLIC_KEY_HEX', 'SIGNING_PUBLIC_KEY']) {
    const response = await call('/health', { SIGNING_PRIVATE_KEY: seed, [field]: publicKey });
    assert.equal((await response.json()).signing.configured, true);
  }
  for (const keys of [
    {}, { SIGNING_PRIVATE_KEY: seed },
    { SIGNING_PRIVATE_KEY: 'x', PUBLIC_KEY_HEX: publicKey },
    { SIGNING_PRIVATE_KEY: seed, PUBLIC_KEY_HEX: '00'.repeat(32) },
  ]) {
    assert.equal((await (await call('/health', keys)).json()).signing.configured, false);
  }
});

test('crypto signing failures fail closed and make health signing unavailable', async () => {
  Object.defineProperty(globalThis, 'crypto', { configurable: true, value: { subtle: {
    importKey: webcrypto.subtle.importKey.bind(webcrypto.subtle),
    sign: async () => { throw new Error('synthetic crypto failure'); },
  } } });
  const keys = { SIGNING_PRIVATE_KEY: seed, PUBLIC_KEY_HEX: publicKey };
  for (const path of ['/plugins/jobs/tarball', '/plugins/jobs/signature']) {
    assert.equal((await call(path, keys)).status, 500);
  }
  assert.equal((await (await call('/health', keys)).json()).signing.configured, false);
});

test('unconfigured signing preserves unsigned endpoint behavior', async () => {
  assert.equal((await call('/plugins/jobs/tarball')).status, 302);
  assert.equal((await (await call('/plugins/jobs/signature')).json()).signature, null);
});
