/**
 * Webhook handling utilities for nself plugins
 */

import crypto from 'crypto';
import type { FastifyRequest, FastifyReply, FastifyInstance } from 'fastify';
import type { WebhookEvent, RetryConfig } from './types.js';
import { createLogger } from './logger.js';

const logger = createLogger('webhook');

export interface WebhookHandler {
  (event: WebhookEvent): Promise<void>;
}

export interface WebhookConfig {
  path: string;
  secret?: string;
  signatureHeader?: string;
  signaturePrefix?: string;
  timestampHeader?: string;
  timestampTolerance?: number;
  handlers: Map<string, WebhookHandler>;
  defaultHandler?: WebhookHandler;
}

/**
 * Verify HMAC-SHA256 webhook signature
 */
export function verifyHmacSignature(
  payload: string,
  signature: string,
  secret: string,
  algorithm = 'sha256'
): boolean {
  const expected = crypto
    .createHmac(algorithm, secret)
    .update(payload, 'utf8')
    .digest('hex');

  return crypto.timingSafeEqual(
    Buffer.from(signature.toLowerCase()),
    Buffer.from(expected.toLowerCase())
  );
}

/**
 * Verify Stripe webhook signature
 */
export function verifyStripeSignature(
  payload: string,
  signatureHeader: string,
  secret: string,
  tolerance = 300
): boolean {
  const parts = signatureHeader.split(',');
  const timestamp = parts.find(p => p.startsWith('t='))?.slice(2);
  const signatures = parts
    .filter(p => p.startsWith('v1='))
    .map(p => p.slice(3));

  if (!timestamp || signatures.length === 0) {
    return false;
  }

  const timestampNum = parseInt(timestamp, 10);
  const now = Math.floor(Date.now() / 1000);

  if (Math.abs(now - timestampNum) > tolerance) {
    logger.warn('Stripe signature timestamp outside tolerance', {
      timestamp: timestampNum,
      now,
      diff: Math.abs(now - timestampNum),
    });
    return false;
  }

  const signedPayload = `${timestamp}.${payload}`;
  const expectedSignature = crypto
    .createHmac('sha256', secret)
    .update(signedPayload, 'utf8')
    .digest('hex');

  return signatures.some(sig =>
    crypto.timingSafeEqual(
      Buffer.from(sig),
      Buffer.from(expectedSignature)
    )
  );
}

/**
 * Verify GitHub webhook signature
 */
export function verifyGitHubSignature(
  payload: string,
  signatureHeader: string,
  secret: string
): boolean {
  const signature = signatureHeader.startsWith('sha256=')
    ? signatureHeader.slice(7)
    : signatureHeader;

  return verifyHmacSignature(payload, signature, secret, 'sha256');
}

/**
 * Verify Shopify webhook signature
 */
export function verifyShopifySignature(
  payload: string,
  signatureHeader: string,
  secret: string
): boolean {
  const expected = crypto
    .createHmac('sha256', secret)
    .update(payload, 'utf8')
    .digest('base64');

  return crypto.timingSafeEqual(
    Buffer.from(signatureHeader),
    Buffer.from(expected)
  );
}

/**
 * Generate a unique webhook event ID
 */
export function generateEventId(): string {
  return `evt_${Date.now()}_${crypto.randomBytes(8).toString('hex')}`;
}

/**
 * Retry a function with exponential backoff
 */
export async function withRetry<T>(
  fn: () => Promise<T>,
  config: RetryConfig = {
    maxRetries: 3,
    baseDelay: 1000,
    maxDelay: 30000,
    backoffMultiplier: 2,
  }
): Promise<T> {
  let lastError: Error | undefined;

  for (let attempt = 0; attempt <= config.maxRetries; attempt++) {
    try {
      return await fn();
    } catch (error) {
      lastError = error instanceof Error ? error : new Error(String(error));

      if (attempt === config.maxRetries) {
        break;
      }

      const delay = Math.min(
        config.baseDelay * Math.pow(config.backoffMultiplier, attempt),
        config.maxDelay
      );

      logger.warn(`Retry attempt ${attempt + 1}/${config.maxRetries}`, {
        error: lastError.message,
        nextRetryIn: delay,
      });

      await new Promise(resolve => setTimeout(resolve, delay));
    }
  }

  throw lastError;
}

/**
 * Create Fastify webhook route handler
 */
export function createWebhookRoute(
  app: FastifyInstance,
  config: WebhookConfig,
  verifySignature: (payload: string, signature: string, secret: string) => boolean
): void {
  app.post(config.path, {
    config: {
      rawBody: true,
    },
  }, async (request: FastifyRequest, reply: FastifyReply) => {
    const startTime = Date.now();
    const rawBody = (request as unknown as { rawBody?: string }).rawBody ?? '';

    // Verify signature if secret is configured
    if (config.secret && config.signatureHeader) {
      const signature = request.headers[config.signatureHeader.toLowerCase()] as string | undefined;

      if (!signature) {
        logger.warn('Missing webhook signature');
        return reply.status(401).send({ error: 'Missing signature' });
      }

      if (!verifySignature(rawBody, signature, config.secret)) {
        logger.warn('Invalid webhook signature');
        return reply.status(401).send({ error: 'Invalid signature' });
      }
    }

    // Parse the event
    let payload: Record<string, unknown>;
    try {
      payload = JSON.parse(rawBody);
    } catch {
      logger.warn('Invalid JSON payload');
      return reply.status(400).send({ error: 'Invalid JSON' });
    }

    const eventType = (payload.type ?? payload.topic ?? payload.action ?? 'unknown') as string;
    const eventId = (payload.id ?? generateEventId()) as string;

    const event: WebhookEvent = {
      id: eventId,
      type: eventType,
      data: payload,
      timestamp: new Date(),
      signature: request.headers[config.signatureHeader?.toLowerCase() ?? ''] as string,
      processed: false,
      retryCount: 0,
    };

    logger.info('Webhook received', { type: eventType, id: eventId });

    // Find handler
    const handler = config.handlers.get(eventType) ?? config.defaultHandler;

    if (!handler) {
      logger.warn('No handler for event type', { type: eventType });
      return reply.status(200).send({ received: true, processed: false });
    }

    try {
      await handler(event);
      event.processed = true;
      event.processedAt = new Date();

      const duration = Date.now() - startTime;
      logger.success('Webhook processed', { type: eventType, id: eventId, duration });

      return reply.status(200).send({ received: true, processed: true });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      event.error = message;

      logger.error('Webhook processing failed', { type: eventType, id: eventId, error: message });

      return reply.status(500).send({ error: 'Processing failed' });
    }
  });
}

/**
 * Webhook event processor with queue support
 */
export class WebhookProcessor {
  private handlers: Map<string, WebhookHandler> = new Map();
  private defaultHandler?: WebhookHandler;
  private queue: WebhookEvent[] = [];
  private processing = false;
  private retryConfig: RetryConfig;

  constructor(retryConfig?: Partial<RetryConfig>) {
    this.retryConfig = {
      maxRetries: retryConfig?.maxRetries ?? 3,
      baseDelay: retryConfig?.baseDelay ?? 1000,
      maxDelay: retryConfig?.maxDelay ?? 30000,
      backoffMultiplier: retryConfig?.backoffMultiplier ?? 2,
    };
  }

  registerHandler(eventType: string, handler: WebhookHandler): void {
    this.handlers.set(eventType, handler);
  }

  setDefaultHandler(handler: WebhookHandler): void {
    this.defaultHandler = handler;
  }

  async enqueue(event: WebhookEvent): Promise<void> {
    this.queue.push(event);
    this.processQueue();
  }

  private async processQueue(): Promise<void> {
    if (this.processing) return;
    this.processing = true;

    while (this.queue.length > 0) {
      const event = this.queue.shift()!;
      await this.processEvent(event);
    }

    this.processing = false;
  }

  private async processEvent(event: WebhookEvent): Promise<void> {
    const handler = this.handlers.get(event.type) ?? this.defaultHandler;

    if (!handler) {
      logger.warn('No handler for event', { type: event.type });
      return;
    }

    try {
      await withRetry(() => handler(event), this.retryConfig);
      event.processed = true;
      event.processedAt = new Date();
    } catch (error) {
      event.error = error instanceof Error ? error.message : 'Unknown error';
      event.retryCount = this.retryConfig.maxRetries;
      logger.error('Event processing failed after retries', {
        type: event.type,
        id: event.id,
        error: event.error,
      });
    }
  }
}
