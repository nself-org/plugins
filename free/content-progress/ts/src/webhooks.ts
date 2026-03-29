import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('content-progress:webhooks');

type Handler = (payload: unknown) => Promise<void>;

export class ContentProgressWebhookHandler {
  private handlers = new Map<string, Handler>();

  constructor() {
    this.register('progress.updated', this.onProgressUpdated.bind(this));
    this.register('content.completed', this.onContentCompleted.bind(this));
  }

  register(type: string, handler: Handler): void {
    this.handlers.set(type, handler);
  }

  async handle(type: string, payload: unknown): Promise<void> {
    logger.debug('Webhook', { type });
    const handler = this.handlers.get(type);
    if (handler) await handler(payload);
    else logger.warn('Unhandled webhook type', { type });
  }

  verifySignature(payload: string, signature: string, key: string): boolean {
    const { createHmac, timingSafeEqual } = require('node:crypto');
    const expected = createHmac('sha256', key).update(payload).digest('hex');
    try {
      return timingSafeEqual(Buffer.from(signature), Buffer.from(expected));
    } catch {
      return false;
    }
  }

  private async onProgressUpdated(payload: unknown): Promise<void> {
    logger.info('progress.updated', { payload });
  }

  private async onContentCompleted(payload: unknown): Promise<void> {
    logger.info('content.completed', { payload });
  }
}
