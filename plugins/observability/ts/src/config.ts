/**
 * Observability Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';

export interface Config {
  // Server
  port: number;
  host: string;

  // Prometheus
  prometheusEnabled: boolean;
  metricsPath: string;

  // Loki (log aggregation)
  lokiEnabled: boolean;
  lokiUrl: string;

  // Tempo (distributed tracing)
  tempoEnabled: boolean;
  tempoUrl: string;

  // Grafana
  grafanaUrl: string;
  grafanaApiKey: string;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('OBSERVABILITY');

  const config: Config = {
    // Server
    port: parseInt(process.env.OBSERVABILITY_PLUGIN_PORT ?? process.env.PORT ?? '3215', 10),
    host: process.env.OBSERVABILITY_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Prometheus
    prometheusEnabled: process.env.OBSERVABILITY_PROMETHEUS_ENABLED !== 'false',
    metricsPath: process.env.OBSERVABILITY_METRICS_PATH ?? '/metrics',

    // Loki
    lokiEnabled: process.env.OBSERVABILITY_LOKI_ENABLED !== 'false',
    lokiUrl: process.env.OBSERVABILITY_LOKI_URL ?? 'http://loki:3100',

    // Tempo
    tempoEnabled: process.env.OBSERVABILITY_TEMPO_ENABLED !== 'false',
    tempoUrl: process.env.OBSERVABILITY_TEMPO_URL ?? 'http://tempo:9411',

    // Grafana
    grafanaUrl: process.env.OBSERVABILITY_GRAFANA_URL ?? 'http://grafana:3000',
    grafanaApiKey: process.env.OBSERVABILITY_GRAFANA_API_KEY ?? '',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (config.port < 1 || config.port > 65535) {
    throw new Error('OBSERVABILITY_PLUGIN_PORT must be between 1 and 65535');
  }

  if (config.lokiEnabled && !config.lokiUrl) {
    throw new Error('OBSERVABILITY_LOKI_URL is required when Loki is enabled');
  }

  if (config.tempoEnabled && !config.tempoUrl) {
    throw new Error('OBSERVABILITY_TEMPO_URL is required when Tempo is enabled');
  }

  return config;
}
