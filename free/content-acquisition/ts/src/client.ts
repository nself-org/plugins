import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('content-acquisition:client');

export interface AddFeedOptions {
  category?: string;
  quality?: string;
}

export interface Feed {
  id: string;
  url: string;
  title?: string;
}

export interface FeedDetail {
  id: string;
  url: string;
  title?: string;
  last_fetched?: string;
  item_count: number;
}

export interface FeedSummary {
  id: string;
  url: string;
  title?: string;
  status: string;
}

export interface FeedItem {
  id: string;
  title: string;
  url: string;
  published?: string;
  content?: string;
}

export interface GetItemsOptions {
  limit?: number;
  unread?: boolean;
}

export class ContentAcquisitionClient {
  private baseUrl: string;
  private apiKey?: string;

  constructor(baseUrl: string, apiKey?: string) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
    this.apiKey = apiKey;
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (this.apiKey) headers['Authorization'] = `Bearer ${this.apiKey}`;
    logger.debug(`${method} ${path}`);
    const res = await fetch(`${this.baseUrl}${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!res.ok) throw new Error(`${method} ${path} failed: ${res.status}`);
    return res.json() as Promise<T>;
  }

  async addFeed(url: string, options?: AddFeedOptions): Promise<Feed> {
    return this.request<Feed>('POST', '/feeds', { url, ...options });
  }

  async getFeed(id: string): Promise<FeedDetail> {
    return this.request<FeedDetail>('GET', `/feeds/${id}`);
  }

  async listFeeds(): Promise<FeedSummary[]> {
    return this.request<FeedSummary[]>('GET', '/feeds');
  }

  async removeFeed(id: string): Promise<void> {
    await this.request<void>('DELETE', `/feeds/${id}`);
  }

  async getItems(feedId?: string, options?: GetItemsOptions): Promise<FeedItem[]> {
    const params = new URLSearchParams();
    if (feedId) params.set('feedId', feedId);
    if (options?.limit !== undefined) params.set('limit', String(options.limit));
    if (options?.unread !== undefined) params.set('unread', String(options.unread));
    const qs = params.toString() ? `?${params}` : '';
    return this.request<FeedItem[]>('GET', `/items${qs}`);
  }
}
