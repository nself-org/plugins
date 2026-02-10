/**
 * PayPal Sync Service
 * Full sync, incremental sync, reconciliation
 */

import { createLogger } from '@nself/plugin-utils';
import type { PayPalClient } from './client.js';
import type { PayPalDatabase } from './database.js';
import type {
  SyncResult,
  SyncStats,
  SyncOptions,
  TransactionRecord,
  ProductRecord,
  SubscriptionPlanRecord,
  DisputeRecord,
  InvoiceRecord,
  PayPalTransactionDetail,
} from './types.js';

const logger = createLogger('paypal:sync');

export { SyncResult };

export class PayPalSyncService {
  constructor(
    private client: PayPalClient,
    private db: PayPalDatabase,
  ) {}

  // ─── Full Sync ─────────────────────────────────────────────────────────

  async sync(options: SyncOptions = {}): Promise<SyncResult> {
    const startedAt = Date.now();
    const stats = emptySyncStats();
    const errors: string[] = [];

    logger.info('Starting PayPal sync', { incremental: options.incremental ?? false });

    const syncTasks: Array<{ name: string; key: keyof SyncStats; fn: () => Promise<number> }> = [
      { name: 'Products', key: 'products', fn: () => this.syncProducts() },
      { name: 'Subscription Plans', key: 'subscriptionPlans', fn: () => this.syncSubscriptionPlans() },
      { name: 'Transactions', key: 'transactions', fn: () => this.syncTransactions(options) },
      { name: 'Disputes', key: 'disputes', fn: () => this.syncDisputes() },
      { name: 'Invoices', key: 'invoices', fn: () => this.syncInvoices() },
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

    logger.info('PayPal sync complete', { duration, errors: errors.length });

    return {
      success: errors.length === 0,
      stats,
      errors,
      duration,
    };
  }

  // ─── Reconciliation ────────────────────────────────────────────────────

  async reconcile(lookbackDays = 7): Promise<SyncResult> {
    const startedAt = Date.now();
    const stats = emptySyncStats();
    const errors: string[] = [];

    const since = new Date(Date.now() - lookbackDays * 86400_000);
    logger.info('Starting PayPal reconciliation', { lookbackDays, since: since.toISOString() });

    const reconcileTasks: Array<{ name: string; key: keyof SyncStats; fn: () => Promise<number> }> = [
      {
        name: 'Transactions',
        key: 'transactions',
        fn: async () => {
          const items = await this.client.listAllTransactions({ startDate: since });
          const records = items.map(t => mapTransactionDetail(t));
          return this.db.upsertTransactions(records);
        },
      },
      {
        name: 'Disputes',
        key: 'disputes',
        fn: () => this.syncDisputes(since.toISOString()),
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

    logger.info('PayPal reconciliation complete', { duration, errors: errors.length });

    return {
      success: errors.length === 0,
      stats,
      errors,
      duration,
    };
  }

  // ─── Individual Sync Methods ───────────────────────────────────────────

  private async syncTransactions(options: SyncOptions = {}): Promise<number> {
    const startDate = options.since ?? options.incremental
      ? new Date(Date.now() - 30 * 86400_000) // Last 30 days for incremental
      : undefined;

    const items = await this.client.listAllTransactions({ startDate: startDate ?? undefined });
    const records = items.map(t => mapTransactionDetail(t));
    return this.db.upsertTransactions(records);
  }

  private async syncProducts(): Promise<number> {
    const items = await this.client.listAllProducts();
    const records: ProductRecord[] = items.map(p => ({
      id: p.id,
      source_account_id: 'primary',
      name: p.name,
      description: p.description ?? null,
      type: p.type,
      category: p.category ?? null,
      image_url: p.image_url ?? null,
      home_url: p.home_url ?? null,
      created_at: p.create_time ? new Date(p.create_time) : null,
      updated_at: p.update_time ? new Date(p.update_time) : null,
      synced_at: new Date(),
    }));
    return this.db.upsertProducts(records);
  }

  private async syncSubscriptionPlans(): Promise<number> {
    const items = await this.client.listAllSubscriptionPlans();
    const records: SubscriptionPlanRecord[] = items.map(p => ({
      id: p.id,
      source_account_id: 'primary',
      product_id: p.product_id,
      name: p.name,
      description: p.description ?? null,
      status: p.status,
      billing_cycles: p.billing_cycles as unknown as Record<string, unknown>[],
      payment_preferences: p.payment_preferences as unknown as Record<string, unknown> | null ?? null,
      taxes: p.taxes as unknown as Record<string, unknown> | null ?? null,
      created_at: p.create_time ? new Date(p.create_time) : null,
      updated_at: p.update_time ? new Date(p.update_time) : null,
      synced_at: new Date(),
    }));
    return this.db.upsertSubscriptionPlans(records);
  }

  private async syncDisputes(startDate?: string): Promise<number> {
    const items = await this.client.listAllDisputes({ startDate });
    const records: DisputeRecord[] = items.map(d => ({
      id: d.dispute_id,
      source_account_id: 'primary',
      reason: d.reason,
      status: d.status,
      amount: parseFloat(d.dispute_amount?.value ?? '0'),
      currency: d.dispute_amount?.currency_code ?? 'USD',
      outcome_code: d.dispute_outcome?.outcome_code ?? null,
      refunded_amount: d.dispute_outcome?.amount_refunded ? parseFloat(d.dispute_outcome.amount_refunded.value) : null,
      life_cycle_stage: d.dispute_life_cycle_stage ?? null,
      channel: d.dispute_channel ?? null,
      seller_transaction_id: d.disputed_transactions?.[0]?.seller_transaction_id ?? null,
      buyer_transaction_id: d.disputed_transactions?.[0]?.buyer_transaction_id ?? null,
      metadata: {},
      created_at: d.create_time ? new Date(d.create_time) : null,
      updated_at: d.update_time ? new Date(d.update_time) : null,
      synced_at: new Date(),
    }));
    return this.db.upsertDisputes(records);
  }

  private async syncInvoices(): Promise<number> {
    const items = await this.client.listAllInvoices();
    const records: InvoiceRecord[] = items.map(inv => ({
      id: inv.id,
      source_account_id: 'primary',
      status: inv.status,
      invoice_number: inv.detail?.invoice_number ?? null,
      invoice_date: inv.detail?.invoice_date ?? null,
      currency: inv.detail?.currency_code ?? inv.amount?.currency_code ?? 'USD',
      recipient_email: inv.primary_recipients?.[0]?.billing_info?.email_address ?? null,
      recipient_name: inv.primary_recipients?.[0]?.billing_info?.name?.full_name ??
        ([inv.primary_recipients?.[0]?.billing_info?.name?.given_name, inv.primary_recipients?.[0]?.billing_info?.name?.surname].filter(Boolean).join(' ') || null),
      total_amount: inv.amount ? parseFloat(inv.amount.value) : null,
      due_amount: inv.due_amount ? parseFloat(inv.due_amount.value) : null,
      paid_amount: inv.payments?.paid_amount ? parseFloat(inv.payments.paid_amount.value) : null,
      note: inv.detail?.note ?? null,
      due_date: inv.detail?.payment_term?.due_date ?? null,
      metadata: {},
      synced_at: new Date(),
    }));
    return this.db.upsertInvoices(records);
  }
}

// ─── Helpers ───────────────────────────────────────────────────────────────

function emptySyncStats(): SyncStats {
  return {
    transactions: 0, orders: 0, captures: 0, authorizations: 0,
    refunds: 0, subscriptions: 0, subscriptionPlans: 0, products: 0,
    disputes: 0, payouts: 0, invoices: 0, payers: 0, balances: 0,
    lastSyncedAt: null,
  };
}

function mapTransactionDetail(detail: PayPalTransactionDetail): TransactionRecord {
  const info = detail.transaction_info;
  const payer = detail.payer_info;

  return {
    id: info.transaction_id,
    source_account_id: 'primary',
    event_code: info.transaction_event_code ?? '',
    initiation_date: info.transaction_initiation_date ? new Date(info.transaction_initiation_date) : null,
    updated_date: info.transaction_updated_date ? new Date(info.transaction_updated_date) : null,
    amount: parseFloat(info.transaction_amount?.value ?? '0'),
    fee_amount: info.fee_amount ? parseFloat(info.fee_amount.value) : null,
    currency: info.transaction_amount?.currency_code ?? 'USD',
    status: info.transaction_status ?? '',
    subject: info.transaction_subject ?? null,
    note: info.transaction_note ?? null,
    payer_email: payer?.email_address ?? null,
    payer_id: payer?.account_id ?? null,
    payer_name: payer?.payer_name
      ? [payer.payer_name.given_name, payer.payer_name.surname].filter(Boolean).join(' ') || null
      : null,
    invoice_id: info.invoice_id ?? null,
    custom_field: info.custom_field ?? null,
    metadata: {},
    synced_at: new Date(),
  };
}
