import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('mdns:client');

export interface DiscoverOptions {
  type?: string;
  timeout?: number;
}

export interface DiscoveredService {
  name: string;
  host: string;
  port: number;
  type: string;
  txt?: Record<string, string>;
}

export interface ServiceRecord {
  name: string;
  host: string;
  port: number;
  type: string;
  last_seen: string;
}

export interface ResolvedService {
  name: string;
  host: string;
  port: number;
  addresses: string[];
}

export interface AnnounceService {
  name: string;
  type: string;
  port: number;
  txt?: Record<string, string>;
}

export class MDNSClient {
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

  async discover(options?: DiscoverOptions): Promise<DiscoveredService[]> {
    return this.request<DiscoveredService[]>('POST', '/discover', options);
  }

  async listServices(): Promise<ServiceRecord[]> {
    return this.request<ServiceRecord[]>('GET', '/services');
  }

  async resolveService(name: string): Promise<ResolvedService> {
    const params = new URLSearchParams({ name });
    return this.request<ResolvedService>('GET', `/services/resolve?${params}`);
  }

  async announce(service: AnnounceService): Promise<{ announced: boolean }> {
    return this.request<{ announced: boolean }>('POST', '/services/announce', service);
  }
}
