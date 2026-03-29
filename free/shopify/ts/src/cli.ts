#!/usr/bin/env node
/**
 * Shopify Plugin CLI
 * Command-line interface for the Shopify plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { ShopifyClient } from './client.js';
import { ShopifyDatabase } from './database.js';
import { ShopifySyncService } from './sync.js';
import { createServer } from './server.js';

const logger = createLogger('shopify:cli');

const program = new Command();

program
  .name('nself-shopify')
  .description('Shopify plugin for nself - sync Shopify data to PostgreSQL')
  .version('1.0.0');

// Sync command
program
  .command('sync')
  .description('Sync Shopify data to database')
  .option('-r, --resources <resources>', 'Comma-separated list of resources to sync', 'all')
  .action(async (options) => {
    try {
      const config = loadConfig();

      logger.info('Starting Shopify sync...');
      logger.info(`Shop: ${config.shopifyShopDomain}`);

      const client = new ShopifyClient(
        config.shopifyShopDomain,
        config.shopifyAccessToken,
        config.shopifyApiVersion
      );
      const db = new ShopifyDatabase();
      await db.connect();
      await db.initializeSchema();

      const syncService = new ShopifySyncService(client, db);

      const resources = options.resources === 'all'
        ? undefined
        : options.resources.split(',').map((r: string) => r.trim()) as Array<'shop' | 'products' | 'collections' | 'customers' | 'orders' | 'inventory'>;

      const result = await syncService.sync({ resources });

      console.log('\nSync Results:');
      console.log('=============');
      console.log(`Shop:         ${result.stats.shops}`);
      console.log(`Products:     ${result.stats.products}`);
      console.log(`Variants:     ${result.stats.variants}`);
      console.log(`Collections:  ${result.stats.collections}`);
      console.log(`Customers:    ${result.stats.customers}`);
      console.log(`Orders:       ${result.stats.orders}`);
      console.log(`Order Items:  ${result.stats.orderItems}`);
      console.log(`Inventory:    ${result.stats.inventory}`);
      console.log(`\nDuration: ${(result.duration / 1000).toFixed(1)}s`);

      if (result.errors.length > 0) {
        console.log('\nErrors:');
        result.errors.forEach(err => console.log(`  - ${err}`));
      }

      await db.disconnect();
      process.exit(result.success ? 0 : 1);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync failed', { error: message });
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start the webhook server')
  .option('-p, --port <port>', 'Server port', '3003')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      const server = await createServer(config);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config

      const db = new ShopifyDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.success('Database schema initialized');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Init failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show sync status and statistics')
  .action(async () => {
    try {
      const config = loadConfig();

      const db = new ShopifyDatabase();
      await db.connect();

      const shop = await db.getShop();
      const stats = await db.getStats();

      console.log('\nShopify Plugin Status');
      console.log('=====================');
      console.log(`Shop Domain: ${config.shopifyShopDomain}`);
      if (shop) {
        console.log(`Shop Name: ${shop.name}`);
        console.log(`Email: ${shop.email}`);
        console.log(`Currency: ${shop.currency}`);
      }
      console.log('\nSynced Records:');
      console.log(`  Products:     ${stats.products}`);
      console.log(`  Variants:     ${stats.variants}`);
      console.log(`  Collections:  ${stats.collections}`);
      console.log(`  Customers:    ${stats.customers}`);
      console.log(`  Orders:       ${stats.orders}`);
      console.log(`  Order Items:  ${stats.orderItems}`);
      console.log(`  Inventory:    ${stats.inventory}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Products command
program
  .command('products')
  .description('List products')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      const db = new ShopifyDatabase();
      await db.connect();

      const products = await db.listProducts(parseInt(options.limit, 10));
      console.log('\nProducts:');
      console.log('-'.repeat(100));
      products.forEach(p => {
        const status = p.status === 'active' ? '✅' : '❌';
        console.log(`${status} | ${p.title.substring(0, 50).padEnd(50)} | ${p.vendor || 'N/A'} | ${p.product_type || 'N/A'}`);
      });
      console.log(`\nTotal: ${await db.countProducts()}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Customers command
program
  .command('customers')
  .description('List customers')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      const db = new ShopifyDatabase();
      await db.connect();

      const customers = await db.listCustomers(parseInt(options.limit, 10));
      console.log('\nCustomers:');
      console.log('-'.repeat(100));
      customers.forEach(c => {
        const name = `${c.first_name || ''} ${c.last_name || ''}`.trim() || 'N/A';
        console.log(`${name.padEnd(30)} | ${(c.email || 'N/A').padEnd(40)} | Orders: ${c.orders_count}`);
      });
      console.log(`\nTotal: ${await db.countCustomers()}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Orders command
program
  .command('orders')
  .description('List orders')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .option('-s, --status <status>', 'Filter by financial status')
  .action(async (options) => {
    try {
      const db = new ShopifyDatabase();
      await db.connect();

      const orders = await db.listOrders(options.status, parseInt(options.limit, 10));
      console.log('\nOrders:');
      console.log('-'.repeat(100));
      orders.forEach(o => {
        const status = o.financial_status === 'paid' ? '✅' : (o.financial_status === 'refunded' ? '💰' : '⏳');
        const totalPrice = o.total_price != null ? o.total_price.toFixed(2) : '0.00';
        const financialStatus = o.financial_status ?? 'unknown';
        console.log(`${status} | ${o.name.padEnd(10)} | $${totalPrice.padStart(10)} | ${financialStatus.padEnd(12)} | ${o.fulfillment_status || 'unfulfilled'}`);
      });
      console.log(`\nTotal: ${await db.countOrders()}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Collections command
program
  .command('collections')
  .description('List collections')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      const db = new ShopifyDatabase();
      await db.connect();

      const collections = await db.listCollections(parseInt(options.limit, 10));
      console.log('\nCollections:');
      console.log('-'.repeat(100));
      collections.forEach(c => {
        const collectionType = c.collection_type ?? 'N/A';
        console.log(`${c.title.substring(0, 50).padEnd(50)} | ${collectionType.padEnd(10)} | Products: ${c.products_count ?? 'N/A'}`);
      });
      console.log(`\nTotal: ${await db.countCollections()}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Inventory command
program
  .command('inventory')
  .description('List inventory levels')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      const db = new ShopifyDatabase();
      await db.connect();

      const inventory = await db.listInventory(parseInt(options.limit, 10));
      console.log('\nInventory Levels:');
      console.log('-'.repeat(80));
      inventory.forEach(i => {
        const status = i.available > 0 ? '✅' : '⚠️';
        console.log(`${status} | Item: ${i.inventory_item_id} | Location: ${i.location_id} | Available: ${i.available} | On Hand: ${i.on_hand ?? 'N/A'}`);
      });
      console.log(`\nTotal: ${await db.countInventory()}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Webhooks command
program
  .command('webhooks')
  .description('List recent webhook events')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .option('-t, --topic <topic>', 'Filter by topic')
  .action(async (options) => {
    try {
      const db = new ShopifyDatabase();
      await db.connect();

      const events = await db.listWebhookEvents(options.topic, parseInt(options.limit, 10));
      console.log('\nWebhook Events:');
      console.log('-'.repeat(100));
      events.forEach(e => {
        const status = e.processed ? (e.error ? '❌' : '✅') : '⏳';
        const time = new Date(e.received_at).toISOString();
        const shopDomain = e.shop_domain ?? 'N/A';
        console.log(`${status} | ${e.topic.padEnd(30)} | ${shopDomain.padEnd(30)} | ${time}`);
      });

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Analytics command
program
  .command('analytics')
  .description('Show analytics summary')
  .action(async () => {
    try {
      const db = new ShopifyDatabase();
      await db.connect();

      // Daily sales (last 7 days)
      const salesResult = await db.query(
        `SELECT * FROM shopify_sales_overview
         WHERE order_date >= CURRENT_DATE - INTERVAL '7 days'
         ORDER BY order_date DESC`
      );

      console.log('\nDaily Sales (Last 7 Days):');
      console.log('-'.repeat(60));
      for (const row of salesResult.rows) {
        const r = row as { order_date: Date; order_count: number; revenue: string };
        const date = new Date(r.order_date).toISOString().split('T')[0];
        console.log(`${date} | Orders: ${r.order_count.toString().padStart(5)} | Revenue: $${parseFloat(r.revenue).toFixed(2).padStart(12)}`);
      }

      // Top products
      const topProductsResult = await db.query(
        'SELECT * FROM shopify_top_products LIMIT 5'
      );

      console.log('\nTop Products (by units sold):');
      console.log('-'.repeat(80));
      for (const row of topProductsResult.rows) {
        const r = row as { title: string; units_sold: number; revenue: string };
        console.log(`${r.title.substring(0, 40).padEnd(40)} | Qty: ${r.units_sold.toString().padStart(6)} | Revenue: $${parseFloat(r.revenue).toFixed(2).padStart(12)}`);
      }

      // Top customers by value
      const customersResult = await db.query('SELECT * FROM shopify_customer_value LIMIT 10');

      console.log('\nTop Customers (by total spent):');
      console.log('-'.repeat(80));
      for (const row of customersResult.rows) {
        const r = row as { name: string; orders_count: number; total_spent: string };
        console.log(`${(r.name || 'N/A').substring(0, 30).padEnd(30)} | Orders: ${r.orders_count.toString().padStart(4)} | Total Spent: $${parseFloat(r.total_spent).toFixed(2).padStart(12)}`);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
