/**
 * Audit Plugin Types
 * All TypeScript interfaces for the audit plugin
 */

// ============================================================================
// Database Record Types
// ============================================================================

export interface AuditEventRecord {
  id: string;
  app_id: string;
  source_plugin: string;
  event_type: string;
  actor_id: string | null;
  actor_type: string | null;
  resource_type: string | null;
  resource_id: string | null;
  action: string;
  outcome: 'success' | 'failure' | 'unknown';
  severity: 'low' | 'medium' | 'high' | 'critical';
  ip_address: string | null;
  user_agent: string | null;
  location: string | null;
  details: Record<string, unknown>;
  metadata: Record<string, unknown>;
  checksum: string;
  created_at: Date;
}

export interface RetentionPolicyRecord {
  id: string;
  app_id: string;
  name: string;
  description: string | null;
  event_type_pattern: string;
  retention_days: number;
  enabled: boolean;
  last_executed_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface AlertRuleRecord {
  id: string;
  app_id: string;
  name: string;
  description: string | null;
  event_type_pattern: string;
  severity_threshold: 'low' | 'medium' | 'high' | 'critical';
  conditions: Record<string, unknown>;
  webhook_url: string | null;
  enabled: boolean;
  last_triggered_at: Date | null;
  trigger_count: number;
  created_at: Date;
  updated_at: Date;
}

export interface AuditWebhookEventRecord {
  id: string;
  app_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  delivered: boolean;
  delivered_at: Date | null;
  delivery_attempts: number;
  last_error: string | null;
  created_at: Date;
}

// ============================================================================
// API Request/Response Types
// ============================================================================

// Event Logging
export interface LogEventRequest {
  sourcePlugin: string;
  eventType: string;
  actorId?: string;
  actorType?: string;
  resourceType?: string;
  resourceId?: string;
  action: string;
  outcome?: 'success' | 'failure' | 'unknown';
  severity?: 'low' | 'medium' | 'high' | 'critical';
  ipAddress?: string;
  userAgent?: string;
  location?: string;
  details?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface LogEventResponse {
  eventId: string;
  checksum: string;
  createdAt: string;
}

// Event Querying
export interface QueryEventsRequest {
  sourcePlugin?: string;
  eventType?: string;
  actorId?: string;
  resourceType?: string;
  resourceId?: string;
  action?: string;
  outcome?: 'success' | 'failure' | 'unknown';
  severity?: 'low' | 'medium' | 'high' | 'critical';
  startDate?: string;
  endDate?: string;
  limit?: number;
  offset?: number;
}

export interface QueryEventsResponse {
  events: AuditEventInfo[];
  total: number;
  limit: number;
  offset: number;
}

export interface AuditEventInfo {
  id: string;
  sourcePlugin: string;
  eventType: string;
  actorId: string | null;
  actorType: string | null;
  resourceType: string | null;
  resourceId: string | null;
  action: string;
  outcome: string;
  severity: string;
  ipAddress: string | null;
  userAgent: string | null;
  location: string | null;
  details: Record<string, unknown>;
  metadata: Record<string, unknown>;
  checksum: string;
  createdAt: string;
}

// Export
export type ExportFormat = 'csv' | 'json' | 'jsonl' | 'cef' | 'leef' | 'syslog';

export interface ExportEventsRequest {
  format: ExportFormat;
  sourcePlugin?: string;
  eventType?: string;
  startDate?: string;
  endDate?: string;
  limit?: number;
}

export interface ExportEventsResponse {
  format: ExportFormat;
  data: string;
  rowCount: number;
  exportedAt: string;
}

// Retention Policies
export interface CreateRetentionPolicyRequest {
  name: string;
  description?: string;
  eventTypePattern: string;
  retentionDays: number;
  enabled?: boolean;
}

export interface UpdateRetentionPolicyRequest {
  name?: string;
  description?: string;
  eventTypePattern?: string;
  retentionDays?: number;
  enabled?: boolean;
}

export interface RetentionPolicyInfo {
  id: string;
  name: string;
  description: string | null;
  eventTypePattern: string;
  retentionDays: number;
  enabled: boolean;
  lastExecutedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface ExecuteRetentionResponse {
  policiesExecuted: number;
  eventsDeleted: number;
  executedAt: string;
}

// Alert Rules
export interface CreateAlertRuleRequest {
  name: string;
  description?: string;
  eventTypePattern: string;
  severityThreshold: 'low' | 'medium' | 'high' | 'critical';
  conditions?: Record<string, unknown>;
  webhookUrl?: string;
  enabled?: boolean;
}

export interface UpdateAlertRuleRequest {
  name?: string;
  description?: string;
  eventTypePattern?: string;
  severityThreshold?: 'low' | 'medium' | 'high' | 'critical';
  conditions?: Record<string, unknown>;
  webhookUrl?: string;
  enabled?: boolean;
}

export interface AlertRuleInfo {
  id: string;
  name: string;
  description: string | null;
  eventTypePattern: string;
  severityThreshold: string;
  conditions: Record<string, unknown>;
  webhookUrl: string | null;
  enabled: boolean;
  lastTriggeredAt: string | null;
  triggerCount: number;
  createdAt: string;
  updatedAt: string;
}

// Compliance Reports
export type ComplianceFramework = 'SOC2' | 'HIPAA' | 'GDPR' | 'PCI';

export interface GenerateComplianceReportRequest {
  framework: ComplianceFramework;
  startDate?: string;
  endDate?: string;
}

export interface ComplianceReportResponse {
  framework: ComplianceFramework;
  period: {
    startDate: string;
    endDate: string;
  };
  summary: {
    totalEvents: number;
    criticalEvents: number;
    highSeverityEvents: number;
    failedActions: number;
    uniqueActors: number;
    uniqueResources: number;
  };
  eventsByType: Record<string, number>;
  eventsBySeverity: Record<string, number>;
  eventsByOutcome: Record<string, number>;
  topActors: Array<{ actorId: string; eventCount: number }>;
  topResources: Array<{ resourceType: string; resourceId: string; eventCount: number }>;
  alertsTriggered: number;
  complianceChecks: ComplianceCheck[];
  generatedAt: string;
}

export interface ComplianceCheck {
  control: string;
  requirement: string;
  status: 'pass' | 'fail' | 'warning';
  details: string;
}

// Verification
export interface VerifyEventRequest {
  eventId: string;
}

export interface VerifyEventResponse {
  eventId: string;
  valid: boolean;
  expectedChecksum: string;
  actualChecksum: string;
  message: string;
}

// ============================================================================
// Configuration Types
// ============================================================================

export interface AuditConfig {
  port: number;
  host: string;
  logLevel: 'debug' | 'info' | 'warn' | 'error';

  // Database
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };

  // Multi-app
  appIds: string[];

  // Fallback logging (if DB fails)
  fallback: {
    logPath: string;
  };

  // SIEM integration
  siem: {
    splunk?: {
      hecUrl: string;
      hecToken: string;
    };
    elk?: {
      url: string;
      index: string;
      apiKey: string;
    };
    datadog?: {
      apiKey: string;
      site: string;
    };
  };

  // Retention
  retention: {
    defaultDays: number;
  };

  // Compliance
  compliance: {
    frameworks: ComplianceFramework[];
  };

  // Alerts
  alerts: {
    webhookUrl: string | null;
  };

  // Export
  export: {
    maxRows: number;
  };
}

// ============================================================================
// Service Types
// ============================================================================

export interface AuditStats {
  totalEvents: number;
  last24Hours: number;
  last7Days: number;
  retentionPolicies: number;
  alertRules: number;
  oldestEvent: string | null;
  newestEvent: string | null;
  diskUsageMB: number | null;
}

export interface HealthCheckResponse {
  status: 'ok' | 'error';
  plugin: string;
  timestamp: string;
  version: string;
}

export interface ReadyCheckResponse {
  ready: boolean;
  database: 'ok' | 'error';
  immutabilityTriggers: 'ok' | 'missing';
  timestamp: string;
}

export interface LiveCheckResponse {
  alive: boolean;
  uptime: number;
  memory: {
    used: number;
    total: number;
  };
  stats: AuditStats;
}

// ============================================================================
// Internal Types
// ============================================================================

export interface SiemExportResult {
  provider: string;
  success: boolean;
  eventCount: number;
  error?: string;
}
