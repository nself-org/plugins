/**
 * Observability Plugin Server
 * Express server for health probes, watchdog, and service discovery API endpoints
 */

import express from 'express';
import type { Request, Response, NextFunction } from 'express';
import Docker from 'dockerode';
import { createLogger } from '@nself/plugin-utils';
import { ObservabilityDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  RegisterServiceRequest,
  UpdateServiceRequest,
  ListServicesQuery,
  ListHealthHistoryQuery,
  ListEventsQuery,
  HealthResult,
  WatchdogStatus,
  ServiceState,
  HealthStatus,
} from './types.js';

const logger = createLogger('observability:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new ObservabilityDatabase();
  await db.connect();
  await db.initializeSchema();

  // Initialize Docker client (optional)
  let docker: Docker | null = null;
  if (fullConfig.dockerEnabled) {
    try {
      docker = new Docker({ socketPath: fullConfig.dockerSocket });
      await docker.ping();
      logger.info('Docker connection established', { socket: fullConfig.dockerSocket });
    } catch {
      logger.warn('Docker not available, container discovery disabled', { socket: fullConfig.dockerSocket });
      docker = null;
    }
  }

  // Create Express app
  const app = express();
  app.use(express.json({ limit: '10mb' }));

  // CORS middleware
  app.use((_req: Request, res: Response, next: NextFunction) => {
    res.header('Access-Control-Allow-Origin', '*');
    res.header('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
    res.header('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-Source-Account-Id');
    if (_req.method === 'OPTIONS') {
      res.sendStatus(204);
      return;
    }
    next();
  });

  // Multi-app context middleware
  function getScopedDb(req: Request): ObservabilityDatabase {
    const sourceAccountId = (req.headers['x-source-account-id'] as string) ?? 'primary';
    return db.forSourceAccount(sourceAccountId);
  }

  // Watchdog state
  let watchdogInterval: ReturnType<typeof setInterval> | null = null;
  let watchdogStartTime: Date | null = null;
  let lastCheckTime: Date | null = null;

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', (_req: Request, res: Response) => {
    res.json({ status: 'ok', plugin: 'observability', timestamp: new Date().toISOString() });
  });

  app.get('/ready', async (_req: Request, res: Response) => {
    try {
      await db.query('SELECT 1');
      res.json({ ready: true, plugin: 'observability', timestamp: new Date().toISOString() });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      res.status(503).json({
        ready: false,
        plugin: 'observability',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  // =========================================================================
  // Service Endpoints
  // =========================================================================

  app.post('/api/v1/services', async (req: Request, res: Response) => {
    try {
      const body = req.body as RegisterServiceRequest;
      if (!body.name || !body.host) {
        res.status(400).json({ error: 'name and host are required' });
        return;
      }

      const scopedDb = getScopedDb(req);
      const service = await scopedDb.registerService({
        source_account_id: scopedDb.getCurrentSourceAccountId(),
        name: body.name,
        container_id: body.container_id ?? null,
        container_name: body.container_name ?? null,
        image: body.image ?? null,
        service_type: body.service_type ?? 'manual',
        host: body.host,
        port: body.port ?? null,
        health_endpoint: body.health_endpoint ?? null,
        state: 'discovered' as ServiceState,
        last_health_check: null,
        last_healthy: null,
        consecutive_failures: 0,
        metadata: {},
      });

      await scopedDb.createWatchdogEvent({
        serviceId: service.id,
        eventType: 'service_discovered',
        message: `Service "${service.name}" registered at ${service.host}:${service.port ?? 'n/a'}`,
        severity: 'info',
      });

      res.status(201).json(service);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to register service', { error: message });
      res.status(500).json({ error: message });
    }
  });

  app.get('/api/v1/services', async (req: Request, res: Response) => {
    try {
      const query = req.query as unknown as ListServicesQuery;
      const scopedDb = getScopedDb(req);
      const services = await scopedDb.listServices({
        state: query.state as ServiceState | undefined,
        serviceType: query.service_type,
        limit: query.limit ? parseInt(String(query.limit), 10) : 200,
        offset: query.offset ? parseInt(String(query.offset), 10) : undefined,
      });

      res.json({ services, count: services.length });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list services', { error: message });
      res.status(500).json({ error: message });
    }
  });

  app.get('/api/v1/services/:id', async (req: Request, res: Response) => {
    try {
      const scopedDb = getScopedDb(req);
      const service = await scopedDb.getService(req.params.id);
      if (!service) {
        res.status(404).json({ error: 'Service not found' });
        return;
      }
      res.json(service);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get service', { error: message });
      res.status(500).json({ error: message });
    }
  });

  app.put('/api/v1/services/:id', async (req: Request, res: Response) => {
    try {
      const body = req.body as UpdateServiceRequest;
      const scopedDb = getScopedDb(req);
      const service = await scopedDb.updateService(req.params.id, body as Partial<typeof service & Record<string, unknown>>);
      if (!service) {
        res.status(404).json({ error: 'Service not found' });
        return;
      }
      res.json(service);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to update service', { error: message });
      res.status(500).json({ error: message });
    }
  });

  app.delete('/api/v1/services/:id', async (req: Request, res: Response) => {
    try {
      const scopedDb = getScopedDb(req);
      const deleted = await scopedDb.deleteService(req.params.id);
      if (!deleted) {
        res.status(404).json({ error: 'Service not found' });
        return;
      }
      res.json({ success: true });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to delete service', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Docker Discovery Endpoint
  // =========================================================================

  app.post('/api/v1/services/discover', async (req: Request, res: Response) => {
    try {
      if (!docker) {
        res.status(503).json({ error: 'Docker not available' });
        return;
      }

      const scopedDb = getScopedDb(req);
      const containers = await docker.listContainers({ all: false });
      let discovered = 0;

      for (const container of containers) {
        const name = container.Names?.[0]?.replace(/^\//, '') ?? container.Id.substring(0, 12);
        const ports = container.Ports ?? [];
        const primaryPort = ports.find(p => p.PublicPort) ?? ports[0];

        await scopedDb.registerService({
          source_account_id: scopedDb.getCurrentSourceAccountId(),
          name,
          container_id: container.Id.substring(0, 12),
          container_name: name,
          image: container.Image ?? null,
          service_type: 'docker',
          host: '127.0.0.1',
          port: primaryPort?.PublicPort ?? primaryPort?.PrivatePort ?? null,
          health_endpoint: null,
          state: container.State === 'running' ? 'discovered' : 'unknown',
          last_health_check: null,
          last_healthy: null,
          consecutive_failures: 0,
          metadata: {
            labels: container.Labels ?? {},
            status: container.Status ?? '',
          },
        });
        discovered++;
      }

      await scopedDb.createWatchdogEvent({
        eventType: 'service_discovered',
        message: `Docker discovery found ${discovered} container(s)`,
        severity: 'info',
        metadata: { container_count: discovered },
      });

      res.json({ discovered, timestamp: new Date().toISOString() });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Docker discovery failed', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.post('/api/v1/services/:id/check', async (req: Request, res: Response) => {
    try {
      const scopedDb = getScopedDb(req);
      const service = await scopedDb.getService(req.params.id);
      if (!service) {
        res.status(404).json({ error: 'Service not found' });
        return;
      }

      const result = await performHealthCheck(scopedDb, service.id, service.name,
        service.host, service.port, service.health_endpoint);

      res.json(result);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Health check failed', { error: message });
      res.status(500).json({ error: message });
    }
  });

  app.post('/api/v1/health/check-all', async (req: Request, res: Response) => {
    try {
      const scopedDb = getScopedDb(req);
      const services = await scopedDb.listServices({ limit: 500 });
      const results: HealthResult[] = [];

      for (const service of services) {
        if (service.state === 'removed') continue;
        const result = await performHealthCheck(scopedDb, service.id, service.name,
          service.host, service.port, service.health_endpoint);
        results.push(result);
      }

      res.json({ results, count: results.length, timestamp: new Date().toISOString() });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Check all failed', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Health History Endpoints
  // =========================================================================

  app.get('/api/v1/health/history', async (req: Request, res: Response) => {
    try {
      const query = req.query as unknown as ListHealthHistoryQuery;
      const scopedDb = getScopedDb(req);
      const history = await scopedDb.listHealthHistory({
        serviceId: query.service_id,
        status: query.status as HealthStatus | undefined,
        from: query.from ? new Date(query.from) : undefined,
        to: query.to ? new Date(query.to) : undefined,
        limit: query.limit ? parseInt(String(query.limit), 10) : 100,
      });

      res.json({ history, count: history.length });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list health history', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Watchdog Endpoints
  // =========================================================================

  app.get('/api/v1/watchdog', async (req: Request, res: Response) => {
    try {
      const scopedDb = getScopedDb(req);
      const services = await scopedDb.listServices({ limit: 10000 });
      const monitoredCount = services.filter(s => s.state !== 'removed').length;

      const status: WatchdogStatus = {
        enabled: fullConfig.watchdogEnabled,
        running: watchdogInterval !== null,
        check_interval_seconds: fullConfig.checkIntervalSeconds,
        timeout_seconds: fullConfig.watchdogTimeoutSeconds,
        services_monitored: monitoredCount,
        last_check: lastCheckTime,
        uptime_seconds: watchdogStartTime
          ? Math.floor((Date.now() - watchdogStartTime.getTime()) / 1000)
          : 0,
      };

      res.json(status);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get watchdog status', { error: message });
      res.status(500).json({ error: message });
    }
  });

  app.post('/api/v1/watchdog/start', async (_req: Request, res: Response) => {
    try {
      if (watchdogInterval) {
        res.status(409).json({ error: 'Watchdog already running' });
        return;
      }

      startWatchdog();
      res.json({ message: 'Watchdog started', interval_seconds: fullConfig.checkIntervalSeconds });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start watchdog', { error: message });
      res.status(500).json({ error: message });
    }
  });

  app.post('/api/v1/watchdog/stop', async (_req: Request, res: Response) => {
    try {
      if (!watchdogInterval) {
        res.status(409).json({ error: 'Watchdog not running' });
        return;
      }

      stopWatchdog();
      res.json({ message: 'Watchdog stopped' });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to stop watchdog', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Events Endpoints
  // =========================================================================

  app.get('/api/v1/events', async (req: Request, res: Response) => {
    try {
      const query = req.query as unknown as ListEventsQuery;
      const scopedDb = getScopedDb(req);
      const events = await scopedDb.listWatchdogEvents({
        serviceId: query.service_id,
        eventType: query.event_type as import('./types.js').WatchdogEventType | undefined,
        severity: query.severity,
        from: query.from ? new Date(query.from) : undefined,
        to: query.to ? new Date(query.to) : undefined,
        limit: query.limit ? parseInt(String(query.limit), 10) : 100,
      });

      res.json({ events, count: events.length });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list events', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/v1/stats', async (req: Request, res: Response) => {
    try {
      const scopedDb = getScopedDb(req);
      const stats = await scopedDb.getStats();
      res.json({
        plugin: 'observability',
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
  // Health Check Logic
  // =========================================================================

  async function performHealthCheck(
    scopedDb: ObservabilityDatabase,
    serviceId: string,
    serviceName: string,
    host: string,
    port: number | null,
    healthEndpoint: string | null,
  ): Promise<HealthResult> {
    const startMs = Date.now();
    let status: HealthStatus = 'healthy';
    let statusCode: number | null = null;
    let errorMessage: string | null = null;

    const endpoint = healthEndpoint ?? '/health';
    const baseUrl = port ? `http://${host}:${port}` : `http://${host}`;
    const url = `${baseUrl}${endpoint}`;

    try {
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), fullConfig.watchdogTimeoutSeconds * 1000);

      const response = await fetch(url, {
        signal: controller.signal,
        headers: { 'Accept': 'application/json' },
      });

      clearTimeout(timeout);
      statusCode = response.status;

      if (!response.ok) {
        status = response.status >= 500 ? 'unhealthy' : 'degraded';
        errorMessage = `HTTP ${response.status}`;
      }
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        status = 'timeout';
        errorMessage = `Health check timed out after ${fullConfig.watchdogTimeoutSeconds}s`;
      } else {
        status = 'error';
        errorMessage = error instanceof Error ? error.message : 'Unknown error';
      }
    }

    const responseTimeMs = Date.now() - startMs;

    // Record health check result
    await scopedDb.recordHealthCheck({
      serviceId,
      status,
      responseTimeMs,
      statusCode,
      errorMessage,
    });

    // Update service state
    const service = await scopedDb.getService(serviceId);
    const previousState = service?.state;

    if (status === 'healthy') {
      await scopedDb.updateServiceState(serviceId, 'healthy');
    } else if (status === 'degraded') {
      await scopedDb.updateServiceState(serviceId, 'degraded');
    } else {
      const failures = (service?.consecutive_failures ?? 0) + 1;
      await scopedDb.updateServiceState(serviceId, 'unhealthy', failures);
    }

    // Create watchdog events for state changes
    if (previousState && previousState !== 'removed') {
      const newState = status === 'healthy' ? 'healthy' : status === 'degraded' ? 'degraded' : 'unhealthy';
      if (previousState !== newState) {
        if (newState === 'healthy' && previousState === 'unhealthy') {
          await scopedDb.createWatchdogEvent({
            serviceId,
            eventType: 'health_check_recovered',
            message: `Service "${serviceName}" recovered (was ${previousState})`,
            severity: 'info',
            metadata: { previous_state: previousState, response_time_ms: responseTimeMs },
          });
        } else if (newState === 'unhealthy') {
          await scopedDb.createWatchdogEvent({
            serviceId,
            eventType: 'health_check_failed',
            message: `Service "${serviceName}" health check failed: ${errorMessage ?? status}`,
            severity: 'warning',
            metadata: { status, status_code: statusCode, error: errorMessage, response_time_ms: responseTimeMs },
          });
        } else {
          await scopedDb.createWatchdogEvent({
            serviceId,
            eventType: 'service_state_changed',
            message: `Service "${serviceName}" state changed: ${previousState} -> ${newState}`,
            severity: 'info',
            metadata: { previous_state: previousState, new_state: newState, response_time_ms: responseTimeMs },
          });
        }
      }
    }

    return {
      service_id: serviceId,
      service_name: serviceName,
      status,
      response_time_ms: responseTimeMs,
      status_code: statusCode,
      error_message: errorMessage,
      checked_at: new Date(),
    };
  }

  // =========================================================================
  // Watchdog Loop
  // =========================================================================

  async function watchdogLoop(): Promise<void> {
    try {
      lastCheckTime = new Date();
      const services = await db.listServices({ limit: 10000 });

      for (const service of services) {
        if (service.state === 'removed') continue;

        try {
          await performHealthCheck(
            db, service.id, service.name,
            service.host, service.port, service.health_endpoint,
          );
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          logger.error(`Watchdog check failed for service "${service.name}"`, { error: message });
        }
      }

      // Cleanup old health history
      const cleaned = await db.cleanupOldHealthHistory(fullConfig.healthHistoryRetainDays);
      if (cleaned > 0) {
        logger.info(`Cleaned up ${cleaned} old health history records`);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Watchdog loop error', { error: message });
    }
  }

  function startWatchdog(): void {
    if (watchdogInterval) return;

    watchdogStartTime = new Date();
    watchdogInterval = setInterval(
      () => { watchdogLoop().catch(() => {}); },
      fullConfig.checkIntervalSeconds * 1000,
    );

    db.createWatchdogEvent({
      eventType: 'watchdog_started',
      message: `Watchdog started with ${fullConfig.checkIntervalSeconds}s interval`,
      severity: 'info',
      metadata: { interval_seconds: fullConfig.checkIntervalSeconds },
    }).catch(() => {});

    logger.info(`Watchdog started (interval: ${fullConfig.checkIntervalSeconds}s)`);

    // Run initial check
    watchdogLoop().catch(() => {});
  }

  function stopWatchdog(): void {
    if (!watchdogInterval) return;

    clearInterval(watchdogInterval);
    watchdogInterval = null;
    watchdogStartTime = null;

    db.createWatchdogEvent({
      eventType: 'watchdog_stopped',
      message: 'Watchdog stopped',
      severity: 'info',
    }).catch(() => {});

    logger.info('Watchdog stopped');
  }

  // =========================================================================
  // Server Lifecycle
  // =========================================================================

  const server = {
    app,
    config: fullConfig,

    async start() {
      return new Promise<void>((resolve, reject) => {
        try {
          const httpServer = app.listen(fullConfig.port, fullConfig.host, () => {
            logger.info(`Observability server listening on ${fullConfig.host}:${fullConfig.port}`);

            // Auto-start watchdog if enabled
            if (fullConfig.watchdogEnabled) {
              startWatchdog();
            }

            resolve();
          });

          httpServer.on('error', reject);
        } catch (error) {
          reject(error);
        }
      });
    },

    async stop() {
      stopWatchdog();
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
