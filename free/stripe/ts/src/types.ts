/**
 * Stripe Plugin Types
 * Complete type definitions for all synced Stripe objects
 */

import type Stripe from 'stripe';

export interface StripePluginConfig {
  apiKey: string;
  apiVersion?: string;
  webhookSecret?: string;
  port: number;
  host: string;
  syncInterval?: number;
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
// Core Objects
// =============================================================================

export interface StripeCustomerRecord {
  id: string;
  email: string | null;
  name: string | null;
  phone: string | null;
  description: string | null;
  currency: string | null;
  default_source: string | null;
  invoice_prefix: string | null;
  balance: number;
  delinquent: boolean;
  tax_exempt: string;
  metadata: Record<string, string>;
  address: Stripe.Address | null;
  shipping: Stripe.Customer.Shipping | null;
  created_at: Date;
  deleted_at: Date | null;
}

export interface StripeProductRecord {
  id: string;
  name: string;
  description: string | null;
  active: boolean;
  type: string;
  images: string[];
  metadata: Record<string, string>;
  attributes: string[];
  shippable: boolean | null;
  statement_descriptor: string | null;
  tax_code: string | null;
  unit_label: string | null;
  url: string | null;
  default_price_id: string | null;
  created_at: Date;
  deleted_at: Date | null;
}

export interface StripePriceRecord {
  id: string;
  product_id: string;
  active: boolean;
  currency: string;
  unit_amount: number | null;
  unit_amount_decimal: string | null;
  type: string;
  billing_scheme: string;
  recurring: Stripe.Price.Recurring | null;
  tiers: Stripe.Price.Tier[] | null;
  tiers_mode: string | null;
  transform_quantity: Stripe.Price.TransformQuantity | null;
  lookup_key: string | null;
  nickname: string | null;
  tax_behavior: string;
  metadata: Record<string, string>;
  created_at: Date;
  deleted_at: Date | null;
}

// =============================================================================
// Billing Objects
// =============================================================================

export interface StripeSubscriptionRecord {
  id: string;
  customer_id: string;
  status: string;
  current_period_start: Date;
  current_period_end: Date;
  cancel_at: Date | null;
  canceled_at: Date | null;
  cancel_at_period_end: boolean;
  ended_at: Date | null;
  trial_start: Date | null;
  trial_end: Date | null;
  collection_method: string;
  billing_cycle_anchor: Date;
  billing_thresholds: Stripe.Subscription.BillingThresholds | null;
  days_until_due: number | null;
  default_payment_method_id: string | null;
  default_source: string | null;
  discount: Stripe.Discount | null;
  items: StripeSubscriptionItem[];
  latest_invoice_id: string | null;
  pending_setup_intent: string | null;
  pending_update: Stripe.Subscription.PendingUpdate | null;
  schedule_id: string | null;
  start_date: Date;
  transfer_data: Stripe.Subscription.TransferData | null;
  application_fee_percent: number | null;
  automatic_tax: Stripe.Subscription.AutomaticTax;
  payment_settings: Stripe.Subscription.PaymentSettings;
  metadata: Record<string, string>;
  created_at: Date;
}

export interface StripeSubscriptionItem {
  id: string;
  price: {
    id: string;
    product: string;
    unit_amount: number | null;
    currency: string;
    recurring: Stripe.Price.Recurring | null;
  };
  quantity: number;
}

export interface StripeSubscriptionItemRecord {
  id: string;
  subscription_id: string;
  price_id: string;
  quantity: number;
  billing_thresholds: Stripe.SubscriptionItem.BillingThresholds | null;
  metadata: Record<string, string>;
  created_at: Date;
}

export interface StripeSubscriptionScheduleRecord {
  id: string;
  customer_id: string;
  subscription_id: string | null;
  status: string;
  current_phase: Record<string, unknown> | null;
  default_settings: Record<string, unknown>;
  end_behavior: string;
  phases: Record<string, unknown>[];
  released_at: Date | null;
  released_subscription: string | null;
  metadata: Record<string, string>;
  created_at: Date;
  canceled_at: Date | null;
  completed_at: Date | null;
}

export interface StripeInvoiceRecord {
  id: string;
  customer_id: string;
  subscription_id: string | null;
  status: string | null;
  collection_method: string | null;
  currency: string;
  amount_due: number;
  amount_paid: number;
  amount_remaining: number;
  subtotal: number;
  subtotal_excluding_tax: number | null;
  total: number;
  total_excluding_tax: number | null;
  tax: number | null;
  total_tax_amounts: Stripe.Invoice.TotalTaxAmount[];
  discount: Stripe.Discount | null;
  discounts: string[];
  account_country: string | null;
  account_name: string | null;
  billing_reason: string | null;
  number: string | null;
  receipt_number: string | null;
  statement_descriptor: string | null;
  description: string | null;
  footer: string | null;
  customer_email: string | null;
  customer_name: string | null;
  customer_address: Stripe.Address | null;
  customer_phone: string | null;
  customer_shipping: Stripe.Invoice.CustomerShipping | null;
  customer_tax_exempt: string | null;
  customer_tax_ids: Stripe.Invoice.CustomerTaxId[];
  default_payment_method_id: string | null;
  default_source: string | null;
  lines: StripeInvoiceLine[];
  hosted_invoice_url: string | null;
  invoice_pdf: string | null;
  payment_intent_id: string | null;
  charge_id: string | null;
  attempt_count: number;
  attempted: boolean;
  auto_advance: boolean | null;
  next_payment_attempt: Date | null;
  webhooks_delivered_at: Date | null;
  paid: boolean;
  paid_out_of_band: boolean;
  period_start: Date;
  period_end: Date;
  due_date: Date | null;
  effective_at: Date | null;
  finalized_at: Date | null;
  marked_uncollectible_at: Date | null;
  voided_at: Date | null;
  metadata: Record<string, string>;
  created_at: Date;
}

export interface StripeInvoiceLine {
  id: string;
  amount: number;
  currency: string;
  description: string | null;
  quantity: number | null;
  price: {
    id: string;
    product: string;
    unit_amount: number | null;
  } | null;
}

export interface StripeInvoiceItemRecord {
  id: string;
  customer_id: string;
  invoice_id: string | null;
  subscription_id: string | null;
  subscription_item_id: string | null;
  price_id: string | null;
  amount: number;
  currency: string;
  description: string | null;
  discountable: boolean;
  quantity: number;
  unit_amount: number | null;
  unit_amount_decimal: string | null;
  period_start: Date;
  period_end: Date;
  proration: boolean;
  metadata: Record<string, string>;
  created_at: Date;
}

export interface StripeCreditNoteRecord {
  id: string;
  invoice_id: string;
  customer_id: string;
  type: string;
  status: string;
  currency: string;
  amount: number;
  subtotal: number;
  subtotal_excluding_tax: number | null;
  total: number;
  total_excluding_tax: number | null;
  discount_amount: number;
  out_of_band_amount: number | null;
  reason: string | null;
  memo: string | null;
  number: string;
  pdf: string;
  voided_at: Date | null;
  metadata: Record<string, string>;
  created_at: Date;
}

// =============================================================================
// Payment Objects
// =============================================================================

export interface StripeChargeRecord {
  id: string;
  customer_id: string | null;
  payment_intent_id: string | null;
  invoice_id: string | null;
  amount: number;
  amount_captured: number;
  amount_refunded: number;
  currency: string;
  status: string;
  paid: boolean;
  captured: boolean;
  refunded: boolean;
  disputed: boolean;
  failure_code: string | null;
  failure_message: string | null;
  outcome: Record<string, unknown> | null;
  description: string | null;
  receipt_email: string | null;
  receipt_number: string | null;
  receipt_url: string | null;
  statement_descriptor: string | null;
  statement_descriptor_suffix: string | null;
  payment_method_id: string | null;
  payment_method_details: Record<string, unknown> | null;
  billing_details: Stripe.Charge.BillingDetails | null;
  shipping: Stripe.Charge.Shipping | null;
  fraud_details: Stripe.Charge.FraudDetails | null;
  balance_transaction_id: string | null;
  application_fee_id: string | null;
  application_fee_amount: number | null;
  transfer_id: string | null;
  transfer_group: string | null;
  on_behalf_of: string | null;
  source_transfer: string | null;
  metadata: Record<string, string>;
  created_at: Date;
}

export interface StripeRefundRecord {
  id: string;
  charge_id: string | null;
  payment_intent_id: string | null;
  amount: number;
  currency: string;
  status: string;
  reason: string | null;
  receipt_number: string | null;
  description: string | null;
  failure_balance_transaction: string | null;
  failure_reason: string | null;
  balance_transaction_id: string | null;
  source_transfer_reversal: string | null;
  transfer_reversal: string | null;
  metadata: Record<string, string>;
  created_at: Date;
}

export interface StripeDisputeRecord {
  id: string;
  charge_id: string;
  payment_intent_id: string | null;
  amount: number;
  currency: string;
  status: string;
  reason: string;
  is_charge_refundable: boolean;
  balance_transactions: string[];
  evidence: Record<string, unknown>;
  evidence_details: Record<string, unknown>;
  metadata: Record<string, string>;
  created_at: Date;
}

export interface StripePaymentIntentRecord {
  id: string;
  customer_id: string | null;
  invoice_id: string | null;
  amount: number;
  amount_capturable: number;
  amount_received: number;
  currency: string;
  status: string;
  capture_method: string;
  confirmation_method: string;
  payment_method_id: string | null;
  payment_method_types: string[];
  setup_future_usage: string | null;
  client_secret: string | null;
  description: string | null;
  receipt_email: string | null;
  statement_descriptor: string | null;
  statement_descriptor_suffix: string | null;
  shipping: Stripe.PaymentIntent.Shipping | null;
  application_fee_amount: number | null;
  transfer_data: Stripe.PaymentIntent.TransferData | null;
  transfer_group: string | null;
  on_behalf_of: string | null;
  cancellation_reason: string | null;
  canceled_at: Date | null;
  charges: Stripe.Charge[];
  last_payment_error: Stripe.PaymentIntent.LastPaymentError | null;
  next_action: Stripe.PaymentIntent.NextAction | null;
  processing: Stripe.PaymentIntent.Processing | null;
  review: string | null;
  automatic_payment_methods: Stripe.PaymentIntent.AutomaticPaymentMethods | null;
  metadata: Record<string, string>;
  created_at: Date;
}

export interface StripeSetupIntentRecord {
  id: string;
  customer_id: string | null;
  payment_method_id: string | null;
  status: string;
  usage: string;
  payment_method_types: string[];
  client_secret: string | null;
  description: string | null;
  cancellation_reason: string | null;
  last_setup_error: Record<string, unknown> | null;
  next_action: Record<string, unknown> | null;
  single_use_mandate: string | null;
  mandate: string | null;
  on_behalf_of: string | null;
  application: string | null;
  metadata: Record<string, string>;
  created_at: Date;
}

export interface StripePaymentMethodRecord {
  id: string;
  customer_id: string | null;
  type: string;
  billing_details: Stripe.PaymentMethod.BillingDetails;
  card: Stripe.PaymentMethod.Card | null;
  bank_account: Record<string, unknown> | null;
  sepa_debit: Stripe.PaymentMethod.SepaDebit | null;
  us_bank_account: Stripe.PaymentMethod.UsBankAccount | null;
  link: Stripe.PaymentMethod.Link | null;
  metadata: Record<string, string>;
  created_at: Date;
}

export interface StripeBalanceTransactionRecord {
  id: string;
  amount: number;
  currency: string;
  net: number;
  fee: number;
  fee_details: Stripe.BalanceTransaction.FeeDetail[];
  type: string;
  status: string;
  description: string | null;
  source: string | null;
  reporting_category: string;
  available_on: Date;
  created_at: Date;
}

// =============================================================================
// Checkout Objects
// =============================================================================

export interface StripeCheckoutSessionRecord {
  id: string;
  customer_id: string | null;
  customer_email: string | null;
  payment_intent_id: string | null;
  subscription_id: string | null;
  invoice_id: string | null;
  mode: string;
  status: string;
  payment_status: string;
  currency: string | null;
  amount_total: number | null;
  amount_subtotal: number | null;
  total_details: Record<string, unknown> | null;
  success_url: string | null;
  cancel_url: string | null;
  url: string | null;
  client_reference_id: string | null;
  customer_creation: string | null;
  billing_address_collection: string | null;
  shipping_address_collection: Record<string, unknown> | null;
  shipping_cost: Record<string, unknown> | null;
  shipping_details: Record<string, unknown> | null;
  custom_text: Record<string, unknown> | null;
  consent: Record<string, unknown> | null;
  consent_collection: Record<string, unknown> | null;
  expires_at: Date;
  livemode: boolean;
  locale: string | null;
  metadata: Record<string, string>;
  created_at: Date;
}

export interface StripeCheckoutSessionLineItemRecord {
  id: string;
  session_id: string;
  price_id: string | null;
  product_id: string | null;
  description: string | null;
  quantity: number | null;
  amount_total: number;
  amount_subtotal: number;
  amount_discount: number;
  amount_tax: number;
  currency: string;
}

// =============================================================================
// Discounts & Promotions
// =============================================================================

export interface StripeCouponRecord {
  id: string;
  name: string | null;
  amount_off: number | null;
  percent_off: number | null;
  currency: string | null;
  duration: string;
  duration_in_months: number | null;
  max_redemptions: number | null;
  times_redeemed: number;
  redeem_by: Date | null;
  valid: boolean;
  applies_to: { products: string[] } | null;
  metadata: Record<string, string>;
  created_at: Date;
  deleted_at: Date | null;
}

export interface StripePromotionCodeRecord {
  id: string;
  coupon_id: string;
  code: string;
  customer_id: string | null;
  active: boolean;
  max_redemptions: number | null;
  times_redeemed: number;
  expires_at: Date | null;
  restrictions: Record<string, unknown>;
  metadata: Record<string, string>;
  created_at: Date;
}

export interface StripeDiscountRecord {
  id: string;
  coupon_id: string;
  customer_id: string | null;
  subscription_id: string | null;
  invoice_id: string | null;
  invoice_item_id: string | null;
  promotion_code_id: string | null;
  checkout_session_id: string | null;
  start: Date;
  end: Date | null;
}

// =============================================================================
// Tax Objects
// =============================================================================

export interface StripeTaxIdRecord {
  id: string;
  customer_id: string;
  type: string;
  value: string;
  country: string | null;
  verification: Record<string, unknown> | null;
  created_at: Date;
}

export interface StripeTaxRateRecord {
  id: string;
  display_name: string;
  description: string | null;
  percentage: number;
  inclusive: boolean;
  active: boolean;
  country: string | null;
  state: string | null;
  jurisdiction: string | null;
  tax_type: string | null;
  metadata: Record<string, string>;
  created_at: Date;
}

// =============================================================================
// Webhook Events
// =============================================================================

export interface StripeWebhookEventRecord {
  id: string;
  type: string;
  api_version: string | null;
  data: Record<string, unknown>;
  object_type: string;
  object_id: string;
  request_id: string | null;
  request_idempotency_key: string | null;
  livemode: boolean;
  pending_webhooks: number;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  retry_count: number;
  created_at: Date;
  received_at: Date;
}

// =============================================================================
// Sync Types
// =============================================================================

export interface SyncStats {
  customers: number;
  products: number;
  prices: number;
  coupons: number;
  promotionCodes: number;
  subscriptions: number;
  subscriptionItems: number;
  subscriptionSchedules: number;
  invoices: number;
  invoiceItems: number;
  creditNotes: number;
  charges: number;
  refunds: number;
  disputes: number;
  paymentIntents: number;
  setupIntents: number;
  paymentMethods: number;
  balanceTransactions: number;
  checkoutSessions: number;
  taxIds: number;
  taxRates: number;
  lastSyncedAt?: Date | null;
}

export interface SyncOptions {
  incremental?: boolean;
  since?: Date;
  resources?: Array<
    | 'customers'
    | 'products'
    | 'prices'
    | 'coupons'
    | 'promotion_codes'
    | 'subscriptions'
    | 'subscription_items'
    | 'subscription_schedules'
    | 'invoices'
    | 'invoice_items'
    | 'credit_notes'
    | 'charges'
    | 'refunds'
    | 'disputes'
    | 'payment_intents'
    | 'setup_intents'
    | 'payment_methods'
    | 'balance_transactions'
    | 'checkout_sessions'
    | 'tax_ids'
    | 'tax_rates'
  >;
  limit?: number;
}
