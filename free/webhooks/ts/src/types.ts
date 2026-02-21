/**
 * Webhooks Plugin Types
 * Complete type definitions for outbound webhook delivery
 */

export interface WebhooksPluginConfig {
  port: number;
  host: string;
  maxAttempts: number;
  requestTimeoutMs: number;
  maxPayloadSize: number;
  concurrentDeliveries: number;
  retryDelays: number[];
  autoDisableThreshold: number;
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
// Webhook Endpoints
// =============================================================================

export interface WebhookEndpointRecord {
  id: string;
  source_account_id: string;
  url: string;
  description: string | null;
  secret: string;
  events: string[];
  headers: Record<string, string>;
  enabled: boolean;
  failure_count: number;
  last_success_at: Date | null;
  last_failure_at: Date | null;
  disabled_at: Date | null;
  disabled_reason: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateEndpointInput {
  url: string;
  description?: string;
  events: string[];
  headers?: Record<string, string>;
  metadata?: Record<string, unknown>;
}

export interface UpdateEndpointInput {
  url?: string;
  description?: string;
  events?: string[];
  headers?: Record<string, string>;
  enabled?: boolean;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Webhook Deliveries
// =============================================================================

export type DeliveryStatus = 'pending' | 'delivering' | 'delivered' | 'failed' | 'dead_letter';

export interface WebhookDeliveryRecord {
  id: string;
  source_account_id: string;
  endpoint_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  status: DeliveryStatus;
  response_status: number | null;
  response_body: string | null;
  response_time_ms: number | null;
  attempt_count: number;
  max_attempts: number;
  next_retry_at: Date | null;
  error_message: string | null;
  signature: string;
  delivered_at: Date | null;
  created_at: Date;
}

export interface DispatchEventInput {
  event_type: string;
  payload: Record<string, unknown>;
  endpoints?: string[];
  idempotency_key?: string;
}

// =============================================================================
// Event Types
// =============================================================================

export interface WebhookEventTypeRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  source_plugin: string | null;
  schema: Record<string, unknown> | null;
  sample_payload: Record<string, unknown> | null;
  created_at: Date;
}

export interface RegisterEventTypeInput {
  name: string;
  description?: string;
  source_plugin?: string;
  schema?: Record<string, unknown>;
  sample_payload?: Record<string, unknown>;
}

// =============================================================================
// Dead Letters
// =============================================================================

export interface WebhookDeadLetterRecord {
  id: string;
  source_account_id: string;
  delivery_id: string;
  endpoint_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  last_error: string;
  attempt_count: number;
  resolved: boolean;
  resolved_at: Date | null;
  created_at: Date;
}

// =============================================================================
// Statistics
// =============================================================================

export interface WebhookStats {
  endpoints: {
    total: number;
    enabled: number;
    disabled: number;
  };
  deliveries: {
    total: number;
    pending: number;
    delivered: number;
    failed: number;
    dead_letter: number;
  };
  dead_letters: {
    total: number;
    unresolved: number;
    resolved: number;
  };
  event_types: number;
}

export interface DeliveryStatsByEndpoint {
  endpoint_id: string;
  endpoint_url: string;
  total_deliveries: number;
  successful: number;
  failed: number;
  success_rate: number;
  avg_response_time_ms: number;
}

export interface DeliveryStatsByEventType {
  event_type: string;
  total_deliveries: number;
  successful: number;
  failed: number;
  success_rate: number;
}

// =============================================================================
// Signature Format
// =============================================================================

export interface WebhookSignature {
  timestamp: number;
  signature: string;
}
