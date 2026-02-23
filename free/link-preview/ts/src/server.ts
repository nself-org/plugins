/**
 * Link Preview Plugin Server
 * HTTP server for URL metadata extraction, caching, oEmbed, blocklist, and analytics
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { LinkPreviewDatabase } from './database.js';
import { loadConfig, type LinkPreviewConfig } from './config.js';
import { MetadataFetcher } from './metadata-fetcher.js';
import { OEmbedService } from './oembed-service.js';
import type {
  FetchPreviewRequest,
  BatchFetchRequest,
  CreateTemplateRequest,
  UpdateTemplateRequest,
  AddOEmbedProviderRequest,
  AddToBlocklistRequest,
  UpdateSettingsRequest,
  TrackUsageRequest,
  SettingsScope,
} from './types.js';
import { createHash } from 'crypto';

const logger = createLogger('link-preview:server');

/** Generate a SHA-256 hash of a normalized URL */
function hashUrl(url: string): string {
  const normalized = url.toLowerCase().trim();
  return createHash('sha256').update(normalized).digest('hex');
}

export async function createServer(config?: Partial<LinkPreviewConfig>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new LinkPreviewDatabase();
  await db.connect();
  await db.initializeSchema();

  const metadataFetcher = new MetadataFetcher(10000); // 10 second timeout
  const oembedService = new OEmbedService(10000); // 10 second timeout

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(fullConfig.rateLimitMax, fullConfig.rateLimitWindowMs);

  // Add rate limiting to all requests
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  // Add API key authentication (skips health check endpoints)
  if (fullConfig.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context: resolve source_account_id per request
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    const scopedDb = db.forSourceAccount(ctx.sourceAccountId);
    (request as unknown as Record<string, unknown>).scopedDb = scopedDb;
  });

  /** Extract scoped database from request */
  function scopedDb(request: unknown): LinkPreviewDatabase {
    return (request as Record<string, unknown>).scopedDb as LinkPreviewDatabase;
  }

  // =========================================================================
  // Health & Status Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'link-preview', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'link-preview', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'link-preview',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getCacheStats();
    return {
      alive: true,
      plugin: 'link-preview',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getCacheStats();
    return {
      plugin: 'link-preview',
      version: '1.0.0',
      status: 'running',
      enabled: fullConfig.enabled,
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Preview Fetching Endpoints
  // =========================================================================

  app.get('/api/link-preview', async (request, reply) => {
    const { url } = request.query as { url?: string };
    if (!url) return reply.status(400).send({ error: 'url query parameter is required' });

    // Check blocklist
    const blocked = await scopedDb(request).isUrlBlocked(url);
    if (blocked) return reply.status(403).send({ error: 'URL is blocked' });

    // Try cache first
    const cached = await scopedDb(request).getPreviewByUrl(url);
    if (cached) return cached;

    const urlHash = hashUrl(url);

    try {
      // Fetch real metadata for uncached URL
      const metadata = await metadataFetcher.fetchMetadata(url);

      const preview = await scopedDb(request).upsertPreview({
        url,
        url_hash: urlHash,
        title: metadata.title,
        description: metadata.description,
        image_url: metadata.image,
        site_name: metadata.siteName,
        author_name: metadata.author,
        published_date: metadata.publishedTime,
        content_type: metadata.type,
        favicon_url: metadata.favicon,
        language: metadata.language,
        video_url: metadata.videoUrl,
        audio_url: metadata.audioUrl,
        reading_time_minutes: metadata.estimatedReadTime,
        status: 'success',
      });

      return preview;
    } catch (error) {
      logger.error('Failed to fetch metadata', { url, error });

      // Store failed preview rather than returning a stub
      const preview = await scopedDb(request).upsertPreview({
        url,
        url_hash: urlHash,
        status: 'failed',
      });

      return preview;
    }
  });

  app.post<{ Body: FetchPreviewRequest }>('/api/link-preview/fetch', async (request, reply) => {
    const { url, force } = request.body;
    if (!url) return reply.status(400).send({ error: 'url is required' });

    // Check blocklist
    const blocked = await scopedDb(request).isUrlBlocked(url);
    if (blocked) return reply.status(403).send({ error: 'URL is blocked' });

    // If not forcing, check cache
    if (!force) {
      const cached = await scopedDb(request).getPreviewByUrl(url);
      if (cached) return cached;
    }

    const urlHash = hashUrl(url);

    try {
      // Fetch real metadata
      const metadata = await metadataFetcher.fetchMetadata(url);

      // Store in database with metadata
      const preview = await scopedDb(request).upsertPreview({
        url,
        url_hash: urlHash,
        title: metadata.title,
        description: metadata.description,
        image_url: metadata.image,
        site_name: metadata.siteName,
        author_name: metadata.author,
        published_date: metadata.publishedTime,
        content_type: metadata.type,
        favicon_url: metadata.favicon,
        language: metadata.language,
        video_url: metadata.videoUrl,
        audio_url: metadata.audioUrl,
        reading_time_minutes: metadata.estimatedReadTime,
        status: 'success',
      });

      return preview;
    } catch (error) {
      logger.error('Failed to fetch metadata', { url, error });

      // Store failed preview
      const preview = await scopedDb(request).upsertPreview({
        url,
        url_hash: urlHash,
        status: 'failed',
      });

      return preview;
    }
  });

  app.get<{ Params: { id: string } }>('/api/link-preview/:id', async (request, reply) => {
    const preview = await scopedDb(request).getPreview(request.params.id);
    if (!preview) return reply.status(404).send({ error: 'Preview not found' });
    return preview;
  });

  app.delete<{ Params: { id: string } }>('/api/link-preview/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deletePreview(request.params.id);
    if (!deleted) return reply.status(404).send({ error: 'Preview not found' });
    return { success: true };
  });

  app.post<{ Params: { id: string } }>('/api/link-preview/refresh/:id', async (request, reply) => {
    const preview = await scopedDb(request).getPreview(request.params.id);
    if (!preview) return reply.status(404).send({ error: 'Preview not found' });

    try {
      // Re-fetch metadata from URL
      const metadata = await metadataFetcher.fetchMetadata(preview.url);

      // Update database with fresh metadata
      const updated = await scopedDb(request).upsertPreview({
        url: preview.url,
        url_hash: preview.url_hash,
        title: metadata.title,
        description: metadata.description,
        image_url: metadata.image,
        site_name: metadata.siteName,
        author_name: metadata.author,
        published_date: metadata.publishedTime,
        content_type: metadata.type,
        favicon_url: metadata.favicon,
        language: metadata.language,
        video_url: metadata.videoUrl,
        audio_url: metadata.audioUrl,
        reading_time_minutes: metadata.estimatedReadTime,
        status: 'success',
      });

      return updated;
    } catch (error) {
      logger.error('Failed to refresh preview', { id: request.params.id, url: preview.url, error });

      // Update status to failed
      const updated = await scopedDb(request).upsertPreview({
        url: preview.url,
        url_hash: preview.url_hash,
        status: 'failed',
      });

      return updated;
    }
  });

  // =========================================================================
  // Batch Operations
  // =========================================================================

  app.post<{ Body: BatchFetchRequest }>('/api/link-preview/batch', async (request, reply) => {
    const { urls } = request.body;
    if (!Array.isArray(urls) || urls.length === 0) {
      return reply.status(400).send({ error: 'urls array is required and must not be empty' });
    }
    if (urls.length > 50) {
      return reply.status(400).send({ error: 'Maximum 50 URLs per batch' });
    }

    const results = [];
    for (const url of urls) {
      const blocked = await scopedDb(request).isUrlBlocked(url);
      if (blocked) {
        results.push({ url, blocked: true, preview: null });
        continue;
      }

      const cached = await scopedDb(request).getPreviewByUrl(url);
      if (cached) {
        results.push({ url, blocked: false, preview: cached });
      } else {
        const urlHash = hashUrl(url);
        const preview = await scopedDb(request).upsertPreview({
          url,
          url_hash: urlHash,
          status: 'partial',
        });
        results.push({ url, blocked: false, preview });
      }
    }

    return { data: results, total: urls.length };
  });

  app.get<{ Params: { messageId: string } }>('/api/link-preview/message/:messageId', async (request) => {
    const previews = await scopedDb(request).getPreviewsForMessage(request.params.messageId);
    return { data: previews };
  });

  // =========================================================================
  // Template Endpoints
  // =========================================================================

  app.get('/api/link-preview/templates', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const templates = await scopedDb(request).listTemplates(limit, offset);
    return { data: templates, limit, offset };
  });

  app.post<{ Body: CreateTemplateRequest }>('/api/link-preview/templates', async (request, reply) => {
    const data = request.body;
    if (!data.name || !data.url_pattern || !data.template_html) {
      return reply.status(400).send({ error: 'name, url_pattern, and template_html are required' });
    }
    try {
      const template = await scopedDb(request).createTemplate(data);
      return template;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Template creation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Params: { id: string } }>('/api/link-preview/templates/:id', async (request, reply) => {
    const template = await scopedDb(request).getTemplate(request.params.id);
    if (!template) return reply.status(404).send({ error: 'Template not found' });
    return template;
  });

  app.put<{ Params: { id: string }; Body: UpdateTemplateRequest }>('/api/link-preview/templates/:id', async (request, reply) => {
    try {
      const template = await scopedDb(request).updateTemplate(request.params.id, request.body);
      if (!template) return reply.status(404).send({ error: 'Template not found' });
      return template;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Template update failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete<{ Params: { id: string } }>('/api/link-preview/templates/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteTemplate(request.params.id);
    if (!deleted) return reply.status(404).send({ error: 'Template not found' });
    return { success: true };
  });

  app.post<{ Params: { id: string }; Body: { url: string } }>('/api/link-preview/templates/:id/test', async (request, reply) => {
    const template = await scopedDb(request).getTemplate(request.params.id);
    if (!template) return reply.status(404).send({ error: 'Template not found' });

    const { url } = request.body;
    if (!url) return reply.status(400).send({ error: 'url is required' });

    try {
      const regex = new RegExp(template.url_pattern);
      const matches = regex.test(url);
      return { matches, template_id: template.id, url, pattern: template.url_pattern };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Invalid regex pattern';
      return reply.status(400).send({ error: message });
    }
  });

  // =========================================================================
  // oEmbed Endpoints
  // =========================================================================

  app.get('/api/link-preview/oembed/providers', async (request) => {
    const dbProviders = await scopedDb(request).listOEmbedProviders();
    const builtInProviders = oembedService.getSupportedProviders();

    return {
      builtIn: builtInProviders,
      custom: dbProviders,
    };
  });

  app.post<{ Body: AddOEmbedProviderRequest }>('/api/link-preview/oembed/providers', async (request, reply) => {
    const data = request.body;
    if (!data.provider_name || !data.provider_url || !data.endpoint_url || !data.url_schemes) {
      return reply.status(400).send({
        error: 'provider_name, provider_url, endpoint_url, and url_schemes are required',
      });
    }
    try {
      const provider = await scopedDb(request).addOEmbedProvider(data);
      return provider;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('oEmbed provider creation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/link-preview/oembed/discover', async (request, reply) => {
    const { url } = request.query as { url?: string };
    if (!url) return reply.status(400).send({ error: 'url query parameter is required' });

    // Check built-in providers first
    const providerMatch = oembedService.findProvider(url);
    if (providerMatch) {
      return {
        provider_name: providerMatch.provider.name,
        endpoint_url: providerMatch.endpoint,
        url: providerMatch.provider.url,
        supported: true,
      };
    }

    // Fallback to database-configured providers
    const provider = await scopedDb(request).findOEmbedProvider(url);
    if (!provider) return reply.status(404).send({ error: 'No oEmbed provider found for this URL' });
    return provider;
  });

  app.get('/api/link-preview/oembed/fetch', async (request, reply) => {
    const { url, maxwidth, maxheight } = request.query as { url?: string; maxwidth?: string; maxheight?: string };
    if (!url) return reply.status(400).send({ error: 'url query parameter is required' });

    // Try using the built-in oEmbed service first
    const maxWidth = maxwidth ? parseInt(maxwidth, 10) : undefined;
    const maxHeight = maxheight ? parseInt(maxheight, 10) : undefined;

    const embedData = await oembedService.fetchEmbed(url, maxWidth, maxHeight);
    if (embedData) {
      return embedData;
    }

    // Fallback to database-configured providers
    const provider = await scopedDb(request).findOEmbedProvider(url);
    if (!provider) {
      return reply.status(404).send({ error: 'No oEmbed provider found for this URL' });
    }

    // Return provider info if no embed data was fetched
    return {
      provider: provider.provider_name,
      endpoint: provider.endpoint_url,
      url,
      message: 'Provider found but embed fetch failed',
    };
  });

  // =========================================================================
  // Blocklist Endpoints
  // =========================================================================

  app.get('/api/link-preview/blocklist', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const entries = await scopedDb(request).listBlocklist(limit, offset);
    return { data: entries, limit, offset };
  });

  app.post<{ Body: AddToBlocklistRequest }>('/api/link-preview/blocklist', async (request, reply) => {
    const data = request.body;
    if (!data.url_pattern || !data.pattern_type || !data.reason) {
      return reply.status(400).send({ error: 'url_pattern, pattern_type, and reason are required' });
    }
    try {
      const entry = await scopedDb(request).addToBlocklist(data);
      return entry;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Blocklist entry creation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete<{ Params: { id: string } }>('/api/link-preview/blocklist/:id', async (request, reply) => {
    const deleted = await scopedDb(request).removeFromBlocklist(request.params.id);
    if (!deleted) return reply.status(404).send({ error: 'Blocklist entry not found' });
    return { success: true };
  });

  app.post<{ Body: { url: string } }>('/api/link-preview/blocklist/check', async (request, reply) => {
    const { url } = request.body;
    if (!url) return reply.status(400).send({ error: 'url is required' });
    const blocked = await scopedDb(request).isUrlBlocked(url);
    return { url, blocked };
  });

  // =========================================================================
  // Settings Endpoints
  // =========================================================================

  app.get('/api/link-preview/settings', async (request) => {
    const { scope = 'global', scope_id } = request.query as { scope?: SettingsScope; scope_id?: string };
    const settings = await scopedDb(request).getSettings(scope, scope_id);
    return settings ?? {
      scope,
      scope_id: scope_id ?? null,
      enabled: true,
      auto_expand: false,
      show_images: true,
      show_videos: true,
      max_previews_per_message: 3,
      preview_position: 'bottom',
      blocked_domains: [],
      allowed_domains: [],
    };
  });

  app.put<{ Body: UpdateSettingsRequest }>('/api/link-preview/settings', async (request, reply) => {
    const data = request.body;
    if (!data.scope) return reply.status(400).send({ error: 'scope is required' });
    try {
      const settings = await scopedDb(request).upsertSettings(data);
      return settings;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Settings update failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Params: { id: string } }>('/api/link-preview/settings/channel/:id', async (request) => {
    const settings = await scopedDb(request).getSettings('channel', request.params.id);
    return settings ?? {
      scope: 'channel',
      scope_id: request.params.id,
      enabled: true,
      auto_expand: false,
      show_images: true,
      show_videos: true,
      max_previews_per_message: 3,
      preview_position: 'bottom',
      blocked_domains: [],
      allowed_domains: [],
    };
  });

  app.put<{ Params: { id: string }; Body: UpdateSettingsRequest }>('/api/link-preview/settings/channel/:id', async (request, reply) => {
    try {
      const settings = await scopedDb(request).upsertSettings({
        ...request.body,
        scope: 'channel',
        scope_id: request.params.id,
      });
      return settings;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Channel settings update failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Analytics Endpoints
  // =========================================================================

  app.get('/api/link-preview/analytics', async (request) => {
    const { start_date, end_date, preview_id } = request.query as {
      start_date?: string; end_date?: string; preview_id?: string;
    };

    const start = start_date ?? new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString().split('T')[0];
    const end = end_date ?? new Date().toISOString().split('T')[0];

    const analytics = await scopedDb(request).getAnalytics(start, end, preview_id);
    return { data: analytics, start_date: start, end_date: end };
  });

  app.get('/api/link-preview/popular', async (request) => {
    const { limit = 20 } = request.query as { limit?: number };
    const popular = await scopedDb(request).getPopularPreviews(limit);
    return { data: popular };
  });

  app.post<{ Params: { usageId: string } }>('/api/link-preview/click/:usageId', async (request, reply) => {
    const clicked = await scopedDb(request).recordClick(request.params.usageId);
    if (!clicked) return reply.status(404).send({ error: 'Usage record not found or already clicked' });
    return { success: true };
  });

  // =========================================================================
  // Admin Endpoints
  // =========================================================================

  app.post('/api/link-preview/admin/cache/clear', async (request) => {
    const count = await scopedDb(request).clearCache();
    return { success: true, cleared: count };
  });

  app.get('/api/link-preview/admin/stats', async (request) => {
    const stats = await scopedDb(request).getCacheStats();
    return stats;
  });

  app.post('/api/link-preview/admin/cleanup', async (request) => {
    const count = await scopedDb(request).cleanupExpiredPreviews();
    return { success: true, cleaned: count };
  });

  // =========================================================================
  // Usage Tracking Endpoint
  // =========================================================================

  app.post<{ Body: TrackUsageRequest }>('/api/link-preview/usage', async (request, reply) => {
    const data = request.body;
    if (!data.preview_id) return reply.status(400).send({ error: 'preview_id is required' });
    try {
      const usage = await scopedDb(request).trackUsage(data);
      return usage;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Usage tracking failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // Start server
  const start = async () => {
    try {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.success(`Server listening on ${fullConfig.host}:${fullConfig.port}`);
    } catch (err) {
      const error = err instanceof Error ? err.message : 'Unknown error';
      logger.error('Server failed to start', { error });
      process.exit(1);
    }
  };

  return { app, start, db };
}
