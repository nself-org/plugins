/**
 * Shopify Data Synchronization Service
 * Handles historical data sync and incremental updates
 */

import { createLogger } from '@nself/plugin-utils';
import { ShopifyClient } from './client.js';
import { ShopifyDatabase } from './database.js';
import type { SyncOptions, SyncStats } from './types.js';

const logger = createLogger('shopify:sync');

export interface SyncResult {
  success: boolean;
  stats: SyncStats;
  errors: string[];
  duration: number;
}

export class ShopifySyncService {
  private client: ShopifyClient;
  private db: ShopifyDatabase;
  private syncing = false;

  constructor(client: ShopifyClient, db: ShopifyDatabase) {
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
      shops: 0,
      locations: 0,
      products: 0,
      variants: 0,
      collections: 0,
      customers: 0,
      orders: 0,
      orderItems: 0,
      fulfillments: 0,
      transactions: 0,
      refunds: 0,
      draftOrders: 0,
      inventory: 0,
      priceRules: 0,
      discountCodes: 0,
      giftCards: 0,
      metafields: 0,
      checkouts: 0,
    };

    const resources = options.resources ?? [
      'shop',
      'locations',
      'products',
      'collections',
      'customers',
      'orders',
      'fulfillments',
      'transactions',
      'refunds',
      'draft_orders',
      'inventory',
      'price_rules',
      'discount_codes',
      'gift_cards',
      'metafields',
      'checkouts',
    ];

    logger.info('Starting Shopify data sync', { resources });

    try {
      // Sync Shop first
      let shopId: number | undefined;
      if (resources.includes('shop')) {
        try {
          const shop = await this.client.getShop();
          await this.db.upsertShop(shop);
          shopId = shop.id;
          stats.shops = 1;
          logger.success('Synced shop info');
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Shop sync failed: ${message}`);
          logger.error('Shop sync failed', { error: message });
        }
      } else {
        const existingShop = await this.db.getShop();
        shopId = existingShop?.id;
      }

      // Sync Locations
      if (resources.includes('locations')) {
        try {
          const locations = await this.client.listLocations();
          stats.locations = await this.db.upsertLocations(locations);
          logger.success(`Synced ${stats.locations} locations`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Locations sync failed: ${message}`);
          logger.error('Locations sync failed', { error: message });
        }
      }

      // Sync Products
      if (resources.includes('products')) {
        try {
          const { products, variants } = await this.client.listAllProducts(shopId);
          stats.products = await this.db.upsertProducts(products);
          stats.variants = await this.db.upsertVariants(variants);
          logger.success(`Synced ${stats.products} products, ${stats.variants} variants`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Products sync failed: ${message}`);
          logger.error('Products sync failed', { error: message });
        }
      }

      // Sync Collections
      if (resources.includes('collections')) {
        try {
          const collections = await this.client.listAllCollections(shopId);
          stats.collections = await this.db.upsertCollections(collections);
          logger.success(`Synced ${stats.collections} collections`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Collections sync failed: ${message}`);
          logger.error('Collections sync failed', { error: message });
        }
      }

      // Sync Customers
      if (resources.includes('customers')) {
        try {
          const customers = await this.client.listAllCustomers(shopId);
          stats.customers = await this.db.upsertCustomers(customers);
          logger.success(`Synced ${stats.customers} customers`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Customers sync failed: ${message}`);
          logger.error('Customers sync failed', { error: message });
        }
      }

      // Sync Orders
      if (resources.includes('orders')) {
        try {
          const { orders, lineItems } = await this.client.listAllOrders(shopId);
          stats.orders = await this.db.upsertOrders(orders);
          stats.orderItems = await this.db.upsertOrderItems(lineItems);
          logger.success(`Synced ${stats.orders} orders, ${stats.orderItems} line items`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Orders sync failed: ${message}`);
          logger.error('Orders sync failed', { error: message });
        }
      }

      // Sync Fulfillments (synced per-order during order sync)
      if (resources.includes('fulfillments')) {
        logger.info('Fulfillments are synced per-order during order sync');
      }

      // Sync Transactions (synced per-order during order sync)
      if (resources.includes('transactions')) {
        logger.info('Transactions are synced per-order during order sync');
      }

      // Sync Refunds (synced per-order during order sync)
      if (resources.includes('refunds')) {
        logger.info('Refunds are synced per-order during order sync');
      }

      // Sync Draft Orders
      if (resources.includes('draft_orders')) {
        try {
          const draftOrders = await this.client.listAllDraftOrders();
          stats.draftOrders = await this.db.upsertDraftOrders(draftOrders);
          logger.success(`Synced ${stats.draftOrders} draft orders`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Draft orders sync failed: ${message}`);
          logger.error('Draft orders sync failed', { error: message });
        }
      }

      // Sync Inventory
      if (resources.includes('inventory')) {
        try {
          const inventoryLevels = await this.client.listInventoryLevels();
          stats.inventory = await this.db.upsertInventoryLevels(inventoryLevels);
          logger.success(`Synced ${stats.inventory} inventory levels`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Inventory sync failed: ${message}`);
          logger.error('Inventory sync failed', { error: message });
        }
      }

      // Sync Price Rules
      if (resources.includes('price_rules')) {
        try {
          const priceRules = await this.client.listAllPriceRules();
          stats.priceRules = await this.db.upsertPriceRules(priceRules);
          logger.success(`Synced ${stats.priceRules} price rules`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Price rules sync failed: ${message}`);
          logger.error('Price rules sync failed', { error: message });
        }
      }

      // Sync Discount Codes (synced per-price-rule during price rules sync)
      if (resources.includes('discount_codes')) {
        logger.info('Discount codes are synced per-price-rule during price rules sync');
      }

      // Sync Gift Cards
      if (resources.includes('gift_cards')) {
        try {
          const giftCards = await this.client.listAllGiftCards();
          stats.giftCards = await this.db.upsertGiftCards(giftCards);
          logger.success(`Synced ${stats.giftCards} gift cards`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Gift cards sync failed: ${message}`);
          logger.error('Gift cards sync failed', { error: message });
        }
      }

      // Sync Metafields (shop-level)
      if (resources.includes('metafields')) {
        try {
          const metafields = await this.client.listMetafields('shop', shopId ?? 0);
          stats.metafields = await this.db.upsertMetafields(metafields);
          logger.success(`Synced ${stats.metafields} metafields`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Metafields sync failed: ${message}`);
          logger.error('Metafields sync failed', { error: message });
        }
      }

      // Sync Checkouts (abandoned)
      if (resources.includes('checkouts')) {
        try {
          const checkouts = await this.client.listAllCheckouts();
          stats.checkouts = await this.db.upsertCheckouts(checkouts);
          logger.success(`Synced ${stats.checkouts} abandoned checkouts`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Checkouts sync failed: ${message}`);
          logger.error('Checkouts sync failed', { error: message });
        }
      }

      const duration = Date.now() - startTime;

      logger.success('Shopify sync completed', {
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

  async syncProduct(productId: number, shopId?: number): Promise<boolean> {
    logger.info('Syncing single product', { productId });

    try {
      const result = await this.client.getProduct(productId, shopId);
      if (result) {
        await this.db.upsertProduct(result.product);
        await this.db.upsertVariants(result.variants);
        return true;
      }
      return false;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to sync product', { productId, error: message });
      return false;
    }
  }

  async syncCustomer(customerId: number, shopId?: number): Promise<boolean> {
    logger.info('Syncing single customer', { customerId });

    try {
      const customer = await this.client.getCustomer(customerId, shopId);
      if (customer) {
        await this.db.upsertCustomer(customer);
        return true;
      }
      return false;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to sync customer', { customerId, error: message });
      return false;
    }
  }

  async syncOrder(orderId: number, shopId?: number): Promise<boolean> {
    logger.info('Syncing single order', { orderId });

    try {
      const result = await this.client.getOrder(orderId, shopId);
      if (result) {
        await this.db.upsertOrder(result.order);
        await this.db.upsertOrderItems(result.lineItems);
        return true;
      }
      return false;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to sync order', { orderId, error: message });
      return false;
    }
  }
}
