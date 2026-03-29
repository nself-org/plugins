import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('vpn:webhooks');

type Handler = (payload: unknown) => Promise<void>;

export class VPNWebhookHandler {
  private handlers = new Map<string, Handler>();

  constructor() {
    this.register('vpn.connected', this.onVpnConnected.bind(this));
    this.register('vpn.disconnected', this.onVpnDisconnected.bind(this));
    this.register('vpn.error', this.onVpnError.bind(this));
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

  private async onVpnConnected(payload: unknown): Promise<void> {
    logger.info('vpn.connected', { payload });
  }

  private async onVpnDisconnected(payload: unknown): Promise<void> {
    logger.info('vpn.disconnected', { payload });
  }

  private async onVpnError(payload: unknown): Promise<void> {
    logger.info('vpn.error', { payload });
  }
}
