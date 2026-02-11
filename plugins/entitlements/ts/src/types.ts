/**
 * Entitlements Plugin Types
 * Complete type definitions for all entitlement objects
 */

// =============================================================================
// Enums / Union Types
// =============================================================================

export type BillingInterval = 'month' | 'year' | 'one_time' | 'usage';
export type PlanType = 'free' | 'standard' | 'enterprise' | 'custom' | 'addon';
export type SubscriptionStatus = 'trialing' | 'active' | 'past_due' | 'canceled' | 'unpaid' | 'expired' | 'paused';
export type PaymentProvider = 'stripe' | 'paddle' | 'paypal' | 'manual';
export type FeatureType = 'boolean' | 'limit' | 'list';
export type ResetInterval = 'none' | 'daily' | 'weekly' | 'monthly' | 'yearly' | 'billing_period';
export type AddonStatus = 'active' | 'canceled';
export type PauseCollection = 'void' | 'mark_uncollectible' | 'keep_as_draft';

export type EntitlementEventType =
  | 'subscription_created' | 'subscription_updated' | 'subscription_canceled'
  | 'subscription_renewed' | 'subscription_expired' | 'subscription_paused'
  | 'trial_started' | 'trial_ended' | 'trial_extended'
  | 'quota_exceeded' | 'quota_reset' | 'usage_tracked'
  | 'addon_added' | 'addon_removed'
  | 'grant_created' | 'grant_revoked' | 'grant_expired'
  | 'plan_upgraded' | 'plan_downgraded';

// =============================================================================
// Plan Types
// =============================================================================

export interface EntitlementPlanRecord {
  id: string;
  source_account_id: string;
  name: string;
  slug: string;
  description: string | null;
  billing_interval: BillingInterval;
  price_cents: number;
  currency: string;
  trial_days: number;
  trial_limits: Record<string, unknown> | null;
  plan_type: PlanType;
  is_public: boolean;
  is_archived: boolean;
  features: Record<string, unknown>;
  quotas: Record<string, unknown>;
  metadata: Record<string, unknown> | null;
  display_order: number;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreatePlanRequest {
  name: string;
  slug: string;
  description?: string;
  billing_interval: BillingInterval;
  price_cents: number;
  currency?: string;
  trial_days?: number;
  trial_limits?: Record<string, unknown>;
  plan_type: PlanType;
  is_public?: boolean;
  features: Record<string, unknown>;
  quotas: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  display_order?: number;
}

export interface UpdatePlanRequest {
  name?: string;
  description?: string;
  price_cents?: number;
  trial_days?: number;
  is_public?: boolean;
  features?: Record<string, unknown>;
  quotas?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  display_order?: number;
}

// =============================================================================
// Subscription Types
// =============================================================================

export interface EntitlementSubscriptionRecord {
  id: string;
  source_account_id: string;
  workspace_id: string | null;
  user_id: string | null;
  plan_id: string;
  status: SubscriptionStatus;
  billing_interval: BillingInterval;
  price_cents: number;
  currency: string;
  is_custom_pricing: boolean;
  custom_quotas: Record<string, unknown> | null;
  custom_features: Record<string, unknown> | null;
  payment_provider: PaymentProvider | null;
  payment_provider_subscription_id: string | null;
  payment_provider_customer_id: string | null;
  trial_start: Date | null;
  trial_end: Date | null;
  current_period_start: Date;
  current_period_end: Date;
  cancel_at_period_end: boolean;
  canceled_at: Date | null;
  cancellation_reason: string | null;
  pause_collection: PauseCollection | null;
  pause_start: Date | null;
  pause_end: Date | null;
  metadata: Record<string, unknown> | null;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateSubscriptionRequest {
  workspace_id?: string;
  user_id?: string;
  plan_id: string;
  billing_interval?: BillingInterval;
  custom_price_cents?: number;
  custom_quotas?: Record<string, unknown>;
  custom_features?: Record<string, unknown>;
  payment_provider?: PaymentProvider;
  payment_provider_subscription_id?: string;
  payment_provider_customer_id?: string;
  start_trial?: boolean;
  metadata?: Record<string, unknown>;
}

export interface UpdateSubscriptionRequest {
  custom_quotas?: Record<string, unknown>;
  custom_features?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Feature Types
// =============================================================================

export interface EntitlementFeatureRecord {
  id: string;
  source_account_id: string;
  key: string;
  name: string;
  description: string | null;
  feature_type: FeatureType;
  default_value: unknown;
  category: string | null;
  metadata: Record<string, unknown> | null;
  is_active: boolean;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateFeatureRequest {
  key: string;
  name: string;
  description?: string;
  feature_type: FeatureType;
  default_value?: unknown;
  category?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdateFeatureRequest {
  name?: string;
  description?: string;
  default_value?: unknown;
  category?: string;
  is_active?: boolean;
}

// =============================================================================
// Quota Types
// =============================================================================

export interface EntitlementQuotaRecord {
  id: string;
  source_account_id: string;
  workspace_id: string | null;
  user_id: string | null;
  subscription_id: string;
  quota_key: string;
  quota_name: string;
  limit_value: number | null;
  is_unlimited: boolean;
  current_usage: number;
  reset_interval: ResetInterval | null;
  last_reset_at: Date | null;
  next_reset_at: Date | null;
  metadata: Record<string, unknown> | null;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

// =============================================================================
// Usage Types
// =============================================================================

export interface EntitlementUsageRecord {
  id: string;
  source_account_id: string;
  workspace_id: string | null;
  user_id: string | null;
  quota_id: string;
  quota_key: string;
  usage_amount: number;
  resource_type: string | null;
  resource_id: string | null;
  metadata: Record<string, unknown> | null;
  created_at: Date;
  [key: string]: unknown;
}

export interface TrackUsageRequest {
  workspace_id?: string;
  user_id?: string;
  quota_key: string;
  usage_amount?: number;
  resource_type?: string;
  resource_id?: string;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Addon Types
// =============================================================================

export interface EntitlementAddonRecord {
  id: string;
  source_account_id: string;
  addon_plan_id: string;
  subscription_id: string;
  quantity: number;
  price_cents: number;
  currency: string;
  status: AddonStatus;
  current_period_start: Date;
  current_period_end: Date;
  payment_provider_item_id: string | null;
  metadata: Record<string, unknown> | null;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface AddAddonRequest {
  subscription_id: string;
  addon_plan_id: string;
  quantity?: number;
}

// =============================================================================
// Grant Types
// =============================================================================

export interface EntitlementGrantRecord {
  id: string;
  source_account_id: string;
  workspace_id: string | null;
  user_id: string | null;
  feature_key: string;
  feature_value: unknown;
  granted_by: string | null;
  grant_reason: string | null;
  expires_at: Date | null;
  is_active: boolean;
  metadata: Record<string, unknown> | null;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateGrantRequest {
  workspace_id?: string;
  user_id?: string;
  feature_key: string;
  feature_value: unknown;
  granted_by?: string;
  grant_reason?: string;
  expires_at?: string;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Event Types
// =============================================================================

export interface EntitlementEventRecord {
  id: string;
  source_account_id: string;
  event_type: EntitlementEventType;
  workspace_id: string | null;
  user_id: string | null;
  subscription_id: string | null;
  plan_id: string | null;
  event_data: Record<string, unknown> | null;
  actor_user_id: string | null;
  created_at: Date;
  [key: string]: unknown;
}

// =============================================================================
// Access Check Types
// =============================================================================

export interface FeatureAccessResult {
  has_access: boolean;
  value: unknown;
  source: 'grant' | 'subscription' | 'none';
}

export interface QuotaAvailabilityResult {
  available: boolean;
  reason?: string;
  current_usage?: number;
  limit_value?: number;
  requested?: number;
  remaining?: number;
  is_unlimited?: boolean;
}

export interface UsageTrackingResult {
  success: boolean;
  error?: string;
  usage_id?: string;
  new_usage?: number;
  limit_value?: number;
  remaining?: number;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface EntitlementStats {
  total_plans: number;
  active_subscriptions: number;
  trialing_subscriptions: number;
  total_features: number;
  total_grants: number;
  active_quotas: number;
  exceeded_quotas: number;
  total_events: number;
  mrr_cents: number;
}
