/**
 * Invitations Plugin Types
 * Complete type definitions for invitation management
 */

// =============================================================================
// Core Types
// =============================================================================

export type InvitationType =
  | 'app_signup'
  | 'family_join'
  | 'team_join'
  | 'event_attend'
  | 'share_access';

export type InvitationStatus =
  | 'pending'
  | 'sent'
  | 'delivered'
  | 'accepted'
  | 'declined'
  | 'expired'
  | 'revoked';

export type InvitationChannel = 'email' | 'sms' | 'link';

export type BulkSendStatus = 'pending' | 'processing' | 'completed' | 'failed';

// =============================================================================
// Database Records
// =============================================================================

export interface InvitationRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  type: InvitationType;
  inviter_id: string;
  invitee_email: string | null;
  invitee_phone: string | null;
  invitee_name: string | null;
  code: string;
  status: InvitationStatus;
  channel: InvitationChannel;
  message: string | null;
  role: string | null;
  resource_type: string | null;
  resource_id: string | null;
  expires_at: Date | null;
  sent_at: Date | null;
  delivered_at: Date | null;
  accepted_at: Date | null;
  accepted_by: string | null;
  declined_at: Date | null;
  revoked_at: Date | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface TemplateRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  name: string;
  type: InvitationType;
  channel: InvitationChannel;
  subject: string | null;
  body: string;
  variables: string[];
  enabled: boolean;
  created_at: Date;
  updated_at: Date;
}

export interface BulkSendRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  inviter_id: string;
  template_id: string | null;
  type: InvitationType;
  total_count: number;
  sent_count: number;
  failed_count: number;
  status: BulkSendStatus;
  invitees: BulkInvitee[];
  metadata: Record<string, unknown>;
  started_at: Date | null;
  completed_at: Date | null;
  created_at: Date;
}

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
// API Request/Response Types
// =============================================================================

export interface CreateInvitationRequest {
  type: InvitationType;
  inviter_id: string;
  invitee_email?: string;
  invitee_phone?: string;
  invitee_name?: string;
  channel?: InvitationChannel;
  message?: string;
  role?: string;
  resource_type?: string;
  resource_id?: string;
  expires_in_hours?: number;
  metadata?: Record<string, unknown>;
  send_immediately?: boolean;
}

export interface CreateInvitationResponse {
  id: string;
  code: string;
  invite_url: string;
  status: InvitationStatus;
  expires_at: Date | null;
  created_at: Date;
}

export interface ValidateInvitationResponse {
  valid: boolean;
  invitation?: {
    id: string;
    type: InvitationType;
    inviter_id: string;
    invitee_name: string | null;
    role: string | null;
    resource_type: string | null;
    resource_id: string | null;
    expires_at: Date | null;
    message: string | null;
  };
  error?: string;
}

export interface AcceptInvitationRequest {
  accepted_by: string;
  metadata?: Record<string, unknown>;
}

export interface AcceptInvitationResponse {
  id: string;
  type: InvitationType;
  inviter_id: string;
  role: string | null;
  resource_type: string | null;
  resource_id: string | null;
  accepted_at: Date;
  metadata: Record<string, unknown>;
}

export interface BulkInvitee {
  email?: string;
  phone?: string;
  name?: string;
  metadata?: Record<string, unknown>;
}

export interface CreateBulkSendRequest {
  inviter_id: string;
  type: InvitationType;
  template_id?: string;
  invitees: BulkInvitee[];
  role?: string;
  resource_type?: string;
  resource_id?: string;
  expires_in_hours?: number;
  metadata?: Record<string, unknown>;
}

export interface CreateBulkSendResponse {
  id: string;
  total_count: number;
  status: BulkSendStatus;
  created_at: Date;
}

export interface BulkSendStatusResponse {
  id: string;
  status: BulkSendStatus;
  total_count: number;
  sent_count: number;
  failed_count: number;
  started_at: Date | null;
  completed_at: Date | null;
  created_at: Date;
}

export interface CreateTemplateRequest {
  name: string;
  type: InvitationType;
  channel: InvitationChannel;
  subject?: string;
  body: string;
  variables?: string[];
  enabled?: boolean;
}

export interface UpdateTemplateRequest {
  name?: string;
  subject?: string;
  body?: string;
  variables?: string[];
  enabled?: boolean;
}

export interface InvitationStats {
  total: number;
  pending: number;
  sent: number;
  delivered: number;
  accepted: number;
  declined: number;
  expired: number;
  revoked: number;
  conversionRate: number;
  byType: Record<InvitationType, number>;
  byChannel: Record<InvitationChannel, number>;
}

// =============================================================================
// Configuration
// =============================================================================

export interface InvitationsPluginConfig {
  port: number;
  host: string;
  defaultExpiryHours: number;
  codeLength: number;
  maxBulkSize: number;
  acceptUrlTemplate: string;
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
// Webhook Event Types
// =============================================================================

export interface InvitationWebhookEvent {
  id: string;
  type: string;
  data: {
    invitation?: InvitationRecord;
    bulk_send?: BulkSendRecord;
    template?: TemplateRecord;
  };
  created_at: Date;
}
