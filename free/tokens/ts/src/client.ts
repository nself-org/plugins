import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('tokens:client');

export interface CreateTokenOptions {
  userId?: string;
  name?: string;
  expiresIn?: number;
  scopes?: string[];
}

export interface TokenCreated {
  id: string;
  token: string;
  expires_at?: string;
}

export interface TokenValidation {
  valid: boolean;
  userId?: string;
  scopes?: string[];
  expires_at?: string;
}

export interface TokenRecord {
  id: string;
  name?: string;
  last_used?: string;
  expires_at?: string;
  scopes?: string[];
}

export class TokensClient {
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

  async create(options?: CreateTokenOptions): Promise<TokenCreated> {
    return this.request<TokenCreated>('POST', '/tokens', options);
  }

  async validate(token: string): Promise<TokenValidation> {
    return this.request<TokenValidation>('POST', '/tokens/validate', { token });
  }

  async list(userId?: string): Promise<TokenRecord[]> {
    const params = new URLSearchParams();
    if (userId) params.set('userId', userId);
    const qs = params.toString() ? `?${params}` : '';
    return this.request<TokenRecord[]>('GET', `/tokens${qs}`);
  }

  async revoke(id: string): Promise<void> {
    await this.request<void>('DELETE', `/tokens/${id}`);
  }

  async revokeAll(userId: string): Promise<{ revoked: number }> {
    return this.request<{ revoked: number }>('DELETE', '/tokens', { userId });
  }
}
