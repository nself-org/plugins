/**
 * Donorbox multi-account sync orchestration
 */

import { DonorboxClient } from './client.js';
import { DonorboxSyncService, type SyncResult } from './sync.js';
import { DonorboxWebhookHandler } from './webhooks.js';
import type { DonorboxAccountConfig, DonorboxConfig, SyncOptions, SyncStats } from './types.js';
import { DonorboxDatabase } from './database.js';

export interface DonorboxAccountContext {
  account: DonorboxAccountConfig;
  db: DonorboxDatabase;
  client: DonorboxClient;
  syncService: DonorboxSyncService;
  webhookHandler: DonorboxWebhookHandler;
}

export interface DonorboxAccountSyncResult {
  accountId: string;
  result: SyncResult;
}

export interface DonorboxAggregateSyncResult {
  success: boolean;
  stats: SyncStats;
  errors: string[];
  duration: number;
  accounts: DonorboxAccountSyncResult[];
}

function emptySyncStats(): SyncStats {
  return {
    campaigns: 0, donors: 0, donations: 0, plans: 0,
    events: 0, tickets: 0, lastSyncedAt: null,
  };
}

function mergeSyncStats(target: SyncStats, source: SyncStats): void {
  target.campaigns += source.campaigns;
  target.donors += source.donors;
  target.donations += source.donations;
  target.plans += source.plans;
  target.events += source.events;
  target.tickets += source.tickets;

  if (source.lastSyncedAt) {
    if (!target.lastSyncedAt || source.lastSyncedAt > target.lastSyncedAt) {
      target.lastSyncedAt = source.lastSyncedAt;
    }
  }
}

export function createDonorboxAccountContexts(config: DonorboxConfig, db: DonorboxDatabase): DonorboxAccountContext[] {
  return config.accounts.map(account => {
    const accountDb = db.forSourceAccount(account.id);
    const client = new DonorboxClient(account.email, account.apiKey);
    const syncService = new DonorboxSyncService(client, accountDb);
    const webhookHandler = new DonorboxWebhookHandler(accountDb);

    return { account, db: accountDb, client, syncService, webhookHandler };
  });
}

export async function runDonorboxAccountReconcile(
  contexts: DonorboxAccountContext[],
  lookbackDays = 7
): Promise<DonorboxAggregateSyncResult> {
  const startedAt = Date.now();
  const aggregateStats = emptySyncStats();
  const aggregateErrors: string[] = [];
  const accountResults: DonorboxAccountSyncResult[] = [];

  for (const context of contexts) {
    const result = await context.syncService.reconcile(lookbackDays);
    mergeSyncStats(aggregateStats, result.stats);
    aggregateErrors.push(...result.errors.map(e => `[${context.account.id}] ${e}`));
    accountResults.push({ accountId: context.account.id, result });
  }

  return {
    success: accountResults.every(a => a.result.success),
    stats: aggregateStats,
    errors: aggregateErrors,
    duration: Date.now() - startedAt,
    accounts: accountResults,
  };
}

export async function runDonorboxAccountSync(
  contexts: DonorboxAccountContext[],
  options: SyncOptions = {}
): Promise<DonorboxAggregateSyncResult> {
  const startedAt = Date.now();
  const aggregateStats = emptySyncStats();
  const aggregateErrors: string[] = [];
  const accountResults: DonorboxAccountSyncResult[] = [];

  for (const context of contexts) {
    const result = await context.syncService.sync(options);
    mergeSyncStats(aggregateStats, result.stats);
    aggregateErrors.push(...result.errors.map(e => `[${context.account.id}] ${e}`));
    accountResults.push({ accountId: context.account.id, result });
  }

  return {
    success: accountResults.every(a => a.result.success),
    stats: aggregateStats,
    errors: aggregateErrors,
    duration: Date.now() - startedAt,
    accounts: accountResults,
  };
}
