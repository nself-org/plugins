/**
 * PayPal Webhook Handler
 * Postback verification and event processing
 */

import { createLogger } from '@nself/plugin-utils';
import type { PayPalClient } from './client.js';
import type { PayPalDatabase } from './database.js';
import type { PayPalSyncService } from './sync.js';
import type {
  PayPalWebhookEvent,
  PayPalCapture,
  PayPalRefund,
  PayPalOrder,
  PayPalSubscription,
  PayPalDispute,
  PayPalPayout,
  PayPalInvoice,
  CaptureRecord,
  RefundRecord,
  OrderRecord,
  SubscriptionRecord,
  DisputeRecord,
  PayoutRecord,
  InvoiceRecord,
} from './types.js';

const logger = createLogger('paypal:webhooks');

type EventHandler = (event: PayPalWebhookEvent) => Promise<void>;

export class PayPalWebhookHandler {
  private handlers: Map<string, EventHandler>;

  constructor(
    _client: PayPalClient,
    private db: PayPalDatabase,
    _syncService: PayPalSyncService,
  ) {
    this.handlers = new Map();
    this.registerHandlers();
  }

  private registerHandlers(): void {
    // Payment Captures
    this.handlers.set('PAYMENT.CAPTURE.COMPLETED', (e) => this.handleCapture(e));
    this.handlers.set('PAYMENT.CAPTURE.DENIED', (e) => this.handleCapture(e));
    this.handlers.set('PAYMENT.CAPTURE.REFUNDED', (e) => this.handleCapture(e));
    this.handlers.set('PAYMENT.CAPTURE.REVERSED', (e) => this.handleCapture(e));
    this.handlers.set('PAYMENT.CAPTURE.PENDING', (e) => this.handleCapture(e));

    // Checkout Orders
    this.handlers.set('CHECKOUT.ORDER.COMPLETED', (e) => this.handleOrder(e));
    this.handlers.set('CHECKOUT.ORDER.APPROVED', (e) => this.handleOrder(e));
    this.handlers.set('CHECKOUT.ORDER.VOIDED', (e) => this.handleOrder(e));

    // Subscriptions
    this.handlers.set('BILLING.SUBSCRIPTION.CREATED', (e) => this.handleSubscription(e));
    this.handlers.set('BILLING.SUBSCRIPTION.ACTIVATED', (e) => this.handleSubscription(e));
    this.handlers.set('BILLING.SUBSCRIPTION.UPDATED', (e) => this.handleSubscription(e));
    this.handlers.set('BILLING.SUBSCRIPTION.CANCELLED', (e) => this.handleSubscription(e));
    this.handlers.set('BILLING.SUBSCRIPTION.SUSPENDED', (e) => this.handleSubscription(e));
    this.handlers.set('BILLING.SUBSCRIPTION.EXPIRED', (e) => this.handleSubscription(e));

    // Disputes
    this.handlers.set('CUSTOMER.DISPUTE.CREATED', (e) => this.handleDispute(e));
    this.handlers.set('CUSTOMER.DISPUTE.UPDATED', (e) => this.handleDispute(e));
    this.handlers.set('CUSTOMER.DISPUTE.RESOLVED', (e) => this.handleDispute(e));
    this.handlers.set('CUSTOMER.DISPUTE.OTHER', (e) => this.handleDispute(e));

    // Payouts
    this.handlers.set('PAYMENT.PAYOUTSBATCH.SUCCESS', (e) => this.handlePayout(e));
    this.handlers.set('PAYMENT.PAYOUTSBATCH.DENIED', (e) => this.handlePayout(e));
    this.handlers.set('PAYMENT.PAYOUTSBATCH.PROCESSING', (e) => this.handlePayout(e));

    // Invoices
    this.handlers.set('INVOICING.INVOICE.PAID', (e) => this.handleInvoice(e));
    this.handlers.set('INVOICING.INVOICE.CANCELLED', (e) => this.handleInvoice(e));

    // Refunds
    this.handlers.set('PAYMENT.SALE.REFUNDED', (e) => this.handleRefund(e));
  }

  async handleEvent(event: PayPalWebhookEvent): Promise<void> {
    // Store raw event
    await this.db.insertWebhookEvent({
      id: event.id,
      event_type: event.event_type,
      resource_type: event.resource_type,
      summary: event.summary,
      resource: event.resource,
      created_at: new Date(event.create_time),
    });

    try {
      const handler = this.handlers.get(event.event_type);
      if (handler) {
        await handler(event);
        logger.info('Webhook event processed', { type: event.event_type, id: event.id });
      } else {
        logger.debug('No handler for event type', { type: event.event_type });
      }

      await this.db.markEventProcessed(event.id);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      await this.db.markEventProcessed(event.id, message);
      throw error;
    }
  }

  // ─── Event Handlers ────────────────────────────────────────────────────

  private async handleCapture(event: PayPalWebhookEvent): Promise<void> {
    const resource = event.resource as unknown as PayPalCapture;
    const record: CaptureRecord = {
      id: resource.id,
      source_account_id: 'primary',
      order_id: null,
      status: resource.status,
      amount: parseFloat(resource.amount?.value ?? '0'),
      currency: resource.amount?.currency_code ?? 'USD',
      fee_amount: resource.seller_receivable_breakdown?.paypal_fee
        ? parseFloat(resource.seller_receivable_breakdown.paypal_fee.value) : null,
      net_amount: resource.seller_receivable_breakdown?.net_amount
        ? parseFloat(resource.seller_receivable_breakdown.net_amount.value) : null,
      final_capture: resource.final_capture ?? false,
      invoice_id: resource.invoice_id ?? null,
      custom_id: resource.custom_id ?? null,
      seller_protection: resource.seller_protection?.status ?? null,
      created_at: resource.create_time ? new Date(resource.create_time) : null,
      updated_at: resource.update_time ? new Date(resource.update_time) : null,
      synced_at: new Date(),
    };
    await this.db.upsertCaptures([record]);
  }

  private async handleOrder(event: PayPalWebhookEvent): Promise<void> {
    const resource = event.resource as unknown as PayPalOrder;
    const purchaseUnit = resource.purchase_units?.[0];
    const record: OrderRecord = {
      id: resource.id,
      source_account_id: 'primary',
      status: resource.status,
      intent: resource.intent ?? '',
      payer_email: resource.payer?.email_address ?? null,
      payer_id: resource.payer?.payer_id ?? null,
      payer_name: resource.payer?.name?.full_name ??
        ([resource.payer?.name?.given_name, resource.payer?.name?.surname].filter(Boolean).join(' ') || null),
      total_amount: purchaseUnit ? parseFloat(purchaseUnit.amount.value) : 0,
      currency: purchaseUnit?.amount.currency_code ?? 'USD',
      description: purchaseUnit?.description ?? null,
      metadata: {},
      created_at: resource.create_time ? new Date(resource.create_time) : null,
      updated_at: resource.update_time ? new Date(resource.update_time) : null,
      synced_at: new Date(),
    };
    await this.db.upsertOrders([record]);
  }

  private async handleSubscription(event: PayPalWebhookEvent): Promise<void> {
    const resource = event.resource as unknown as PayPalSubscription;
    const record: SubscriptionRecord = {
      id: resource.id,
      source_account_id: 'primary',
      plan_id: resource.plan_id ?? '',
      status: resource.status,
      subscriber_email: resource.subscriber?.email_address ?? null,
      subscriber_payer_id: resource.subscriber?.payer_id ?? null,
      subscriber_name: resource.subscriber?.name?.full_name ??
        ([resource.subscriber?.name?.given_name, resource.subscriber?.name?.surname].filter(Boolean).join(' ') || null),
      start_time: resource.start_time ? new Date(resource.start_time) : null,
      quantity: resource.quantity ?? null,
      outstanding_balance: resource.billing_info?.outstanding_balance
        ? parseFloat(resource.billing_info.outstanding_balance.value) : null,
      last_payment_amount: resource.billing_info?.last_payment
        ? parseFloat(resource.billing_info.last_payment.amount.value) : null,
      last_payment_time: resource.billing_info?.last_payment?.time
        ? new Date(resource.billing_info.last_payment.time) : null,
      next_billing_time: resource.billing_info?.next_billing_time
        ? new Date(resource.billing_info.next_billing_time) : null,
      failed_payments_count: resource.billing_info?.failed_payments_count ?? 0,
      currency: resource.billing_info?.outstanding_balance?.currency_code ?? null,
      metadata: {},
      created_at: resource.create_time ? new Date(resource.create_time) : null,
      updated_at: resource.update_time ? new Date(resource.update_time) : null,
      synced_at: new Date(),
    };
    await this.db.upsertSubscriptions([record]);
  }

  private async handleDispute(event: PayPalWebhookEvent): Promise<void> {
    const resource = event.resource as unknown as PayPalDispute;
    const record: DisputeRecord = {
      id: resource.dispute_id,
      source_account_id: 'primary',
      reason: resource.reason ?? '',
      status: resource.status ?? '',
      amount: parseFloat(resource.dispute_amount?.value ?? '0'),
      currency: resource.dispute_amount?.currency_code ?? 'USD',
      outcome_code: resource.dispute_outcome?.outcome_code ?? null,
      refunded_amount: resource.dispute_outcome?.amount_refunded
        ? parseFloat(resource.dispute_outcome.amount_refunded.value) : null,
      life_cycle_stage: resource.dispute_life_cycle_stage ?? null,
      channel: resource.dispute_channel ?? null,
      seller_transaction_id: resource.disputed_transactions?.[0]?.seller_transaction_id ?? null,
      buyer_transaction_id: resource.disputed_transactions?.[0]?.buyer_transaction_id ?? null,
      metadata: {},
      created_at: resource.create_time ? new Date(resource.create_time) : null,
      updated_at: resource.update_time ? new Date(resource.update_time) : null,
      synced_at: new Date(),
    };
    await this.db.upsertDisputes([record]);
  }

  private async handlePayout(event: PayPalWebhookEvent): Promise<void> {
    const resource = event.resource as unknown as PayPalPayout;
    const header = resource.batch_header;
    const record: PayoutRecord = {
      id: header.payout_batch_id,
      source_account_id: 'primary',
      batch_status: header.batch_status,
      sender_batch_id: header.sender_batch_header?.sender_batch_id ?? null,
      email_subject: header.sender_batch_header?.email_subject ?? null,
      amount: header.amount ? parseFloat(header.amount.value) : null,
      currency: header.amount?.currency_code ?? null,
      fees: header.fees ? parseFloat(header.fees.value) : null,
      time_created: header.time_created ? new Date(header.time_created) : null,
      time_completed: header.time_completed ? new Date(header.time_completed) : null,
      synced_at: new Date(),
    };
    await this.db.upsertPayouts([record]);
  }

  private async handleInvoice(event: PayPalWebhookEvent): Promise<void> {
    const resource = event.resource as unknown as PayPalInvoice;
    const record: InvoiceRecord = {
      id: resource.id,
      source_account_id: 'primary',
      status: resource.status,
      invoice_number: resource.detail?.invoice_number ?? null,
      invoice_date: resource.detail?.invoice_date ?? null,
      currency: resource.detail?.currency_code ?? resource.amount?.currency_code ?? 'USD',
      recipient_email: resource.primary_recipients?.[0]?.billing_info?.email_address ?? null,
      recipient_name: resource.primary_recipients?.[0]?.billing_info?.name?.full_name ?? null,
      total_amount: resource.amount ? parseFloat(resource.amount.value) : null,
      due_amount: resource.due_amount ? parseFloat(resource.due_amount.value) : null,
      paid_amount: resource.payments?.paid_amount ? parseFloat(resource.payments.paid_amount.value) : null,
      note: resource.detail?.note ?? null,
      due_date: resource.detail?.payment_term?.due_date ?? null,
      metadata: {},
      synced_at: new Date(),
    };
    await this.db.upsertInvoices([record]);
  }

  private async handleRefund(event: PayPalWebhookEvent): Promise<void> {
    const resource = event.resource as unknown as PayPalRefund;
    const record: RefundRecord = {
      id: resource.id,
      source_account_id: 'primary',
      capture_id: null,
      status: resource.status,
      amount: parseFloat(resource.amount?.value ?? '0'),
      currency: resource.amount?.currency_code ?? 'USD',
      fee_amount: resource.seller_payable_breakdown?.paypal_fee
        ? parseFloat(resource.seller_payable_breakdown.paypal_fee.value) : null,
      net_amount: resource.seller_payable_breakdown?.net_amount
        ? parseFloat(resource.seller_payable_breakdown.net_amount.value) : null,
      invoice_id: resource.invoice_id ?? null,
      note_to_payer: resource.note_to_payer ?? null,
      created_at: resource.create_time ? new Date(resource.create_time) : null,
      updated_at: resource.update_time ? new Date(resource.update_time) : null,
      synced_at: new Date(),
    };
    await this.db.upsertRefunds([record]);
  }
}
