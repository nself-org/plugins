import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('feature-flags:client');

export interface FlagEvaluation {
  flag: string;
  enabled: boolean;
  value?: unknown;
  variant?: string;
}

export interface FlagSummary {
  key: string;
  enabled: boolean;
  description?: string;
  type: string;
}

export interface SetFlagOptions {
  value?: unknown;
  rollout?: number;
}

export class FeatureFlagsClient {
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

  async evaluate(
    flag: string,
    userId?: string,
    context?: Record<string, unknown>,
  ): Promise<FlagEvaluation> {
    return this.request<FlagEvaluation>('POST', `/flags/${flag}/evaluate`, { userId, context });
  }

  async listFlags(): Promise<FlagSummary[]> {
    return this.request<FlagSummary[]>('GET', '/flags');
  }

  async setFlag(key: string, enabled: boolean, options?: SetFlagOptions): Promise<void> {
    await this.request<void>('PUT', `/flags/${key}`, { enabled, ...options });
  }

  async deleteFlag(key: string): Promise<void> {
    await this.request<void>('DELETE', `/flags/${key}`);
  }

  async evaluateAll(
    userId?: string,
    context?: Record<string, unknown>,
  ): Promise<Record<string, boolean>> {
    return this.request<Record<string, boolean>>('POST', '/flags/evaluate-all', {
      userId,
      context,
    });
  }
}
