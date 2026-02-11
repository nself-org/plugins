/**
 * Export/Import Plugin Server
 * HTTP server for data export, import, migration, backup, restore, and transformations
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { ExportImportDatabase } from './database.js';
import { loadConfig, type ExportImportConfig } from './config.js';
import type {
  CreateExportJobRequest,
  CreateImportJobRequest,
  CreateMigrationJobRequest,
  CreateBackupSnapshotRequest,
  CreateRestoreJobRequest,
  CreateTransformTemplateRequest,
  UpdateTransformTemplateRequest,
  EstimateExportRequest,
  BackupScheduleConfig,
  ExportJobStatus,
  ImportJobStatus,
  MigrationJobStatus,
  JobType,
} from './types.js';

const logger = createLogger('export-import:server');

export async function createServer(config?: Partial<ExportImportConfig>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new ExportImportDatabase();
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 100 * 1024 * 1024, // 100MB for file uploads
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(fullConfig.rateLimitMax, fullConfig.rateLimitWindowMs);

  // Add rate limiting to all requests
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  // Add API key authentication (skips health check endpoints)
  if (fullConfig.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context: resolve source_account_id per request
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    const scopedDb = db.forSourceAccount(ctx.sourceAccountId);
    (request as unknown as Record<string, unknown>).scopedDb = scopedDb;
  });

  /** Extract scoped database from request */
  function scopedDb(request: unknown): ExportImportDatabase {
    return (request as Record<string, unknown>).scopedDb as ExportImportDatabase;
  }

  // =========================================================================
  // Health & Status Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'export-import', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'export-import', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'export-import',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'export-import',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'export-import',
      version: '1.0.0',
      status: 'running',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Export Endpoints
  // =========================================================================

  app.post<{ Body: CreateExportJobRequest }>('/api/export/create', async (request, reply) => {
    const data = request.body;
    if (!data.user_id || !data.name || !data.export_type || !data.format || !data.scope) {
      return reply.status(400).send({ error: 'user_id, name, export_type, format, and scope are required' });
    }
    try {
      const job = await scopedDb(request).createExportJob(data);
      return job;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Export job creation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/export/jobs', async (request) => {
    const { limit = 100, offset = 0, status } = request.query as {
      limit?: number; offset?: number; status?: ExportJobStatus;
    };
    const jobs = await scopedDb(request).listExportJobs(limit, offset, status);
    return { data: jobs, limit, offset };
  });

  app.get<{ Params: { id: string } }>('/api/export/jobs/:id', async (request, reply) => {
    const job = await scopedDb(request).getExportJob(request.params.id);
    if (!job) return reply.status(404).send({ error: 'Export job not found' });
    return job;
  });

  app.get<{ Params: { id: string } }>('/api/export/jobs/:id/download', async (request, reply) => {
    const job = await scopedDb(request).getExportJob(request.params.id);
    if (!job) return reply.status(404).send({ error: 'Export job not found' });
    if (job.status !== 'completed' || !job.output_path) {
      return reply.status(400).send({ error: 'Export job is not completed or has no output' });
    }
    return { download_url: job.output_path, checksum: job.checksum, size_bytes: job.output_size_bytes };
  });

  app.delete<{ Params: { id: string } }>('/api/export/jobs/:id', async (request, reply) => {
    const job = await scopedDb(request).getExportJob(request.params.id);
    if (!job) return reply.status(404).send({ error: 'Export job not found' });

    if (job.status === 'running') {
      await scopedDb(request).updateExportJobStatus(request.params.id, 'cancelled');
      return { success: true, action: 'cancelled' };
    }

    const deleted = await scopedDb(request).deleteExportJob(request.params.id);
    return { success: deleted, action: 'deleted' };
  });

  app.post<{ Body: EstimateExportRequest }>('/api/export/estimate', async (request, reply) => {
    const { scope } = request.body;
    if (!scope) return reply.status(400).send({ error: 'scope is required' });
    // Estimation logic would query actual data tables; returning a placeholder
    return { estimated_size_bytes: 0, estimated_records: 0, scope };
  });

  app.get('/api/export/templates', async (request) => {
    const templates = await scopedDb(request).listTransformTemplates(100, 0);
    const exportTemplates = templates.filter(t => t.source_format === 'database');
    return { data: exportTemplates };
  });

  // =========================================================================
  // Import Endpoints
  // =========================================================================

  app.post<{ Body: CreateImportJobRequest }>('/api/import/create', async (request, reply) => {
    const data = request.body;
    if (!data.user_id || !data.name || !data.import_type || !data.source_format || !data.source_path) {
      return reply.status(400).send({
        error: 'user_id, name, import_type, source_format, and source_path are required',
      });
    }
    try {
      const job = await scopedDb(request).createImportJob(data);
      return job;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Import job creation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/import/validate', async (_request, reply) => {
    // File validation would parse the file and check schema; returning placeholder
    return reply.status(200).send({ valid: true, errors: [], warnings: [] });
  });

  app.post('/api/import/upload', async (_request, reply) => {
    // File upload would store the file and return a path; returning placeholder
    return reply.status(200).send({ path: '/tmp/uploaded-file', size_bytes: 0 });
  });

  app.get('/api/import/jobs', async (request) => {
    const { limit = 100, offset = 0, status } = request.query as {
      limit?: number; offset?: number; status?: ImportJobStatus;
    };
    const jobs = await scopedDb(request).listImportJobs(limit, offset, status);
    return { data: jobs, limit, offset };
  });

  app.get<{ Params: { id: string } }>('/api/import/jobs/:id', async (request, reply) => {
    const job = await scopedDb(request).getImportJob(request.params.id);
    if (!job) return reply.status(404).send({ error: 'Import job not found' });
    return job;
  });

  app.post<{ Params: { id: string } }>('/api/import/jobs/:id/start', async (request, reply) => {
    const job = await scopedDb(request).getImportJob(request.params.id);
    if (!job) return reply.status(404).send({ error: 'Import job not found' });
    if (job.status !== 'pending') {
      return reply.status(400).send({ error: `Cannot start job with status: ${job.status}` });
    }
    const updated = await scopedDb(request).updateImportJobStatus(request.params.id, 'running');
    return updated;
  });

  app.delete<{ Params: { id: string } }>('/api/import/jobs/:id', async (request, reply) => {
    const job = await scopedDb(request).getImportJob(request.params.id);
    if (!job) return reply.status(404).send({ error: 'Import job not found' });

    if (job.status === 'running' || job.status === 'validating') {
      await scopedDb(request).updateImportJobStatus(request.params.id, 'cancelled');
      return { success: true, action: 'cancelled' };
    }

    const deleted = await scopedDb(request).deleteImportJob(request.params.id);
    return { success: deleted, action: 'deleted' };
  });

  app.get<{ Params: { format: string } }>('/api/import/mappings/:format', async (request) => {
    const { format } = request.params;
    // Return default field mappings for known platform formats
    const mappings: Record<string, Record<string, string>> = {
      slack: { channels: 'channels', messages: 'messages', users: 'users', files: 'files' },
      discord: { guilds: 'channels', messages: 'messages', members: 'users', attachments: 'files' },
      teams: { teams: 'channels', messages: 'messages', members: 'users', files: 'files' },
      json: { data: 'data' },
      csv: { rows: 'data' },
    };
    return { format, mappings: mappings[format] ?? {} };
  });

  // =========================================================================
  // Migration Endpoints
  // =========================================================================

  app.post('/api/migrate/analyze', async (_request, reply) => {
    // Analysis would connect to source platform and assess data; returning placeholder
    return reply.status(200).send({
      platform_status: 'connected',
      available_data: { channels: 0, messages: 0, users: 0, files: 0 },
      estimated_duration_minutes: 0,
    });
  });

  app.post<{ Body: CreateMigrationJobRequest }>('/api/migrate/create', async (request, reply) => {
    const data = request.body;
    if (!data.user_id || !data.name || !data.source_platform || !data.source_credentials || !data.destination_scope || !data.migration_plan) {
      return reply.status(400).send({ error: 'All migration fields are required' });
    }
    try {
      const job = await scopedDb(request).createMigrationJob(data);
      return job;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Migration job creation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/migrate/jobs', async (request) => {
    const { limit = 100, offset = 0, status } = request.query as {
      limit?: number; offset?: number; status?: MigrationJobStatus;
    };
    const jobs = await scopedDb(request).listMigrationJobs(limit, offset, status);
    return { data: jobs, limit, offset };
  });

  app.get<{ Params: { id: string } }>('/api/migrate/jobs/:id', async (request, reply) => {
    const job = await scopedDb(request).getMigrationJob(request.params.id);
    if (!job) return reply.status(404).send({ error: 'Migration job not found' });
    return job;
  });

  app.post<{ Params: { id: string } }>('/api/migrate/jobs/:id/start', async (request, reply) => {
    const job = await scopedDb(request).getMigrationJob(request.params.id);
    if (!job) return reply.status(404).send({ error: 'Migration job not found' });
    if (job.status !== 'pending') {
      return reply.status(400).send({ error: `Cannot start job with status: ${job.status}` });
    }
    const updated = await scopedDb(request).updateMigrationJobStatus(request.params.id, 'running', { phase: 'initializing' });
    return updated;
  });

  app.delete<{ Params: { id: string } }>('/api/migrate/jobs/:id', async (request, reply) => {
    const job = await scopedDb(request).getMigrationJob(request.params.id);
    if (!job) return reply.status(404).send({ error: 'Migration job not found' });

    if (job.status === 'running' || job.status === 'analyzing') {
      await scopedDb(request).updateMigrationJobStatus(request.params.id, 'cancelled');
      return { success: true, action: 'cancelled' };
    }

    const deleted = await scopedDb(request).deleteMigrationJob(request.params.id);
    return { success: deleted, action: 'deleted' };
  });

  app.get('/api/migrate/platforms', async () => {
    return {
      platforms: [
        { id: 'slack', name: 'Slack', status: 'supported' },
        { id: 'discord', name: 'Discord', status: 'supported' },
        { id: 'teams', name: 'Microsoft Teams', status: 'supported' },
        { id: 'mattermost', name: 'Mattermost', status: 'supported' },
        { id: 'rocket_chat', name: 'Rocket.Chat', status: 'supported' },
        { id: 'telegram', name: 'Telegram', status: 'supported' },
      ],
    };
  });

  // =========================================================================
  // Backup Endpoints
  // =========================================================================

  app.post<{ Body: CreateBackupSnapshotRequest }>('/api/backup/create', async (request, reply) => {
    const data = request.body;
    if (!data.name || !data.backup_type || !data.scope || !data.compression || !data.storage_backend) {
      return reply.status(400).send({ error: 'name, backup_type, scope, compression, and storage_backend are required' });
    }
    try {
      const snapshot = await scopedDb(request).createBackupSnapshot(data);
      return snapshot;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Backup creation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/backup/snapshots', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const snapshots = await scopedDb(request).listBackupSnapshots(limit, offset);
    return { data: snapshots, limit, offset };
  });

  app.get<{ Params: { id: string } }>('/api/backup/snapshots/:id', async (request, reply) => {
    const snapshot = await scopedDb(request).getBackupSnapshot(request.params.id);
    if (!snapshot) return reply.status(404).send({ error: 'Backup snapshot not found' });
    return snapshot;
  });

  app.post<{ Params: { id: string } }>('/api/backup/snapshots/:id/verify', async (request, reply) => {
    const snapshot = await scopedDb(request).getBackupSnapshot(request.params.id);
    if (!snapshot) return reply.status(404).send({ error: 'Backup snapshot not found' });
    const updated = await scopedDb(request).verifyBackupSnapshot(request.params.id);
    return updated;
  });

  app.delete<{ Params: { id: string } }>('/api/backup/snapshots/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteBackupSnapshot(request.params.id);
    if (!deleted) return reply.status(404).send({ error: 'Backup snapshot not found' });
    return { success: true };
  });

  app.get('/api/backup/schedule', async (request) => {
    const schedule = await scopedDb(request).getBackupSchedule();
    return schedule ?? { enabled: false, frequency: 'daily', time: '02:00', backup_type: 'incremental', retention_days: 90, storage_backend: 'local' };
  });

  app.put<{ Body: BackupScheduleConfig }>('/api/backup/schedule', async (request, reply) => {
    const data = request.body;
    if (!data.frequency || !data.time || !data.backup_type || !data.storage_backend) {
      return reply.status(400).send({ error: 'frequency, time, backup_type, and storage_backend are required' });
    }
    try {
      const schedule = await scopedDb(request).upsertBackupSchedule(data);
      return schedule;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Backup schedule update failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Restore Endpoints
  // =========================================================================

  app.post<{ Body: CreateRestoreJobRequest }>('/api/restore/create', async (request, reply) => {
    const data = request.body;
    if (!data.user_id || !data.snapshot_id || !data.restore_type || !data.target_scope) {
      return reply.status(400).send({ error: 'user_id, snapshot_id, restore_type, and target_scope are required' });
    }

    // Verify snapshot exists
    const snapshot = await scopedDb(request).getBackupSnapshot(data.snapshot_id);
    if (!snapshot) return reply.status(404).send({ error: 'Backup snapshot not found' });

    try {
      const job = await scopedDb(request).createRestoreJob(data);
      return job;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Restore job creation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Body: CreateRestoreJobRequest }>('/api/restore/preview', async (request, reply) => {
    const { snapshot_id, target_scope } = request.body;
    if (!snapshot_id || !target_scope) {
      return reply.status(400).send({ error: 'snapshot_id and target_scope are required' });
    }
    const snapshot = await scopedDb(request).getBackupSnapshot(snapshot_id);
    if (!snapshot) return reply.status(404).send({ error: 'Backup snapshot not found' });
    return {
      snapshot_id,
      snapshot_name: snapshot.name,
      target_scope,
      estimated_items: 0,
      conflicts: [],
    };
  });

  app.get('/api/restore/jobs', async (request) => {
    const { limit = 100, offset = 0, snapshot_id } = request.query as {
      limit?: number; offset?: number; snapshot_id?: string;
    };
    const jobs = await scopedDb(request).listRestoreJobs(limit, offset, snapshot_id);
    return { data: jobs, limit, offset };
  });

  app.get<{ Params: { id: string } }>('/api/restore/jobs/:id', async (request, reply) => {
    const job = await scopedDb(request).getRestoreJob(request.params.id);
    if (!job) return reply.status(404).send({ error: 'Restore job not found' });
    return job;
  });

  app.post<{ Params: { id: string } }>('/api/restore/jobs/:id/start', async (request, reply) => {
    const job = await scopedDb(request).getRestoreJob(request.params.id);
    if (!job) return reply.status(404).send({ error: 'Restore job not found' });
    if (job.status !== 'pending') {
      return reply.status(400).send({ error: `Cannot start job with status: ${job.status}` });
    }
    const updated = await scopedDb(request).updateRestoreJobStatus(request.params.id, 'running');
    return updated;
  });

  app.delete<{ Params: { id: string } }>('/api/restore/jobs/:id', async (request, reply) => {
    const job = await scopedDb(request).getRestoreJob(request.params.id);
    if (!job) return reply.status(404).send({ error: 'Restore job not found' });

    if (job.status === 'running' || job.status === 'validating') {
      await scopedDb(request).updateRestoreJobStatus(request.params.id, 'cancelled');
      return { success: true, action: 'cancelled' };
    }

    const deleted = await scopedDb(request).deleteRestoreJob(request.params.id);
    return { success: deleted, action: 'deleted' };
  });

  // =========================================================================
  // Transform Template Endpoints
  // =========================================================================

  app.get('/api/transform/templates', async (request) => {
    const { limit = 100, offset = 0, source_format, target_format } = request.query as {
      limit?: number; offset?: number; source_format?: string; target_format?: string;
    };
    const templates = await scopedDb(request).listTransformTemplates(limit, offset, {
      source_format,
      target_format,
    });
    return { data: templates, limit, offset };
  });

  app.post<{ Body: CreateTransformTemplateRequest }>('/api/transform/templates', async (request, reply) => {
    const data = request.body;
    if (!data.name || !data.source_format || !data.target_format || !data.transformations) {
      return reply.status(400).send({ error: 'name, source_format, target_format, and transformations are required' });
    }
    try {
      const template = await scopedDb(request).createTransformTemplate(data);
      return template;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Transform template creation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Params: { id: string } }>('/api/transform/templates/:id', async (request, reply) => {
    const template = await scopedDb(request).getTransformTemplate(request.params.id);
    if (!template) return reply.status(404).send({ error: 'Transform template not found' });
    return template;
  });

  app.put<{ Params: { id: string }; Body: UpdateTransformTemplateRequest }>('/api/transform/templates/:id', async (request, reply) => {
    try {
      const template = await scopedDb(request).updateTransformTemplate(request.params.id, request.body);
      if (!template) return reply.status(404).send({ error: 'Transform template not found' });
      return template;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Transform template update failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete<{ Params: { id: string } }>('/api/transform/templates/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteTransformTemplate(request.params.id);
    if (!deleted) return reply.status(404).send({ error: 'Transform template not found' });
    return { success: true };
  });

  app.post<{ Body: { template_id: string; data: unknown } }>('/api/transform/apply', async (request, reply) => {
    const { template_id } = request.body;
    if (!template_id) return reply.status(400).send({ error: 'template_id is required' });

    const template = await scopedDb(request).getTransformTemplate(template_id);
    if (!template) return reply.status(404).send({ error: 'Transform template not found' });

    await scopedDb(request).incrementTemplateUsage(template_id);
    // Actual transformation logic would be applied here
    return { applied: true, template_id, transformations: template.transformations };
  });

  // =========================================================================
  // Audit Endpoints
  // =========================================================================

  app.get('/api/audit/transfers', async (request) => {
    const { limit = 100, offset = 0, job_type, user_id } = request.query as {
      limit?: number; offset?: number; job_type?: JobType; user_id?: string;
    };
    const entries = await scopedDb(request).listAuditEntries(limit, offset, {
      job_type,
      user_id,
    });
    return { data: entries, limit, offset };
  });

  app.get<{ Params: { id: string } }>('/api/audit/transfers/:id', async (request, reply) => {
    const entry = await scopedDb(request).getAuditEntry(request.params.id);
    if (!entry) return reply.status(404).send({ error: 'Audit entry not found' });
    return entry;
  });

  app.get('/api/audit/export', async (request) => {
    const { limit = 1000, job_type } = request.query as { limit?: number; job_type?: JobType };
    const entries = await scopedDb(request).listAuditEntries(limit, 0, {
      job_type,
    });
    return { data: entries, total: entries.length, format: 'json' };
  });

  // Start server
  const start = async () => {
    try {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.success(`Server listening on ${fullConfig.host}:${fullConfig.port}`);
    } catch (err) {
      const error = err instanceof Error ? err.message : 'Unknown error';
      logger.error('Server failed to start', { error });
      process.exit(1);
    }
  };

  return { app, start, db };
}
