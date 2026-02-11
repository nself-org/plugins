/**
 * Support Plugin Types
 * Complete type definitions for helpdesk, ticketing, SLA, knowledge base
 */

// =============================================================================
// Ticket Types
// =============================================================================

export type TicketStatus = 'new' | 'open' | 'pending' | 'resolved' | 'closed' | 'on_hold';
export type TicketPriority = 'low' | 'medium' | 'high' | 'urgent';
export type TicketSource = 'chat' | 'email' | 'api' | 'web_form';

export interface SupportTicketRecord {
  id: string;
  source_account_id: string;
  ticket_number: string;
  customer_id: string | null;
  customer_name: string | null;
  customer_email: string | null;
  customer_phone: string | null;
  subject: string;
  description: string;
  status: TicketStatus;
  priority: TicketPriority;
  assigned_to: string | null;
  assigned_at: Date | null;
  team_id: string | null;
  channel_id: string | null;
  source: TicketSource;
  category: string | null;
  tags: string[];
  sla_policy_id: string | null;
  first_response_due_at: Date | null;
  first_response_at: Date | null;
  resolution_due_at: Date | null;
  resolved_at: Date | null;
  first_response_breached: boolean;
  resolution_breached: boolean;
  satisfaction_rating: number | null;
  satisfaction_comment: string | null;
  satisfaction_submitted_at: Date | null;
  custom_fields: Record<string, unknown>;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
  closed_at: Date | null;
}

export interface CreateTicketRequest {
  subject: string;
  description: string;
  priority?: TicketPriority;
  source?: TicketSource;
  customerId?: string;
  customerName?: string;
  customerEmail?: string;
  customerPhone?: string;
  category?: string;
  tags?: string[];
  assignedTo?: string;
  teamId?: string;
  channelId?: string;
  slaPolicyId?: string;
  customFields?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface UpdateTicketRequest {
  subject?: string;
  description?: string;
  status?: TicketStatus;
  priority?: TicketPriority;
  assignedTo?: string | null;
  teamId?: string | null;
  category?: string;
  tags?: string[];
  slaPolicyId?: string;
  customFields?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface TicketListOptions {
  status?: TicketStatus;
  priority?: TicketPriority;
  assignedTo?: string;
  teamId?: string;
  customerId?: string;
  tags?: string[];
  search?: string;
  sort?: string;
  limit?: number;
  offset?: number;
}

// =============================================================================
// Team Types
// =============================================================================

export type AssignmentMethod = 'round_robin' | 'load_balanced' | 'skill_based';

export interface SupportTeamRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  email: string | null;
  is_active: boolean;
  business_hours: Record<string, unknown> | null;
  timezone: string;
  auto_assignment_enabled: boolean;
  assignment_method: AssignmentMethod;
  default_sla_policy_id: string | null;
  open_tickets_count: number;
  member_count: number;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateTeamRequest {
  name: string;
  description?: string;
  email?: string;
  timezone?: string;
  autoAssignmentEnabled?: boolean;
  assignmentMethod?: AssignmentMethod;
  defaultSlaPolicyId?: string;
  businessHours?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface UpdateTeamRequest {
  name?: string;
  description?: string;
  email?: string;
  isActive?: boolean;
  timezone?: string;
  autoAssignmentEnabled?: boolean;
  assignmentMethod?: AssignmentMethod;
  defaultSlaPolicyId?: string;
  businessHours?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Team Member Types
// =============================================================================

export type MemberRole = 'agent' | 'lead' | 'manager';
export type AvailabilityStatus = 'available' | 'busy' | 'away' | 'offline';

export interface SupportTeamMemberRecord {
  id: string;
  source_account_id: string;
  team_id: string;
  user_id: string;
  role: MemberRole;
  skills: string[];
  skill_level: number;
  max_concurrent_tickets: number;
  current_ticket_count: number;
  is_active: boolean;
  is_available: boolean;
  availability_status: AvailabilityStatus;
  total_tickets_handled: number;
  avg_first_response_time_seconds: number | null;
  avg_resolution_time_seconds: number | null;
  customer_satisfaction_avg: number | null;
  joined_at: Date;
  updated_at: Date;
}

export interface CreateTeamMemberRequest {
  teamId: string;
  userId: string;
  role?: MemberRole;
  skills?: string[];
  skillLevel?: number;
  maxConcurrentTickets?: number;
}

export interface UpdateTeamMemberRequest {
  role?: MemberRole;
  skills?: string[];
  skillLevel?: number;
  maxConcurrentTickets?: number;
  isActive?: boolean;
  isAvailable?: boolean;
  availabilityStatus?: AvailabilityStatus;
}

// =============================================================================
// SLA Policy Types
// =============================================================================

export interface SlaPolicyRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  urgent_first_response_minutes: number;
  urgent_resolution_minutes: number;
  high_first_response_minutes: number;
  high_resolution_minutes: number;
  medium_first_response_minutes: number;
  medium_resolution_minutes: number;
  low_first_response_minutes: number;
  low_resolution_minutes: number;
  applies_during_business_hours_only: boolean;
  business_hours: Record<string, unknown> | null;
  timezone: string;
  escalation_enabled: boolean;
  escalation_threshold_minutes: number;
  escalate_to_team_id: string | null;
  is_active: boolean;
  is_default: boolean;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateSlaPolicyRequest {
  name: string;
  description?: string;
  urgentFirstResponseMinutes?: number;
  urgentResolutionMinutes?: number;
  highFirstResponseMinutes?: number;
  highResolutionMinutes?: number;
  mediumFirstResponseMinutes?: number;
  mediumResolutionMinutes?: number;
  lowFirstResponseMinutes?: number;
  lowResolutionMinutes?: number;
  appliesDuringBusinessHoursOnly?: boolean;
  businessHours?: Record<string, unknown>;
  timezone?: string;
  escalationEnabled?: boolean;
  escalationThresholdMinutes?: number;
  escalateToTeamId?: string;
  isDefault?: boolean;
  metadata?: Record<string, unknown>;
}

export interface UpdateSlaPolicyRequest {
  name?: string;
  description?: string;
  urgentFirstResponseMinutes?: number;
  urgentResolutionMinutes?: number;
  highFirstResponseMinutes?: number;
  highResolutionMinutes?: number;
  mediumFirstResponseMinutes?: number;
  mediumResolutionMinutes?: number;
  lowFirstResponseMinutes?: number;
  lowResolutionMinutes?: number;
  appliesDuringBusinessHoursOnly?: boolean;
  businessHours?: Record<string, unknown>;
  timezone?: string;
  escalationEnabled?: boolean;
  escalationThresholdMinutes?: number;
  escalateToTeamId?: string;
  isActive?: boolean;
  isDefault?: boolean;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Canned Response Types
// =============================================================================

export type ResponseVisibility = 'personal' | 'team' | 'global';

export interface CannedResponseRecord {
  id: string;
  source_account_id: string;
  title: string;
  shortcut: string | null;
  content: string;
  category: string | null;
  tags: string[];
  visibility: ResponseVisibility;
  team_id: string | null;
  created_by: string;
  attachments: Record<string, unknown>[];
  usage_count: number;
  last_used_at: Date | null;
  is_active: boolean;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateCannedResponseRequest {
  title: string;
  shortcut?: string;
  content: string;
  category?: string;
  tags?: string[];
  visibility?: ResponseVisibility;
  teamId?: string;
  createdBy: string;
  attachments?: Record<string, unknown>[];
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Knowledge Base Article Types
// =============================================================================

export interface KbArticleRecord {
  id: string;
  source_account_id: string;
  title: string;
  slug: string;
  content: string;
  summary: string | null;
  author_id: string;
  category: string | null;
  tags: string[];
  is_published: boolean;
  is_public: boolean;
  meta_title: string | null;
  meta_description: string | null;
  attachments: Record<string, unknown>[];
  related_articles: string[];
  view_count: number;
  helpful_count: number;
  not_helpful_count: number;
  version: number;
  previous_version_id: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
  published_at: Date | null;
}

export interface CreateKbArticleRequest {
  title: string;
  slug?: string;
  content: string;
  summary?: string;
  authorId: string;
  category?: string;
  tags?: string[];
  isPublic?: boolean;
  metaTitle?: string;
  metaDescription?: string;
  attachments?: Record<string, unknown>[];
  relatedArticles?: string[];
  metadata?: Record<string, unknown>;
}

export interface UpdateKbArticleRequest {
  title?: string;
  content?: string;
  summary?: string;
  category?: string;
  tags?: string[];
  isPublished?: boolean;
  isPublic?: boolean;
  metaTitle?: string;
  metaDescription?: string;
  attachments?: Record<string, unknown>[];
  relatedArticles?: string[];
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Ticket Message Types
// =============================================================================

export interface TicketMessageRecord {
  id: string;
  source_account_id: string;
  ticket_id: string;
  user_id: string | null;
  content: string;
  is_internal: boolean;
  is_system: boolean;
  attachments: Record<string, unknown>[];
  email_message_id: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateTicketMessageRequest {
  ticketId: string;
  userId?: string;
  content: string;
  isInternal?: boolean;
  isSystem?: boolean;
  attachments?: Record<string, unknown>[];
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Ticket Event Types
// =============================================================================

export interface TicketEventRecord {
  id: string;
  source_account_id: string;
  ticket_id: string;
  user_id: string | null;
  event_type: string;
  field_name: string | null;
  old_value: string | null;
  new_value: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
}

// =============================================================================
// Analytics Types
// =============================================================================

export interface SupportAnalytics {
  openTickets: number;
  avgFirstResponseTime: number;
  avgResolutionTime: number;
  slaCompliance: number;
  customerSatisfaction: number;
  ticketsByStatus: Record<string, number>;
  ticketsByPriority: Record<string, number>;
}

export interface AgentPerformance {
  userId: string;
  ticketsHandled: number;
  avgFirstResponseTime: number;
  avgResolutionTime: number;
  satisfactionAvg: number;
  currentTickets: number;
}

// =============================================================================
// Webhook Event Types
// =============================================================================

export interface SupportWebhookEventRecord {
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
// Stats Types
// =============================================================================

export interface SupportStats {
  totalTickets: number;
  openTickets: number;
  pendingTickets: number;
  resolvedTickets: number;
  totalTeams: number;
  totalAgents: number;
  totalSlaPolicies: number;
  totalCannedResponses: number;
  totalKbArticles: number;
  publishedKbArticles: number;
}
