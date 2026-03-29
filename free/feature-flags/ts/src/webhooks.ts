import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('feature-flags:webhooks');

type Handler = (payload: unknown) => Promise<void>;

export class FeatureFlagsWebhookHandler {
  private handlers = new Map<string, Handler>();

  constructor() {
    this.register('flag.enabled', this.onFlagEnabled.bind(this));
    this.register('flag.disabled', this.onFlagDisabled.bind(this));
    this.register('flag.updated', this.onFlagUpdated.bind(this));
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

  private async onFlagEnabled(payload: unknown): Promise<void> {
    logger.info('flag.enabled', { payload });
  }

  private async onFlagDisabled(payload: unknown): Promise<void> {
    logger.info('flag.disabled', { payload });
  }

  private async onFlagUpdated(payload: unknown): Promise<void> {
    logger.info('flag.updated', { payload });
  }
}
