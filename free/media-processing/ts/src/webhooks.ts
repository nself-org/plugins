import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('media-processing:webhooks');

type HandlerFn = (payload: unknown) => Promise<void>;

export class MediaProcessingWebhookHandler {
  private handlers = new Map<string, HandlerFn>();

  constructor() {
    this.register('transcode.completed', this.handleTranscodeCompleted.bind(this));
    this.register('transcode.failed', this.handleTranscodeFailed.bind(this));
    this.register('thumbnail.ready', this.handleThumbnailReady.bind(this));
  }

  register(type: string, fn: HandlerFn): void {
    this.handlers.set(type, fn);
  }

  async handle(type: string, payload: unknown): Promise<void> {
    logger.debug('Webhook', { type });
    const fn = this.handlers.get(type);
    if (fn) await fn(payload);
    else logger.warn('No handler', { type });
  }

  verifySignature(payload: string, sig: string, secret: string): boolean {
    const { createHmac, timingSafeEqual } = require('node:crypto');
    const expected = createHmac('sha256', secret).update(payload).digest('hex');
    try {
      return timingSafeEqual(Buffer.from(sig), Buffer.from(expected));
    } catch {
      return false;
    }
  }

  private async handleTranscodeCompleted(payload: unknown): Promise<void> {
    logger.info('transcode.completed received', { payload });
  }

  private async handleTranscodeFailed(payload: unknown): Promise<void> {
    logger.info('transcode.failed received', { payload });
  }

  private async handleThumbnailReady(payload: unknown): Promise<void> {
    logger.info('thumbnail.ready received', { payload });
  }
}
