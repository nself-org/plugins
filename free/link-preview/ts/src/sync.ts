import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('link-preview:sync');

export interface SyncResult {
  synced: number;
  errors: string[];
  duration: number;
}

export class LinkPreviewSyncService {
  async sync(): Promise<SyncResult> {
    logger.debug('sync noop');
    return { synced: 0, errors: [], duration: 0 };
  }
}
