/**
 * Content Moderation Plugin Types
 * All TypeScript interfaces for the content-moderation plugin
 */

// ============================================================================
// Database Record Types
// ============================================================================

export interface ModReviewRecord {
  id: string;
  source_account_id: string;
  content_type: string;
  content_id: string;
  content_source: string | null;
  content_text: string | null;
  content_url: string | null;
  author_id: string | null;
  status: string;
  auto_result: Record<string, unknown> | null;
  auto_action: string | null;
  auto_confidence: number | null;
  manual_result: string | null;
  manual_action: string | null;
  reviewer_id: string | null;
  reviewed_at: Date | null;
  reason: string | null;
  policy_violated: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface ModPolicyRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  content_types: string[];
  rules: Record<string, unknown>;
  auto_action: string;
  severity: string;
  active: boolean;
  created_at: Date;
  updated_at: Date;
}

export interface ModAppealRecord {
  id: string;
  source_account_id: string;
  review_id: string;
  appellant_id: string;
  reason: string;
  status: string;
  resolved_by: string | null;
  resolution: string | null;
  resolved_at: Date | null;
  created_at: Date;
}

export interface ModUserStrikeRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  review_id: string | null;
  strike_type: string;
  severity: string;
  reason: string | null;
  expires_at: Date | null;
  created_at: Date;
}

export interface ModWebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  retry_count: number;
  created_at: Date;
}

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface SubmitReviewRequest {
  contentType: string;
  contentId: string;
  contentSource?: string;
  contentText?: string;
  contentUrl?: string;
  authorId?: string;
  metadata?: Record<string, unknown>;
}

export interface SubmitReviewResponse {
  reviewId: string;
  status: string;
  autoResult: Record<string, unknown> | null;
  action: string;
}

export interface BatchReviewRequest {
  items: Array<{
    contentType: string;
    contentId: string;
    contentText?: string;
    contentUrl?: string;
    authorId?: string;
  }>;
}

export interface BatchReviewResponse {
  results: SubmitReviewResponse[];
  total: number;
}

export interface QueueQuery {
  status?: string;
  contentType?: string;
  limit?: string;
  offset?: string;
  sortBy?: string;
}

export interface ManualReviewRequest {
  manualAction: 'approve' | 'reject' | 'escalate';
  reason?: string;
  policyViolated?: string;
  reviewerId?: string;
}

export interface ReviewListQuery {
  authorId?: string;
  status?: string;
  contentType?: string;
  from?: string;
  to?: string;
  limit?: string;
  offset?: string;
}

export interface SubmitAppealRequest {
  reviewId: string;
  reason: string;
  appellantId?: string;
}

export interface ResolveAppealRequest {
  status: 'upheld' | 'overturned';
  resolution: string;
  resolvedBy?: string;
}

export interface AppealListQuery {
  status?: string;
  limit?: string;
  offset?: string;
}

export interface CreatePolicyRequest {
  name: string;
  description?: string;
  contentTypes?: string[];
  rules: Record<string, unknown>;
  autoAction?: string;
  severity?: string;
  active?: boolean;
}

export interface UpdatePolicyRequest {
  name?: string;
  description?: string;
  contentTypes?: string[];
  rules?: Record<string, unknown>;
  autoAction?: string;
  severity?: string;
  active?: boolean;
}

export interface AddStrikeRequest {
  strikeType: string;
  severity?: string;
  reason?: string;
  reviewId?: string;
  expiresAt?: string;
}

export interface UserStatusResponse {
  userId: string;
  totalStrikes: number;
  activeStrikes: number;
  isBanned: boolean;
  restrictions: string[];
  strikes: ModUserStrikeRecord[];
}

export interface StatsQuery {
  from?: string;
  to?: string;
}

export interface ModerationStats {
  totalReviews: number;
  autoApproved: number;
  autoRejected: number;
  flagged: number;
  pendingManual: number;
  manualReviewed: number;
  appeals: number;
  pendingAppeals: number;
  totalStrikes: number;
}

// ============================================================================
// Configuration Types
// ============================================================================

export interface ContentModerationConfig {
  port: number;
  host: string;
  logLevel: 'debug' | 'info' | 'warn' | 'error';

  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };

  appIds: string[];

  // Provider
  provider: string;
  openaiApiKey: string;
  googleVisionKey: string;
  awsRekognitionKey: string;
  awsRekognitionSecret: string;
  awsRekognitionRegion: string;

  // Thresholds
  autoApproveBelow: number;
  autoRejectAbove: number;
  flagThreshold: number;

  // Strikes
  strikeWarnThreshold: number;
  strikeBanThreshold: number;
  strikeExpiryDays: number;

  // Queue
  reviewSlaHours: number;
  queueWorkerConcurrency: number;
}

// ============================================================================
// Stats & Health Types
// ============================================================================

export interface HealthCheckResponse {
  status: 'ok' | 'error';
  plugin: string;
  timestamp: string;
  version: string;
}
