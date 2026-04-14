/**
 * Client API latency wrapper for nSelf web apps.
 * Wraps fetch to record client_api_duration_seconds and client_api_errors_total,
 * batched via Beacon every 30s to /api/telemetry/client.
 *
 * Usage:
 *   import { createApiClient } from '@nself/monitoring/api-client';
 *   const api = createApiClient({ app: 'claw' });
 *   const res = await api.fetch('/api/chat', { method: 'POST', body: ... });
 */

interface ApiClientConfig {
  /** App identifier. */
  app: string;
  /** Telemetry endpoint (default: /api/telemetry/client). */
  endpoint?: string;
  /** Base URL for API calls (default: ''). */
  baseUrl?: string;
}

interface LatencyEntry {
  endpoint: string;
  status: number;
  duration_ms: number;
  app: string;
  timestamp: number;
}

interface ErrorEntry {
  endpoint: string;
  kind: string;
  app: string;
  timestamp: number;
}

interface TelemetryBatch {
  latencies: LatencyEntry[];
  errors: ErrorEntry[];
}

const latencyBuffer: LatencyEntry[] = [];
const errorBuffer: ErrorEntry[] = [];
let clientConfig: ApiClientConfig;
let flushTimer: ReturnType<typeof setInterval> | null = null;

function flushClientMetrics(): void {
  if (latencyBuffer.length === 0 && errorBuffer.length === 0) return;

  const endpoint = clientConfig.endpoint || "/api/telemetry/client";
  const batch: TelemetryBatch = {
    latencies: latencyBuffer.splice(0),
    errors: errorBuffer.splice(0),
  };

  const payload = JSON.stringify(batch);
  if (navigator.sendBeacon) {
    navigator.sendBeacon(endpoint, payload);
  } else {
    fetch(endpoint, {
      method: "POST",
      body: payload,
      headers: { "Content-Type": "application/json" },
      keepalive: true,
    }).catch(() => {});
  }
}

/**
 * Create an instrumented API client that records latency and errors.
 */
export function createApiClient(cfg: ApiClientConfig) {
  clientConfig = cfg;

  // Start periodic flush.
  if (!flushTimer) {
    flushTimer = setInterval(flushClientMetrics, 30000);
    document.addEventListener("visibilitychange", () => {
      if (document.visibilityState === "hidden") {
        flushClientMetrics();
      }
    });
  }

  return {
    async fetch(
      url: string,
      init?: RequestInit
    ): Promise<Response> {
      const fullUrl = (cfg.baseUrl || "") + url;
      const start = performance.now();

      try {
        const res = await globalThis.fetch(fullUrl, init);
        const duration = performance.now() - start;

        latencyBuffer.push({
          endpoint: url,
          status: res.status,
          duration_ms: Math.round(duration),
          app: cfg.app,
          timestamp: Date.now(),
        });

        return res;
      } catch (err) {
        const duration = performance.now() - start;

        latencyBuffer.push({
          endpoint: url,
          status: 0,
          duration_ms: Math.round(duration),
          app: cfg.app,
          timestamp: Date.now(),
        });

        errorBuffer.push({
          endpoint: url,
          kind:
            err instanceof TypeError ? "network" : "unknown",
          app: cfg.app,
          timestamp: Date.now(),
        });

        throw err;
      }
    },
  };
}
