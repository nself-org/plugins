/**
 * Content Policy Plugin Types
 * Complete type definitions for content policy evaluation system
 */

// =============================================================================
// Configuration
// =============================================================================

export interface ContentPolicyConfig {
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

  // Policy defaults
  defaultAction: 'allow' | 'deny' | 'flag' | 'quarantine';
  profanityEnabled: boolean;
  maxContentLength: number;
  evaluationLogEnabled: boolean;

  // Security
  apiKey?: string;
  rateLimitMax: number;
  rateLimitWindowMs: number;

  logLevel: string;
}

// =============================================================================
// Database Records
// =============================================================================

export interface PolicyRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  content_types: string[];
  enabled: boolean;
  priority: number;
  mode: 'enforce' | 'monitor' | 'disabled';
  created_at: Date;
  updated_at: Date;
}

export type RuleType =
  | 'keyword'
  | 'regex'
  | 'length'
  | 'profanity'
  | 'external_api'
  | 'media_type'
  | 'link_check';

export type RuleAction = 'allow' | 'deny' | 'flag' | 'quarantine';

export type RuleSeverity = 'low' | 'medium' | 'high' | 'critical';

export interface RuleRecord {
  id: string;
  source_account_id: string;
  policy_id: string;
  name: string;
  rule_type: RuleType;
  config: RuleConfig;
  action: RuleAction;
  severity: RuleSeverity;
  message: string | null;
  enabled: boolean;
  created_at: Date;
  updated_at: Date;
}

export type RuleConfig =
  | KeywordRuleConfig
  | RegexRuleConfig
  | LengthRuleConfig
  | ProfanityRuleConfig
  | MediaTypeRuleConfig
  | LinkCheckRuleConfig
  | ExternalApiRuleConfig;

export interface KeywordRuleConfig {
  type: 'keyword';
  word_list_id?: string;
  words?: string[];
  case_sensitive?: boolean;
}

export interface RegexRuleConfig {
  type: 'regex';
  pattern: string;
  flags?: string;
}

export interface LengthRuleConfig {
  type: 'length';
  min_length?: number;
  max_length?: number;
}

export interface ProfanityRuleConfig {
  type: 'profanity';
  level?: 'mild' | 'moderate' | 'severe';
}

export interface MediaTypeRuleConfig {
  type: 'media_type';
  allowed_types?: string[];
  blocked_types?: string[];
}

export interface LinkCheckRuleConfig {
  type: 'link_check';
  blocked_domains?: string[];
  allowed_domains?: string[];
  check_redirects?: boolean;
}

export interface ExternalApiRuleConfig {
  type: 'external_api';
  endpoint: string;
  method?: 'GET' | 'POST';
  headers?: Record<string, string>;
  timeout?: number;
}

export type EvaluationResult = 'allowed' | 'denied' | 'flagged' | 'quarantined';

export interface EvaluationRecord {
  id: string;
  source_account_id: string;
  content_type: string;
  content_id: string | null;
  content_text: string | null;
  submitter_id: string | null;
  policy_id: string | null;
  rule_id: string | null;
  result: EvaluationResult;
  matched_rules: MatchedRule[];
  score: number;
  processing_time_ms: number;
  override_id: string | null;
  created_at: Date;
}

export interface MatchedRule {
  rule_id: string;
  rule_name: string;
  rule_type: RuleType;
  action: RuleAction;
  severity: RuleSeverity;
  message?: string;
  matched_text?: string;
}

export interface WordListRecord {
  id: string;
  source_account_id: string;
  name: string;
  list_type: 'blocklist' | 'allowlist';
  words: string[];
  case_sensitive: boolean;
  created_at: Date;
  updated_at: Date;
}

export interface OverrideRecord {
  id: string;
  source_account_id: string;
  evaluation_id: string;
  content_type: string;
  content_id: string | null;
  original_result: EvaluationResult;
  override_result: EvaluationResult;
  moderator_id: string;
  reason: string | null;
  created_at: Date;
}

// =============================================================================
// API Request/Response Types
// =============================================================================

export interface EvaluateRequest {
  content_type: string;
  content_text: string;
  content_id?: string;
  submitter_id?: string;
  policy_ids?: string[];
}

export interface EvaluateResponse {
  result: EvaluationResult;
  matched_rules: MatchedRule[];
  score: number;
  processing_time_ms: number;
  evaluation_id: string;
  message?: string;
}

export interface BatchEvaluateRequest {
  items: EvaluateRequest[];
}

export interface BatchEvaluateResponse {
  results: EvaluateResponse[];
  total: number;
  processed: number;
  failed: number;
}

export interface CreatePolicyRequest {
  name: string;
  description?: string;
  content_types?: string[];
  enabled?: boolean;
  priority?: number;
  mode?: 'enforce' | 'monitor' | 'disabled';
}

export interface UpdatePolicyRequest {
  name?: string;
  description?: string;
  content_types?: string[];
  enabled?: boolean;
  priority?: number;
  mode?: 'enforce' | 'monitor' | 'disabled';
}

export interface CreateRuleRequest {
  name: string;
  rule_type: RuleType;
  config: RuleConfig;
  action?: RuleAction;
  severity?: RuleSeverity;
  message?: string;
  enabled?: boolean;
}

export interface UpdateRuleRequest {
  name?: string;
  config?: RuleConfig;
  action?: RuleAction;
  severity?: RuleSeverity;
  message?: string;
  enabled?: boolean;
}

export interface CreateWordListRequest {
  name: string;
  list_type: 'blocklist' | 'allowlist';
  words: string[];
  case_sensitive?: boolean;
}

export interface UpdateWordListRequest {
  name?: string;
  words?: string[];
  add_words?: string[];
  remove_words?: string[];
  case_sensitive?: boolean;
}

export interface CreateOverrideRequest {
  evaluation_id: string;
  override_result: EvaluationResult;
  moderator_id: string;
  reason?: string;
}

export interface TestRuleRequest {
  content_text: string;
  rule_type: RuleType;
  config: RuleConfig;
}

export interface TestRuleResponse {
  matched: boolean;
  message?: string;
  matched_text?: string;
}

// =============================================================================
// Statistics
// =============================================================================

export interface PolicyStats {
  total_evaluations: number;
  allowed: number;
  denied: number;
  flagged: number;
  quarantined: number;
  override_count: number;
  avg_processing_time_ms: number;
  top_violations: Array<{
    rule_name: string;
    count: number;
  }>;
  evaluations_by_content_type: Array<{
    content_type: string;
    count: number;
  }>;
  evaluations_by_result: Array<{
    result: EvaluationResult;
    count: number;
  }>;
}

export interface QueueItem {
  evaluation_id: string;
  content_type: string;
  content_id: string | null;
  content_text: string | null;
  submitter_id: string | null;
  result: EvaluationResult;
  matched_rules: MatchedRule[];
  score: number;
  created_at: Date;
}

// =============================================================================
// Evaluation Engine Types
// =============================================================================

export interface EvaluationContext {
  content_type: string;
  content_text: string;
  content_id?: string;
  submitter_id?: string;
  policies: PolicyRecord[];
  word_lists: Map<string, WordListRecord>;
}

export interface RuleEvaluationResult {
  matched: boolean;
  rule: RuleRecord;
  message?: string;
  matched_text?: string;
}
