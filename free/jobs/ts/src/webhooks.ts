import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('jobs:webhooks');

/**
 * Jobs plugin is an internal infrastructure service.
 * It does not receive external webhooks - job events are processed
 * internally via BullMQ event handlers in worker.ts.
 */
export function getWebhookInfo(): { supported: false; reason: string } {
  return {
    supported: false,
    reason: 'Jobs plugin processes events internally via BullMQ - no external webhooks needed',
  };
}
