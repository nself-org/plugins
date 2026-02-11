/**
 * AI Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';
import type { AiProvider } from './types.js';

export interface Config {
  // Server
  port: number;
  host: string;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // Providers
  openaiEnabled: boolean;
  openaiApiKey: string;
  openaiOrgId: string;
  openaiDefaultModel: string;

  anthropicEnabled: boolean;
  anthropicApiKey: string;
  anthropicDefaultModel: string;

  googleEnabled: boolean;
  googleApiKey: string;
  googleDefaultModel: string;

  cohereEnabled: boolean;
  cohereApiKey: string;

  localEnabled: boolean;
  localBaseUrl: string;
  localDefaultModel: string;

  // Defaults
  defaultProvider: AiProvider;
  defaultTemperature: number;
  defaultMaxTokens: number;
  enableStreaming: boolean;
  enableFunctionCalling: boolean;

  // Embeddings
  embeddingsEnabled: boolean;
  embeddingsModel: string;
  embeddingsDimensions: number;

  // Rate limiting
  rateLimitEnabled: boolean;
  rateLimitRequestsPerMinute: number;
  rateLimitTokensPerMinute: number;

  // Quotas
  defaultDailyRequests: number;
  defaultDailyTokens: number;
  defaultDailyCost: number;

  // Features
  chatAssistantEnabled: boolean;
  summarizationEnabled: boolean;
  translationEnabled: boolean;
  sentimentAnalysisEnabled: boolean;
  smartSearchEnabled: boolean;

  // Caching
  cacheEnabled: boolean;
  cacheTtlSeconds: number;

  // Monitoring
  logRequests: boolean;
  logResponses: boolean;
  trackCosts: boolean;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

function parseProvider(value: string | undefined): AiProvider {
  const normalized = (value || 'openai').toLowerCase();
  const valid: AiProvider[] = ['openai', 'anthropic', 'google', 'cohere', 'huggingface', 'local', 'custom'];
  if (valid.includes(normalized as AiProvider)) {
    return normalized as AiProvider;
  }
  throw new Error(`Invalid AI provider: ${value}. Must be one of: ${valid.join(', ')}`);
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('AI');

  const config: Config = {
    // Server
    port: parseInt(process.env.AI_PLUGIN_PORT ?? process.env.PORT ?? '3705', 10),
    host: process.env.AI_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Providers
    openaiEnabled: process.env.AI_OPENAI_ENABLED === 'true',
    openaiApiKey: process.env.AI_OPENAI_API_KEY ?? '',
    openaiOrgId: process.env.AI_OPENAI_ORG_ID ?? '',
    openaiDefaultModel: process.env.AI_OPENAI_DEFAULT_MODEL ?? 'gpt-4-turbo',

    anthropicEnabled: process.env.AI_ANTHROPIC_ENABLED === 'true',
    anthropicApiKey: process.env.AI_ANTHROPIC_API_KEY ?? '',
    anthropicDefaultModel: process.env.AI_ANTHROPIC_DEFAULT_MODEL ?? 'claude-3-opus-20240229',

    googleEnabled: process.env.AI_GOOGLE_ENABLED === 'true',
    googleApiKey: process.env.AI_GOOGLE_API_KEY ?? '',
    googleDefaultModel: process.env.AI_GOOGLE_DEFAULT_MODEL ?? 'gemini-pro',

    cohereEnabled: process.env.AI_COHERE_ENABLED === 'true',
    cohereApiKey: process.env.AI_COHERE_API_KEY ?? '',

    localEnabled: process.env.AI_LOCAL_ENABLED === 'true',
    localBaseUrl: process.env.AI_LOCAL_BASE_URL ?? 'http://localhost:11434',
    localDefaultModel: process.env.AI_LOCAL_DEFAULT_MODEL ?? 'llama2',

    // Defaults
    defaultProvider: parseProvider(process.env.AI_DEFAULT_PROVIDER),
    defaultTemperature: parseFloat(process.env.AI_DEFAULT_TEMPERATURE ?? '0.7'),
    defaultMaxTokens: parseInt(process.env.AI_DEFAULT_MAX_TOKENS ?? '1000', 10),
    enableStreaming: process.env.AI_ENABLE_STREAMING !== 'false',
    enableFunctionCalling: process.env.AI_ENABLE_FUNCTION_CALLING !== 'false',

    // Embeddings
    embeddingsEnabled: process.env.AI_EMBEDDINGS_ENABLED !== 'false',
    embeddingsModel: process.env.AI_EMBEDDINGS_MODEL ?? 'text-embedding-3-large',
    embeddingsDimensions: parseInt(process.env.AI_EMBEDDINGS_DIMENSIONS ?? '1536', 10),

    // Rate limiting
    rateLimitEnabled: process.env.AI_RATE_LIMIT_ENABLED !== 'false',
    rateLimitRequestsPerMinute: parseInt(process.env.AI_RATE_LIMIT_REQUESTS_PER_MINUTE ?? '60', 10),
    rateLimitTokensPerMinute: parseInt(process.env.AI_RATE_LIMIT_TOKENS_PER_MINUTE ?? '90000', 10),

    // Quotas
    defaultDailyRequests: parseInt(process.env.AI_DEFAULT_DAILY_REQUESTS ?? '1000', 10),
    defaultDailyTokens: parseInt(process.env.AI_DEFAULT_DAILY_TOKENS ?? '100000', 10),
    defaultDailyCost: parseFloat(process.env.AI_DEFAULT_DAILY_COST ?? '10.00'),

    // Features
    chatAssistantEnabled: process.env.AI_CHAT_ASSISTANT_ENABLED !== 'false',
    summarizationEnabled: process.env.AI_SUMMARIZATION_ENABLED !== 'false',
    translationEnabled: process.env.AI_TRANSLATION_ENABLED !== 'false',
    sentimentAnalysisEnabled: process.env.AI_SENTIMENT_ANALYSIS_ENABLED === 'true',
    smartSearchEnabled: process.env.AI_SMART_SEARCH_ENABLED !== 'false',

    // Caching
    cacheEnabled: process.env.AI_CACHE_ENABLED !== 'false',
    cacheTtlSeconds: parseInt(process.env.AI_CACHE_TTL_SECONDS ?? '3600', 10),

    // Monitoring
    logRequests: process.env.AI_LOG_REQUESTS !== 'false',
    logResponses: process.env.AI_LOG_RESPONSES === 'true',
    trackCosts: process.env.AI_TRACK_COSTS !== 'false',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (config.defaultTemperature < 0 || config.defaultTemperature > 2) {
    throw new Error('AI_DEFAULT_TEMPERATURE must be between 0 and 2');
  }

  if (config.defaultMaxTokens < 1) {
    throw new Error('AI_DEFAULT_MAX_TOKENS must be at least 1');
  }

  return config;
}
