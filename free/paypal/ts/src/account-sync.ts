/**
 * PayPal multi-account sync orchestration
 */

import { PayPalClient } from './client.js';
import { PayPalSyncService, type SyncResult } from './sync.js';
import { PayPalWebhookHandler } from './webhooks.js';
import type { PayPalAccountConfig, PayPalConfig, SyncOptions, SyncStats } from './types.js';
import { PayPalDatabase } from './database.js';
import { isSandbox } from './config.js';

export interface PayPalAccountContext {
  account: PayPalAccountConfig;
  db: PayPalDatabase;
  client: PayPalClient;
  syncService: PayPalSyncService;
  webhookHandler: PayPalWebhookHandler;
}

export interface PayPalAccountSyncResult {
  accountId: string;
  mode: 'SANDBOX' | 'LIVE';
  result: SyncResult;
}

export interface PayPalAggregateSyncResult {
  success: boolean;
  stats: SyncStats;
  errors: string[];
  duration: number;
  accounts: PayPalAccountSyncResult[];
}

function emptySyncStats(): SyncStats {
  return {
    transactions: 0, orders: 0, captures: 0, authorizations: 0,
    refunds: 0, subscriptions: 0, subscriptionPlans: 0, products: 0,
    disputes: 0, payouts: 0, invoices: 0, payers: 0, balances: 0,
    lastSyncedAt: null,
  };
}

function mergeSyncStats(target: SyncStats, source: SyncStats): void {
  target.transactions += source.transactions;
  target.orders += source.orders;
  target.captures += source.captures;
  target.authorizations += source.authorizations;
  target.refunds += source.refunds;
  target.subscriptions += source.subscriptions;
  target.subscriptionPlans += source.subscriptionPlans;
  target.products += source.products;
  target.disputes += source.disputes;
  target.payouts += source.payouts;
  target.invoices += source.invoices;
  target.payers += source.payers;
  target.balances += source.balances;

  if (source.lastSyncedAt) {
    if (!target.lastSyncedAt || source.lastSyncedAt > target.lastSyncedAt) {
      target.lastSyncedAt = source.lastSyncedAt;
    }
  }
}

export function createPayPalAccountContexts(config: PayPalConfig, db: PayPalDatabase): PayPalAccountContext[] {
  return config.accounts.map(account => {
    const accountDb = db.forSourceAccount(account.id);
    const client = new PayPalClient(account.clientId, account.clientSecret, config.environment);
    const syncService = new PayPalSyncService(client, accountDb);
    const webhookHandler = new PayPalWebhookHandler(client, accountDb, syncService);

    return { account, db: accountDb, client, syncService, webhookHandler };
  });
}

export async function runPayPalAccountReconcile(
  contexts: PayPalAccountContext[],
  config: PayPalConfig,
  lookbackDays = 7
): Promise<PayPalAggregateSyncResult> {
  const startedAt = Date.now();
  const aggregateStats = emptySyncStats();
  const aggregateErrors: string[] = [];
  const accountResults: PayPalAccountSyncResult[] = [];

  for (const context of contexts) {
    const result = await context.syncService.reconcile(lookbackDays);
    mergeSyncStats(aggregateStats, result.stats);
    aggregateErrors.push(...result.errors.map(e => `[${context.account.id}] ${e}`));
    accountResults.push({
      accountId: context.account.id,
      mode: isSandbox(config) ? 'SANDBOX' : 'LIVE',
      result,
    });
  }

  return {
    success: accountResults.every(a => a.result.success),
    stats: aggregateStats,
    errors: aggregateErrors,
    duration: Date.now() - startedAt,
    accounts: accountResults,
  };
}

export async function runPayPalAccountSync(
  contexts: PayPalAccountContext[],
  config: PayPalConfig,
  options: SyncOptions = {}
): Promise<PayPalAggregateSyncResult> {
  const startedAt = Date.now();
  const aggregateStats = emptySyncStats();
  const aggregateErrors: string[] = [];
  const accountResults: PayPalAccountSyncResult[] = [];

  for (const context of contexts) {
    const result = await context.syncService.sync(options);
    mergeSyncStats(aggregateStats, result.stats);
    aggregateErrors.push(...result.errors.map(e => `[${context.account.id}] ${e}`));
    accountResults.push({
      accountId: context.account.id,
      mode: isSandbox(config) ? 'SANDBOX' : 'LIVE',
      result,
    });
  }

  return {
    success: accountResults.every(a => a.result.success),
    stats: aggregateStats,
    errors: aggregateErrors,
    duration: Date.now() - startedAt,
    accounts: accountResults,
  };
}
