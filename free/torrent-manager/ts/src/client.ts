import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('torrent-manager:client');

export interface AddTorrentOptions {
  savePath?: string;
  category?: string;
}

export interface TorrentAdded {
  hash: string;
  name?: string;
}

export interface TorrentSummary {
  hash: string;
  name: string;
  status: string;
  progress: number;
  size_bytes: number;
}

export interface TorrentDetail {
  hash: string;
  name: string;
  status: string;
  progress: number;
  download_speed?: number;
  seeds?: number;
}

export interface ListTorrentsOptions {
  category?: string;
  status?: string;
}

export class TorrentManagerClient {
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

  async add(magnetOrUrl: string, options?: AddTorrentOptions): Promise<TorrentAdded> {
    return this.request<TorrentAdded>('POST', '/torrents', { magnetOrUrl, ...options });
  }

  async list(options?: ListTorrentsOptions): Promise<TorrentSummary[]> {
    const params = new URLSearchParams();
    if (options?.category) params.set('category', options.category);
    if (options?.status) params.set('status', options.status);
    const qs = params.toString() ? `?${params}` : '';
    return this.request<TorrentSummary[]>('GET', `/torrents${qs}`);
  }

  async get(hash: string): Promise<TorrentDetail> {
    return this.request<TorrentDetail>('GET', `/torrents/${hash}`);
  }

  async pause(hash: string): Promise<void> {
    await this.request<void>('POST', `/torrents/${hash}/pause`);
  }

  async resume(hash: string): Promise<void> {
    await this.request<void>('POST', `/torrents/${hash}/resume`);
  }

  async delete(hash: string, deleteFiles?: boolean): Promise<void> {
    await this.request<void>('DELETE', `/torrents/${hash}`, { deleteFiles });
  }
}
