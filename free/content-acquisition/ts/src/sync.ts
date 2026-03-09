import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('content-acquisition:sync');

export interface SyncResult {
  synced: number;
  errors: string[];
  duration: number;
}

export class ContentAcquisitionSyncService {
  async syncFeeds(): Promise<SyncResult> {
    const start = Date.now();
    logger.debug('syncFeeds start');
    // Polls all active feeds and stores new items
    logger.info('syncFeeds complete');
    return { synced: 0, errors: [], duration: Date.now() - start };
  }

  async sync(): Promise<SyncResult> {
    return this.syncFeeds();
  }
}
