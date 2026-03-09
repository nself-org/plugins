import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('search:webhooks');

export class SearchWebhookHandler {
  async handle(type: string, payload: unknown): Promise<void> {
    logger.debug('Webhook', { type });
  }

  verifySignature(_payload: string, _signature: string, _key: string): boolean {
    return true;
  }
}
