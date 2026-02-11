/**
 * Analytics Plugin Types
 * Complete type definitions for analytics objects
 */

export interface AnalyticsPluginConfig {
  port: number;
  host: string;
  batchSize: number;
  rollupIntervalMs: number;
  eventRetentionDays: number;
  counterRetentionDays: number;
  apiKey?: string;
  rateLimitMax: number;
  rateLimitWindowMs: number;
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl?: boolean;
  };
}

// =============================================================================
// Event Types
// =============================================================================

export interface AnalyticsEventRecord {
  id: string;
  source_account_id: string;
  event_name: string;
  event_category: string | null;
  user_id: string | null;
  session_id: string | null;
  properties: Record<string, unknown>;
  context: Record<string, unknown>;
  source_plugin: string | null;
  timestamp: Date;
  created_at: Date;
  [key: string]: unknown;
}

export interface TrackEventRequest {
  event_name: string;
  event_category?: string;
  user_id?: string;
  session_id?: string;
  properties?: Record<string, unknown>;
  context?: Record<string, unknown>;
  source_plugin?: string;
  timestamp?: string;
}

export interface TrackEventBatchRequest {
  events: TrackEventRequest[];
}

// =============================================================================
// Counter Types
// =============================================================================

export type CounterPeriod = 'hourly' | 'daily' | 'monthly' | 'all_time';

export interface AnalyticsCounterRecord {
  id: string;
  source_account_id: string;
  counter_name: string;
  dimension: string;
  period: CounterPeriod;
  period_start: Date;
  value: bigint;
  metadata: Record<string, unknown>;
  updated_at: Date;
  [key: string]: unknown;
}

export interface IncrementCounterRequest {
  counter_name: string;
  dimension?: string;
  increment?: number;
  metadata?: Record<string, unknown>;
}

export interface CounterQueryParams {
  counter_name: string;
  dimension?: string;
  period?: CounterPeriod;
  start_date?: string;
  end_date?: string;
}

export interface CounterValue {
  counter_name: string;
  dimension: string;
  period: CounterPeriod;
  period_start: Date;
  value: number;
  updated_at: Date;
}

export interface CounterTimeseriesPoint {
  timestamp: Date;
  value: number;
}

// =============================================================================
// Funnel Types
// =============================================================================

export interface FunnelStep {
  name: string;
  event_name: string;
  filters?: Record<string, unknown>;
}

export interface AnalyticsFunnelRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  steps: FunnelStep[];
  window_hours: number;
  enabled: boolean;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateFunnelRequest {
  name: string;
  description?: string;
  steps: FunnelStep[];
  window_hours?: number;
  enabled?: boolean;
}

export interface UpdateFunnelRequest {
  name?: string;
  description?: string;
  steps?: FunnelStep[];
  window_hours?: number;
  enabled?: boolean;
}

export interface FunnelAnalysisResult {
  funnel_id: string;
  funnel_name: string;
  steps: FunnelStepResult[];
  total_entered: number;
  total_completed: number;
  overall_conversion_rate: number;
  analysis_timestamp: Date;
}

export interface FunnelStepResult {
  step_number: number;
  step_name: string;
  event_name: string;
  users: number;
  conversion_rate: number;
  drop_off_rate: number;
}

// =============================================================================
// Quota Types
// =============================================================================

export type QuotaScope = 'app' | 'user' | 'device';
export type QuotaAction = 'warn' | 'block' | 'throttle';

export interface AnalyticsQuotaRecord {
  id: string;
  source_account_id: string;
  name: string;
  scope: QuotaScope;
  scope_id: string | null;
  counter_name: string;
  max_value: bigint;
  period: CounterPeriod;
  action_on_exceed: QuotaAction;
  enabled: boolean;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateQuotaRequest {
  name: string;
  scope?: QuotaScope;
  scope_id?: string;
  counter_name: string;
  max_value: number;
  period: CounterPeriod;
  action_on_exceed?: QuotaAction;
  enabled?: boolean;
}

export interface UpdateQuotaRequest {
  name?: string;
  scope?: QuotaScope;
  scope_id?: string;
  counter_name?: string;
  max_value?: number;
  period?: CounterPeriod;
  action_on_exceed?: QuotaAction;
  enabled?: boolean;
}

export interface AnalyticsQuotaViolationRecord {
  id: string;
  source_account_id: string;
  quota_id: string;
  scope_id: string | null;
  current_value: bigint;
  max_value: bigint;
  action_taken: QuotaAction;
  notified: boolean;
  created_at: Date;
  [key: string]: unknown;
}

export interface QuotaCheckRequest {
  counter_name: string;
  scope_id?: string;
  increment?: number;
}

export interface QuotaCheckResult {
  allowed: boolean;
  quota_name: string;
  current_value: number;
  max_value: number;
  remaining: number;
  action: QuotaAction;
}

// =============================================================================
// Webhook Types
// =============================================================================

export interface AnalyticsWebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  created_at: Date;
  [key: string]: unknown;
}

// =============================================================================
// Dashboard & Stats Types
// =============================================================================

export interface DashboardStats {
  total_events: number;
  unique_users: number;
  unique_sessions: number;
  active_quotas: number;
  quota_violations: number;
  top_events: Array<{
    event_name: string;
    count: number;
  }>;
  recent_events: Array<{
    event_name: string;
    timestamp: Date;
    user_id: string | null;
  }>;
  quota_status: Array<{
    quota_name: string;
    current_value: number;
    max_value: number;
    usage_percent: number;
  }>;
}

export interface AnalyticsStats {
  events: number;
  counters: number;
  funnels: number;
  quotas: number;
  violations: number;
  lastEventAt?: Date | null;
}
