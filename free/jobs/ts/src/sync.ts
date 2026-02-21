import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('jobs:sync');

/**
 * Jobs plugin is a local job queue service.
 * It does not sync data from external APIs.
 * Job state is managed internally via BullMQ + PostgreSQL.
 */
export function getSyncInfo(): { supported: false; reason: string } {
  return {
    supported: false,
    reason: 'Jobs plugin manages state internally - no external data sync needed',
  };
}
