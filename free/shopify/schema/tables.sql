-- =============================================================================
-- Shopify Plugin Schema
-- Tables for storing synced Shopify store data
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================================
-- Shops
-- =============================================================================

CREATE TABLE IF NOT EXISTS shopify_shops (
    id BIGINT PRIMARY KEY,                          -- Shopify shop ID
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
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- =============================================================================
-- Products
-- =============================================================================

CREATE TABLE IF NOT EXISTS shopify_products (
    id BIGINT PRIMARY KEY,                          -- Shopify product ID
    shop_id BIGINT REFERENCES shopify_shops(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    body_html TEXT,
    vendor VARCHAR(255),
    product_type VARCHAR(255),
    handle VARCHAR(255),
    status VARCHAR(50) DEFAULT 'active',            -- active, archived, draft
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
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_shopify_products_shop ON shopify_products(shop_id);
CREATE INDEX IF NOT EXISTS idx_shopify_products_handle ON shopify_products(handle);
CREATE INDEX IF NOT EXISTS idx_shopify_products_status ON shopify_products(status);
CREATE INDEX IF NOT EXISTS idx_shopify_products_vendor ON shopify_products(vendor);

-- =============================================================================
-- Product Variants
-- =============================================================================

CREATE TABLE IF NOT EXISTS shopify_variants (
    id BIGINT PRIMARY KEY,                          -- Shopify variant ID
    product_id BIGINT REFERENCES shopify_products(id) ON DELETE CASCADE,
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
    inventory_policy VARCHAR(50) DEFAULT 'deny',    -- deny, continue
    inventory_management VARCHAR(50),               -- shopify, null
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
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_shopify_variants_product ON shopify_variants(product_id);
CREATE INDEX IF NOT EXISTS idx_shopify_variants_sku ON shopify_variants(sku);
CREATE INDEX IF NOT EXISTS idx_shopify_variants_inventory ON shopify_variants(inventory_item_id);

-- =============================================================================
-- Collections
-- =============================================================================

CREATE TABLE IF NOT EXISTS shopify_collections (
    id BIGINT PRIMARY KEY,                          -- Shopify collection ID
    shop_id BIGINT REFERENCES shopify_shops(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    body_html TEXT,
    handle VARCHAR(255),
    collection_type VARCHAR(50),                    -- custom, smart
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
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_shopify_collections_shop ON shopify_collections(shop_id);
CREATE INDEX IF NOT EXISTS idx_shopify_collections_handle ON shopify_collections(handle);

-- =============================================================================
-- Customers
-- =============================================================================

CREATE TABLE IF NOT EXISTS shopify_customers (
    id BIGINT PRIMARY KEY,                          -- Shopify customer ID
    shop_id BIGINT REFERENCES shopify_shops(id) ON DELETE CASCADE,
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
    state VARCHAR(50) DEFAULT 'disabled',           -- disabled, invited, enabled, declined
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
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_shopify_customers_shop ON shopify_customers(shop_id);
CREATE INDEX IF NOT EXISTS idx_shopify_customers_email ON shopify_customers(email);
CREATE INDEX IF NOT EXISTS idx_shopify_customers_state ON shopify_customers(state);

-- =============================================================================
-- Orders
-- =============================================================================

CREATE TABLE IF NOT EXISTS shopify_orders (
    id BIGINT PRIMARY KEY,                          -- Shopify order ID
    shop_id BIGINT REFERENCES shopify_shops(id) ON DELETE CASCADE,
    order_number INTEGER NOT NULL,
    name VARCHAR(50) NOT NULL,                      -- #1001, etc.
    email VARCHAR(255),
    phone VARCHAR(50),
    customer_id BIGINT REFERENCES shopify_customers(id) ON DELETE SET NULL,
    financial_status VARCHAR(50),                   -- pending, paid, refunded, etc.
    fulfillment_status VARCHAR(50),                 -- null, partial, fulfilled
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
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_shopify_orders_shop ON shopify_orders(shop_id);
CREATE INDEX IF NOT EXISTS idx_shopify_orders_customer ON shopify_orders(customer_id);
CREATE INDEX IF NOT EXISTS idx_shopify_orders_number ON shopify_orders(order_number);
CREATE INDEX IF NOT EXISTS idx_shopify_orders_financial ON shopify_orders(financial_status);
CREATE INDEX IF NOT EXISTS idx_shopify_orders_fulfillment ON shopify_orders(fulfillment_status);
CREATE INDEX IF NOT EXISTS idx_shopify_orders_created ON shopify_orders(created_at);

-- =============================================================================
-- Order Line Items
-- =============================================================================

CREATE TABLE IF NOT EXISTS shopify_order_items (
    id BIGINT PRIMARY KEY,                          -- Shopify line item ID
    order_id BIGINT REFERENCES shopify_orders(id) ON DELETE CASCADE,
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
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_shopify_items_order ON shopify_order_items(order_id);
CREATE INDEX IF NOT EXISTS idx_shopify_items_product ON shopify_order_items(product_id);
CREATE INDEX IF NOT EXISTS idx_shopify_items_variant ON shopify_order_items(variant_id);

-- =============================================================================
-- Inventory
-- =============================================================================

CREATE TABLE IF NOT EXISTS shopify_inventory (
    id SERIAL PRIMARY KEY,
    inventory_item_id BIGINT NOT NULL,
    location_id BIGINT NOT NULL,
    variant_id BIGINT REFERENCES shopify_variants(id),
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
    UNIQUE(inventory_item_id, location_id)
);

CREATE INDEX IF NOT EXISTS idx_shopify_inventory_item ON shopify_inventory(inventory_item_id);
CREATE INDEX IF NOT EXISTS idx_shopify_inventory_location ON shopify_inventory(location_id);
CREATE INDEX IF NOT EXISTS idx_shopify_inventory_variant ON shopify_inventory(variant_id);

-- =============================================================================
-- Webhook Events
-- =============================================================================

CREATE TABLE IF NOT EXISTS shopify_webhook_events (
    id VARCHAR(255) PRIMARY KEY,                    -- Shopify webhook ID
    topic VARCHAR(100) NOT NULL,                    -- orders/create, products/update, etc.
    shop_id BIGINT,
    shop_domain VARCHAR(255),
    data JSONB NOT NULL,
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_shopify_events_topic ON shopify_webhook_events(topic);
CREATE INDEX IF NOT EXISTS idx_shopify_events_shop ON shopify_webhook_events(shop_id);
CREATE INDEX IF NOT EXISTS idx_shopify_events_processed ON shopify_webhook_events(processed);
CREATE INDEX IF NOT EXISTS idx_shopify_events_received ON shopify_webhook_events(received_at);

-- =============================================================================
-- Views
-- =============================================================================

-- Sales overview
CREATE OR REPLACE VIEW shopify_sales_overview AS
SELECT
    DATE(o.created_at) AS order_date,
    COUNT(*) AS order_count,
    SUM(o.total_price) AS revenue,
    AVG(o.total_price) AS avg_order_value,
    COUNT(DISTINCT o.customer_id) AS unique_customers
FROM shopify_orders o
WHERE o.financial_status = 'paid'
  AND o.test = false
GROUP BY DATE(o.created_at)
ORDER BY order_date DESC;

-- Top products
CREATE OR REPLACE VIEW shopify_top_products AS
SELECT
    p.id,
    p.title,
    p.vendor,
    COUNT(DISTINCT li.order_id) AS order_count,
    SUM(li.quantity) AS units_sold,
    SUM(li.quantity * li.price) AS revenue
FROM shopify_products p
JOIN shopify_order_items li ON p.id = li.product_id
JOIN shopify_orders o ON li.order_id = o.id
WHERE o.financial_status = 'paid'
  AND o.test = false
GROUP BY p.id, p.title, p.vendor
ORDER BY revenue DESC;

-- Low inventory
CREATE OR REPLACE VIEW shopify_low_inventory AS
SELECT
    p.id AS product_id,
    p.title AS product_title,
    v.id AS variant_id,
    v.title AS variant_title,
    v.sku,
    i.available,
    i.on_hand
FROM shopify_inventory i
JOIN shopify_variants v ON i.variant_id = v.id
JOIN shopify_products p ON v.product_id = p.id
WHERE i.available <= 5
  AND p.status = 'active'
ORDER BY i.available ASC;

-- Customer value
CREATE OR REPLACE VIEW shopify_customer_value AS
SELECT
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
