/**
 * Prometheus Metrics Collection
 */

import { Registry, Counter, Histogram, Gauge, collectDefaultMetrics } from 'prom-client';
import type { MetricLabels } from './types.js';

export class MetricsCollector {
  private registry: Registry;

  // HTTP metrics
  private httpRequestsTotal: Counter;
  private httpRequestDuration: Histogram;
  private httpRequestSize: Histogram;
  private httpResponseSize: Histogram;

  // Database metrics
  private dbQueriesTotal: Counter;
  private dbQueryDuration: Histogram;
  private dbConnectionsActive: Gauge;
  private dbConnectionsIdle: Gauge;

  // Queue metrics
  private queueSize: Gauge;
  private queueJobsProcessedTotal: Counter;
  private queueJobDuration: Histogram;
  private queueJobsFailedTotal: Counter;

  // Business metrics
  private videosUploadedTotal: Counter;
  private streamsStartedTotal: Counter;
  private usersActive: Gauge;
  private errorsTotal: Counter;

  constructor() {
    this.registry = new Registry();

    // Collect default Node.js metrics (CPU, memory, etc.)
    collectDefaultMetrics({ register: this.registry });

    // HTTP metrics
    this.httpRequestsTotal = new Counter({
      name: 'http_requests_total',
      help: 'Total number of HTTP requests',
      labelNames: ['method', 'endpoint', 'status'],
      registers: [this.registry],
    });

    this.httpRequestDuration = new Histogram({
      name: 'http_request_duration_seconds',
      help: 'HTTP request duration in seconds',
      labelNames: ['method', 'endpoint'],
      buckets: [0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10],
      registers: [this.registry],
    });

    this.httpRequestSize = new Histogram({
      name: 'http_request_size_bytes',
      help: 'HTTP request size in bytes',
      labelNames: ['method', 'endpoint'],
      buckets: [100, 1000, 10000, 100000, 1000000],
      registers: [this.registry],
    });

    this.httpResponseSize = new Histogram({
      name: 'http_response_size_bytes',
      help: 'HTTP response size in bytes',
      labelNames: ['method', 'endpoint'],
      buckets: [100, 1000, 10000, 100000, 1000000],
      registers: [this.registry],
    });

    // Database metrics
    this.dbQueriesTotal = new Counter({
      name: 'db_queries_total',
      help: 'Total number of database queries',
      labelNames: ['operation', 'table'],
      registers: [this.registry],
    });

    this.dbQueryDuration = new Histogram({
      name: 'db_query_duration_seconds',
      help: 'Database query duration in seconds',
      labelNames: ['operation', 'table'],
      buckets: [0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5],
      registers: [this.registry],
    });

    this.dbConnectionsActive = new Gauge({
      name: 'db_connections_active',
      help: 'Number of active database connections',
      registers: [this.registry],
    });

    this.dbConnectionsIdle = new Gauge({
      name: 'db_connections_idle',
      help: 'Number of idle database connections',
      registers: [this.registry],
    });

    // Queue metrics
    this.queueSize = new Gauge({
      name: 'queue_size',
      help: 'Current queue size',
      labelNames: ['queue_name'],
      registers: [this.registry],
    });

    this.queueJobsProcessedTotal = new Counter({
      name: 'queue_jobs_processed_total',
      help: 'Total number of jobs processed',
      labelNames: ['queue_name', 'status'],
      registers: [this.registry],
    });

    this.queueJobDuration = new Histogram({
      name: 'queue_job_duration_seconds',
      help: 'Job processing duration in seconds',
      labelNames: ['queue_name'],
      buckets: [0.1, 0.5, 1, 5, 10, 30, 60, 300],
      registers: [this.registry],
    });

    this.queueJobsFailedTotal = new Counter({
      name: 'queue_jobs_failed_total',
      help: 'Total number of failed jobs',
      labelNames: ['queue_name', 'error_type'],
      registers: [this.registry],
    });

    // Business metrics
    this.videosUploadedTotal = new Counter({
      name: 'videos_uploaded_total',
      help: 'Total number of videos uploaded',
      labelNames: ['user_id', 'source_account_id'],
      registers: [this.registry],
    });

    this.streamsStartedTotal = new Counter({
      name: 'streams_started_total',
      help: 'Total number of streams started',
      labelNames: ['user_id', 'source_account_id'],
      registers: [this.registry],
    });

    this.usersActive = new Gauge({
      name: 'users_active',
      help: 'Number of active users',
      labelNames: ['source_account_id'],
      registers: [this.registry],
    });

    this.errorsTotal = new Counter({
      name: 'errors_total',
      help: 'Total number of errors',
      labelNames: ['error_type', 'service'],
      registers: [this.registry],
    });
  }

  // HTTP metric methods
  recordHttpRequest(method: string, endpoint: string, status: number): void {
    this.httpRequestsTotal.inc({ method, endpoint, status: status.toString() });
  }

  recordHttpDuration(method: string, endpoint: string, durationSeconds: number): void {
    this.httpRequestDuration.observe({ method, endpoint }, durationSeconds);
  }

  recordHttpRequestSize(method: string, endpoint: string, sizeBytes: number): void {
    this.httpRequestSize.observe({ method, endpoint }, sizeBytes);
  }

  recordHttpResponseSize(method: string, endpoint: string, sizeBytes: number): void {
    this.httpResponseSize.observe({ method, endpoint }, sizeBytes);
  }

  // Database metric methods
  recordDbQuery(operation: string, table: string): void {
    this.dbQueriesTotal.inc({ operation, table });
  }

  recordDbQueryDuration(operation: string, table: string, durationSeconds: number): void {
    this.dbQueryDuration.observe({ operation, table }, durationSeconds);
  }

  setDbConnectionsActive(count: number): void {
    this.dbConnectionsActive.set(count);
  }

  setDbConnectionsIdle(count: number): void {
    this.dbConnectionsIdle.set(count);
  }

  // Queue metric methods
  setQueueSize(queueName: string, size: number): void {
    this.queueSize.set({ queue_name: queueName }, size);
  }

  recordQueueJobProcessed(queueName: string, status: 'success' | 'failure'): void {
    this.queueJobsProcessedTotal.inc({ queue_name: queueName, status });
  }

  recordQueueJobDuration(queueName: string, durationSeconds: number): void {
    this.queueJobDuration.observe({ queue_name: queueName }, durationSeconds);
  }

  recordQueueJobFailed(queueName: string, errorType: string): void {
    this.queueJobsFailedTotal.inc({ queue_name: queueName, error_type: errorType });
  }

  // Business metric methods
  recordVideoUploaded(userId: string, sourceAccountId: string): void {
    this.videosUploadedTotal.inc({ user_id: userId, source_account_id: sourceAccountId });
  }

  recordStreamStarted(userId: string, sourceAccountId: string): void {
    this.streamsStartedTotal.inc({ user_id: userId, source_account_id: sourceAccountId });
  }

  setActiveUsers(sourceAccountId: string, count: number): void {
    this.usersActive.set({ source_account_id: sourceAccountId }, count);
  }

  recordError(errorType: string, service: string): void {
    this.errorsTotal.inc({ error_type: errorType, service });
  }

  // Generic methods
  incrementCounter(name: string, labels?: MetricLabels): void {
    const metric = this.registry.getSingleMetric(name);
    if (metric && 'inc' in metric) {
      if (labels) {
        (metric as Counter).inc(labels);
      } else {
        (metric as Counter).inc();
      }
    }
  }

  recordHistogram(name: string, value: number, labels?: MetricLabels): void {
    const metric = this.registry.getSingleMetric(name);
    if (metric && 'observe' in metric) {
      (metric as Histogram).observe(labels ?? {}, value);
    }
  }

  setGauge(name: string, value: number, labels?: MetricLabels): void {
    const metric = this.registry.getSingleMetric(name);
    if (metric && 'set' in metric) {
      (metric as Gauge).set(labels ?? {}, value);
    }
  }

  async getMetrics(): Promise<string> {
    return this.registry.metrics();
  }

  getRegistry(): Registry {
    return this.registry;
  }
}
