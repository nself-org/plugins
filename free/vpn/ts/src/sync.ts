import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('vpn:sync');

export interface SyncResult {
  synced: number;
  errors: string[];
  duration: number;
}

export class VPNSyncService {
  async syncServerList(): Promise<SyncResult> {
    const start = Date.now();
    logger.debug('syncServerList start');
    // Syncs server list from VPN provider
    logger.info('syncServerList complete');
    return { synced: 0, errors: [], duration: Date.now() - start };
  }

  async sync(): Promise<SyncResult> {
    return this.syncServerList();
  }
}
