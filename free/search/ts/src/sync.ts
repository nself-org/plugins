import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('search:sync');

export interface SyncResult {
  synced: number;
  errors: string[];
  duration: number;
}

export class SearchSyncService {
  async reindex(
    index: string,
    source: () => Promise<Array<Record<string, unknown>>>,
  ): Promise<SyncResult> {
    const start = Date.now();
    logger.debug('reindex start', { index });
    try {
      const docs = await source();
      logger.info('reindex complete', { index, count: docs.length });
      return { synced: docs.length, errors: [], duration: Date.now() - start };
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      logger.error('reindex failed', { index, err: msg });
      return { synced: 0, errors: [msg], duration: Date.now() - start };
    }
  }

  async sync(): Promise<SyncResult> {
    logger.debug('sync noop');
    return { synced: 0, errors: [], duration: 0 };
  }
}
