import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('invitations:client');

export interface CreateInvitationOptions {
  role?: string;
  expiresIn?: number;
  data?: Record<string, unknown>;
}

export interface InvitationCreated {
  id: string;
  token: string;
  url: string;
}

export interface InvitationDetail {
  id: string;
  email: string;
  status: string;
  expires_at?: string;
  created_at: string;
}

export interface InvitationSummary {
  id: string;
  email: string;
  status: string;
  created_at: string;
}

export interface ListInvitationsOptions {
  status?: string;
  limit?: number;
}

export class InvitationsClient {
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

  async createInvitation(
    email: string,
    options?: CreateInvitationOptions,
  ): Promise<InvitationCreated> {
    return this.request<InvitationCreated>('POST', '/invitations', { email, ...options });
  }

  async getInvitation(tokenOrId: string): Promise<InvitationDetail> {
    return this.request<InvitationDetail>('GET', `/invitations/${tokenOrId}`);
  }

  async listInvitations(options?: ListInvitationsOptions): Promise<InvitationSummary[]> {
    const params = new URLSearchParams();
    if (options?.status) params.set('status', options.status);
    if (options?.limit !== undefined) params.set('limit', String(options.limit));
    const qs = params.toString() ? `?${params}` : '';
    return this.request<InvitationSummary[]>('GET', `/invitations${qs}`);
  }

  async revokeInvitation(id: string): Promise<void> {
    await this.request<void>('DELETE', `/invitations/${id}`);
  }

  async acceptInvitation(token: string, userId: string): Promise<{ accepted: boolean }> {
    return this.request<{ accepted: boolean }>('POST', '/invitations/accept', { token, userId });
  }
}
