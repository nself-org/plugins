/**
 * Admin API Plugin Types
 * Type definitions for system metrics, dashboard config, sessions, and storage
 */

// =============================================================================
// Database Record Types
// =============================================================================

export interface MetricsSnapshotRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  metric_type: MetricType;
  cpu_usage_percent: number | null;
  memory_used_bytes: number | null;
  memory_total_bytes: number | null;
  disk_used_bytes: number | null;
  disk_total_bytes: number | null;
  active_connections: number | null;
  request_count: number | null;
  error_count: number | null;
  avg_response_time_ms: number | null;
  active_sessions: number | null;
  metadata: Record<string, unknown>;
  created_at: Date;
}

export interface DashboardConfigRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  config_key: string;
  config_value: Record<string, unknown>;
  description: string | null;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Enum Types
// =============================================================================

export type MetricType = 'system' | 'database' | 'storage' | 'network' | 'application';

export type HealthStatus = 'healthy' | 'degraded' | 'unhealthy' | 'unknown';

// =============================================================================
// System Metrics Types
// =============================================================================

export interface SystemMetrics {
  cpu: CpuMetrics;
  memory: MemoryMetrics;
  disk: DiskMetrics;
  network: NetworkMetrics;
  process: ProcessMetrics;
  timestamp: string;
}

export interface CpuMetrics {
  usage_percent: number;
  load_average_1m: number;
  load_average_5m: number;
  load_average_15m: number;
}

export interface MemoryMetrics {
  used_bytes: number;
  total_bytes: number;
  free_bytes: number;
  usage_percent: number;
  heap_used_bytes: number;
  heap_total_bytes: number;
  external_bytes: number;
  rss_bytes: number;
}

export interface DiskMetrics {
  used_bytes: number;
  total_bytes: number;
  free_bytes: number;
  usage_percent: number;
}

export interface NetworkMetrics {
  active_connections: number;
  requests_per_minute: number;
  avg_response_time_ms: number;
  error_rate_percent: number;
}

export interface ProcessMetrics {
  uptime_seconds: number;
  pid: number;
  node_version: string;
  platform: string;
}

// =============================================================================
// Dashboard Config Types
// =============================================================================

export interface DashboardConfig {
  refresh_interval_seconds: number;
  visible_panels: string[];
  alert_thresholds: AlertThresholds;
  retention_days: number;
}

export interface AlertThresholds {
  cpu_warning_percent: number;
  cpu_critical_percent: number;
  memory_warning_percent: number;
  memory_critical_percent: number;
  disk_warning_percent: number;
  disk_critical_percent: number;
  error_rate_warning_percent: number;
  error_rate_critical_percent: number;
}

// =============================================================================
// Session Types
// =============================================================================

export interface SessionInfo {
  total_active: number;
  total_idle: number;
  total_waiting: number;
  max_connections: number;
  sessions: SessionDetail[];
  timestamp: string;
}

export interface SessionDetail {
  pid: number;
  state: string;
  query_start: string | null;
  wait_event_type: string | null;
  wait_event: string | null;
  backend_type: string;
  application_name: string;
  client_addr: string | null;
  duration_seconds: number | null;
}

// =============================================================================
// Storage Breakdown Types
// =============================================================================

export interface StorageBreakdown {
  database: DatabaseStorageInfo;
  tables: TableStorageInfo[];
  total_size_bytes: number;
  timestamp: string;
}

export interface DatabaseStorageInfo {
  name: string;
  size_bytes: number;
  size_pretty: string;
  table_count: number;
  index_count: number;
}

export interface TableStorageInfo {
  schema_name: string;
  table_name: string;
  total_size_bytes: number;
  table_size_bytes: number;
  index_size_bytes: number;
  row_estimate: number;
  size_pretty: string;
}

// =============================================================================
// Health Overview Types
// =============================================================================

export interface SystemHealthOverview {
  status: HealthStatus;
  uptime_seconds: number;
  database: DatabaseHealth;
  services: ServiceHealth[];
  prometheus: PrometheusHealth | null;
  timestamp: string;
}

export interface DatabaseHealth {
  status: HealthStatus;
  latency_ms: number;
  connection_count: number;
  max_connections: number;
  version: string;
}

export interface ServiceHealth {
  name: string;
  status: HealthStatus;
  latency_ms: number | null;
  last_check: string;
  error: string | null;
}

export interface PrometheusHealth {
  status: HealthStatus;
  url: string;
  latency_ms: number | null;
  error: string | null;
}

// =============================================================================
// Dashboard Stats Types
// =============================================================================

export interface DashboardStats {
  snapshots_total: number;
  snapshots_today: number;
  oldest_snapshot: string | null;
  newest_snapshot: string | null;
  config_entries: number;
  avg_cpu_24h: number | null;
  avg_memory_24h: number | null;
  peak_connections_24h: number | null;
  total_requests_24h: number | null;
  total_errors_24h: number | null;
}

// =============================================================================
// API Request/Response Types
// =============================================================================

export interface CreateMetricsSnapshotRequest {
  metric_type: MetricType;
  cpu_usage_percent?: number;
  memory_used_bytes?: number;
  memory_total_bytes?: number;
  disk_used_bytes?: number;
  disk_total_bytes?: number;
  active_connections?: number;
  request_count?: number;
  error_count?: number;
  avg_response_time_ms?: number;
  active_sessions?: number;
  metadata?: Record<string, unknown>;
}

export interface UpdateDashboardConfigRequest {
  config_key: string;
  config_value: Record<string, unknown>;
  description?: string;
}

export interface MetricsQueryParams {
  metric_type?: MetricType;
  from?: string;
  to?: string;
  limit?: number;
}

// =============================================================================
// WebSocket Types
// =============================================================================

export interface WsMessage {
  type: 'metrics' | 'health' | 'sessions' | 'storage' | 'error';
  data: unknown;
  timestamp: string;
}

// =============================================================================
// Config Type
// =============================================================================

export interface Config {
  // Server
  port: number;
  host: string;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // Prometheus
  prometheusUrl: string;

  // Cache
  cacheTtlSeconds: number;

  // Metrics
  metricsRetentionDays: number;
  snapshotIntervalMinutes: number;

  // WebSocket
  wsEnabled: boolean;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export interface SecurityConfig {
  apiKey?: string;
  rateLimitMax?: number;
  rateLimitWindowMs?: number;
}
