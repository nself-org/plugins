import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('torrent-manager:webhooks');

type Handler = (payload: unknown) => Promise<void>;

export class TorrentManagerWebhookHandler {
  private handlers = new Map<string, Handler>();

  constructor() {
    this.register('torrent.added', this.onTorrentAdded.bind(this));
    this.register('torrent.completed', this.onTorrentCompleted.bind(this));
    this.register('torrent.error', this.onTorrentError.bind(this));
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

  private async onTorrentAdded(payload: unknown): Promise<void> {
    logger.info('torrent.added', { payload });
  }

  private async onTorrentCompleted(payload: unknown): Promise<void> {
    logger.info('torrent.completed', { payload });
  }

  private async onTorrentError(payload: unknown): Promise<void> {
    logger.info('torrent.error', { payload });
  }
}
