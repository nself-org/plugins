/**
 * Shopify Database Operations
 * CRUD operations for Shopify data in PostgreSQL
 * Multi-app support: all tables use composite PKs with source_account_id
 */

import { createDatabase, createLogger, normalizeSourceAccountId, type Database } from '@nself/plugin-utils';
import type {
  ShopifyShopRecord,
  ShopifyLocationRecord,
  ShopifyProductRecord,
  ShopifyVariantRecord,
  ShopifyCollectionRecord,
  ShopifyCustomerRecord,
  ShopifyOrderRecord,
  ShopifyOrderItemRecord,
  ShopifyFulfillmentRecord,
  ShopifyTransactionRecord,
  ShopifyRefundRecord,
  ShopifyDraftOrderRecord,
  ShopifyInventoryRecord,
  ShopifyInventoryItemRecord,
  ShopifyPriceRuleRecord,
  ShopifyDiscountCodeRecord,
  ShopifyGiftCardRecord,
  ShopifyMetafieldRecord,
  ShopifyCheckoutRecord,
  ShopifyWebhookEventRecord,
  SyncStats,
} from './types.js';

const logger = createLogger('shopify:db');

/** All Shopify tables with their primary key columns (used for migration) */
const ALL_TABLES: { name: string; pk: string }[] = [
  { name: 'shopify_shops', pk: 'id' },
  { name: 'shopify_locations', pk: 'id' },
  { name: 'shopify_products', pk: 'id' },
  { name: 'shopify_variants', pk: 'id' },
  { name: 'shopify_collections', pk: 'id' },
  { name: 'shopify_customers', pk: 'id' },
  { name: 'shopify_orders', pk: 'id' },
  { name: 'shopify_order_items', pk: 'id' },
  { name: 'shopify_fulfillments', pk: 'id' },
  { name: 'shopify_transactions', pk: 'id' },
  { name: 'shopify_refunds', pk: 'id' },
  { name: 'shopify_draft_orders', pk: 'id' },
  { name: 'shopify_inventory_items', pk: 'id' },
  { name: 'shopify_inventory', pk: 'inventory_item_id, location_id' },
  { name: 'shopify_price_rules', pk: 'id' },
  { name: 'shopify_discount_codes', pk: 'id' },
  { name: 'shopify_gift_cards', pk: 'id' },
  { name: 'shopify_metafields', pk: 'id' },
  { name: 'shopify_checkouts', pk: 'id' },
  { name: 'shopify_webhook_events', pk: 'id' },
];

/** Ordered for cleanup: children before parents */
const CLEANUP_TABLE_ORDER: string[] = [
  'shopify_webhook_events',
  'shopify_discount_codes',
  'shopify_price_rules',
  'shopify_metafields',
  'shopify_gift_cards',
  'shopify_checkouts',
  'shopify_inventory',
  'shopify_inventory_items',
  'shopify_refunds',
  'shopify_transactions',
  'shopify_fulfillments',
  'shopify_order_items',
  'shopify_draft_orders',
  'shopify_orders',
  'shopify_customers',
  'shopify_collections',
  'shopify_variants',
  'shopify_products',
  'shopify_locations',
  'shopify_shops',
];

export class ShopifyDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = normalizeSourceAccountId(sourceAccountId);
  }

  /**
   * Create a new ShopifyDatabase instance scoped to a different source account.
   * Shares the same underlying database connection pool.
   */
  forSourceAccount(sourceAccountId: string): ShopifyDatabase {
    return new ShopifyDatabase(this.db, sourceAccountId);
  }

  /** Get the current source account ID this instance is scoped to. */
  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  async execute(sql: string, params?: unknown[]): Promise<number> {
    const result = await this.db.query(sql, params);
    return result.rowCount ?? 0;
  }

  // =========================================================================
  // Schema Management
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing Shopify schema...');

    // Core tables
    await this.db.executeSqlFile(`
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- Shops
      CREATE TABLE IF NOT EXISTS shopify_shops (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        email VARCHAR(255),
        domain VARCHAR(255),
        myshopify_domain VARCHAR(255) NOT NULL,
        shop_owner VARCHAR(255),
        phone VARCHAR(50),
        address1 TEXT,
        address2 TEXT,
        city VARCHAR(255),
        province VARCHAR(255),
        province_code VARCHAR(10),
        country VARCHAR(255),
        country_code VARCHAR(10),
        zip VARCHAR(50),
        currency VARCHAR(10) DEFAULT 'USD',
        money_format VARCHAR(50),
        timezone VARCHAR(100),
        iana_timezone VARCHAR(100),
        plan_name VARCHAR(100),
        plan_display_name VARCHAR(255),
        weight_unit VARCHAR(10) DEFAULT 'kg',
        primary_locale VARCHAR(10) DEFAULT 'en',
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      -- Locations
      CREATE TABLE IF NOT EXISTS shopify_locations (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        shop_id BIGINT,
        name VARCHAR(255) NOT NULL,
        address1 TEXT,
        address2 TEXT,
        city VARCHAR(255),
        province VARCHAR(255),
        province_code VARCHAR(10),
        country VARCHAR(255),
        country_code VARCHAR(10),
        zip VARCHAR(50),
        phone VARCHAR(50),
        active BOOLEAN DEFAULT TRUE,
        legacy BOOLEAN DEFAULT FALSE,
        localized_country_name VARCHAR(255),
        localized_province_name VARCHAR(255),
        admin_graphql_api_id VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_locations_shop ON shopify_locations(shop_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_locations_active ON shopify_locations(active);
      CREATE INDEX IF NOT EXISTS idx_shopify_locations_source ON shopify_locations(source_account_id);

      -- Products
      CREATE TABLE IF NOT EXISTS shopify_products (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        shop_id BIGINT,
        title VARCHAR(255) NOT NULL,
        body_html TEXT,
        vendor VARCHAR(255),
        product_type VARCHAR(255),
        handle VARCHAR(255),
        status VARCHAR(50) DEFAULT 'active',
        template_suffix VARCHAR(255),
        published_scope VARCHAR(50),
        tags TEXT,
        admin_graphql_api_id VARCHAR(255),
        image_id BIGINT,
        image_src TEXT,
        images JSONB DEFAULT '[]',
        options JSONB DEFAULT '[]',
        published_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_products_shop ON shopify_products(shop_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_products_handle ON shopify_products(handle);
      CREATE INDEX IF NOT EXISTS idx_shopify_products_status ON shopify_products(status);
      CREATE INDEX IF NOT EXISTS idx_shopify_products_vendor ON shopify_products(vendor);
      CREATE INDEX IF NOT EXISTS idx_shopify_products_source ON shopify_products(source_account_id);

      -- Variants
      CREATE TABLE IF NOT EXISTS shopify_variants (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        product_id BIGINT,
        title VARCHAR(255),
        price DECIMAL(10, 2),
        compare_at_price DECIMAL(10, 2),
        sku VARCHAR(255),
        barcode VARCHAR(255),
        position INTEGER DEFAULT 1,
        grams INTEGER DEFAULT 0,
        weight DECIMAL(10, 3),
        weight_unit VARCHAR(10) DEFAULT 'kg',
        inventory_item_id BIGINT,
        inventory_quantity INTEGER DEFAULT 0,
        inventory_policy VARCHAR(50) DEFAULT 'deny',
        inventory_management VARCHAR(50),
        fulfillment_service VARCHAR(100) DEFAULT 'manual',
        requires_shipping BOOLEAN DEFAULT TRUE,
        taxable BOOLEAN DEFAULT TRUE,
        option1 VARCHAR(255),
        option2 VARCHAR(255),
        option3 VARCHAR(255),
        image_id BIGINT,
        admin_graphql_api_id VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_variants_product ON shopify_variants(product_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_variants_sku ON shopify_variants(sku);
      CREATE INDEX IF NOT EXISTS idx_shopify_variants_inventory ON shopify_variants(inventory_item_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_variants_source ON shopify_variants(source_account_id);

      -- Collections
      CREATE TABLE IF NOT EXISTS shopify_collections (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        shop_id BIGINT,
        title VARCHAR(255) NOT NULL,
        body_html TEXT,
        handle VARCHAR(255),
        collection_type VARCHAR(50),
        sort_order VARCHAR(50),
        template_suffix VARCHAR(255),
        products_count INTEGER DEFAULT 0,
        disjunctive BOOLEAN DEFAULT FALSE,
        rules JSONB DEFAULT '[]',
        image JSONB,
        published_at TIMESTAMP WITH TIME ZONE,
        published_scope VARCHAR(50) DEFAULT 'web',
        admin_graphql_api_id VARCHAR(255),
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_collections_shop ON shopify_collections(shop_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_collections_handle ON shopify_collections(handle);
      CREATE INDEX IF NOT EXISTS idx_shopify_collections_source ON shopify_collections(source_account_id);

      -- Customers
      CREATE TABLE IF NOT EXISTS shopify_customers (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        shop_id BIGINT,
        email VARCHAR(255),
        first_name VARCHAR(255),
        last_name VARCHAR(255),
        phone VARCHAR(50),
        verified_email BOOLEAN DEFAULT FALSE,
        accepts_marketing BOOLEAN DEFAULT FALSE,
        accepts_marketing_updated_at TIMESTAMP WITH TIME ZONE,
        marketing_opt_in_level VARCHAR(50),
        orders_count INTEGER DEFAULT 0,
        total_spent DECIMAL(12, 2) DEFAULT 0,
        state VARCHAR(50) DEFAULT 'disabled',
        note TEXT,
        tags TEXT,
        currency VARCHAR(10),
        tax_exempt BOOLEAN DEFAULT FALSE,
        tax_exemptions JSONB DEFAULT '[]',
        default_address JSONB,
        addresses JSONB DEFAULT '[]',
        admin_graphql_api_id VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_customers_shop ON shopify_customers(shop_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_customers_email ON shopify_customers(email);
      CREATE INDEX IF NOT EXISTS idx_shopify_customers_state ON shopify_customers(state);
      CREATE INDEX IF NOT EXISTS idx_shopify_customers_source ON shopify_customers(source_account_id);
    `);

    // Orders and related tables
    await this.db.executeSqlFile(`
      -- Orders
      CREATE TABLE IF NOT EXISTS shopify_orders (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        shop_id BIGINT,
        order_number INTEGER NOT NULL,
        name VARCHAR(50) NOT NULL,
        email VARCHAR(255),
        phone VARCHAR(50),
        customer_id BIGINT,
        financial_status VARCHAR(50),
        fulfillment_status VARCHAR(50),
        cancel_reason VARCHAR(50),
        cancelled_at TIMESTAMP WITH TIME ZONE,
        closed_at TIMESTAMP WITH TIME ZONE,
        confirmed BOOLEAN DEFAULT TRUE,
        contact_email VARCHAR(255),
        currency VARCHAR(10) DEFAULT 'USD',
        current_subtotal_price DECIMAL(12, 2),
        current_total_discounts DECIMAL(12, 2),
        current_total_price DECIMAL(12, 2),
        current_total_tax DECIMAL(12, 2),
        subtotal_price DECIMAL(12, 2),
        total_discounts DECIMAL(12, 2),
        total_line_items_price DECIMAL(12, 2),
        total_price DECIMAL(12, 2),
        total_tax DECIMAL(12, 2),
        total_weight INTEGER DEFAULT 0,
        total_tip_received DECIMAL(12, 2) DEFAULT 0,
        discount_codes JSONB DEFAULT '[]',
        note TEXT,
        note_attributes JSONB DEFAULT '[]',
        tags TEXT,
        tax_lines JSONB DEFAULT '[]',
        taxes_included BOOLEAN DEFAULT FALSE,
        test BOOLEAN DEFAULT FALSE,
        token VARCHAR(255),
        gateway VARCHAR(100),
        payment_gateway_names JSONB DEFAULT '[]',
        processing_method VARCHAR(50),
        source_name VARCHAR(100),
        source_identifier VARCHAR(255),
        source_url VARCHAR(2048),
        landing_site VARCHAR(2048),
        referring_site VARCHAR(2048),
        billing_address JSONB,
        shipping_address JSONB,
        shipping_lines JSONB DEFAULT '[]',
        admin_graphql_api_id VARCHAR(255),
        processed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_orders_shop ON shopify_orders(shop_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_orders_customer ON shopify_orders(customer_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_orders_number ON shopify_orders(order_number);
      CREATE INDEX IF NOT EXISTS idx_shopify_orders_financial ON shopify_orders(financial_status);
      CREATE INDEX IF NOT EXISTS idx_shopify_orders_fulfillment ON shopify_orders(fulfillment_status);
      CREATE INDEX IF NOT EXISTS idx_shopify_orders_created ON shopify_orders(created_at);
      CREATE INDEX IF NOT EXISTS idx_shopify_orders_source ON shopify_orders(source_account_id);

      -- Order Items
      CREATE TABLE IF NOT EXISTS shopify_order_items (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        order_id BIGINT,
        product_id BIGINT,
        variant_id BIGINT,
        title VARCHAR(255) NOT NULL,
        variant_title VARCHAR(255),
        sku VARCHAR(255),
        vendor VARCHAR(255),
        quantity INTEGER NOT NULL DEFAULT 1,
        price DECIMAL(12, 2),
        total_discount DECIMAL(12, 2) DEFAULT 0,
        fulfillment_status VARCHAR(50),
        fulfillable_quantity INTEGER DEFAULT 0,
        fulfillment_service VARCHAR(100),
        grams INTEGER DEFAULT 0,
        requires_shipping BOOLEAN DEFAULT TRUE,
        taxable BOOLEAN DEFAULT TRUE,
        tax_lines JSONB DEFAULT '[]',
        properties JSONB DEFAULT '[]',
        gift_card BOOLEAN DEFAULT FALSE,
        admin_graphql_api_id VARCHAR(255),
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_items_order ON shopify_order_items(order_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_items_product ON shopify_order_items(product_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_items_variant ON shopify_order_items(variant_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_items_source ON shopify_order_items(source_account_id);

      -- Fulfillments
      CREATE TABLE IF NOT EXISTS shopify_fulfillments (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        order_id BIGINT,
        location_id BIGINT,
        status VARCHAR(50),
        tracking_company VARCHAR(255),
        tracking_number VARCHAR(255),
        tracking_numbers JSONB DEFAULT '[]',
        tracking_url VARCHAR(2048),
        tracking_urls JSONB DEFAULT '[]',
        shipment_status VARCHAR(50),
        service VARCHAR(100),
        name VARCHAR(100),
        receipt JSONB,
        line_items JSONB DEFAULT '[]',
        notify_customer BOOLEAN DEFAULT FALSE,
        admin_graphql_api_id VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_fulfillments_order ON shopify_fulfillments(order_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_fulfillments_location ON shopify_fulfillments(location_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_fulfillments_status ON shopify_fulfillments(status);
      CREATE INDEX IF NOT EXISTS idx_shopify_fulfillments_source ON shopify_fulfillments(source_account_id);

      -- Transactions
      CREATE TABLE IF NOT EXISTS shopify_transactions (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        order_id BIGINT,
        parent_id BIGINT,
        kind VARCHAR(50) NOT NULL,
        gateway VARCHAR(100),
        status VARCHAR(50),
        message TEXT,
        amount DECIMAL(12, 2),
        currency VARCHAR(10),
        authorization VARCHAR(255),
        source_name VARCHAR(100),
        payment_details JSONB,
        error_code VARCHAR(100),
        receipt JSONB,
        test BOOLEAN DEFAULT FALSE,
        admin_graphql_api_id VARCHAR(255),
        processed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_transactions_order ON shopify_transactions(order_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_transactions_kind ON shopify_transactions(kind);
      CREATE INDEX IF NOT EXISTS idx_shopify_transactions_status ON shopify_transactions(status);
      CREATE INDEX IF NOT EXISTS idx_shopify_transactions_source ON shopify_transactions(source_account_id);

      -- Refunds
      CREATE TABLE IF NOT EXISTS shopify_refunds (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        order_id BIGINT,
        note TEXT,
        restock BOOLEAN DEFAULT FALSE,
        user_id BIGINT,
        refund_line_items JSONB DEFAULT '[]',
        transactions JSONB DEFAULT '[]',
        order_adjustments JSONB DEFAULT '[]',
        duties JSONB DEFAULT '[]',
        admin_graphql_api_id VARCHAR(255),
        processed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_refunds_order ON shopify_refunds(order_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_refunds_created ON shopify_refunds(created_at);
      CREATE INDEX IF NOT EXISTS idx_shopify_refunds_source ON shopify_refunds(source_account_id);
    `);

    // Draft orders, inventory, and promotions
    await this.db.executeSqlFile(`
      -- Draft Orders
      CREATE TABLE IF NOT EXISTS shopify_draft_orders (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        shop_id BIGINT,
        order_id BIGINT,
        name VARCHAR(50),
        email VARCHAR(255),
        customer_id BIGINT,
        status VARCHAR(50) DEFAULT 'open',
        currency VARCHAR(10) DEFAULT 'USD',
        subtotal_price DECIMAL(12, 2),
        total_price DECIMAL(12, 2),
        total_tax DECIMAL(12, 2),
        taxes_included BOOLEAN DEFAULT FALSE,
        tax_exempt BOOLEAN DEFAULT FALSE,
        tax_lines JSONB DEFAULT '[]',
        discount_codes JSONB DEFAULT '[]',
        applied_discount JSONB,
        line_items JSONB DEFAULT '[]',
        shipping_line JSONB,
        billing_address JSONB,
        shipping_address JSONB,
        note TEXT,
        note_attributes JSONB DEFAULT '[]',
        tags TEXT,
        invoice_url VARCHAR(2048),
        invoice_sent_at TIMESTAMP WITH TIME ZONE,
        admin_graphql_api_id VARCHAR(255),
        completed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_draft_orders_shop ON shopify_draft_orders(shop_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_draft_orders_customer ON shopify_draft_orders(customer_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_draft_orders_status ON shopify_draft_orders(status);
      CREATE INDEX IF NOT EXISTS idx_shopify_draft_orders_source ON shopify_draft_orders(source_account_id);

      -- Inventory Items
      CREATE TABLE IF NOT EXISTS shopify_inventory_items (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        sku VARCHAR(255),
        cost DECIMAL(10, 2),
        country_code_of_origin VARCHAR(10),
        country_harmonized_system_codes JSONB DEFAULT '[]',
        harmonized_system_code VARCHAR(50),
        province_code_of_origin VARCHAR(10),
        tracked BOOLEAN DEFAULT TRUE,
        requires_shipping BOOLEAN DEFAULT TRUE,
        admin_graphql_api_id VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_inventory_items_sku ON shopify_inventory_items(sku);
      CREATE INDEX IF NOT EXISTS idx_shopify_inventory_items_source ON shopify_inventory_items(source_account_id);

      -- Inventory Levels
      CREATE TABLE IF NOT EXISTS shopify_inventory (
        inventory_item_id BIGINT NOT NULL,
        location_id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        variant_id BIGINT,
        available INTEGER DEFAULT 0,
        incoming INTEGER DEFAULT 0,
        committed INTEGER DEFAULT 0,
        damaged INTEGER DEFAULT 0,
        on_hand INTEGER DEFAULT 0,
        quality_control INTEGER DEFAULT 0,
        reserved INTEGER DEFAULT 0,
        safety_stock INTEGER DEFAULT 0,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (inventory_item_id, location_id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_inventory_item ON shopify_inventory(inventory_item_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_inventory_location ON shopify_inventory(location_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_inventory_variant ON shopify_inventory(variant_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_inventory_source ON shopify_inventory(source_account_id);

      -- Price Rules
      CREATE TABLE IF NOT EXISTS shopify_price_rules (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        shop_id BIGINT,
        title VARCHAR(255) NOT NULL,
        target_type VARCHAR(50),
        target_selection VARCHAR(50),
        allocation_method VARCHAR(50),
        value_type VARCHAR(50),
        value DECIMAL(12, 2),
        once_per_customer BOOLEAN DEFAULT FALSE,
        usage_limit INTEGER,
        customer_selection VARCHAR(50),
        prerequisite_subtotal_range JSONB,
        prerequisite_quantity_range JSONB,
        prerequisite_shipping_price_range JSONB,
        prerequisite_customer_ids JSONB DEFAULT '[]',
        prerequisite_product_ids JSONB DEFAULT '[]',
        prerequisite_variant_ids JSONB DEFAULT '[]',
        prerequisite_collection_ids JSONB DEFAULT '[]',
        entitled_product_ids JSONB DEFAULT '[]',
        entitled_variant_ids JSONB DEFAULT '[]',
        entitled_collection_ids JSONB DEFAULT '[]',
        entitled_country_ids JSONB DEFAULT '[]',
        allocation_limit INTEGER,
        admin_graphql_api_id VARCHAR(255),
        starts_at TIMESTAMP WITH TIME ZONE,
        ends_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_price_rules_shop ON shopify_price_rules(shop_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_price_rules_active ON shopify_price_rules(starts_at, ends_at);
      CREATE INDEX IF NOT EXISTS idx_shopify_price_rules_source ON shopify_price_rules(source_account_id);

      -- Discount Codes
      CREATE TABLE IF NOT EXISTS shopify_discount_codes (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        price_rule_id BIGINT,
        code VARCHAR(255) NOT NULL,
        usage_count INTEGER DEFAULT 0,
        admin_graphql_api_id VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_discount_codes_price_rule ON shopify_discount_codes(price_rule_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_discount_codes_code ON shopify_discount_codes(code);
      CREATE INDEX IF NOT EXISTS idx_shopify_discount_codes_source ON shopify_discount_codes(source_account_id);
    `);

    // Gift cards, metafields, checkouts, webhooks, and views
    await this.db.executeSqlFile(`
      -- Gift Cards
      CREATE TABLE IF NOT EXISTS shopify_gift_cards (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        shop_id BIGINT,
        balance DECIMAL(12, 2),
        initial_value DECIMAL(12, 2),
        currency VARCHAR(10) DEFAULT 'USD',
        code VARCHAR(255),
        last_characters VARCHAR(10),
        note TEXT,
        template_suffix VARCHAR(255),
        customer_id BIGINT,
        order_id BIGINT,
        line_item_id BIGINT,
        user_id BIGINT,
        disabled_at TIMESTAMP WITH TIME ZONE,
        expires_on DATE,
        admin_graphql_api_id VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_gift_cards_shop ON shopify_gift_cards(shop_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_gift_cards_customer ON shopify_gift_cards(customer_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_gift_cards_code ON shopify_gift_cards(last_characters);
      CREATE INDEX IF NOT EXISTS idx_shopify_gift_cards_source ON shopify_gift_cards(source_account_id);

      -- Metafields
      CREATE TABLE IF NOT EXISTS shopify_metafields (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        namespace VARCHAR(255) NOT NULL,
        key VARCHAR(255) NOT NULL,
        value TEXT,
        type VARCHAR(100),
        description TEXT,
        owner_id BIGINT NOT NULL,
        owner_resource VARCHAR(100) NOT NULL,
        admin_graphql_api_id VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_metafields_owner ON shopify_metafields(owner_resource, owner_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_metafields_namespace ON shopify_metafields(namespace, key);
      CREATE INDEX IF NOT EXISTS idx_shopify_metafields_source ON shopify_metafields(source_account_id);

      -- Checkouts (Abandoned)
      CREATE TABLE IF NOT EXISTS shopify_checkouts (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        token VARCHAR(255) NOT NULL,
        cart_token VARCHAR(255),
        shop_id BIGINT,
        email VARCHAR(255),
        customer_id BIGINT,
        customer_locale VARCHAR(10),
        gateway VARCHAR(100),
        currency VARCHAR(10) DEFAULT 'USD',
        subtotal_price DECIMAL(12, 2),
        total_price DECIMAL(12, 2),
        total_tax DECIMAL(12, 2),
        total_discounts DECIMAL(12, 2),
        total_line_items_price DECIMAL(12, 2),
        taxes_included BOOLEAN DEFAULT FALSE,
        discount_codes JSONB DEFAULT '[]',
        line_items JSONB DEFAULT '[]',
        tax_lines JSONB DEFAULT '[]',
        shipping_line JSONB,
        shipping_address JSONB,
        billing_address JSONB,
        note TEXT,
        note_attributes JSONB DEFAULT '[]',
        landing_site VARCHAR(2048),
        referring_site VARCHAR(2048),
        source_name VARCHAR(100),
        source_identifier VARCHAR(255),
        source_url VARCHAR(2048),
        completed_at TIMESTAMP WITH TIME ZONE,
        abandoned_checkout_url VARCHAR(2048),
        admin_graphql_api_id VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_checkouts_shop ON shopify_checkouts(shop_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_checkouts_customer ON shopify_checkouts(customer_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_checkouts_token ON shopify_checkouts(token);
      CREATE INDEX IF NOT EXISTS idx_shopify_checkouts_completed ON shopify_checkouts(completed_at);
      CREATE INDEX IF NOT EXISTS idx_shopify_checkouts_source ON shopify_checkouts(source_account_id);

      -- Webhook Events
      CREATE TABLE IF NOT EXISTS shopify_webhook_events (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        topic VARCHAR(100) NOT NULL,
        shop_id BIGINT,
        shop_domain VARCHAR(255),
        data JSONB NOT NULL,
        processed BOOLEAN DEFAULT FALSE,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_shopify_events_topic ON shopify_webhook_events(topic);
      CREATE INDEX IF NOT EXISTS idx_shopify_events_shop ON shopify_webhook_events(shop_id);
      CREATE INDEX IF NOT EXISTS idx_shopify_events_processed ON shopify_webhook_events(processed);
      CREATE INDEX IF NOT EXISTS idx_shopify_events_received ON shopify_webhook_events(received_at);
      CREATE INDEX IF NOT EXISTS idx_shopify_events_source ON shopify_webhook_events(source_account_id);
    `);

    // Create views (include source_account_id in all views)
    await this.db.executeSqlFile(`
      -- Sales Overview
      CREATE OR REPLACE VIEW shopify_sales_overview AS
      SELECT
        o.source_account_id,
        DATE(o.created_at) AS order_date,
        COUNT(*) AS order_count,
        SUM(o.total_price) AS revenue,
        AVG(o.total_price) AS avg_order_value,
        COUNT(DISTINCT o.customer_id) AS unique_customers
      FROM shopify_orders o
      WHERE o.financial_status = 'paid'
        AND o.test = false
      GROUP BY o.source_account_id, DATE(o.created_at)
      ORDER BY order_date DESC;

      -- Top Products
      CREATE OR REPLACE VIEW shopify_top_products AS
      SELECT
        p.source_account_id,
        p.id,
        p.title,
        p.vendor,
        COUNT(DISTINCT li.order_id) AS order_count,
        SUM(li.quantity) AS units_sold,
        SUM(li.quantity * li.price) AS revenue
      FROM shopify_products p
      JOIN shopify_order_items li ON p.id = li.product_id AND p.source_account_id = li.source_account_id
      JOIN shopify_orders o ON li.order_id = o.id AND li.source_account_id = o.source_account_id
      WHERE o.financial_status = 'paid'
        AND o.test = false
      GROUP BY p.source_account_id, p.id, p.title, p.vendor
      ORDER BY revenue DESC;

      -- Low Inventory
      CREATE OR REPLACE VIEW shopify_low_inventory AS
      SELECT
        i.source_account_id,
        p.id AS product_id,
        p.title AS product_title,
        v.id AS variant_id,
        v.title AS variant_title,
        v.sku,
        i.available,
        i.on_hand
      FROM shopify_inventory i
      JOIN shopify_variants v ON i.variant_id = v.id AND i.source_account_id = v.source_account_id
      JOIN shopify_products p ON v.product_id = p.id AND v.source_account_id = p.source_account_id
      WHERE i.available <= 5
        AND p.status = 'active'
      ORDER BY i.available ASC;

      -- Customer Lifetime Value
      CREATE OR REPLACE VIEW shopify_customer_value AS
      SELECT
        c.source_account_id,
        c.id,
        c.email,
        c.first_name || ' ' || c.last_name AS name,
        c.orders_count,
        c.total_spent,
        CASE WHEN c.orders_count > 0
             THEN c.total_spent / c.orders_count
             ELSE 0
        END AS avg_order_value,
        c.created_at AS customer_since
      FROM shopify_customers c
      WHERE c.orders_count > 0
      ORDER BY c.total_spent DESC;

      -- Fulfillment Status
      CREATE OR REPLACE VIEW shopify_fulfillment_status AS
      SELECT
        o.source_account_id,
        o.id AS order_id,
        o.name AS order_name,
        o.fulfillment_status,
        f.id AS fulfillment_id,
        f.status AS fulfillment_detail_status,
        f.tracking_company,
        f.tracking_number,
        f.shipment_status,
        f.created_at AS fulfilled_at
      FROM shopify_orders o
      LEFT JOIN shopify_fulfillments f ON o.id = f.order_id AND o.source_account_id = f.source_account_id
      WHERE o.fulfillment_status IS NOT NULL
      ORDER BY o.created_at DESC;

      -- Active Discounts
      CREATE OR REPLACE VIEW shopify_active_discounts AS
      SELECT
        pr.source_account_id,
        pr.id AS price_rule_id,
        pr.title,
        pr.value_type,
        pr.value,
        pr.target_type,
        pr.customer_selection,
        pr.usage_limit,
        dc.id AS discount_code_id,
        dc.code,
        dc.usage_count,
        pr.starts_at,
        pr.ends_at
      FROM shopify_price_rules pr
      LEFT JOIN shopify_discount_codes dc ON pr.id = dc.price_rule_id AND pr.source_account_id = dc.source_account_id
      WHERE pr.starts_at <= NOW()
        AND (pr.ends_at IS NULL OR pr.ends_at >= NOW())
      ORDER BY pr.starts_at DESC;

      -- Abandoned Checkouts
      CREATE OR REPLACE VIEW shopify_abandoned_checkouts AS
      SELECT
        ch.source_account_id,
        ch.id,
        ch.email,
        ch.customer_id,
        ch.total_price,
        ch.currency,
        ch.abandoned_checkout_url,
        ch.created_at,
        ch.updated_at,
        EXTRACT(EPOCH FROM (NOW() - ch.updated_at)) / 3600 AS hours_abandoned
      FROM shopify_checkouts ch
      WHERE ch.completed_at IS NULL
        AND ch.total_price > 0
      ORDER BY ch.updated_at DESC;

      -- Gift Card Balances
      CREATE OR REPLACE VIEW shopify_gift_card_summary AS
      SELECT
        gc.source_account_id,
        gc.id,
        gc.last_characters,
        gc.initial_value,
        gc.balance,
        gc.initial_value - gc.balance AS amount_used,
        gc.currency,
        gc.customer_id,
        gc.disabled_at,
        gc.expires_on,
        gc.created_at
      FROM shopify_gift_cards gc
      WHERE gc.disabled_at IS NULL
        AND (gc.expires_on IS NULL OR gc.expires_on >= CURRENT_DATE)
      ORDER BY gc.balance DESC;
    `);

    // Run migration for existing databases that lack source_account_id
    await this.migrateForMultiApp();

    logger.success('Shopify schema initialized');
  }

  /**
   * Migrate existing Shopify tables to add source_account_id and composite PKs.
   * Safe to run multiple times (idempotent).
   */
  private async migrateForMultiApp(): Promise<void> {
    const migrationCheck = await this.db.queryOne<{ exists: boolean }>(
      `SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'shopify_products' AND column_name = 'source_account_id'
      )`
    );

    if (migrationCheck?.exists) {
      return; // Already migrated
    }

    logger.info('Migrating Shopify schema for multi-app support...');

    for (const { name, pk } of ALL_TABLES) {
      // shopify_inventory has a special case: it used SERIAL id + UNIQUE constraint
      if (name === 'shopify_inventory') {
        // Drop the old serial id column and unique constraint, remake with composite PK
        await this.db.query(`ALTER TABLE ${name} ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'`);
        await this.db.query(`ALTER TABLE ${name} DROP CONSTRAINT IF EXISTS ${name}_pkey`);
        await this.db.query(`ALTER TABLE ${name} DROP CONSTRAINT IF EXISTS shopify_inventory_inventory_item_id_location_id_key`);
        // Drop the old serial id column if it exists
        const hasIdCol = await this.db.queryOne<{ exists: boolean }>(
          `SELECT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'shopify_inventory' AND column_name = 'id' AND data_type = 'integer'
          )`
        );
        if (hasIdCol?.exists) {
          await this.db.query(`ALTER TABLE ${name} DROP COLUMN IF EXISTS id`);
        }
        await this.db.query(`ALTER TABLE ${name} ADD PRIMARY KEY (${pk}, source_account_id)`);
      } else {
        await this.db.query(`ALTER TABLE ${name} ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'`);
        await this.db.query(`ALTER TABLE ${name} DROP CONSTRAINT IF EXISTS ${name}_pkey`);
        await this.db.query(`ALTER TABLE ${name} ADD PRIMARY KEY (${pk}, source_account_id)`);
      }
    }

    logger.success('Shopify multi-app migration complete');
  }

  // =========================================================================
  // Cleanup
  // =========================================================================

  /**
   * Delete all data for a specific source account across all Shopify tables.
   */
  async cleanupForAccount(sourceAccountId?: string): Promise<number> {
    const accountId = sourceAccountId ?? this.sourceAccountId;
    return this.db.cleanupForAccount(CLEANUP_TABLE_ORDER, accountId);
  }

  // =========================================================================
  // Shop
  // =========================================================================

  async upsertShop(shop: ShopifyShopRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_shops (
        id, source_account_id, name, email, domain, myshopify_domain, shop_owner, phone,
        address1, address2, city, province, province_code, country,
        country_code, zip, currency, money_format, timezone, iana_timezone,
        plan_name, plan_display_name, weight_unit, primary_locale,
        created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        name = EXCLUDED.name, email = EXCLUDED.email, domain = EXCLUDED.domain,
        myshopify_domain = EXCLUDED.myshopify_domain, shop_owner = EXCLUDED.shop_owner,
        phone = EXCLUDED.phone, address1 = EXCLUDED.address1, address2 = EXCLUDED.address2,
        city = EXCLUDED.city, province = EXCLUDED.province, province_code = EXCLUDED.province_code,
        country = EXCLUDED.country, country_code = EXCLUDED.country_code, zip = EXCLUDED.zip,
        currency = EXCLUDED.currency, money_format = EXCLUDED.money_format,
        timezone = EXCLUDED.timezone, iana_timezone = EXCLUDED.iana_timezone,
        plan_name = EXCLUDED.plan_name, plan_display_name = EXCLUDED.plan_display_name,
        weight_unit = EXCLUDED.weight_unit, primary_locale = EXCLUDED.primary_locale,
        updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        shop.id, this.sourceAccountId, shop.name, shop.email, shop.domain, shop.myshopify_domain,
        shop.shop_owner, shop.phone, shop.address1, shop.address2, shop.city,
        shop.province, shop.province_code, shop.country, shop.country_code,
        shop.zip, shop.currency, shop.money_format, shop.timezone,
        shop.iana_timezone, shop.plan_name, shop.plan_display_name,
        shop.weight_unit, shop.primary_locale, shop.created_at, shop.updated_at,
      ]
    );
  }

  async getShop(): Promise<ShopifyShopRecord | null> {
    return this.db.queryOne<ShopifyShopRecord>(
      'SELECT * FROM shopify_shops WHERE source_account_id = $1 LIMIT 1',
      [this.sourceAccountId]
    );
  }

  // =========================================================================
  // Locations
  // =========================================================================

  async upsertLocation(location: ShopifyLocationRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_locations (
        id, source_account_id, shop_id, name, address1, address2, city, province, province_code,
        country, country_code, zip, phone, active, legacy, localized_country_name,
        localized_province_name, admin_graphql_api_id, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        shop_id = EXCLUDED.shop_id, name = EXCLUDED.name, address1 = EXCLUDED.address1,
        address2 = EXCLUDED.address2, city = EXCLUDED.city, province = EXCLUDED.province,
        province_code = EXCLUDED.province_code, country = EXCLUDED.country,
        country_code = EXCLUDED.country_code, zip = EXCLUDED.zip, phone = EXCLUDED.phone,
        active = EXCLUDED.active, legacy = EXCLUDED.legacy,
        localized_country_name = EXCLUDED.localized_country_name,
        localized_province_name = EXCLUDED.localized_province_name,
        admin_graphql_api_id = EXCLUDED.admin_graphql_api_id,
        updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        location.id, this.sourceAccountId, location.shop_id, location.name, location.address1, location.address2,
        location.city, location.province, location.province_code, location.country,
        location.country_code, location.zip, location.phone, location.active, location.legacy,
        location.localized_country_name, location.localized_province_name,
        location.admin_graphql_api_id, location.created_at, location.updated_at,
      ]
    );
  }

  async upsertLocations(locations: ShopifyLocationRecord[]): Promise<number> {
    for (const location of locations) await this.upsertLocation(location);
    return locations.length;
  }

  async listLocations(): Promise<ShopifyLocationRecord[]> {
    const result = await this.db.query<ShopifyLocationRecord>(
      'SELECT * FROM shopify_locations WHERE active = true AND source_account_id = $1 ORDER BY name',
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async countLocations(): Promise<number> {
    return this.db.countScoped('shopify_locations', this.sourceAccountId);
  }

  // =========================================================================
  // Products
  // =========================================================================

  async upsertProduct(product: ShopifyProductRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_products (
        id, source_account_id, shop_id, title, body_html, vendor, product_type, handle, status,
        template_suffix, published_scope, tags, admin_graphql_api_id,
        image_id, image_src, images, options, published_at, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        shop_id = EXCLUDED.shop_id, title = EXCLUDED.title, body_html = EXCLUDED.body_html,
        vendor = EXCLUDED.vendor, product_type = EXCLUDED.product_type, handle = EXCLUDED.handle,
        status = EXCLUDED.status, template_suffix = EXCLUDED.template_suffix,
        published_scope = EXCLUDED.published_scope, tags = EXCLUDED.tags,
        admin_graphql_api_id = EXCLUDED.admin_graphql_api_id, image_id = EXCLUDED.image_id,
        image_src = EXCLUDED.image_src, images = EXCLUDED.images, options = EXCLUDED.options,
        published_at = EXCLUDED.published_at, updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        product.id, this.sourceAccountId, product.shop_id, product.title, product.body_html,
        product.vendor, product.product_type, product.handle, product.status,
        product.template_suffix, product.published_scope, product.tags,
        product.admin_graphql_api_id, product.image_id, product.image_src,
        JSON.stringify(product.images), JSON.stringify(product.options),
        product.published_at, product.created_at, product.updated_at,
      ]
    );
  }

  async upsertProducts(products: ShopifyProductRecord[]): Promise<number> {
    for (const product of products) await this.upsertProduct(product);
    return products.length;
  }

  async listProducts(limit = 100, offset = 0): Promise<ShopifyProductRecord[]> {
    const result = await this.db.query<ShopifyProductRecord>(
      'SELECT * FROM shopify_products WHERE source_account_id = $1 ORDER BY updated_at DESC LIMIT $2 OFFSET $3',
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async countProducts(): Promise<number> {
    return this.db.countScoped('shopify_products', this.sourceAccountId);
  }

  async getProduct(id: number): Promise<ShopifyProductRecord | null> {
    return this.db.queryOne<ShopifyProductRecord>(
      'SELECT * FROM shopify_products WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  async deleteProduct(id: number): Promise<void> {
    await this.db.execute(
      'DELETE FROM shopify_products WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Variants
  // =========================================================================

  async upsertVariant(variant: ShopifyVariantRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_variants (
        id, source_account_id, product_id, title, price, compare_at_price, sku, barcode,
        position, grams, weight, weight_unit, inventory_item_id,
        inventory_quantity, inventory_policy, inventory_management,
        fulfillment_service, requires_shipping, taxable, option1, option2,
        option3, image_id, admin_graphql_api_id, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        product_id = EXCLUDED.product_id, title = EXCLUDED.title, price = EXCLUDED.price,
        compare_at_price = EXCLUDED.compare_at_price, sku = EXCLUDED.sku, barcode = EXCLUDED.barcode,
        position = EXCLUDED.position, grams = EXCLUDED.grams, weight = EXCLUDED.weight,
        weight_unit = EXCLUDED.weight_unit, inventory_item_id = EXCLUDED.inventory_item_id,
        inventory_quantity = EXCLUDED.inventory_quantity, inventory_policy = EXCLUDED.inventory_policy,
        inventory_management = EXCLUDED.inventory_management, fulfillment_service = EXCLUDED.fulfillment_service,
        requires_shipping = EXCLUDED.requires_shipping, taxable = EXCLUDED.taxable,
        option1 = EXCLUDED.option1, option2 = EXCLUDED.option2, option3 = EXCLUDED.option3,
        image_id = EXCLUDED.image_id, admin_graphql_api_id = EXCLUDED.admin_graphql_api_id,
        updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        variant.id, this.sourceAccountId, variant.product_id, variant.title, variant.price,
        variant.compare_at_price, variant.sku, variant.barcode, variant.position,
        variant.grams, variant.weight, variant.weight_unit, variant.inventory_item_id,
        variant.inventory_quantity, variant.inventory_policy, variant.inventory_management,
        variant.fulfillment_service, variant.requires_shipping, variant.taxable,
        variant.option1, variant.option2, variant.option3, variant.image_id,
        variant.admin_graphql_api_id, variant.created_at, variant.updated_at,
      ]
    );
  }

  async upsertVariants(variants: ShopifyVariantRecord[]): Promise<number> {
    for (const variant of variants) await this.upsertVariant(variant);
    return variants.length;
  }

  async countVariants(): Promise<number> {
    return this.db.countScoped('shopify_variants', this.sourceAccountId);
  }

  async getProductVariants(productId: number): Promise<ShopifyVariantRecord[]> {
    const result = await this.db.query<ShopifyVariantRecord>(
      'SELECT * FROM shopify_variants WHERE product_id = $1 AND source_account_id = $2 ORDER BY position',
      [productId, this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Collections
  // =========================================================================

  async upsertCollection(collection: ShopifyCollectionRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_collections (
        id, source_account_id, shop_id, title, body_html, handle, collection_type, sort_order,
        template_suffix, products_count, disjunctive, rules, image,
        published_at, published_scope, admin_graphql_api_id, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        shop_id = EXCLUDED.shop_id, title = EXCLUDED.title, body_html = EXCLUDED.body_html,
        handle = EXCLUDED.handle, collection_type = EXCLUDED.collection_type,
        sort_order = EXCLUDED.sort_order, template_suffix = EXCLUDED.template_suffix,
        products_count = EXCLUDED.products_count, disjunctive = EXCLUDED.disjunctive,
        rules = EXCLUDED.rules, image = EXCLUDED.image, published_at = EXCLUDED.published_at,
        published_scope = EXCLUDED.published_scope, admin_graphql_api_id = EXCLUDED.admin_graphql_api_id,
        updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        collection.id, this.sourceAccountId, collection.shop_id, collection.title, collection.body_html,
        collection.handle, collection.collection_type, collection.sort_order,
        collection.template_suffix, collection.products_count, collection.disjunctive,
        JSON.stringify(collection.rules), collection.image ? JSON.stringify(collection.image) : null,
        collection.published_at, collection.published_scope, collection.admin_graphql_api_id,
        collection.updated_at,
      ]
    );
  }

  async upsertCollections(collections: ShopifyCollectionRecord[]): Promise<number> {
    for (const collection of collections) await this.upsertCollection(collection);
    return collections.length;
  }

  async countCollections(): Promise<number> {
    return this.db.countScoped('shopify_collections', this.sourceAccountId);
  }

  async listCollections(limit = 100, offset = 0): Promise<ShopifyCollectionRecord[]> {
    const result = await this.db.query<ShopifyCollectionRecord>(
      'SELECT * FROM shopify_collections WHERE source_account_id = $1 ORDER BY title LIMIT $2 OFFSET $3',
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async deleteCollection(id: number): Promise<void> {
    await this.db.execute(
      'DELETE FROM shopify_collections WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Customers
  // =========================================================================

  async upsertCustomer(customer: ShopifyCustomerRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_customers (
        id, source_account_id, shop_id, email, first_name, last_name, phone, verified_email,
        accepts_marketing, accepts_marketing_updated_at, marketing_opt_in_level,
        orders_count, total_spent, state, note, tags, currency, tax_exempt,
        tax_exemptions, default_address, addresses, admin_graphql_api_id,
        created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        shop_id = EXCLUDED.shop_id, email = EXCLUDED.email, first_name = EXCLUDED.first_name,
        last_name = EXCLUDED.last_name, phone = EXCLUDED.phone, verified_email = EXCLUDED.verified_email,
        accepts_marketing = EXCLUDED.accepts_marketing, accepts_marketing_updated_at = EXCLUDED.accepts_marketing_updated_at,
        marketing_opt_in_level = EXCLUDED.marketing_opt_in_level, orders_count = EXCLUDED.orders_count,
        total_spent = EXCLUDED.total_spent, state = EXCLUDED.state, note = EXCLUDED.note,
        tags = EXCLUDED.tags, currency = EXCLUDED.currency, tax_exempt = EXCLUDED.tax_exempt,
        tax_exemptions = EXCLUDED.tax_exemptions, default_address = EXCLUDED.default_address,
        addresses = EXCLUDED.addresses, admin_graphql_api_id = EXCLUDED.admin_graphql_api_id,
        updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        customer.id, this.sourceAccountId, customer.shop_id, customer.email, customer.first_name,
        customer.last_name, customer.phone, customer.verified_email,
        customer.accepts_marketing, customer.accepts_marketing_updated_at,
        customer.marketing_opt_in_level, customer.orders_count, customer.total_spent,
        customer.state, customer.note, customer.tags, customer.currency,
        customer.tax_exempt, JSON.stringify(customer.tax_exemptions),
        customer.default_address ? JSON.stringify(customer.default_address) : null,
        JSON.stringify(customer.addresses), customer.admin_graphql_api_id,
        customer.created_at, customer.updated_at,
      ]
    );
  }

  async upsertCustomers(customers: ShopifyCustomerRecord[]): Promise<number> {
    for (const customer of customers) await this.upsertCustomer(customer);
    return customers.length;
  }

  async listCustomers(limit = 100, offset = 0): Promise<ShopifyCustomerRecord[]> {
    const result = await this.db.query<ShopifyCustomerRecord>(
      'SELECT * FROM shopify_customers WHERE source_account_id = $1 ORDER BY updated_at DESC LIMIT $2 OFFSET $3',
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async countCustomers(): Promise<number> {
    return this.db.countScoped('shopify_customers', this.sourceAccountId);
  }

  async getCustomer(id: number): Promise<ShopifyCustomerRecord | null> {
    return this.db.queryOne<ShopifyCustomerRecord>(
      'SELECT * FROM shopify_customers WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  async deleteCustomer(id: number): Promise<void> {
    await this.db.execute(
      'DELETE FROM shopify_customers WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Orders
  // =========================================================================

  async upsertOrder(order: ShopifyOrderRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_orders (
        id, source_account_id, shop_id, order_number, name, email, phone, customer_id,
        financial_status, fulfillment_status, cancel_reason, cancelled_at,
        closed_at, confirmed, contact_email, currency, current_subtotal_price,
        current_total_discounts, current_total_price, current_total_tax,
        subtotal_price, total_discounts, total_line_items_price, total_price,
        total_tax, total_weight, total_tip_received, discount_codes, note,
        note_attributes, tags, tax_lines, taxes_included, test, token,
        gateway, payment_gateway_names, processing_method, source_name,
        source_identifier, source_url, landing_site, referring_site,
        billing_address, shipping_address, shipping_lines, admin_graphql_api_id,
        processed_at, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39, $40, $41, $42, $43, $44, $45, $46, $47, $48, $49, $50, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        shop_id = EXCLUDED.shop_id, order_number = EXCLUDED.order_number, name = EXCLUDED.name,
        email = EXCLUDED.email, phone = EXCLUDED.phone, customer_id = EXCLUDED.customer_id,
        financial_status = EXCLUDED.financial_status, fulfillment_status = EXCLUDED.fulfillment_status,
        cancel_reason = EXCLUDED.cancel_reason, cancelled_at = EXCLUDED.cancelled_at,
        closed_at = EXCLUDED.closed_at, confirmed = EXCLUDED.confirmed, contact_email = EXCLUDED.contact_email,
        current_subtotal_price = EXCLUDED.current_subtotal_price, current_total_discounts = EXCLUDED.current_total_discounts,
        current_total_price = EXCLUDED.current_total_price, current_total_tax = EXCLUDED.current_total_tax,
        subtotal_price = EXCLUDED.subtotal_price, total_discounts = EXCLUDED.total_discounts,
        total_line_items_price = EXCLUDED.total_line_items_price, total_price = EXCLUDED.total_price,
        total_tax = EXCLUDED.total_tax, total_weight = EXCLUDED.total_weight,
        total_tip_received = EXCLUDED.total_tip_received, discount_codes = EXCLUDED.discount_codes,
        note = EXCLUDED.note, note_attributes = EXCLUDED.note_attributes, tags = EXCLUDED.tags,
        tax_lines = EXCLUDED.tax_lines, taxes_included = EXCLUDED.taxes_included, test = EXCLUDED.test,
        token = EXCLUDED.token, gateway = EXCLUDED.gateway, payment_gateway_names = EXCLUDED.payment_gateway_names,
        processing_method = EXCLUDED.processing_method, source_name = EXCLUDED.source_name,
        source_identifier = EXCLUDED.source_identifier, source_url = EXCLUDED.source_url,
        landing_site = EXCLUDED.landing_site, referring_site = EXCLUDED.referring_site,
        billing_address = EXCLUDED.billing_address, shipping_address = EXCLUDED.shipping_address,
        shipping_lines = EXCLUDED.shipping_lines, admin_graphql_api_id = EXCLUDED.admin_graphql_api_id,
        processed_at = EXCLUDED.processed_at, updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        order.id, this.sourceAccountId, order.shop_id, order.order_number, order.name, order.email,
        order.phone, order.customer_id, order.financial_status,
        order.fulfillment_status, order.cancel_reason, order.cancelled_at,
        order.closed_at, order.confirmed, order.contact_email, order.currency,
        order.current_subtotal_price, order.current_total_discounts,
        order.current_total_price, order.current_total_tax, order.subtotal_price,
        order.total_discounts, order.total_line_items_price, order.total_price,
        order.total_tax, order.total_weight, order.total_tip_received,
        JSON.stringify(order.discount_codes), order.note,
        JSON.stringify(order.note_attributes), order.tags,
        JSON.stringify(order.tax_lines), order.taxes_included, order.test,
        order.token, order.gateway, JSON.stringify(order.payment_gateway_names),
        order.processing_method, order.source_name, order.source_identifier,
        order.source_url, order.landing_site, order.referring_site,
        order.billing_address ? JSON.stringify(order.billing_address) : null,
        order.shipping_address ? JSON.stringify(order.shipping_address) : null,
        JSON.stringify(order.shipping_lines), order.admin_graphql_api_id,
        order.processed_at, order.created_at, order.updated_at,
      ]
    );
  }

  async upsertOrders(orders: ShopifyOrderRecord[]): Promise<number> {
    for (const order of orders) await this.upsertOrder(order);
    return orders.length;
  }

  async listOrders(status?: string, limit = 100, offset = 0): Promise<ShopifyOrderRecord[]> {
    if (status) {
      const result = await this.db.query<ShopifyOrderRecord>(
        'SELECT * FROM shopify_orders WHERE financial_status = $1 AND source_account_id = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4',
        [status, this.sourceAccountId, limit, offset]
      );
      return result.rows;
    }
    const result = await this.db.query<ShopifyOrderRecord>(
      'SELECT * FROM shopify_orders WHERE source_account_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3',
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async getOrder(id: number): Promise<ShopifyOrderRecord | null> {
    return this.db.queryOne<ShopifyOrderRecord>(
      'SELECT * FROM shopify_orders WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  async countOrders(status?: string): Promise<number> {
    if (status) return this.db.countScoped('shopify_orders', this.sourceAccountId, 'financial_status = $1', [status]);
    return this.db.countScoped('shopify_orders', this.sourceAccountId);
  }

  async deleteOrder(id: number): Promise<void> {
    await this.db.execute(
      'DELETE FROM shopify_orders WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Order Items
  // =========================================================================

  async upsertOrderItem(item: ShopifyOrderItemRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_order_items (
        id, source_account_id, order_id, product_id, variant_id, title, variant_title, sku,
        vendor, quantity, price, total_discount, fulfillment_status,
        fulfillable_quantity, fulfillment_service, grams, requires_shipping,
        taxable, tax_lines, properties, gift_card, admin_graphql_api_id, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        order_id = EXCLUDED.order_id, product_id = EXCLUDED.product_id, variant_id = EXCLUDED.variant_id,
        title = EXCLUDED.title, variant_title = EXCLUDED.variant_title, sku = EXCLUDED.sku,
        vendor = EXCLUDED.vendor, quantity = EXCLUDED.quantity, price = EXCLUDED.price,
        total_discount = EXCLUDED.total_discount, fulfillment_status = EXCLUDED.fulfillment_status,
        fulfillable_quantity = EXCLUDED.fulfillable_quantity, fulfillment_service = EXCLUDED.fulfillment_service,
        grams = EXCLUDED.grams, requires_shipping = EXCLUDED.requires_shipping, taxable = EXCLUDED.taxable,
        tax_lines = EXCLUDED.tax_lines, properties = EXCLUDED.properties, gift_card = EXCLUDED.gift_card,
        admin_graphql_api_id = EXCLUDED.admin_graphql_api_id, synced_at = NOW()`,
      [
        item.id, this.sourceAccountId, item.order_id, item.product_id, item.variant_id, item.title,
        item.variant_title, item.sku, item.vendor, item.quantity, item.price,
        item.total_discount, item.fulfillment_status, item.fulfillable_quantity,
        item.fulfillment_service, item.grams, item.requires_shipping, item.taxable,
        JSON.stringify(item.tax_lines), JSON.stringify(item.properties),
        item.gift_card, item.admin_graphql_api_id,
      ]
    );
  }

  async upsertOrderItems(items: ShopifyOrderItemRecord[]): Promise<number> {
    for (const item of items) await this.upsertOrderItem(item);
    return items.length;
  }

  async countOrderItems(): Promise<number> {
    return this.db.countScoped('shopify_order_items', this.sourceAccountId);
  }

  async getOrderItems(orderId: number): Promise<ShopifyOrderItemRecord[]> {
    const result = await this.db.query<ShopifyOrderItemRecord>(
      'SELECT * FROM shopify_order_items WHERE order_id = $1 AND source_account_id = $2 ORDER BY id',
      [orderId, this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Fulfillments
  // =========================================================================

  async upsertFulfillment(fulfillment: ShopifyFulfillmentRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_fulfillments (
        id, source_account_id, order_id, location_id, status, tracking_company, tracking_number,
        tracking_numbers, tracking_url, tracking_urls, shipment_status, service,
        name, receipt, line_items, notify_customer, admin_graphql_api_id,
        created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        order_id = EXCLUDED.order_id, location_id = EXCLUDED.location_id, status = EXCLUDED.status,
        tracking_company = EXCLUDED.tracking_company, tracking_number = EXCLUDED.tracking_number,
        tracking_numbers = EXCLUDED.tracking_numbers, tracking_url = EXCLUDED.tracking_url,
        tracking_urls = EXCLUDED.tracking_urls, shipment_status = EXCLUDED.shipment_status,
        service = EXCLUDED.service, name = EXCLUDED.name, receipt = EXCLUDED.receipt,
        line_items = EXCLUDED.line_items, notify_customer = EXCLUDED.notify_customer,
        admin_graphql_api_id = EXCLUDED.admin_graphql_api_id,
        updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        fulfillment.id, this.sourceAccountId, fulfillment.order_id, fulfillment.location_id, fulfillment.status,
        fulfillment.tracking_company, fulfillment.tracking_number,
        JSON.stringify(fulfillment.tracking_numbers), fulfillment.tracking_url,
        JSON.stringify(fulfillment.tracking_urls), fulfillment.shipment_status,
        fulfillment.service, fulfillment.name, fulfillment.receipt ? JSON.stringify(fulfillment.receipt) : null,
        JSON.stringify(fulfillment.line_items), fulfillment.notify_customer,
        fulfillment.admin_graphql_api_id, fulfillment.created_at, fulfillment.updated_at,
      ]
    );
  }

  async upsertFulfillments(fulfillments: ShopifyFulfillmentRecord[]): Promise<number> {
    for (const fulfillment of fulfillments) await this.upsertFulfillment(fulfillment);
    return fulfillments.length;
  }

  async countFulfillments(): Promise<number> {
    return this.db.countScoped('shopify_fulfillments', this.sourceAccountId);
  }

  // =========================================================================
  // Transactions
  // =========================================================================

  async upsertTransaction(transaction: ShopifyTransactionRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_transactions (
        id, source_account_id, order_id, parent_id, kind, gateway, status, message, amount, currency,
        authorization, source_name, payment_details, error_code, receipt, test,
        admin_graphql_api_id, processed_at, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        order_id = EXCLUDED.order_id, parent_id = EXCLUDED.parent_id, kind = EXCLUDED.kind,
        gateway = EXCLUDED.gateway, status = EXCLUDED.status, message = EXCLUDED.message,
        amount = EXCLUDED.amount, currency = EXCLUDED.currency, authorization = EXCLUDED.authorization,
        source_name = EXCLUDED.source_name, payment_details = EXCLUDED.payment_details,
        error_code = EXCLUDED.error_code, receipt = EXCLUDED.receipt, test = EXCLUDED.test,
        admin_graphql_api_id = EXCLUDED.admin_graphql_api_id, processed_at = EXCLUDED.processed_at,
        synced_at = NOW()`,
      [
        transaction.id, this.sourceAccountId, transaction.order_id, transaction.parent_id, transaction.kind,
        transaction.gateway, transaction.status, transaction.message, transaction.amount,
        transaction.currency, transaction.authorization, transaction.source_name,
        transaction.payment_details ? JSON.stringify(transaction.payment_details) : null,
        transaction.error_code, transaction.receipt ? JSON.stringify(transaction.receipt) : null,
        transaction.test, transaction.admin_graphql_api_id, transaction.processed_at,
        transaction.created_at,
      ]
    );
  }

  async upsertTransactions(transactions: ShopifyTransactionRecord[]): Promise<number> {
    for (const transaction of transactions) await this.upsertTransaction(transaction);
    return transactions.length;
  }

  async countTransactions(): Promise<number> {
    return this.db.countScoped('shopify_transactions', this.sourceAccountId);
  }

  // =========================================================================
  // Refunds
  // =========================================================================

  async upsertRefund(refund: ShopifyRefundRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_refunds (
        id, source_account_id, order_id, note, restock, user_id, refund_line_items, transactions,
        order_adjustments, duties, admin_graphql_api_id, processed_at, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        order_id = EXCLUDED.order_id, note = EXCLUDED.note, restock = EXCLUDED.restock,
        user_id = EXCLUDED.user_id, refund_line_items = EXCLUDED.refund_line_items,
        transactions = EXCLUDED.transactions, order_adjustments = EXCLUDED.order_adjustments,
        duties = EXCLUDED.duties, admin_graphql_api_id = EXCLUDED.admin_graphql_api_id,
        processed_at = EXCLUDED.processed_at, synced_at = NOW()`,
      [
        refund.id, this.sourceAccountId, refund.order_id, refund.note, refund.restock, refund.user_id,
        JSON.stringify(refund.refund_line_items), JSON.stringify(refund.transactions),
        JSON.stringify(refund.order_adjustments), JSON.stringify(refund.duties),
        refund.admin_graphql_api_id, refund.processed_at, refund.created_at,
      ]
    );
  }

  async upsertRefunds(refunds: ShopifyRefundRecord[]): Promise<number> {
    for (const refund of refunds) await this.upsertRefund(refund);
    return refunds.length;
  }

  async countRefunds(): Promise<number> {
    return this.db.countScoped('shopify_refunds', this.sourceAccountId);
  }

  // =========================================================================
  // Draft Orders
  // =========================================================================

  async upsertDraftOrder(draftOrder: ShopifyDraftOrderRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_draft_orders (
        id, source_account_id, shop_id, order_id, name, email, customer_id, status, currency,
        subtotal_price, total_price, total_tax, taxes_included, tax_exempt,
        tax_lines, discount_codes, applied_discount, line_items, shipping_line,
        billing_address, shipping_address, note, note_attributes, tags,
        invoice_url, invoice_sent_at, admin_graphql_api_id, completed_at,
        created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        shop_id = EXCLUDED.shop_id, order_id = EXCLUDED.order_id, name = EXCLUDED.name,
        email = EXCLUDED.email, customer_id = EXCLUDED.customer_id, status = EXCLUDED.status,
        currency = EXCLUDED.currency, subtotal_price = EXCLUDED.subtotal_price,
        total_price = EXCLUDED.total_price, total_tax = EXCLUDED.total_tax,
        taxes_included = EXCLUDED.taxes_included, tax_exempt = EXCLUDED.tax_exempt,
        tax_lines = EXCLUDED.tax_lines, discount_codes = EXCLUDED.discount_codes,
        applied_discount = EXCLUDED.applied_discount, line_items = EXCLUDED.line_items,
        shipping_line = EXCLUDED.shipping_line, billing_address = EXCLUDED.billing_address,
        shipping_address = EXCLUDED.shipping_address, note = EXCLUDED.note,
        note_attributes = EXCLUDED.note_attributes, tags = EXCLUDED.tags,
        invoice_url = EXCLUDED.invoice_url, invoice_sent_at = EXCLUDED.invoice_sent_at,
        admin_graphql_api_id = EXCLUDED.admin_graphql_api_id, completed_at = EXCLUDED.completed_at,
        updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        draftOrder.id, this.sourceAccountId, draftOrder.shop_id, draftOrder.order_id, draftOrder.name,
        draftOrder.email, draftOrder.customer_id, draftOrder.status, draftOrder.currency,
        draftOrder.subtotal_price, draftOrder.total_price, draftOrder.total_tax,
        draftOrder.taxes_included, draftOrder.tax_exempt, JSON.stringify(draftOrder.tax_lines),
        JSON.stringify(draftOrder.discount_codes), draftOrder.applied_discount ? JSON.stringify(draftOrder.applied_discount) : null,
        JSON.stringify(draftOrder.line_items), draftOrder.shipping_line ? JSON.stringify(draftOrder.shipping_line) : null,
        draftOrder.billing_address ? JSON.stringify(draftOrder.billing_address) : null,
        draftOrder.shipping_address ? JSON.stringify(draftOrder.shipping_address) : null,
        draftOrder.note, JSON.stringify(draftOrder.note_attributes), draftOrder.tags,
        draftOrder.invoice_url, draftOrder.invoice_sent_at, draftOrder.admin_graphql_api_id,
        draftOrder.completed_at, draftOrder.created_at, draftOrder.updated_at,
      ]
    );
  }

  async upsertDraftOrders(draftOrders: ShopifyDraftOrderRecord[]): Promise<number> {
    for (const draftOrder of draftOrders) await this.upsertDraftOrder(draftOrder);
    return draftOrders.length;
  }

  async countDraftOrders(): Promise<number> {
    return this.db.countScoped('shopify_draft_orders', this.sourceAccountId);
  }

  async deleteDraftOrder(id: number): Promise<void> {
    await this.db.execute(
      'DELETE FROM shopify_draft_orders WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Inventory Items
  // =========================================================================

  async upsertInventoryItem(item: ShopifyInventoryItemRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_inventory_items (
        id, source_account_id, sku, cost, country_code_of_origin, country_harmonized_system_codes,
        harmonized_system_code, province_code_of_origin, tracked, requires_shipping,
        admin_graphql_api_id, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        sku = EXCLUDED.sku, cost = EXCLUDED.cost,
        country_code_of_origin = EXCLUDED.country_code_of_origin,
        country_harmonized_system_codes = EXCLUDED.country_harmonized_system_codes,
        harmonized_system_code = EXCLUDED.harmonized_system_code,
        province_code_of_origin = EXCLUDED.province_code_of_origin,
        tracked = EXCLUDED.tracked, requires_shipping = EXCLUDED.requires_shipping,
        admin_graphql_api_id = EXCLUDED.admin_graphql_api_id,
        updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        item.id, this.sourceAccountId, item.sku, item.cost, item.country_code_of_origin,
        JSON.stringify(item.country_harmonized_system_codes), item.harmonized_system_code,
        item.province_code_of_origin, item.tracked, item.requires_shipping,
        item.admin_graphql_api_id, item.created_at, item.updated_at,
      ]
    );
  }

  async upsertInventoryItems(items: ShopifyInventoryItemRecord[]): Promise<number> {
    for (const item of items) await this.upsertInventoryItem(item);
    return items.length;
  }

  async countInventoryItems(): Promise<number> {
    return this.db.countScoped('shopify_inventory_items', this.sourceAccountId);
  }

  // =========================================================================
  // Inventory Levels
  // =========================================================================

  async upsertInventory(inventory: ShopifyInventoryRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_inventory (
        inventory_item_id, location_id, source_account_id, variant_id, available, incoming,
        committed, damaged, on_hand, quality_control, reserved, safety_stock,
        updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
      ON CONFLICT (inventory_item_id, location_id, source_account_id) DO UPDATE SET
        variant_id = EXCLUDED.variant_id, available = EXCLUDED.available,
        incoming = EXCLUDED.incoming, committed = EXCLUDED.committed,
        damaged = EXCLUDED.damaged, on_hand = EXCLUDED.on_hand,
        quality_control = EXCLUDED.quality_control, reserved = EXCLUDED.reserved,
        safety_stock = EXCLUDED.safety_stock, updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        inventory.inventory_item_id, inventory.location_id, this.sourceAccountId, inventory.variant_id,
        inventory.available, inventory.incoming, inventory.committed,
        inventory.damaged, inventory.on_hand, inventory.quality_control,
        inventory.reserved, inventory.safety_stock, inventory.updated_at,
      ]
    );
  }

  async upsertInventoryLevels(levels: ShopifyInventoryRecord[]): Promise<number> {
    for (const level of levels) await this.upsertInventory(level);
    return levels.length;
  }

  async countInventory(): Promise<number> {
    return this.db.countScoped('shopify_inventory', this.sourceAccountId);
  }

  async listInventory(limit = 100, offset = 0): Promise<ShopifyInventoryRecord[]> {
    const result = await this.db.query<ShopifyInventoryRecord>(
      'SELECT * FROM shopify_inventory WHERE source_account_id = $1 ORDER BY inventory_item_id LIMIT $2 OFFSET $3',
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  // =========================================================================
  // Price Rules
  // =========================================================================

  async upsertPriceRule(priceRule: ShopifyPriceRuleRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_price_rules (
        id, source_account_id, shop_id, title, target_type, target_selection, allocation_method,
        value_type, value, once_per_customer, usage_limit, customer_selection,
        prerequisite_subtotal_range, prerequisite_quantity_range, prerequisite_shipping_price_range,
        prerequisite_customer_ids, prerequisite_product_ids, prerequisite_variant_ids,
        prerequisite_collection_ids, entitled_product_ids, entitled_variant_ids,
        entitled_collection_ids, entitled_country_ids, allocation_limit,
        admin_graphql_api_id, starts_at, ends_at, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        shop_id = EXCLUDED.shop_id, title = EXCLUDED.title, target_type = EXCLUDED.target_type,
        target_selection = EXCLUDED.target_selection, allocation_method = EXCLUDED.allocation_method,
        value_type = EXCLUDED.value_type, value = EXCLUDED.value,
        once_per_customer = EXCLUDED.once_per_customer, usage_limit = EXCLUDED.usage_limit,
        customer_selection = EXCLUDED.customer_selection,
        prerequisite_subtotal_range = EXCLUDED.prerequisite_subtotal_range,
        prerequisite_quantity_range = EXCLUDED.prerequisite_quantity_range,
        prerequisite_shipping_price_range = EXCLUDED.prerequisite_shipping_price_range,
        prerequisite_customer_ids = EXCLUDED.prerequisite_customer_ids,
        prerequisite_product_ids = EXCLUDED.prerequisite_product_ids,
        prerequisite_variant_ids = EXCLUDED.prerequisite_variant_ids,
        prerequisite_collection_ids = EXCLUDED.prerequisite_collection_ids,
        entitled_product_ids = EXCLUDED.entitled_product_ids,
        entitled_variant_ids = EXCLUDED.entitled_variant_ids,
        entitled_collection_ids = EXCLUDED.entitled_collection_ids,
        entitled_country_ids = EXCLUDED.entitled_country_ids,
        allocation_limit = EXCLUDED.allocation_limit,
        admin_graphql_api_id = EXCLUDED.admin_graphql_api_id,
        starts_at = EXCLUDED.starts_at, ends_at = EXCLUDED.ends_at,
        updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        priceRule.id, this.sourceAccountId, priceRule.shop_id, priceRule.title, priceRule.target_type,
        priceRule.target_selection, priceRule.allocation_method, priceRule.value_type,
        priceRule.value, priceRule.once_per_customer, priceRule.usage_limit,
        priceRule.customer_selection,
        priceRule.prerequisite_subtotal_range ? JSON.stringify(priceRule.prerequisite_subtotal_range) : null,
        priceRule.prerequisite_quantity_range ? JSON.stringify(priceRule.prerequisite_quantity_range) : null,
        priceRule.prerequisite_shipping_price_range ? JSON.stringify(priceRule.prerequisite_shipping_price_range) : null,
        JSON.stringify(priceRule.prerequisite_customer_ids),
        JSON.stringify(priceRule.prerequisite_product_ids),
        JSON.stringify(priceRule.prerequisite_variant_ids),
        JSON.stringify(priceRule.prerequisite_collection_ids),
        JSON.stringify(priceRule.entitled_product_ids),
        JSON.stringify(priceRule.entitled_variant_ids),
        JSON.stringify(priceRule.entitled_collection_ids),
        JSON.stringify(priceRule.entitled_country_ids),
        priceRule.allocation_limit, priceRule.admin_graphql_api_id,
        priceRule.starts_at, priceRule.ends_at, priceRule.created_at, priceRule.updated_at,
      ]
    );
  }

  async upsertPriceRules(priceRules: ShopifyPriceRuleRecord[]): Promise<number> {
    for (const priceRule of priceRules) await this.upsertPriceRule(priceRule);
    return priceRules.length;
  }

  async countPriceRules(): Promise<number> {
    return this.db.countScoped('shopify_price_rules', this.sourceAccountId);
  }

  async deletePriceRule(id: number): Promise<void> {
    await this.db.execute(
      'DELETE FROM shopify_price_rules WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Discount Codes
  // =========================================================================

  async upsertDiscountCode(discountCode: ShopifyDiscountCodeRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_discount_codes (
        id, source_account_id, price_rule_id, code, usage_count, admin_graphql_api_id,
        created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        price_rule_id = EXCLUDED.price_rule_id, code = EXCLUDED.code,
        usage_count = EXCLUDED.usage_count, admin_graphql_api_id = EXCLUDED.admin_graphql_api_id,
        updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        discountCode.id, this.sourceAccountId, discountCode.price_rule_id, discountCode.code,
        discountCode.usage_count, discountCode.admin_graphql_api_id,
        discountCode.created_at, discountCode.updated_at,
      ]
    );
  }

  async upsertDiscountCodes(discountCodes: ShopifyDiscountCodeRecord[]): Promise<number> {
    for (const discountCode of discountCodes) await this.upsertDiscountCode(discountCode);
    return discountCodes.length;
  }

  async countDiscountCodes(): Promise<number> {
    return this.db.countScoped('shopify_discount_codes', this.sourceAccountId);
  }

  async deleteDiscountCode(id: number): Promise<void> {
    await this.db.execute(
      'DELETE FROM shopify_discount_codes WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Gift Cards
  // =========================================================================

  async upsertGiftCard(giftCard: ShopifyGiftCardRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_gift_cards (
        id, source_account_id, shop_id, balance, initial_value, currency, code, last_characters,
        note, template_suffix, customer_id, order_id, line_item_id, user_id,
        disabled_at, expires_on, admin_graphql_api_id, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        shop_id = EXCLUDED.shop_id, balance = EXCLUDED.balance, initial_value = EXCLUDED.initial_value,
        currency = EXCLUDED.currency, code = EXCLUDED.code, last_characters = EXCLUDED.last_characters,
        note = EXCLUDED.note, template_suffix = EXCLUDED.template_suffix,
        customer_id = EXCLUDED.customer_id, order_id = EXCLUDED.order_id,
        line_item_id = EXCLUDED.line_item_id, user_id = EXCLUDED.user_id,
        disabled_at = EXCLUDED.disabled_at, expires_on = EXCLUDED.expires_on,
        admin_graphql_api_id = EXCLUDED.admin_graphql_api_id,
        updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        giftCard.id, this.sourceAccountId, giftCard.shop_id, giftCard.balance, giftCard.initial_value,
        giftCard.currency, giftCard.code, giftCard.last_characters, giftCard.note,
        giftCard.template_suffix, giftCard.customer_id, giftCard.order_id,
        giftCard.line_item_id, giftCard.user_id, giftCard.disabled_at,
        giftCard.expires_on, giftCard.admin_graphql_api_id, giftCard.created_at,
        giftCard.updated_at,
      ]
    );
  }

  async upsertGiftCards(giftCards: ShopifyGiftCardRecord[]): Promise<number> {
    for (const giftCard of giftCards) await this.upsertGiftCard(giftCard);
    return giftCards.length;
  }

  async countGiftCards(): Promise<number> {
    return this.db.countScoped('shopify_gift_cards', this.sourceAccountId);
  }

  async disableGiftCard(id: number): Promise<void> {
    await this.db.execute(
      'UPDATE shopify_gift_cards SET disabled_at = NOW() WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Metafields
  // =========================================================================

  async upsertMetafield(metafield: ShopifyMetafieldRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_metafields (
        id, source_account_id, namespace, key, value, type, description, owner_id, owner_resource,
        admin_graphql_api_id, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        namespace = EXCLUDED.namespace, key = EXCLUDED.key, value = EXCLUDED.value,
        type = EXCLUDED.type, description = EXCLUDED.description,
        owner_id = EXCLUDED.owner_id, owner_resource = EXCLUDED.owner_resource,
        admin_graphql_api_id = EXCLUDED.admin_graphql_api_id,
        updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        metafield.id, this.sourceAccountId, metafield.namespace, metafield.key, metafield.value,
        metafield.type, metafield.description, metafield.owner_id,
        metafield.owner_resource, metafield.admin_graphql_api_id,
        metafield.created_at, metafield.updated_at,
      ]
    );
  }

  async upsertMetafields(metafields: ShopifyMetafieldRecord[]): Promise<number> {
    for (const metafield of metafields) await this.upsertMetafield(metafield);
    return metafields.length;
  }

  async countMetafields(): Promise<number> {
    return this.db.countScoped('shopify_metafields', this.sourceAccountId);
  }

  async deleteMetafield(id: number): Promise<void> {
    await this.db.execute(
      'DELETE FROM shopify_metafields WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Checkouts
  // =========================================================================

  async upsertCheckout(checkout: ShopifyCheckoutRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_checkouts (
        id, source_account_id, token, cart_token, shop_id, email, customer_id, customer_locale,
        gateway, currency, subtotal_price, total_price, total_tax, total_discounts,
        total_line_items_price, taxes_included, discount_codes, line_items, tax_lines,
        shipping_line, shipping_address, billing_address, note, note_attributes,
        landing_site, referring_site, source_name, source_identifier, source_url,
        completed_at, abandoned_checkout_url, admin_graphql_api_id, created_at,
        updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        token = EXCLUDED.token, cart_token = EXCLUDED.cart_token, shop_id = EXCLUDED.shop_id,
        email = EXCLUDED.email, customer_id = EXCLUDED.customer_id,
        customer_locale = EXCLUDED.customer_locale, gateway = EXCLUDED.gateway,
        currency = EXCLUDED.currency, subtotal_price = EXCLUDED.subtotal_price,
        total_price = EXCLUDED.total_price, total_tax = EXCLUDED.total_tax,
        total_discounts = EXCLUDED.total_discounts,
        total_line_items_price = EXCLUDED.total_line_items_price,
        taxes_included = EXCLUDED.taxes_included, discount_codes = EXCLUDED.discount_codes,
        line_items = EXCLUDED.line_items, tax_lines = EXCLUDED.tax_lines,
        shipping_line = EXCLUDED.shipping_line, shipping_address = EXCLUDED.shipping_address,
        billing_address = EXCLUDED.billing_address, note = EXCLUDED.note,
        note_attributes = EXCLUDED.note_attributes, landing_site = EXCLUDED.landing_site,
        referring_site = EXCLUDED.referring_site, source_name = EXCLUDED.source_name,
        source_identifier = EXCLUDED.source_identifier, source_url = EXCLUDED.source_url,
        completed_at = EXCLUDED.completed_at, abandoned_checkout_url = EXCLUDED.abandoned_checkout_url,
        admin_graphql_api_id = EXCLUDED.admin_graphql_api_id,
        updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
      [
        checkout.id, this.sourceAccountId, checkout.token, checkout.cart_token, checkout.shop_id,
        checkout.email, checkout.customer_id, checkout.customer_locale,
        checkout.gateway, checkout.currency, checkout.subtotal_price,
        checkout.total_price, checkout.total_tax, checkout.total_discounts,
        checkout.total_line_items_price, checkout.taxes_included,
        JSON.stringify(checkout.discount_codes), JSON.stringify(checkout.line_items),
        JSON.stringify(checkout.tax_lines),
        checkout.shipping_line ? JSON.stringify(checkout.shipping_line) : null,
        checkout.shipping_address ? JSON.stringify(checkout.shipping_address) : null,
        checkout.billing_address ? JSON.stringify(checkout.billing_address) : null,
        checkout.note, JSON.stringify(checkout.note_attributes),
        checkout.landing_site, checkout.referring_site, checkout.source_name,
        checkout.source_identifier, checkout.source_url, checkout.completed_at,
        checkout.abandoned_checkout_url, checkout.admin_graphql_api_id,
        checkout.created_at, checkout.updated_at,
      ]
    );
  }

  async upsertCheckouts(checkouts: ShopifyCheckoutRecord[]): Promise<number> {
    for (const checkout of checkouts) await this.upsertCheckout(checkout);
    return checkouts.length;
  }

  async countCheckouts(): Promise<number> {
    return this.db.countScoped('shopify_checkouts', this.sourceAccountId);
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(event: ShopifyWebhookEventRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO shopify_webhook_events (
        id, source_account_id, topic, shop_id, shop_domain, data, processed, processed_at, error, received_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        processed = EXCLUDED.processed, processed_at = EXCLUDED.processed_at, error = EXCLUDED.error`,
      [
        event.id, this.sourceAccountId, event.topic, event.shop_id, event.shop_domain,
        JSON.stringify(event.data), event.processed, event.processed_at, event.error,
      ]
    );
  }

  async markEventProcessed(id: string, error?: string): Promise<void> {
    await this.db.execute(
      `UPDATE shopify_webhook_events SET
        processed = TRUE, processed_at = NOW(), error = $2
      WHERE id = $1 AND source_account_id = $3`,
      [id, error ?? null, this.sourceAccountId]
    );
  }

  async listWebhookEvents(topic?: string, limit = 50): Promise<ShopifyWebhookEventRecord[]> {
    if (topic) {
      const result = await this.db.query<ShopifyWebhookEventRecord>(
        'SELECT * FROM shopify_webhook_events WHERE topic = $1 AND source_account_id = $2 ORDER BY received_at DESC LIMIT $3',
        [topic, this.sourceAccountId, limit]
      );
      return result.rows;
    }
    const result = await this.db.query<ShopifyWebhookEventRecord>(
      'SELECT * FROM shopify_webhook_events WHERE source_account_id = $1 ORDER BY received_at DESC LIMIT $2',
      [this.sourceAccountId, limit]
    );
    return result.rows;
  }

  // =========================================================================
  // Raw Query (for analytics)
  // =========================================================================

  async query<T extends Record<string, unknown> = Record<string, unknown>>(
    sql: string,
    params?: unknown[]
  ): Promise<{ rows: T[] }> {
    return this.db.query<T>(sql, params);
  }

  // =========================================================================
  // Stats
  // =========================================================================

  async getStats(): Promise<SyncStats> {
    const [
      shops, locations, products, variants, collections, customers, orders,
      orderItems, fulfillments, transactions, refunds, draftOrders,
      inventory, inventoryItems, priceRules, discountCodes, giftCards, metafields, checkouts,
    ] = await Promise.all([
      this.db.countScoped('shopify_shops', this.sourceAccountId),
      this.countLocations(),
      this.countProducts(),
      this.countVariants(),
      this.countCollections(),
      this.countCustomers(),
      this.countOrders(),
      this.countOrderItems(),
      this.countFulfillments(),
      this.countTransactions(),
      this.countRefunds(),
      this.countDraftOrders(),
      this.countInventory(),
      this.countInventoryItems(),
      this.countPriceRules(),
      this.countDiscountCodes(),
      this.countGiftCards(),
      this.countMetafields(),
      this.countCheckouts(),
    ]);

    return {
      shops,
      locations,
      products,
      variants,
      collections,
      customers,
      orders,
      orderItems,
      fulfillments,
      transactions,
      refunds,
      draftOrders,
      inventory,
      inventoryItems,
      priceRules,
      discountCodes,
      giftCards,
      metafields,
      checkouts,
    };
  }
}
