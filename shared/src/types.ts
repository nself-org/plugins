/**
 * Shared types for nself plugins
 */

export interface PluginConfig {
  name: string;
  version: string;
  port: number;
  host: string;
  database: DatabaseConfig;
  webhookPath: string;
  webhookSecret?: string;
  syncInterval?: number;
  logLevel: LogLevel;
}

export interface DatabaseConfig {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl?: boolean;
  maxConnections?: number;
}

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface WebhookEvent {
  id: string;
  type: string;
  data: unknown;
  timestamp: Date;
  signature?: string;
  processed: boolean;
  processedAt?: Date;
  error?: string;
  retryCount: number;
}

export interface SyncResult {
  success: boolean;
  resource: string;
  synced: number;
  errors: number;
  duration: number;
  lastSyncedId?: string;
}

export interface SyncOptions {
  incremental?: boolean;
  since?: Date;
  limit?: number;
  resources?: string[];
}

export interface PaginatedResponse<T> {
  data: T[];
  hasMore: boolean;
  nextCursor?: string;
  total?: number;
}

export interface ApiClient<T = unknown> {
  get(endpoint: string, params?: Record<string, string>): Promise<T>;
  post(endpoint: string, data?: unknown): Promise<T>;
  put(endpoint: string, data?: unknown): Promise<T>;
  delete(endpoint: string): Promise<T>;
  list<R>(endpoint: string, params?: Record<string, string>): AsyncGenerator<R[], void, unknown>;
}

export interface PluginService {
  name: string;
  start(): Promise<void>;
  stop(): Promise<void>;
  sync(options?: SyncOptions): Promise<SyncResult[]>;
  handleWebhook(event: WebhookEvent): Promise<void>;
  getStatus(): Promise<PluginStatus>;
}

export interface PluginStatus {
  name: string;
  version: string;
  running: boolean;
  lastSync?: Date;
  lastWebhook?: Date;
  syncedResources: Record<string, number>;
  errors: string[];
}

export interface RetryConfig {
  maxRetries: number;
  baseDelay: number;
  maxDelay: number;
  backoffMultiplier: number;
}

export const DEFAULT_RETRY_CONFIG: RetryConfig = {
  maxRetries: 3,
  baseDelay: 1000,
  maxDelay: 30000,
  backoffMultiplier: 2,
};

// Re-export multi-app types from app-context for convenience
export type { AppContext, AccountConfig, MultiAppConfig } from './app-context.js';
