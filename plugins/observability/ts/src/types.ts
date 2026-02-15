/**
 * Observability Plugin Types
 * Type definitions for services, health results, watchdog events, and service state
 */

// =============================================================================
// Database Record Types
// =============================================================================

export type ServiceState = 'discovered' | 'healthy' | 'unhealthy' | 'degraded' | 'unknown' | 'removed';

export type HealthStatus = 'healthy' | 'unhealthy' | 'degraded' | 'timeout' | 'error';

export type WatchdogEventType =
  | 'watchdog_started'
  | 'watchdog_stopped'
  | 'watchdog_timeout'
  | 'watchdog_reset'
  | 'service_discovered'
  | 'service_removed'
  | 'service_state_changed'
  | 'health_check_failed'
  | 'health_check_recovered';

export interface ServiceRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  name: string;
  container_id: string | null;
  container_name: string | null;
  image: string | null;
  service_type: string;
  host: string;
  port: number | null;
  health_endpoint: string | null;
  state: ServiceState;
  last_health_check: Date | null;
  last_healthy: Date | null;
  consecutive_failures: number;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface HealthHistoryRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  service_id: string;
  status: HealthStatus;
  response_time_ms: number | null;
  status_code: number | null;
  error_message: string | null;
  metadata: Record<string, unknown>;
  checked_at: Date;
}

export interface WatchdogEventRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  service_id: string | null;
  event_type: WatchdogEventType;
  message: string;
  severity: string;
  metadata: Record<string, unknown>;
  created_at: Date;
}

// =============================================================================
// API Request Types
// =============================================================================

export interface RegisterServiceRequest {
  name: string;
  container_id?: string;
  container_name?: string;
  image?: string;
  service_type?: string;
  host: string;
  port?: number;
  health_endpoint?: string;
}

export interface UpdateServiceRequest {
  name?: string;
  host?: string;
  port?: number;
  health_endpoint?: string;
  state?: ServiceState;
  metadata?: Record<string, unknown>;
}

export interface ListServicesQuery {
  state?: string;
  service_type?: string;
  limit?: number;
  offset?: number;
}

export interface ListHealthHistoryQuery {
  service_id?: string;
  status?: string;
  from?: string;
  to?: string;
  limit?: number;
}

export interface ListEventsQuery {
  service_id?: string;
  event_type?: string;
  severity?: string;
  from?: string;
  to?: string;
  limit?: number;
}

// =============================================================================
// API Response Types
// =============================================================================

export interface ServiceInfo {
  id: string;
  name: string;
  container_id: string | null;
  container_name: string | null;
  image: string | null;
  service_type: string;
  host: string;
  port: number | null;
  health_endpoint: string | null;
  state: ServiceState;
  last_health_check: Date | null;
  last_healthy: Date | null;
  consecutive_failures: number;
}

export interface HealthResult {
  service_id: string;
  service_name: string;
  status: HealthStatus;
  response_time_ms: number | null;
  status_code: number | null;
  error_message: string | null;
  checked_at: Date;
}

export interface WatchdogStatus {
  enabled: boolean;
  running: boolean;
  check_interval_seconds: number;
  timeout_seconds: number;
  services_monitored: number;
  last_check: Date | null;
  uptime_seconds: number;
}

export interface WatchdogEvent {
  id: string;
  service_id: string | null;
  event_type: WatchdogEventType;
  message: string;
  severity: string;
  metadata: Record<string, unknown>;
  created_at: Date;
}

// =============================================================================
// Docker Types
// =============================================================================

export interface DockerContainerInfo {
  id: string;
  name: string;
  image: string;
  state: string;
  status: string;
  ports: Array<{
    privatePort: number;
    publicPort?: number;
    type: string;
  }>;
  labels: Record<string, string>;
  created: number;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface ObservabilityStats {
  total_services: number;
  healthy_services: number;
  unhealthy_services: number;
  degraded_services: number;
  total_health_checks: number;
  total_watchdog_events: number;
  oldest_service: Date | null;
  newest_service: Date | null;
}
