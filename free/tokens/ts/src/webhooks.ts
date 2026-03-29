import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('tokens:webhooks');

type Handler = (payload: unknown) => Promise<void>;

export class TokensWebhookHandler {
  private handlers = new Map<string, Handler>();

  constructor() {
    this.register('token.created', this.onTokenCreated.bind(this));
    this.register('token.revoked', this.onTokenRevoked.bind(this));
    this.register('token.expired', this.onTokenExpired.bind(this));
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

  private async onTokenCreated(payload: unknown): Promise<void> {
    logger.info('token.created', { payload });
  }

  private async onTokenRevoked(payload: unknown): Promise<void> {
    logger.info('token.revoked', { payload });
  }

  private async onTokenExpired(payload: unknown): Promise<void> {
    logger.info('token.expired', { payload });
  }
}
