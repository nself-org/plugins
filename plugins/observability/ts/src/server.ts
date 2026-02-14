/**
 * Observability Plugin Server
 * HTTP server for metrics, logging, and tracing endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook } from '@nself/plugin-utils';
import { loadConfig, type Config } from './config.js';
import { MetricsCollector } from './metrics.js';
import { LoggingService } from './logging.js';
import { TracingService } from './tracing.js';
import type { IngestLogRequest, IngestTraceRequest, Dashboard } from './types.js';

const logger = createLogger('observability:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024,
  });

  await app.register(cors, { origin: true, credentials: true });

  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 1000,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );

  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Initialize services
  const metrics = new MetricsCollector();
  const logging = new LoggingService(fullConfig.lokiUrl, fullConfig.lokiEnabled);
  const tracing = new TracingService(fullConfig.tempoUrl, fullConfig.tempoEnabled);

  // =========================================================================
  // Health Checks
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'observability', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async () => {
    return {
      ready: true,
      plugin: 'observability',
      prometheus_enabled: fullConfig.prometheusEnabled,
      loki_enabled: fullConfig.lokiEnabled,
      tempo_enabled: fullConfig.tempoEnabled,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Prometheus Metrics
  // =========================================================================

  app.get(fullConfig.metricsPath, async (_request, reply) => {
    const metricsText = await metrics.getMetrics();
    return reply.type('text/plain').send(metricsText);
  });

  // =========================================================================
  // Logging Endpoints
  // =========================================================================

  app.post<{ Body: IngestLogRequest }>('/api/observability/logs', async (request, reply) => {
    try {
      const { level, message } = request.body;
      if (!level || !message) {
        return reply.status(400).send({ error: 'level and message are required' });
      }

      await logging.ingestLog(request.body);

      return reply.status(201).send({ success: true, timestamp: new Date().toISOString() });
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to ingest log', { error: errorMessage });
      return reply.status(500).send({ error: errorMessage });
    }
  });

  app.get<{
    Querystring: {
      query: string;
      start_time?: string;
      end_time?: string;
      limit?: string;
    };
  }>('/api/observability/logs', async (request, reply) => {
    try {
      const { query, start_time, end_time, limit } = request.query;
      if (!query) {
        return reply.status(400).send({ error: 'query parameter is required' });
      }

      const logs = await logging.queryLogs(
        query,
        start_time,
        end_time,
        limit ? parseInt(limit, 10) : undefined
      );

      return { logs, count: logs.length };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to query logs', { error: errorMessage });
      return reply.status(500).send({ error: errorMessage });
    }
  });

  // =========================================================================
  // Tracing Endpoints
  // =========================================================================

  app.post<{ Body: IngestTraceRequest }>('/api/observability/traces', async (request, reply) => {
    try {
      const { trace_id, span_id, operation_name, start_time, end_time, duration_ms } = request.body;
      if (!trace_id || !span_id || !operation_name || !start_time || !end_time || duration_ms === undefined) {
        return reply.status(400).send({
          error: 'trace_id, span_id, operation_name, start_time, end_time, and duration_ms are required',
        });
      }

      await tracing.ingestTrace(request.body);

      return reply.status(201).send({ success: true, timestamp: new Date().toISOString() });
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to ingest trace', { error: errorMessage });
      return reply.status(500).send({ error: errorMessage });
    }
  });

  app.get<{
    Querystring: {
      trace_id?: string;
      service?: string;
      operation?: string;
      start_time?: string;
      end_time?: string;
      limit?: string;
    };
  }>('/api/observability/traces', async (request, reply) => {
    try {
      const { trace_id, service, operation, start_time, end_time, limit } = request.query;

      const traces = await tracing.queryTraces(
        trace_id,
        service,
        operation,
        start_time,
        end_time,
        limit ? parseInt(limit, 10) : undefined
      );

      return { traces, count: traces.length };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to query traces', { error: errorMessage });
      return reply.status(500).send({ error: errorMessage });
    }
  });

  // =========================================================================
  // Grafana Dashboards
  // =========================================================================

  app.get('/api/observability/dashboards', async (_request, reply) => {
    try {
      if (!fullConfig.grafanaApiKey) {
        return reply.status(503).send({ error: 'Grafana API key not configured' });
      }

      const response = await fetch(`${fullConfig.grafanaUrl}/api/search?type=dash-db`, {
        headers: {
          Authorization: `Bearer ${fullConfig.grafanaApiKey}`,
        },
      });

      if (!response.ok) {
        throw new Error(`Grafana API error: ${response.statusText}`);
      }

      const dashboards = (await response.json()) as Array<{
        id?: number;
        uid?: string;
        title?: string;
        tags?: string[];
      }>;

      return {
        dashboards: dashboards.map((d) => ({
          id: d.uid ?? String(d.id),
          title: d.title ?? 'Untitled',
          tags: d.tags ?? [],
        })),
        count: dashboards.length,
      };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to fetch dashboards', { error: errorMessage });
      return reply.status(500).send({ error: errorMessage });
    }
  });

  app.get<{ Params: { id: string } }>('/api/observability/dashboards/:id', async (request, reply) => {
    try {
      if (!fullConfig.grafanaApiKey) {
        return reply.status(503).send({ error: 'Grafana API key not configured' });
      }

      const response = await fetch(`${fullConfig.grafanaUrl}/api/dashboards/uid/${request.params.id}`, {
        headers: {
          Authorization: `Bearer ${fullConfig.grafanaApiKey}`,
        },
      });

      if (!response.ok) {
        if (response.status === 404) {
          return reply.status(404).send({ error: 'Dashboard not found' });
        }
        throw new Error(`Grafana API error: ${response.statusText}`);
      }

      const data = (await response.json()) as {
        dashboard?: {
          id?: number;
          uid?: string;
          title?: string;
          tags?: string[];
        };
      };

      if (!data.dashboard) {
        return reply.status(404).send({ error: 'Dashboard not found' });
      }

      const dashboard: Dashboard = {
        id: data.dashboard.uid ?? String(data.dashboard.id),
        title: data.dashboard.title ?? 'Untitled',
        tags: data.dashboard.tags,
        json: data.dashboard,
      };

      return dashboard;
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to fetch dashboard', { error: errorMessage });
      return reply.status(500).send({ error: errorMessage });
    }
  });

  // =========================================================================
  // Metrics Recording Endpoints (for custom metrics)
  // =========================================================================

  app.post<{
    Body: {
      metric: string;
      value: number;
      labels?: Record<string, string>;
      type: 'counter' | 'histogram' | 'gauge';
    };
  }>('/api/observability/metrics/record', async (request, reply) => {
    try {
      const { metric, value, labels, type } = request.body;
      if (!metric || value === undefined || !type) {
        return reply.status(400).send({ error: 'metric, value, and type are required' });
      }

      switch (type) {
        case 'counter':
          metrics.incrementCounter(metric, labels);
          break;
        case 'histogram':
          metrics.recordHistogram(metric, value, labels);
          break;
        case 'gauge':
          metrics.setGauge(metric, value, labels);
          break;
        default:
          return reply.status(400).send({ error: 'type must be counter, histogram, or gauge' });
      }

      return { success: true };
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to record metric', { error: errorMessage });
      return reply.status(500).send({ error: errorMessage });
    }
  });

  return { app, config: fullConfig, metrics, logging, tracing };
}

export async function startServer(config?: Partial<Config>) {
  const { app, config: fullConfig } = await createServer(config);

  await app.listen({ port: fullConfig.port, host: fullConfig.host });
  logger.info(`Observability server listening on ${fullConfig.host}:${fullConfig.port}`);
  logger.info(`Metrics available at: ${fullConfig.metricsPath}`);

  return app;
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
