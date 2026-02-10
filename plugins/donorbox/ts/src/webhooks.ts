/**
 * Donorbox Webhook Handler
 * HMAC-SHA256 verification and event processing
 */

import crypto from 'crypto';
import { createLogger } from '@nself/plugin-utils';
import type { DonorboxDatabase } from './database.js';
import type { DonorboxDonation, DonationRecord } from './types.js';

const logger = createLogger('donorbox:webhooks');

export class DonorboxWebhookHandler {
  constructor(
    private db: DonorboxDatabase,
  ) {}

  static verifySignature(payload: string, signature: string, secret: string): boolean {
    if (!secret || !signature) return false;

    const expected = crypto
      .createHmac('sha256', secret)
      .update(payload, 'utf8')
      .digest('hex');

    try {
      return crypto.timingSafeEqual(
        Buffer.from(signature),
        Buffer.from(expected)
      );
    } catch {
      return false;
    }
  }

  async handleEvent(eventType: string, payload: Record<string, unknown>): Promise<void> {
    const eventId = `donorbox_${Date.now()}_${crypto.randomBytes(4).toString('hex')}`;

    // Store raw event
    await this.db.insertWebhookEvent({
      id: eventId,
      event_type: eventType,
      payload,
    });

    try {
      switch (eventType) {
        case 'donation.created':
          await this.handleDonationCreated(payload);
          break;
        default:
          logger.debug('No handler for event type', { type: eventType });
      }

      await this.db.markEventProcessed(eventId);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      await this.db.markEventProcessed(eventId, message);
      throw error;
    }
  }

  private async handleDonationCreated(payload: Record<string, unknown>): Promise<void> {
    const donation = payload as unknown as DonorboxDonation;

    const record: DonationRecord = {
      id: donation.id,
      source_account_id: 'primary',
      campaign_id: donation.campaign?.id ?? null,
      campaign_name: donation.campaign?.name ?? null,
      donor_id: donation.donor?.id ?? null,
      donor_email: donation.donor?.email ?? null,
      donor_name: donation.donor?.name ??
        ([donation.donor?.first_name, donation.donor?.last_name].filter(Boolean).join(' ') || null),
      amount: parseFloat(donation.amount ?? '0'),
      converted_amount: donation.converted_amount ? parseFloat(donation.converted_amount) : null,
      converted_net_amount: donation.converted_net_amount ? parseFloat(donation.converted_net_amount) : null,
      amount_refunded: parseFloat(donation.amount_refunded ?? '0'),
      currency: donation.currency ?? 'USD',
      donation_type: donation.donation_type ?? '',
      donation_date: donation.donation_date ? new Date(donation.donation_date) : null,
      processing_fee: donation.processing_fee ? parseFloat(donation.processing_fee) : null,
      status: donation.status ?? '',
      recurring: donation.recurring ?? false,
      comment: donation.comment || null,
      designation: donation.designation || null,
      stripe_charge_id: donation.stripe_charge_id || null,
      paypal_transaction_id: donation.paypal_transaction_id || null,
      questions: donation.questions as unknown as Record<string, unknown>[] ?? [],
      created_at: donation.created_at ? new Date(donation.created_at) : null,
      updated_at: donation.updated_at ? new Date(donation.updated_at) : null,
      synced_at: new Date(),
    };

    await this.db.upsertDonations([record]);
    logger.info('Donation created via webhook', { id: donation.id, amount: donation.amount });
  }
}
