/**
 * PayPal Plugin Types
 */

// ─── Configuration ───────────────────────────────────────────────────────────

export interface PayPalAccountConfig {
  id: string;
  clientId: string;
  clientSecret: string;
  webhookId: string;
  webhookSecret: string;
}

export interface PayPalConfig {
  clientId: string;
  clientSecret: string;
  environment: 'sandbox' | 'live';
  accounts: PayPalAccountConfig[];
  port: number;
  host: string;
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;
  syncInterval: number;
  logLevel: string;
}

// ─── OAuth2 ──────────────────────────────────────────────────────────────────

export interface PayPalTokenResponse {
  scope: string;
  access_token: string;
  token_type: string;
  app_id: string;
  expires_in: number;
  nonce: string;
}

// ─── API Response Types ──────────────────────────────────────────────────────

export interface PayPalMoney {
  currency_code: string;
  value: string;
}

export interface PayPalLink {
  href: string;
  rel: string;
  method: string;
}

export interface PayPalName {
  given_name?: string;
  surname?: string;
  full_name?: string;
}

export interface PayPalAddress {
  address_line_1?: string;
  address_line_2?: string;
  admin_area_1?: string;
  admin_area_2?: string;
  postal_code?: string;
  country_code?: string;
}

export interface PayPalPayer {
  email_address?: string;
  payer_id?: string;
  name?: PayPalName;
  phone?: { phone_number?: { national_number?: string } };
  address?: PayPalAddress;
}

// ─── Transaction Search ──────────────────────────────────────────────────────

export interface PayPalTransactionInfo {
  transaction_id: string;
  transaction_event_code: string;
  transaction_initiation_date: string;
  transaction_updated_date: string;
  transaction_amount: PayPalMoney;
  fee_amount?: PayPalMoney;
  insurance_amount?: PayPalMoney;
  shipping_amount?: PayPalMoney;
  shipping_discount_amount?: PayPalMoney;
  transaction_status: string;
  transaction_subject?: string;
  transaction_note?: string;
  invoice_id?: string;
  custom_field?: string;
  protection_eligibility?: string;
}

export interface PayPalTransactionDetail {
  transaction_info: PayPalTransactionInfo;
  payer_info?: {
    account_id?: string;
    email_address?: string;
    address_status?: string;
    payer_status?: string;
    payer_name?: PayPalName;
  };
  shipping_info?: {
    name?: string;
    address?: PayPalAddress;
  };
  cart_info?: {
    item_details?: Array<{
      item_name?: string;
      item_quantity?: string;
      item_unit_price?: PayPalMoney;
      item_amount?: PayPalMoney;
    }>;
  };
}

export interface PayPalTransactionSearchResponse {
  transaction_details: PayPalTransactionDetail[];
  account_number: string;
  start_date: string;
  end_date: string;
  last_refreshed_datetime: string;
  page: number;
  total_items: number;
  total_pages: number;
  links: PayPalLink[];
}

// ─── Orders ──────────────────────────────────────────────────────────────────

export interface PayPalOrderPurchaseUnit {
  reference_id?: string;
  amount: {
    currency_code: string;
    value: string;
    breakdown?: {
      item_total?: PayPalMoney;
      shipping?: PayPalMoney;
      tax_total?: PayPalMoney;
      discount?: PayPalMoney;
    };
  };
  payee?: { email_address?: string; merchant_id?: string };
  description?: string;
  items?: Array<{
    name: string;
    quantity: string;
    unit_amount: PayPalMoney;
    description?: string;
    sku?: string;
    category?: string;
  }>;
  shipping?: {
    name?: { full_name?: string };
    address?: PayPalAddress;
  };
  payments?: {
    captures?: PayPalCapture[];
    authorizations?: PayPalAuthorization[];
    refunds?: PayPalRefund[];
  };
}

export interface PayPalOrder {
  id: string;
  status: string;
  intent: string;
  payer?: PayPalPayer;
  purchase_units: PayPalOrderPurchaseUnit[];
  create_time: string;
  update_time: string;
  links: PayPalLink[];
}

// ─── Captures ────────────────────────────────────────────────────────────────

export interface PayPalCapture {
  id: string;
  status: string;
  amount: PayPalMoney;
  final_capture?: boolean;
  seller_protection?: { status: string };
  seller_receivable_breakdown?: {
    gross_amount: PayPalMoney;
    paypal_fee: PayPalMoney;
    net_amount: PayPalMoney;
  };
  invoice_id?: string;
  custom_id?: string;
  create_time: string;
  update_time: string;
  links: PayPalLink[];
}

// ─── Authorizations ──────────────────────────────────────────────────────────

export interface PayPalAuthorization {
  id: string;
  status: string;
  amount: PayPalMoney;
  invoice_id?: string;
  custom_id?: string;
  seller_protection?: { status: string };
  expiration_time?: string;
  create_time: string;
  update_time: string;
  links: PayPalLink[];
}

// ─── Refunds ─────────────────────────────────────────────────────────────────

export interface PayPalRefund {
  id: string;
  status: string;
  amount: PayPalMoney;
  invoice_id?: string;
  note_to_payer?: string;
  seller_payable_breakdown?: {
    gross_amount: PayPalMoney;
    paypal_fee: PayPalMoney;
    net_amount: PayPalMoney;
    total_refunded_amount?: PayPalMoney;
  };
  create_time: string;
  update_time: string;
  links: PayPalLink[];
}

// ─── Subscriptions ───────────────────────────────────────────────────────────

export interface PayPalSubscriptionPlan {
  id: string;
  product_id: string;
  name: string;
  description?: string;
  status: string;
  billing_cycles: Array<{
    frequency: { interval_unit: string; interval_count: number };
    tenure_type: string;
    sequence: number;
    total_cycles?: number;
    pricing_scheme: {
      fixed_price: PayPalMoney;
    };
  }>;
  payment_preferences?: {
    auto_bill_outstanding?: boolean;
    setup_fee?: PayPalMoney;
    setup_fee_failure_action?: string;
    payment_failure_threshold?: number;
  };
  taxes?: { percentage: string; inclusive: boolean };
  create_time: string;
  update_time: string;
  links: PayPalLink[];
}

export interface PayPalSubscription {
  id: string;
  plan_id: string;
  status: string;
  status_update_time?: string;
  start_time: string;
  quantity?: string;
  shipping_amount?: PayPalMoney;
  subscriber?: {
    name?: PayPalName;
    email_address?: string;
    payer_id?: string;
    shipping_address?: {
      name?: { full_name?: string };
      address?: PayPalAddress;
    };
  };
  billing_info?: {
    outstanding_balance?: PayPalMoney;
    cycle_executions?: Array<{
      tenure_type: string;
      sequence: number;
      cycles_completed: number;
      cycles_remaining?: number;
      current_pricing_scheme_version?: number;
      total_cycles?: number;
    }>;
    last_payment?: {
      amount: PayPalMoney;
      time: string;
    };
    next_billing_time?: string;
    failed_payments_count?: number;
  };
  create_time: string;
  update_time: string;
  plan_overridden?: boolean;
  links: PayPalLink[];
}

// ─── Products ────────────────────────────────────────────────────────────────

export interface PayPalProduct {
  id: string;
  name: string;
  description?: string;
  type: string;
  category?: string;
  image_url?: string;
  home_url?: string;
  create_time: string;
  update_time: string;
  links: PayPalLink[];
}

// ─── Disputes ────────────────────────────────────────────────────────────────

export interface PayPalDispute {
  dispute_id: string;
  create_time: string;
  update_time: string;
  reason: string;
  status: string;
  dispute_amount: PayPalMoney;
  dispute_outcome?: {
    outcome_code: string;
    amount_refunded?: PayPalMoney;
  };
  disputed_transactions?: Array<{
    buyer_transaction_id?: string;
    seller_transaction_id?: string;
    create_time?: string;
    transaction_status?: string;
    gross_amount?: PayPalMoney;
    buyer?: { name?: string };
    seller?: { name?: string; email?: string; merchant_id?: string };
  }>;
  dispute_life_cycle_stage?: string;
  dispute_channel?: string;
  links: PayPalLink[];
}

// ─── Payouts ─────────────────────────────────────────────────────────────────

export interface PayPalPayout {
  batch_header: {
    payout_batch_id: string;
    batch_status: string;
    time_created?: string;
    time_completed?: string;
    sender_batch_header: {
      sender_batch_id?: string;
      email_subject?: string;
      email_message?: string;
    };
    amount?: PayPalMoney;
    fees?: PayPalMoney;
  };
  links: PayPalLink[];
}

// ─── Invoices ────────────────────────────────────────────────────────────────

export interface PayPalInvoice {
  id: string;
  status: string;
  detail: {
    invoice_number?: string;
    invoice_date?: string;
    currency_code: string;
    note?: string;
    term?: string;
    memo?: string;
    payment_term?: {
      term_type?: string;
      due_date?: string;
    };
  };
  invoicer?: {
    name?: PayPalName;
    email_address?: string;
    business_name?: string;
  };
  primary_recipients?: Array<{
    billing_info?: {
      name?: PayPalName;
      email_address?: string;
    };
  }>;
  amount?: {
    currency_code: string;
    value: string;
    breakdown?: {
      item_total?: PayPalMoney;
      discount?: { invoice_discount?: PayPalMoney };
      tax_total?: PayPalMoney;
      shipping?: PayPalMoney;
    };
  };
  due_amount?: PayPalMoney;
  payments?: {
    paid_amount?: PayPalMoney;
    transactions?: Array<{
      payment_id?: string;
      payment_date?: string;
      method: string;
      amount: PayPalMoney;
    }>;
  };
  links: PayPalLink[];
}

// ─── Webhook Events ──────────────────────────────────────────────────────────

export interface PayPalWebhookEvent {
  id: string;
  event_version: string;
  create_time: string;
  resource_type: string;
  event_type: string;
  summary: string;
  resource: Record<string, unknown>;
  links: PayPalLink[];
}

export interface PayPalWebhookVerifyRequest {
  auth_algo: string;
  cert_url: string;
  transmission_id: string;
  transmission_sig: string;
  transmission_time: string;
  webhook_id: string;
  webhook_event: PayPalWebhookEvent;
}

// ─── Paginated List Responses ────────────────────────────────────────────────

export interface PayPalListResponse<T> {
  items?: T[];
  total_items?: number;
  total_pages?: number;
  links: PayPalLink[];
}

export interface PayPalDisputeListResponse {
  items?: PayPalDispute[];
  links: PayPalLink[];
}

// ─── Database Record Types ───────────────────────────────────────────────────

export interface TransactionRecord {
  id: string;
  source_account_id: string;
  event_code: string;
  initiation_date: Date | null;
  updated_date: Date | null;
  amount: number;
  fee_amount: number | null;
  currency: string;
  status: string;
  subject: string | null;
  note: string | null;
  payer_email: string | null;
  payer_id: string | null;
  payer_name: string | null;
  invoice_id: string | null;
  custom_field: string | null;
  metadata: Record<string, unknown>;
  synced_at: Date;
}

export interface OrderRecord {
  id: string;
  source_account_id: string;
  status: string;
  intent: string;
  payer_email: string | null;
  payer_id: string | null;
  payer_name: string | null;
  total_amount: number;
  currency: string;
  description: string | null;
  metadata: Record<string, unknown>;
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface CaptureRecord {
  id: string;
  source_account_id: string;
  order_id: string | null;
  status: string;
  amount: number;
  currency: string;
  fee_amount: number | null;
  net_amount: number | null;
  final_capture: boolean;
  invoice_id: string | null;
  custom_id: string | null;
  seller_protection: string | null;
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface AuthorizationRecord {
  id: string;
  source_account_id: string;
  order_id: string | null;
  status: string;
  amount: number;
  currency: string;
  invoice_id: string | null;
  custom_id: string | null;
  seller_protection: string | null;
  expiration_time: Date | null;
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface RefundRecord {
  id: string;
  source_account_id: string;
  capture_id: string | null;
  status: string;
  amount: number;
  currency: string;
  fee_amount: number | null;
  net_amount: number | null;
  invoice_id: string | null;
  note_to_payer: string | null;
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface SubscriptionRecord {
  id: string;
  source_account_id: string;
  plan_id: string;
  status: string;
  subscriber_email: string | null;
  subscriber_payer_id: string | null;
  subscriber_name: string | null;
  start_time: Date | null;
  quantity: string | null;
  outstanding_balance: number | null;
  last_payment_amount: number | null;
  last_payment_time: Date | null;
  next_billing_time: Date | null;
  failed_payments_count: number;
  currency: string | null;
  metadata: Record<string, unknown>;
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface SubscriptionPlanRecord {
  id: string;
  source_account_id: string;
  product_id: string;
  name: string;
  description: string | null;
  status: string;
  billing_cycles: Record<string, unknown>[];
  payment_preferences: Record<string, unknown> | null;
  taxes: Record<string, unknown> | null;
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface ProductRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  type: string;
  category: string | null;
  image_url: string | null;
  home_url: string | null;
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface DisputeRecord {
  id: string;
  source_account_id: string;
  reason: string;
  status: string;
  amount: number;
  currency: string;
  outcome_code: string | null;
  refunded_amount: number | null;
  life_cycle_stage: string | null;
  channel: string | null;
  seller_transaction_id: string | null;
  buyer_transaction_id: string | null;
  metadata: Record<string, unknown>;
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface PayoutRecord {
  id: string;
  source_account_id: string;
  batch_status: string;
  sender_batch_id: string | null;
  email_subject: string | null;
  amount: number | null;
  currency: string | null;
  fees: number | null;
  time_created: Date | null;
  time_completed: Date | null;
  synced_at: Date;
}

export interface InvoiceRecord {
  id: string;
  source_account_id: string;
  status: string;
  invoice_number: string | null;
  invoice_date: string | null;
  currency: string;
  recipient_email: string | null;
  recipient_name: string | null;
  total_amount: number | null;
  due_amount: number | null;
  paid_amount: number | null;
  note: string | null;
  due_date: string | null;
  metadata: Record<string, unknown>;
  synced_at: Date;
}

export interface PayerRecord {
  id: string;
  source_account_id: string;
  email: string | null;
  name: string | null;
  given_name: string | null;
  surname: string | null;
  phone: string | null;
  country_code: string | null;
  first_seen: Date | null;
  last_seen: Date | null;
  total_amount: number;
  transaction_count: number;
  synced_at: Date;
}

export interface BalanceRecord {
  currency_code: string;
  source_account_id: string;
  total_balance: number | null;
  available_balance: number | null;
  withheld_balance: number | null;
  captured_at: Date;
  synced_at: Date;
}

// ─── Sync Types ──────────────────────────────────────────────────────────────

export interface SyncStats {
  transactions: number;
  orders: number;
  captures: number;
  authorizations: number;
  refunds: number;
  subscriptions: number;
  subscriptionPlans: number;
  products: number;
  disputes: number;
  payouts: number;
  invoices: number;
  payers: number;
  balances: number;
  lastSyncedAt: Date | null;
}

export interface SyncOptions {
  incremental?: boolean;
  since?: Date;
  resources?: string[];
}

export interface SyncResult {
  success: boolean;
  stats: SyncStats;
  errors: string[];
  duration: number;
}
