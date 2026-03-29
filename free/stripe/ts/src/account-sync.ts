/**
 * Stripe multi-account sync orchestration.
 */

import { StripeClient } from './client.js';
import { StripeSyncService, type SyncResult } from './sync.js';
import { StripeWebhookHandler } from './webhooks.js';
import type { Config, StripeAccountConfig } from './config.js';
import type { SyncOptions, SyncStats } from './types.js';
import { StripeDatabase } from './database.js';
import { isTestMode } from './config.js';

export interface StripeAccountContext {
  account: StripeAccountConfig;
  db: StripeDatabase;
  client: StripeClient;
  syncService: StripeSyncService;
  webhookHandler: StripeWebhookHandler;
}

export interface StripeAccountSyncResult {
  accountId: string;
  mode: 'TEST' | 'LIVE';
  result: SyncResult;
}

export interface StripeAggregateSyncResult {
  success: boolean;
  stats: SyncStats;
  errors: string[];
  duration: number;
  accounts: StripeAccountSyncResult[];
}

function emptySyncStats(): SyncStats {
  return {
    customers: 0,
    products: 0,
    prices: 0,
    coupons: 0,
    promotionCodes: 0,
    subscriptions: 0,
    subscriptionItems: 0,
    subscriptionSchedules: 0,
    invoices: 0,
    invoiceItems: 0,
    creditNotes: 0,
    charges: 0,
    refunds: 0,
    disputes: 0,
    paymentIntents: 0,
    setupIntents: 0,
    paymentMethods: 0,
    balanceTransactions: 0,
    checkoutSessions: 0,
    taxIds: 0,
    taxRates: 0,
    lastSyncedAt: null,
  };
}

function mergeSyncStats(target: SyncStats, source: SyncStats): void {
  target.customers += source.customers;
  target.products += source.products;
  target.prices += source.prices;
  target.coupons += source.coupons;
  target.promotionCodes += source.promotionCodes;
  target.subscriptions += source.subscriptions;
  target.subscriptionItems += source.subscriptionItems;
  target.subscriptionSchedules += source.subscriptionSchedules;
  target.invoices += source.invoices;
  target.invoiceItems += source.invoiceItems;
  target.creditNotes += source.creditNotes;
  target.charges += source.charges;
  target.refunds += source.refunds;
  target.disputes += source.disputes;
  target.paymentIntents += source.paymentIntents;
  target.setupIntents += source.setupIntents;
  target.paymentMethods += source.paymentMethods;
  target.balanceTransactions += source.balanceTransactions;
  target.checkoutSessions += source.checkoutSessions;
  target.taxIds += source.taxIds;
  target.taxRates += source.taxRates;

  if (source.lastSyncedAt) {
    if (!target.lastSyncedAt || source.lastSyncedAt > target.lastSyncedAt) {
      target.lastSyncedAt = source.lastSyncedAt;
    }
  }
}

export function createStripeAccountContexts(config: Config, db: StripeDatabase): StripeAccountContext[] {
  return config.stripeAccounts.map(account => {
    const accountDb = db.forSourceAccount(account.id);
    const client = new StripeClient(account.apiKey, config.stripeApiVersion);
    const syncService = new StripeSyncService(client, accountDb);
    const webhookHandler = new StripeWebhookHandler(client, accountDb, syncService);

    return {
      account,
      db: accountDb,
      client,
      syncService,
      webhookHandler,
    };
  });
}

export async function runStripeAccountReconcile(
  contexts: StripeAccountContext[],
  lookbackDays = 7
): Promise<StripeAggregateSyncResult> {
  const startedAt = Date.now();
  const aggregateStats = emptySyncStats();
  const aggregateErrors: string[] = [];
  const accountResults: StripeAccountSyncResult[] = [];

  for (const context of contexts) {
    const result = await context.syncService.reconcile(lookbackDays);

    mergeSyncStats(aggregateStats, result.stats);
    aggregateErrors.push(...result.errors.map(error => `[${context.account.id}] ${error}`));

    accountResults.push({
      accountId: context.account.id,
      mode: isTestMode(context.account.apiKey) ? 'TEST' : 'LIVE',
      result,
    });
  }

  return {
    success: accountResults.every(account => account.result.success),
    stats: aggregateStats,
    errors: aggregateErrors,
    duration: Date.now() - startedAt,
    accounts: accountResults,
  };
}

export async function runStripeAccountSync(
  contexts: StripeAccountContext[],
  options: SyncOptions = {}
): Promise<StripeAggregateSyncResult> {
  const startedAt = Date.now();
  const aggregateStats = emptySyncStats();
  const aggregateErrors: string[] = [];
  const accountResults: StripeAccountSyncResult[] = [];

  for (const context of contexts) {
    const result = await context.syncService.sync(options);

    mergeSyncStats(aggregateStats, result.stats);
    aggregateErrors.push(...result.errors.map(error => `[${context.account.id}] ${error}`));

    accountResults.push({
      accountId: context.account.id,
      mode: isTestMode(context.account.apiKey) ? 'TEST' : 'LIVE',
      result,
    });
  }

  return {
    success: accountResults.every(account => account.result.success),
    stats: aggregateStats,
    errors: aggregateErrors,
    duration: Date.now() - startedAt,
    accounts: accountResults,
  };
}
