/**
 * Devices Plugin Server
 * HTTP server for device enrollment, command dispatch, telemetry, and ingest management
 */

import Fastify, { type FastifyRequest } from 'fastify';
import cors from '@fastify/cors';
import crypto from 'node:crypto';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { DevicesDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  RegisterDeviceRequest,
  UpdateDeviceRequest,
  ChallengeResponse,
  DispatchCommandRequest,
  SubmitTelemetryRequest,
  StartIngestRequest,
  IngestHeartbeatRequest,
  RevokeRequest,
  CreateBootstrapTokenRequest,
  EnrollDeviceRequest,
  DeviceHeartbeatRequest,
  SendCommandRequest,
  DeviceStatus,
  DeviceType,
  TrustLevel,
  TelemetryType,
  IngestStatus,
} from './types.js';

const logger = createLogger('devices:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new DevicesDatabase(undefined, 'primary');
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 5 * 1024 * 1024,
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 500,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );

  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): DevicesDatabase {
    return (request as Record<string, unknown>).scopedDb as DevicesDatabase;
  }

  function getAppId(request: unknown): string {
    const ctx = getAppContext(request as FastifyRequest);
    return ctx.sourceAccountId ?? 'default';
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'devices', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'devices', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'devices',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getFleetStats();
    return {
      alive: true,
      plugin: 'devices',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        totalDevices: stats.total_devices,
        enrolledDevices: stats.enrolled_devices,
        onlineDevices: stats.online_devices,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Device Endpoints
  // =========================================================================

  app.get('/api/devices', async (request) => {
    const { status, type, trust_level, limit = 100, offset = 0 } = request.query as {
      status?: DeviceStatus;
      type?: DeviceType;
      trust_level?: TrustLevel;
      limit?: number;
      offset?: number;
    };
    const appId = getAppId(request);
    const devices = await scopedDb(request).listDevices(appId, status, type, trust_level, limit, offset);
    return { data: devices, limit, offset };
  });

  app.post('/api/devices/register', async (request, reply) => {
    try {
      const body = request.body as RegisterDeviceRequest;
      const appId = getAppId(request);

      if (!body.device_id || !body.device_type) {
        return reply.status(400).send({ error: 'device_id and device_type are required' });
      }

      const device = await scopedDb(request).registerDevice(appId, body);

      // Audit log
      await scopedDb(request).createAuditEntry(appId, 'device.registered', device.id, undefined, {
        device_id: body.device_id,
        device_type: body.device_type,
      });

      return reply.status(201).send(device);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Register device failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/devices/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const device = await scopedDb(request).getDevice(id);

    if (!device) {
      return reply.status(404).send({ error: 'Device not found' });
    }

    return device;
  });

  app.put('/api/devices/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const body = request.body as UpdateDeviceRequest;
    const device = await scopedDb(request).updateDevice(id, body);

    if (!device) {
      return reply.status(404).send({ error: 'Device not found' });
    }

    return device;
  });

  app.post('/api/devices/:id/enroll', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const appId = getAppId(request);

      // Generate enrollment token and challenge
      const token = crypto.randomBytes(32).toString('hex');
      const challenge = crypto.randomBytes(32).toString('hex');

      const device = await scopedDb(request).startEnrollment(id, token, challenge);

      if (!device) {
        return reply.status(404).send({ error: 'Device not found' });
      }

      // Audit log
      await scopedDb(request).createAuditEntry(appId, 'device.enrollment_started', id);

      return {
        device_id: device.device_id,
        enrollment_token: token,
        enrollment_challenge: challenge,
        expires_at: new Date(Date.now() + fullConfig.enrollmentTokenTtl * 1000).toISOString(),
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Start enrollment failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/devices/:id/challenge', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const body = request.body as ChallengeResponse;
      const appId = getAppId(request);

      if (!body.challenge_response || !body.public_key) {
        return reply.status(400).send({ error: 'challenge_response and public_key are required' });
      }

      // In production, verify the challenge_response against the stored challenge
      const device = await scopedDb(request).getDevice(id);
      if (!device) {
        return reply.status(404).send({ error: 'Device not found' });
      }

      if (device.status !== 'bootstrap_ready') {
        return reply.status(400).send({ error: 'Device is not in enrollment state' });
      }

      const enrolled = await scopedDb(request).completeEnrollment(id, body.public_key);

      // Audit log
      await scopedDb(request).createAuditEntry(appId, 'device.enrolled', id);

      return enrolled;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Challenge response failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/devices/:id/revoke', async (request, reply) => {
    const { id } = request.params as { id: string };
    const body = request.body as RevokeRequest;
    const appId = getAppId(request);

    if (!body.reason) {
      return reply.status(400).send({ error: 'reason is required' });
    }

    const device = await scopedDb(request).revokeDevice(id, body.reason, body.actor_id);

    if (!device) {
      return reply.status(404).send({ error: 'Device not found' });
    }

    // Audit log
    await scopedDb(request).createAuditEntry(appId, 'device.revoked', id, body.actor_id, { reason: body.reason });

    return device;
  });

  app.post('/api/devices/:id/suspend', async (request, reply) => {
    const { id } = request.params as { id: string };
    const appId = getAppId(request);

    const device = await scopedDb(request).suspendDevice(id);

    if (!device) {
      return reply.status(404).send({ error: 'Device not found or not enrolled' });
    }

    await scopedDb(request).createAuditEntry(appId, 'device.suspended', id);

    return device;
  });

  app.post('/api/devices/:id/reinstate', async (request, reply) => {
    const { id } = request.params as { id: string };
    const appId = getAppId(request);

    const device = await scopedDb(request).reinstateDevice(id);

    if (!device) {
      return reply.status(404).send({ error: 'Device not found or not suspended' });
    }

    await scopedDb(request).createAuditEntry(appId, 'device.reinstated', id);

    return device;
  });

  app.post('/api/devices/:id/heartbeat', async (request, reply) => {
    const { id } = request.params as { id: string };
    const ip = request.ip;

    const device = await scopedDb(request).updateDeviceHeartbeat(id, ip);

    if (!device) {
      return reply.status(404).send({ error: 'Device not found' });
    }

    return { ok: true, next_heartbeat_seconds: fullConfig.heartbeatInterval };
  });

  app.post('/api/devices/:id/telemetry', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const body = request.body as SubmitTelemetryRequest;
      const appId = getAppId(request);

      if (!body.telemetry_type || !body.data) {
        return reply.status(400).send({ error: 'telemetry_type and data are required' });
      }

      const record = await scopedDb(request).submitTelemetry(appId, id, body);

      // Also update heartbeat
      await scopedDb(request).updateDeviceHeartbeat(id, request.ip);

      return reply.status(201).send(record);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Submit telemetry failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/devices/:id/telemetry', async (request) => {
    const { id } = request.params as { id: string };
    const { type, limit = 100, offset = 0 } = request.query as {
      type?: TelemetryType;
      limit?: number;
      offset?: number;
    };

    const telemetry = await scopedDb(request).getDeviceTelemetry(id, type, limit, offset);
    return { data: telemetry, limit, offset };
  });

  app.get('/api/devices/:id/commands', async (request) => {
    const { id } = request.params as { id: string };
    const { limit = 50 } = request.query as { limit?: number };
    const commands = await scopedDb(request).getDeviceCommands(id, limit);
    return { data: commands };
  });

  app.get('/api/devices/:id/diagnostics', async (request, reply) => {
    const { id } = request.params as { id: string };
    const diagnostics = await scopedDb(request).getDiagnostics(id);

    if (!diagnostics.device) {
      return reply.status(404).send({ error: 'Device not found' });
    }

    return diagnostics;
  });

  // =========================================================================
  // Command Endpoints
  // =========================================================================

  app.post('/api/commands', async (request, reply) => {
    try {
      const body = request.body as DispatchCommandRequest;
      const appId = getAppId(request);

      if (!body.device_id || !body.command_type) {
        return reply.status(400).send({ error: 'device_id and command_type are required' });
      }

      const timeout = body.timeout_seconds ?? fullConfig.commandDefaultTimeout;
      const command = await scopedDb(request).dispatchCommand(appId, body.device_id, body, timeout);

      // Audit log
      await scopedDb(request).createAuditEntry(appId, 'command.dispatched', body.device_id, undefined, {
        command_id: command.id,
        command_type: body.command_type,
      });

      return reply.status(201).send(command);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Dispatch command failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/commands/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const command = await scopedDb(request).getCommand(id);

    if (!command) {
      return reply.status(404).send({ error: 'Command not found' });
    }

    return command;
  });

  app.post('/api/commands/:id/cancel', async (request, reply) => {
    const { id } = request.params as { id: string };
    const command = await scopedDb(request).cancelCommand(id);

    if (!command) {
      return reply.status(404).send({ error: 'Command not found or cannot be cancelled' });
    }

    return command;
  });

  // =========================================================================
  // Ingest Session Endpoints
  // =========================================================================

  app.get('/api/ingest/sessions', async (request) => {
    const { device_id, status, limit = 100, offset = 0 } = request.query as {
      device_id?: string;
      status?: IngestStatus;
      limit?: number;
      offset?: number;
    };
    const sessions = await scopedDb(request).listIngestSessions(device_id, status, limit, offset);
    return { data: sessions, limit, offset };
  });

  app.get('/api/ingest/sessions/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const session = await scopedDb(request).getIngestSession(id);

    if (!session) {
      return reply.status(404).send({ error: 'Ingest session not found' });
    }

    return session;
  });

  app.post('/api/ingest/sessions', async (request, reply) => {
    try {
      const body = request.body as StartIngestRequest;
      const appId = getAppId(request);

      if (!body.device_id || !body.stream_id) {
        return reply.status(400).send({ error: 'device_id and stream_id are required' });
      }

      const session = await scopedDb(request).startIngestSession(appId, body.device_id, body);

      // Audit log
      await scopedDb(request).createAuditEntry(appId, 'ingest.started', body.device_id, undefined, {
        session_id: session.id,
        stream_id: body.stream_id,
      });

      return reply.status(201).send(session);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Start ingest session failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/ingest/sessions/:id/heartbeat', async (request, reply) => {
    const { id } = request.params as { id: string };
    const body = (request.body as IngestHeartbeatRequest) ?? {};

    const session = await scopedDb(request).heartbeatIngestSession(id, body);

    if (!session) {
      return reply.status(404).send({ error: 'Ingest session not found' });
    }

    return { ok: true, next_heartbeat_seconds: fullConfig.ingestHeartbeatInterval };
  });

  app.post('/api/ingest/sessions/:id/end', async (request, reply) => {
    const { id } = request.params as { id: string };
    const appId = getAppId(request);

    const session = await scopedDb(request).endIngestSession(id);

    if (!session) {
      return reply.status(404).send({ error: 'Active ingest session not found' });
    }

    // Audit log
    await scopedDb(request).createAuditEntry(appId, 'ingest.ended', session.device_id as string, undefined, {
      session_id: session.id,
      stream_id: session.stream_id,
    });

    return { ok: true, session };
  });

  // =========================================================================
  // nTV v1 API Endpoints
  // =========================================================================

  // POST /api/v1/bootstrap-tokens - Create a bootstrap token for device enrollment
  app.post('/api/v1/bootstrap-tokens', async (request, reply) => {
    try {
      const body = request.body as CreateBootstrapTokenRequest;
      const appId = getAppId(request);

      if (!body.name) {
        return reply.status(400).send({ error: 'name is required' });
      }

      const token = crypto.randomBytes(32).toString('hex');
      const expiresAt = new Date(Date.now() + fullConfig.bootstrapTokenTtl * 1000);
      const capabilities = body.capabilities ?? [];

      const record = await scopedDb(request).createBootstrapToken(
        body.name,
        token,
        capabilities,
        expiresAt
      );

      await scopedDb(request).createAuditEntry(appId, 'bootstrap_token.created', undefined, undefined, {
        token_id: record.id,
        name: body.name,
      });

      return reply.status(201).send({
        token: record.token,
        expires_at: record.expires_at,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create bootstrap token failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // POST /api/v1/devices/enroll - Enroll a device using bootstrap token
  app.post('/api/v1/devices/enroll', async (request, reply) => {
    try {
      const body = request.body as EnrollDeviceRequest;
      const appId = getAppId(request);

      if (!body.token || !body.name || !body.public_key) {
        return reply.status(400).send({ error: 'token, name, and public_key are required' });
      }

      // Validate the bootstrap token
      const bootstrapToken = await scopedDb(request).getBootstrapToken(body.token);

      if (!bootstrapToken) {
        return reply.status(401).send({ error: 'Invalid bootstrap token' });
      }

      if (bootstrapToken.used) {
        return reply.status(410).send({ error: 'Bootstrap token has already been used' });
      }

      if (new Date(bootstrapToken.expires_at) < new Date()) {
        return reply.status(410).send({ error: 'Bootstrap token has expired' });
      }

      // Create the device record
      const capabilities = bootstrapToken.capabilities ?? [];
      const device = await scopedDb(request).enrollDeviceWithToken(
        appId,
        body.name,
        body.public_key,
        capabilities
      );

      // Mark the token as used
      await scopedDb(request).markBootstrapTokenUsed(body.token, device.id);

      // Audit log
      await scopedDb(request).createAuditEntry(appId, 'device.enrolled_via_token', device.id, undefined, {
        token_name: bootstrapToken.name,
        device_name: body.name,
      });

      return reply.status(201).send(device);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Enroll device via token failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // POST /api/v1/devices/:id/heartbeat - Device heartbeat with telemetry
  app.post('/api/v1/devices/:id/heartbeat', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const body = (request.body as DeviceHeartbeatRequest) ?? {};
      const appId = getAppId(request);
      const ip = request.ip;

      // Update device heartbeat with signal quality check
      const device = await scopedDb(request).updateDeviceHeartbeatWithTelemetry(
        id,
        ip,
        body.signal_quality
      );

      if (!device) {
        return reply.status(404).send({ error: 'Device not found' });
      }

      // Store telemetry data if provided
      const telemetryData: Record<string, unknown> = {};
      if (body.cpu_usage !== undefined) telemetryData.cpu_usage = body.cpu_usage;
      if (body.memory_usage !== undefined) telemetryData.memory_usage = body.memory_usage;
      if (body.temperature !== undefined) telemetryData.temperature = body.temperature;
      if (body.disk_usage !== undefined) telemetryData.disk_usage = body.disk_usage;
      if (body.signal_quality !== undefined) telemetryData.signal_quality = body.signal_quality;

      if (Object.keys(telemetryData).length > 0) {
        await scopedDb(request).submitTelemetry(appId, id, {
          telemetry_type: 'heartbeat',
          data: telemetryData,
        });
      }

      // Compute status for response
      let status: 'online' | 'degraded' = 'online';
      if (body.signal_quality !== undefined && body.signal_quality < 0.5) {
        status = 'degraded';
      }

      return {
        ok: true,
        status,
        next_heartbeat_seconds: fullConfig.heartbeatInterval,
        offline_timeout_seconds: fullConfig.heartbeatOfflineTimeout,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('v1 heartbeat failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // POST /api/v1/devices/:id/commands - Send command to device
  app.post('/api/v1/devices/:id/commands', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const body = request.body as SendCommandRequest;
      const appId = getAppId(request);

      if (!body.type) {
        return reply.status(400).send({ error: 'type is required' });
      }

      const validTypes = ['SCAN_CHANNELS', 'START_EVENT', 'STOP_EVENT', 'HEALTH', 'UPDATE'];
      if (!validTypes.includes(body.type)) {
        return reply.status(400).send({
          error: `Invalid command type. Must be one of: ${validTypes.join(', ')}`,
        });
      }

      // Verify device exists
      const device = await scopedDb(request).getDevice(id);
      if (!device) {
        return reply.status(404).send({ error: 'Device not found' });
      }

      const timeout = fullConfig.commandDefaultTimeout;
      const command = await scopedDb(request).dispatchCommand(appId, id, {
        device_id: id,
        command_type: body.type,
        payload: body.payload,
      }, timeout);

      // Audit log
      await scopedDb(request).createAuditEntry(appId, 'command.dispatched', id, undefined, {
        command_id: command.id,
        command_type: body.type,
      });

      return reply.status(201).send({ commandId: command.id });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('v1 send command failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // GET /api/v1/devices - List all devices for the account
  app.get('/api/v1/devices', async (request) => {
    const { status, type, trust_level, limit = 100, offset = 0 } = request.query as {
      status?: DeviceStatus;
      type?: DeviceType;
      trust_level?: TrustLevel;
      limit?: number;
      offset?: number;
    };
    const appId = getAppId(request);
    const devices = await scopedDb(request).listDevices(appId, status, type, trust_level, limit, offset);

    // Augment each device with computed online/offline/degraded status
    const now = Date.now();
    const timeoutMs = fullConfig.heartbeatOfflineTimeout * 1000;
    const data = devices.map(d => {
      let heartbeatStatus: 'online' | 'offline' | 'degraded' = 'offline';
      if (d.last_seen_at && (now - new Date(d.last_seen_at as string | Date).getTime()) < timeoutMs) {
        const meta = d.metadata as Record<string, unknown> | undefined;
        heartbeatStatus = meta?.heartbeat_status === 'degraded' ? 'degraded' : 'online';
      }
      return { ...d, heartbeat_status: heartbeatStatus };
    });

    return { data, limit, offset };
  });

  // GET /api/v1/devices/:id - Get single device
  app.get('/api/v1/devices/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const device = await scopedDb(request).getDevice(id);

    if (!device) {
      return reply.status(404).send({ error: 'Device not found' });
    }

    // Compute heartbeat status
    const now = Date.now();
    const timeoutMs = fullConfig.heartbeatOfflineTimeout * 1000;
    let heartbeatStatus: 'online' | 'offline' | 'degraded' = 'offline';
    if (device.last_seen_at && (now - new Date(device.last_seen_at as string | Date).getTime()) < timeoutMs) {
      const meta = device.metadata as Record<string, unknown> | undefined;
      heartbeatStatus = meta?.heartbeat_status === 'degraded' ? 'degraded' : 'online';
    }

    return { ...device, heartbeat_status: heartbeatStatus };
  });

  // DELETE /api/v1/devices/:id - Revoke/delete a device
  app.delete('/api/v1/devices/:id', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const appId = getAppId(request);

      const device = await scopedDb(request).getDevice(id);
      if (!device) {
        return reply.status(404).send({ error: 'Device not found' });
      }

      // Audit log before deletion
      await scopedDb(request).createAuditEntry(appId, 'device.deleted', id, undefined, {
        device_id: device.device_id,
        name: device.name,
      });

      const deleted = await scopedDb(request).deleteDevice(id);

      if (!deleted) {
        return reply.status(500).send({ error: 'Failed to delete device' });
      }

      return reply.status(204).send();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('v1 delete device failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getFleetStats();
    return {
      plugin: 'devices',
      version: '1.0.0',
      stats,
      config: {
        heartbeatInterval: fullConfig.heartbeatInterval,
        heartbeatTimeout: fullConfig.heartbeatTimeout,
        commandDefaultTimeout: fullConfig.commandDefaultTimeout,
        commandMaxRetries: fullConfig.commandMaxRetries,
        ingestHeartbeatInterval: fullConfig.ingestHeartbeatInterval,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // Graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down...');
    await app.close();
    await db.disconnect();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return {
    app,
    db,
    start: async () => {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.success(`Devices plugin server running on http://${fullConfig.host}:${fullConfig.port}`);
      logger.info(`Heartbeat: ${fullConfig.heartbeatInterval}s, Command timeout: ${fullConfig.commandDefaultTimeout}s`);
    },
    stop: shutdown,
  };
}

// Start server if run directly
const isMainModule = import.meta.url === `file://${process.argv[1]}`;
if (isMainModule) {
  createServer()
    .then(server => server.start())
    .catch(error => {
      logger.error('Failed to start server', { error: error.message });
      process.exit(1);
    });
}
