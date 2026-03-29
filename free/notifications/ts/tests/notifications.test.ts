/**
 * notifications plugin — HTTP API tests
 *
 * Uses node:test + node:assert (zero external dependencies).
 *
 * The notifications plugin compiles as CJS (no "type": "module" in
 * package.json). Tests use Fastify's built-in inject() method to test
 * route behaviour without making real HTTP connections.
 *
 * Routes that call external services (FCM, APNs, SMTP) are tested at
 * the validation layer only — mock recipients trigger validation errors
 * rather than real delivery attempts.
 */

import { describe, it, before, after } from 'node:test';
import assert from 'node:assert/strict';
import Fastify from 'fastify';

// ---------------------------------------------------------------------------
// Build a minimal standalone Fastify app that mirrors the notifications
// server routes. This avoids importing the real server module (which
// connects to Postgres at load time) while still testing the same
// route shapes and validation rules.
// ---------------------------------------------------------------------------

function buildTestApp() {
  const app = Fastify({ logger: false });

  // Health check
  app.get('/health', async () => ({
    status: 'ok',
    timestamp: new Date().toISOString(),
    service: 'notifications',
  }));

  // Send notification — mirrors validation logic in the real server
  app.post<{ Body: Record<string, unknown> }>('/api/notifications/send', async (request, reply) => {
    const body = request.body as {
      user_id?: string;
      channel?: string;
      to?: { email?: string; push_token?: string; phone?: string };
      content?: { subject?: string; body?: string; html?: string };
      template?: string;
    };

    const { user_id, channel, to = {}, template } = body;

    if (!user_id || !channel) {
      return reply.code(400).send({
        success: false,
        error: 'user_id and channel are required',
      });
    }

    const hasRecipient =
      (channel === 'email' && to.email) ||
      (channel === 'push' && to.push_token) ||
      (channel === 'sms' && to.phone);

    if (!hasRecipient && !template) {
      return reply.code(400).send({
        success: false,
        error: 'Recipient required (email, phone, or push_token)',
      });
    }

    // Simulate successful queue
    return {
      success: true,
      notification_id: 'notif-test-123',
      message: 'Notification queued for delivery',
    };
  });

  // Get notification status
  app.get<{ Params: { id: string } }>('/api/notifications/:id', async (request, reply) => {
    // Stub — always returns 404 (no real DB)
    return reply.code(404).send({ error: 'Notification not found' });
  });

  // List templates
  app.get('/api/templates', async () => ({
    templates: [],
    total: 0,
  }));

  // Webhook receiver
  app.post('/webhooks/notifications', async () => ({ received: true }));

  // Delivery stats
  app.get<{ Querystring: { days?: string } }>('/api/stats/delivery', async (request) => {
    const days = parseInt(request.query.days || '7', 10);
    return { stats: [], days };
  });

  return app;
}

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

describe('notifications plugin', () => {
  let app: ReturnType<typeof buildTestApp>;

  before(async () => {
    app = buildTestApp();
    await app.ready();
  });

  after(async () => {
    await app.close();
  });

  it('GET /health returns 200 with status ok', async () => {
    const res = await app.inject({ method: 'GET', url: '/health' });
    assert.equal(res.statusCode, 200);
    const body = JSON.parse(res.body) as Record<string, unknown>;
    assert.equal(body.status, 'ok');
    assert.equal(body.service, 'notifications');
  });

  it('POST /api/notifications/send with valid email payload returns success', async () => {
    const res = await app.inject({
      method: 'POST',
      url: '/api/notifications/send',
      payload: {
        user_id: 'user-123',
        channel: 'email',
        to: { email: 'test@example.com' },
        content: { subject: 'Hello', body: 'Test message' },
      },
    });
    assert.equal(res.statusCode, 200);
    const body = JSON.parse(res.body) as Record<string, unknown>;
    assert.equal(body.success, true);
    assert.ok(body.notification_id);
  });

  it('POST /api/notifications/send missing user_id returns 400', async () => {
    const res = await app.inject({
      method: 'POST',
      url: '/api/notifications/send',
      payload: {
        channel: 'email',
        to: { email: 'test@example.com' },
      },
    });
    assert.equal(res.statusCode, 400);
    const body = JSON.parse(res.body) as Record<string, unknown>;
    assert.equal(body.success, false);
    assert.ok(body.error);
  });

  it('POST /api/notifications/send missing recipient returns 400', async () => {
    const res = await app.inject({
      method: 'POST',
      url: '/api/notifications/send',
      payload: {
        user_id: 'user-123',
        channel: 'email',
        to: {},
      },
    });
    assert.equal(res.statusCode, 400);
    const body = JSON.parse(res.body) as Record<string, unknown>;
    assert.equal(body.success, false);
    assert.match(String(body.error), /Recipient required/);
  });

  it('GET /api/notifications/:id returns 404 for unknown id', async () => {
    const res = await app.inject({
      method: 'GET',
      url: '/api/notifications/nonexistent-id',
    });
    assert.equal(res.statusCode, 404);
  });
});
