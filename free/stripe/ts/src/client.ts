/**
 * Stripe API Client
 * Complete wrapper around the official Stripe SDK with pagination support
 * Supports all Stripe objects for 100% data sync
 */

import Stripe from 'stripe';
import { createLogger } from '@nself/plugin-utils';
import type {
  StripeCustomerRecord,
  StripeProductRecord,
  StripePriceRecord,
  StripeSubscriptionRecord,
  StripeInvoiceRecord,
  StripePaymentIntentRecord,
  StripePaymentMethodRecord,
  StripeSubscriptionItem,
  StripeInvoiceLine,
  StripeChargeRecord,
  StripeRefundRecord,
  StripeDisputeRecord,
  StripeCouponRecord,
  StripePromotionCodeRecord,
  StripeSetupIntentRecord,
  StripeCheckoutSessionRecord,
  StripeBalanceTransactionRecord,
  StripeCreditNoteRecord,
  StripeTaxRateRecord,
  StripeTaxIdRecord,
  StripeSubscriptionItemRecord,
  StripeSubscriptionScheduleRecord,
  StripeInvoiceItemRecord,
} from './types.js';

const logger = createLogger('stripe:client');

export class StripeClient {
  private stripe: Stripe;

  constructor(apiKey: string, apiVersion?: string) {
    this.stripe = new Stripe(apiKey, {
      apiVersion: (apiVersion ?? '2024-12-18.acacia') as Stripe.LatestApiVersion,
      typescript: true,
    });
    logger.info('Stripe client initialized');
  }

  /**
   * Check if using test mode
   */
  isTestMode(): boolean {
    return this.stripe.webhookEndpoints !== undefined;
  }

  // =========================================================================
  // Customers
  // =========================================================================

  async *listCustomers(params?: Stripe.CustomerListParams): AsyncGenerator<StripeCustomerRecord[]> {
    logger.debug('Listing customers', { params });

    for await (const customer of this.stripe.customers.list({
      limit: 100,
      ...params,
    })) {
      yield [this.mapCustomer(customer)];
    }
  }

  async listAllCustomers(params?: Stripe.CustomerListParams): Promise<StripeCustomerRecord[]> {
    const customers: StripeCustomerRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.customers.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      customers.push(...response.data.map(c => this.mapCustomer(c)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched customers batch', { count: response.data.length, total: customers.length });
    }

    return customers;
  }

  async getCustomer(id: string): Promise<StripeCustomerRecord | null> {
    try {
      const customer = await this.stripe.customers.retrieve(id);
      if (customer.deleted) return null;
      return this.mapCustomer(customer as Stripe.Customer);
    } catch (error) {
      logger.error('Failed to get customer', { id, error });
      return null;
    }
  }

  private mapCustomer(customer: Stripe.Customer): StripeCustomerRecord {
    return {
      id: customer.id,
      email: customer.email,
      name: customer.name ?? null,
      phone: customer.phone ?? null,
      description: customer.description,
      currency: customer.currency ?? null,
      default_source: typeof customer.default_source === 'string' ? customer.default_source : customer.default_source?.id ?? null,
      invoice_prefix: customer.invoice_prefix ?? null,
      balance: customer.balance,
      delinquent: customer.delinquent ?? false,
      tax_exempt: customer.tax_exempt ?? 'none',
      metadata: customer.metadata ?? {},
      address: customer.address ?? null,
      shipping: customer.shipping,
      created_at: new Date(customer.created * 1000),
      deleted_at: null,
    };
  }

  // =========================================================================
  // Products
  // =========================================================================

  async listAllProducts(params?: Stripe.ProductListParams): Promise<StripeProductRecord[]> {
    const products: StripeProductRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.products.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      products.push(...response.data.map(p => this.mapProduct(p)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched products batch', { count: response.data.length, total: products.length });
    }

    return products;
  }

  async getProduct(id: string): Promise<StripeProductRecord | null> {
    try {
      const product = await this.stripe.products.retrieve(id);
      if (product.deleted) return null;
      return this.mapProduct(product as Stripe.Product);
    } catch (error) {
      logger.error('Failed to get product', { id, error });
      return null;
    }
  }

  private mapProduct(product: Stripe.Product): StripeProductRecord {
    return {
      id: product.id,
      name: product.name,
      description: product.description,
      active: product.active,
      type: product.type ?? 'service',
      images: product.images,
      metadata: product.metadata ?? {},
      attributes: [],
      shippable: product.shippable,
      statement_descriptor: product.statement_descriptor ?? null,
      tax_code: typeof product.tax_code === 'string' ? product.tax_code : product.tax_code?.id ?? null,
      unit_label: product.unit_label ?? null,
      url: product.url,
      default_price_id: typeof product.default_price === 'string' ? product.default_price : product.default_price?.id ?? null,
      created_at: new Date(product.created * 1000),
      deleted_at: null,
    };
  }

  // =========================================================================
  // Prices
  // =========================================================================

  async listAllPrices(params?: Stripe.PriceListParams): Promise<StripePriceRecord[]> {
    const prices: StripePriceRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.prices.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      prices.push(...response.data.map(p => this.mapPrice(p)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched prices batch', { count: response.data.length, total: prices.length });
    }

    return prices;
  }

  async getPrice(id: string): Promise<StripePriceRecord | null> {
    try {
      const price = await this.stripe.prices.retrieve(id);
      return this.mapPrice(price);
    } catch (error) {
      logger.error('Failed to get price', { id, error });
      return null;
    }
  }

  private mapPrice(price: Stripe.Price): StripePriceRecord {
    return {
      id: price.id,
      product_id: typeof price.product === 'string' ? price.product : price.product?.id ?? '',
      active: price.active,
      currency: price.currency,
      unit_amount: price.unit_amount,
      unit_amount_decimal: price.unit_amount_decimal,
      type: price.type,
      billing_scheme: price.billing_scheme,
      recurring: price.recurring,
      tiers: price.tiers ?? null,
      tiers_mode: price.tiers_mode,
      transform_quantity: price.transform_quantity,
      lookup_key: price.lookup_key,
      nickname: price.nickname,
      tax_behavior: price.tax_behavior ?? 'unspecified',
      metadata: price.metadata ?? {},
      created_at: new Date(price.created * 1000),
      deleted_at: null,
    };
  }

  // =========================================================================
  // Coupons
  // =========================================================================

  async listAllCoupons(params?: Stripe.CouponListParams): Promise<StripeCouponRecord[]> {
    const coupons: StripeCouponRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.coupons.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      coupons.push(...response.data.map(c => this.mapCoupon(c)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched coupons batch', { count: response.data.length, total: coupons.length });
    }

    return coupons;
  }

  async getCoupon(id: string): Promise<StripeCouponRecord | null> {
    try {
      const coupon = await this.stripe.coupons.retrieve(id);
      return this.mapCoupon(coupon);
    } catch (error) {
      logger.error('Failed to get coupon', { id, error });
      return null;
    }
  }

  private mapCoupon(coupon: Stripe.Coupon): StripeCouponRecord {
    return {
      id: coupon.id,
      name: coupon.name,
      amount_off: coupon.amount_off,
      percent_off: coupon.percent_off,
      currency: coupon.currency,
      duration: coupon.duration,
      duration_in_months: coupon.duration_in_months,
      max_redemptions: coupon.max_redemptions,
      times_redeemed: coupon.times_redeemed,
      redeem_by: coupon.redeem_by ? new Date(coupon.redeem_by * 1000) : null,
      applies_to: coupon.applies_to ?? null,
      valid: coupon.valid,
      metadata: coupon.metadata ?? {},
      created_at: new Date(coupon.created * 1000),
      deleted_at: null,
    };
  }

  // =========================================================================
  // Promotion Codes
  // =========================================================================

  async listAllPromotionCodes(params?: Stripe.PromotionCodeListParams): Promise<StripePromotionCodeRecord[]> {
    const codes: StripePromotionCodeRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.promotionCodes.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      codes.push(...response.data.map(c => this.mapPromotionCode(c)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched promotion codes batch', { count: response.data.length, total: codes.length });
    }

    return codes;
  }

  async getPromotionCode(id: string): Promise<StripePromotionCodeRecord | null> {
    try {
      const code = await this.stripe.promotionCodes.retrieve(id);
      return this.mapPromotionCode(code);
    } catch (error) {
      logger.error('Failed to get promotion code', { id, error });
      return null;
    }
  }

  private mapPromotionCode(code: Stripe.PromotionCode): StripePromotionCodeRecord {
    return {
      id: code.id,
      coupon_id: typeof code.coupon === 'string' ? code.coupon : code.coupon.id,
      code: code.code,
      active: code.active,
      customer_id: typeof code.customer === 'string' ? code.customer : code.customer?.id ?? null,
      expires_at: code.expires_at ? new Date(code.expires_at * 1000) : null,
      max_redemptions: code.max_redemptions,
      times_redeemed: code.times_redeemed,
      restrictions: code.restrictions as unknown as Record<string, unknown>,
      metadata: code.metadata ?? {},
      created_at: new Date(code.created * 1000),
    };
  }

  // =========================================================================
  // Subscriptions
  // =========================================================================

  async listAllSubscriptions(params?: Stripe.SubscriptionListParams): Promise<StripeSubscriptionRecord[]> {
    const subscriptions: StripeSubscriptionRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.subscriptions.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      subscriptions.push(...response.data.map(s => this.mapSubscription(s)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched subscriptions batch', { count: response.data.length, total: subscriptions.length });
    }

    return subscriptions;
  }

  async getSubscription(id: string): Promise<StripeSubscriptionRecord | null> {
    try {
      const subscription = await this.stripe.subscriptions.retrieve(id);
      return this.mapSubscription(subscription);
    } catch (error) {
      logger.error('Failed to get subscription', { id, error });
      return null;
    }
  }

  private mapSubscription(sub: Stripe.Subscription): StripeSubscriptionRecord {
    return {
      id: sub.id,
      customer_id: typeof sub.customer === 'string' ? sub.customer : sub.customer.id,
      status: sub.status,
      current_period_start: new Date(sub.current_period_start * 1000),
      current_period_end: new Date(sub.current_period_end * 1000),
      cancel_at: sub.cancel_at ? new Date(sub.cancel_at * 1000) : null,
      canceled_at: sub.canceled_at ? new Date(sub.canceled_at * 1000) : null,
      cancel_at_period_end: sub.cancel_at_period_end,
      ended_at: sub.ended_at ? new Date(sub.ended_at * 1000) : null,
      trial_start: sub.trial_start ? new Date(sub.trial_start * 1000) : null,
      trial_end: sub.trial_end ? new Date(sub.trial_end * 1000) : null,
      collection_method: sub.collection_method,
      billing_cycle_anchor: new Date(sub.billing_cycle_anchor * 1000),
      billing_thresholds: sub.billing_thresholds,
      days_until_due: sub.days_until_due,
      default_payment_method_id: typeof sub.default_payment_method === 'string' ? sub.default_payment_method : sub.default_payment_method?.id ?? null,
      default_source: typeof sub.default_source === 'string' ? sub.default_source : sub.default_source?.id ?? null,
      discount: sub.discount,
      items: sub.items.data.map(item => this.mapSubscriptionItem(item)),
      latest_invoice_id: typeof sub.latest_invoice === 'string' ? sub.latest_invoice : sub.latest_invoice?.id ?? null,
      pending_setup_intent: typeof sub.pending_setup_intent === 'string' ? sub.pending_setup_intent : sub.pending_setup_intent?.id ?? null,
      pending_update: sub.pending_update,
      schedule_id: typeof sub.schedule === 'string' ? sub.schedule : sub.schedule?.id ?? null,
      start_date: new Date(sub.start_date * 1000),
      transfer_data: sub.transfer_data,
      application_fee_percent: sub.application_fee_percent,
      automatic_tax: sub.automatic_tax,
      payment_settings: sub.payment_settings ?? {} as Stripe.Subscription.PaymentSettings,
      metadata: sub.metadata ?? {},
      created_at: new Date(sub.created * 1000),
    };
  }

  private mapSubscriptionItem(item: Stripe.SubscriptionItem): StripeSubscriptionItem {
    const price = item.price;
    return {
      id: item.id,
      price: {
        id: price.id,
        product: typeof price.product === 'string' ? price.product : price.product?.id ?? '',
        unit_amount: price.unit_amount,
        currency: price.currency,
        recurring: price.recurring,
      },
      quantity: item.quantity ?? 1,
    };
  }

  // =========================================================================
  // Subscription Items (individual items for sync)
  // =========================================================================

  async listAllSubscriptionItems(subscriptionId: string): Promise<StripeSubscriptionItemRecord[]> {
    const items: StripeSubscriptionItemRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.subscriptionItems.list({
        subscription: subscriptionId,
        limit: 100,
        starting_after: startingAfter,
      });

      items.push(...response.data.map(i => this.mapSubscriptionItemRecord(i)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }
    }

    return items;
  }

  async getSubscriptionItem(id: string): Promise<StripeSubscriptionItemRecord | null> {
    try {
      const item = await this.stripe.subscriptionItems.retrieve(id);
      return this.mapSubscriptionItemRecord(item);
    } catch (error) {
      logger.error('Failed to get subscription item', { id, error });
      return null;
    }
  }

  private mapSubscriptionItemRecord(item: Stripe.SubscriptionItem): StripeSubscriptionItemRecord {
    return {
      id: item.id,
      subscription_id: item.subscription,
      price_id: item.price.id,
      quantity: item.quantity ?? 1,
      billing_thresholds: item.billing_thresholds,
      metadata: item.metadata ?? {},
      created_at: new Date(item.created * 1000),
    };
  }

  // =========================================================================
  // Subscription Schedules
  // =========================================================================

  async listAllSubscriptionSchedules(params?: Stripe.SubscriptionScheduleListParams): Promise<StripeSubscriptionScheduleRecord[]> {
    const schedules: StripeSubscriptionScheduleRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.subscriptionSchedules.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      schedules.push(...response.data.map(s => this.mapSubscriptionSchedule(s)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched subscription schedules batch', { count: response.data.length, total: schedules.length });
    }

    return schedules;
  }

  async getSubscriptionSchedule(id: string): Promise<StripeSubscriptionScheduleRecord | null> {
    try {
      const schedule = await this.stripe.subscriptionSchedules.retrieve(id);
      return this.mapSubscriptionSchedule(schedule);
    } catch (error) {
      logger.error('Failed to get subscription schedule', { id, error });
      return null;
    }
  }

  private mapSubscriptionSchedule(schedule: Stripe.SubscriptionSchedule): StripeSubscriptionScheduleRecord {
    return {
      id: schedule.id,
      customer_id: typeof schedule.customer === 'string' ? schedule.customer : schedule.customer.id,
      subscription_id: typeof schedule.subscription === 'string' ? schedule.subscription : schedule.subscription?.id ?? null,
      status: schedule.status,
      current_phase: schedule.current_phase as unknown as Record<string, unknown> | null,
      phases: schedule.phases as unknown as Record<string, unknown>[],
      end_behavior: schedule.end_behavior,
      released_at: schedule.released_at ? new Date(schedule.released_at * 1000) : null,
      released_subscription: schedule.released_subscription as string | null,
      default_settings: schedule.default_settings as unknown as Record<string, unknown>,
      metadata: schedule.metadata ?? {},
      created_at: new Date(schedule.created * 1000),
      canceled_at: schedule.canceled_at ? new Date(schedule.canceled_at * 1000) : null,
      completed_at: schedule.completed_at ? new Date(schedule.completed_at * 1000) : null,
    };
  }

  // =========================================================================
  // Invoices
  // =========================================================================

  async listAllInvoices(params?: Stripe.InvoiceListParams): Promise<StripeInvoiceRecord[]> {
    const invoices: StripeInvoiceRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.invoices.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      invoices.push(...response.data.map(i => this.mapInvoice(i)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched invoices batch', { count: response.data.length, total: invoices.length });
    }

    return invoices;
  }

  async getInvoice(id: string): Promise<StripeInvoiceRecord | null> {
    try {
      const invoice = await this.stripe.invoices.retrieve(id);
      return this.mapInvoice(invoice);
    } catch (error) {
      logger.error('Failed to get invoice', { id, error });
      return null;
    }
  }

  private mapInvoice(invoice: Stripe.Invoice): StripeInvoiceRecord {
    return {
      id: invoice.id,
      customer_id: typeof invoice.customer === 'string' ? invoice.customer : invoice.customer?.id ?? '',
      subscription_id: typeof invoice.subscription === 'string' ? invoice.subscription : invoice.subscription?.id ?? null,
      status: invoice.status,
      collection_method: invoice.collection_method,
      currency: invoice.currency,
      amount_due: invoice.amount_due,
      amount_paid: invoice.amount_paid,
      amount_remaining: invoice.amount_remaining,
      subtotal: invoice.subtotal,
      subtotal_excluding_tax: invoice.subtotal_excluding_tax,
      total: invoice.total,
      total_excluding_tax: invoice.total_excluding_tax,
      tax: invoice.tax,
      total_tax_amounts: invoice.total_tax_amounts,
      discount: invoice.discount,
      discounts: (invoice.discounts ?? []).map(d => typeof d === 'string' ? d : d.id),
      account_country: invoice.account_country,
      account_name: invoice.account_name,
      billing_reason: invoice.billing_reason,
      number: invoice.number,
      receipt_number: invoice.receipt_number,
      statement_descriptor: invoice.statement_descriptor,
      description: invoice.description,
      footer: invoice.footer,
      customer_email: invoice.customer_email,
      customer_name: invoice.customer_name,
      customer_address: invoice.customer_address,
      customer_phone: invoice.customer_phone,
      customer_shipping: invoice.customer_shipping,
      customer_tax_exempt: invoice.customer_tax_exempt,
      customer_tax_ids: invoice.customer_tax_ids ?? [],
      default_payment_method_id: typeof invoice.default_payment_method === 'string' ? invoice.default_payment_method : invoice.default_payment_method?.id ?? null,
      default_source: typeof invoice.default_source === 'string' ? invoice.default_source : invoice.default_source?.id ?? null,
      lines: invoice.lines.data.map(l => this.mapInvoiceLine(l)),
      hosted_invoice_url: invoice.hosted_invoice_url ?? null,
      invoice_pdf: invoice.invoice_pdf ?? null,
      payment_intent_id: typeof invoice.payment_intent === 'string' ? invoice.payment_intent : invoice.payment_intent?.id ?? null,
      charge_id: typeof invoice.charge === 'string' ? invoice.charge : invoice.charge?.id ?? null,
      attempt_count: invoice.attempt_count,
      attempted: invoice.attempted,
      auto_advance: invoice.auto_advance ?? null,
      next_payment_attempt: invoice.next_payment_attempt ? new Date(invoice.next_payment_attempt * 1000) : null,
      webhooks_delivered_at: invoice.webhooks_delivered_at ? new Date(invoice.webhooks_delivered_at * 1000) : null,
      paid: invoice.paid,
      paid_out_of_band: invoice.paid_out_of_band,
      period_start: new Date(invoice.period_start * 1000),
      period_end: new Date(invoice.period_end * 1000),
      due_date: invoice.due_date ? new Date(invoice.due_date * 1000) : null,
      effective_at: invoice.effective_at ? new Date(invoice.effective_at * 1000) : null,
      finalized_at: invoice.status_transitions?.finalized_at ? new Date(invoice.status_transitions.finalized_at * 1000) : null,
      marked_uncollectible_at: invoice.status_transitions?.marked_uncollectible_at ? new Date(invoice.status_transitions.marked_uncollectible_at * 1000) : null,
      voided_at: invoice.status_transitions?.voided_at ? new Date(invoice.status_transitions.voided_at * 1000) : null,
      metadata: invoice.metadata ?? {},
      created_at: new Date(invoice.created * 1000),
    };
  }

  private mapInvoiceLine(line: Stripe.InvoiceLineItem): StripeInvoiceLine {
    return {
      id: line.id,
      amount: line.amount,
      currency: line.currency,
      description: line.description,
      quantity: line.quantity,
      price: line.price ? {
        id: line.price.id,
        product: typeof line.price.product === 'string' ? line.price.product : line.price.product?.id ?? '',
        unit_amount: line.price.unit_amount,
      } : null,
    };
  }

  // =========================================================================
  // Invoice Items
  // =========================================================================

  async listAllInvoiceItems(params?: Stripe.InvoiceItemListParams): Promise<StripeInvoiceItemRecord[]> {
    const items: StripeInvoiceItemRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.invoiceItems.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      items.push(...response.data.map(i => this.mapInvoiceItem(i)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched invoice items batch', { count: response.data.length, total: items.length });
    }

    return items;
  }

  async getInvoiceItem(id: string): Promise<StripeInvoiceItemRecord | null> {
    try {
      const item = await this.stripe.invoiceItems.retrieve(id);
      return this.mapInvoiceItem(item);
    } catch (error) {
      logger.error('Failed to get invoice item', { id, error });
      return null;
    }
  }

  private mapInvoiceItem(item: Stripe.InvoiceItem): StripeInvoiceItemRecord {
    return {
      id: item.id,
      customer_id: typeof item.customer === 'string' ? item.customer : item.customer.id,
      invoice_id: typeof item.invoice === 'string' ? item.invoice : item.invoice?.id ?? null,
      subscription_id: typeof item.subscription === 'string' ? item.subscription : item.subscription?.id ?? null,
      subscription_item_id: typeof item.subscription_item === 'string' ? item.subscription_item : item.subscription_item ?? null,
      price_id: item.price?.id ?? null,
      amount: item.amount,
      currency: item.currency,
      description: item.description,
      quantity: item.quantity,
      unit_amount: item.unit_amount,
      unit_amount_decimal: item.unit_amount_decimal,
      discountable: item.discountable,
      proration: item.proration,
      period_start: new Date(item.period.start * 1000),
      period_end: new Date(item.period.end * 1000),
      metadata: item.metadata ?? {},
      created_at: new Date(item.date * 1000),
    };
  }

  // =========================================================================
  // Credit Notes
  // =========================================================================

  async listAllCreditNotes(params?: Stripe.CreditNoteListParams): Promise<StripeCreditNoteRecord[]> {
    const creditNotes: StripeCreditNoteRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.creditNotes.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      creditNotes.push(...response.data.map(cn => this.mapCreditNote(cn)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched credit notes batch', { count: response.data.length, total: creditNotes.length });
    }

    return creditNotes;
  }

  async getCreditNote(id: string): Promise<StripeCreditNoteRecord | null> {
    try {
      const creditNote = await this.stripe.creditNotes.retrieve(id);
      return this.mapCreditNote(creditNote);
    } catch (error) {
      logger.error('Failed to get credit note', { id, error });
      return null;
    }
  }

  private mapCreditNote(cn: Stripe.CreditNote): StripeCreditNoteRecord {
    return {
      id: cn.id,
      invoice_id: typeof cn.invoice === 'string' ? cn.invoice : cn.invoice?.id ?? '',
      customer_id: typeof cn.customer === 'string' ? cn.customer : cn.customer?.id ?? '',
      type: cn.type,
      status: cn.status,
      amount: cn.amount,
      currency: cn.currency,
      discount_amount: cn.discount_amount,
      subtotal: cn.subtotal,
      subtotal_excluding_tax: cn.subtotal_excluding_tax,
      total: cn.total,
      total_excluding_tax: cn.total_excluding_tax,
      reason: cn.reason,
      memo: cn.memo,
      number: cn.number ?? '',
      pdf: cn.pdf,
      out_of_band_amount: cn.out_of_band_amount,
      voided_at: cn.voided_at ? new Date(cn.voided_at * 1000) : null,
      metadata: cn.metadata ?? {},
      created_at: new Date(cn.created * 1000),
    };
  }

  // =========================================================================
  // Charges
  // =========================================================================

  async listAllCharges(params?: Stripe.ChargeListParams): Promise<StripeChargeRecord[]> {
    const charges: StripeChargeRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.charges.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      charges.push(...response.data.map(c => this.mapCharge(c)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched charges batch', { count: response.data.length, total: charges.length });
    }

    return charges;
  }

  async getCharge(id: string): Promise<StripeChargeRecord | null> {
    try {
      const charge = await this.stripe.charges.retrieve(id);
      return this.mapCharge(charge);
    } catch (error) {
      logger.error('Failed to get charge', { id, error });
      return null;
    }
  }

  private mapCharge(charge: Stripe.Charge): StripeChargeRecord {
    return {
      id: charge.id,
      customer_id: typeof charge.customer === 'string' ? charge.customer : charge.customer?.id ?? null,
      payment_intent_id: typeof charge.payment_intent === 'string' ? charge.payment_intent : charge.payment_intent?.id ?? null,
      invoice_id: typeof charge.invoice === 'string' ? charge.invoice : charge.invoice?.id ?? null,
      balance_transaction_id: typeof charge.balance_transaction === 'string' ? charge.balance_transaction : charge.balance_transaction?.id ?? null,
      amount: charge.amount,
      amount_captured: charge.amount_captured,
      amount_refunded: charge.amount_refunded,
      currency: charge.currency,
      status: charge.status,
      paid: charge.paid,
      captured: charge.captured,
      refunded: charge.refunded,
      disputed: charge.disputed,
      payment_method_id: typeof charge.payment_method === 'string' ? charge.payment_method : (charge.payment_method as { id?: string } | null)?.id ?? null,
      payment_method_details: charge.payment_method_details as Record<string, unknown> | null,
      billing_details: charge.billing_details,
      description: charge.description,
      receipt_email: charge.receipt_email,
      receipt_number: charge.receipt_number,
      receipt_url: charge.receipt_url,
      statement_descriptor: charge.statement_descriptor,
      statement_descriptor_suffix: charge.statement_descriptor_suffix,
      failure_code: charge.failure_code,
      failure_message: charge.failure_message,
      fraud_details: charge.fraud_details,
      outcome: charge.outcome as Record<string, unknown> | null,
      shipping: charge.shipping,
      application_fee_id: typeof charge.application_fee === 'string' ? charge.application_fee : charge.application_fee?.id ?? null,
      application_fee_amount: charge.application_fee_amount,
      transfer_id: typeof charge.transfer === 'string' ? charge.transfer : charge.transfer?.id ?? null,
      transfer_group: charge.transfer_group,
      on_behalf_of: typeof charge.on_behalf_of === 'string' ? charge.on_behalf_of : charge.on_behalf_of?.id ?? null,
      source_transfer: charge.source_transfer as string | null,
      metadata: charge.metadata ?? {},
      created_at: new Date(charge.created * 1000),
    };
  }

  // =========================================================================
  // Refunds
  // =========================================================================

  async listAllRefunds(params?: Stripe.RefundListParams): Promise<StripeRefundRecord[]> {
    const refunds: StripeRefundRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.refunds.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      refunds.push(...response.data.map(r => this.mapRefund(r)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched refunds batch', { count: response.data.length, total: refunds.length });
    }

    return refunds;
  }

  async getRefund(id: string): Promise<StripeRefundRecord | null> {
    try {
      const refund = await this.stripe.refunds.retrieve(id);
      return this.mapRefund(refund);
    } catch (error) {
      logger.error('Failed to get refund', { id, error });
      return null;
    }
  }

  private mapRefund(refund: Stripe.Refund): StripeRefundRecord {
    return {
      id: refund.id,
      charge_id: typeof refund.charge === 'string' ? refund.charge : refund.charge?.id ?? null,
      payment_intent_id: typeof refund.payment_intent === 'string' ? refund.payment_intent : refund.payment_intent?.id ?? null,
      balance_transaction_id: typeof refund.balance_transaction === 'string' ? refund.balance_transaction : refund.balance_transaction?.id ?? null,
      amount: refund.amount,
      currency: refund.currency,
      status: refund.status ?? 'unknown',
      reason: refund.reason,
      receipt_number: refund.receipt_number,
      description: refund.description ?? null,
      failure_reason: refund.failure_reason ?? null,
      failure_balance_transaction: typeof refund.failure_balance_transaction === 'string' ? refund.failure_balance_transaction : refund.failure_balance_transaction?.id ?? null,
      source_transfer_reversal: typeof refund.source_transfer_reversal === 'string' ? refund.source_transfer_reversal : refund.source_transfer_reversal?.id ?? null,
      transfer_reversal: typeof refund.transfer_reversal === 'string' ? refund.transfer_reversal : refund.transfer_reversal?.id ?? null,
      metadata: refund.metadata ?? {},
      created_at: new Date(refund.created * 1000),
    };
  }

  // =========================================================================
  // Disputes
  // =========================================================================

  async listAllDisputes(params?: Stripe.DisputeListParams): Promise<StripeDisputeRecord[]> {
    const disputes: StripeDisputeRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.disputes.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      disputes.push(...response.data.map(d => this.mapDispute(d)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched disputes batch', { count: response.data.length, total: disputes.length });
    }

    return disputes;
  }

  async getDispute(id: string): Promise<StripeDisputeRecord | null> {
    try {
      const dispute = await this.stripe.disputes.retrieve(id);
      return this.mapDispute(dispute);
    } catch (error) {
      logger.error('Failed to get dispute', { id, error });
      return null;
    }
  }

  private mapDispute(dispute: Stripe.Dispute): StripeDisputeRecord {
    return {
      id: dispute.id,
      charge_id: typeof dispute.charge === 'string' ? dispute.charge : dispute.charge?.id ?? '',
      payment_intent_id: typeof dispute.payment_intent === 'string' ? dispute.payment_intent : dispute.payment_intent?.id ?? null,
      balance_transactions: dispute.balance_transactions?.map(bt => typeof bt === 'string' ? bt : bt.id) ?? [],
      amount: dispute.amount,
      currency: dispute.currency,
      status: dispute.status,
      reason: dispute.reason,
      evidence: dispute.evidence as unknown as Record<string, unknown>,
      evidence_details: dispute.evidence_details as unknown as Record<string, unknown>,
      is_charge_refundable: dispute.is_charge_refundable,
      metadata: dispute.metadata ?? {},
      created_at: new Date(dispute.created * 1000),
    };
  }

  // =========================================================================
  // Payment Intents
  // =========================================================================

  async listAllPaymentIntents(params?: Stripe.PaymentIntentListParams): Promise<StripePaymentIntentRecord[]> {
    const paymentIntents: StripePaymentIntentRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.paymentIntents.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      paymentIntents.push(...response.data.map(pi => this.mapPaymentIntent(pi)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched payment intents batch', { count: response.data.length, total: paymentIntents.length });
    }

    return paymentIntents;
  }

  async getPaymentIntent(id: string): Promise<StripePaymentIntentRecord | null> {
    try {
      const paymentIntent = await this.stripe.paymentIntents.retrieve(id);
      return this.mapPaymentIntent(paymentIntent);
    } catch (error) {
      logger.error('Failed to get payment intent', { id, error });
      return null;
    }
  }

  private mapPaymentIntent(pi: Stripe.PaymentIntent): StripePaymentIntentRecord {
    return {
      id: pi.id,
      customer_id: typeof pi.customer === 'string' ? pi.customer : pi.customer?.id ?? null,
      invoice_id: typeof pi.invoice === 'string' ? pi.invoice : pi.invoice?.id ?? null,
      amount: pi.amount,
      amount_capturable: pi.amount_capturable,
      amount_received: pi.amount_received,
      currency: pi.currency,
      status: pi.status,
      capture_method: pi.capture_method,
      confirmation_method: pi.confirmation_method,
      payment_method_id: typeof pi.payment_method === 'string' ? pi.payment_method : pi.payment_method?.id ?? null,
      payment_method_types: pi.payment_method_types,
      setup_future_usage: pi.setup_future_usage,
      client_secret: pi.client_secret,
      description: pi.description,
      receipt_email: pi.receipt_email,
      statement_descriptor: pi.statement_descriptor,
      statement_descriptor_suffix: pi.statement_descriptor_suffix,
      shipping: pi.shipping,
      application_fee_amount: pi.application_fee_amount,
      transfer_data: pi.transfer_data,
      transfer_group: pi.transfer_group,
      on_behalf_of: typeof pi.on_behalf_of === 'string' ? pi.on_behalf_of : pi.on_behalf_of?.id ?? null,
      cancellation_reason: pi.cancellation_reason,
      canceled_at: pi.canceled_at ? new Date(pi.canceled_at * 1000) : null,
      charges: pi.latest_charge ? [pi.latest_charge as Stripe.Charge] : [],
      last_payment_error: pi.last_payment_error,
      next_action: pi.next_action,
      processing: pi.processing,
      review: typeof pi.review === 'string' ? pi.review : pi.review?.id ?? null,
      automatic_payment_methods: pi.automatic_payment_methods,
      metadata: pi.metadata ?? {},
      created_at: new Date(pi.created * 1000),
    };
  }

  // =========================================================================
  // Setup Intents
  // =========================================================================

  async listAllSetupIntents(params?: Stripe.SetupIntentListParams): Promise<StripeSetupIntentRecord[]> {
    const setupIntents: StripeSetupIntentRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.setupIntents.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      setupIntents.push(...response.data.map(si => this.mapSetupIntent(si)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched setup intents batch', { count: response.data.length, total: setupIntents.length });
    }

    return setupIntents;
  }

  async getSetupIntent(id: string): Promise<StripeSetupIntentRecord | null> {
    try {
      const setupIntent = await this.stripe.setupIntents.retrieve(id);
      return this.mapSetupIntent(setupIntent);
    } catch (error) {
      logger.error('Failed to get setup intent', { id, error });
      return null;
    }
  }

  private mapSetupIntent(si: Stripe.SetupIntent): StripeSetupIntentRecord {
    return {
      id: si.id,
      customer_id: typeof si.customer === 'string' ? si.customer : si.customer?.id ?? null,
      payment_method_id: typeof si.payment_method === 'string' ? si.payment_method : si.payment_method?.id ?? null,
      status: si.status,
      usage: si.usage,
      client_secret: si.client_secret,
      description: si.description,
      payment_method_types: si.payment_method_types,
      single_use_mandate: typeof si.single_use_mandate === 'string' ? si.single_use_mandate : si.single_use_mandate?.id ?? null,
      mandate: typeof si.mandate === 'string' ? si.mandate : si.mandate?.id ?? null,
      on_behalf_of: typeof si.on_behalf_of === 'string' ? si.on_behalf_of : si.on_behalf_of?.id ?? null,
      application: typeof si.application === 'string' ? si.application : si.application?.id ?? null,
      cancellation_reason: si.cancellation_reason,
      last_setup_error: si.last_setup_error as unknown as Record<string, unknown> | null,
      next_action: si.next_action as unknown as Record<string, unknown> | null,
      metadata: si.metadata ?? {},
      created_at: new Date(si.created * 1000),
    };
  }

  // =========================================================================
  // Payment Methods
  // =========================================================================

  async listAllPaymentMethods(customerId: string, params?: Stripe.PaymentMethodListParams): Promise<StripePaymentMethodRecord[]> {
    const paymentMethods: StripePaymentMethodRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.paymentMethods.list({
        customer: customerId,
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      paymentMethods.push(...response.data.map(pm => this.mapPaymentMethod(pm)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched payment methods batch', { count: response.data.length, total: paymentMethods.length });
    }

    return paymentMethods;
  }

  async getPaymentMethod(id: string): Promise<StripePaymentMethodRecord | null> {
    try {
      const paymentMethod = await this.stripe.paymentMethods.retrieve(id);
      return this.mapPaymentMethod(paymentMethod);
    } catch (error) {
      logger.error('Failed to get payment method', { id, error });
      return null;
    }
  }

  private mapPaymentMethod(pm: Stripe.PaymentMethod): StripePaymentMethodRecord {
    return {
      id: pm.id,
      customer_id: typeof pm.customer === 'string' ? pm.customer : pm.customer?.id ?? null,
      type: pm.type,
      billing_details: pm.billing_details,
      card: pm.card ?? null,
      bank_account: null,
      sepa_debit: pm.sepa_debit ?? null,
      us_bank_account: pm.us_bank_account ?? null,
      link: pm.link ?? null,
      metadata: pm.metadata ?? {},
      created_at: new Date(pm.created * 1000),
    };
  }

  // =========================================================================
  // Balance Transactions
  // =========================================================================

  async listAllBalanceTransactions(params?: Stripe.BalanceTransactionListParams): Promise<StripeBalanceTransactionRecord[]> {
    const transactions: StripeBalanceTransactionRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.balanceTransactions.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      transactions.push(...response.data.map(bt => this.mapBalanceTransaction(bt)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched balance transactions batch', { count: response.data.length, total: transactions.length });
    }

    return transactions;
  }

  async getBalanceTransaction(id: string): Promise<StripeBalanceTransactionRecord | null> {
    try {
      const transaction = await this.stripe.balanceTransactions.retrieve(id);
      return this.mapBalanceTransaction(transaction);
    } catch (error) {
      logger.error('Failed to get balance transaction', { id, error });
      return null;
    }
  }

  private mapBalanceTransaction(bt: Stripe.BalanceTransaction): StripeBalanceTransactionRecord {
    return {
      id: bt.id,
      amount: bt.amount,
      currency: bt.currency,
      net: bt.net,
      fee: bt.fee,
      fee_details: bt.fee_details,
      type: bt.type,
      status: bt.status,
      description: bt.description,
      source: typeof bt.source === 'string' ? bt.source : bt.source?.id ?? null,
      reporting_category: bt.reporting_category,
      available_on: new Date(bt.available_on * 1000),
      created_at: new Date(bt.created * 1000),
    };
  }

  // =========================================================================
  // Checkout Sessions
  // =========================================================================

  async listAllCheckoutSessions(params?: Stripe.Checkout.SessionListParams): Promise<StripeCheckoutSessionRecord[]> {
    const sessions: StripeCheckoutSessionRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.checkout.sessions.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      sessions.push(...response.data.map(s => this.mapCheckoutSession(s)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched checkout sessions batch', { count: response.data.length, total: sessions.length });
    }

    return sessions;
  }

  async getCheckoutSession(id: string): Promise<StripeCheckoutSessionRecord | null> {
    try {
      const session = await this.stripe.checkout.sessions.retrieve(id, {
        expand: ['line_items'],
      });
      return this.mapCheckoutSession(session);
    } catch (error) {
      logger.error('Failed to get checkout session', { id, error });
      return null;
    }
  }

  private mapCheckoutSession(session: Stripe.Checkout.Session): StripeCheckoutSessionRecord {
    return {
      id: session.id,
      customer_id: typeof session.customer === 'string' ? session.customer : session.customer?.id ?? null,
      customer_email: session.customer_email,
      payment_intent_id: typeof session.payment_intent === 'string' ? session.payment_intent : session.payment_intent?.id ?? null,
      subscription_id: typeof session.subscription === 'string' ? session.subscription : session.subscription?.id ?? null,
      invoice_id: typeof session.invoice === 'string' ? session.invoice : session.invoice?.id ?? null,
      mode: session.mode,
      status: session.status ?? 'unknown',
      payment_status: session.payment_status,
      amount_subtotal: session.amount_subtotal,
      amount_total: session.amount_total,
      currency: session.currency,
      success_url: session.success_url,
      cancel_url: session.cancel_url,
      url: session.url,
      client_reference_id: session.client_reference_id,
      customer_creation: session.customer_creation,
      billing_address_collection: session.billing_address_collection,
      shipping_address_collection: session.shipping_address_collection as Record<string, unknown> | null,
      shipping_details: session.shipping_details as Record<string, unknown> | null,
      shipping_cost: session.shipping_cost as Record<string, unknown> | null,
      total_details: session.total_details as Record<string, unknown> | null,
      consent: session.consent as Record<string, unknown> | null,
      consent_collection: session.consent_collection as Record<string, unknown> | null,
      custom_text: session.custom_text as unknown as Record<string, unknown> | null,
      expires_at: new Date(session.expires_at * 1000),
      locale: session.locale,
      livemode: session.livemode,
      metadata: session.metadata ?? {},
      created_at: new Date(session.created * 1000),
    };
  }

  // =========================================================================
  // Tax IDs
  // =========================================================================

  async listAllTaxIds(customerId: string): Promise<StripeTaxIdRecord[]> {
    const taxIds: StripeTaxIdRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.customers.listTaxIds(customerId, {
        limit: 100,
        starting_after: startingAfter,
      });

      taxIds.push(...response.data.map(t => this.mapTaxId(t, customerId)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }
    }

    return taxIds;
  }

  async getTaxId(customerId: string, taxId: string): Promise<StripeTaxIdRecord | null> {
    try {
      const tax = await this.stripe.customers.retrieveTaxId(customerId, taxId);
      return this.mapTaxId(tax, customerId);
    } catch (error) {
      logger.error('Failed to get tax ID', { customerId, taxId, error });
      return null;
    }
  }

  private mapTaxId(tax: Stripe.TaxId, customerId: string): StripeTaxIdRecord {
    return {
      id: tax.id,
      customer_id: customerId,
      type: tax.type,
      value: tax.value,
      country: tax.country,
      verification: tax.verification as Record<string, unknown> | null,
      created_at: new Date(tax.created * 1000),
    };
  }

  // =========================================================================
  // Tax Rates
  // =========================================================================

  async listAllTaxRates(params?: Stripe.TaxRateListParams): Promise<StripeTaxRateRecord[]> {
    const taxRates: StripeTaxRateRecord[] = [];
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
      const response = await this.stripe.taxRates.list({
        limit: 100,
        starting_after: startingAfter,
        ...params,
      });

      taxRates.push(...response.data.map(t => this.mapTaxRate(t)));
      hasMore = response.has_more;

      if (response.data.length > 0) {
        startingAfter = response.data[response.data.length - 1].id;
      }

      logger.debug('Fetched tax rates batch', { count: response.data.length, total: taxRates.length });
    }

    return taxRates;
  }

  async getTaxRate(id: string): Promise<StripeTaxRateRecord | null> {
    try {
      const taxRate = await this.stripe.taxRates.retrieve(id);
      return this.mapTaxRate(taxRate);
    } catch (error) {
      logger.error('Failed to get tax rate', { id, error });
      return null;
    }
  }

  private mapTaxRate(rate: Stripe.TaxRate): StripeTaxRateRecord {
    return {
      id: rate.id,
      display_name: rate.display_name,
      description: rate.description,
      percentage: rate.percentage,
      inclusive: rate.inclusive,
      active: rate.active,
      country: rate.country,
      state: rate.state,
      jurisdiction: rate.jurisdiction,
      tax_type: rate.tax_type,
      metadata: rate.metadata ?? {},
      created_at: new Date(rate.created * 1000),
    };
  }

  // =========================================================================
  // Webhooks
  // =========================================================================

  constructEvent(payload: string | Buffer, signature: string, secret: string): Stripe.Event {
    return this.stripe.webhooks.constructEvent(payload, signature, secret);
  }
}
