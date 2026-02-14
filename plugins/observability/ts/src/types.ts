/**
 * Observability Plugin Types
 */

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface LogEntry {
  timestamp: string;
  level: LogLevel;
  message: string;
  trace_id?: string;
  span_id?: string;
  user_id?: string;
  source_account_id?: string;
  service?: string;
  metadata?: Record<string, unknown>;
}

export interface TraceSpan {
  trace_id: string;
  span_id: string;
  parent_span_id?: string | null;
  operation_name: string;
  start_time: string;
  end_time: string;
  duration_ms: number;
  tags?: Record<string, string | number | boolean>;
  logs?: Array<{
    timestamp: string;
    message: string;
    fields?: Record<string, unknown>;
  }>;
}

export interface MetricLabels {
  [key: string]: string;
}

export interface HistogramBucket {
  le: string;
  count: number;
}

export interface HistogramMetric {
  buckets: HistogramBucket[];
  sum: number;
  count: number;
}

export interface Dashboard {
  id: string;
  title: string;
  description?: string;
  tags?: string[];
  json: Record<string, unknown>;
}

export interface IngestLogRequest {
  level: LogLevel;
  message: string;
  timestamp?: string;
  trace_id?: string;
  span_id?: string;
  user_id?: string;
  source_account_id?: string;
  service?: string;
  metadata?: Record<string, unknown>;
}

export interface IngestTraceRequest {
  trace_id: string;
  span_id: string;
  parent_span_id?: string | null;
  operation_name: string;
  start_time: string;
  end_time: string;
  duration_ms: number;
  tags?: Record<string, string | number | boolean>;
  logs?: Array<{
    timestamp: string;
    message: string;
    fields?: Record<string, unknown>;
  }>;
}

export interface MetricsSnapshot {
  http_requests_total: Record<string, number>;
  http_request_duration_seconds: Record<string, HistogramMetric>;
  db_queries_total: Record<string, number>;
  db_query_duration_seconds: Record<string, HistogramMetric>;
  queue_size: number;
  queue_jobs_processed_total: number;
  queue_jobs_failed_total: number;
  videos_uploaded_total: number;
  streams_started_total: number;
  users_active: number;
  errors_total: Record<string, number>;
}
