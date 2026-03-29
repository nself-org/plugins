/**
 * Stripe Webhook Handlers
 * Complete webhook event processing for all Stripe objects
 */

import type Stripe from 'stripe';
import { createLogger } from '@nself/plugin-utils';
import { StripeClient } from './client.js';
import { StripeDatabase } from './database.js';
import { StripeSyncService } from './sync.js';
import type { StripeWebhookEventRecord } from './types.js';

const logger = createLogger('stripe:webhooks');

export type WebhookHandlerFn = (event: Stripe.Event) => Promise<void>;

export class StripeWebhookHandler {
  private client: StripeClient;
  private db: StripeDatabase;
  private syncService: StripeSyncService;
  private handlers: Map<string, WebhookHandlerFn>;

  constructor(client: StripeClient, db: StripeDatabase, syncService: StripeSyncService) {
    this.client = client;
    this.db = db;
    this.syncService = syncService;
    this.handlers = new Map();

    this.registerDefaultHandlers();
  }

  private registerDefaultHandlers(): void {
    // =========================================================================
    // Customer events
    // =========================================================================
    this.register('customer.created', this.handleCustomerCreated.bind(this));
    this.register('customer.updated', this.handleCustomerUpdated.bind(this));
    this.register('customer.deleted', this.handleCustomerDeleted.bind(this));

    // Customer tax ID events
    this.register('customer.tax_id.created', this.handleCustomerTaxIdCreated.bind(this));
    this.register('customer.tax_id.deleted', this.handleCustomerTaxIdDeleted.bind(this));
    this.register('customer.tax_id.updated', this.handleCustomerTaxIdUpdated.bind(this));

    // =========================================================================
    // Product events
    // =========================================================================
    this.register('product.created', this.handleProductCreated.bind(this));
    this.register('product.updated', this.handleProductUpdated.bind(this));
    this.register('product.deleted', this.handleProductDeleted.bind(this));

    // =========================================================================
    // Price events
    // =========================================================================
    this.register('price.created', this.handlePriceCreated.bind(this));
    this.register('price.updated', this.handlePriceUpdated.bind(this));
    this.register('price.deleted', this.handlePriceDeleted.bind(this));

    // =========================================================================
    // Coupon events
    // =========================================================================
    this.register('coupon.created', this.handleCouponCreated.bind(this));
    this.register('coupon.updated', this.handleCouponUpdated.bind(this));
    this.register('coupon.deleted', this.handleCouponDeleted.bind(this));

    // =========================================================================
    // Promotion code events
    // =========================================================================
    this.register('promotion_code.created', this.handlePromotionCodeCreated.bind(this));
    this.register('promotion_code.updated', this.handlePromotionCodeUpdated.bind(this));
    this.register('promotion_code.deleted', this.handlePromotionCodeDeleted.bind(this));

    // =========================================================================
    // Subscription events
    // =========================================================================
    this.register('customer.subscription.created', this.handleSubscriptionCreated.bind(this));
    this.register('customer.subscription.updated', this.handleSubscriptionUpdated.bind(this));
    this.register('customer.subscription.deleted', this.handleSubscriptionDeleted.bind(this));
    this.register('customer.subscription.trial_will_end', this.handleSubscriptionTrialWillEnd.bind(this));
    this.register('customer.subscription.paused', this.handleSubscriptionPaused.bind(this));
    this.register('customer.subscription.resumed', this.handleSubscriptionResumed.bind(this));
    this.register('customer.subscription.pending_update_applied', this.handleSubscriptionPendingUpdateApplied.bind(this));
    this.register('customer.subscription.pending_update_expired', this.handleSubscriptionPendingUpdateExpired.bind(this));

    // =========================================================================
    // Subscription Schedule events
    // =========================================================================
    this.register('subscription_schedule.aborted', this.handleSubscriptionScheduleAborted.bind(this));
    this.register('subscription_schedule.canceled', this.handleSubscriptionScheduleCanceled.bind(this));
    this.register('subscription_schedule.completed', this.handleSubscriptionScheduleCompleted.bind(this));
    this.register('subscription_schedule.created', this.handleSubscriptionScheduleCreated.bind(this));
    this.register('subscription_schedule.expiring', this.handleSubscriptionScheduleExpiring.bind(this));
    this.register('subscription_schedule.released', this.handleSubscriptionScheduleReleased.bind(this));
    this.register('subscription_schedule.updated', this.handleSubscriptionScheduleUpdated.bind(this));

    // =========================================================================
    // Invoice events
    // =========================================================================
    this.register('invoice.created', this.handleInvoiceCreated.bind(this));
    this.register('invoice.updated', this.handleInvoiceUpdated.bind(this));
    this.register('invoice.deleted', this.handleInvoiceDeleted.bind(this));
    this.register('invoice.finalized', this.handleInvoiceFinalized.bind(this));
    this.register('invoice.paid', this.handleInvoicePaid.bind(this));
    this.register('invoice.payment_failed', this.handleInvoicePaymentFailed.bind(this));
    this.register('invoice.payment_succeeded', this.handleInvoicePaymentSucceeded.bind(this));
    this.register('invoice.voided', this.handleInvoiceVoided.bind(this));
    this.register('invoice.marked_uncollectible', this.handleInvoiceMarkedUncollectible.bind(this));
    this.register('invoice.sent', this.handleInvoiceSent.bind(this));
    this.register('invoice.upcoming', this.handleInvoiceUpcoming.bind(this));
    this.register('invoice.payment_action_required', this.handleInvoicePaymentActionRequired.bind(this));

    // Invoice Item events
    this.register('invoiceitem.created', this.handleInvoiceItemCreated.bind(this));
    this.register('invoiceitem.deleted', this.handleInvoiceItemDeleted.bind(this));

    // =========================================================================
    // Credit Note events
    // =========================================================================
    this.register('credit_note.created', this.handleCreditNoteCreated.bind(this));
    this.register('credit_note.updated', this.handleCreditNoteUpdated.bind(this));
    this.register('credit_note.voided', this.handleCreditNoteVoided.bind(this));

    // =========================================================================
    // Charge events
    // =========================================================================
    this.register('charge.captured', this.handleChargeCaptured.bind(this));
    this.register('charge.expired', this.handleChargeExpired.bind(this));
    this.register('charge.failed', this.handleChargeFailed.bind(this));
    this.register('charge.pending', this.handleChargePending.bind(this));
    this.register('charge.refunded', this.handleChargeRefunded.bind(this));
    this.register('charge.succeeded', this.handleChargeSucceeded.bind(this));
    this.register('charge.updated', this.handleChargeUpdated.bind(this));

    // =========================================================================
    // Refund events
    // =========================================================================
    this.register('refund.created', this.handleRefundCreated.bind(this));
    this.register('refund.updated', this.handleRefundUpdated.bind(this));
    this.register('refund.failed', this.handleRefundFailed.bind(this));

    // =========================================================================
    // Dispute events
    // =========================================================================
    this.register('charge.dispute.created', this.handleDisputeCreated.bind(this));
    this.register('charge.dispute.updated', this.handleDisputeUpdated.bind(this));
    this.register('charge.dispute.closed', this.handleDisputeClosed.bind(this));
    this.register('charge.dispute.funds_reinstated', this.handleDisputeFundsReinstated.bind(this));
    this.register('charge.dispute.funds_withdrawn', this.handleDisputeFundsWithdrawn.bind(this));

    // =========================================================================
    // Payment Intent events
    // =========================================================================
    this.register('payment_intent.created', this.handlePaymentIntentCreated.bind(this));
    this.register('payment_intent.succeeded', this.handlePaymentIntentSucceeded.bind(this));
    this.register('payment_intent.payment_failed', this.handlePaymentIntentFailed.bind(this));
    this.register('payment_intent.canceled', this.handlePaymentIntentCanceled.bind(this));
    this.register('payment_intent.processing', this.handlePaymentIntentProcessing.bind(this));
    this.register('payment_intent.requires_action', this.handlePaymentIntentRequiresAction.bind(this));
    this.register('payment_intent.amount_capturable_updated', this.handlePaymentIntentAmountCapturableUpdated.bind(this));
    this.register('payment_intent.partially_funded', this.handlePaymentIntentPartiallyFunded.bind(this));

    // =========================================================================
    // Setup Intent events
    // =========================================================================
    this.register('setup_intent.created', this.handleSetupIntentCreated.bind(this));
    this.register('setup_intent.canceled', this.handleSetupIntentCanceled.bind(this));
    this.register('setup_intent.requires_action', this.handleSetupIntentRequiresAction.bind(this));
    this.register('setup_intent.setup_failed', this.handleSetupIntentFailed.bind(this));
    this.register('setup_intent.succeeded', this.handleSetupIntentSucceeded.bind(this));

    // =========================================================================
    // Payment Method events
    // =========================================================================
    this.register('payment_method.attached', this.handlePaymentMethodAttached.bind(this));
    this.register('payment_method.detached', this.handlePaymentMethodDetached.bind(this));
    this.register('payment_method.updated', this.handlePaymentMethodUpdated.bind(this));
    this.register('payment_method.automatically_updated', this.handlePaymentMethodAutomaticallyUpdated.bind(this));

    // =========================================================================
    // Checkout Session events
    // =========================================================================
    this.register('checkout.session.completed', this.handleCheckoutSessionCompleted.bind(this));
    this.register('checkout.session.async_payment_succeeded', this.handleCheckoutAsyncPaymentSucceeded.bind(this));
    this.register('checkout.session.async_payment_failed', this.handleCheckoutAsyncPaymentFailed.bind(this));
    this.register('checkout.session.expired', this.handleCheckoutSessionExpired.bind(this));

    // =========================================================================
    // Balance events
    // =========================================================================
    this.register('balance.available', this.handleBalanceAvailable.bind(this));

    // =========================================================================
    // Tax Rate events
    // =========================================================================
    this.register('tax_rate.created', this.handleTaxRateCreated.bind(this));
    this.register('tax_rate.updated', this.handleTaxRateUpdated.bind(this));

    // =========================================================================
    // Payout events (informational)
    // =========================================================================
    this.register('payout.canceled', this.handlePayoutEvent.bind(this));
    this.register('payout.created', this.handlePayoutEvent.bind(this));
    this.register('payout.failed', this.handlePayoutEvent.bind(this));
    this.register('payout.paid', this.handlePayoutEvent.bind(this));
    this.register('payout.reconciliation_completed', this.handlePayoutEvent.bind(this));
    this.register('payout.updated', this.handlePayoutEvent.bind(this));
  }

  register(eventType: string, handler: WebhookHandlerFn): void {
    this.handlers.set(eventType, handler);
  }

  async handle(event: Stripe.Event): Promise<void> {
    const eventRecord: StripeWebhookEventRecord = {
      id: event.id,
      type: event.type,
      api_version: event.api_version,
      data: event.data as unknown as Record<string, unknown>,
      object_type: (event.data.object as { object?: string }).object ?? 'unknown',
      object_id: (event.data.object as { id?: string }).id ?? 'unknown',
      request_id: event.request?.id ?? null,
      request_idempotency_key: event.request?.idempotency_key ?? null,
      livemode: event.livemode,
      pending_webhooks: event.pending_webhooks,
      processed: false,
      processed_at: null,
      error: null,
      retry_count: 0,
      created_at: new Date(event.created * 1000),
      received_at: new Date(),
    };

    // Store the event
    await this.db.insertWebhookEvent(eventRecord);
    logger.info('Webhook event received', { type: event.type, id: event.id });

    // Find and execute handler
    const handler = this.handlers.get(event.type);

    if (handler) {
      try {
        await handler(event);
        await this.db.markEventProcessed(event.id);
        logger.success('Webhook event processed', { type: event.type, id: event.id });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        await this.db.markEventProcessed(event.id, message);
        logger.error('Webhook event processing failed', { type: event.type, id: event.id, error: message });
        throw error;
      }
    } else {
      // Store unhandled events but mark as processed
      await this.db.markEventProcessed(event.id);
      logger.debug('No handler for event type', { type: event.type });
    }
  }

  // =========================================================================
  // Customer Handlers
  // =========================================================================

  private async handleCustomerCreated(event: Stripe.Event): Promise<void> {
    const customer = event.data.object as Stripe.Customer;
    await this.syncService.syncSingleResource('customer', customer.id);
  }

  private async handleCustomerUpdated(event: Stripe.Event): Promise<void> {
    const customer = event.data.object as Stripe.Customer;
    await this.syncService.syncSingleResource('customer', customer.id);
  }

  private async handleCustomerDeleted(event: Stripe.Event): Promise<void> {
    const customer = event.data.object as Stripe.Customer;
    await this.db.markCustomerDeleted(customer.id);
  }

  private async handleCustomerTaxIdCreated(event: Stripe.Event): Promise<void> {
    const taxId = event.data.object as Stripe.TaxId;
    const customerId = typeof taxId.customer === 'string' ? taxId.customer : taxId.customer?.id;
    if (customerId) {
      const taxIds = await this.client.listAllTaxIds(customerId);
      await this.db.upsertTaxIds(taxIds);
    }
  }

  private async handleCustomerTaxIdDeleted(event: Stripe.Event): Promise<void> {
    const taxId = event.data.object as Stripe.TaxId;
    await this.db.execute('DELETE FROM stripe_tax_ids WHERE id = $1', [taxId.id]);
  }

  private async handleCustomerTaxIdUpdated(event: Stripe.Event): Promise<void> {
    const taxId = event.data.object as Stripe.TaxId;
    const customerId = typeof taxId.customer === 'string' ? taxId.customer : taxId.customer?.id;
    if (customerId) {
      const taxIds = await this.client.listAllTaxIds(customerId);
      await this.db.upsertTaxIds(taxIds);
    }
  }

  // =========================================================================
  // Product Handlers
  // =========================================================================

  private async handleProductCreated(event: Stripe.Event): Promise<void> {
    const product = event.data.object as Stripe.Product;
    await this.syncService.syncSingleResource('product', product.id);
  }

  private async handleProductUpdated(event: Stripe.Event): Promise<void> {
    const product = event.data.object as Stripe.Product;
    await this.syncService.syncSingleResource('product', product.id);
  }

  private async handleProductDeleted(event: Stripe.Event): Promise<void> {
    const product = event.data.object as Stripe.Product;
    await this.db.execute('UPDATE stripe_products SET deleted_at = NOW() WHERE id = $1', [product.id]);
  }

  // =========================================================================
  // Price Handlers
  // =========================================================================

  private async handlePriceCreated(event: Stripe.Event): Promise<void> {
    const price = event.data.object as Stripe.Price;
    await this.syncService.syncSingleResource('price', price.id);
  }

  private async handlePriceUpdated(event: Stripe.Event): Promise<void> {
    const price = event.data.object as Stripe.Price;
    await this.syncService.syncSingleResource('price', price.id);
  }

  private async handlePriceDeleted(event: Stripe.Event): Promise<void> {
    const price = event.data.object as Stripe.Price;
    await this.db.execute('UPDATE stripe_prices SET deleted_at = NOW() WHERE id = $1', [price.id]);
  }

  // =========================================================================
  // Coupon Handlers
  // =========================================================================

  private async handleCouponCreated(event: Stripe.Event): Promise<void> {
    const coupon = event.data.object as Stripe.Coupon;
    await this.syncService.syncSingleResource('coupon', coupon.id);
  }

  private async handleCouponUpdated(event: Stripe.Event): Promise<void> {
    const coupon = event.data.object as Stripe.Coupon;
    await this.syncService.syncSingleResource('coupon', coupon.id);
  }

  private async handleCouponDeleted(event: Stripe.Event): Promise<void> {
    const coupon = event.data.object as Stripe.Coupon;
    await this.db.execute('UPDATE stripe_coupons SET deleted_at = NOW() WHERE id = $1', [coupon.id]);
  }

  // =========================================================================
  // Promotion Code Handlers
  // =========================================================================

  private async handlePromotionCodeCreated(event: Stripe.Event): Promise<void> {
    const code = event.data.object as Stripe.PromotionCode;
    await this.syncService.syncSingleResource('promotion_code', code.id);
  }

  private async handlePromotionCodeUpdated(event: Stripe.Event): Promise<void> {
    const code = event.data.object as Stripe.PromotionCode;
    await this.syncService.syncSingleResource('promotion_code', code.id);
  }

  private async handlePromotionCodeDeleted(event: Stripe.Event): Promise<void> {
    const code = event.data.object as Stripe.PromotionCode;
    await this.db.execute('DELETE FROM stripe_promotion_codes WHERE id = $1', [code.id]);
  }

  // =========================================================================
  // Subscription Handlers
  // =========================================================================

  private async handleSubscriptionCreated(event: Stripe.Event): Promise<void> {
    const subscription = event.data.object as Stripe.Subscription;
    await this.syncService.syncSingleResource('subscription', subscription.id);
  }

  private async handleSubscriptionUpdated(event: Stripe.Event): Promise<void> {
    const subscription = event.data.object as Stripe.Subscription;
    await this.syncService.syncSingleResource('subscription', subscription.id);
  }

  private async handleSubscriptionDeleted(event: Stripe.Event): Promise<void> {
    const subscription = event.data.object as Stripe.Subscription;
    await this.syncService.syncSingleResource('subscription', subscription.id);
  }

  private async handleSubscriptionTrialWillEnd(event: Stripe.Event): Promise<void> {
    const subscription = event.data.object as Stripe.Subscription;
    await this.syncService.syncSingleResource('subscription', subscription.id);
    logger.info('Subscription trial ending soon', {
      subscriptionId: subscription.id,
      trialEnd: subscription.trial_end,
    });
  }

  private async handleSubscriptionPaused(event: Stripe.Event): Promise<void> {
    const subscription = event.data.object as Stripe.Subscription;
    await this.syncService.syncSingleResource('subscription', subscription.id);
  }

  private async handleSubscriptionResumed(event: Stripe.Event): Promise<void> {
    const subscription = event.data.object as Stripe.Subscription;
    await this.syncService.syncSingleResource('subscription', subscription.id);
  }

  private async handleSubscriptionPendingUpdateApplied(event: Stripe.Event): Promise<void> {
    const subscription = event.data.object as Stripe.Subscription;
    await this.syncService.syncSingleResource('subscription', subscription.id);
  }

  private async handleSubscriptionPendingUpdateExpired(event: Stripe.Event): Promise<void> {
    const subscription = event.data.object as Stripe.Subscription;
    await this.syncService.syncSingleResource('subscription', subscription.id);
  }

  // =========================================================================
  // Subscription Schedule Handlers
  // =========================================================================

  private async handleSubscriptionScheduleAborted(event: Stripe.Event): Promise<void> {
    const schedule = event.data.object as Stripe.SubscriptionSchedule;
    await this.syncService.syncSingleResource('subscription_schedule', schedule.id);
  }

  private async handleSubscriptionScheduleCanceled(event: Stripe.Event): Promise<void> {
    const schedule = event.data.object as Stripe.SubscriptionSchedule;
    await this.syncService.syncSingleResource('subscription_schedule', schedule.id);
  }

  private async handleSubscriptionScheduleCompleted(event: Stripe.Event): Promise<void> {
    const schedule = event.data.object as Stripe.SubscriptionSchedule;
    await this.syncService.syncSingleResource('subscription_schedule', schedule.id);
  }

  private async handleSubscriptionScheduleCreated(event: Stripe.Event): Promise<void> {
    const schedule = event.data.object as Stripe.SubscriptionSchedule;
    await this.syncService.syncSingleResource('subscription_schedule', schedule.id);
  }

  private async handleSubscriptionScheduleExpiring(event: Stripe.Event): Promise<void> {
    const schedule = event.data.object as Stripe.SubscriptionSchedule;
    await this.syncService.syncSingleResource('subscription_schedule', schedule.id);
    logger.info('Subscription schedule expiring soon', { scheduleId: schedule.id });
  }

  private async handleSubscriptionScheduleReleased(event: Stripe.Event): Promise<void> {
    const schedule = event.data.object as Stripe.SubscriptionSchedule;
    await this.syncService.syncSingleResource('subscription_schedule', schedule.id);
  }

  private async handleSubscriptionScheduleUpdated(event: Stripe.Event): Promise<void> {
    const schedule = event.data.object as Stripe.SubscriptionSchedule;
    await this.syncService.syncSingleResource('subscription_schedule', schedule.id);
  }

  // =========================================================================
  // Invoice Handlers
  // =========================================================================

  private async handleInvoiceCreated(event: Stripe.Event): Promise<void> {
    const invoice = event.data.object as Stripe.Invoice;
    await this.syncService.syncSingleResource('invoice', invoice.id);
  }

  private async handleInvoiceUpdated(event: Stripe.Event): Promise<void> {
    const invoice = event.data.object as Stripe.Invoice;
    await this.syncService.syncSingleResource('invoice', invoice.id);
  }

  private async handleInvoiceDeleted(event: Stripe.Event): Promise<void> {
    const invoice = event.data.object as Stripe.Invoice;
    await this.db.execute('DELETE FROM stripe_invoices WHERE id = $1', [invoice.id]);
  }

  private async handleInvoiceFinalized(event: Stripe.Event): Promise<void> {
    const invoice = event.data.object as Stripe.Invoice;
    await this.syncService.syncSingleResource('invoice', invoice.id);
  }

  private async handleInvoicePaid(event: Stripe.Event): Promise<void> {
    const invoice = event.data.object as Stripe.Invoice;
    await this.syncService.syncSingleResource('invoice', invoice.id);
    logger.info('Invoice paid', {
      invoiceId: invoice.id,
      amount: invoice.amount_paid,
      currency: invoice.currency,
    });
  }

  private async handleInvoicePaymentFailed(event: Stripe.Event): Promise<void> {
    const invoice = event.data.object as Stripe.Invoice;
    await this.syncService.syncSingleResource('invoice', invoice.id);
    logger.warn('Invoice payment failed', {
      invoiceId: invoice.id,
      attemptCount: invoice.attempt_count,
    });
  }

  private async handleInvoicePaymentSucceeded(event: Stripe.Event): Promise<void> {
    const invoice = event.data.object as Stripe.Invoice;
    await this.syncService.syncSingleResource('invoice', invoice.id);
  }

  private async handleInvoiceVoided(event: Stripe.Event): Promise<void> {
    const invoice = event.data.object as Stripe.Invoice;
    await this.syncService.syncSingleResource('invoice', invoice.id);
  }

  private async handleInvoiceMarkedUncollectible(event: Stripe.Event): Promise<void> {
    const invoice = event.data.object as Stripe.Invoice;
    await this.syncService.syncSingleResource('invoice', invoice.id);
  }

  private async handleInvoiceSent(event: Stripe.Event): Promise<void> {
    const invoice = event.data.object as Stripe.Invoice;
    await this.syncService.syncSingleResource('invoice', invoice.id);
  }

  private async handleInvoiceUpcoming(_event: Stripe.Event): Promise<void> {
    // Upcoming invoices don't have an ID yet, just log
    logger.info('Upcoming invoice notification received');
  }

  private async handleInvoicePaymentActionRequired(event: Stripe.Event): Promise<void> {
    const invoice = event.data.object as Stripe.Invoice;
    await this.syncService.syncSingleResource('invoice', invoice.id);
    logger.info('Invoice payment action required', { invoiceId: invoice.id });
  }

  // =========================================================================
  // Invoice Item Handlers
  // =========================================================================

  private async handleInvoiceItemCreated(event: Stripe.Event): Promise<void> {
    const item = event.data.object as Stripe.InvoiceItem;
    await this.syncService.syncSingleResource('invoice_item', item.id);
  }

  private async handleInvoiceItemDeleted(event: Stripe.Event): Promise<void> {
    const item = event.data.object as Stripe.InvoiceItem;
    await this.db.execute('DELETE FROM stripe_invoice_items WHERE id = $1', [item.id]);
  }

  // =========================================================================
  // Credit Note Handlers
  // =========================================================================

  private async handleCreditNoteCreated(event: Stripe.Event): Promise<void> {
    const creditNote = event.data.object as Stripe.CreditNote;
    await this.syncService.syncSingleResource('credit_note', creditNote.id);
  }

  private async handleCreditNoteUpdated(event: Stripe.Event): Promise<void> {
    const creditNote = event.data.object as Stripe.CreditNote;
    await this.syncService.syncSingleResource('credit_note', creditNote.id);
  }

  private async handleCreditNoteVoided(event: Stripe.Event): Promise<void> {
    const creditNote = event.data.object as Stripe.CreditNote;
    await this.syncService.syncSingleResource('credit_note', creditNote.id);
  }

  // =========================================================================
  // Charge Handlers
  // =========================================================================

  private async handleChargeCaptured(event: Stripe.Event): Promise<void> {
    const charge = event.data.object as Stripe.Charge;
    await this.syncService.syncSingleResource('charge', charge.id);
    logger.info('Charge captured', { chargeId: charge.id, amount: charge.amount_captured });
  }

  private async handleChargeExpired(event: Stripe.Event): Promise<void> {
    const charge = event.data.object as Stripe.Charge;
    await this.syncService.syncSingleResource('charge', charge.id);
  }

  private async handleChargeFailed(event: Stripe.Event): Promise<void> {
    const charge = event.data.object as Stripe.Charge;
    await this.syncService.syncSingleResource('charge', charge.id);
    logger.warn('Charge failed', {
      chargeId: charge.id,
      failureCode: charge.failure_code,
      failureMessage: charge.failure_message,
    });
  }

  private async handleChargePending(event: Stripe.Event): Promise<void> {
    const charge = event.data.object as Stripe.Charge;
    await this.syncService.syncSingleResource('charge', charge.id);
  }

  private async handleChargeRefunded(event: Stripe.Event): Promise<void> {
    const charge = event.data.object as Stripe.Charge;
    await this.syncService.syncSingleResource('charge', charge.id);
    logger.info('Charge refunded', {
      chargeId: charge.id,
      amountRefunded: charge.amount_refunded,
    });
  }

  private async handleChargeSucceeded(event: Stripe.Event): Promise<void> {
    const charge = event.data.object as Stripe.Charge;
    await this.syncService.syncSingleResource('charge', charge.id);
    logger.info('Charge succeeded', {
      chargeId: charge.id,
      amount: charge.amount,
      currency: charge.currency,
    });
  }

  private async handleChargeUpdated(event: Stripe.Event): Promise<void> {
    const charge = event.data.object as Stripe.Charge;
    await this.syncService.syncSingleResource('charge', charge.id);
  }

  // =========================================================================
  // Refund Handlers
  // =========================================================================

  private async handleRefundCreated(event: Stripe.Event): Promise<void> {
    const refund = event.data.object as Stripe.Refund;
    await this.syncService.syncSingleResource('refund', refund.id);
    logger.info('Refund created', { refundId: refund.id, amount: refund.amount });
  }

  private async handleRefundUpdated(event: Stripe.Event): Promise<void> {
    const refund = event.data.object as Stripe.Refund;
    await this.syncService.syncSingleResource('refund', refund.id);
  }

  private async handleRefundFailed(event: Stripe.Event): Promise<void> {
    const refund = event.data.object as Stripe.Refund;
    await this.syncService.syncSingleResource('refund', refund.id);
    logger.warn('Refund failed', { refundId: refund.id, reason: refund.failure_reason });
  }

  // =========================================================================
  // Dispute Handlers
  // =========================================================================

  private async handleDisputeCreated(event: Stripe.Event): Promise<void> {
    const dispute = event.data.object as Stripe.Dispute;
    await this.syncService.syncSingleResource('dispute', dispute.id);
    logger.warn('Dispute created', {
      disputeId: dispute.id,
      chargeId: dispute.charge,
      amount: dispute.amount,
      reason: dispute.reason,
    });
  }

  private async handleDisputeUpdated(event: Stripe.Event): Promise<void> {
    const dispute = event.data.object as Stripe.Dispute;
    await this.syncService.syncSingleResource('dispute', dispute.id);
  }

  private async handleDisputeClosed(event: Stripe.Event): Promise<void> {
    const dispute = event.data.object as Stripe.Dispute;
    await this.syncService.syncSingleResource('dispute', dispute.id);
    logger.info('Dispute closed', { disputeId: dispute.id, status: dispute.status });
  }

  private async handleDisputeFundsReinstated(event: Stripe.Event): Promise<void> {
    const dispute = event.data.object as Stripe.Dispute;
    await this.syncService.syncSingleResource('dispute', dispute.id);
    logger.info('Dispute funds reinstated', { disputeId: dispute.id });
  }

  private async handleDisputeFundsWithdrawn(event: Stripe.Event): Promise<void> {
    const dispute = event.data.object as Stripe.Dispute;
    await this.syncService.syncSingleResource('dispute', dispute.id);
    logger.warn('Dispute funds withdrawn', { disputeId: dispute.id, amount: dispute.amount });
  }

  // =========================================================================
  // Payment Intent Handlers
  // =========================================================================

  private async handlePaymentIntentCreated(event: Stripe.Event): Promise<void> {
    const paymentIntent = event.data.object as Stripe.PaymentIntent;
    await this.syncService.syncSingleResource('payment_intent', paymentIntent.id);
  }

  private async handlePaymentIntentSucceeded(event: Stripe.Event): Promise<void> {
    const paymentIntent = event.data.object as Stripe.PaymentIntent;
    await this.syncService.syncSingleResource('payment_intent', paymentIntent.id);
    logger.info('Payment succeeded', {
      paymentIntentId: paymentIntent.id,
      amount: paymentIntent.amount_received,
      currency: paymentIntent.currency,
    });
  }

  private async handlePaymentIntentFailed(event: Stripe.Event): Promise<void> {
    const paymentIntent = event.data.object as Stripe.PaymentIntent;
    await this.syncService.syncSingleResource('payment_intent', paymentIntent.id);
    logger.warn('Payment failed', {
      paymentIntentId: paymentIntent.id,
      error: paymentIntent.last_payment_error?.message,
    });
  }

  private async handlePaymentIntentCanceled(event: Stripe.Event): Promise<void> {
    const paymentIntent = event.data.object as Stripe.PaymentIntent;
    await this.syncService.syncSingleResource('payment_intent', paymentIntent.id);
  }

  private async handlePaymentIntentProcessing(event: Stripe.Event): Promise<void> {
    const paymentIntent = event.data.object as Stripe.PaymentIntent;
    await this.syncService.syncSingleResource('payment_intent', paymentIntent.id);
  }

  private async handlePaymentIntentRequiresAction(event: Stripe.Event): Promise<void> {
    const paymentIntent = event.data.object as Stripe.PaymentIntent;
    await this.syncService.syncSingleResource('payment_intent', paymentIntent.id);
  }

  private async handlePaymentIntentAmountCapturableUpdated(event: Stripe.Event): Promise<void> {
    const paymentIntent = event.data.object as Stripe.PaymentIntent;
    await this.syncService.syncSingleResource('payment_intent', paymentIntent.id);
  }

  private async handlePaymentIntentPartiallyFunded(event: Stripe.Event): Promise<void> {
    const paymentIntent = event.data.object as Stripe.PaymentIntent;
    await this.syncService.syncSingleResource('payment_intent', paymentIntent.id);
  }

  // =========================================================================
  // Setup Intent Handlers
  // =========================================================================

  private async handleSetupIntentCreated(event: Stripe.Event): Promise<void> {
    const setupIntent = event.data.object as Stripe.SetupIntent;
    await this.syncService.syncSingleResource('setup_intent', setupIntent.id);
  }

  private async handleSetupIntentCanceled(event: Stripe.Event): Promise<void> {
    const setupIntent = event.data.object as Stripe.SetupIntent;
    await this.syncService.syncSingleResource('setup_intent', setupIntent.id);
  }

  private async handleSetupIntentRequiresAction(event: Stripe.Event): Promise<void> {
    const setupIntent = event.data.object as Stripe.SetupIntent;
    await this.syncService.syncSingleResource('setup_intent', setupIntent.id);
  }

  private async handleSetupIntentFailed(event: Stripe.Event): Promise<void> {
    const setupIntent = event.data.object as Stripe.SetupIntent;
    await this.syncService.syncSingleResource('setup_intent', setupIntent.id);
    logger.warn('Setup intent failed', {
      setupIntentId: setupIntent.id,
      error: setupIntent.last_setup_error?.message,
    });
  }

  private async handleSetupIntentSucceeded(event: Stripe.Event): Promise<void> {
    const setupIntent = event.data.object as Stripe.SetupIntent;
    await this.syncService.syncSingleResource('setup_intent', setupIntent.id);
    logger.info('Setup intent succeeded', { setupIntentId: setupIntent.id });
  }

  // =========================================================================
  // Payment Method Handlers
  // =========================================================================

  private async handlePaymentMethodAttached(event: Stripe.Event): Promise<void> {
    const paymentMethod = event.data.object as Stripe.PaymentMethod;
    await this.syncService.syncSingleResource('payment_method', paymentMethod.id);
  }

  private async handlePaymentMethodDetached(event: Stripe.Event): Promise<void> {
    const paymentMethod = event.data.object as Stripe.PaymentMethod;
    await this.db.execute(
      'UPDATE stripe_payment_methods SET customer_id = NULL, updated_at = NOW() WHERE id = $1',
      [paymentMethod.id]
    );
  }

  private async handlePaymentMethodUpdated(event: Stripe.Event): Promise<void> {
    const paymentMethod = event.data.object as Stripe.PaymentMethod;
    await this.syncService.syncSingleResource('payment_method', paymentMethod.id);
  }

  private async handlePaymentMethodAutomaticallyUpdated(event: Stripe.Event): Promise<void> {
    const paymentMethod = event.data.object as Stripe.PaymentMethod;
    await this.syncService.syncSingleResource('payment_method', paymentMethod.id);
  }

  // =========================================================================
  // Checkout Session Handlers
  // =========================================================================

  private async handleCheckoutSessionCompleted(event: Stripe.Event): Promise<void> {
    const session = event.data.object as Stripe.Checkout.Session;
    await this.syncService.syncSingleResource('checkout_session', session.id);
    logger.info('Checkout session completed', {
      sessionId: session.id,
      customerId: session.customer,
      subscriptionId: session.subscription,
    });

    // Sync related resources
    if (session.customer && typeof session.customer === 'string') {
      await this.syncService.syncSingleResource('customer', session.customer);
    }
    if (session.subscription && typeof session.subscription === 'string') {
      await this.syncService.syncSingleResource('subscription', session.subscription);
    }
    if (session.payment_intent && typeof session.payment_intent === 'string') {
      await this.syncService.syncSingleResource('payment_intent', session.payment_intent);
    }
  }

  private async handleCheckoutAsyncPaymentSucceeded(event: Stripe.Event): Promise<void> {
    const session = event.data.object as Stripe.Checkout.Session;
    await this.syncService.syncSingleResource('checkout_session', session.id);
    logger.info('Checkout async payment succeeded', { sessionId: session.id });
  }

  private async handleCheckoutAsyncPaymentFailed(event: Stripe.Event): Promise<void> {
    const session = event.data.object as Stripe.Checkout.Session;
    await this.syncService.syncSingleResource('checkout_session', session.id);
    logger.warn('Checkout async payment failed', { sessionId: session.id });
  }

  private async handleCheckoutSessionExpired(event: Stripe.Event): Promise<void> {
    const session = event.data.object as Stripe.Checkout.Session;
    await this.syncService.syncSingleResource('checkout_session', session.id);
    logger.info('Checkout session expired', { sessionId: session.id });
  }

  // =========================================================================
  // Balance Handlers
  // =========================================================================

  private async handleBalanceAvailable(_event: Stripe.Event): Promise<void> {
    // Balance events are informational, just log
    logger.info('Balance available updated');
  }

  // =========================================================================
  // Tax Rate Handlers
  // =========================================================================

  private async handleTaxRateCreated(event: Stripe.Event): Promise<void> {
    const taxRate = event.data.object as Stripe.TaxRate;
    await this.syncService.syncSingleResource('tax_rate', taxRate.id);
  }

  private async handleTaxRateUpdated(event: Stripe.Event): Promise<void> {
    const taxRate = event.data.object as Stripe.TaxRate;
    await this.syncService.syncSingleResource('tax_rate', taxRate.id);
  }

  // =========================================================================
  // Payout Handlers (informational only)
  // =========================================================================

  private async handlePayoutEvent(event: Stripe.Event): Promise<void> {
    const payout = event.data.object as Stripe.Payout;
    logger.info('Payout event', { type: event.type, payoutId: payout.id, status: payout.status });
  }
}
