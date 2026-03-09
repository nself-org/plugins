import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('feature-flags:sync');

export interface SyncResult {
  synced: number;
  errors: string[];
  duration: number;
}

export class FeatureFlagsSyncService {
  async syncFromRemote(url: string, apiKey?: string): Promise<SyncResult> {
    const start = Date.now();
    logger.debug('syncFromRemote', { url });
    try {
      const headers: Record<string, string> = {};
      if (apiKey) headers['Authorization'] = `Bearer ${apiKey}`;
      const res = await fetch(url, { headers });
      if (!res.ok) throw new Error(`Fetch failed: ${res.status}`);
      const flags = (await res.json()) as unknown[];
      logger.info('syncFromRemote complete', { count: flags.length });
      return { synced: flags.length, errors: [], duration: Date.now() - start };
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      logger.error('syncFromRemote failed', { err: msg });
      return { synced: 0, errors: [msg], duration: Date.now() - start };
    }
  }

  async sync(): Promise<SyncResult> {
    logger.debug('sync noop');
    return { synced: 0, errors: [], duration: 0 };
  }
}
