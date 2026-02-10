/**
 * Donorbox Plugin Types
 */

// ─── Configuration ───────────────────────────────────────────────────────────

export interface DonorboxAccountConfig {
  id: string;
  email: string;
  apiKey: string;
  webhookSecret: string;
}

export interface DonorboxConfig {
  email: string;
  apiKey: string;
  accounts: DonorboxAccountConfig[];
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

// ─── API Response Types ──────────────────────────────────────────────────────

export interface DonorboxCampaign {
  id: number;
  name: string;
  slug: string;
  currency: string;
  created_at: string;
  updated_at: string;
  goal_amount: string;
  total_raised: string;
  donations_count: number;
  formatted_goal_amount: string;
  formatted_total_raised: string;
  is_active: boolean;
}

export interface DonorboxDonor {
  id: number;
  created_at: string;
  updated_at: string;
  first_name: string;
  last_name: string;
  email: string;
  phone: string;
  address: string;
  city: string;
  state: string;
  zip_code: string;
  country: string;
  employer: string;
  comment: string;
  donations_count: number;
  last_donation_at: string;
  total: string;
}

export interface DonorboxDonation {
  id: number;
  campaign: { id: number; name: string };
  donor: { id: number; name: string; email: string; first_name: string; last_name: string };
  amount: string;
  formatted_amount: string;
  converted_amount: string;
  converted_net_amount: string;
  recurring: boolean;
  first_recurring_donation: boolean;
  amount_refunded: string;
  currency: string;
  donation_type: string;
  donation_date: string;
  processing_fee: string;
  status: string;
  comment: string;
  designation: string;
  join_mailing_list: boolean;
  stripe_charge_id: string | null;
  paypal_transaction_id: string | null;
  questions: Array<{ question: string; answer: string }>;
  created_at: string;
  updated_at: string;
}

export interface DonorboxPlan {
  id: number;
  campaign: { id: number; name: string };
  donor: { id: number; name: string; email: string };
  type: string;
  amount: string;
  currency: string;
  status: string;
  started_at: string;
  last_donation_date: string;
  next_donation_date: string;
  created_at: string;
  updated_at: string;
}

export interface DonorboxEvent {
  id: number;
  name: string;
  slug: string;
  description: string;
  start_date: string;
  end_date: string;
  timezone: string;
  venue_name: string;
  address: string;
  city: string;
  state: string;
  country: string;
  zip_code: string;
  currency: string;
  tickets_count: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface DonorboxTicket {
  id: number;
  event: { id: number; name: string };
  donor: { id: number; name: string; email: string };
  ticket_type: string;
  quantity: number;
  amount: string;
  currency: string;
  status: string;
  created_at: string;
  updated_at: string;
}

// ─── Database Record Types ───────────────────────────────────────────────────

export interface CampaignRecord {
  id: number;
  source_account_id: string;
  name: string;
  slug: string;
  currency: string;
  goal_amount: number | null;
  total_raised: number;
  donations_count: number;
  is_active: boolean;
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface DonorRecord {
  id: number;
  source_account_id: string;
  first_name: string;
  last_name: string;
  email: string;
  phone: string | null;
  address: string | null;
  city: string | null;
  state: string | null;
  zip_code: string | null;
  country: string | null;
  employer: string | null;
  donations_count: number;
  last_donation_at: Date | null;
  total: number;
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface DonationRecord {
  id: number;
  source_account_id: string;
  campaign_id: number | null;
  campaign_name: string | null;
  donor_id: number | null;
  donor_email: string | null;
  donor_name: string | null;
  amount: number;
  converted_amount: number | null;
  converted_net_amount: number | null;
  amount_refunded: number;
  currency: string;
  donation_type: string;
  donation_date: Date | null;
  processing_fee: number | null;
  status: string;
  recurring: boolean;
  comment: string | null;
  designation: string | null;
  stripe_charge_id: string | null;
  paypal_transaction_id: string | null;
  questions: Record<string, unknown>[];
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface PlanRecord {
  id: number;
  source_account_id: string;
  campaign_id: number | null;
  campaign_name: string | null;
  donor_id: number | null;
  donor_email: string | null;
  type: string;
  amount: number;
  currency: string;
  status: string;
  started_at: Date | null;
  last_donation_date: Date | null;
  next_donation_date: Date | null;
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface EventRecord {
  id: number;
  source_account_id: string;
  name: string;
  slug: string;
  description: string | null;
  start_date: Date | null;
  end_date: Date | null;
  timezone: string | null;
  venue_name: string | null;
  address: string | null;
  city: string | null;
  state: string | null;
  country: string | null;
  zip_code: string | null;
  currency: string;
  tickets_count: number;
  is_active: boolean;
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

export interface TicketRecord {
  id: number;
  source_account_id: string;
  event_id: number | null;
  event_name: string | null;
  donor_id: number | null;
  donor_email: string | null;
  ticket_type: string | null;
  quantity: number;
  amount: number;
  currency: string;
  status: string;
  created_at: Date | null;
  updated_at: Date | null;
  synced_at: Date;
}

// ─── Sync Types ──────────────────────────────────────────────────────────────

export interface SyncStats {
  campaigns: number;
  donors: number;
  donations: number;
  plans: number;
  events: number;
  tickets: number;
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
