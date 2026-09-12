/**
 * Minimal Prometheus-text-format request/error counters for nself plugins.
 *
 * createMetrics(name) returns a per-plugin counter pair exposed at a
 * plugin's `/metrics` route (see free/file-processing/ts/src/server.ts and
 * free/media-processing/ts/src/server.ts, which both hook Fastify's
 * onRequest/onError and serve `metrics.format()` as `text/plain;
 * version=0.0.4`). Deliberately dependency-free (no prom-client) since a
 * plugin only needs two monotonic counters, not full histogram/summary
 * support.
 */

export interface PluginMetrics {
  /** Increment the request counter. Call once per inbound request. */
  incrementRequest: () => void;
  /** Increment the error counter. Call once per request that errors. */
  incrementError: () => void;
  /** Render both counters in Prometheus text exposition format. */
  format: () => string;
}

export function createMetrics(name: string): PluginMetrics {
  const metricName = name.replace(/[^a-zA-Z0-9_]/g, '_');
  let requestCount = 0;
  let errorCount = 0;

  return {
    incrementRequest(): void {
      requestCount += 1;
    },
    incrementError(): void {
      errorCount += 1;
    },
    format(): string {
      return [
        `# HELP ${metricName}_requests_total Total number of requests handled.`,
        `# TYPE ${metricName}_requests_total counter`,
        `${metricName}_requests_total ${requestCount}`,
        `# HELP ${metricName}_errors_total Total number of requests that errored.`,
        `# TYPE ${metricName}_errors_total counter`,
        `${metricName}_errors_total ${errorCount}`,
        '',
      ].join('\n');
    },
  };
}
