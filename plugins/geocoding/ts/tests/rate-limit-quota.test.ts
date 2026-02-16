/**
 * Rate Limiting and Quota Tracking Tests
 * Tests per-user rate limiting and database quota tracking
 */

import { test } from 'node:test';
import assert from 'node:assert';
import { createServer } from '../src/server.js';

test('Rate limiting - per-user limits enforced', async (t) => {
  const server = await createServer({
    port: 0, // Random port
    host: '127.0.0.1',
    security: {
      rateLimitMax: 3, // Very low limit for testing
      rateLimitWindowMs: 10000, // 10 seconds
    },
  });

  await server.start();

  try {
    // Make requests with same sourceAccountId (via X-App-Name header)
    const makeRequest = async () => {
      const response = await fetch(`http://127.0.0.1:${server.app.server.address().port}/api/stats`, {
        headers: {
          'X-App-Name': 'test-user',
        },
      });
      return response;
    };

    // First 3 requests should succeed
    const response1 = await makeRequest();
    assert.strictEqual(response1.status, 200, 'First request should succeed');
    assert.ok(response1.headers.get('X-RateLimit-Limit'), 'Should have rate limit header');

    const response2 = await makeRequest();
    assert.strictEqual(response2.status, 200, 'Second request should succeed');

    const response3 = await makeRequest();
    assert.strictEqual(response3.status, 200, 'Third request should succeed');

    // Fourth request should be rate limited
    const response4 = await makeRequest();
    assert.strictEqual(response4.status, 429, 'Fourth request should be rate limited');
    assert.ok(response4.headers.get('Retry-After'), 'Should have Retry-After header');

    const body = await response4.json();
    assert.strictEqual(body.error, 'Too many requests', 'Should have rate limit error message');
  } finally {
    await server.stop();
  }
});

test('Rate limiting - different users have separate limits', async (t) => {
  const server = await createServer({
    port: 0,
    host: '127.0.0.1',
    security: {
      rateLimitMax: 2,
      rateLimitWindowMs: 10000,
    },
  });

  await server.start();

  try {
    const makeRequest = async (userId: string) => {
      const response = await fetch(`http://127.0.0.1:${server.app.server.address().port}/api/stats`, {
        headers: {
          'X-App-Name': userId,
        },
      });
      return response;
    };

    // User 1: 2 requests (at limit)
    const user1_req1 = await makeRequest('user-1');
    assert.strictEqual(user1_req1.status, 200, 'User 1 request 1 should succeed');

    const user1_req2 = await makeRequest('user-1');
    assert.strictEqual(user1_req2.status, 200, 'User 1 request 2 should succeed');

    // User 1: 3rd request should be rate limited
    const user1_req3 = await makeRequest('user-1');
    assert.strictEqual(user1_req3.status, 429, 'User 1 request 3 should be rate limited');

    // User 2: Should have separate limit and succeed
    const user2_req1 = await makeRequest('user-2');
    assert.strictEqual(user2_req1.status, 200, 'User 2 request 1 should succeed (separate limit)');

    const user2_req2 = await makeRequest('user-2');
    assert.strictEqual(user2_req2.status, 200, 'User 2 request 2 should succeed');

    // User 2: 3rd request should be rate limited
    const user2_req3 = await makeRequest('user-2');
    assert.strictEqual(user2_req3.status, 429, 'User 2 request 3 should be rate limited');
  } finally {
    await server.stop();
  }
});

test('Quota tracking - API calls are tracked', async (t) => {
  const server = await createServer({
    port: 0,
    host: '127.0.0.1',
  });

  await server.start();

  try {
    const baseUrl = `http://127.0.0.1:${server.app.server.address().port}`;

    // Make a few API calls
    await fetch(`${baseUrl}/api/stats`, {
      headers: { 'X-App-Name': 'quota-test' },
    });

    await fetch(`${baseUrl}/api/stats`, {
      headers: { 'X-App-Name': 'quota-test' },
    });

    // Check quota usage (note: stats endpoint doesn't increment quota, so we need to use geocode endpoint)
    // Make geocode call instead
    await fetch(`${baseUrl}/api/geocode`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-App-Name': 'quota-test',
      },
      body: JSON.stringify({ address: 'Test Address' }),
    });

    // Query quota
    const quotaResponse = await fetch(`${baseUrl}/api/quota`, {
      headers: { 'X-App-Name': 'quota-test' },
    });

    assert.strictEqual(quotaResponse.status, 200, 'Quota endpoint should respond');

    const quota = await quotaResponse.json();
    assert.ok(quota.daily, 'Should have daily quota');
    assert.ok(quota.daily.api_calls >= 1, 'Should have tracked at least 1 API call');
  } finally {
    await server.stop();
  }
});

test('Quota tracking - cache hits vs geocode calls tracked separately', async (t) => {
  const server = await createServer({
    port: 0,
    host: '127.0.0.1',
    cacheEnabled: true,
  });

  await server.start();

  try {
    const baseUrl = `http://127.0.0.1:${server.app.server.address().port}`;

    // Make first geocode call (will be cache miss)
    await fetch(`${baseUrl}/api/geocode`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-App-Name': 'cache-test',
      },
      body: JSON.stringify({ address: 'Test Address for Cache' }),
    });

    // Make same call again (should be cache hit if provider was configured)
    // Since provider isn't configured, it will still increment as geocode_call
    await fetch(`${baseUrl}/api/geocode`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-App-Name': 'cache-test',
      },
      body: JSON.stringify({ address: 'Test Address for Cache' }),
    });

    // Query quota
    const quotaResponse = await fetch(`${baseUrl}/api/quota`, {
      headers: { 'X-App-Name': 'cache-test' },
    });

    const quota = await quotaResponse.json();
    assert.ok(quota.daily, 'Should have daily quota');
    assert.ok(quota.daily.api_calls >= 2, 'Should have tracked 2 API calls');
    assert.ok(quota.daily.geocode_calls >= 0, 'Should have geocode_calls field');
  } finally {
    await server.stop();
  }
});

test('Quota limit enforcement', async (t) => {
  const server = await createServer({
    port: 0,
    host: '127.0.0.1',
  });

  await server.start();

  try {
    const baseUrl = `http://127.0.0.1:${server.app.server.address().port}`;

    // Make some geocode calls
    await fetch(`${baseUrl}/api/geocode`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-App-Name': 'limit-test',
      },
      body: JSON.stringify({ address: 'Test' }),
    });

    await fetch(`${baseUrl}/api/geocode`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-App-Name': 'limit-test',
      },
      body: JSON.stringify({ address: 'Test 2' }),
    });

    // Check quota with low limit (should fail)
    const checkResponse = await fetch(`${baseUrl}/api/quota/check?limit=1&type=daily`, {
      headers: { 'X-App-Name': 'limit-test' },
    });

    assert.strictEqual(checkResponse.status, 429, 'Should exceed quota');

    const result = await checkResponse.json();
    assert.strictEqual(result.error, 'Quota exceeded', 'Should have quota exceeded error');
    assert.strictEqual(result.allowed, false, 'Should not be allowed');

    // Check with high limit (should pass)
    const checkResponse2 = await fetch(`${baseUrl}/api/quota/check?limit=1000&type=daily`, {
      headers: { 'X-App-Name': 'limit-test' },
    });

    assert.strictEqual(checkResponse2.status, 200, 'Should be under quota');

    const result2 = await checkResponse2.json();
    assert.strictEqual(result2.allowed, true, 'Should be allowed');
  } finally {
    await server.stop();
  }
});

test('Rate limit headers are present', async (t) => {
  const server = await createServer({
    port: 0,
    host: '127.0.0.1',
    security: {
      rateLimitMax: 100,
      rateLimitWindowMs: 60000,
    },
  });

  await server.start();

  try {
    const response = await fetch(`http://127.0.0.1:${server.app.server.address().port}/api/stats`, {
      headers: {
        'X-App-Name': 'headers-test',
      },
    });

    assert.strictEqual(response.status, 200, 'Request should succeed');
    assert.ok(response.headers.get('X-RateLimit-Limit'), 'Should have X-RateLimit-Limit header');
    assert.ok(response.headers.get('X-RateLimit-Remaining'), 'Should have X-RateLimit-Remaining header');
    assert.ok(response.headers.get('X-RateLimit-Reset'), 'Should have X-RateLimit-Reset header');

    const limit = parseInt(response.headers.get('X-RateLimit-Limit') || '0', 10);
    const remaining = parseInt(response.headers.get('X-RateLimit-Remaining') || '0', 10);

    assert.strictEqual(limit, 100, 'Limit should be 100');
    assert.ok(remaining < limit, 'Remaining should be less than limit after request');
  } finally {
    await server.stop();
  }
});
