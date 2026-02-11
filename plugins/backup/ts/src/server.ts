/**
 * Backup Plugin Server
 * HTTP server for backup management API
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createReadStream } from 'fs';
import { stat } from 'fs/promises';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { BackupDatabase } from './database.js';
import { BackupService } from './backup.js';
import { BackupScheduler } from './scheduler.js';
import { loadConfig } from './config.js';
import type { BackupPluginConfig } from './types.js';
import type {
  CreateScheduleRequest,
  UpdateScheduleRequest,
  RestoreRequest,
} from './types.js';

const logger = createLogger('backup:server');

export async function createServer(config?: Partial<BackupPluginConfig>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new BackupDatabase();
  await db.connect();
  await db.initializeSchema();

  const backupService = new BackupService(fullConfig, db);
  const scheduler = new BackupScheduler(db, backupService, fullConfig.maxConcurrent);

  // Start scheduler
  scheduler.start();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax,
    fullConfig.security.rateLimitWindowMs
  );

  // Add rate limiting to all requests
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  // Add API key authentication (skips health check endpoints)
  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context: resolve source_account_id per request and create scoped DB
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  /** Extract scoped BackupDatabase from request */
  function scopedDb(request: unknown): BackupDatabase {
    return (request as Record<string, unknown>).scopedDb as BackupDatabase;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'backup', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'backup', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'backup',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'backup',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Status Endpoint
  // =========================================================================

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'backup',
      version: '1.0.0',
      status: 'running',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Schedule Endpoints
  // =========================================================================

  app.post<{ Body: CreateScheduleRequest }>('/v1/schedules', async (request, reply) => {
    try {
      const data = request.body;

      // Validate cron expression
      if (!BackupScheduler.validateCronExpression(data.schedule_cron)) {
        return reply.status(400).send({ error: 'Invalid cron expression' });
      }

      const schedule = await scopedDb(request).createSchedule(data);

      // Calculate next run time
      const nextRunAt = scheduler.calculateNextRun(schedule.schedule_cron);
      if (nextRunAt) {
        await scopedDb(request).updateScheduleLastRun(schedule.id, new Date(), nextRunAt);
      }

      return schedule;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create schedule', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/schedules', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const schedules = await scopedDb(request).listSchedules(limit, offset);
    return { data: schedules, limit, offset };
  });

  app.get<{ Params: { id: string } }>('/v1/schedules/:id', async (request, reply) => {
    const { id } = request.params;
    const schedule = await scopedDb(request).getSchedule(id);
    if (!schedule) {
      return reply.status(404).send({ error: 'Schedule not found' });
    }
    return schedule;
  });

  app.put<{ Params: { id: string }; Body: UpdateScheduleRequest }>('/v1/schedules/:id', async (request, reply) => {
    try {
      const { id } = request.params;
      const data = request.body;

      // Validate cron expression if provided
      if (data.schedule_cron && !BackupScheduler.validateCronExpression(data.schedule_cron)) {
        return reply.status(400).send({ error: 'Invalid cron expression' });
      }

      const schedule = await scopedDb(request).updateSchedule(id, data);
      if (!schedule) {
        return reply.status(404).send({ error: 'Schedule not found' });
      }

      // Recalculate next run if cron changed
      if (data.schedule_cron) {
        const nextRunAt = scheduler.calculateNextRun(schedule.schedule_cron);
        if (nextRunAt) {
          await scopedDb(request).updateScheduleLastRun(schedule.id, schedule.last_run_at ?? new Date(), nextRunAt);
        }
      }

      return schedule;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to update schedule', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete<{ Params: { id: string } }>('/v1/schedules/:id', async (request, reply) => {
    const { id } = request.params;
    const deleted = await scopedDb(request).deleteSchedule(id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Schedule not found' });
    }
    return { deleted: true };
  });

  app.post<{ Params: { id: string } }>('/v1/schedules/:id/run', async (request, reply) => {
    try {
      const { id } = request.params;
      const schedule = await scopedDb(request).getSchedule(id);

      if (!schedule) {
        return reply.status(404).send({ error: 'Schedule not found' });
      }

      // Trigger backup
      const result = await backupService.executeBackup({
        scheduleId: schedule.id,
        backupType: schedule.backup_type,
        includeTables: schedule.include_tables.length > 0 ? schedule.include_tables : undefined,
        excludeTables: schedule.exclude_tables.length > 0 ? schedule.exclude_tables : undefined,
        compression: schedule.compression,
        encryption: schedule.encryption_enabled ? {
          enabled: true,
          keyId: schedule.encryption_key_id ?? undefined,
        } : undefined,
        targetProvider: schedule.target_provider,
        targetConfig: schedule.target_config,
        retentionDays: schedule.retention_days,
      });

      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to run backup', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Artifact Endpoints
  // =========================================================================

  app.get('/v1/artifacts', async (request) => {
    const { limit = 100, offset = 0, status } = request.query as {
      limit?: number;
      offset?: number;
      status?: string;
    };
    const artifacts = await scopedDb(request).listArtifacts(limit, offset, status);
    return { data: artifacts, limit, offset };
  });

  app.get<{ Params: { id: string } }>('/v1/artifacts/:id', async (request, reply) => {
    const { id } = request.params;
    const artifact = await scopedDb(request).getArtifact(id);
    if (!artifact) {
      return reply.status(404).send({ error: 'Artifact not found' });
    }
    return artifact;
  });

  app.delete<{ Params: { id: string } }>('/v1/artifacts/:id', async (request, reply) => {
    const { id } = request.params;
    const artifact = await scopedDb(request).getArtifact(id);

    if (!artifact) {
      return reply.status(404).send({ error: 'Artifact not found' });
    }

    // Delete file if exists
    if (artifact.file_path) {
      try {
        await import('fs/promises').then(fs => fs.unlink(artifact.file_path!));
      } catch (error) {
        logger.warn('Failed to delete backup file', { path: artifact.file_path, error });
      }
    }

    const deleted = await scopedDb(request).deleteArtifact(id);
    return { deleted };
  });

  app.get<{ Params: { id: string } }>('/v1/artifacts/:id/download', async (request, reply) => {
    const { id } = request.params;
    const artifact = await scopedDb(request).getArtifact(id);

    if (!artifact) {
      return reply.status(404).send({ error: 'Artifact not found' });
    }

    if (!artifact.file_path) {
      return reply.status(404).send({ error: 'Backup file not found' });
    }

    try {
      const stats = await stat(artifact.file_path);
      const stream = createReadStream(artifact.file_path);

      reply.header('Content-Type', 'application/octet-stream');
      reply.header('Content-Disposition', `attachment; filename="backup-${id}.pgdump"`);
      reply.header('Content-Length', stats.size);

      return reply.send(stream);
    } catch (error) {
      logger.error('Failed to download artifact', { id, error });
      return reply.status(500).send({ error: 'Failed to read backup file' });
    }
  });

  // =========================================================================
  // Restore Endpoints
  // =========================================================================

  app.post<{ Body: RestoreRequest }>('/v1/restore', async (request, reply) => {
    try {
      const data = request.body;

      const result = await backupService.executeRestore({
        artifactId: data.artifact_id,
        targetDatabase: data.target_database ?? fullConfig.databaseName,
        tablesToRestore: data.tables_to_restore,
        restoreMode: data.restore_mode ?? 'merge',
        conflictStrategy: data.conflict_strategy ?? 'skip',
      });

      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start restore', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Params: { id: string } }>('/v1/restore/:id', async (request, reply) => {
    const { id } = request.params;
    const job = await scopedDb(request).getRestoreJob(id);
    if (!job) {
      return reply.status(404).send({ error: 'Restore job not found' });
    }
    return job;
  });

  app.post<{ Params: { id: string } }>('/v1/restore/:id/cancel', async (request, reply) => {
    const { id } = request.params;
    const job = await scopedDb(request).getRestoreJob(id);

    if (!job) {
      return reply.status(404).send({ error: 'Restore job not found' });
    }

    if (job.status !== 'pending' && job.status !== 'running') {
      return reply.status(400).send({ error: 'Cannot cancel completed job' });
    }

    await scopedDb(request).updateRestoreJob(id, {
      status: 'cancelled',
      completedAt: new Date(),
    });

    return { cancelled: true };
  });

  // Start server
  const start = async () => {
    try {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.info(`Server listening on ${fullConfig.host}:${fullConfig.port}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed to start', { error: message });
      process.exit(1);
    }
  };

  // Graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down server...');
    scheduler.stop();
    await app.close();
    await db.disconnect();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return { app, start };
}
