import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('link-preview:client');

export interface LinkPreview {
  url: string;
  title?: string;
  description?: string;
  image?: string;
  site_name?: string;
  type?: string;
}

export interface BatchPreviewResult {
  url: string;
  title?: string;
  description?: string;
  image?: string;
  error?: string;
}

export class LinkPreviewClient {
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

  async getPreview(url: string): Promise<LinkPreview> {
    const params = new URLSearchParams({ url });
    return this.request<LinkPreview>('GET', `/preview?${params}`);
  }

  async batchPreview(urls: string[]): Promise<BatchPreviewResult[]> {
    return this.request<BatchPreviewResult[]>('POST', '/preview/batch', { urls });
  }

  async clearCache(url: string): Promise<void> {
    await this.request<void>('DELETE', '/preview/cache', { url });
  }
}
