/**
 * Donorbox Sync Service
 * Full sync, incremental sync, reconciliation
 */

import { createLogger } from '@nself/plugin-utils';
import type { DonorboxClient } from './client.js';
import type { DonorboxDatabase } from './database.js';
import type {
  SyncResult,
  SyncStats,
  SyncOptions,
  CampaignRecord,
  DonorRecord,
  DonationRecord,
  PlanRecord,
  EventRecord,
  TicketRecord,
} from './types.js';

const logger = createLogger('donorbox:sync');

export { SyncResult };

export class DonorboxSyncService {
  constructor(
    private client: DonorboxClient,
    private db: DonorboxDatabase,
  ) {}

  // ─── Full Sync ─────────────────────────────────────────────────────────

  async sync(options: SyncOptions = {}): Promise<SyncResult> {
    const startedAt = Date.now();
    const stats = emptySyncStats();
    const errors: string[] = [];

    logger.info('Starting Donorbox sync', { incremental: options.incremental ?? false });

    const syncTasks: Array<{ name: string; key: keyof SyncStats; fn: () => Promise<number> }> = [
      { name: 'Campaigns', key: 'campaigns', fn: () => this.syncCampaigns() },
      { name: 'Donors', key: 'donors', fn: () => this.syncDonors() },
      { name: 'Donations', key: 'donations', fn: () => this.syncDonations(options) },
      { name: 'Plans', key: 'plans', fn: () => this.syncPlans() },
      { name: 'Events', key: 'events', fn: () => this.syncEvents() },
      { name: 'Tickets', key: 'tickets', fn: () => this.syncTickets() },
    ];

    for (const task of syncTasks) {
      try {
        logger.info(`Syncing ${task.name}...`);
        const count = await task.fn();
        (stats as unknown as Record<string, unknown>)[task.key] = count;
        logger.success(`${task.name}: ${count} records`);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        errors.push(`${task.name}: ${message}`);
        logger.error(`Failed to sync ${task.name}`, { error: message });
      }
    }

    stats.lastSyncedAt = new Date();
    const duration = Date.now() - startedAt;

    logger.info('Donorbox sync complete', { duration, errors: errors.length });

    return { success: errors.length === 0, stats, errors, duration };
  }

  // ─── Reconciliation ────────────────────────────────────────────────────

  async reconcile(lookbackDays = 7): Promise<SyncResult> {
    const startedAt = Date.now();
    const stats = emptySyncStats();
    const errors: string[] = [];

    const since = new Date(Date.now() - lookbackDays * 86400_000);
    const dateFrom = since.toISOString().split('T')[0]; // YYYY-MM-DD
    logger.info('Starting Donorbox reconciliation', { lookbackDays, dateFrom });

    const reconcileTasks: Array<{ name: string; key: keyof SyncStats; fn: () => Promise<number> }> = [
      {
        name: 'Donations',
        key: 'donations',
        fn: async () => {
          const items = await this.client.listAllDonations({ dateFrom });
          const records = items.map(d => mapDonation(d));
          return this.db.upsertDonations(records);
        },
      },
      {
        name: 'Donors',
        key: 'donors',
        fn: () => this.syncDonors(),
      },
    ];

    for (const task of reconcileTasks) {
      try {
        logger.info(`Reconciling ${task.name}...`);
        const count = await task.fn();
        (stats as unknown as Record<string, unknown>)[task.key] = count;
        logger.success(`${task.name}: ${count} records reconciled`);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        errors.push(`${task.name}: ${message}`);
        logger.error(`Failed to reconcile ${task.name}`, { error: message });
      }
    }

    stats.lastSyncedAt = new Date();
    const duration = Date.now() - startedAt;

    logger.info('Donorbox reconciliation complete', { duration, errors: errors.length });

    return { success: errors.length === 0, stats, errors, duration };
  }

  // ─── Individual Sync Methods ───────────────────────────────────────────

  private async syncCampaigns(): Promise<number> {
    const items = await this.client.listAllCampaigns();
    const records: CampaignRecord[] = items.map(c => ({
      id: c.id,
      source_account_id: 'primary',
      name: c.name,
      slug: c.slug,
      currency: c.currency ?? 'USD',
      goal_amount: c.goal_amount ? parseFloat(c.goal_amount) : null,
      total_raised: parseFloat(c.total_raised ?? '0'),
      donations_count: c.donations_count ?? 0,
      is_active: c.is_active ?? true,
      created_at: c.created_at ? new Date(c.created_at) : null,
      updated_at: c.updated_at ? new Date(c.updated_at) : null,
      synced_at: new Date(),
    }));
    return this.db.upsertCampaigns(records);
  }

  private async syncDonors(): Promise<number> {
    const items = await this.client.listAllDonors();
    const records: DonorRecord[] = items.map(d => ({
      id: d.id,
      source_account_id: 'primary',
      first_name: d.first_name ?? '',
      last_name: d.last_name ?? '',
      email: d.email ?? '',
      phone: d.phone || null,
      address: d.address || null,
      city: d.city || null,
      state: d.state || null,
      zip_code: d.zip_code || null,
      country: d.country || null,
      employer: d.employer || null,
      donations_count: d.donations_count ?? 0,
      last_donation_at: d.last_donation_at ? new Date(d.last_donation_at) : null,
      total: parseFloat(d.total ?? '0'),
      created_at: d.created_at ? new Date(d.created_at) : null,
      updated_at: d.updated_at ? new Date(d.updated_at) : null,
      synced_at: new Date(),
    }));
    return this.db.upsertDonors(records);
  }

  private async syncDonations(options: SyncOptions = {}): Promise<number> {
    const dateFrom = options.since
      ? options.since.toISOString().split('T')[0]
      : options.incremental
        ? new Date(Date.now() - 30 * 86400_000).toISOString().split('T')[0]
        : undefined;

    const items = await this.client.listAllDonations({ dateFrom });
    const records = items.map(d => mapDonation(d));
    return this.db.upsertDonations(records);
  }

  private async syncPlans(): Promise<number> {
    const items = await this.client.listAllPlans();
    const records: PlanRecord[] = items.map(p => ({
      id: p.id,
      source_account_id: 'primary',
      campaign_id: p.campaign?.id ?? null,
      campaign_name: p.campaign?.name ?? null,
      donor_id: p.donor?.id ?? null,
      donor_email: p.donor?.email ?? null,
      type: p.type ?? '',
      amount: parseFloat(p.amount ?? '0'),
      currency: p.currency ?? 'USD',
      status: p.status ?? '',
      started_at: p.started_at ? new Date(p.started_at) : null,
      last_donation_date: p.last_donation_date ? new Date(p.last_donation_date) : null,
      next_donation_date: p.next_donation_date ? new Date(p.next_donation_date) : null,
      created_at: p.created_at ? new Date(p.created_at) : null,
      updated_at: p.updated_at ? new Date(p.updated_at) : null,
      synced_at: new Date(),
    }));
    return this.db.upsertPlans(records);
  }

  private async syncEvents(): Promise<number> {
    const items = await this.client.listAllEvents();
    const records: EventRecord[] = items.map(e => ({
      id: e.id,
      source_account_id: 'primary',
      name: e.name,
      slug: e.slug,
      description: e.description || null,
      start_date: e.start_date ? new Date(e.start_date) : null,
      end_date: e.end_date ? new Date(e.end_date) : null,
      timezone: e.timezone || null,
      venue_name: e.venue_name || null,
      address: e.address || null,
      city: e.city || null,
      state: e.state || null,
      country: e.country || null,
      zip_code: e.zip_code || null,
      currency: e.currency ?? 'USD',
      tickets_count: e.tickets_count ?? 0,
      is_active: e.is_active ?? true,
      created_at: e.created_at ? new Date(e.created_at) : null,
      updated_at: e.updated_at ? new Date(e.updated_at) : null,
      synced_at: new Date(),
    }));
    return this.db.upsertEvents(records);
  }

  private async syncTickets(): Promise<number> {
    const items = await this.client.listAllTickets();
    const records: TicketRecord[] = items.map(t => ({
      id: t.id,
      source_account_id: 'primary',
      event_id: t.event?.id ?? null,
      event_name: t.event?.name ?? null,
      donor_id: t.donor?.id ?? null,
      donor_email: t.donor?.email ?? null,
      ticket_type: t.ticket_type || null,
      quantity: t.quantity ?? 0,
      amount: parseFloat(t.amount ?? '0'),
      currency: t.currency ?? 'USD',
      status: t.status ?? '',
      created_at: t.created_at ? new Date(t.created_at) : null,
      updated_at: t.updated_at ? new Date(t.updated_at) : null,
      synced_at: new Date(),
    }));
    return this.db.upsertTickets(records);
  }
}

// ─── Helpers ───────────────────────────────────────────────────────────────

function emptySyncStats(): SyncStats {
  return {
    campaigns: 0, donors: 0, donations: 0, plans: 0,
    events: 0, tickets: 0, lastSyncedAt: null,
  };
}

function mapDonation(d: import('./types.js').DonorboxDonation): DonationRecord {
  return {
    id: d.id,
    source_account_id: 'primary',
    campaign_id: d.campaign?.id ?? null,
    campaign_name: d.campaign?.name ?? null,
    donor_id: d.donor?.id ?? null,
    donor_email: d.donor?.email ?? null,
    donor_name: d.donor?.name ?? ([d.donor?.first_name, d.donor?.last_name].filter(Boolean).join(' ') || null),
    amount: parseFloat(d.amount ?? '0'),
    converted_amount: d.converted_amount ? parseFloat(d.converted_amount) : null,
    converted_net_amount: d.converted_net_amount ? parseFloat(d.converted_net_amount) : null,
    amount_refunded: parseFloat(d.amount_refunded ?? '0'),
    currency: d.currency ?? 'USD',
    donation_type: d.donation_type ?? '',
    donation_date: d.donation_date ? new Date(d.donation_date) : null,
    processing_fee: d.processing_fee ? parseFloat(d.processing_fee) : null,
    status: d.status ?? '',
    recurring: d.recurring ?? false,
    comment: d.comment || null,
    designation: d.designation || null,
    stripe_charge_id: d.stripe_charge_id || null,
    paypal_transaction_id: d.paypal_transaction_id || null,
    questions: d.questions as unknown as Record<string, unknown>[] ?? [],
    created_at: d.created_at ? new Date(d.created_at) : null,
    updated_at: d.updated_at ? new Date(d.updated_at) : null,
    synced_at: new Date(),
  };
}
