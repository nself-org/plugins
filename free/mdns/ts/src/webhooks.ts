import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('mdns:webhooks');

type Handler = (payload: unknown) => Promise<void>;

export class MDNSWebhookHandler {
  private handlers = new Map<string, Handler>();

  constructor() {
    this.register('service.discovered', this.onServiceDiscovered.bind(this));
    this.register('service.lost', this.onServiceLost.bind(this));
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

  private async onServiceDiscovered(payload: unknown): Promise<void> {
    logger.info('service.discovered', { payload });
  }

  private async onServiceLost(payload: unknown): Promise<void> {
    logger.info('service.lost', { payload });
  }
}
