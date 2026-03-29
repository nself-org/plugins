import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('invitations:webhooks');

type Handler = (payload: unknown) => Promise<void>;

export class InvitationsWebhookHandler {
  private handlers = new Map<string, Handler>();

  constructor() {
    this.register('invitation.sent', this.onInvitationSent.bind(this));
    this.register('invitation.accepted', this.onInvitationAccepted.bind(this));
    this.register('invitation.expired', this.onInvitationExpired.bind(this));
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

  private async onInvitationSent(payload: unknown): Promise<void> {
    logger.info('invitation.sent', { payload });
  }

  private async onInvitationAccepted(payload: unknown): Promise<void> {
    logger.info('invitation.accepted', { payload });
  }

  private async onInvitationExpired(payload: unknown): Promise<void> {
    logger.info('invitation.expired', { payload });
  }
}
