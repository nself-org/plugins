/**
 * Webhook Delivery Service
 * Handles actual HTTP delivery with retry logic, HMAC signing, and dead-letter queue
 */

import { createLogger } from '@nself/plugin-utils';
import { createHmac } from 'crypto';
import fetch from 'node-fetch';
import type { WebhooksDatabase } from './database.js';
import type { WebhookDeliveryRecord, DispatchEventInput } from './types.js';
import type { Config } from './config.js';

const logger = createLogger('webhooks:delivery');

export class WebhookDeliveryService {
  private db: WebhooksDatabase;
  private config: Config;
  private activeDeliveries = 0;
  private processingInterval: NodeJS.Timeout | null = null;

  constructor(db: WebhooksDatabase, config: Config) {
    this.db = db;
    this.config = config;
  }

  /**
   * Generate HMAC-SHA256 signature for webhook payload
   */
  generateSignature(payload: Record<string, unknown>, secret: string): string {
    const timestamp = Math.floor(Date.now() / 1000);
    const payloadString = JSON.stringify(payload);
    const signedPayload = `${timestamp}.${payloadString}`;
    const signature = createHmac('sha256', secret)
      .update(signedPayload)
      .digest('hex');

    return `t=${timestamp},v1=${signature}`;
  }

  /**
   * Dispatch an event to all matching endpoints
   */
  async dispatchEvent(input: DispatchEventInput): Promise<{
    dispatched: number;
    endpoints: string[];
  }> {
    logger.info(`Dispatching event: ${input.event_type}`);

    // Find all enabled endpoints that subscribe to this event
    const allEndpoints = await this.db.listEndpoints({ enabled: true });
    const matchingEndpoints = allEndpoints.filter(endpoint =>
      endpoint.events.includes(input.event_type) || endpoint.events.includes('*')
    );

    // Filter by specific endpoints if provided
    const targetEndpoints = input.endpoints && input.endpoints.length > 0
      ? matchingEndpoints.filter(endpoint => input.endpoints!.includes(endpoint.id))
      : matchingEndpoints;

    if (targetEndpoints.length === 0) {
      logger.warn(`No matching endpoints for event: ${input.event_type}`);
      return { dispatched: 0, endpoints: [] };
    }

    // Add idempotency key to payload
    const payload = {
      ...input.payload,
      event_type: input.event_type,
      idempotency_key: input.idempotency_key ?? `${input.event_type}_${Date.now()}`,
      timestamp: new Date().toISOString(),
    };

    // Create delivery records for each endpoint
    const deliveries: WebhookDeliveryRecord[] = [];
    for (const endpoint of targetEndpoints) {
      const signature = this.generateSignature(payload, endpoint.secret);
      const delivery = await this.db.createDelivery(
        endpoint.id,
        input.event_type,
        payload,
        signature,
        this.config.maxAttempts
      );
      deliveries.push(delivery);
    }

    logger.info(`Created ${deliveries.length} deliveries for event: ${input.event_type}`);

    return {
      dispatched: deliveries.length,
      endpoints: targetEndpoints.map(e => e.id),
    };
  }

  /**
   * Process a single delivery
   */
  async processDelivery(delivery: WebhookDeliveryRecord): Promise<void> {
    const endpoint = await this.db.getEndpoint(delivery.endpoint_id);
    if (!endpoint) {
      logger.error(`Endpoint not found: ${delivery.endpoint_id}`);
      const details: { errorMessage: string } = { errorMessage: 'Endpoint not found' };
      await this.db.updateDeliveryStatus(delivery.id, 'failed', details);
      return;
    }

    if (!endpoint.enabled) {
      logger.warn(`Endpoint disabled: ${endpoint.id}`);
      const details: { errorMessage: string } = { errorMessage: 'Endpoint is disabled' };
      await this.db.updateDeliveryStatus(delivery.id, 'failed', details);
      return;
    }

    const startTime = Date.now();

    try {
      // Update status to delivering
      const deliveringDetails: Record<string, never> = {};
      await this.db.updateDeliveryStatus(delivery.id, 'delivering', deliveringDetails);

      // Prepare request
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
        'User-Agent': 'nself-webhooks/1.0',
        'X-Webhook-Signature': delivery.signature,
        'X-Webhook-Event-Type': delivery.event_type,
        'X-Webhook-Delivery-Id': delivery.id,
        'X-Webhook-Attempt': String(delivery.attempt_count + 1),
        ...endpoint.headers,
      };

      const payloadSize = JSON.stringify(delivery.payload).length;
      if (payloadSize > this.config.maxPayloadSize) {
        throw new Error(`Payload size (${payloadSize}) exceeds limit (${this.config.maxPayloadSize})`);
      }

      // Make HTTP request
      logger.debug(`Delivering to ${endpoint.url}`, {
        deliveryId: delivery.id,
        attempt: delivery.attempt_count + 1,
      });

      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), this.config.requestTimeoutMs);

      const response = await fetch(endpoint.url, {
        method: 'POST',
        headers,
        body: JSON.stringify(delivery.payload),
        signal: controller.signal,
      });

      clearTimeout(timeout);

      const responseTime = Date.now() - startTime;
      const responseBody = await response.text();

      // Check response status
      if (response.ok) {
        // Success
        logger.success(`Delivery successful: ${delivery.id}`, {
          endpoint: endpoint.url,
          status: response.status,
          responseTime: `${responseTime}ms`,
        });

        await this.db.updateDeliveryStatus(delivery.id, 'delivered', {
          responseStatus: response.status,
          responseBody: responseBody.substring(0, 1000), // Limit response body size
          responseTimeMs: responseTime,
        });

        await this.db.recordEndpointSuccess(endpoint.id);
      } else {
        // HTTP error
        throw new Error(`HTTP ${response.status}: ${responseBody.substring(0, 200)}`);
      }
    } catch (error) {
      // Delivery failed
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      const responseTime = Date.now() - startTime;

      logger.error(`Delivery failed: ${delivery.id}`, {
        endpoint: endpoint.url,
        attempt: delivery.attempt_count + 1,
        error: errorMessage,
      });

      // Determine if we should retry
      const newAttemptCount = delivery.attempt_count + 1;
      const shouldRetry = newAttemptCount < delivery.max_attempts;

      if (shouldRetry) {
        // Schedule retry with exponential backoff
        const delayIndex = Math.min(newAttemptCount - 1, this.config.retryDelays.length - 1);
        const retryDelay = this.config.retryDelays[delayIndex];
        const nextRetryAt = new Date(Date.now() + retryDelay);

        logger.info(`Scheduling retry for ${delivery.id}`, {
          attempt: newAttemptCount + 1,
          delay: `${retryDelay}ms`,
          nextRetryAt: nextRetryAt.toISOString(),
        });

        await this.db.updateDeliveryStatus(delivery.id, 'pending', {
          responseTimeMs: responseTime,
          errorMessage,
          nextRetryAt,
        });
      } else {
        // Max attempts reached - move to dead letter
        logger.warn(`Max attempts reached for delivery: ${delivery.id}`);

        await this.db.updateDeliveryStatus(delivery.id, 'dead_letter', {
          responseTimeMs: responseTime,
          errorMessage,
        });

        // Create dead letter record
        const updatedDelivery = await this.db.getDelivery(delivery.id);
        if (updatedDelivery) {
          await this.db.createDeadLetter(updatedDelivery);
        }
      }

      await this.db.recordEndpointFailure(endpoint.id, this.config.autoDisableThreshold);
    }
  }

  /**
   * Process pending deliveries
   */
  async processPendingDeliveries(): Promise<void> {
    // Check if we've hit the concurrent delivery limit
    if (this.activeDeliveries >= this.config.concurrentDeliveries) {
      logger.debug('Concurrent delivery limit reached, skipping batch');
      return;
    }

    const availableSlots = this.config.concurrentDeliveries - this.activeDeliveries;
    const deliveries = await this.db.getPendingDeliveries(availableSlots);

    if (deliveries.length === 0) {
      return;
    }

    logger.info(`Processing ${deliveries.length} pending deliveries`);

    // Process deliveries concurrently
    const promises = deliveries.map(async delivery => {
      this.activeDeliveries++;
      try {
        await this.processDelivery(delivery);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        logger.error(`Error processing delivery ${delivery.id}:`, { error: message });
      } finally {
        this.activeDeliveries--;
      }
    });

    await Promise.all(promises);
  }

  /**
   * Start background processing of deliveries
   */
  startProcessing(intervalMs = 5000): void {
    if (this.processingInterval) {
      logger.warn('Processing already started');
      return;
    }

    logger.info('Starting webhook delivery processor', {
      interval: `${intervalMs}ms`,
      concurrentDeliveries: this.config.concurrentDeliveries,
    });

    this.processingInterval = setInterval(async () => {
      try {
        await this.processPendingDeliveries();
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        logger.error('Error in delivery processor:', { error: message });
      }
    }, intervalMs);
  }

  /**
   * Stop background processing
   */
  stopProcessing(): void {
    if (this.processingInterval) {
      clearInterval(this.processingInterval);
      this.processingInterval = null;
      logger.info('Stopped webhook delivery processor');
    }
  }

  /**
   * Test an endpoint with a sample payload
   */
  async testEndpoint(endpointId: string): Promise<{
    success: boolean;
    status?: number;
    responseTime?: number;
    error?: string;
  }> {
    const endpoint = await this.db.getEndpoint(endpointId);
    if (!endpoint) {
      return { success: false, error: 'Endpoint not found' };
    }

    const testPayload = {
      event_type: 'test.webhook',
      test: true,
      timestamp: new Date().toISOString(),
      message: 'This is a test webhook from nself',
    };

    const signature = this.generateSignature(testPayload, endpoint.secret);

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'User-Agent': 'nself-webhooks/1.0',
      'X-Webhook-Signature': signature,
      'X-Webhook-Event-Type': 'test.webhook',
      'X-Webhook-Test': 'true',
      ...endpoint.headers,
    };

    const startTime = Date.now();

    try {
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), this.config.requestTimeoutMs);

      const response = await fetch(endpoint.url, {
        method: 'POST',
        headers,
        body: JSON.stringify(testPayload),
        signal: controller.signal,
      });

      clearTimeout(timeout);

      const responseTime = Date.now() - startTime;

      return {
        success: response.ok,
        status: response.status,
        responseTime,
        error: response.ok ? undefined : `HTTP ${response.status}`,
      };
    } catch (error) {
      const responseTime = Date.now() - startTime;
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';

      return {
        success: false,
        responseTime,
        error: errorMessage,
      };
    }
  }
}
