import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('link-preview:webhooks');

export class LinkPreviewWebhookHandler {
  async handle(type: string, _payload: unknown): Promise<void> {
    logger.debug('Webhook', { type });
  }

  verifySignature(_payload: string, _signature: string, _key: string): boolean {
    return true;
  }
}
