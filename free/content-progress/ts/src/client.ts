import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('content-progress:client');

export interface ProgressUpdate {
  position_seconds?: number;
  percentage?: number;
  completed?: boolean;
}

export interface ProgressRecord {
  position_seconds?: number;
  percentage?: number;
  completed: boolean;
  updated_at: string;
}

export interface ProgressSummary {
  content_id: string;
  percentage?: number;
  completed: boolean;
  updated_at: string;
}

export interface ListProgressOptions {
  completed?: boolean;
  limit?: number;
}

export class ContentProgressClient {
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

  async updateProgress(
    userId: string,
    contentId: string,
    progress: ProgressUpdate,
  ): Promise<void> {
    await this.request<void>('PUT', `/users/${userId}/progress/${contentId}`, progress);
  }

  async getProgress(userId: string, contentId: string): Promise<ProgressRecord> {
    return this.request<ProgressRecord>('GET', `/users/${userId}/progress/${contentId}`);
  }

  async listProgress(userId: string, options?: ListProgressOptions): Promise<ProgressSummary[]> {
    const params = new URLSearchParams();
    if (options?.completed !== undefined) params.set('completed', String(options.completed));
    if (options?.limit !== undefined) params.set('limit', String(options.limit));
    const qs = params.toString() ? `?${params}` : '';
    return this.request<ProgressSummary[]>('GET', `/users/${userId}/progress${qs}`);
  }

  async deleteProgress(userId: string, contentId: string): Promise<void> {
    await this.request<void>('DELETE', `/users/${userId}/progress/${contentId}`);
  }
}
