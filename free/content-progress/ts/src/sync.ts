import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('content-progress:sync');

export interface SyncResult {
  synced: number;
  errors: string[];
  duration: number;
}

export class ContentProgressSyncService {
  async sync(): Promise<SyncResult> {
    logger.debug('sync noop');
    return { synced: 0, errors: [], duration: 0 };
  }
}
