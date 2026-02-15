/**
 * Admin API Plugin Server
 * Express HTTP server with optional WebSocket support for real-time dashboard updates
 */

import http from 'node:http';
import os from 'node:os';
import express from 'express';
import { WebSocketServer, WebSocket } from 'ws';
import { createLogger, getAppContext } from '@nself/plugin-utils';
import { AdminApiDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  SystemMetrics,
  SystemHealthOverview,
  CreateMetricsSnapshotRequest,
  UpdateDashboardConfigRequest,
  MetricsQueryParams,
  WsMessage,
  MetricType,
} from './types.js';

const logger = createLogger('admin-api:server');

// =========================================================================
// Helpers
// =========================================================================

function getSystemMetrics(): SystemMetrics {
  const mem = process.memoryUsage();
  const cpus = os.cpus();
  const loadAvg = os.loadavg();
  const totalMem = os.totalmem();
  const freeMem = os.freemem();

  // Rough CPU usage from cpus
  let totalIdle = 0;
  let totalTick = 0;
  for (const cpu of cpus) {
    for (const type of Object.keys(cpu.times) as Array<keyof typeof cpu.times>) {
      totalTick += cpu.times[type];
    }
    totalIdle += cpu.times.idle;
  }
  const cpuUsage = totalTick > 0 ? ((1 - totalIdle / totalTick) * 100) : 0;

  return {
    cpu: {
      usage_percent: Math.round(cpuUsage * 100) / 100,
      load_average_1m: loadAvg[0],
      load_average_5m: loadAvg[1],
      load_average_15m: loadAvg[2],
    },
    memory: {
      used_bytes: totalMem - freeMem,
      total_bytes: totalMem,
      free_bytes: freeMem,
      usage_percent: Math.round(((totalMem - freeMem) / totalMem) * 10000) / 100,
      heap_used_bytes: mem.heapUsed,
      heap_total_bytes: mem.heapTotal,
      external_bytes: mem.external,
      rss_bytes: mem.rss,
    },
    disk: {
      used_bytes: 0,
      total_bytes: 0,
      free_bytes: 0,
      usage_percent: 0,
    },
    network: {
      active_connections: 0,
      requests_per_minute: 0,
      avg_response_time_ms: 0,
      error_rate_percent: 0,
    },
    process: {
      uptime_seconds: process.uptime(),
      pid: process.pid,
      node_version: process.version,
      platform: process.platform,
    },
    timestamp: new Date().toISOString(),
  };
}

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new AdminApiDatabase();
  await db.connect();
  await db.initializeSchema();

  // Create Express app
  const app = express();
  app.use(express.json({ limit: '10mb' }));

  // CORS middleware
  app.use((_req, res, next) => {
    res.header('Access-Control-Allow-Origin', '*');
    res.header('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
    res.header('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-App-Id, X-Source-Account-Id');
    if (_req.method === 'OPTIONS') {
      res.sendStatus(204);
      return;
    }
    next();
  });

  // Multi-app context middleware
  function scopedDb(req: express.Request): AdminApiDatabase {
    const ctx = getAppContext(req);
    return db.forSourceAccount(ctx.sourceAccountId);
  }

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', (_req, res) => {
    res.json({ status: 'ok', plugin: 'admin-api', timestamp: new Date().toISOString() });
  });

  app.get('/ready', async (_req, res) => {
    try {
      await db.query('SELECT 1');
      res.json({ ready: true, plugin: 'admin-api', timestamp: new Date().toISOString() });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      res.status(503).json({
        ready: false,
        plugin: 'admin-api',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (req, res) => {
    const stats = await scopedDb(req).getStats();
    res.json({
      alive: true,
      plugin: 'admin-api',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        snapshotsTotal: stats.snapshots_total,
        snapshotsToday: stats.snapshots_today,
        configEntries: stats.config_entries,
      },
      timestamp: new Date().toISOString(),
    });
  });

  // =========================================================================
  // Metrics Endpoints
  // =========================================================================

  /**
   * GET /api/v1/metrics - Get current system metrics
   */
  app.get('/api/v1/metrics', (_req, res) => {
    const metrics = getSystemMetrics();
    res.json(metrics);
  });

  /**
   * GET /api/v1/metrics/snapshots - List stored metrics snapshots
   */
  app.get('/api/v1/metrics/snapshots', async (req, res) => {
    try {
      const query = req.query as MetricsQueryParams;
      const snapshots = await scopedDb(req).listSnapshots({
        metricType: query.metric_type as MetricType | undefined,
        from: query.from ? new Date(query.from) : undefined,
        to: query.to ? new Date(query.to) : undefined,
        limit: query.limit ? parseInt(String(query.limit), 10) : 100,
      });

      res.json({ snapshots, count: snapshots.length });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list snapshots', { error: message });
      res.status(500).json({ error: message });
    }
  });

  /**
   * POST /api/v1/metrics/snapshots - Create a new metrics snapshot
   */
  app.post('/api/v1/metrics/snapshots', async (req, res) => {
    try {
      const body = req.body as CreateMetricsSnapshotRequest;

      const snapshot = await scopedDb(req).createSnapshot({
        source_account_id: scopedDb(req).getCurrentSourceAccountId(),
        metric_type: body.metric_type ?? 'system',
        cpu_usage_percent: body.cpu_usage_percent ?? null,
        memory_used_bytes: body.memory_used_bytes ?? null,
        memory_total_bytes: body.memory_total_bytes ?? null,
        disk_used_bytes: body.disk_used_bytes ?? null,
        disk_total_bytes: body.disk_total_bytes ?? null,
        active_connections: body.active_connections ?? null,
        request_count: body.request_count ?? null,
        error_count: body.error_count ?? null,
        avg_response_time_ms: body.avg_response_time_ms ?? null,
        active_sessions: body.active_sessions ?? null,
        metadata: body.metadata ?? {},
      });

      // Broadcast to WebSocket clients
      broadcastToClients({
        type: 'metrics',
        data: snapshot,
        timestamp: new Date().toISOString(),
      });

      res.status(201).json(snapshot);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create snapshot', { error: message });
      res.status(500).json({ error: message });
    }
  });

  /**
   * POST /api/v1/metrics/collect - Collect and store current system metrics
   */
  app.post('/api/v1/metrics/collect', async (req, res) => {
    try {
      const metrics = getSystemMetrics();

      const snapshot = await scopedDb(req).createSnapshot({
        source_account_id: scopedDb(req).getCurrentSourceAccountId(),
        metric_type: 'system',
        cpu_usage_percent: metrics.cpu.usage_percent,
        memory_used_bytes: metrics.memory.used_bytes,
        memory_total_bytes: metrics.memory.total_bytes,
        disk_used_bytes: metrics.disk.used_bytes,
        disk_total_bytes: metrics.disk.total_bytes,
        active_connections: metrics.network.active_connections,
        request_count: null,
        error_count: null,
        avg_response_time_ms: metrics.network.avg_response_time_ms,
        active_sessions: null,
        metadata: {
          cpu: metrics.cpu,
          process: metrics.process,
        },
      });

      broadcastToClients({
        type: 'metrics',
        data: snapshot,
        timestamp: new Date().toISOString(),
      });

      res.status(201).json({ snapshot, metrics });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to collect metrics', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Sessions Endpoints
  // =========================================================================

  /**
   * GET /api/v1/sessions - Get active database sessions
   */
  app.get('/api/v1/sessions', async (req, res) => {
    try {
      const sessions = await scopedDb(req).getActiveSessions();

      broadcastToClients({
        type: 'sessions',
        data: sessions,
        timestamp: new Date().toISOString(),
      });

      res.json(sessions);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get sessions', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Storage Endpoints
  // =========================================================================

  /**
   * GET /api/v1/storage - Get storage breakdown
   */
  app.get('/api/v1/storage', async (req, res) => {
    try {
      const storage = await scopedDb(req).getStorageBreakdown();

      broadcastToClients({
        type: 'storage',
        data: storage,
        timestamp: new Date().toISOString(),
      });

      res.json(storage);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get storage breakdown', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Health Overview Endpoints
  // =========================================================================

  /**
   * GET /api/v1/health - System health overview
   */
  app.get('/api/v1/health', async (req, res) => {
    try {
      const dbHealth = await scopedDb(req).checkDatabaseHealth();

      // Check Prometheus if configured
      let prometheusHealth = null;
      if (fullConfig.prometheusUrl) {
        try {
          const start = Date.now();
          const promResponse = await fetch(`${fullConfig.prometheusUrl}/-/healthy`, {
            signal: AbortSignal.timeout(5000),
          });
          prometheusHealth = {
            status: promResponse.ok ? 'healthy' as const : 'unhealthy' as const,
            url: fullConfig.prometheusUrl,
            latency_ms: Date.now() - start,
            error: promResponse.ok ? null : `HTTP ${promResponse.status}`,
          };
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          prometheusHealth = {
            status: 'unhealthy' as const,
            url: fullConfig.prometheusUrl,
            latency_ms: null,
            error: message,
          };
        }
      }

      const overallStatus = dbHealth.status === 'healthy'
        ? (prometheusHealth && prometheusHealth.status !== 'healthy' ? 'degraded' as const : 'healthy' as const)
        : 'unhealthy' as const;

      const overview: SystemHealthOverview = {
        status: overallStatus,
        uptime_seconds: process.uptime(),
        database: {
          status: dbHealth.status === 'healthy' ? 'healthy' : 'unhealthy',
          latency_ms: dbHealth.latency_ms,
          connection_count: dbHealth.connection_count,
          max_connections: dbHealth.max_connections,
          version: dbHealth.version,
        },
        services: [
          {
            name: 'admin-api',
            status: 'healthy',
            latency_ms: null,
            last_check: new Date().toISOString(),
            error: null,
          },
        ],
        prometheus: prometheusHealth,
        timestamp: new Date().toISOString(),
      };

      broadcastToClients({
        type: 'health',
        data: overview,
        timestamp: new Date().toISOString(),
      });

      res.json(overview);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get health overview', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Dashboard Stats Endpoints
  // =========================================================================

  /**
   * GET /api/v1/stats - Dashboard statistics
   */
  app.get('/api/v1/stats', async (req, res) => {
    try {
      const stats = await scopedDb(req).getStats();
      res.json({
        plugin: 'admin-api',
        version: '1.0.0',
        stats,
        timestamp: new Date().toISOString(),
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get stats', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Dashboard Config Endpoints
  // =========================================================================

  /**
   * GET /api/v1/config - List all dashboard configurations
   */
  app.get('/api/v1/config', async (req, res) => {
    try {
      const configs = await scopedDb(req).listConfigs();
      res.json({ configs, count: configs.length });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list configs', { error: message });
      res.status(500).json({ error: message });
    }
  });

  /**
   * GET /api/v1/config/:key - Get a specific dashboard configuration
   */
  app.get('/api/v1/config/:key', async (req, res) => {
    try {
      const configEntry = await scopedDb(req).getConfig(req.params.key);
      if (!configEntry) {
        res.status(404).json({ error: 'Config not found' });
        return;
      }
      res.json(configEntry);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get config', { error: message });
      res.status(500).json({ error: message });
    }
  });

  /**
   * PUT /api/v1/config - Create or update a dashboard configuration
   */
  app.put('/api/v1/config', async (req, res) => {
    try {
      const body = req.body as UpdateDashboardConfigRequest;

      if (!body.config_key || !body.config_value) {
        res.status(400).json({ error: 'config_key and config_value are required' });
        return;
      }

      const configEntry = await scopedDb(req).upsertConfig(
        body.config_key,
        body.config_value,
        body.description
      );

      res.json(configEntry);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to upsert config', { error: message });
      res.status(500).json({ error: message });
    }
  });

  /**
   * DELETE /api/v1/config/:key - Delete a dashboard configuration
   */
  app.delete('/api/v1/config/:key', async (req, res) => {
    try {
      const deleted = await scopedDb(req).deleteConfig(req.params.key);
      if (!deleted) {
        res.status(404).json({ error: 'Config not found' });
        return;
      }
      res.json({ success: true });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to delete config', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Cleanup Endpoint
  // =========================================================================

  /**
   * POST /api/v1/metrics/cleanup - Clean up old metrics snapshots
   */
  app.post('/api/v1/metrics/cleanup', async (req, res) => {
    try {
      const retentionDays = fullConfig.metricsRetentionDays;
      const deleted = await scopedDb(req).cleanupOldSnapshots(retentionDays);

      logger.info('Metrics cleanup completed', { deleted, retentionDays });
      res.json({ deleted, retention_days: retentionDays });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to cleanup metrics', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // WebSocket Support
  // =========================================================================

  let wss: WebSocketServer | null = null;
  const wsClients = new Set<WebSocket>();

  function broadcastToClients(message: WsMessage): void {
    if (!wss || wsClients.size === 0) return;

    const data = JSON.stringify(message);
    for (const client of wsClients) {
      if (client.readyState === WebSocket.OPEN) {
        client.send(data);
      }
    }
  }

  // =========================================================================
  // Server Lifecycle
  // =========================================================================

  const httpServer = http.createServer(app);

  // Setup WebSocket if enabled
  if (fullConfig.wsEnabled) {
    wss = new WebSocketServer({ server: httpServer, path: '/ws' });

    wss.on('connection', (ws) => {
      wsClients.add(ws);
      logger.info('WebSocket client connected', { total: wsClients.size });

      ws.on('close', () => {
        wsClients.delete(ws);
        logger.info('WebSocket client disconnected', { total: wsClients.size });
      });

      ws.on('error', (error) => {
        logger.error('WebSocket error', { error: error.message });
        wsClients.delete(ws);
      });

      // Send initial metrics on connect
      const metrics = getSystemMetrics();
      ws.send(JSON.stringify({
        type: 'metrics',
        data: metrics,
        timestamp: new Date().toISOString(),
      }));
    });

    logger.info('WebSocket server enabled on /ws');
  }

  const server = {
    async start() {
      try {
        await new Promise<void>((resolve, reject) => {
          httpServer.listen(fullConfig.port, fullConfig.host, () => {
            resolve();
          });
          httpServer.on('error', reject);
        });
        logger.info(`Admin API server listening on ${fullConfig.host}:${fullConfig.port}`);
        if (fullConfig.wsEnabled) {
          logger.info(`WebSocket available at ws://${fullConfig.host}:${fullConfig.port}/ws`);
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Server failed to start', { error: message });
        throw error;
      }
    },

    async stop() {
      // Close WebSocket connections
      for (const client of wsClients) {
        client.close();
      }
      wsClients.clear();

      if (wss) {
        wss.close();
      }

      await new Promise<void>((resolve) => {
        httpServer.close(() => resolve());
      });

      await db.disconnect();
      logger.info('Server stopped');
    },
  };

  return server;
}

export async function startServer(config?: Partial<Config>): Promise<void> {
  const server = await createServer(config);
  await server.start();

  process.on('SIGTERM', async () => {
    logger.info('SIGTERM received, shutting down gracefully');
    await server.stop();
    process.exit(0);
  });

  process.on('SIGINT', async () => {
    logger.info('SIGINT received, shutting down gracefully');
    await server.stop();
    process.exit(0);
  });
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
