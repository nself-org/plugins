/**
 * PayPal API Client
 * OAuth2 authentication, rate limiting, pagination, Transaction Search windowing
 */

import { RateLimiter, createLogger } from '@nself/plugin-utils';
import type {
  PayPalTokenResponse,
  PayPalTransactionSearchResponse,
  PayPalTransactionDetail,
  PayPalOrder,
  PayPalSubscription,
  PayPalSubscriptionPlan,
  PayPalProduct,
  PayPalDispute,
  PayPalPayout,
  PayPalInvoice,
  PayPalCapture,
  PayPalRefund,
  PayPalListResponse,
  PayPalDisputeListResponse,
  PayPalWebhookEvent,
  PayPalWebhookVerifyRequest,
} from './types.js';

const logger = createLogger('paypal:client');

interface TokenCache {
  accessToken: string;
  expiresAt: number;
}

export class PayPalClient {
  private baseUrl: string;
  private clientId: string;
  private clientSecret: string;
  private rateLimiter: RateLimiter;
  private tokenCache: TokenCache | null = null;
  private tokenRefreshPromise: Promise<string> | null = null;

  constructor(clientId: string, clientSecret: string, environment: 'sandbox' | 'live' = 'live') {
    this.clientId = clientId;
    this.clientSecret = clientSecret;
    this.baseUrl = environment === 'sandbox'
      ? 'https://api-m.sandbox.paypal.com'
      : 'https://api-m.paypal.com';
    this.rateLimiter = new RateLimiter(30);
  }

  // ─── OAuth2 Token Management ─────────────────────────────────────────────

  private async getAccessToken(): Promise<string> {
    // Return cached token if still valid (5-min buffer)
    if (this.tokenCache && Date.now() < this.tokenCache.expiresAt - 300_000) {
      return this.tokenCache.accessToken;
    }

    // Deduplicate concurrent refresh requests
    if (this.tokenRefreshPromise) {
      return this.tokenRefreshPromise;
    }

    this.tokenRefreshPromise = this.refreshToken();
    try {
      return await this.tokenRefreshPromise;
    } finally {
      this.tokenRefreshPromise = null;
    }
  }

  private async refreshToken(): Promise<string> {
    logger.debug('Refreshing OAuth2 token');

    const credentials = Buffer.from(`${this.clientId}:${this.clientSecret}`).toString('base64');
    const response = await fetch(`${this.baseUrl}/v1/oauth2/token`, {
      method: 'POST',
      headers: {
        'Authorization': `Basic ${credentials}`,
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: 'grant_type=client_credentials',
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`PayPal OAuth2 token request failed (${response.status}): ${errorText}`);
    }

    const data = await response.json() as PayPalTokenResponse;
    this.tokenCache = {
      accessToken: data.access_token,
      expiresAt: Date.now() + data.expires_in * 1000,
    };

    logger.debug('OAuth2 token refreshed', { expiresIn: data.expires_in });
    return data.access_token;
  }

  // ─── HTTP Methods ────────────────────────────────────────────────────────

  private async request<T>(method: string, path: string, options?: {
    body?: unknown;
    params?: Record<string, string>;
    contentType?: string;
  }): Promise<T> {
    await this.rateLimiter.acquire();
    const token = await this.getAccessToken();

    let url = `${this.baseUrl}${path}`;
    if (options?.params) {
      const searchParams = new URLSearchParams();
      for (const [key, value] of Object.entries(options.params)) {
        if (value !== undefined && value !== null) {
          searchParams.append(key, value);
        }
      }
      const paramString = searchParams.toString();
      if (paramString) url += `?${paramString}`;
    }

    const headers: Record<string, string> = {
      'Authorization': `Bearer ${token}`,
      'Content-Type': options?.contentType ?? 'application/json',
    };

    const fetchOptions: RequestInit = { method, headers };
    if (options?.body) {
      fetchOptions.body = JSON.stringify(options.body);
    }

    const response = await fetch(url, fetchOptions);

    if (response.status === 401) {
      // Token may have expired; refresh and retry once
      this.tokenCache = null;
      const newToken = await this.getAccessToken();
      headers['Authorization'] = `Bearer ${newToken}`;
      const retryResponse = await fetch(url, { method, headers, body: fetchOptions.body });
      if (!retryResponse.ok) {
        const errorText = await retryResponse.text();
        throw new Error(`PayPal API error (${retryResponse.status}): ${errorText}`);
      }
      if (retryResponse.status === 204) return {} as T;
      return await retryResponse.json() as T;
    }

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`PayPal API error (${response.status}): ${errorText}`);
    }

    if (response.status === 204) return {} as T;
    return await response.json() as T;
  }

  private async get<T>(path: string, params?: Record<string, string>): Promise<T> {
    return this.request<T>('GET', path, { params });
  }

  private async post<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>('POST', path, { body });
  }

  // ─── Transaction Search (31-day windowing) ───────────────────────────────

  async listAllTransactions(options?: {
    startDate?: Date;
    endDate?: Date;
  }): Promise<PayPalTransactionDetail[]> {
    const endDate = options?.endDate ?? new Date();
    const startDate = options?.startDate ?? new Date('2020-01-01');
    const allTransactions: PayPalTransactionDetail[] = [];
    const maxWindowDays = 31;

    let windowStart = new Date(startDate);
    while (windowStart < endDate) {
      const windowEnd = new Date(Math.min(
        windowStart.getTime() + maxWindowDays * 86400_000,
        endDate.getTime()
      ));

      logger.debug('Fetching transactions window', {
        start: windowStart.toISOString(),
        end: windowEnd.toISOString(),
      });

      let page = 1;
      let totalPages = 1;

      while (page <= totalPages) {
        const response = await this.get<PayPalTransactionSearchResponse>(
          '/v1/reporting/transactions',
          {
            start_date: windowStart.toISOString(),
            end_date: windowEnd.toISOString(),
            fields: 'all',
            page_size: '500',
            page: String(page),
          }
        );

        if (response.transaction_details?.length > 0) {
          allTransactions.push(...response.transaction_details);
        }

        totalPages = response.total_pages || 1;
        page++;
      }

      windowStart = windowEnd;
    }

    logger.info('Transaction search complete', { total: allTransactions.length });
    return allTransactions;
  }

  // ─── Orders ──────────────────────────────────────────────────────────────

  async getOrder(orderId: string): Promise<PayPalOrder> {
    return this.get<PayPalOrder>(`/v2/checkout/orders/${orderId}`);
  }

  // ─── Captures ────────────────────────────────────────────────────────────

  async getCapture(captureId: string): Promise<PayPalCapture> {
    return this.get<PayPalCapture>(`/v2/payments/captures/${captureId}`);
  }

  // ─── Refunds ─────────────────────────────────────────────────────────────

  async getRefund(refundId: string): Promise<PayPalRefund> {
    return this.get<PayPalRefund>(`/v2/payments/refunds/${refundId}`);
  }

  // ─── Subscriptions ───────────────────────────────────────────────────────

  async listAllSubscriptionPlans(): Promise<PayPalSubscriptionPlan[]> {
    const plans: PayPalSubscriptionPlan[] = [];
    let page = 1;
    let hasMore = true;

    while (hasMore) {
      const response = await this.get<PayPalListResponse<PayPalSubscriptionPlan>>(
        '/v1/billing/plans',
        { page_size: '20', page: String(page), total_required: 'true' }
      );

      if (response.items && response.items.length > 0) {
        plans.push(...response.items);
        page++;
      } else {
        hasMore = false;
      }

      if (response.total_pages && page > response.total_pages) {
        hasMore = false;
      }
    }

    return plans;
  }

  async getSubscription(subscriptionId: string): Promise<PayPalSubscription> {
    return this.get<PayPalSubscription>(`/v1/billing/subscriptions/${subscriptionId}`);
  }

  async listAllSubscriptions(planId?: string): Promise<PayPalSubscription[]> {
    // PayPal doesn't have a list-all-subscriptions endpoint.
    // Subscriptions are typically discovered via webhooks or transaction search.
    // If a planId is given, we can't list by plan either — return empty.
    logger.debug('Subscriptions are discovered via webhooks/transactions', { planId });
    return [];
  }

  // ─── Products ────────────────────────────────────────────────────────────

  async listAllProducts(): Promise<PayPalProduct[]> {
    const products: PayPalProduct[] = [];
    let page = 1;
    let hasMore = true;

    while (hasMore) {
      const response = await this.get<PayPalListResponse<PayPalProduct>>(
        '/v1/catalogs/products',
        { page_size: '20', page: String(page), total_required: 'true' }
      );

      if (response.items && response.items.length > 0) {
        products.push(...response.items);
        page++;
      } else {
        hasMore = false;
      }

      if (response.total_pages && page > response.total_pages) {
        hasMore = false;
      }
    }

    return products;
  }

  // ─── Disputes ────────────────────────────────────────────────────────────

  async listAllDisputes(options?: { startDate?: string }): Promise<PayPalDispute[]> {
    const disputes: PayPalDispute[] = [];
    let nextUrl: string | null = null;
    let isFirst = true;

    while (isFirst || nextUrl) {
      isFirst = false;

      let response: PayPalDisputeListResponse;
      if (nextUrl) {
        // nextUrl is a full URL from PayPal
        await this.rateLimiter.acquire();
        const token = await this.getAccessToken();
        const fetchResponse = await fetch(nextUrl, {
          headers: { 'Authorization': `Bearer ${token}`, 'Content-Type': 'application/json' },
        });
        response = await fetchResponse.json() as PayPalDisputeListResponse;
      } else {
        const params: Record<string, string> = { page_size: '50' };
        if (options?.startDate) {
          params.start_time = options.startDate;
        }
        response = await this.get<PayPalDisputeListResponse>('/v1/customer/disputes', params);
      }

      if (response.items && response.items.length > 0) {
        disputes.push(...response.items);
      }

      const nextLink = response.links?.find(l => l.rel === 'next');
      nextUrl = nextLink?.href ?? null;
    }

    return disputes;
  }

  // ─── Payouts ─────────────────────────────────────────────────────────────

  async getPayout(payoutBatchId: string): Promise<PayPalPayout> {
    return this.get<PayPalPayout>(`/v1/payments/payouts/${payoutBatchId}`);
  }

  // ─── Invoices ────────────────────────────────────────────────────────────

  async listAllInvoices(): Promise<PayPalInvoice[]> {
    const invoices: PayPalInvoice[] = [];
    let page = 1;
    let hasMore = true;

    while (hasMore) {
      const response = await this.get<{ items?: PayPalInvoice[]; total_items?: number; total_pages?: number }>(
        '/v2/invoicing/invoices',
        { page_size: '100', page: String(page), total_required: 'true' }
      );

      if (response.items && response.items.length > 0) {
        invoices.push(...response.items);
        page++;
      } else {
        hasMore = false;
      }

      if (response.total_pages && page > response.total_pages) {
        hasMore = false;
      }
    }

    return invoices;
  }

  // ─── Webhook Verification ────────────────────────────────────────────────

  async verifyWebhookSignature(
    webhookId: string,
    headers: Record<string, string>,
    body: string
  ): Promise<boolean> {
    try {
      const webhookEvent = JSON.parse(body) as PayPalWebhookEvent;
      const verifyRequest: PayPalWebhookVerifyRequest = {
        auth_algo: headers['paypal-auth-algo'] ?? '',
        cert_url: headers['paypal-cert-url'] ?? '',
        transmission_id: headers['paypal-transmission-id'] ?? '',
        transmission_sig: headers['paypal-transmission-sig'] ?? '',
        transmission_time: headers['paypal-transmission-time'] ?? '',
        webhook_id: webhookId,
        webhook_event: webhookEvent,
      };

      const response = await this.post<{ verification_status: string }>(
        '/v1/notifications/verify-webhook-signature',
        verifyRequest
      );

      return response.verification_status === 'SUCCESS';
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Webhook verification failed', { error: message });
      return false;
    }
  }

  // ─── Webhook Events List ─────────────────────────────────────────────────

  async listWebhookEvents(options?: {
    startTime?: string;
    endTime?: string;
    pageSize?: number;
  }): Promise<PayPalWebhookEvent[]> {
    const events: PayPalWebhookEvent[] = [];
    const params: Record<string, string> = {
      page_size: String(options?.pageSize ?? 100),
    };
    if (options?.startTime) params.start_time = options.startTime;
    if (options?.endTime) params.end_time = options.endTime;

    const response = await this.get<{ events?: PayPalWebhookEvent[] }>(
      '/v1/notifications/webhooks-events',
      params
    );

    if (response.events) {
      events.push(...response.events);
    }

    return events;
  }
}
