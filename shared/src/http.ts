/**
 * HTTP client utilities for nself plugins
 */

import type { RetryConfig } from './types.js';
import { withRetry } from './webhook.js';

export interface HttpClientConfig {
  baseUrl: string;
  headers?: Record<string, string>;
  timeout?: number;
  retryConfig?: RetryConfig;
}

export interface HttpResponse<T = unknown> {
  status: number;
  data: T;
  headers: Headers;
}

export class HttpClient {
  private config: HttpClientConfig;
  private retryConfig: RetryConfig;

  constructor(config: HttpClientConfig) {
    this.config = config;
    this.retryConfig = config.retryConfig ?? {
      maxRetries: 3,
      baseDelay: 1000,
      maxDelay: 30000,
      backoffMultiplier: 2,
    };
  }

  private async request<T>(
    method: string,
    endpoint: string,
    options?: {
      body?: unknown;
      params?: Record<string, string>;
      headers?: Record<string, string>;
    }
  ): Promise<HttpResponse<T>> {
    let url = `${this.config.baseUrl}${endpoint}`;

    if (options?.params) {
      const searchParams = new URLSearchParams();
      for (const [key, value] of Object.entries(options.params)) {
        if (value !== undefined && value !== null) {
          searchParams.append(key, value);
        }
      }
      const paramString = searchParams.toString();
      if (paramString) {
        url += `?${paramString}`;
      }
    }

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...this.config.headers,
      ...options?.headers,
    };

    const fetchOptions: RequestInit = {
      method,
      headers,
      signal: this.config.timeout
        ? AbortSignal.timeout(this.config.timeout)
        : undefined,
    };

    if (options?.body) {
      fetchOptions.body = JSON.stringify(options.body);
    }

    return withRetry(async () => {
      const response = await fetch(url, fetchOptions);

      let data: T;
      const contentType = response.headers.get('content-type');
      if (contentType?.includes('application/json')) {
        data = (await response.json()) as T;
      } else {
        data = (await response.text()) as T;
      }

      if (!response.ok) {
        const errorMessage = typeof data === 'object' && data !== null
          ? JSON.stringify(data)
          : String(data);
        throw new HttpError(response.status, errorMessage, data);
      }

      return {
        status: response.status,
        data,
        headers: response.headers,
      };
    }, this.retryConfig);
  }

  async get<T>(endpoint: string, params?: Record<string, string>): Promise<T> {
    const response = await this.request<T>('GET', endpoint, { params });
    return response.data;
  }

  async post<T>(endpoint: string, body?: unknown): Promise<T> {
    const response = await this.request<T>('POST', endpoint, { body });
    return response.data;
  }

  async put<T>(endpoint: string, body?: unknown): Promise<T> {
    const response = await this.request<T>('PUT', endpoint, { body });
    return response.data;
  }

  async patch<T>(endpoint: string, body?: unknown): Promise<T> {
    const response = await this.request<T>('PATCH', endpoint, { body });
    return response.data;
  }

  async delete<T>(endpoint: string): Promise<T> {
    const response = await this.request<T>('DELETE', endpoint);
    return response.data;
  }

  /**
   * Iterate through paginated results
   */
  async *paginate<T>(
    endpoint: string,
    params?: Record<string, string>,
    options?: {
      pageSize?: number;
      cursorParam?: string;
      cursorExtractor?: (response: unknown) => string | undefined;
      hasMoreExtractor?: (response: unknown) => boolean;
      dataExtractor?: (response: unknown) => T[];
    }
  ): AsyncGenerator<T[], void, unknown> {
    const {
      pageSize = 100,
      cursorParam = 'starting_after',
      cursorExtractor = (r: unknown) => {
        const data = (r as { data?: Array<{ id?: string }> }).data;
        return data?.[data.length - 1]?.id;
      },
      hasMoreExtractor = (r: unknown) => (r as { has_more?: boolean }).has_more === true,
      dataExtractor = (r: unknown) => (r as { data?: T[] }).data ?? [],
    } = options ?? {};

    let cursor: string | undefined;
    let hasMore = true;

    while (hasMore) {
      const requestParams: Record<string, string> = {
        ...params,
        limit: String(pageSize),
      };

      if (cursor) {
        requestParams[cursorParam] = cursor;
      }

      const response = await this.get<unknown>(endpoint, requestParams);
      const data = dataExtractor(response);

      if (data.length > 0) {
        yield data;
        cursor = cursorExtractor(response);
      }

      hasMore = hasMoreExtractor(response);

      if (data.length < pageSize) {
        hasMore = false;
      }
    }
  }
}

export class HttpError extends Error {
  status: number;
  response: unknown;

  constructor(status: number, message: string, response?: unknown) {
    super(message);
    this.name = 'HttpError';
    this.status = status;
    this.response = response;
  }
}

/**
 * Rate limiter for API requests
 */
export class RateLimiter {
  private tokens: number;
  private maxTokens: number;
  private refillRate: number;
  private lastRefill: number;

  constructor(requestsPerSecond: number) {
    this.maxTokens = requestsPerSecond;
    this.tokens = requestsPerSecond;
    this.refillRate = requestsPerSecond;
    this.lastRefill = Date.now();
  }

  private refill(): void {
    const now = Date.now();
    const elapsed = (now - this.lastRefill) / 1000;
    this.tokens = Math.min(this.maxTokens, this.tokens + elapsed * this.refillRate);
    this.lastRefill = now;
  }

  async acquire(): Promise<void> {
    this.refill();

    if (this.tokens >= 1) {
      this.tokens -= 1;
      return;
    }

    const waitTime = ((1 - this.tokens) / this.refillRate) * 1000;
    await new Promise(resolve => setTimeout(resolve, waitTime));
    this.tokens = 0;
    this.lastRefill = Date.now();
  }
}
