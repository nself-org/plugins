/**
 * Bots Plugin Types
 * Complete type definitions for bot framework, commands, subscriptions, marketplace
 */

// =============================================================================
// Bot Types
// =============================================================================

export type BotType = 'custom' | 'integration' | 'official';

export interface BotRecord {
  id: string;
  source_account_id: string;
  name: string;
  username: string;
  description: string | null;
  avatar_url: string | null;
  bot_type: BotType;
  owner_id: string;
  workspace_id: string | null;
  token_hash: string;
  oauth_client_id: string | null;
  oauth_client_secret_encrypted: string | null;
  permissions: number;
  is_enabled: boolean;
  is_verified: boolean;
  is_public: boolean;
  category: string | null;
  tags: string[];
  website_url: string | null;
  support_url: string | null;
  privacy_policy_url: string | null;
  terms_of_service_url: string | null;
  install_count: number;
  message_count: number;
  command_count: number;
  rating_avg: number;
  rating_count: number;
  last_active_at: Date | null;
  last_message_at: Date | null;
  rate_limit_per_minute: number;
  rate_limit_per_hour: number;
  rate_limit_per_day: number;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateBotRequest {
  name: string;
  username: string;
  description?: string;
  avatarUrl?: string;
  botType?: BotType;
  ownerId: string;
  workspaceId?: string;
  permissions?: number;
  isPublic?: boolean;
  category?: string;
  tags?: string[];
  websiteUrl?: string;
  supportUrl?: string;
  privacyPolicyUrl?: string;
  termsOfServiceUrl?: string;
  rateLimitPerMinute?: number;
  rateLimitPerHour?: number;
  rateLimitPerDay?: number;
  metadata?: Record<string, unknown>;
}

export interface UpdateBotRequest {
  name?: string;
  description?: string;
  avatarUrl?: string;
  permissions?: number;
  isEnabled?: boolean;
  isVerified?: boolean;
  isPublic?: boolean;
  category?: string;
  tags?: string[];
  websiteUrl?: string;
  supportUrl?: string;
  privacyPolicyUrl?: string;
  termsOfServiceUrl?: string;
  rateLimitPerMinute?: number;
  rateLimitPerHour?: number;
  rateLimitPerDay?: number;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Bot Command Types
// =============================================================================

export type CommandType = 'message' | 'slash' | 'context_menu';
export type CommandScope = 'all' | 'dm' | 'channel';

export interface BotCommandRecord {
  id: string;
  source_account_id: string;
  bot_id: string;
  command: string;
  description: string;
  usage_hint: string | null;
  command_type: CommandType;
  scope: CommandScope;
  parameters: CommandParameter[];
  required_permissions: number;
  rate_limit_per_minute: number | null;
  rate_limit_per_hour: number | null;
  is_enabled: boolean;
  usage_count: number;
  last_used_at: Date | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CommandParameter {
  name: string;
  type: string;
  required?: boolean;
  description?: string;
  autocomplete?: boolean;
  choices?: Array<{ name: string; value: string }>;
}

export interface CreateCommandRequest {
  botId: string;
  command: string;
  description: string;
  usageHint?: string;
  commandType?: CommandType;
  scope?: CommandScope;
  parameters?: CommandParameter[];
  requiredPermissions?: number;
  rateLimitPerMinute?: number;
  rateLimitPerHour?: number;
  metadata?: Record<string, unknown>;
}

export interface UpdateCommandRequest {
  description?: string;
  usageHint?: string;
  scope?: CommandScope;
  parameters?: CommandParameter[];
  isEnabled?: boolean;
  requiredPermissions?: number;
  rateLimitPerMinute?: number;
  rateLimitPerHour?: number;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Subscription Types
// =============================================================================

export type DeliveryMode = 'webhook' | 'polling';

export interface BotSubscriptionRecord {
  id: string;
  source_account_id: string;
  bot_id: string;
  workspace_id: string | null;
  channel_id: string | null;
  event_type: string;
  filters: Record<string, unknown>;
  delivery_mode: DeliveryMode;
  webhook_url: string | null;
  webhook_secret: string | null;
  is_active: boolean;
  event_count: number;
  last_event_at: Date | null;
  failed_delivery_count: number;
  last_failure_at: Date | null;
  last_error_message: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateSubscriptionRequest {
  botId: string;
  eventType: string;
  workspaceId?: string;
  channelId?: string;
  filters?: Record<string, unknown>;
  deliveryMode?: DeliveryMode;
  webhookUrl?: string;
  webhookSecret?: string;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Installation Types
// =============================================================================

export type InstallationScope = 'workspace' | 'channel';

export interface BotInstallationRecord {
  id: string;
  source_account_id: string;
  bot_id: string;
  workspace_id: string;
  installed_by: string;
  scope: InstallationScope;
  channel_id: string | null;
  config: Record<string, unknown>;
  granted_permissions: number;
  is_active: boolean;
  oauth_access_token_encrypted: string | null;
  oauth_refresh_token_encrypted: string | null;
  oauth_expires_at: Date | null;
  oauth_scope: string | null;
  message_count: number;
  command_count: number;
  last_used_at: Date | null;
  installed_at: Date;
  updated_at: Date;
  uninstalled_at: Date | null;
  uninstalled_by: string | null;
}

export interface InstallBotRequest {
  botId: string;
  workspaceId: string;
  installedBy: string;
  scope?: InstallationScope;
  channelId?: string;
  config?: Record<string, unknown>;
  grantedPermissions: number;
}

// =============================================================================
// Bot Message Types
// =============================================================================

export type BotMessageType = 'text' | 'card' | 'form' | 'button_group' | 'embed';

export interface BotMessageRecord {
  id: string;
  source_account_id: string;
  bot_id: string;
  message_id: string;
  channel_id: string;
  message_type: BotMessageType;
  interaction_count: number;
  last_interaction_at: Date | null;
  metadata: Record<string, unknown>;
  created_at: Date;
}

export interface CreateBotMessageRequest {
  botId: string;
  channelId: string;
  content: string;
  messageType?: BotMessageType;
  metadata?: Record<string, unknown>;
}

export interface SendCardRequest {
  channelId: string;
  card: {
    title: string;
    description?: string;
    fields?: Array<{ name: string; value: string; inline?: boolean }>;
    image?: string;
    buttons?: Array<{ label: string; action: string; style?: string; value?: Record<string, unknown> }>;
  };
}

// =============================================================================
// Interaction Types
// =============================================================================

export type InteractionType = 'button_click' | 'form_submit' | 'menu_select' | 'modal_submit';

export interface BotInteractionRecord {
  id: string;
  source_account_id: string;
  bot_id: string;
  message_id: string;
  user_id: string;
  interaction_type: InteractionType;
  interaction_id: string;
  interaction_value: Record<string, unknown> | null;
  response_sent: boolean;
  response_message_id: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
}

export interface CreateInteractionRequest {
  botId: string;
  messageId: string;
  userId: string;
  interactionType: InteractionType;
  interactionId: string;
  interactionValue?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Review Types
// =============================================================================

export interface BotReviewRecord {
  id: string;
  source_account_id: string;
  bot_id: string;
  user_id: string;
  rating: number;
  title: string | null;
  comment: string | null;
  is_published: boolean;
  is_flagged: boolean;
  moderated_at: Date | null;
  moderated_by: string | null;
  moderation_reason: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateReviewRequest {
  botId: string;
  userId: string;
  rating: number;
  title?: string;
  comment?: string;
}

export interface UpdateReviewRequest {
  rating?: number;
  title?: string;
  comment?: string;
}

// =============================================================================
// API Key Types
// =============================================================================

export interface BotApiKeyRecord {
  id: string;
  source_account_id: string;
  bot_id: string;
  key_name: string;
  key_hash: string;
  key_prefix: string;
  permissions: number;
  scopes: string[];
  is_active: boolean;
  rate_limit_per_minute: number | null;
  rate_limit_per_hour: number | null;
  expires_at: Date | null;
  last_used_at: Date | null;
  use_count: number;
  revoked_at: Date | null;
  revoked_by: string | null;
  revoke_reason: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateApiKeyRequest {
  botId: string;
  keyName: string;
  permissions: number;
  scopes?: string[];
  rateLimitPerMinute?: number;
  rateLimitPerHour?: number;
  expiresAt?: string;
}

// =============================================================================
// Webhook Event Types
// =============================================================================

export interface BotsWebhookEventRecord {
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

export interface BotsStats {
  totalBots: number;
  enabledBots: number;
  publicBots: number;
  verifiedBots: number;
  totalCommands: number;
  totalSubscriptions: number;
  totalInstallations: number;
  activeInstallations: number;
  totalApiKeys: number;
}

// =============================================================================
// Marketplace Types
// =============================================================================

export interface MarketplaceQuery {
  category?: string;
  tags?: string[];
  verified?: boolean;
  search?: string;
  sort?: 'installs' | 'rating' | 'recent';
  limit?: number;
  offset?: number;
}
