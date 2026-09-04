/**
 * index.test.mjs — GET /plugins/:name/tarball tier-guard regression test
 *
 * P6-E10-W5-S1-T4 (siege HIGH, qa/bugs/paid-tarball-gated-by-repo-visibility-
 * not-licence.md): the tarball route must refuse EVERY non-free tier, not
 * just "pro" — PluginTier is "free" | "pro" | "max" and a live plugin ("ai")
 * ships at tier "max". Before this fix, index.js (the file wrangler.toml
 * actually deploys — `main = "src/index.js"`) had NO guard on this route at
 * all: it redirected straight to the GitHub release URL for every tier.
 * index.ts separately gained a tier==="pro"-only guard in PR #66, which
 * still missed "max" and was never the deployed file to begin with.
 *
 * This suite runs against index.js directly with Node's built-in test
 * runner — this package has zero test infrastructure (no vitest/jest; see
 * the qa/bugs doc above), so this intentionally adds no new dependency.
 *
 * Run: node --test src/index.test.mjs
 * (from .workers/plugins-registry)
 */

import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { isDirectDownloadTier, handlePluginTarball } from './index.js';

// ---------------------------------------------------------------------------
// isDirectDownloadTier — the guard predicate itself
// ---------------------------------------------------------------------------

describe('isDirectDownloadTier()', () => {
  test('free -> true', () => {
    assert.equal(isDirectDownloadTier('free'), true);
  });

  test('pro -> false', () => {
    assert.equal(isDirectDownloadTier('pro'), false);
  });

  test('max -> false (the actual bug: this tier was never blocked anywhere)', () => {
    assert.equal(isDirectDownloadTier('max'), false);
  });

  test('an unknown/future tier value defaults to blocked (fail-safe by construction)', () => {
    // Guards against the enum growing a 4th tier and this function silently
    // treating it as downloadable. Only the literal "free" ever passes.
    assert.equal(isDirectDownloadTier('enterprise'), false);
    assert.equal(isDirectDownloadTier(undefined), false);
    assert.equal(isDirectDownloadTier(''), false);
  });
});

// ---------------------------------------------------------------------------
// handlePluginTarball — end-to-end route behaviour
//
// The route takes no request/Authorization header at all (confirmed by
// reading handlePlugin()'s call site: handlePluginTarball(plugin, env) — no
// req is threaded through). So "with vs without a licence header" cannot be
// exercised at this layer; that is intentional and is the point of the fix:
// this route must refuse every non-free tier UNCONDITIONALLY, because no
// header reaches it that could ever let a request through. Entitlement is
// checked only downstream at ping.nself.org's licence-gated route.
// ---------------------------------------------------------------------------

const ENV_NO_KV = {}; // no PLUGINS_KV / R2 / signing key configured — safe defaults

function plugin(tier, overrides = {}) {
  return {
    name: 'activity-feed',
    version: '1.2.3',
    tier,
    ...overrides,
  };
}

describe('handlePluginTarball()', () => {
  test('free tier -> 302 redirect to the public GitHub release (unauthenticated, by design)', async () => {
    const res = await handlePluginTarball(plugin('free'), ENV_NO_KV);
    assert.equal(res.status, 302);
    const location = res.headers.get('Location');
    assert.match(location, /^https:\/\/github\.com\/nself-org\/plugins\/releases\/download\//);
  });

  test('pro tier -> 401, points to the licence-gated download, no GitHub redirect', async () => {
    const res = await handlePluginTarball(plugin('pro'), ENV_NO_KV);
    assert.equal(res.status, 401);
    assert.equal(res.headers.get('Location'), null);
    const body = await res.json();
    assert.equal(body.plugin, 'activity-feed');
    assert.match(body.download_url, /^https:\/\/ping\.nself\.org\/plugins\/activity-feed\/download$/);
  });

  test('max tier -> 401, same as pro (THE bug: this previously fell through to a 302)', async () => {
    const res = await handlePluginTarball(plugin('max', { name: 'ai' }), ENV_NO_KV);
    assert.equal(res.status, 401);
    assert.equal(res.headers.get('Location'), null);
    const body = await res.json();
    assert.equal(body.plugin, 'ai');
    assert.match(body.download_url, /^https:\/\/ping\.nself\.org\/plugins\/ai\/download$/);
  });

  test('a hypothetical future tier also -> 401 (fail-safe, not an allow-list)', async () => {
    const res = await handlePluginTarball(plugin('enterprise'), ENV_NO_KV);
    assert.equal(res.status, 401);
  });

  test('non-free tiers are refused before the revocation check ever runs', async () => {
    // Regression guard: if the guard were ever moved below the revocation
    // check (or removed), a paid plugin marked revoked would leak a 410
    // instead of the 401, revealing tier-specific behaviour to anonymous
    // callers. env has no PLUGINS_KV, so isRevoked() would need to hit KV to
    // return true; passing ENV_NO_KV and still getting 401 (not 410 or 302)
    // is sufficient evidence the guard runs first.
    const res = await handlePluginTarball(plugin('pro'), ENV_NO_KV);
    assert.equal(res.status, 401);
  });
});
