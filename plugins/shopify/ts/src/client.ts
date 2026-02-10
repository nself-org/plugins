/**
 * Shopify API Client
 * REST Admin API client with pagination support
 */

import { createLogger, HttpClient, RateLimiter } from '@nself/plugin-utils';
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
  ShopifyImage,
  ShopifyProductOption,
  ShopifyAddress,
  ShopifyDiscountCode,
  ShopifyNoteAttribute,
  ShopifyTaxLine,
  ShopifyShippingLine,
  ShopifyLineItemProperty,
  ShopifyCollectionRule,
} from './types.js';

const logger = createLogger('shopify:client');

export class ShopifyClient {
  private http: HttpClient;
  private rateLimiter: RateLimiter;

  constructor(store: string, accessToken: string, apiVersion = '2024-01') {

    const baseUrl = `https://${store}.myshopify.com/admin/api/${apiVersion}`;

    this.http = new HttpClient({
      baseUrl,
      headers: {
        'X-Shopify-Access-Token': accessToken,
        'Content-Type': 'application/json',
      },
      timeout: 30000,
    });

    // Shopify rate limit: 2 requests per second for REST
    this.rateLimiter = new RateLimiter(2);

    logger.info('Shopify client initialized', { store, apiVersion });
  }

  private async request<T>(method: 'GET' | 'POST' | 'PUT' | 'DELETE', endpoint: string, data?: unknown): Promise<T> {
    await this.rateLimiter.acquire();

    switch (method) {
      case 'GET':
        return this.http.get<T>(endpoint);
      case 'POST':
        return this.http.post<T>(endpoint, data);
      case 'PUT':
        return this.http.put<T>(endpoint, data);
      case 'DELETE':
        return this.http.delete<T>(endpoint);
    }
  }

  // =========================================================================
  // Shop
  // =========================================================================

  async getShop(): Promise<ShopifyShopRecord> {
    logger.info('Fetching shop info');
    const response = await this.request<{ shop: Record<string, unknown> }>('GET', '/shop.json');
    return this.mapShop(response.shop);
  }

  private mapShop(shop: Record<string, unknown>): ShopifyShopRecord {
    return {
      id: shop.id as number,
      source_account_id: 'primary',
      name: shop.name as string,
      email: shop.email as string | null,
      domain: shop.domain as string | null,
      myshopify_domain: shop.myshopify_domain as string,
      shop_owner: shop.shop_owner as string | null,
      phone: shop.phone as string | null,
      address1: shop.address1 as string | null,
      address2: shop.address2 as string | null,
      city: shop.city as string | null,
      province: shop.province as string | null,
      province_code: shop.province_code as string | null,
      country: shop.country as string | null,
      country_code: shop.country_code as string | null,
      zip: shop.zip as string | null,
      currency: shop.currency as string ?? 'USD',
      money_format: shop.money_format as string | null,
      money_with_currency_format: shop.money_with_currency_format as string | null,
      timezone: shop.timezone as string | null,
      iana_timezone: shop.iana_timezone as string | null,
      plan_name: shop.plan_name as string | null,
      plan_display_name: shop.plan_display_name as string | null,
      weight_unit: shop.weight_unit as string ?? 'kg',
      primary_locale: shop.primary_locale as string ?? 'en',
      enabled_presentment_currencies: (shop.enabled_presentment_currencies as string[]) ?? [],
      has_storefront: shop.has_storefront as boolean ?? false,
      has_discounts: shop.has_discounts as boolean ?? false,
      has_gift_cards: shop.has_gift_cards as boolean ?? false,
      eligible_for_payments: shop.eligible_for_payments as boolean ?? false,
      multi_location_enabled: shop.multi_location_enabled as boolean ?? false,
      setup_required: shop.setup_required as boolean ?? false,
      pre_launch_enabled: shop.pre_launch_enabled as boolean ?? false,
      checkout_api_supported: shop.checkout_api_supported as boolean ?? false,
      created_at: new Date(shop.created_at as string),
      updated_at: new Date(shop.updated_at as string),
    };
  }

  // =========================================================================
  // Locations
  // =========================================================================

  async listLocations(shopId?: number): Promise<ShopifyLocationRecord[]> {
    logger.info('Listing locations');
    const response = await this.request<{ locations: Array<Record<string, unknown>> }>('GET', '/locations.json');
    return response.locations.map(loc => this.mapLocation(loc, shopId));
  }

  async getLocation(id: number, shopId?: number): Promise<ShopifyLocationRecord | null> {
    try {
      const response = await this.request<{ location: Record<string, unknown> }>('GET', `/locations/${id}.json`);
      return this.mapLocation(response.location, shopId);
    } catch (error) {
      logger.error('Failed to get location', { id, error });
      return null;
    }
  }

  private mapLocation(loc: Record<string, unknown>, shopId?: number): ShopifyLocationRecord {
    return {
      id: loc.id as number,
      source_account_id: 'primary',
      shop_id: shopId ?? null,
      name: loc.name as string,
      address1: loc.address1 as string | null,
      address2: loc.address2 as string | null,
      city: loc.city as string | null,
      province: loc.province as string | null,
      province_code: loc.province_code as string | null,
      country: loc.country as string | null,
      country_code: loc.country_code as string | null,
      zip: loc.zip as string | null,
      phone: loc.phone as string | null,
      active: loc.active as boolean ?? true,
      legacy: loc.legacy as boolean ?? false,
      localized_country_name: loc.localized_country_name as string | null,
      localized_province_name: loc.localized_province_name as string | null,
      admin_graphql_api_id: loc.admin_graphql_api_id as string | null,
      created_at: loc.created_at ? new Date(loc.created_at as string) : null,
      updated_at: loc.updated_at ? new Date(loc.updated_at as string) : null,
    };
  }

  // =========================================================================
  // Products
  // =========================================================================

  async listAllProducts(shopId?: number): Promise<{ products: ShopifyProductRecord[]; variants: ShopifyVariantRecord[] }> {
    logger.info('Listing all products');
    const products: ShopifyProductRecord[] = [];
    const variants: ShopifyVariantRecord[] = [];
    let pageInfo: string | undefined;

    do {
      const endpoint = pageInfo
        ? `/products.json?page_info=${pageInfo}&limit=250`
        : '/products.json?limit=250';

      const response = await this.request<{ products: Array<Record<string, unknown>> }>('GET', endpoint);

      for (const product of response.products) {
        products.push(this.mapProduct(product, shopId));

        const productVariants = product.variants as Array<Record<string, unknown>> | undefined;
        if (productVariants) {
          variants.push(...productVariants.map(v => this.mapVariant(v, product.id as number)));
        }
      }

      pageInfo = undefined;
      if (response.products.length < 250) break;

      logger.debug('Fetched products batch', { count: response.products.length, total: products.length });
    } while (pageInfo);

    return { products, variants };
  }

  async getProduct(id: number, shopId?: number): Promise<{ product: ShopifyProductRecord; variants: ShopifyVariantRecord[] } | null> {
    try {
      const response = await this.request<{ product: Record<string, unknown> }>('GET', `/products/${id}.json`);
      const product = this.mapProduct(response.product, shopId);
      const productVariants = response.product.variants as Array<Record<string, unknown>> | undefined;
      const variants = productVariants?.map(v => this.mapVariant(v, id)) ?? [];
      return { product, variants };
    } catch (error) {
      logger.error('Failed to get product', { id, error });
      return null;
    }
  }

  private mapProduct(product: Record<string, unknown>, shopId?: number): ShopifyProductRecord {
    const images = product.images as Array<Record<string, unknown>> | undefined;
    const options = product.options as Array<Record<string, unknown>> | undefined;
    const image = product.image as Record<string, unknown> | null;

    return {
      id: product.id as number,
      source_account_id: 'primary',
      shop_id: shopId ?? null,
      title: product.title as string,
      body_html: product.body_html as string | null,
      vendor: product.vendor as string | null,
      product_type: product.product_type as string | null,
      handle: product.handle as string | null,
      status: product.status as string ?? 'active',
      template_suffix: product.template_suffix as string | null,
      published_scope: product.published_scope as string | null,
      tags: product.tags as string | null,
      admin_graphql_api_id: product.admin_graphql_api_id as string | null,
      image_id: image?.id as number | null ?? null,
      image_src: image?.src as string | null ?? null,
      images: this.mapImages(images),
      options: this.mapOptions(options),
      published_at: product.published_at ? new Date(product.published_at as string) : null,
      created_at: new Date(product.created_at as string),
      updated_at: new Date(product.updated_at as string),
    };
  }

  private mapVariant(variant: Record<string, unknown>, productId: number): ShopifyVariantRecord {
    return {
      id: variant.id as number,
      source_account_id: 'primary',
      product_id: productId,
      title: variant.title as string | null,
      price: parseFloat(variant.price as string ?? '0'),
      compare_at_price: variant.compare_at_price ? parseFloat(variant.compare_at_price as string) : null,
      sku: variant.sku as string | null,
      barcode: variant.barcode as string | null,
      position: variant.position as number ?? 1,
      grams: variant.grams as number ?? 0,
      weight: variant.weight as number | null,
      weight_unit: variant.weight_unit as string ?? 'kg',
      inventory_item_id: variant.inventory_item_id as number | null,
      inventory_quantity: variant.inventory_quantity as number ?? 0,
      inventory_policy: variant.inventory_policy as string ?? 'deny',
      inventory_management: variant.inventory_management as string | null,
      fulfillment_service: variant.fulfillment_service as string ?? 'manual',
      requires_shipping: variant.requires_shipping as boolean ?? true,
      taxable: variant.taxable as boolean ?? true,
      option1: variant.option1 as string | null,
      option2: variant.option2 as string | null,
      option3: variant.option3 as string | null,
      image_id: variant.image_id as number | null,
      admin_graphql_api_id: variant.admin_graphql_api_id as string | null,
      created_at: new Date(variant.created_at as string),
      updated_at: new Date(variant.updated_at as string),
    };
  }

  private mapImages(images?: Array<Record<string, unknown>>): ShopifyImage[] {
    if (!images) return [];
    return images.map(img => ({
      id: img.id as number,
      position: img.position as number ?? 0,
      src: img.src as string,
      width: img.width as number ?? 0,
      height: img.height as number ?? 0,
      alt: img.alt as string | null,
    }));
  }

  private mapOptions(options?: Array<Record<string, unknown>>): ShopifyProductOption[] {
    if (!options) return [];
    return options.map(opt => ({
      id: opt.id as number,
      name: opt.name as string,
      position: opt.position as number ?? 0,
      values: (opt.values as string[]) ?? [],
    }));
  }

  // =========================================================================
  // Collections
  // =========================================================================

  async listAllCollections(shopId?: number): Promise<ShopifyCollectionRecord[]> {
    logger.info('Listing all collections');
    const collections: ShopifyCollectionRecord[] = [];

    // Custom collections
    let pageInfo: string | undefined;
    do {
      const endpoint = pageInfo
        ? `/custom_collections.json?page_info=${pageInfo}&limit=250`
        : '/custom_collections.json?limit=250';

      const response = await this.request<{ custom_collections: Array<Record<string, unknown>> }>('GET', endpoint);
      collections.push(...response.custom_collections.map(c => this.mapCollection(c, shopId, 'custom')));

      if (response.custom_collections.length < 250) break;
      pageInfo = undefined;
    } while (pageInfo);

    // Smart collections
    pageInfo = undefined;
    do {
      const endpoint = pageInfo
        ? `/smart_collections.json?page_info=${pageInfo}&limit=250`
        : '/smart_collections.json?limit=250';

      const response = await this.request<{ smart_collections: Array<Record<string, unknown>> }>('GET', endpoint);
      collections.push(...response.smart_collections.map(c => this.mapCollection(c, shopId, 'smart')));

      if (response.smart_collections.length < 250) break;
      pageInfo = undefined;
    } while (pageInfo);

    return collections;
  }

  private mapCollection(collection: Record<string, unknown>, shopId?: number, type?: string): ShopifyCollectionRecord {
    const image = collection.image as Record<string, unknown> | null;
    const rules = collection.rules as Array<Record<string, unknown>> | undefined;

    return {
      id: collection.id as number,
      source_account_id: 'primary',
      shop_id: shopId ?? null,
      title: collection.title as string,
      body_html: collection.body_html as string | null,
      handle: collection.handle as string | null,
      collection_type: type ?? null,
      sort_order: collection.sort_order as string | null,
      template_suffix: collection.template_suffix as string | null,
      products_count: collection.products_count as number ?? 0,
      disjunctive: collection.disjunctive as boolean ?? false,
      rules: this.mapCollectionRules(rules),
      image: image ? {
        id: image.id as number,
        position: 0,
        src: image.src as string,
        width: image.width as number ?? 0,
        height: image.height as number ?? 0,
        alt: image.alt as string | null,
      } : null,
      published_at: collection.published_at ? new Date(collection.published_at as string) : null,
      published_scope: collection.published_scope as string ?? 'web',
      admin_graphql_api_id: collection.admin_graphql_api_id as string | null,
      updated_at: new Date(collection.updated_at as string),
    };
  }

  private mapCollectionRules(rules?: Array<Record<string, unknown>>): ShopifyCollectionRule[] {
    if (!rules) return [];
    return rules.map(r => ({
      column: r.column as string,
      relation: r.relation as string,
      condition: r.condition as string,
    }));
  }

  // =========================================================================
  // Customers
  // =========================================================================

  async listAllCustomers(shopId?: number): Promise<ShopifyCustomerRecord[]> {
    logger.info('Listing all customers');
    const customers: ShopifyCustomerRecord[] = [];
    let pageInfo: string | undefined;

    do {
      const endpoint = pageInfo
        ? `/customers.json?page_info=${pageInfo}&limit=250`
        : '/customers.json?limit=250';

      const response = await this.request<{ customers: Array<Record<string, unknown>> }>('GET', endpoint);
      customers.push(...response.customers.map(c => this.mapCustomer(c, shopId)));

      if (response.customers.length < 250) break;
      pageInfo = undefined;

      logger.debug('Fetched customers batch', { count: response.customers.length, total: customers.length });
    } while (pageInfo);

    return customers;
  }

  async getCustomer(id: number, shopId?: number): Promise<ShopifyCustomerRecord | null> {
    try {
      const response = await this.request<{ customer: Record<string, unknown> }>('GET', `/customers/${id}.json`);
      return this.mapCustomer(response.customer, shopId);
    } catch (error) {
      logger.error('Failed to get customer', { id, error });
      return null;
    }
  }

  private mapCustomer(customer: Record<string, unknown>, shopId?: number): ShopifyCustomerRecord {
    const addresses = customer.addresses as Array<Record<string, unknown>> | undefined;
    const defaultAddress = customer.default_address as Record<string, unknown> | null;
    const taxExemptions = customer.tax_exemptions as string[] | undefined;
    const smsConsent = customer.sms_marketing_consent as Record<string, unknown> | null;

    return {
      id: customer.id as number,
      source_account_id: 'primary',
      shop_id: shopId ?? null,
      email: customer.email as string | null,
      first_name: customer.first_name as string | null,
      last_name: customer.last_name as string | null,
      phone: customer.phone as string | null,
      verified_email: customer.verified_email as boolean ?? false,
      accepts_marketing: customer.accepts_marketing as boolean ?? false,
      accepts_marketing_updated_at: customer.accepts_marketing_updated_at
        ? new Date(customer.accepts_marketing_updated_at as string)
        : null,
      marketing_opt_in_level: customer.marketing_opt_in_level as string | null,
      sms_marketing_consent: smsConsent ? {
        state: (smsConsent.state as string) ?? 'not_subscribed',
        opt_in_level: (smsConsent.opt_in_level as string) ?? null,
        consent_updated_at: (smsConsent.consent_updated_at as string) ?? null,
        consent_collected_from: (smsConsent.consent_collected_from as string) ?? null,
      } : null,
      orders_count: customer.orders_count as number ?? 0,
      total_spent: parseFloat(customer.total_spent as string ?? '0'),
      state: customer.state as string ?? 'disabled',
      note: customer.note as string | null,
      tags: customer.tags as string | null,
      currency: customer.currency as string | null,
      tax_exempt: customer.tax_exempt as boolean ?? false,
      tax_exemptions: taxExemptions ?? [],
      default_address: defaultAddress ? this.mapAddress(defaultAddress) : null,
      addresses: this.mapAddresses(addresses),
      admin_graphql_api_id: customer.admin_graphql_api_id as string | null,
      created_at: new Date(customer.created_at as string),
      updated_at: new Date(customer.updated_at as string),
    };
  }

  private mapAddress(address: Record<string, unknown>): ShopifyAddress {
    return {
      id: address.id as number,
      customer_id: address.customer_id as number,
      first_name: address.first_name as string | null,
      last_name: address.last_name as string | null,
      company: address.company as string | null,
      address1: address.address1 as string | null,
      address2: address.address2 as string | null,
      city: address.city as string | null,
      province: address.province as string | null,
      province_code: address.province_code as string | null,
      country: address.country as string | null,
      country_code: address.country_code as string | null,
      zip: address.zip as string | null,
      phone: address.phone as string | null,
      name: address.name as string | null,
      default: address.default as boolean ?? false,
    };
  }

  private mapAddresses(addresses?: Array<Record<string, unknown>>): ShopifyAddress[] {
    if (!addresses) return [];
    return addresses.map(a => this.mapAddress(a));
  }

  // =========================================================================
  // Orders
  // =========================================================================

  async listAllOrders(shopId?: number, status = 'any'): Promise<{ orders: ShopifyOrderRecord[]; lineItems: ShopifyOrderItemRecord[] }> {
    logger.info('Listing all orders', { status });
    const orders: ShopifyOrderRecord[] = [];
    const lineItems: ShopifyOrderItemRecord[] = [];
    let pageInfo: string | undefined;

    do {
      const endpoint = pageInfo
        ? `/orders.json?page_info=${pageInfo}&limit=250`
        : `/orders.json?status=${status}&limit=250`;

      const response = await this.request<{ orders: Array<Record<string, unknown>> }>('GET', endpoint);

      for (const order of response.orders) {
        orders.push(this.mapOrder(order, shopId));

        const items = order.line_items as Array<Record<string, unknown>> | undefined;
        if (items) {
          lineItems.push(...items.map(item => this.mapLineItem(item, order.id as number)));
        }
      }

      if (response.orders.length < 250) break;
      pageInfo = undefined;

      logger.debug('Fetched orders batch', { count: response.orders.length, total: orders.length });
    } while (pageInfo);

    return { orders, lineItems };
  }

  async getOrder(id: number, shopId?: number): Promise<{ order: ShopifyOrderRecord; lineItems: ShopifyOrderItemRecord[] } | null> {
    try {
      const response = await this.request<{ order: Record<string, unknown> }>('GET', `/orders/${id}.json`);
      const order = this.mapOrder(response.order, shopId);
      const items = response.order.line_items as Array<Record<string, unknown>> | undefined;
      const lineItems = items?.map(item => this.mapLineItem(item, id)) ?? [];
      return { order, lineItems };
    } catch (error) {
      logger.error('Failed to get order', { id, error });
      return null;
    }
  }

  private mapOrder(order: Record<string, unknown>, shopId?: number): ShopifyOrderRecord {
    const discountCodes = order.discount_codes as Array<Record<string, unknown>> | undefined;
    const noteAttributes = order.note_attributes as Array<Record<string, unknown>> | undefined;
    const taxLines = order.tax_lines as Array<Record<string, unknown>> | undefined;
    const shippingLines = order.shipping_lines as Array<Record<string, unknown>> | undefined;
    const billingAddress = order.billing_address as Record<string, unknown> | null;
    const shippingAddress = order.shipping_address as Record<string, unknown> | null;
    const paymentGatewayNames = order.payment_gateway_names as string[] | undefined;

    return {
      id: order.id as number,
      source_account_id: 'primary',
      shop_id: shopId ?? null,
      order_number: order.order_number as number,
      name: order.name as string,
      email: order.email as string | null,
      phone: order.phone as string | null,
      customer_id: (order.customer as Record<string, unknown> | null)?.id as number | null ?? null,
      financial_status: order.financial_status as string | null,
      fulfillment_status: order.fulfillment_status as string | null,
      cancel_reason: order.cancel_reason as string | null,
      cancelled_at: order.cancelled_at ? new Date(order.cancelled_at as string) : null,
      closed_at: order.closed_at ? new Date(order.closed_at as string) : null,
      confirmed: order.confirmed as boolean ?? true,
      contact_email: order.contact_email as string | null,
      currency: order.currency as string ?? 'USD',
      current_subtotal_price: order.current_subtotal_price ? parseFloat(order.current_subtotal_price as string) : null,
      current_total_discounts: order.current_total_discounts ? parseFloat(order.current_total_discounts as string) : null,
      current_total_price: order.current_total_price ? parseFloat(order.current_total_price as string) : null,
      current_total_tax: order.current_total_tax ? parseFloat(order.current_total_tax as string) : null,
      subtotal_price: order.subtotal_price ? parseFloat(order.subtotal_price as string) : null,
      total_discounts: order.total_discounts ? parseFloat(order.total_discounts as string) : null,
      total_line_items_price: order.total_line_items_price ? parseFloat(order.total_line_items_price as string) : null,
      total_price: order.total_price ? parseFloat(order.total_price as string) : null,
      total_tax: order.total_tax ? parseFloat(order.total_tax as string) : null,
      total_weight: order.total_weight as number ?? 0,
      total_tip_received: order.total_tip_received ? parseFloat(order.total_tip_received as string) : 0,
      discount_codes: this.mapDiscountCodes(discountCodes),
      note: order.note as string | null,
      note_attributes: this.mapNoteAttributes(noteAttributes),
      tags: order.tags as string | null,
      tax_lines: this.mapTaxLines(taxLines),
      taxes_included: order.taxes_included as boolean ?? false,
      test: order.test as boolean ?? false,
      token: order.token as string | null,
      gateway: order.gateway as string | null,
      payment_gateway_names: paymentGatewayNames ?? [],
      processing_method: order.processing_method as string | null,
      source_name: order.source_name as string | null,
      source_identifier: order.source_identifier as string | null,
      source_url: order.source_url as string | null,
      landing_site: order.landing_site as string | null,
      referring_site: order.referring_site as string | null,
      billing_address: billingAddress ? this.mapAddress(billingAddress) : null,
      shipping_address: shippingAddress ? this.mapAddress(shippingAddress) : null,
      shipping_lines: this.mapShippingLines(shippingLines),
      admin_graphql_api_id: order.admin_graphql_api_id as string | null,
      processed_at: order.processed_at ? new Date(order.processed_at as string) : null,
      created_at: new Date(order.created_at as string),
      updated_at: new Date(order.updated_at as string),
    } as unknown as ShopifyOrderRecord;
  }

  private mapLineItem(item: Record<string, unknown>, orderId: number): ShopifyOrderItemRecord {
    const taxLines = item.tax_lines as Array<Record<string, unknown>> | undefined;
    const properties = item.properties as Array<Record<string, unknown>> | undefined;

    return {
      id: item.id as number,
      source_account_id: 'primary',
      order_id: orderId,
      product_id: item.product_id as number | null,
      variant_id: item.variant_id as number | null,
      title: item.title as string,
      variant_title: item.variant_title as string | null,
      sku: item.sku as string | null,
      vendor: item.vendor as string | null,
      quantity: item.quantity as number ?? 1,
      price: item.price ? parseFloat(item.price as string) : null,
      total_discount: item.total_discount ? parseFloat(item.total_discount as string) : 0,
      fulfillment_status: item.fulfillment_status as string | null,
      fulfillable_quantity: item.fulfillable_quantity as number ?? 0,
      fulfillment_service: item.fulfillment_service as string | null,
      grams: item.grams as number ?? 0,
      requires_shipping: item.requires_shipping as boolean ?? true,
      taxable: item.taxable as boolean ?? true,
      tax_lines: this.mapTaxLines(taxLines),
      properties: this.mapLineItemProperties(properties),
      gift_card: item.gift_card as boolean ?? false,
      admin_graphql_api_id: item.admin_graphql_api_id as string | null,
    } as unknown as ShopifyOrderItemRecord;
  }

  private mapDiscountCodes(codes?: Array<Record<string, unknown>>): ShopifyDiscountCode[] {
    if (!codes) return [];
    return codes.map(c => ({
      code: c.code as string,
      amount: c.amount as string,
      type: c.type as string,
    }));
  }

  private mapNoteAttributes(attrs?: Array<Record<string, unknown>>): ShopifyNoteAttribute[] {
    if (!attrs) return [];
    return attrs.map(a => ({ name: a.name as string, value: a.value as string }));
  }

  private mapTaxLines(lines?: Array<Record<string, unknown>>): ShopifyTaxLine[] {
    if (!lines) return [];
    return lines.map(l => ({ title: l.title as string, price: l.price as string, rate: l.rate as number })) as ShopifyTaxLine[];
  }

  private mapShippingLines(lines?: Array<Record<string, unknown>>): ShopifyShippingLine[] {
    if (!lines) return [];
    return lines.map(l => ({
      id: l.id as number,
      title: l.title as string,
      price: l.price as string,
      code: l.code as string | null,
      source: l.source as string | null,
    })) as ShopifyShippingLine[];
  }

  private mapLineItemProperties(props?: Array<Record<string, unknown>>): ShopifyLineItemProperty[] {
    if (!props) return [];
    return props.map(p => ({ name: p.name as string, value: String(p.value) }));
  }

  // =========================================================================
  // Fulfillments
  // =========================================================================

  async listFulfillments(orderId: number): Promise<ShopifyFulfillmentRecord[]> {
    logger.info('Listing fulfillments', { orderId });
    const response = await this.request<{ fulfillments: Array<Record<string, unknown>> }>('GET', `/orders/${orderId}/fulfillments.json`);
    return response.fulfillments.map(f => this.mapFulfillment(f, orderId));
  }

  async getFulfillment(orderId: number, fulfillmentId: number): Promise<ShopifyFulfillmentRecord | null> {
    try {
      const response = await this.request<{ fulfillment: Record<string, unknown> }>('GET', `/orders/${orderId}/fulfillments/${fulfillmentId}.json`);
      return this.mapFulfillment(response.fulfillment, orderId);
    } catch (error) {
      logger.error('Failed to get fulfillment', { orderId, fulfillmentId, error });
      return null;
    }
  }

  private mapFulfillment(f: Record<string, unknown>, orderId: number): ShopifyFulfillmentRecord {
    return {
      id: f.id as number,
      source_account_id: 'primary',
      order_id: orderId,
      location_id: f.location_id as number | null,
      status: f.status as string | null,
      tracking_company: f.tracking_company as string | null,
      tracking_number: f.tracking_number as string | null,
      tracking_numbers: (f.tracking_numbers as string[]) ?? [],
      tracking_url: f.tracking_url as string | null,
      tracking_urls: (f.tracking_urls as string[]) ?? [],
      shipment_status: f.shipment_status as string | null,
      service: f.service as string | null,
      name: f.name as string | null,
      receipt: f.receipt as Record<string, unknown> | null,
      line_items: (f.line_items as Array<Record<string, unknown>>) ?? [],
      notify_customer: f.notify_customer as boolean ?? false,
      admin_graphql_api_id: f.admin_graphql_api_id as string | null,
      created_at: new Date(f.created_at as string),
      updated_at: new Date(f.updated_at as string),
    } as unknown as ShopifyFulfillmentRecord;
  }

  // =========================================================================
  // Transactions
  // =========================================================================

  async listTransactions(orderId: number): Promise<ShopifyTransactionRecord[]> {
    logger.info('Listing transactions', { orderId });
    const response = await this.request<{ transactions: Array<Record<string, unknown>> }>('GET', `/orders/${orderId}/transactions.json`);
    return response.transactions.map(t => this.mapTransaction(t, orderId));
  }

  async getTransaction(orderId: number, transactionId: number): Promise<ShopifyTransactionRecord | null> {
    try {
      const response = await this.request<{ transaction: Record<string, unknown> }>('GET', `/orders/${orderId}/transactions/${transactionId}.json`);
      return this.mapTransaction(response.transaction, orderId);
    } catch (error) {
      logger.error('Failed to get transaction', { orderId, transactionId, error });
      return null;
    }
  }

  private mapTransaction(t: Record<string, unknown>, orderId: number): ShopifyTransactionRecord {
    return {
      id: t.id as number,
      source_account_id: 'primary',
      order_id: orderId,
      parent_id: t.parent_id as number | null,
      kind: t.kind as string,
      gateway: t.gateway as string | null,
      status: t.status as string | null,
      message: t.message as string | null,
      amount: t.amount ? parseFloat(t.amount as string) : null,
      currency: t.currency as string | null,
      authorization: t.authorization as string | null,
      source_name: t.source_name as string | null,
      payment_details: t.payment_details as Record<string, unknown> | null,
      error_code: t.error_code as string | null,
      receipt: t.receipt as Record<string, unknown> | null,
      test: t.test as boolean ?? false,
      admin_graphql_api_id: t.admin_graphql_api_id as string | null,
      processed_at: t.processed_at ? new Date(t.processed_at as string) : null,
      created_at: new Date(t.created_at as string),
    } as unknown as ShopifyTransactionRecord;
  }

  // =========================================================================
  // Refunds
  // =========================================================================

  async listRefunds(orderId: number): Promise<ShopifyRefundRecord[]> {
    logger.info('Listing refunds', { orderId });
    const response = await this.request<{ refunds: Array<Record<string, unknown>> }>('GET', `/orders/${orderId}/refunds.json`);
    return response.refunds.map(r => this.mapRefund(r, orderId));
  }

  async getRefund(orderId: number, refundId: number): Promise<ShopifyRefundRecord | null> {
    try {
      const response = await this.request<{ refund: Record<string, unknown> }>('GET', `/orders/${orderId}/refunds/${refundId}.json`);
      return this.mapRefund(response.refund, orderId);
    } catch (error) {
      logger.error('Failed to get refund', { orderId, refundId, error });
      return null;
    }
  }

  private mapRefund(r: Record<string, unknown>, orderId: number): ShopifyRefundRecord {
    return {
      id: r.id as number,
      source_account_id: 'primary',
      order_id: orderId,
      note: r.note as string | null,
      restock: r.restock as boolean ?? false,
      user_id: r.user_id as number | null,
      refund_line_items: (r.refund_line_items as Array<Record<string, unknown>>) ?? [],
      transactions: (r.transactions as Array<Record<string, unknown>>) ?? [],
      order_adjustments: (r.order_adjustments as Array<Record<string, unknown>>) ?? [],
      duties: (r.duties as Array<Record<string, unknown>>) ?? [],
      admin_graphql_api_id: r.admin_graphql_api_id as string | null,
      processed_at: r.processed_at ? new Date(r.processed_at as string) : null,
      created_at: new Date(r.created_at as string),
    } as unknown as ShopifyRefundRecord;
  }

  // =========================================================================
  // Draft Orders
  // =========================================================================

  async listAllDraftOrders(shopId?: number): Promise<ShopifyDraftOrderRecord[]> {
    logger.info('Listing all draft orders');
    const draftOrders: ShopifyDraftOrderRecord[] = [];
    let pageInfo: string | undefined;

    do {
      const endpoint = pageInfo
        ? `/draft_orders.json?page_info=${pageInfo}&limit=250`
        : '/draft_orders.json?limit=250';

      const response = await this.request<{ draft_orders: Array<Record<string, unknown>> }>('GET', endpoint);
      draftOrders.push(...response.draft_orders.map(d => this.mapDraftOrder(d, shopId)));

      if (response.draft_orders.length < 250) break;
      pageInfo = undefined;
    } while (pageInfo);

    return draftOrders;
  }

  async getDraftOrder(id: number, shopId?: number): Promise<ShopifyDraftOrderRecord | null> {
    try {
      const response = await this.request<{ draft_order: Record<string, unknown> }>('GET', `/draft_orders/${id}.json`);
      return this.mapDraftOrder(response.draft_order, shopId);
    } catch (error) {
      logger.error('Failed to get draft order', { id, error });
      return null;
    }
  }

  private mapDraftOrder(d: Record<string, unknown>, shopId?: number): ShopifyDraftOrderRecord {
    const customer = d.customer as Record<string, unknown> | null;
    return {
      id: d.id as number,
      source_account_id: 'primary',
      shop_id: shopId ?? null,
      order_id: d.order_id as number | null,
      name: d.name as string | null,
      email: d.email as string | null,
      customer_id: customer?.id as number | null ?? null,
      status: d.status as string ?? 'open',
      currency: d.currency as string ?? 'USD',
      subtotal_price: d.subtotal_price ? parseFloat(d.subtotal_price as string) : null,
      total_price: d.total_price ? parseFloat(d.total_price as string) : null,
      total_tax: d.total_tax ? parseFloat(d.total_tax as string) : null,
      taxes_included: d.taxes_included as boolean ?? false,
      tax_exempt: d.tax_exempt as boolean ?? false,
      tax_lines: (d.tax_lines as Array<Record<string, unknown>>) ?? [],
      discount_codes: [],
      applied_discount: d.applied_discount as Record<string, unknown> | null,
      line_items: (d.line_items as Array<Record<string, unknown>>) ?? [],
      shipping_line: d.shipping_line as Record<string, unknown> | null,
      billing_address: d.billing_address as Record<string, unknown> | null,
      shipping_address: d.shipping_address as Record<string, unknown> | null,
      note: d.note as string | null,
      note_attributes: (d.note_attributes as Array<Record<string, unknown>>) ?? [],
      tags: d.tags as string | null,
      invoice_url: d.invoice_url as string | null,
      invoice_sent_at: d.invoice_sent_at ? new Date(d.invoice_sent_at as string) : null,
      admin_graphql_api_id: d.admin_graphql_api_id as string | null,
      completed_at: d.completed_at ? new Date(d.completed_at as string) : null,
      created_at: new Date(d.created_at as string),
      updated_at: new Date(d.updated_at as string),
    } as unknown as ShopifyDraftOrderRecord;
  }

  // =========================================================================
  // Inventory Items
  // =========================================================================

  async getInventoryItem(id: number): Promise<ShopifyInventoryItemRecord | null> {
    try {
      const response = await this.request<{ inventory_item: Record<string, unknown> }>('GET', `/inventory_items/${id}.json`);
      return this.mapInventoryItem(response.inventory_item);
    } catch (error) {
      logger.error('Failed to get inventory item', { id, error });
      return null;
    }
  }

  async listInventoryItems(ids: number[]): Promise<ShopifyInventoryItemRecord[]> {
    logger.info('Listing inventory items', { count: ids.length });
    const items: ShopifyInventoryItemRecord[] = [];

    // Batch in groups of 100
    for (let i = 0; i < ids.length; i += 100) {
      const batch = ids.slice(i, i + 100);
      const response = await this.request<{ inventory_items: Array<Record<string, unknown>> }>('GET', `/inventory_items.json?ids=${batch.join(',')}`);
      items.push(...response.inventory_items.map(item => this.mapInventoryItem(item)));
    }

    return items;
  }

  private mapInventoryItem(item: Record<string, unknown>): ShopifyInventoryItemRecord {
    return {
      id: item.id as number,
      source_account_id: 'primary',
      sku: item.sku as string | null,
      cost: item.cost ? parseFloat(item.cost as string) : null,
      country_code_of_origin: item.country_code_of_origin as string | null,
      country_harmonized_system_codes: (item.country_harmonized_system_codes as Array<Record<string, unknown>>) ?? [],
      harmonized_system_code: item.harmonized_system_code as string | null,
      province_code_of_origin: item.province_code_of_origin as string | null,
      tracked: item.tracked as boolean ?? true,
      requires_shipping: item.requires_shipping as boolean ?? true,
      admin_graphql_api_id: item.admin_graphql_api_id as string | null,
      created_at: item.created_at ? new Date(item.created_at as string) : null,
      updated_at: item.updated_at ? new Date(item.updated_at as string) : null,
    } as unknown as ShopifyInventoryItemRecord;
  }

  // =========================================================================
  // Inventory Levels
  // =========================================================================

  async listInventoryLevels(locationIds?: number[]): Promise<ShopifyInventoryRecord[]> {
    logger.info('Listing inventory levels');
    const inventory: ShopifyInventoryRecord[] = [];

    if (!locationIds) {
      const locResponse = await this.request<{ locations: Array<{ id: number }> }>('GET', '/locations.json');
      locationIds = locResponse.locations.map(l => l.id);
    }

    for (const locationId of locationIds) {
      let pageInfo: string | undefined;

      do {
        const endpoint = pageInfo
          ? `/inventory_levels.json?page_info=${pageInfo}&limit=250`
          : `/inventory_levels.json?location_ids=${locationId}&limit=250`;

        const response = await this.request<{ inventory_levels: Array<Record<string, unknown>> }>('GET', endpoint);
        inventory.push(...response.inventory_levels.map(inv => this.mapInventoryLevel(inv)));

        if (response.inventory_levels.length < 250) break;
        pageInfo = undefined;
      } while (pageInfo);
    }

    return inventory;
  }

  private mapInventoryLevel(inv: Record<string, unknown>): ShopifyInventoryRecord {
    return {
      id: 0,
      source_account_id: 'primary',
      inventory_item_id: inv.inventory_item_id as number,
      location_id: inv.location_id as number,
      variant_id: null,
      available: inv.available as number ?? 0,
      incoming: 0,
      committed: 0,
      damaged: 0,
      on_hand: inv.available as number ?? 0,
      quality_control: 0,
      reserved: 0,
      safety_stock: 0,
      updated_at: new Date(inv.updated_at as string),
    };
  }

  // =========================================================================
  // Price Rules
  // =========================================================================

  async listAllPriceRules(shopId?: number): Promise<ShopifyPriceRuleRecord[]> {
    logger.info('Listing all price rules');
    const priceRules: ShopifyPriceRuleRecord[] = [];
    let pageInfo: string | undefined;

    do {
      const endpoint = pageInfo
        ? `/price_rules.json?page_info=${pageInfo}&limit=250`
        : '/price_rules.json?limit=250';

      const response = await this.request<{ price_rules: Array<Record<string, unknown>> }>('GET', endpoint);
      priceRules.push(...response.price_rules.map(pr => this.mapPriceRule(pr, shopId)));

      if (response.price_rules.length < 250) break;
      pageInfo = undefined;
    } while (pageInfo);

    return priceRules;
  }

  async getPriceRule(id: number, shopId?: number): Promise<ShopifyPriceRuleRecord | null> {
    try {
      const response = await this.request<{ price_rule: Record<string, unknown> }>('GET', `/price_rules/${id}.json`);
      return this.mapPriceRule(response.price_rule, shopId);
    } catch (error) {
      logger.error('Failed to get price rule', { id, error });
      return null;
    }
  }

  private mapPriceRule(pr: Record<string, unknown>, shopId?: number): ShopifyPriceRuleRecord {
    return {
      id: pr.id as number,
      source_account_id: 'primary',
      shop_id: shopId ?? null,
      title: pr.title as string,
      target_type: pr.target_type as string | null,
      target_selection: pr.target_selection as string | null,
      allocation_method: pr.allocation_method as string | null,
      value_type: pr.value_type as string | null,
      value: pr.value ? parseFloat(pr.value as string) : null,
      once_per_customer: pr.once_per_customer as boolean ?? false,
      usage_limit: pr.usage_limit as number | null,
      customer_selection: pr.customer_selection as string | null,
      prerequisite_subtotal_range: pr.prerequisite_subtotal_range as Record<string, unknown> | null,
      prerequisite_quantity_range: pr.prerequisite_quantity_range as Record<string, unknown> | null,
      prerequisite_shipping_price_range: pr.prerequisite_shipping_price_range as Record<string, unknown> | null,
      prerequisite_customer_ids: (pr.prerequisite_customer_ids as number[]) ?? [],
      prerequisite_product_ids: (pr.prerequisite_product_ids as number[]) ?? [],
      prerequisite_variant_ids: (pr.prerequisite_variant_ids as number[]) ?? [],
      prerequisite_collection_ids: (pr.prerequisite_collection_ids as number[]) ?? [],
      entitled_product_ids: (pr.entitled_product_ids as number[]) ?? [],
      entitled_variant_ids: (pr.entitled_variant_ids as number[]) ?? [],
      entitled_collection_ids: (pr.entitled_collection_ids as number[]) ?? [],
      entitled_country_ids: (pr.entitled_country_ids as number[]) ?? [],
      allocation_limit: pr.allocation_limit as number | null,
      admin_graphql_api_id: pr.admin_graphql_api_id as string | null,
      starts_at: pr.starts_at ? new Date(pr.starts_at as string) : null,
      ends_at: pr.ends_at ? new Date(pr.ends_at as string) : null,
      created_at: new Date(pr.created_at as string),
      updated_at: new Date(pr.updated_at as string),
    } as unknown as ShopifyPriceRuleRecord;
  }

  // =========================================================================
  // Discount Codes
  // =========================================================================

  async listDiscountCodes(priceRuleId: number): Promise<ShopifyDiscountCodeRecord[]> {
    logger.info('Listing discount codes', { priceRuleId });
    const response = await this.request<{ discount_codes: Array<Record<string, unknown>> }>('GET', `/price_rules/${priceRuleId}/discount_codes.json`);
    return response.discount_codes.map(dc => this.mapDiscountCodeRecord(dc, priceRuleId));
  }

  async getDiscountCode(priceRuleId: number, discountCodeId: number): Promise<ShopifyDiscountCodeRecord | null> {
    try {
      const response = await this.request<{ discount_code: Record<string, unknown> }>('GET', `/price_rules/${priceRuleId}/discount_codes/${discountCodeId}.json`);
      return this.mapDiscountCodeRecord(response.discount_code, priceRuleId);
    } catch (error) {
      logger.error('Failed to get discount code', { priceRuleId, discountCodeId, error });
      return null;
    }
  }

  private mapDiscountCodeRecord(dc: Record<string, unknown>, priceRuleId: number): ShopifyDiscountCodeRecord {
    return {
      id: dc.id as number,
      source_account_id: 'primary',
      price_rule_id: priceRuleId,
      code: dc.code as string,
      usage_count: dc.usage_count as number ?? 0,
      admin_graphql_api_id: dc.admin_graphql_api_id as string | null,
      created_at: new Date(dc.created_at as string),
      updated_at: new Date(dc.updated_at as string),
    };
  }

  // =========================================================================
  // Gift Cards
  // =========================================================================

  async listAllGiftCards(shopId?: number): Promise<ShopifyGiftCardRecord[]> {
    logger.info('Listing all gift cards');
    const giftCards: ShopifyGiftCardRecord[] = [];
    let pageInfo: string | undefined;

    do {
      const endpoint = pageInfo
        ? `/gift_cards.json?page_info=${pageInfo}&limit=250`
        : '/gift_cards.json?limit=250';

      const response = await this.request<{ gift_cards: Array<Record<string, unknown>> }>('GET', endpoint);
      giftCards.push(...response.gift_cards.map(gc => this.mapGiftCard(gc, shopId)));

      if (response.gift_cards.length < 250) break;
      pageInfo = undefined;
    } while (pageInfo);

    return giftCards;
  }

  async getGiftCard(id: number, shopId?: number): Promise<ShopifyGiftCardRecord | null> {
    try {
      const response = await this.request<{ gift_card: Record<string, unknown> }>('GET', `/gift_cards/${id}.json`);
      return this.mapGiftCard(response.gift_card, shopId);
    } catch (error) {
      logger.error('Failed to get gift card', { id, error });
      return null;
    }
  }

  private mapGiftCard(gc: Record<string, unknown>, shopId?: number): ShopifyGiftCardRecord {
    return {
      id: gc.id as number,
      source_account_id: 'primary',
      shop_id: shopId ?? null,
      balance: gc.balance ? parseFloat(gc.balance as string) : null,
      initial_value: gc.initial_value ? parseFloat(gc.initial_value as string) : null,
      currency: gc.currency as string ?? 'USD',
      code: gc.code as string | null,
      last_characters: gc.last_characters as string | null,
      note: gc.note as string | null,
      template_suffix: gc.template_suffix as string | null,
      customer_id: gc.customer_id as number | null,
      order_id: gc.order_id as number | null,
      line_item_id: gc.line_item_id as number | null,
      user_id: gc.user_id as number | null,
      disabled_at: gc.disabled_at ? new Date(gc.disabled_at as string) : null,
      expires_on: gc.expires_on as string | null,
      admin_graphql_api_id: gc.admin_graphql_api_id as string | null,
      created_at: new Date(gc.created_at as string),
      updated_at: new Date(gc.updated_at as string),
    } as unknown as ShopifyGiftCardRecord;
  }

  // =========================================================================
  // Metafields
  // =========================================================================

  async listMetafields(ownerResource: string, ownerId: number): Promise<ShopifyMetafieldRecord[]> {
    logger.info('Listing metafields', { ownerResource, ownerId });
    const endpoint = `/${ownerResource}/${ownerId}/metafields.json`;
    const response = await this.request<{ metafields: Array<Record<string, unknown>> }>('GET', endpoint);
    return response.metafields.map(mf => this.mapMetafield(mf, ownerResource, ownerId));
  }

  async getMetafield(ownerResource: string, ownerId: number, metafieldId: number): Promise<ShopifyMetafieldRecord | null> {
    try {
      const endpoint = `/${ownerResource}/${ownerId}/metafields/${metafieldId}.json`;
      const response = await this.request<{ metafield: Record<string, unknown> }>('GET', endpoint);
      return this.mapMetafield(response.metafield, ownerResource, ownerId);
    } catch (error) {
      logger.error('Failed to get metafield', { ownerResource, ownerId, metafieldId, error });
      return null;
    }
  }

  private mapMetafield(mf: Record<string, unknown>, ownerResource: string, ownerId: number): ShopifyMetafieldRecord {
    return {
      id: mf.id as number,
      source_account_id: 'primary',
      namespace: mf.namespace as string,
      key: mf.key as string,
      value: mf.value as string | null,
      type: mf.type as string | null,
      description: mf.description as string | null,
      owner_id: ownerId,
      owner_resource: ownerResource,
      admin_graphql_api_id: mf.admin_graphql_api_id as string | null,
      created_at: new Date(mf.created_at as string),
      updated_at: new Date(mf.updated_at as string),
    } as unknown as ShopifyMetafieldRecord;
  }

  // =========================================================================
  // Checkouts (Abandoned)
  // =========================================================================

  async listAllCheckouts(shopId?: number): Promise<ShopifyCheckoutRecord[]> {
    logger.info('Listing all abandoned checkouts');
    const checkouts: ShopifyCheckoutRecord[] = [];
    let pageInfo: string | undefined;

    do {
      const endpoint = pageInfo
        ? `/checkouts.json?page_info=${pageInfo}&limit=250`
        : '/checkouts.json?limit=250';

      const response = await this.request<{ checkouts: Array<Record<string, unknown>> }>('GET', endpoint);
      checkouts.push(...response.checkouts.map(ch => this.mapCheckout(ch, shopId)));

      if (response.checkouts.length < 250) break;
      pageInfo = undefined;
    } while (pageInfo);

    return checkouts;
  }

  async getCheckout(token: string, shopId?: number): Promise<ShopifyCheckoutRecord | null> {
    try {
      const response = await this.request<{ checkout: Record<string, unknown> }>('GET', `/checkouts/${token}.json`);
      return this.mapCheckout(response.checkout, shopId);
    } catch (error) {
      logger.error('Failed to get checkout', { token, error });
      return null;
    }
  }

  private mapCheckout(ch: Record<string, unknown>, shopId?: number): ShopifyCheckoutRecord {
    const customer = ch.customer as Record<string, unknown> | null;
    return {
      id: ch.id as number,
      source_account_id: 'primary',
      token: ch.token as string,
      cart_token: ch.cart_token as string | null,
      shop_id: shopId ?? null,
      email: ch.email as string | null,
      customer_id: customer?.id as number | null ?? null,
      customer_locale: ch.customer_locale as string | null,
      gateway: ch.gateway as string | null,
      currency: ch.currency as string ?? 'USD',
      subtotal_price: ch.subtotal_price ? parseFloat(ch.subtotal_price as string) : null,
      total_price: ch.total_price ? parseFloat(ch.total_price as string) : null,
      total_tax: ch.total_tax ? parseFloat(ch.total_tax as string) : null,
      total_discounts: ch.total_discounts ? parseFloat(ch.total_discounts as string) : null,
      total_line_items_price: ch.total_line_items_price ? parseFloat(ch.total_line_items_price as string) : null,
      taxes_included: ch.taxes_included as boolean ?? false,
      discount_codes: (ch.discount_codes as Array<Record<string, unknown>>) ?? [],
      line_items: (ch.line_items as Array<Record<string, unknown>>) ?? [],
      tax_lines: (ch.tax_lines as Array<Record<string, unknown>>) ?? [],
      shipping_line: ch.shipping_line as Record<string, unknown> | null,
      shipping_address: ch.shipping_address as Record<string, unknown> | null,
      billing_address: ch.billing_address as Record<string, unknown> | null,
      note: ch.note as string | null,
      note_attributes: (ch.note_attributes as Array<Record<string, unknown>>) ?? [],
      landing_site: ch.landing_site as string | null,
      referring_site: ch.referring_site as string | null,
      source_name: ch.source_name as string | null,
      source_identifier: ch.source_identifier as string | null,
      source_url: ch.source_url as string | null,
      completed_at: ch.completed_at ? new Date(ch.completed_at as string) : null,
      abandoned_checkout_url: ch.abandoned_checkout_url as string | null,
      admin_graphql_api_id: ch.admin_graphql_api_id as string | null,
      created_at: new Date(ch.created_at as string),
      updated_at: new Date(ch.updated_at as string),
    } as unknown as ShopifyCheckoutRecord;
  }
}
