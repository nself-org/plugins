import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('notifications:client');

export interface Notification {
  id: string;
  type: string;
  title: string;
  body: string;
  read: boolean;
  created_at: string;
}

export interface SendNotificationOptions {
  userId: string;
  type: string;
  title: string;
  body: string;
  data?: Record<string, unknown>;
}

export interface ListNotificationsOptions {
  read?: boolean;
  limit?: number;
}

export class NotificationsClient {
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

  async send(notification: SendNotificationOptions): Promise<{ id: string }> {
    return this.request<{ id: string }>('POST', '/notifications', notification);
  }

  async list(userId: string, options?: ListNotificationsOptions): Promise<Notification[]> {
    const params = new URLSearchParams({ userId });
    if (options?.read !== undefined) params.set('read', String(options.read));
    if (options?.limit !== undefined) params.set('limit', String(options.limit));
    return this.request<Notification[]>('GET', `/notifications?${params}`);
  }

  async markRead(id: string): Promise<void> {
    await this.request<void>('PUT', `/notifications/${id}/read`);
  }

  async markAllRead(userId: string): Promise<{ marked: number }> {
    return this.request<{ marked: number }>('PUT', `/notifications/read-all`, { userId });
  }

  async delete(id: string): Promise<void> {
    await this.request<void>('DELETE', `/notifications/${id}`);
  }
}
