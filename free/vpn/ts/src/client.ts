import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('vpn:client');

export interface VPNStatus {
  connected: boolean;
  server?: string;
  ip?: string;
  protocol?: string;
}

export interface VPNServer {
  id: string;
  name: string;
  country: string;
  load?: number;
}

export interface ListServersOptions {
  country?: string;
  type?: string;
}

export interface VPNStats {
  bytes_sent: number;
  bytes_received: number;
  connected_since?: string;
}

export class VPNClient {
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

  async getStatus(): Promise<VPNStatus> {
    return this.request<VPNStatus>('GET', '/vpn/status');
  }

  async listServers(options?: ListServersOptions): Promise<VPNServer[]> {
    const params = new URLSearchParams();
    if (options?.country) params.set('country', options.country);
    if (options?.type) params.set('type', options.type);
    const qs = params.toString() ? `?${params}` : '';
    return this.request<VPNServer[]>('GET', `/vpn/servers${qs}`);
  }

  async connect(serverId: string, protocol?: string): Promise<{ connected: boolean; ip?: string }> {
    return this.request<{ connected: boolean; ip?: string }>('POST', '/vpn/connect', {
      serverId,
      protocol,
    });
  }

  async disconnect(): Promise<{ disconnected: boolean }> {
    return this.request<{ disconnected: boolean }>('POST', '/vpn/disconnect');
  }

  async getStats(): Promise<VPNStats> {
    return this.request<VPNStats>('GET', '/vpn/stats');
  }
}
