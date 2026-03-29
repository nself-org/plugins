import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('search:client');

export interface SearchOptions {
  limit?: number;
  filters?: string;
  sort?: string[];
}

export interface SearchResult {
  hits: Array<Record<string, unknown>>;
  total: number;
  took_ms: number;
}

export interface IndexInfo {
  name: string;
  primary_key: string;
  document_count: number;
}

export class SearchClient {
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

  async search(index: string, query: string, options?: SearchOptions): Promise<SearchResult> {
    return this.request<SearchResult>('POST', `/indexes/${index}/search`, { query, ...options });
  }

  async index(
    index: string,
    documents: Array<{ id: string } & Record<string, unknown>>,
  ): Promise<{ task_id: string }> {
    return this.request<{ task_id: string }>('POST', `/indexes/${index}/documents`, documents);
  }

  async deleteDocument(index: string, id: string): Promise<void> {
    await this.request<void>('DELETE', `/indexes/${index}/documents/${id}`);
  }

  async listIndexes(): Promise<IndexInfo[]> {
    return this.request<IndexInfo[]>('GET', '/indexes');
  }

  async clearIndex(index: string): Promise<void> {
    await this.request<void>('DELETE', `/indexes/${index}/documents`);
  }
}
