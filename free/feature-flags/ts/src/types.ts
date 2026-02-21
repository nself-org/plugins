/**
 * Feature Flags Plugin Types
 * Complete type definitions for feature flags, rules, segments, and evaluations
 */

// =============================================================================
// Configuration
// =============================================================================

export interface FeatureFlagsConfig {
  port: number;
  host: string;
  evaluationLogEnabled: boolean;
  evaluationLogSampleRate: number;
  cacheTtlSeconds: number;
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
// Flag Types
// =============================================================================

export type FlagType = 'release' | 'ops' | 'experiment' | 'kill_switch';

export interface FlagRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  key: string;
  name: string | null;
  description: string | null;
  flag_type: FlagType;
  enabled: boolean;
  default_value: unknown;
  tags: string[];
  owner: string | null;
  stale_after_days: number | null;
  last_evaluated_at: Date | null;
  evaluation_count: number;
  created_at: Date;
  updated_at: Date;
}

export interface CreateFlagRequest {
  key: string;
  name?: string;
  description?: string;
  flag_type?: FlagType;
  enabled?: boolean;
  default_value?: unknown;
  tags?: string[];
  owner?: string;
  stale_after_days?: number;
}

export interface UpdateFlagRequest {
  name?: string;
  description?: string;
  flag_type?: FlagType;
  enabled?: boolean;
  default_value?: unknown;
  tags?: string[];
  owner?: string;
  stale_after_days?: number;
}

// =============================================================================
// Rule Types
// =============================================================================

export type RuleType = 'percentage' | 'user_list' | 'segment' | 'attribute' | 'schedule';

export interface RuleRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  flag_id: string;
  name: string | null;
  rule_type: RuleType;
  conditions: RuleConditions;
  value: unknown;
  priority: number;
  enabled: boolean;
  created_at: Date;
  updated_at: Date;
}

export interface RuleConditions {
  // Percentage rollout
  percentage?: number;

  // User list targeting
  users?: string[];

  // Segment targeting
  segment_id?: string;

  // Attribute matching
  attribute?: string;
  operator?: 'eq' | 'neq' | 'gt' | 'lt' | 'gte' | 'lte' | 'contains' | 'regex';
  attribute_value?: unknown;

  // Schedule-based
  start_at?: string; // ISO 8601
  end_at?: string; // ISO 8601
}

export interface CreateRuleRequest {
  flag_key: string;
  name?: string;
  rule_type: RuleType;
  conditions: RuleConditions;
  value?: unknown;
  priority?: number;
  enabled?: boolean;
}

export interface UpdateRuleRequest {
  name?: string;
  conditions?: RuleConditions;
  value?: unknown;
  priority?: number;
  enabled?: boolean;
}

// =============================================================================
// Segment Types
// =============================================================================

export type SegmentMatchType = 'all' | 'any';

export interface SegmentRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  match_type: SegmentMatchType;
  rules: SegmentRule[];
  created_at: Date;
  updated_at: Date;
}

export interface SegmentRule {
  attribute: string;
  operator: 'eq' | 'neq' | 'gt' | 'lt' | 'gte' | 'lte' | 'contains' | 'regex';
  value: unknown;
}

export interface CreateSegmentRequest {
  name: string;
  description?: string;
  match_type?: SegmentMatchType;
  rules: SegmentRule[];
}

export interface UpdateSegmentRequest {
  name?: string;
  description?: string;
  match_type?: SegmentMatchType;
  rules?: SegmentRule[];
}

// =============================================================================
// Evaluation Types
// =============================================================================

export interface EvaluationContext {
  [key: string]: unknown;
}

export interface EvaluationRequest {
  flag_key: string;
  user_id?: string;
  context?: EvaluationContext;
}

export interface BatchEvaluationRequest {
  flag_keys: string[];
  user_id?: string;
  context?: EvaluationContext;
}

export type EvaluationReason =
  | 'disabled'
  | 'rule_match'
  | 'default'
  | 'not_found'
  | 'error';

export interface EvaluationResult {
  flag_key: string;
  value: unknown;
  reason: EvaluationReason;
  rule_id?: string;
  error?: string;
}

export interface EvaluationRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  flag_key: string;
  user_id: string | null;
  context: EvaluationContext | null;
  result: unknown;
  rule_id: string | null;
  reason: EvaluationReason;
  evaluated_at: Date;
}

// =============================================================================
// Webhook Event Types
// =============================================================================

export interface WebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  created_at: Date;
}

// =============================================================================
// Statistics Types
// =============================================================================

export interface FlagStats {
  flags: number;
  rules: number;
  segments: number;
  evaluations: number;
  lastEvaluatedAt: Date | null;
}

export interface FlagDetail extends FlagRecord {
  rules: RuleRecord[];
}

// =============================================================================
// Query Options
// =============================================================================

export interface ListFlagsOptions {
  flag_type?: FlagType;
  tag?: string;
  enabled?: boolean;
  limit?: number;
  offset?: number;
}

export interface ListEvaluationsOptions {
  flag_key?: string;
  user_id?: string;
  reason?: EvaluationReason;
  since?: Date;
  limit?: number;
  offset?: number;
}
