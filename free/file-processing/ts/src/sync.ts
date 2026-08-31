import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('file-processing:sync');

/**
 * File processing plugin processes files on-demand.
 * It does not sync data from external APIs.
 * File metadata is stored locally when files are processed.
 */
export function getSyncInfo(): { supported: false; reason: string } {
  return {
    supported: false,
    reason: 'File processing operates on-demand - no external data sync needed',
  };
}
