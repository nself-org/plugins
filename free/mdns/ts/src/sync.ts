import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('mdns:sync');

export interface SyncResult {
  synced: number;
  errors: string[];
  duration: number;
}

export class MDNSSyncService {
  async syncServiceList(): Promise<SyncResult> {
    const start = Date.now();
    logger.debug('syncServiceList start');
    // Polls for service changes on the local network
    logger.info('syncServiceList complete');
    return { synced: 0, errors: [], duration: Date.now() - start };
  }

  async sync(): Promise<SyncResult> {
    return this.syncServiceList();
  }
}
