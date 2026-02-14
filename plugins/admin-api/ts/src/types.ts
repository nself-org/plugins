/**
 * Admin API Plugin Type Definitions
 */

// =============================================================================
// Core Types
// =============================================================================

export type AdminRole = 'super_admin' | 'admin' | 'moderator';

export type AdminAction =
  | 'user_banned'
  | 'user_unbanned'
  | 'user_deleted'
  | 'content_deleted'
  | 'service_restarted'
  | 'config_updated'
  | 'alert_acknowledged';

export type EntityType = 'user' | 'media_item' | 'service' | 'config' | 'alert';

export type HealthStatus = 'healthy' | 'degraded' | 'unhealthy';

export type AlertSeverity = 'info' | 'warning' | 'error' | 'critical';

export type AlertStatus = 'active' | 'acknowledged' | 'resolved';

// =============================================================================
// Admin User Types
// =============================================================================

export interface AdminUserRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  email: string;
  password_hash: string;
  role: AdminRole;
  active: boolean;
  last_login_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface CreateAdminUserInput {
  email: string;
  password: string;
  role: AdminRole;
}

export interface UpdateAdminUserInput {
  email?: string;
  password?: string;
  role?: AdminRole;
  active?: boolean;
}

// =============================================================================
// Audit Log Types
// =============================================================================

export interface AuditLogRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  admin_user_id: string | null;
  action: AdminAction;
  entity_type: EntityType | null;
  entity_id: string | null;
  details: Record<string, unknown>;
  ip_address: string | null;
  user_agent: string | null;
  created_at: Date;
}

export interface CreateAuditLogInput {
  admin_user_id?: string;
  action: AdminAction;
  entity_type?: EntityType;
  entity_id?: string;
  details?: Record<string, unknown>;
  ip_address?: string;
  user_agent?: string;
}

// =============================================================================
// System Health Types
// =============================================================================

export interface SystemHealth {
  status: HealthStatus;
  timestamp: Date;
  uptime: number;
  database: DatabaseHealth;
  storage: StorageHealth;
  queue: QueueHealth;
  services: ServiceHealth[];
}

export interface DatabaseHealth {
  status: HealthStatus;
  connection_pool: {
    total: number;
    idle: number;
    active: number;
    waiting: number;
  };
  slow_queries: number;
  avg_query_time_ms: number;
}

export interface StorageHealth {
  status: HealthStatus;
  total_bytes: number;
  used_bytes: number;
  available_bytes: number;
  usage_percent: number;
  buckets: BucketStats[];
}

export interface BucketStats {
  name: string;
  size_bytes: number;
  file_count: number;
}

export interface QueueHealth {
  status: HealthStatus;
  pending_jobs: number;
  failed_jobs: number;
  processing_jobs: number;
  avg_wait_time_ms: number;
}

export interface ServiceHealth {
  name: string;
  status: HealthStatus;
  uptime: number;
  last_check: Date;
  error?: string;
}

// =============================================================================
// User Metrics Types
// =============================================================================

export interface UserMetrics {
  dau: number;
  wau: number;
  mau: number;
  signups_today: number;
  signups_week: number;
  signups_month: number;
  total_users: number;
  active_users: number;
  banned_users: number;
  retention_7d: number;
  retention_30d: number;
}

export interface UserListItem {
  id: string;
  email: string;
  created_at: Date;
  last_login_at: Date | null;
  banned: boolean;
  banned_at: Date | null;
  banned_reason: string | null;
}

export interface UserDetails extends UserListItem {
  metadata: Record<string, unknown>;
  total_content: number;
  total_playback_hours: number;
}

// =============================================================================
// Content Metrics Types
// =============================================================================

export interface ContentMetrics {
  total_items: number;
  added_today: number;
  added_week: number;
  added_month: number;
  total_storage_bytes: number;
  storage_by_type: Record<string, number>;
  most_viewed: ContentItem[];
}

export interface ContentItem {
  id: string;
  title: string;
  type: string;
  view_count: number;
  storage_bytes: number;
  created_at: Date;
}

// =============================================================================
// Playback Metrics Types
// =============================================================================

export interface PlaybackMetrics {
  active_streams: number;
  bandwidth_bytes_per_sec: number;
  total_streams_today: number;
  total_streams_week: number;
  total_streams_month: number;
  peak_concurrent_streams: number;
  errors_per_hour: number;
  avg_bitrate_kbps: number;
}

// =============================================================================
// Alert Types
// =============================================================================

export interface AlertRecord {
  id: string;
  source_account_id: string;
  severity: AlertSeverity;
  status: AlertStatus;
  title: string;
  message: string;
  details: Record<string, unknown>;
  triggered_at: Date;
  acknowledged_at: Date | null;
  acknowledged_by: string | null;
  resolved_at: Date | null;
  created_at: Date;
}

export interface CreateAlertInput {
  severity: AlertSeverity;
  title: string;
  message: string;
  details?: Record<string, unknown>;
}

// =============================================================================
// Configuration Types
// =============================================================================

export interface Config {
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };
  server: {
    port: number;
    host: string;
  };
  auth: {
    jwtSecret: string;
    sessionTimeoutMinutes: number;
  };
  metrics: {
    collectionIntervalSeconds: number;
  };
  security: {
    apiKey?: string;
    rateLimitMax: number;
    rateLimitWindowMs: number;
  };
}

// =============================================================================
// Database Stats
// =============================================================================

export interface AdminStats {
  total_users: number;
  total_audit_logs: number;
  audit_logs_today: number;
  most_common_actions: Array<{
    action: AdminAction;
    count: number;
  }>;
}
