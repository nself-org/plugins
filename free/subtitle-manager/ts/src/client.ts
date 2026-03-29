import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('subtitle-manager:client');

export interface SearchOptions {
  language?: string;
  imdbId?: string;
  limit?: number;
}

export interface SubtitleSearchResult {
  id: string;
  filename: string;
  language: string;
  release?: string;
}

export interface SubtitleDownload {
  content: string;
  filename: string;
  format: string;
}

export interface SubtitleRecord {
  id: string;
  language: string;
  filename: string;
  source: string;
}

export interface SubtitleUpload {
  content: string;
  language: string;
  filename: string;
}

export class SubtitleManagerClient {
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

  async search(query: string, options?: SearchOptions): Promise<SubtitleSearchResult[]> {
    const params = new URLSearchParams({ query });
    if (options?.language) params.set('language', options.language);
    if (options?.imdbId) params.set('imdbId', options.imdbId);
    if (options?.limit !== undefined) params.set('limit', String(options.limit));
    return this.request<SubtitleSearchResult[]>('GET', `/subtitles/search?${params}`);
  }

  async download(subtitleId: string): Promise<SubtitleDownload> {
    return this.request<SubtitleDownload>('GET', `/subtitles/${subtitleId}/download`);
  }

  async list(mediaId: string): Promise<SubtitleRecord[]> {
    return this.request<SubtitleRecord[]>('GET', `/media/${mediaId}/subtitles`);
  }

  async upload(mediaId: string, subtitle: SubtitleUpload): Promise<{ id: string }> {
    return this.request<{ id: string }>('POST', `/media/${mediaId}/subtitles`, subtitle);
  }

  async delete(id: string): Promise<void> {
    await this.request<void>('DELETE', `/subtitles/${id}`);
  }
}
