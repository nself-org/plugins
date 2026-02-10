/**
 * Shopify Webhook Handlers
 * Process incoming Shopify webhook events
 */

import { createLogger } from '@nself/plugin-utils';
import { ShopifyClient } from './client.js';
import { ShopifyDatabase } from './database.js';
import { ShopifySyncService } from './sync.js';
import type { ShopifyWebhookEventRecord } from './types.js';

const logger = createLogger('shopify:webhooks');

export type WebhookPayload = Record<string, unknown>;
export type WebhookHandlerFn = (payload: WebhookPayload) => Promise<void>;

export class ShopifyWebhookHandler {
  private client: ShopifyClient;
  private db: ShopifyDatabase;
  private syncService: ShopifySyncService;
  private handlers: Map<string, WebhookHandlerFn>;

  constructor(client: ShopifyClient, db: ShopifyDatabase, syncService: ShopifySyncService) {
    this.client = client;
    this.db = db;
    this.syncService = syncService;
    this.handlers = new Map();

    this.registerDefaultHandlers();
  }

  private registerDefaultHandlers(): void {
    // Order events
    this.register('orders/create', this.handleOrderCreate.bind(this));
    this.register('orders/updated', this.handleOrderUpdated.bind(this));
    this.register('orders/paid', this.handleOrderPaid.bind(this));
    this.register('orders/fulfilled', this.handleOrderFulfilled.bind(this));
    this.register('orders/cancelled', this.handleOrderCancelled.bind(this));
    this.register('orders/delete', this.handleOrderDelete.bind(this));

    // Product events
    this.register('products/create', this.handleProductCreate.bind(this));
    this.register('products/update', this.handleProductUpdate.bind(this));
    this.register('products/delete', this.handleProductDelete.bind(this));

    // Customer events
    this.register('customers/create', this.handleCustomerCreate.bind(this));
    this.register('customers/update', this.handleCustomerUpdate.bind(this));
    this.register('customers/delete', this.handleCustomerDelete.bind(this));

    // Inventory events
    this.register('inventory_levels/update', this.handleInventoryUpdate.bind(this));
    this.register('inventory_levels/connect', this.handleInventoryConnect.bind(this));
    this.register('inventory_levels/disconnect', this.handleInventoryDisconnect.bind(this));

    // Fulfillment events
    this.register('fulfillments/create', this.handleFulfillmentCreate.bind(this));
    this.register('fulfillments/update', this.handleFulfillmentUpdate.bind(this));

    // Refund events
    this.register('refunds/create', this.handleRefundCreate.bind(this));

    // Collection events
    this.register('collections/create', this.handleCollectionCreate.bind(this));
    this.register('collections/update', this.handleCollectionUpdate.bind(this));
    this.register('collections/delete', this.handleCollectionDelete.bind(this));

    // Shop events
    this.register('shop/update', this.handleShopUpdate.bind(this));

    // Draft order events
    this.register('draft_orders/create', this.handleDraftOrderCreate.bind(this));
    this.register('draft_orders/update', this.handleDraftOrderUpdate.bind(this));
    this.register('draft_orders/delete', this.handleDraftOrderDelete.bind(this));

    // Transaction events
    this.register('order_transactions/create', this.handleTransactionCreate.bind(this));

    // Theme events (informational)
    this.register('themes/create', this.handleThemeEvent.bind(this));
    this.register('themes/update', this.handleThemeEvent.bind(this));
    this.register('themes/delete', this.handleThemeEvent.bind(this));
    this.register('themes/publish', this.handleThemeEvent.bind(this));

    // App events
    this.register('app/uninstalled', this.handleAppUninstalled.bind(this));

    // Checkout events
    this.register('checkouts/create', this.handleCheckoutCreate.bind(this));
    this.register('checkouts/update', this.handleCheckoutUpdate.bind(this));
    this.register('checkouts/delete', this.handleCheckoutDelete.bind(this));
  }

  register(topic: string, handler: WebhookHandlerFn): void {
    this.handlers.set(topic, handler);
  }

  async handle(webhookId: string, topic: string, shopDomain: string, payload: WebhookPayload): Promise<void> {
    const shopId = payload.id as number | undefined;

    const eventRecord: ShopifyWebhookEventRecord = {
      id: webhookId,
      source_account_id: 'primary',
      topic,
      shop_id: shopId ?? null,
      shop_domain: shopDomain,
      data: payload,
      processed: false,
      processed_at: null,
      error: null,
      received_at: new Date(),
    };

    // Store the event
    await this.db.insertWebhookEvent(eventRecord);
    logger.info('Webhook event received', { topic, webhookId, shopDomain });

    // Find and execute handler
    const handler = this.handlers.get(topic);

    if (handler) {
      try {
        await handler(payload);
        await this.db.markEventProcessed(webhookId);
        logger.success('Webhook event processed', { topic, webhookId });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        await this.db.markEventProcessed(webhookId, message);
        logger.error('Webhook event processing failed', { topic, webhookId, error: message });
        throw error;
      }
    } else {
      await this.db.markEventProcessed(webhookId);
      logger.debug('No handler for topic', { topic });
    }
  }

  // =========================================================================
  // Order Handlers
  // =========================================================================

  private async handleOrderCreate(payload: WebhookPayload): Promise<void> {
    const orderId = payload.id as number;
    logger.info('Order created', { orderId, name: payload.name });
    await this.syncService.syncOrder(orderId);
  }

  private async handleOrderUpdated(payload: WebhookPayload): Promise<void> {
    const orderId = payload.id as number;
    logger.info('Order updated', { orderId, name: payload.name });
    await this.syncService.syncOrder(orderId);
  }

  private async handleOrderPaid(payload: WebhookPayload): Promise<void> {
    const orderId = payload.id as number;
    logger.info('Order paid', {
      orderId,
      name: payload.name,
      total: payload.total_price,
    });
    await this.syncService.syncOrder(orderId);
  }

  private async handleOrderFulfilled(payload: WebhookPayload): Promise<void> {
    const orderId = payload.id as number;
    logger.info('Order fulfilled', { orderId, name: payload.name });
    await this.syncService.syncOrder(orderId);
  }

  private async handleOrderCancelled(payload: WebhookPayload): Promise<void> {
    const orderId = payload.id as number;
    logger.info('Order cancelled', {
      orderId,
      name: payload.name,
      reason: payload.cancel_reason,
    });
    await this.syncService.syncOrder(orderId);
  }

  private async handleOrderDelete(payload: WebhookPayload): Promise<void> {
    const orderId = payload.id as number;
    logger.info('Order deleted', { orderId });
    // Mark as deleted in database
    await this.db.execute(
      'DELETE FROM shopify_order_items WHERE order_id = $1',
      [orderId]
    );
    await this.db.execute(
      'DELETE FROM shopify_orders WHERE id = $1',
      [orderId]
    );
  }

  // =========================================================================
  // Product Handlers
  // =========================================================================

  private async handleProductCreate(payload: WebhookPayload): Promise<void> {
    const productId = payload.id as number;
    logger.info('Product created', { productId, title: payload.title });
    await this.syncService.syncProduct(productId);
  }

  private async handleProductUpdate(payload: WebhookPayload): Promise<void> {
    const productId = payload.id as number;
    logger.info('Product updated', { productId, title: payload.title });
    await this.syncService.syncProduct(productId);
  }

  private async handleProductDelete(payload: WebhookPayload): Promise<void> {
    const productId = payload.id as number;
    logger.info('Product deleted', { productId });
    // Delete from database
    await this.db.execute(
      'DELETE FROM shopify_variants WHERE product_id = $1',
      [productId]
    );
    await this.db.execute(
      'DELETE FROM shopify_products WHERE id = $1',
      [productId]
    );
  }

  // =========================================================================
  // Customer Handlers
  // =========================================================================

  private async handleCustomerCreate(payload: WebhookPayload): Promise<void> {
    const customerId = payload.id as number;
    logger.info('Customer created', { customerId, email: payload.email });
    await this.syncService.syncCustomer(customerId);
  }

  private async handleCustomerUpdate(payload: WebhookPayload): Promise<void> {
    const customerId = payload.id as number;
    logger.info('Customer updated', { customerId, email: payload.email });
    await this.syncService.syncCustomer(customerId);
  }

  private async handleCustomerDelete(payload: WebhookPayload): Promise<void> {
    const customerId = payload.id as number;
    logger.info('Customer deleted', { customerId });
    await this.db.execute(
      'DELETE FROM shopify_customers WHERE id = $1',
      [customerId]
    );
  }

  // =========================================================================
  // Inventory Handlers
  // =========================================================================

  private async handleInventoryUpdate(payload: WebhookPayload): Promise<void> {
    const inventoryItemId = payload.inventory_item_id as number;
    const locationId = payload.location_id as number;
    const available = payload.available as number;

    logger.info('Inventory updated', { inventoryItemId, locationId, available });

    await this.db.execute(
      `INSERT INTO shopify_inventory (inventory_item_id, location_id, available, on_hand, updated_at, synced_at)
       VALUES ($1, $2, $3, $3, NOW(), NOW())
       ON CONFLICT (inventory_item_id, location_id) DO UPDATE SET
         available = EXCLUDED.available,
         on_hand = EXCLUDED.available,
         updated_at = NOW(),
         synced_at = NOW()`,
      [inventoryItemId, locationId, available]
    );
  }

  private async handleInventoryConnect(payload: WebhookPayload): Promise<void> {
    const inventoryItemId = payload.inventory_item_id as number;
    const locationId = payload.location_id as number;

    logger.info('Inventory connected', { inventoryItemId, locationId });

    await this.db.execute(
      `INSERT INTO shopify_inventory (inventory_item_id, location_id, available, on_hand, updated_at, synced_at)
       VALUES ($1, $2, 0, 0, NOW(), NOW())
       ON CONFLICT (inventory_item_id, location_id) DO NOTHING`,
      [inventoryItemId, locationId]
    );
  }

  private async handleInventoryDisconnect(payload: WebhookPayload): Promise<void> {
    const inventoryItemId = payload.inventory_item_id as number;
    const locationId = payload.location_id as number;

    logger.info('Inventory disconnected', { inventoryItemId, locationId });

    await this.db.execute(
      'DELETE FROM shopify_inventory WHERE inventory_item_id = $1 AND location_id = $2',
      [inventoryItemId, locationId]
    );
  }

  // =========================================================================
  // Fulfillment Handlers
  // =========================================================================

  private async handleFulfillmentCreate(payload: WebhookPayload): Promise<void> {
    const orderId = payload.order_id as number;
    logger.info('Fulfillment created', {
      orderId,
      status: payload.status,
      trackingCompany: payload.tracking_company,
    });
    await this.syncService.syncOrder(orderId);
  }

  private async handleFulfillmentUpdate(payload: WebhookPayload): Promise<void> {
    const orderId = payload.order_id as number;
    logger.info('Fulfillment updated', { orderId, status: payload.status });
    await this.syncService.syncOrder(orderId);
  }

  // =========================================================================
  // Refund Handlers
  // =========================================================================

  private async handleRefundCreate(payload: WebhookPayload): Promise<void> {
    const orderId = payload.order_id as number;
    const refundAmount = payload.transactions as Array<{ amount: string }>;
    const totalRefunded = refundAmount?.reduce((sum, t) => sum + parseFloat(t.amount), 0) ?? 0;

    logger.info('Refund created', { orderId, totalRefunded });
    await this.syncService.syncOrder(orderId);
  }

  // =========================================================================
  // Collection Handlers
  // =========================================================================

  private async handleCollectionCreate(payload: WebhookPayload): Promise<void> {
    const collectionId = payload.id as number;
    logger.info('Collection created', { collectionId, title: payload.title });
    // Full collections sync would be needed here
  }

  private async handleCollectionUpdate(payload: WebhookPayload): Promise<void> {
    const collectionId = payload.id as number;
    logger.info('Collection updated', { collectionId, title: payload.title });
  }

  private async handleCollectionDelete(payload: WebhookPayload): Promise<void> {
    const collectionId = payload.id as number;
    logger.info('Collection deleted', { collectionId });
    await this.db.execute(
      'DELETE FROM shopify_collections WHERE id = $1',
      [collectionId]
    );
  }

  // =========================================================================
  // Shop Handlers
  // =========================================================================

  private async handleShopUpdate(payload: WebhookPayload): Promise<void> {
    logger.info('Shop updated', { name: payload.name, domain: payload.domain });
    // Re-sync shop info
    const shop = await this.client.getShop();
    await this.db.upsertShop(shop);
  }

  // =========================================================================
  // Draft Order Handlers
  // =========================================================================

  private async handleDraftOrderCreate(payload: WebhookPayload): Promise<void> {
    const draftOrderId = payload.id as number;
    logger.info('Draft order created', { draftOrderId, name: payload.name });
    // Sync draft orders
    const draftOrders = await this.client.listAllDraftOrders();
    await this.db.upsertDraftOrders(draftOrders);
  }

  private async handleDraftOrderUpdate(payload: WebhookPayload): Promise<void> {
    const draftOrderId = payload.id as number;
    logger.info('Draft order updated', { draftOrderId, name: payload.name });
    const draftOrders = await this.client.listAllDraftOrders();
    await this.db.upsertDraftOrders(draftOrders);
  }

  private async handleDraftOrderDelete(payload: WebhookPayload): Promise<void> {
    const draftOrderId = payload.id as number;
    logger.info('Draft order deleted', { draftOrderId });
    await this.db.execute(
      'DELETE FROM shopify_draft_orders WHERE id = $1',
      [draftOrderId]
    );
  }

  // =========================================================================
  // Transaction Handlers
  // =========================================================================

  private async handleTransactionCreate(payload: WebhookPayload): Promise<void> {
    const orderId = payload.order_id as number;
    logger.info('Transaction created', {
      orderId,
      kind: payload.kind,
      status: payload.status,
    });
    // Re-sync the order to get updated transaction info
    await this.syncService.syncOrder(orderId);
  }

  // =========================================================================
  // Theme Handlers (informational)
  // =========================================================================

  private async handleThemeEvent(payload: WebhookPayload): Promise<void> {
    logger.info('Theme event', {
      themeId: payload.id,
      name: payload.name,
      role: payload.role,
    });
    // Theme events are informational, no database action needed
  }

  // =========================================================================
  // App Handlers
  // =========================================================================

  private async handleAppUninstalled(_payload: WebhookPayload): Promise<void> {
    logger.warn('App uninstalled - webhook access will be revoked');
    // This is an important event to handle for cleanup
  }

  // =========================================================================
  // Checkout Handlers (Abandoned Checkouts)
  // =========================================================================

  private async handleCheckoutCreate(payload: WebhookPayload): Promise<void> {
    const checkoutId = payload.id as number;
    logger.info('Checkout created', { checkoutId, email: payload.email });
    const checkouts = await this.client.listAllCheckouts();
    await this.db.upsertCheckouts(checkouts);
  }

  private async handleCheckoutUpdate(payload: WebhookPayload): Promise<void> {
    const checkoutId = payload.id as number;
    logger.info('Checkout updated', { checkoutId });
    const checkouts = await this.client.listAllCheckouts();
    await this.db.upsertCheckouts(checkouts);
  }

  private async handleCheckoutDelete(payload: WebhookPayload): Promise<void> {
    const checkoutId = payload.id as number;
    logger.info('Checkout deleted/completed', { checkoutId });
    await this.db.execute(
      'DELETE FROM shopify_checkouts WHERE id = $1',
      [checkoutId]
    );
  }
}
