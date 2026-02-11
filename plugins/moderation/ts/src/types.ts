/**
 * Moderation Plugin Types
 * Complete type definitions for content moderation operations
 */

// =============================================================================
// Enum Types
// =============================================================================

export type ModerationActionType = 'warn' | 'mute' | 'kick' | 'ban' | 'delete_message' | 'flag' | 'approve' | 'reject';

export type ModerationStatus = 'pending' | 'approved' | 'rejected' | 'auto_approved' | 'auto_rejected';

export type AppealStatus = 'pending' | 'approved' | 'rejected' | 'cancelled';

export type SeverityLevel = 'low' | 'medium' | 'high' | 'critical';

export type FilterType = 'profanity' | 'toxicity' | 'spam' | 'link' | 'mention' | 'custom';

export type ToxicityProvider = 'perspective_api' | 'openai' | 'local';

// =============================================================================
// Rule Types
// =============================================================================

export interface RuleConditions {
  words?: string[];
  case_sensitive?: boolean;
  wordlist_ids?: string[];
  min_severity?: SeverityLevel;
  toxicity_threshold?: number;
  categories?: string[];
  pattern?: string;
  regex?: boolean;
}

export interface RuleAction {
  type: ModerationActionType;
  immediate?: boolean;
  message?: string;
  for_review?: boolean;
  duration_minutes?: number;
}

export interface ThresholdConfig {
  occurrences?: number;
  timeframe_minutes?: number;
  escalation_action?: ModerationActionType;
}

export interface ModerationRuleRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  filter_type: FilterType;
  severity: SeverityLevel;
  is_enabled: boolean;
  conditions: RuleConditions;
  actions: RuleAction[];
  threshold_config: ThresholdConfig;
  channel_id: string | null;
  created_by: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface CreateRuleRequest {
  name: string;
  description?: string;
  filter_type: FilterType;
  severity: SeverityLevel;
  conditions: RuleConditions;
  actions: RuleAction[];
  threshold_config?: ThresholdConfig;
  channel_id?: string;
}

export interface UpdateRuleRequest {
  name?: string;
  description?: string;
  severity?: SeverityLevel;
  is_enabled?: boolean;
  conditions?: RuleConditions;
  actions?: RuleAction[];
  threshold_config?: ThresholdConfig;
}

// =============================================================================
// Wordlist Types
// =============================================================================

export interface WordlistRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  language: string;
  category: string | null;
  words: string[];
  is_regex: boolean;
  case_sensitive: boolean;
  is_enabled: boolean;
  severity: SeverityLevel;
  created_by: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface CreateWordlistRequest {
  name: string;
  description?: string;
  language?: string;
  category?: string;
  words: string[];
  is_regex?: boolean;
  case_sensitive?: boolean;
  severity?: SeverityLevel;
}

export interface UpdateWordlistRequest {
  name?: string;
  description?: string;
  language?: string;
  category?: string;
  words?: string[];
  is_regex?: boolean;
  case_sensitive?: boolean;
  is_enabled?: boolean;
  severity?: SeverityLevel;
}

// =============================================================================
// Action Types
// =============================================================================

export interface ModerationActionRecord {
  id: string;
  source_account_id: string;
  target_user_id: string;
  target_message_id: string | null;
  target_channel_id: string | null;
  action_type: ModerationActionType;
  severity: SeverityLevel;
  reason: string;
  duration_minutes: number | null;
  expires_at: Date | null;
  triggered_by_rule_id: string | null;
  is_automated: boolean;
  moderator_id: string | null;
  moderator_notes: string | null;
  metadata: Record<string, unknown>;
  is_active: boolean;
  revoked_at: Date | null;
  revoked_by: string | null;
  revoke_reason: string | null;
  created_at: Date;
}

export interface CreateActionRequest {
  user_id: string;
  action_type: ModerationActionType;
  reason: string;
  severity?: SeverityLevel;
  duration_minutes?: number;
  moderator_id?: string;
  moderator_notes?: string;
  target_message_id?: string;
  target_channel_id?: string;
  triggered_by_rule_id?: string;
  is_automated?: boolean;
  metadata?: Record<string, unknown>;
}

export interface RevokeActionRequest {
  revoke_reason: string;
  revoked_by?: string;
}

// =============================================================================
// Flag Types
// =============================================================================

export interface ModerationFlagRecord {
  id: string;
  source_account_id: string;
  content_type: string;
  content_id: string;
  content_snapshot: Record<string, unknown> | null;
  flag_reason: string;
  flag_category: string | null;
  severity: SeverityLevel;
  flagged_by_user_id: string | null;
  flagged_by_rule_id: string | null;
  is_automated: boolean;
  status: ModerationStatus;
  reviewed_by: string | null;
  reviewed_at: Date | null;
  review_notes: string | null;
  action_id: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateFlagRequest {
  content_type: string;
  content_id: string;
  flag_reason: string;
  flag_category?: string;
  severity?: SeverityLevel;
  content_snapshot?: Record<string, unknown>;
  flagged_by_user_id?: string;
  flagged_by_rule_id?: string;
  is_automated?: boolean;
}

export interface ReviewFlagRequest {
  status: 'approved' | 'rejected';
  review_notes?: string;
  reviewed_by?: string;
  action?: {
    type: ModerationActionType;
    duration_minutes?: number;
  };
}

// =============================================================================
// Appeal Types
// =============================================================================

export interface ModerationAppealRecord {
  id: string;
  source_account_id: string;
  action_id: string;
  appellant_user_id: string;
  appeal_reason: string;
  supporting_evidence: Record<string, unknown>;
  status: AppealStatus;
  reviewed_by: string | null;
  reviewed_at: Date | null;
  review_decision: string | null;
  was_successful: boolean | null;
  new_action_id: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface CreateAppealRequest {
  action_id: string;
  appellant_user_id: string;
  appeal_reason: string;
  supporting_evidence?: Record<string, unknown>;
}

export interface ReviewAppealRequest {
  status: 'approved' | 'rejected';
  review_decision: string;
  reviewed_by?: string;
}

// =============================================================================
// Report Types
// =============================================================================

export interface ModerationReportRecord {
  id: string;
  source_account_id: string;
  reporter_id: string;
  content_type: string;
  content_id: string;
  report_category: string;
  report_reason: string;
  additional_context: string | null;
  status: ModerationStatus;
  assigned_to: string | null;
  flag_id: string | null;
  action_id: string | null;
  resolution_notes: string | null;
  resolved_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface CreateReportRequest {
  reporter_id: string;
  content_type: string;
  content_id: string;
  report_category: string;
  report_reason: string;
  additional_context?: string;
}

// =============================================================================
// Toxicity Types
// =============================================================================

export interface ToxicityScoreRecord {
  id: string;
  source_account_id: string;
  content_type: string;
  content_id: string;
  content_hash: string | null;
  overall_score: number;
  category_scores: Record<string, number>;
  provider: ToxicityProvider;
  model_version: string | null;
  analyzed_at: Date;
  metadata: Record<string, unknown>;
}

export interface ToxicityCategoryScores {
  toxicity?: number;
  severe_toxicity?: number;
  obscene?: number;
  threat?: number;
  insult?: number;
  identity_attack?: number;
}

// =============================================================================
// User Stats Types
// =============================================================================

export interface UserStatsRecord {
  user_id: string;
  source_account_id: string;
  total_warnings: number;
  total_mutes: number;
  total_bans: number;
  total_flags: number;
  total_reports_filed: number;
  total_reports_against: number;
  average_toxicity_score: number | null;
  toxicity_trend: number | null;
  risk_level: SeverityLevel;
  risk_score: number;
  is_muted: boolean;
  muted_until: Date | null;
  is_banned: boolean;
  banned_until: Date | null;
  first_violation_at: Date | null;
  last_violation_at: Date | null;
  last_calculated_at: Date;
}

// =============================================================================
// Audit Log Types
// =============================================================================

export interface AuditLogRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  event_category: string;
  actor_id: string | null;
  actor_type: string;
  target_type: string | null;
  target_id: string | null;
  details: Record<string, unknown>;
  ip_address: string | null;
  user_agent: string | null;
  created_at: Date;
}

export interface CreateAuditLogRequest {
  event_type: string;
  event_category: string;
  actor_id?: string;
  actor_type?: string;
  target_type?: string;
  target_id?: string;
  details?: Record<string, unknown>;
  ip_address?: string;
  user_agent?: string;
}

// =============================================================================
// Content Analysis Types
// =============================================================================

export interface AnalyzeContentRequest {
  content: string;
  content_type: string;
  channel_id?: string;
}

export interface MatchedRule {
  rule_id: string;
  rule_name: string;
  severity: SeverityLevel;
  matched_words?: string[];
}

export interface SuggestedAction {
  type: ModerationActionType;
  reason: string;
}

export interface AnalyzeContentResponse {
  is_safe: boolean;
  toxicity_score: number;
  matched_rules: MatchedRule[];
  suggested_actions: SuggestedAction[];
}

export interface CheckProfanityRequest {
  content: string;
  language?: string;
}

export interface CheckProfanityResponse {
  contains_profanity: boolean;
  matched_words: string[];
  severity: SeverityLevel;
}

// =============================================================================
// Queue / Stats Types
// =============================================================================

export interface QueueItem {
  id: string;
  content_type: string;
  content_id: string;
  flag_reason: string;
  flag_category: string | null;
  severity: SeverityLevel;
  status: ModerationStatus;
  created_at: Date;
  is_automated: boolean;
}

export interface ModerationOverviewStats {
  total_actions: number;
  total_flags: number;
  total_reports: number;
  actions_by_type: Record<string, number>;
  flags_by_severity: Record<string, number>;
  average_toxicity_score: number;
}
