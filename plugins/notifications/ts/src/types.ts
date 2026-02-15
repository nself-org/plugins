/**
 * Notification Plugin Type Definitions
 */

// =============================================================================
// Core Types
// =============================================================================

export type NotificationChannel = 'email' | 'push' | 'sms';

export type NotificationCategory = 'transactional' | 'marketing' | 'system' | 'alert';

export type NotificationStatus =
  | 'pending'
  | 'queued'
  | 'sent'
  | 'delivered'
  | 'failed'
  | 'bounced';

export type QueueStatus = 'pending' | 'processing' | 'completed' | 'failed';

export type ProviderType = 'email' | 'push' | 'sms';

export type EmailProvider = 'resend' | 'sendgrid' | 'mailgun' | 'ses' | 'smtp';
export type PushProvider = 'fcm' | 'onesignal' | 'webpush';
export type SmsProvider = 'twilio' | 'plivo' | 'sns';

export type FrequencyType = 'immediate' | 'hourly' | 'daily' | 'weekly' | 'disabled';

export type BatchType = 'digest' | 'bulk' | 'scheduled';

// =============================================================================
// Statistics Types
// =============================================================================

export interface DeliveryStats {
  channel: NotificationChannel;
  category: NotificationCategory;
  date: Date;
  total: number;
  delivered: number;
  failed: number;
  bounced: number;
  delivery_rate: number;
}

export interface EngagementStats {
  channel: NotificationChannel;
  category: NotificationCategory;
  date: Date;
  total_sent: number;
  total_opened: number;
  total_clicked: number;
  open_rate: number;
  click_rate: number;
  click_to_open_rate: number;
}

// =============================================================================
// Template Types
// =============================================================================

export interface NotificationTemplate {
  id: string;
  source_account_id: string;
  name: string;
  category: NotificationCategory;
  channels: NotificationChannel[];
  subject?: string;
  body_text?: string;
  body_html?: string;
  push_title?: string;
  push_body?: string;
  sms_body?: string;
  metadata: Record<string, unknown>;
  variables: string[];
  active: boolean;
  created_at: Date;
  updated_at: Date;
  created_by?: string;
  updated_by?: string;
}

export interface TemplateVariables {
  [key: string]: unknown;
}

// =============================================================================
// Preference Types
// =============================================================================

export interface NotificationPreference {
  id: string;
  source_account_id: string;
  user_id: string;
  channel: NotificationChannel;
  category: NotificationCategory;
  enabled: boolean;
  frequency: FrequencyType;
  quiet_hours?: QuietHours;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface QuietHours {
  start: string; // HH:MM format
  end: string;   // HH:MM format
  timezone: string;
}

// =============================================================================
// Notification Types
// =============================================================================

export interface Notification {
  id: string;
  source_account_id: string;
  user_id: string;
  template_id?: string;
  template_name?: string;
  channel: NotificationChannel;
  category: NotificationCategory;
  status: NotificationStatus;
  priority: number;

  // Recipients
  recipient_email?: string;
  recipient_phone?: string;
  recipient_push_token?: string;
  recipient_user_id?: string;

  // Content
  subject?: string;
  body_text?: string;
  body_html?: string;

  // Delivery
  provider?: string;
  provider_message_id?: string;
  provider_response?: Record<string, unknown>;

  // Timing
  scheduled_at?: Date;
  sent_at?: Date;
  delivered_at?: Date;
  failed_at?: Date;

  // Engagement
  opened_at?: Date;
  clicked_at?: Date;
  unsubscribed_at?: Date;

  // Retries
  retry_count: number;
  max_retries: number;
  next_retry_at?: Date;

  // Errors
  error_message?: string;
  error_code?: string;
  error_details?: Record<string, unknown>;

  // Metadata
  metadata: Record<string, unknown>;
  tags: string[];
  batch_id?: string;

  created_at: Date;
  updated_at: Date;
}

export interface CreateNotificationInput {
  user_id: string;
  template_name?: string;
  channel: NotificationChannel;
  category: NotificationCategory;
  recipient_email?: string;
  recipient_phone?: string;
  recipient_push_token?: string;
  subject?: string;
  body_text?: string;
  body_html?: string;
  priority?: number;
  scheduled_at?: Date;
  metadata?: Record<string, unknown>;
  tags?: string[];
  variables?: TemplateVariables;
}

export interface SendNotificationResult {
  success: boolean;
  notification_id?: string;
  error?: string;
  provider_response?: unknown;
}

// =============================================================================
// Queue Types
// =============================================================================

export interface QueueItem {
  id: string;
  source_account_id: string;
  notification_id: string;
  status: QueueStatus;
  priority: number;
  attempts: number;
  max_attempts: number;
  next_attempt_at: Date;
  last_error?: string;
  processing_started_at?: Date;
  processing_completed_at?: Date;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Provider Types
// =============================================================================

export interface NotificationProvider {
  id: string;
  source_account_id: string;
  name: string;
  type: ProviderType;
  priority: number;
  enabled: boolean;
  config: ProviderConfig;
  rate_limit_per_second?: number;
  rate_limit_per_hour?: number;
  rate_limit_per_day?: number;
  success_count: number;
  failure_count: number;
  last_success_at?: Date;
  last_failure_at?: Date;
  health_status: 'healthy' | 'degraded' | 'unhealthy';
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface ProviderConfig {
  // Email configs
  api_key?: string;
  from?: string;
  domain?: string;
  region?: string;

  // SMTP configs
  host?: string;
  port?: number;
  secure?: boolean;
  user?: string;
  pass?: string;

  // Push configs
  server_key?: string;
  app_id?: string;
  project_id?: string;
  vapid_public_key?: string;
  vapid_private_key?: string;
  vapid_subject?: string;

  // SMS configs
  account_sid?: string;
  auth_token?: string;
  auth_id?: string;

  [key: string]: unknown;
}

export interface INotificationProvider {
  send(notification: Notification): Promise<SendNotificationResult>;
  getType(): ProviderType;
  getName(): string;
  isHealthy(): Promise<boolean>;
}

// =============================================================================
// Batch Types
// =============================================================================

export interface NotificationBatch {
  id: string;
  source_account_id: string;
  name?: string;
  category?: string;
  batch_type: BatchType;
  status: QueueStatus;
  interval_seconds: number;
  last_sent_at?: Date;
  next_send_at?: Date;
  total_notifications: number;
  sent_count: number;
  failed_count: number;
  config: Record<string, unknown>;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Statistics Types
// =============================================================================

export interface DeliveryRate {
  channel: NotificationChannel;
  category: NotificationCategory;
  date: Date;
  total: number;
  delivered: number;
  failed: number;
  bounced: number;
  delivery_rate: number;
}

export interface EngagementMetrics {
  channel: NotificationChannel;
  category: NotificationCategory;
  date: Date;
  delivered: number;
  opened: number;
  clicked: number;
  unsubscribed: number;
  open_rate: number;
  click_rate: number;
}

export interface ProviderHealth {
  name: string;
  type: ProviderType;
  enabled: boolean;
  health_status: string;
  success_count: number;
  failure_count: number;
  success_rate: number;
  last_success_at?: Date;
  last_failure_at?: Date;
}

// =============================================================================
// Configuration Types
// =============================================================================

export interface NotificationConfig {
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };
  email: {
    enabled: boolean;
    provider?: EmailProvider;
    from_address?: string;

    // SMTP config
    smtp_host?: string;
    smtp_port?: number;
    smtp_secure?: boolean;
    smtp_user?: string;
    smtp_password?: string;

    // SendGrid config
    sendgrid_api_key?: string;

    // Mailgun config
    mailgun_api_key?: string;
    mailgun_domain?: string;

    // AWS SES config
    ses_region?: string;

    // Resend config
    resend_api_key?: string;

    // Legacy fields for compatibility
    api_key?: string;
    from?: string;
    domain?: string;
    smtp?: {
      host: string;
      port: number;
      secure: boolean;
      user: string;
      pass: string;
    };
  };
  push: {
    enabled: boolean;
    provider?: PushProvider;

    // FCM (Firebase Cloud Messaging) config
    fcm_server_key?: string;
    fcm_service_account?: string;

    // APNs (Apple Push Notification service) config
    apns_key_id?: string;
    apns_key?: string;
    apns_team_id?: string;
    apns_production?: boolean;

    // Legacy fields for compatibility
    api_key?: string;
    app_id?: string;
    project_id?: string;
    vapid?: {
      public_key: string;
      private_key: string;
      subject: string;
    };
  };
  sms: {
    enabled: boolean;
    provider?: SmsProvider;

    // Twilio config
    twilio_account_sid?: string;
    twilio_auth_token?: string;
    twilio_from_number?: string;

    // Legacy fields for compatibility
    account_sid?: string;
    auth_token?: string;
    auth_id?: string;
    from?: string;
  };
  queue: {
    backend: 'redis' | 'postgres';
    redis_url?: string;
  };
  worker: {
    concurrency: number;
    poll_interval: number;
  };
  retry: {
    attempts: number;
    delay: number;
    max_delay: number;
  };
  rate_limits: {
    email: { per_user: number; window: number };
    push: { per_user: number; window: number };
    sms: { per_user: number; window: number };
  };
  batch: {
    enabled: boolean;
    interval: number;
  };
  server: {
    port: number;
    host: string;
  };
  features: {
    tracking_enabled: boolean;
    quiet_hours_enabled: boolean;
  };
  security: {
    encrypt_config: boolean;
    encryption_key?: string;
    webhook_secret?: string;
    webhook_verify: boolean;
  };
  development: {
    dry_run: boolean;
    test_mode: boolean;
    log_level: string;
  };
}

// =============================================================================
// Event Types
// =============================================================================

export interface NotificationEvent {
  type: 'delivery.succeeded' | 'delivery.failed' | 'bounce' | 'complaint' | 'open' | 'click' | 'unsubscribe';
  notification_id: string;
  provider: string;
  provider_message_id?: string;
  timestamp: Date;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// API Types
// =============================================================================

export interface SendNotificationRequest {
  user_id: string;
  template?: string;
  channel: NotificationChannel;
  category?: NotificationCategory;
  to: {
    email?: string;
    phone?: string;
    push_token?: string;
  };
  content?: {
    subject?: string;
    body?: string;
    html?: string;
  };
  variables?: TemplateVariables;
  priority?: number;
  scheduled_at?: string;
  metadata?: Record<string, unknown>;
  tags?: string[];
}

export interface SendNotificationResponse {
  success: boolean;
  notification_id?: string;
  error?: string;
  message?: string;
}

export interface GetNotificationResponse {
  notification: Notification;
}

export interface ListTemplatesResponse {
  templates: NotificationTemplate[];
  total: number;
}

export interface UpdatePreferencesRequest {
  user_id: string;
  channel: NotificationChannel;
  category: NotificationCategory;
  enabled: boolean;
  frequency?: FrequencyType;
  quiet_hours?: QuietHours;
}

export interface UpdatePreferencesResponse {
  success: boolean;
  preference: NotificationPreference;
}
