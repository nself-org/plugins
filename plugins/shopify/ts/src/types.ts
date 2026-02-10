/**
 * Shopify Plugin Types
 * Complete type definitions for all Shopify resources
 */

export interface ShopifyPluginConfig {
  store: string;
  accessToken: string;
  apiVersion?: string;
  webhookSecret?: string;
  port: number;
  host: string;
  syncInterval?: number;
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl?: boolean;
  };
}

// =============================================================================
// Shop Record
// =============================================================================

export interface ShopifyShopRecord {
  id: number;
  source_account_id: string;
  name: string;
  email: string | null;
  domain: string | null;
  myshopify_domain: string;
  shop_owner: string | null;
  phone: string | null;
  address1: string | null;
  address2: string | null;
  city: string | null;
  province: string | null;
  province_code: string | null;
  country: string | null;
  country_code: string | null;
  zip: string | null;
  currency: string;
  money_format: string | null;
  money_with_currency_format: string | null;
  timezone: string | null;
  iana_timezone: string | null;
  plan_name: string | null;
  plan_display_name: string | null;
  weight_unit: string;
  primary_locale: string;
  enabled_presentment_currencies: string[];
  has_storefront: boolean;
  has_discounts: boolean;
  has_gift_cards: boolean;
  eligible_for_payments: boolean;
  multi_location_enabled: boolean;
  setup_required: boolean;
  pre_launch_enabled: boolean;
  checkout_api_supported: boolean;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Location Record
// =============================================================================

export interface ShopifyLocationRecord {
  id: number;
  source_account_id: string;
  shop_id: number | null;
  name: string;
  address1: string | null;
  address2: string | null;
  city: string | null;
  province: string | null;
  province_code: string | null;
  country: string | null;
  country_code: string | null;
  zip: string | null;
  phone: string | null;
  localized_country_name: string | null;
  localized_province_name: string | null;
  active: boolean;
  legacy: boolean;
  fulfills_online_orders?: boolean;
  admin_graphql_api_id: string | null;
  created_at: Date | null;
  updated_at: Date | null;
}

// =============================================================================
// Product Record
// =============================================================================

export interface ShopifyProductRecord {
  id: number;
  source_account_id: string;
  shop_id: number | null;
  title: string;
  body_html: string | null;
  vendor: string | null;
  product_type: string | null;
  handle: string | null;
  status: string;
  template_suffix: string | null;
  published_scope: string | null;
  tags: string | null;
  admin_graphql_api_id: string | null;
  image_id: number | null;
  image_src: string | null;
  images: ShopifyImage[];
  options: ShopifyProductOption[];
  published_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface ShopifyImage {
  id: number;
  position: number;
  src: string;
  width: number;
  height: number;
  alt: string | null;
}

export interface ShopifyProductOption {
  id: number;
  name: string;
  position: number;
  values: string[];
}

// =============================================================================
// Variant Record
// =============================================================================

export interface ShopifyVariantRecord {
  id: number;
  source_account_id: string;
  product_id: number;
  title: string | null;
  price: number;
  compare_at_price: number | null;
  sku: string | null;
  barcode: string | null;
  position: number;
  grams: number;
  weight: number | null;
  weight_unit: string;
  inventory_item_id: number | null;
  inventory_quantity: number;
  inventory_policy: string;
  inventory_management: string | null;
  fulfillment_service: string;
  requires_shipping: boolean;
  taxable: boolean;
  option1: string | null;
  option2: string | null;
  option3: string | null;
  image_id: number | null;
  admin_graphql_api_id: string | null;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Collection Record
// =============================================================================

export interface ShopifyCollectionRecord {
  id: number;
  source_account_id: string;
  shop_id: number | null;
  title: string;
  body_html: string | null;
  handle: string | null;
  collection_type: string | null;
  sort_order: string | null;
  template_suffix: string | null;
  products_count: number;
  disjunctive: boolean;
  rules: ShopifyCollectionRule[];
  image: ShopifyImage | null;
  published_at: Date | null;
  published_scope: string;
  admin_graphql_api_id: string | null;
  updated_at: Date;
}

export interface ShopifyCollectionRule {
  column: string;
  relation: string;
  condition: string;
}

// =============================================================================
// Customer Record
// =============================================================================

export interface ShopifyCustomerRecord {
  id: number;
  source_account_id: string;
  shop_id: number | null;
  email: string | null;
  first_name: string | null;
  last_name: string | null;
  phone: string | null;
  verified_email: boolean;
  accepts_marketing: boolean;
  accepts_marketing_updated_at: Date | null;
  marketing_opt_in_level: string | null;
  sms_marketing_consent: ShopifySMSMarketingConsent | null;
  orders_count: number;
  total_spent: number;
  state: string;
  note: string | null;
  tags: string | null;
  currency: string | null;
  tax_exempt: boolean;
  tax_exemptions: string[];
  default_address: ShopifyAddress | null;
  addresses: ShopifyAddress[];
  admin_graphql_api_id: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface ShopifySMSMarketingConsent {
  state: string;
  opt_in_level: string;
  consent_updated_at: string | null;
  consent_collected_from: string | null;
}

export interface ShopifyAddress {
  id: number;
  customer_id: number;
  first_name: string | null;
  last_name: string | null;
  company: string | null;
  address1: string | null;
  address2: string | null;
  city: string | null;
  province: string | null;
  province_code: string | null;
  country: string | null;
  country_code: string | null;
  zip: string | null;
  phone: string | null;
  name: string | null;
  default: boolean;
}

// =============================================================================
// Order Record
// =============================================================================

export interface ShopifyOrderRecord {
  id: number;
  source_account_id: string;
  shop_id: number | null;
  order_number: number;
  name: string;
  email: string | null;
  phone: string | null;
  customer_id: number | null;
  financial_status: string | null;
  fulfillment_status: string | null;
  cancel_reason: string | null;
  cancelled_at: Date | null;
  closed_at: Date | null;
  confirmed: boolean;
  contact_email: string | null;
  currency: string;
  current_subtotal_price: number | null;
  current_total_discounts: number | null;
  current_total_price: number | null;
  current_total_tax: number | null;
  subtotal_price: number | null;
  total_discounts: number | null;
  total_line_items_price: number | null;
  total_price: number | null;
  total_tax: number | null;
  total_weight: number;
  total_tip_received: number;
  discount_codes: ShopifyDiscountCode[];
  discount_applications: ShopifyDiscountApplication[];
  note: string | null;
  note_attributes: ShopifyNoteAttribute[];
  tags: string | null;
  tax_lines: ShopifyTaxLine[];
  taxes_included: boolean;
  test: boolean;
  token: string | null;
  gateway: string | null;
  payment_gateway_names: string[];
  processing_method: string | null;
  source_name: string | null;
  source_identifier: string | null;
  source_url: string | null;
  landing_site: string | null;
  referring_site: string | null;
  billing_address: ShopifyAddress | null;
  shipping_address: ShopifyAddress | null;
  shipping_lines: ShopifyShippingLine[];
  client_details: ShopifyClientDetails | null;
  checkout_token: string | null;
  checkout_id: number | null;
  cart_token: string | null;
  browser_ip: string | null;
  buyer_accepts_marketing: boolean;
  app_id: number | null;
  location_id: number | null;
  device_id: number | null;
  admin_graphql_api_id: string | null;
  processed_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface ShopifyDiscountCode {
  code: string;
  amount: string;
  type: string;
}

export interface ShopifyDiscountApplication {
  type: string;
  value: string;
  value_type: string;
  allocation_method: string;
  target_selection: string;
  target_type: string;
  code: string | null;
  title: string | null;
  description: string | null;
}

export interface ShopifyNoteAttribute {
  name: string;
  value: string;
}

export interface ShopifyTaxLine {
  title: string;
  price: string;
  rate: number;
  channel_liable: boolean;
}

export interface ShopifyShippingLine {
  id: number;
  title: string;
  price: string;
  code: string | null;
  source: string | null;
  carrier_identifier: string | null;
  requested_fulfillment_service_id: string | null;
  discounted_price: string | null;
  discount_allocations: ShopifyDiscountAllocation[];
  tax_lines: ShopifyTaxLine[];
}

export interface ShopifyDiscountAllocation {
  amount: string;
  discount_application_index: number;
}

export interface ShopifyClientDetails {
  browser_ip: string | null;
  accept_language: string | null;
  user_agent: string | null;
  session_hash: string | null;
  browser_width: number | null;
  browser_height: number | null;
}

// =============================================================================
// Order Item Record
// =============================================================================

export interface ShopifyOrderItemRecord {
  id: number;
  source_account_id: string;
  order_id: number;
  product_id: number | null;
  variant_id: number | null;
  title: string;
  variant_title: string | null;
  sku: string | null;
  vendor: string | null;
  quantity: number;
  price: number | null;
  total_discount: number;
  fulfillment_status: string | null;
  fulfillable_quantity: number;
  fulfillment_service: string | null;
  grams: number;
  requires_shipping: boolean;
  taxable: boolean;
  tax_lines: ShopifyTaxLine[];
  discount_allocations: ShopifyDiscountAllocation[];
  properties: ShopifyLineItemProperty[];
  gift_card: boolean;
  admin_graphql_api_id: string | null;
}

export interface ShopifyLineItemProperty {
  name: string;
  value: string;
}

// =============================================================================
// Fulfillment Record
// =============================================================================

export interface ShopifyFulfillmentRecord {
  id: number;
  source_account_id: string;
  order_id: number;
  location_id: number | null;
  name: string;
  status: string;
  service: string | null;
  tracking_company: string | null;
  tracking_number: string | null;
  tracking_numbers: string[];
  tracking_url: string | null;
  tracking_urls: string[];
  shipment_status: string | null;
  receipt: ShopifyFulfillmentReceipt | null;
  line_items: ShopifyFulfillmentLineItem[];
  notify_customer: boolean;
  variant_inventory_management: string | null;
  admin_graphql_api_id: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface ShopifyFulfillmentReceipt {
  testcase: boolean;
  authorization: string | null;
}

export interface ShopifyFulfillmentLineItem {
  id: number;
  variant_id: number | null;
  product_id: number | null;
  title: string;
  quantity: number;
}

// =============================================================================
// Transaction Record
// =============================================================================

export interface ShopifyTransactionRecord {
  id: number;
  source_account_id: string;
  order_id: number;
  amount: number;
  currency: string;
  kind: string;
  status: string;
  gateway: string;
  test: boolean;
  authorization: string | null;
  error_code: string | null;
  message: string | null;
  parent_id: number | null;
  source_name: string | null;
  receipt: Record<string, unknown>;
  payment_details: ShopifyPaymentDetails | null;
  payment_id: string | null;
  location_id: number | null;
  device_id: number | null;
  admin_graphql_api_id: string | null;
  processed_at: Date;
  created_at: Date;
}

export interface ShopifyPaymentDetails {
  credit_card_bin: string | null;
  avs_result_code: string | null;
  cvv_result_code: string | null;
  credit_card_number: string | null;
  credit_card_company: string | null;
  credit_card_name: string | null;
  credit_card_wallet: string | null;
  credit_card_expiration_month: number | null;
  credit_card_expiration_year: number | null;
}

// =============================================================================
// Refund Record
// =============================================================================

export interface ShopifyRefundRecord {
  id: number;
  source_account_id: string;
  order_id: number;
  note: string | null;
  restock: boolean;
  user_id: number | null;
  processed_at: Date | null;
  refund_line_items: ShopifyRefundLineItem[];
  transactions: ShopifyRefundTransaction[];
  order_adjustments: ShopifyOrderAdjustment[];
  duties: ShopifyRefundDuty[];
  admin_graphql_api_id: string | null;
  created_at: Date;
}

export interface ShopifyRefundLineItem {
  id: number;
  line_item_id: number;
  quantity: number;
  restock_type: string;
  location_id: number | null;
  subtotal: number;
  total_tax: number;
}

export interface ShopifyRefundTransaction {
  id: number;
  order_id: number;
  amount: number;
  kind: string;
  gateway: string;
  status: string;
  message: string | null;
  created_at: Date;
}

export interface ShopifyOrderAdjustment {
  id: number;
  order_id: number;
  refund_id: number;
  amount: number;
  tax_amount: number;
  kind: string;
  reason: string;
}

export interface ShopifyRefundDuty {
  duty_id: number;
  refund_type: string;
}

// =============================================================================
// Draft Order Record
// =============================================================================

export interface ShopifyDraftOrderRecord {
  id: number;
  source_account_id: string;
  shop_id: number | null;
  name: string;
  email: string | null;
  customer_id: number | null;
  order_id: number | null;
  invoice_url: string | null;
  invoice_sent_at: Date | null;
  status: string;
  currency: string;
  subtotal_price: number;
  total_price: number;
  total_tax: number;
  taxes_included: boolean;
  note: string | null;
  tags: string | null;
  discount_codes?: ShopifyDiscountCode[];
  note_attributes?: ShopifyNoteAttribute[];
  line_items: ShopifyDraftOrderLineItem[];
  shipping_address: ShopifyAddress | null;
  billing_address: ShopifyAddress | null;
  shipping_line: ShopifyShippingLine | null;
  applied_discount: ShopifyAppliedDiscount | null;
  tax_lines: ShopifyTaxLine[];
  tax_exempt: boolean;
  payment_terms: ShopifyPaymentTerms | null;
  admin_graphql_api_id: string | null;
  completed_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface ShopifyDraftOrderLineItem {
  id: number;
  variant_id: number | null;
  product_id: number | null;
  title: string;
  variant_title: string | null;
  sku: string | null;
  vendor: string | null;
  quantity: number;
  price: number;
  grams: number;
  requires_shipping: boolean;
  taxable: boolean;
  gift_card: boolean;
  fulfillment_service: string;
  tax_lines: ShopifyTaxLine[];
  applied_discount: ShopifyAppliedDiscount | null;
  custom: boolean;
}

export interface ShopifyAppliedDiscount {
  title: string | null;
  description: string | null;
  value: string;
  value_type: string;
  amount: string;
}

export interface ShopifyPaymentTerms {
  payment_terms_name: string;
  payment_terms_type: string;
  due_in_days: number | null;
  payment_schedules: ShopifyPaymentSchedule[];
}

export interface ShopifyPaymentSchedule {
  amount: number;
  currency: string;
  issued_at: string;
  due_at: string;
  completed_at: string | null;
  expected_payment_method: string;
}

// =============================================================================
// Price Rule Record
// =============================================================================

export interface ShopifyPriceRuleRecord {
  id: number;
  source_account_id: string;
  shop_id: number | null;
  title: string;
  value_type: string;
  value: number;
  customer_selection: string;
  target_type: string;
  target_selection: string;
  allocation_method: string;
  allocation_limit: number | null;
  once_per_customer: boolean;
  usage_limit: number | null;
  starts_at: Date;
  ends_at: Date | null;
  entitled_product_ids: number[];
  entitled_variant_ids: number[];
  entitled_collection_ids: number[];
  entitled_country_ids: number[];
  prerequisite_product_ids: number[];
  prerequisite_variant_ids: number[];
  prerequisite_collection_ids: number[];
  prerequisite_customer_ids: number[];
  prerequisite_saved_search_ids: number[];
  prerequisite_quantity_range: ShopifyPrerequisiteRange | null;
  prerequisite_shipping_price_range: ShopifyPrerequisiteRange | null;
  prerequisite_subtotal_range: ShopifyPrerequisiteRange | null;
  prerequisite_to_entitlement_quantity_ratio: ShopifyQuantityRatio | null;
  admin_graphql_api_id: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface ShopifyPrerequisiteRange {
  greater_than_or_equal_to: string | null;
  less_than_or_equal_to: string | null;
}

export interface ShopifyQuantityRatio {
  prerequisite_quantity: number;
  entitled_quantity: number;
}

// =============================================================================
// Discount Code Record
// =============================================================================

export interface ShopifyDiscountCodeRecord {
  id: number;
  source_account_id: string;
  price_rule_id: number;
  code: string;
  usage_count: number;
  admin_graphql_api_id?: string | null;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Gift Card Record
// =============================================================================

export interface ShopifyGiftCardRecord {
  id: number;
  source_account_id: string;
  shop_id: number | null;
  code: string;
  last_characters: string;
  balance: number;
  initial_value: number;
  currency: string;
  disabled_at: Date | null;
  expires_on: Date | null;
  line_item_id: number | null;
  order_id: number | null;
  customer_id: number | null;
  user_id: number | null;
  template_suffix: string | null;
  note: string | null;
  admin_graphql_api_id: string | null;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Inventory Record
// =============================================================================

export interface ShopifyInventoryRecord {
  id: number;
  source_account_id: string;
  inventory_item_id: number;
  location_id: number;
  variant_id: number | null;
  available: number;
  incoming: number;
  committed: number;
  damaged: number;
  on_hand: number;
  quality_control: number;
  reserved: number;
  safety_stock: number;
  updated_at: Date;
}

export interface ShopifyInventoryItemRecord {
  id: number;
  source_account_id: string;
  sku: string | null;
  created_at: Date;
  updated_at: Date;
  requires_shipping: boolean;
  cost: number | null;
  country_code_of_origin: string | null;
  province_code_of_origin: string | null;
  harmonized_system_code: string | null;
  tracked: boolean;
  country_harmonized_system_codes: ShopifyHSCode[];
  admin_graphql_api_id: string | null;
}

export interface ShopifyHSCode {
  harmonized_system_code: string;
  country_code: string;
}

// =============================================================================
// Metafield Record
// =============================================================================

export interface ShopifyMetafieldRecord {
  id: number;
  source_account_id: string;
  shop_id: number | null;
  namespace: string;
  key: string;
  value: string;
  type: string;
  description: string | null;
  owner_id: number;
  owner_resource: string;
  admin_graphql_api_id: string | null;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Checkout / Abandoned Cart Record
// =============================================================================

export interface ShopifyCheckoutRecord {
  id: number;
  source_account_id: string;
  shop_id: number | null;
  token: string;
  cart_token: string | null;
  email: string | null;
  phone: string | null;
  customer_id: number | null;
  customer_locale: string | null;
  gateway: string | null;
  landing_site: string | null;
  referring_site?: string | null;
  buyer_accepts_marketing: boolean;
  currency: string;
  subtotal_price: number;
  total_price: number;
  total_tax: number;
  total_discounts: number;
  total_line_items_price: number;
  taxes_included: boolean;
  total_weight: number;
  completed_at: Date | null;
  closed_at: Date | null;
  abandoned_checkout_url: string | null;
  discount_codes: ShopifyDiscountCode[];
  tax_lines?: ShopifyTaxLine[];
  line_items: ShopifyCheckoutLineItem[];
  shipping_address: ShopifyAddress | null;
  billing_address: ShopifyAddress | null;
  shipping_line: ShopifyShippingLine | null;
  note: string | null;
  note_attributes: ShopifyNoteAttribute[];
  presentment_currency: string | null;
  source_name: string | null;
  source_identifier: string | null;
  source_url: string | null;
  admin_graphql_api_id?: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface ShopifyCheckoutLineItem {
  id: string;
  key: string;
  product_id: number | null;
  variant_id: number | null;
  title: string;
  variant_title: string | null;
  sku: string | null;
  vendor: string | null;
  quantity: number;
  price: number;
  line_price: number;
  grams: number;
  requires_shipping: boolean;
  taxable: boolean;
  gift_card: boolean;
  fulfillment_service: string;
  compare_at_price: number | null;
  properties: ShopifyLineItemProperty[];
  tax_lines: ShopifyTaxLine[];
  discount_allocations: ShopifyDiscountAllocation[];
}

// =============================================================================
// Webhook Event Record
// =============================================================================

export interface ShopifyWebhookEventRecord {
  id: string;
  source_account_id: string;
  topic: string;
  shop_id: number | null;
  shop_domain: string | null;
  data: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  received_at: Date;
}

// =============================================================================
// Sync Types
// =============================================================================

export const ALL_RESOURCES = [
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
] as const;

export type SyncResource = (typeof ALL_RESOURCES)[number];

export interface SyncStats {
  shops: number;
  locations: number;
  products: number;
  variants: number;
  collections: number;
  customers: number;
  orders: number;
  orderItems: number;
  fulfillments: number;
  transactions: number;
  refunds: number;
  draftOrders: number;
  inventory: number;
  inventoryItems?: number;
  priceRules: number;
  discountCodes: number;
  giftCards: number;
  metafields: number;
  checkouts: number;
  lastSyncedAt?: Date | null;
}

export interface SyncOptions {
  incremental?: boolean;
  since?: Date;
  resources?: SyncResource[];
  limit?: number;
}
