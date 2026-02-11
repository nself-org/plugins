/**
 * Data Export Plugin Server
 * HTTP server for export, deletion, and import API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { promises as fs } from 'fs';
import { ExportDatabase } from './database.js';
import { ExportService } from './export-service.js';
import { loadConfig, type Config } from './config.js';

const logger = createLogger('data-export:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new ExportDatabase();
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 100 * 1024 * 1024, // 100MB for imports
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 50,
    fullConfig.security.rateLimitWindowMs ?? 60000
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
  app.decorateRequest('scopedService', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    const scopedDb = db.forSourceAccount(ctx.sourceAccountId);
    const scopedService = new ExportService(
      scopedDb,
      fullConfig.storagePath,
      fullConfig.downloadExpiryHours,
      fullConfig.deletionCooldownHours,
      fullConfig.verificationCodeLength
    );
    (request as unknown as Record<string, unknown>).scopedDb = scopedDb;
    (request as unknown as Record<string, unknown>).scopedService = scopedService;
  });

  /** Extract scoped ExportDatabase from request */
  function scopedDb(request: unknown): ExportDatabase {
    return (request as Record<string, unknown>).scopedDb as ExportDatabase;
  }

  /** Extract scoped ExportService from request */
  function scopedService(request: unknown): ExportService {
    return (request as Record<string, unknown>).scopedService as ExportService;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'data-export', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'data-export', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'data-export',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'data-export',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        exports: stats.totalExports,
        deletions: stats.totalDeletions,
        lastExport: stats.lastExportAt,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Status Endpoint
  // =========================================================================

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'data-export',
      version: '1.0.0',
      status: 'running',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Export Endpoints
  // =========================================================================

  app.post('/v1/exports', async (request, reply) => {
    const { requestType, requesterId, targetUserId, targetPlugins, format } = request.body as {
      requestType: string;
      requesterId: string;
      targetUserId?: string;
      targetPlugins?: string[];
      format?: string;
    };

    if (!requesterId) {
      return reply.status(400).send({ error: 'requesterId is required' });
    }

    if (!['user_data', 'plugin_data', 'full_backup', 'custom'].includes(requestType)) {
      return reply.status(400).send({ error: 'Invalid requestType' });
    }

    try {
      const id = await scopedDb(request).createExportRequest({
        requestType: requestType as 'user_data' | 'plugin_data' | 'full_backup' | 'custom',
        requesterId,
        targetUserId,
        targetPlugins,
        format: (format as 'json' | 'csv' | 'zip') ?? 'json',
      });

      // Process export asynchronously
      scopedService(request).processExportRequest(id).catch(error => {
        logger.error('Export processing failed', { id, error });
      });

      return { id, status: 'pending', message: 'Export request created' };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create export request', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/exports', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const exports = await scopedDb(request).listExportRequests(limit, offset);
    const total = await scopedDb(request).countExportRequests();
    return { data: exports, total, limit, offset };
  });

  app.get('/v1/exports/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const exportRequest = await scopedDb(request).getExportRequest(id);
    if (!exportRequest) {
      return reply.status(404).send({ error: 'Export request not found' });
    }
    return exportRequest;
  });

  app.get('/v1/exports/:id/download', async (request, reply) => {
    const { token } = request.query as { token: string };

    if (!token) {
      return reply.status(400).send({ error: 'Download token is required' });
    }

    try {
      const result = await scopedService(request).getExportFile(token);
      if (!result) {
        return reply.status(404).send({ error: 'Export not found or expired' });
      }

      const fileContent = await fs.readFile(result.filePath);
      const filename = result.filePath.split('/').pop() ?? 'export.json';

      return reply
        .header('Content-Disposition', `attachment; filename="${filename}"`)
        .type('application/octet-stream')
        .send(fileContent);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Download failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete('/v1/exports/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    try {
      await scopedDb(request).deleteExportRequest(id);
      return { success: true, message: 'Export request deleted' };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to delete export request', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Deletion Endpoints
  // =========================================================================

  app.post('/v1/deletions', async (request, reply) => {
    const { requesterId, targetUserId, reason } = request.body as {
      requesterId: string;
      targetUserId: string;
      reason?: string;
    };

    if (!requesterId || !targetUserId) {
      return reply.status(400).send({ error: 'requesterId and targetUserId are required' });
    }

    try {
      const { id, verificationCode } = await scopedService(request).createDeletionWithVerification(
        requesterId,
        targetUserId,
        reason
      );

      // In production, send verificationCode via email/SMS instead of returning it
      return {
        id,
        status: 'pending',
        message: 'Deletion request created. Verification code sent.',
        // Remove this in production:
        verificationCode,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create deletion request', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/deletions', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const deletions = await scopedDb(request).listDeletionRequests(limit, offset);
    const total = await scopedDb(request).countDeletionRequests();
    return { data: deletions, total, limit, offset };
  });

  app.get('/v1/deletions/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deletion = await scopedDb(request).getDeletionRequest(id);
    if (!deletion) {
      return reply.status(404).send({ error: 'Deletion request not found' });
    }
    return deletion;
  });

  app.post('/v1/deletions/:id/verify', async (request, reply) => {
    const { id } = request.params as { id: string };
    const { code } = request.body as { code: string };

    if (!code) {
      return reply.status(400).send({ error: 'Verification code is required' });
    }

    try {
      const verified = await scopedService(request).verifyDeletion(id, code);
      if (!verified) {
        return reply.status(400).send({ error: 'Invalid verification code' });
      }

      // Process deletion after cooldown (in production, use a job queue)
      const deletion = await scopedDb(request).getDeletionRequest(id);
      if (deletion && deletion.cooldown_until) {
        const cooldownMs = new Date(deletion.cooldown_until).getTime() - Date.now();
        if (cooldownMs > 0) {
          setTimeout(() => {
            scopedService(request).processDeletionRequest(id).catch(error => {
              logger.error('Deletion processing failed', { id, error });
            });
          }, cooldownMs);
        }
      }

      return {
        success: true,
        message: 'Deletion verified. Processing will begin after cooldown period.',
        cooldownUntil: deletion?.cooldown_until,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Verification failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/v1/deletions/:id/cancel', async (request, reply) => {
    const { id } = request.params as { id: string };
    try {
      await scopedDb(request).cancelDeletionRequest(id);
      return { success: true, message: 'Deletion request cancelled' };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to cancel deletion request', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Plugin Registry Endpoints
  // =========================================================================

  app.post('/v1/plugins', async (request, reply) => {
    const { pluginName, tables, userIdColumn, exportQuery, deletionQuery, enabled } = request.body as {
      pluginName: string;
      tables: string[];
      userIdColumn?: string;
      exportQuery?: string;
      deletionQuery?: string;
      enabled?: boolean;
    };

    if (!pluginName || !tables || tables.length === 0) {
      return reply.status(400).send({ error: 'pluginName and tables are required' });
    }

    try {
      const id = await scopedDb(request).registerPlugin({
        pluginName,
        tables,
        userIdColumn,
        exportQuery,
        deletionQuery,
        enabled,
      });

      return { id, message: 'Plugin registered successfully' };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to register plugin', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/plugins', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const plugins = await scopedDb(request).listPluginRegistry(limit, offset);
    const total = await scopedDb(request).countPluginRegistry();
    return { data: plugins, total, limit, offset };
  });

  app.put('/v1/plugins/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const { tables, userIdColumn, exportQuery, deletionQuery, enabled } = request.body as {
      tables?: string[];
      userIdColumn?: string;
      exportQuery?: string;
      deletionQuery?: string;
      enabled?: boolean;
    };

    try {
      await scopedDb(request).updatePluginRegistry(id, {
        tables,
        userIdColumn,
        exportQuery,
        deletionQuery,
        enabled,
      });

      return { success: true, message: 'Plugin updated successfully' };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to update plugin', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete('/v1/plugins/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    try {
      await scopedDb(request).deletePluginRegistry(id);
      return { success: true, message: 'Plugin unregistered successfully' };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to unregister plugin', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Import Endpoints
  // =========================================================================

  app.post('/v1/import', async (request, reply) => {
    const { requesterId, sourceType, sourcePath, format } = request.body as {
      requesterId: string;
      sourceType: string;
      sourcePath: string;
      format?: string;
    };

    if (!requesterId || !sourceType || !sourcePath) {
      return reply.status(400).send({ error: 'requesterId, sourceType, and sourcePath are required' });
    }

    try {
      const id = await scopedDb(request).createImportJob({
        requesterId,
        sourceType: sourceType as 'file' | 'url',
        sourcePath,
        format: (format as 'json' | 'csv' | 'zip') ?? 'json',
      });

      // Process import asynchronously
      scopedService(request).processImportJob(id).catch(error => {
        logger.error('Import processing failed', { id, error });
      });

      return { id, status: 'pending', message: 'Import job created' };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create import job', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/import/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const job = await scopedDb(request).getImportJob(id);
    if (!job) {
      return reply.status(404).send({ error: 'Import job not found' });
    }
    return job;
  });

  // =========================================================================
  // Statistics Endpoint
  // =========================================================================

  app.get('/v1/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    return stats;
  });

  // Start server
  app.listen({ port: fullConfig.port, host: fullConfig.host }, (err, address) => {
    if (err) {
      const message = err instanceof Error ? err.message : 'Unknown error';
      logger.error('Failed to start server', { error: message });
      process.exit(1);
    }
    logger.success(`Data Export server listening on ${address}`);
  });

  return app;
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  createServer().catch(error => {
    logger.error('Failed to start server', error);
    process.exit(1);
  });
}
