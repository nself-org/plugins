/**
 * Web Vitals collector for nSelf web apps.
 * Collects LCP, FID, CLS, TTFB, INP and sends via Beacon API
 * to POST /api/telemetry/vitals on ping_api.
 *
 * Usage in any web/ app:
 *   import { initVitals } from '@nself/monitoring/vitals';
 *   initVitals({ app: 'claw', endpoint: '/api/telemetry/vitals' });
 */

import { onLCP, onFID, onCLS, onTTFB, onINP, type Metric } from "web-vitals";

interface VitalsConfig {
  /** App identifier: 'claw', 'chat', 'org', 'docs', 'task'. */
  app: string;
  /** Telemetry endpoint (default: /api/telemetry/vitals). */
  endpoint?: string;
  /** Device type override. Auto-detected if omitted. */
  device?: "mobile" | "desktop" | "tablet";
}

interface VitalEntry {
  name: string;
  value: number;
  route: string;
  device: string;
  app: string;
  timestamp: number;
}

const buffer: VitalEntry[] = [];
let config: VitalsConfig;

function getDevice(): string {
  if (config?.device) return config.device;
  const ua = navigator.userAgent;
  if (/Mobi|Android/i.test(ua)) return "mobile";
  if (/Tablet|iPad/i.test(ua)) return "tablet";
  return "desktop";
}

function getRoute(): string {
  return window.location.pathname;
}

function handleMetric(metric: Metric): void {
  buffer.push({
    name: metric.name.toLowerCase(),
    value: metric.value,
    route: getRoute(),
    device: getDevice(),
    app: config.app,
    timestamp: Date.now(),
  });

  // Flush when we have all 5 vitals or after 10s.
  if (buffer.length >= 5) {
    flush();
  }
}

function flush(): void {
  if (buffer.length === 0) return;

  const endpoint = config.endpoint || "/api/telemetry/vitals";
  const payload = JSON.stringify(buffer.splice(0));

  // Beacon API for reliability during page unload.
  if (navigator.sendBeacon) {
    navigator.sendBeacon(endpoint, payload);
  } else {
    fetch(endpoint, {
      method: "POST",
      body: payload,
      headers: { "Content-Type": "application/json" },
      keepalive: true,
    }).catch(() => {
      // Silent fail for telemetry.
    });
  }
}

/**
 * Initialize web vitals collection. Call once at app startup.
 */
export function initVitals(cfg: VitalsConfig): void {
  config = cfg;

  onLCP(handleMetric);
  onFID(handleMetric);
  onCLS(handleMetric);
  onTTFB(handleMetric);
  onINP(handleMetric);

  // Flush on page hide (covers tab close, navigation).
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") {
      flush();
    }
  });

  // Safety flush every 30s.
  setInterval(flush, 30000);
}
