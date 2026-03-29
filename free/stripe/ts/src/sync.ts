/**
 * Stripe Data Synchronization Service
 * Handles historical data sync and incremental updates for all Stripe objects
 */

import { createLogger } from '@nself/plugin-utils';
import { StripeClient } from './client.js';
import { StripeDatabase } from './database.js';
import type { SyncOptions, SyncStats } from './types.js';

const logger = createLogger('stripe:sync');

export interface SyncResult {
  success: boolean;
  stats: SyncStats;
  errors: string[];
  duration: number;
}

// All available resources for syncing
export const ALL_RESOURCES = [
  'customers',
  'products',
  'prices',
  'coupons',
  'promotion_codes',
  'subscriptions',
  'subscription_items',
  'subscription_schedules',
  'invoices',
  'invoice_items',
  'credit_notes',
  'charges',
  'refunds',
  'disputes',
  'payment_intents',
  'setup_intents',
  'payment_methods',
  'balance_transactions',
  'checkout_sessions',
  'tax_ids',
  'tax_rates',
] as const;

export type SyncResource = typeof ALL_RESOURCES[number];

export class StripeSyncService {
  private client: StripeClient;
  private db: StripeDatabase;
  private syncing = false;

  constructor(client: StripeClient, db: StripeDatabase) {
    this.client = client;
    this.db = db;
  }

  async sync(options: SyncOptions = {}): Promise<SyncResult> {
    if (this.syncing) {
      throw new Error('Sync already in progress');
    }

    this.syncing = true;
    const startTime = Date.now();
    const errors: string[] = [];
    const stats: SyncStats = {
      customers: 0,
      products: 0,
      prices: 0,
      coupons: 0,
      promotionCodes: 0,
      subscriptions: 0,
      subscriptionItems: 0,
      subscriptionSchedules: 0,
      invoices: 0,
      invoiceItems: 0,
      creditNotes: 0,
      charges: 0,
      refunds: 0,
      disputes: 0,
      paymentIntents: 0,
      setupIntents: 0,
      paymentMethods: 0,
      balanceTransactions: 0,
      checkoutSessions: 0,
      taxIds: 0,
      taxRates: 0,
    };

    // Default resources to sync (core billing objects)
    const resources = options.resources ?? [
      'customers',
      'products',
      'prices',
      'coupons',
      'subscriptions',
      'invoices',
      'charges',
      'refunds',
      'payment_intents',
      'payment_methods',
    ];

    logger.info('Starting Stripe data sync', { resources, incremental: options.incremental });

    try {
      // Sync in dependency order

      // 1. Core entities (no dependencies)
      if (resources.includes('products')) {
        try {
          stats.products = await this.syncProducts();
          logger.success(`Synced ${stats.products} products`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Products sync failed: ${message}`);
          logger.error('Products sync failed', { error: message });
        }
      }

      if (resources.includes('prices')) {
        try {
          stats.prices = await this.syncPrices();
          logger.success(`Synced ${stats.prices} prices`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Prices sync failed: ${message}`);
          logger.error('Prices sync failed', { error: message });
        }
      }

      if (resources.includes('coupons')) {
        try {
          stats.coupons = await this.syncCoupons();
          logger.success(`Synced ${stats.coupons} coupons`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Coupons sync failed: ${message}`);
          logger.error('Coupons sync failed', { error: message });
        }
      }

      if (resources.includes('tax_rates')) {
        try {
          stats.taxRates = await this.syncTaxRates();
          logger.success(`Synced ${stats.taxRates} tax rates`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Tax rates sync failed: ${message}`);
          logger.error('Tax rates sync failed', { error: message });
        }
      }

      // 2. Customers (needed for many other resources)
      if (resources.includes('customers')) {
        try {
          stats.customers = await this.syncCustomers();
          logger.success(`Synced ${stats.customers} customers`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Customers sync failed: ${message}`);
          logger.error('Customers sync failed', { error: message });
        }
      }

      // 3. Promotion codes (depends on coupons)
      if (resources.includes('promotion_codes')) {
        try {
          stats.promotionCodes = await this.syncPromotionCodes();
          logger.success(`Synced ${stats.promotionCodes} promotion codes`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Promotion codes sync failed: ${message}`);
          logger.error('Promotion codes sync failed', { error: message });
        }
      }

      // 4. Subscriptions
      if (resources.includes('subscriptions')) {
        try {
          stats.subscriptions = await this.syncSubscriptions();
          logger.success(`Synced ${stats.subscriptions} subscriptions`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Subscriptions sync failed: ${message}`);
          logger.error('Subscriptions sync failed', { error: message });
        }
      }

      // 5. Subscription Items (depends on subscriptions)
      if (resources.includes('subscription_items')) {
        try {
          stats.subscriptionItems = await this.syncSubscriptionItems();
          logger.success(`Synced ${stats.subscriptionItems} subscription items`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Subscription items sync failed: ${message}`);
          logger.error('Subscription items sync failed', { error: message });
        }
      }

      // 6. Subscription Schedules
      if (resources.includes('subscription_schedules')) {
        try {
          stats.subscriptionSchedules = await this.syncSubscriptionSchedules();
          logger.success(`Synced ${stats.subscriptionSchedules} subscription schedules`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Subscription schedules sync failed: ${message}`);
          logger.error('Subscription schedules sync failed', { error: message });
        }
      }

      // 7. Invoices
      if (resources.includes('invoices')) {
        try {
          stats.invoices = await this.syncInvoices();
          logger.success(`Synced ${stats.invoices} invoices`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Invoices sync failed: ${message}`);
          logger.error('Invoices sync failed', { error: message });
        }
      }

      // 8. Invoice Items
      if (resources.includes('invoice_items')) {
        try {
          stats.invoiceItems = await this.syncInvoiceItems();
          logger.success(`Synced ${stats.invoiceItems} invoice items`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Invoice items sync failed: ${message}`);
          logger.error('Invoice items sync failed', { error: message });
        }
      }

      // 9. Credit Notes (depends on invoices)
      if (resources.includes('credit_notes')) {
        try {
          stats.creditNotes = await this.syncCreditNotes();
          logger.success(`Synced ${stats.creditNotes} credit notes`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Credit notes sync failed: ${message}`);
          logger.error('Credit notes sync failed', { error: message });
        }
      }

      // 10. Payment Intents
      if (resources.includes('payment_intents')) {
        try {
          stats.paymentIntents = await this.syncPaymentIntents();
          logger.success(`Synced ${stats.paymentIntents} payment intents`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Payment intents sync failed: ${message}`);
          logger.error('Payment intents sync failed', { error: message });
        }
      }

      // 11. Setup Intents
      if (resources.includes('setup_intents')) {
        try {
          stats.setupIntents = await this.syncSetupIntents();
          logger.success(`Synced ${stats.setupIntents} setup intents`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Setup intents sync failed: ${message}`);
          logger.error('Setup intents sync failed', { error: message });
        }
      }

      // 12. Charges
      if (resources.includes('charges')) {
        try {
          stats.charges = await this.syncCharges();
          logger.success(`Synced ${stats.charges} charges`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Charges sync failed: ${message}`);
          logger.error('Charges sync failed', { error: message });
        }
      }

      // 13. Refunds (depends on charges)
      if (resources.includes('refunds')) {
        try {
          stats.refunds = await this.syncRefunds();
          logger.success(`Synced ${stats.refunds} refunds`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Refunds sync failed: ${message}`);
          logger.error('Refunds sync failed', { error: message });
        }
      }

      // 14. Disputes (depends on charges)
      if (resources.includes('disputes')) {
        try {
          stats.disputes = await this.syncDisputes();
          logger.success(`Synced ${stats.disputes} disputes`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Disputes sync failed: ${message}`);
          logger.error('Disputes sync failed', { error: message });
        }
      }

      // 15. Balance Transactions
      if (resources.includes('balance_transactions')) {
        try {
          stats.balanceTransactions = await this.syncBalanceTransactions();
          logger.success(`Synced ${stats.balanceTransactions} balance transactions`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Balance transactions sync failed: ${message}`);
          logger.error('Balance transactions sync failed', { error: message });
        }
      }

      // 16. Checkout Sessions
      if (resources.includes('checkout_sessions')) {
        try {
          stats.checkoutSessions = await this.syncCheckoutSessions();
          logger.success(`Synced ${stats.checkoutSessions} checkout sessions`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Checkout sessions sync failed: ${message}`);
          logger.error('Checkout sessions sync failed', { error: message });
        }
      }

      // 17. Payment Methods (depends on customers)
      if (resources.includes('payment_methods')) {
        try {
          stats.paymentMethods = await this.syncPaymentMethods();
          logger.success(`Synced ${stats.paymentMethods} payment methods`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Payment methods sync failed: ${message}`);
          logger.error('Payment methods sync failed', { error: message });
        }
      }

      // 18. Tax IDs (depends on customers)
      if (resources.includes('tax_ids')) {
        try {
          stats.taxIds = await this.syncTaxIds();
          logger.success(`Synced ${stats.taxIds} tax IDs`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Tax IDs sync failed: ${message}`);
          logger.error('Tax IDs sync failed', { error: message });
        }
      }

      const duration = Date.now() - startTime;

      logger.success('Stripe sync completed', {
        duration: `${(duration / 1000).toFixed(1)}s`,
        stats,
        errors: errors.length,
      });

      return {
        success: errors.length === 0,
        stats,
        errors,
        duration,
      };
    } finally {
      this.syncing = false;
    }
  }

  // =========================================================================
  // Reconciliation Sync
  // =========================================================================

  /**
   * Re-sync resources created within a lookback window to catch gaps from
   * missed webhooks or cron failures. Uses Stripe's `created` filter to
   * only fetch objects in the window, then upserts them (idempotent).
   *
   * @param lookbackDays Number of days to look back (default 7)
   * @returns SyncResult with counts of reconciled records
   */
  async reconcile(lookbackDays = 7): Promise<SyncResult> {
    if (this.syncing) {
      throw new Error('Sync already in progress');
    }

    this.syncing = true;
    const startTime = Date.now();
    const errors: string[] = [];
    const stats: SyncStats = {
      customers: 0,
      products: 0,
      prices: 0,
      coupons: 0,
      promotionCodes: 0,
      subscriptions: 0,
      subscriptionItems: 0,
      subscriptionSchedules: 0,
      invoices: 0,
      invoiceItems: 0,
      creditNotes: 0,
      charges: 0,
      refunds: 0,
      disputes: 0,
      paymentIntents: 0,
      setupIntents: 0,
      paymentMethods: 0,
      balanceTransactions: 0,
      checkoutSessions: 0,
      taxIds: 0,
      taxRates: 0,
    };

    const since = Math.floor(Date.now() / 1000) - (lookbackDays * 86400);
    const createdFilter = { gte: since } as const;

    logger.info('Starting reconciliation sync', { lookbackDays, since: new Date(since * 1000).toISOString() });

    try {
      // Reconcile payment-critical resources using created filter
      const reconcileTasks: Array<{ name: string; key: keyof SyncStats; fn: () => Promise<number> }> = [
        { name: 'Charges', key: 'charges', fn: async () => {
          const items = await this.client.listAllCharges({ created: createdFilter });
          return this.db.upsertCharges(items);
        }},
        { name: 'Payment intents', key: 'paymentIntents', fn: async () => {
          const items = await this.client.listAllPaymentIntents({ created: createdFilter });
          return this.db.upsertPaymentIntents(items);
        }},
        { name: 'Invoices', key: 'invoices', fn: async () => {
          const items = await this.client.listAllInvoices({ created: createdFilter });
          return this.db.upsertInvoices(items);
        }},
        { name: 'Customers', key: 'customers', fn: async () => {
          const items = await this.client.listAllCustomers({ created: createdFilter });
          return this.db.upsertCustomers(items);
        }},
        { name: 'Subscriptions', key: 'subscriptions', fn: async () => {
          const items = await this.client.listAllSubscriptions({ created: createdFilter });
          return this.db.upsertSubscriptions(items);
        }},
        { name: 'Refunds', key: 'refunds', fn: async () => {
          const items = await this.client.listAllRefunds({ created: createdFilter });
          return this.db.upsertRefunds(items);
        }},
        { name: 'Balance transactions', key: 'balanceTransactions', fn: async () => {
          const items = await this.client.listAllBalanceTransactions({ created: createdFilter });
          return this.db.upsertBalanceTransactions(items);
        }},
      ];

      for (const task of reconcileTasks) {
        try {
          const count = await task.fn();
          (stats as unknown as Record<string, unknown>)[task.key] = count;
          if (count > 0) {
            logger.info(`Reconciled ${count} ${task.name.toLowerCase()}`);
          }
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`${task.name} reconciliation failed: ${message}`);
          logger.error(`${task.name} reconciliation failed`, { error: message });
        }
      }

      const duration = Date.now() - startTime;
      logger.success('Reconciliation completed', { duration: `${(duration / 1000).toFixed(1)}s`, errors: errors.length });

      return { success: errors.length === 0, stats, errors, duration };
    } finally {
      this.syncing = false;
    }
  }

  // =========================================================================
  // Individual Sync Methods
  // =========================================================================

  private async syncCustomers(): Promise<number> {
    logger.info('Syncing customers...');
    const customers = await this.client.listAllCustomers();
    return await this.db.upsertCustomers(customers);
  }

  private async syncProducts(): Promise<number> {
    logger.info('Syncing products...');
    const products = await this.client.listAllProducts();
    return await this.db.upsertProducts(products);
  }

  private async syncPrices(): Promise<number> {
    logger.info('Syncing prices...');
    const prices = await this.client.listAllPrices();
    return await this.db.upsertPrices(prices);
  }

  private async syncCoupons(): Promise<number> {
    logger.info('Syncing coupons...');
    const coupons = await this.client.listAllCoupons();
    return await this.db.upsertCoupons(coupons);
  }

  private async syncPromotionCodes(): Promise<number> {
    logger.info('Syncing promotion codes...');
    const codes = await this.client.listAllPromotionCodes();
    return await this.db.upsertPromotionCodes(codes);
  }

  private async syncSubscriptions(): Promise<number> {
    logger.info('Syncing subscriptions...');
    const subscriptions = await this.client.listAllSubscriptions();
    return await this.db.upsertSubscriptions(subscriptions);
  }

  private async syncSubscriptionItems(): Promise<number> {
    logger.info('Syncing subscription items...');
    // Fetch from Stripe directly so multi-account sync does not depend on mixed local state.
    const subscriptions = await this.client.listAllSubscriptions({ status: 'all' });
    const activeSubscriptions = subscriptions.filter(s => s.status === 'active');
    let totalCount = 0;

    for (const subscription of activeSubscriptions) {
      try {
        const items = await this.client.listAllSubscriptionItems(subscription.id);
        const count = await this.db.upsertSubscriptionItems(items);
        totalCount += count;
      } catch (error) {
        logger.debug('No subscription items for subscription', { subscriptionId: subscription.id });
      }
    }

    return totalCount;
  }

  private async syncSubscriptionSchedules(): Promise<number> {
    logger.info('Syncing subscription schedules...');
    const schedules = await this.client.listAllSubscriptionSchedules();
    return await this.db.upsertSubscriptionSchedules(schedules);
  }

  private async syncInvoices(): Promise<number> {
    logger.info('Syncing invoices...');
    const invoices = await this.client.listAllInvoices();
    return await this.db.upsertInvoices(invoices);
  }

  private async syncInvoiceItems(): Promise<number> {
    logger.info('Syncing invoice items...');
    const items = await this.client.listAllInvoiceItems();
    return await this.db.upsertInvoiceItems(items);
  }

  private async syncCreditNotes(): Promise<number> {
    logger.info('Syncing credit notes...');
    const creditNotes = await this.client.listAllCreditNotes();
    return await this.db.upsertCreditNotes(creditNotes);
  }

  private async syncCharges(): Promise<number> {
    logger.info('Syncing charges...');
    const charges = await this.client.listAllCharges();
    return await this.db.upsertCharges(charges);
  }

  private async syncRefunds(): Promise<number> {
    logger.info('Syncing refunds...');
    const refunds = await this.client.listAllRefunds();
    return await this.db.upsertRefunds(refunds);
  }

  private async syncDisputes(): Promise<number> {
    logger.info('Syncing disputes...');
    const disputes = await this.client.listAllDisputes();
    return await this.db.upsertDisputes(disputes);
  }

  private async syncPaymentIntents(): Promise<number> {
    logger.info('Syncing payment intents...');
    const paymentIntents = await this.client.listAllPaymentIntents();
    return await this.db.upsertPaymentIntents(paymentIntents);
  }

  private async syncSetupIntents(): Promise<number> {
    logger.info('Syncing setup intents...');
    const setupIntents = await this.client.listAllSetupIntents();
    return await this.db.upsertSetupIntents(setupIntents);
  }

  private async syncPaymentMethods(): Promise<number> {
    logger.info('Syncing payment methods...');
    // Fetch from Stripe directly so each account sync only requests its own customers.
    const customers = await this.client.listAllCustomers();
    let totalCount = 0;

    for (const customer of customers) {
      try {
        const paymentMethods = await this.client.listAllPaymentMethods(customer.id);
        const count = await this.db.upsertPaymentMethods(paymentMethods);
        totalCount += count;
      } catch (error) {
        logger.debug('No payment methods for customer', { customerId: customer.id });
      }
    }

    return totalCount;
  }

  private async syncBalanceTransactions(): Promise<number> {
    logger.info('Syncing balance transactions...');
    const transactions = await this.client.listAllBalanceTransactions();
    return await this.db.upsertBalanceTransactions(transactions);
  }

  private async syncCheckoutSessions(): Promise<number> {
    logger.info('Syncing checkout sessions...');
    const sessions = await this.client.listAllCheckoutSessions();
    return await this.db.upsertCheckoutSessions(sessions);
  }

  private async syncTaxIds(): Promise<number> {
    logger.info('Syncing tax IDs...');
    // Fetch from Stripe directly so each account sync only requests its own customers.
    const customers = await this.client.listAllCustomers();
    let totalCount = 0;

    for (const customer of customers) {
      try {
        const taxIds = await this.client.listAllTaxIds(customer.id);
        const count = await this.db.upsertTaxIds(taxIds);
        totalCount += count;
      } catch (error) {
        logger.debug('No tax IDs for customer', { customerId: customer.id });
      }
    }

    return totalCount;
  }

  private async syncTaxRates(): Promise<number> {
    logger.info('Syncing tax rates...');
    const taxRates = await this.client.listAllTaxRates();
    return await this.db.upsertTaxRates(taxRates);
  }

  // =========================================================================
  // Single Resource Sync
  // =========================================================================

  async syncSingleResource(
    resourceType:
      | 'customer'
      | 'product'
      | 'price'
      | 'coupon'
      | 'promotion_code'
      | 'subscription'
      | 'subscription_item'
      | 'subscription_schedule'
      | 'invoice'
      | 'invoice_item'
      | 'credit_note'
      | 'charge'
      | 'refund'
      | 'dispute'
      | 'payment_intent'
      | 'setup_intent'
      | 'payment_method'
      | 'balance_transaction'
      | 'checkout_session'
      | 'tax_rate',
    resourceId: string
  ): Promise<boolean> {
    logger.info('Syncing single resource', { type: resourceType, id: resourceId });

    try {
      switch (resourceType) {
        case 'customer': {
          const customer = await this.client.getCustomer(resourceId);
          if (customer) {
            await this.db.upsertCustomer(customer);
            return true;
          }
          return false;
        }
        case 'product': {
          const product = await this.client.getProduct(resourceId);
          if (product) {
            await this.db.upsertProduct(product);
            return true;
          }
          return false;
        }
        case 'price': {
          const price = await this.client.getPrice(resourceId);
          if (price) {
            await this.db.upsertPrice(price);
            return true;
          }
          return false;
        }
        case 'coupon': {
          const coupon = await this.client.getCoupon(resourceId);
          if (coupon) {
            await this.db.upsertCoupon(coupon);
            return true;
          }
          return false;
        }
        case 'promotion_code': {
          const code = await this.client.getPromotionCode(resourceId);
          if (code) {
            await this.db.upsertPromotionCode(code);
            return true;
          }
          return false;
        }
        case 'subscription': {
          const subscription = await this.client.getSubscription(resourceId);
          if (subscription) {
            await this.db.upsertSubscription(subscription);
            return true;
          }
          return false;
        }
        case 'subscription_item': {
          const item = await this.client.getSubscriptionItem(resourceId);
          if (item) {
            await this.db.upsertSubscriptionItem(item);
            return true;
          }
          return false;
        }
        case 'subscription_schedule': {
          const schedule = await this.client.getSubscriptionSchedule(resourceId);
          if (schedule) {
            await this.db.upsertSubscriptionSchedule(schedule);
            return true;
          }
          return false;
        }
        case 'invoice': {
          const invoice = await this.client.getInvoice(resourceId);
          if (invoice) {
            await this.db.upsertInvoice(invoice);
            return true;
          }
          return false;
        }
        case 'invoice_item': {
          const item = await this.client.getInvoiceItem(resourceId);
          if (item) {
            await this.db.upsertInvoiceItem(item);
            return true;
          }
          return false;
        }
        case 'credit_note': {
          const creditNote = await this.client.getCreditNote(resourceId);
          if (creditNote) {
            await this.db.upsertCreditNote(creditNote);
            return true;
          }
          return false;
        }
        case 'charge': {
          const charge = await this.client.getCharge(resourceId);
          if (charge) {
            await this.db.upsertCharge(charge);
            return true;
          }
          return false;
        }
        case 'refund': {
          const refund = await this.client.getRefund(resourceId);
          if (refund) {
            await this.db.upsertRefund(refund);
            return true;
          }
          return false;
        }
        case 'dispute': {
          const dispute = await this.client.getDispute(resourceId);
          if (dispute) {
            await this.db.upsertDispute(dispute);
            return true;
          }
          return false;
        }
        case 'payment_intent': {
          const paymentIntent = await this.client.getPaymentIntent(resourceId);
          if (paymentIntent) {
            await this.db.upsertPaymentIntent(paymentIntent);
            return true;
          }
          return false;
        }
        case 'setup_intent': {
          const setupIntent = await this.client.getSetupIntent(resourceId);
          if (setupIntent) {
            await this.db.upsertSetupIntent(setupIntent);
            return true;
          }
          return false;
        }
        case 'payment_method': {
          const paymentMethod = await this.client.getPaymentMethod(resourceId);
          if (paymentMethod) {
            await this.db.upsertPaymentMethod(paymentMethod);
            return true;
          }
          return false;
        }
        case 'balance_transaction': {
          const transaction = await this.client.getBalanceTransaction(resourceId);
          if (transaction) {
            await this.db.upsertBalanceTransaction(transaction);
            return true;
          }
          return false;
        }
        case 'checkout_session': {
          const session = await this.client.getCheckoutSession(resourceId);
          if (session) {
            await this.db.upsertCheckoutSession(session);
            return true;
          }
          return false;
        }
        case 'tax_rate': {
          const taxRate = await this.client.getTaxRate(resourceId);
          if (taxRate) {
            await this.db.upsertTaxRate(taxRate);
            return true;
          }
          return false;
        }
        default:
          logger.warn('Unknown resource type', { type: resourceType });
          return false;
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to sync single resource', { type: resourceType, id: resourceId, error: message });
      return false;
    }
  }
}
