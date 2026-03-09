import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('webhooks:client');

export interface RegisterEndpointOptions {
  url: string;
  events: string[];
  secret?: string;
  name?: string;
}

export interface EndpointCreated {
  id: string;
  token: string;
}

export interface EndpointSummary {
  id: string;
  url: string;
  events: string[];
  active: boolean;
  created_at: string;
}

export interface DeliveryStats {
  sent: number;
  failed: number;
}

export interface EndpointDetail {
  id: string;
  url: string;
  events: string[];
  active: boolean;
  delivery_stats?: DeliveryStats;
}

export interface DeliverySummary {
  id: string;
  event: string;
  status: string;
  created_at: string;
}

export interface ListEndpointsOptions {
  event?: string;
  active?: boolean;
}

export interface ListDeliveriesOptions {
  limit?: number;
}

export class WebhooksClient {
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

  async register(endpoint: RegisterEndpointOptions): Promise<EndpointCreated> {
    return this.request<EndpointCreated>('POST', '/endpoints', endpoint);
  }

  async list(options?: ListEndpointsOptions): Promise<EndpointSummary[]> {
    const params = new URLSearchParams();
    if (options?.event) params.set('event', options.event);
    if (options?.active !== undefined) params.set('active', String(options.active));
    const qs = params.toString() ? `?${params}` : '';
    return this.request<EndpointSummary[]>('GET', `/endpoints${qs}`);
  }

  async get(id: string): Promise<EndpointDetail> {
    return this.request<EndpointDetail>('GET', `/endpoints/${id}`);
  }

  async delete(id: string): Promise<void> {
    await this.request<void>('DELETE', `/endpoints/${id}`);
  }

  async deliver(
    endpointId: string,
    event: string,
    payload: Record<string, unknown>,
  ): Promise<{ delivery_id: string }> {
    return this.request<{ delivery_id: string }>('POST', `/endpoints/${endpointId}/deliver`, {
      event,
      payload,
    });
  }

  async listDeliveries(
    endpointId: string,
    options?: ListDeliveriesOptions,
  ): Promise<DeliverySummary[]> {
    const params = new URLSearchParams();
    if (options?.limit !== undefined) params.set('limit', String(options.limit));
    const qs = params.toString() ? `?${params}` : '';
    return this.request<DeliverySummary[]>('GET', `/endpoints/${endpointId}/deliveries${qs}`);
  }
}
