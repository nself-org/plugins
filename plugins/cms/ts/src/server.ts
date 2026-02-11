/**
 * CMS Plugin Server
 * HTTP server for content management API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { CmsDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreatePostInput,
  UpdatePostInput,
  ListPostsFilters,
  CreateCategoryInput,
  UpdateCategoryInput,
  CreateTagInput,
  CreateContentTypeInput,
  UpdateContentTypeInput,
} from './types.js';

const logger = createLogger('cms:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new CmsDatabase();

  // Connect to database
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: fullConfig.maxBodyLength,
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 200,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );

  // Add rate limiting to all requests
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  // Add API key authentication (skips health check endpoints)
  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context: resolve source_account_id per request and create scoped DB
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  /** Extract scoped CmsDatabase from request */
  function scopedDb(request: unknown): CmsDatabase {
    return (request as Record<string, unknown>).scopedDb as CmsDatabase;
  }

  // Start scheduled post processor
  const scheduledCheckInterval = setInterval(async () => {
    try {
      await db.processScheduledPosts();
    } catch (error) {
      logger.error('Failed to process scheduled posts', { error });
    }
  }, fullConfig.scheduledCheckIntervalMs);

  // Cleanup on shutdown
  app.addHook('onClose', async () => {
    clearInterval(scheduledCheckInterval);
    await db.disconnect();
  });

  // =========================================================================
  // Health & Status Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'cms', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'cms', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'cms',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'cms',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        posts: stats.posts,
        publishedPosts: stats.publishedPosts,
        lastPublished: stats.lastPublishedAt,
      },
      timestamp: new Date().toISOString(),
    };
  });

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'cms',
      version: '1.0.0',
      status: 'running',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  app.get('/v1/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    return stats;
  });

  // =========================================================================
  // Content Types Endpoints
  // =========================================================================

  app.post<{ Body: CreateContentTypeInput }>('/v1/content-types', async (request, reply) => {
    try {
      const contentType = await scopedDb(request).createContentType(request.body);
      return reply.status(201).send(contentType);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create content type', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/content-types', async (request) => {
    const contentTypes = await scopedDb(request).listContentTypes();
    return contentTypes;
  });

  app.get<{ Params: { id: string } }>('/v1/content-types/:id', async (request, reply) => {
    const contentType = await scopedDb(request).getContentType(request.params.id);
    if (!contentType) {
      return reply.status(404).send({ error: 'Content type not found' });
    }
    return contentType;
  });

  app.put<{ Params: { id: string }; Body: UpdateContentTypeInput }>('/v1/content-types/:id', async (request, reply) => {
    const contentType = await scopedDb(request).updateContentType(request.params.id, request.body);
    if (!contentType) {
      return reply.status(404).send({ error: 'Content type not found' });
    }
    return contentType;
  });

  app.delete<{ Params: { id: string } }>('/v1/content-types/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteContentType(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Content type not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Posts Endpoints
  // =========================================================================

  app.post<{ Body: CreatePostInput }>('/v1/posts', async (request, reply) => {
    try {
      const post = await scopedDb(request).createPost(request.body);
      return reply.status(201).send(post);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create post', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: ListPostsFilters }>('/v1/posts', async (request) => {
    const posts = await scopedDb(request).listPosts(request.query);
    return posts;
  });

  app.get<{ Params: { id: string } }>('/v1/posts/:id', async (request, reply) => {
    const post = await scopedDb(request).getPost(request.params.id);
    if (!post) {
      return reply.status(404).send({ error: 'Post not found' });
    }
    return post;
  });

  app.get<{ Params: { slug: string } }>('/v1/posts/slug/:slug', async (request, reply) => {
    const post = await scopedDb(request).getPostBySlug(request.params.slug);
    if (!post) {
      return reply.status(404).send({ error: 'Post not found' });
    }
    return post;
  });

  app.put<{ Params: { id: string }; Body: UpdatePostInput }>('/v1/posts/:id', async (request, reply) => {
    const post = await scopedDb(request).updatePost(request.params.id, request.body);
    if (!post) {
      return reply.status(404).send({ error: 'Post not found' });
    }
    return post;
  });

  app.delete<{ Params: { id: string } }>('/v1/posts/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deletePost(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Post not found' });
    }
    return { success: true };
  });

  // Post Actions

  app.post<{ Params: { id: string } }>('/v1/posts/:id/publish', async (request, reply) => {
    const post = await scopedDb(request).publishPost(request.params.id);
    if (!post) {
      return reply.status(404).send({ error: 'Post not found' });
    }
    return post;
  });

  app.post<{ Params: { id: string } }>('/v1/posts/:id/unpublish', async (request, reply) => {
    const post = await scopedDb(request).unpublishPost(request.params.id);
    if (!post) {
      return reply.status(404).send({ error: 'Post not found' });
    }
    return post;
  });

  app.post<{ Params: { id: string }; Body: { scheduled_at: string } }>('/v1/posts/:id/schedule', async (request, reply) => {
    const scheduledAt = new Date(request.body.scheduled_at);
    if (isNaN(scheduledAt.getTime())) {
      return reply.status(400).send({ error: 'Invalid scheduled_at date' });
    }

    const post = await scopedDb(request).schedulePost(request.params.id, scheduledAt);
    if (!post) {
      return reply.status(404).send({ error: 'Post not found' });
    }
    return post;
  });

  app.post<{ Params: { id: string } }>('/v1/posts/:id/duplicate', async (request, reply) => {
    const post = await scopedDb(request).duplicatePost(request.params.id);
    if (!post) {
      return reply.status(404).send({ error: 'Post not found' });
    }
    return reply.status(201).send(post);
  });

  // Post Versions

  app.get<{ Params: { id: string } }>('/v1/posts/:id/versions', async (request) => {
    const versions = await scopedDb(request).getPostVersions(request.params.id);
    return versions;
  });

  app.get<{ Params: { id: string; version: string } }>('/v1/posts/:id/versions/:version', async (request, reply) => {
    const version = parseInt(request.params.version, 10);
    if (isNaN(version)) {
      return reply.status(400).send({ error: 'Invalid version number' });
    }

    const versionRecord = await scopedDb(request).getPostVersion(request.params.id, version);
    if (!versionRecord) {
      return reply.status(404).send({ error: 'Version not found' });
    }
    return versionRecord;
  });

  app.post<{ Params: { id: string; version: string } }>('/v1/posts/:id/versions/:version/restore', async (request, reply) => {
    const version = parseInt(request.params.version, 10);
    if (isNaN(version)) {
      return reply.status(400).send({ error: 'Invalid version number' });
    }

    const post = await scopedDb(request).restorePostVersion(request.params.id, version);
    if (!post) {
      return reply.status(404).send({ error: 'Post or version not found' });
    }
    return post;
  });

  // Post Relations

  app.post<{ Params: { id: string }; Body: { category_ids: string[] } }>('/v1/posts/:id/categories', async (request) => {
    await scopedDb(request).setPostCategories(request.params.id, request.body.category_ids);
    return { success: true };
  });

  app.post<{ Params: { id: string }; Body: { tag_ids: string[] } }>('/v1/posts/:id/tags', async (request) => {
    await scopedDb(request).setPostTags(request.params.id, request.body.tag_ids);
    return { success: true };
  });

  // =========================================================================
  // Categories Endpoints
  // =========================================================================

  app.post<{ Body: CreateCategoryInput }>('/v1/categories', async (request, reply) => {
    try {
      const category = await scopedDb(request).createCategory(request.body);
      return reply.status(201).send(category);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create category', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/categories', async (request) => {
    const query = request.query as { tree?: string };
    const tree = query.tree === 'true';
    if (tree) {
      const categories = await scopedDb(request).getCategoryTree();
      return categories;
    }
    const categories = await scopedDb(request).listCategories();
    return categories;
  });

  app.get<{ Params: { id: string } }>('/v1/categories/:id', async (request, reply) => {
    const category = await scopedDb(request).getCategory(request.params.id);
    if (!category) {
      return reply.status(404).send({ error: 'Category not found' });
    }
    return category;
  });

  app.put<{ Params: { id: string }; Body: UpdateCategoryInput }>('/v1/categories/:id', async (request, reply) => {
    const category = await scopedDb(request).updateCategory(request.params.id, request.body);
    if (!category) {
      return reply.status(404).send({ error: 'Category not found' });
    }
    return category;
  });

  app.delete<{ Params: { id: string } }>('/v1/categories/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteCategory(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Category not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Tags Endpoints
  // =========================================================================

  app.post<{ Body: CreateTagInput }>('/v1/tags', async (request, reply) => {
    try {
      const tag = await scopedDb(request).createTag(request.body);
      return reply.status(201).send(tag);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create tag', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/tags', async (request) => {
    const tags = await scopedDb(request).listTags();
    return tags;
  });

  app.get<{ Params: { id: string } }>('/v1/tags/:id', async (request, reply) => {
    const tag = await scopedDb(request).getTag(request.params.id);
    if (!tag) {
      return reply.status(404).send({ error: 'Tag not found' });
    }
    return tag;
  });

  app.delete<{ Params: { id: string } }>('/v1/tags/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteTag(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Tag not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Feed Endpoint
  // =========================================================================

  app.get('/v1/feed', async (request, reply) => {
    const format = (request.query as { format?: string }).format ?? 'rss';
    const limit = parseInt((request.query as { limit?: string }).limit ?? '50', 10);

    const posts = await scopedDb(request).listPosts({
      status: 'published',
      limit,
    });

    if (format === 'atom') {
      const feed = generateAtomFeed(posts);
      return reply.header('Content-Type', 'application/atom+xml').send(feed);
    } else {
      const feed = generateRssFeed(posts);
      return reply.header('Content-Type', 'application/rss+xml').send(feed);
    }
  });

  // Helper functions for feed generation
  function generateRssFeed(posts: unknown[]): string {
    const items = (posts as Array<{ title: string; slug: string; excerpt: string | null; published_at: Date | null }>)
      .map(post => `
        <item>
          <title><![CDATA[${post.title}]]></title>
          <link>https://example.com/posts/${post.slug}</link>
          <description><![CDATA[${post.excerpt ?? ''}]]></description>
          <pubDate>${post.published_at?.toUTCString() ?? ''}</pubDate>
        </item>
      `)
      .join('');

    return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>CMS Feed</title>
    <link>https://example.com</link>
    <description>Latest posts</description>
    ${items}
  </channel>
</rss>`;
  }

  function generateAtomFeed(posts: unknown[]): string {
    const entries = (posts as Array<{ id: string; title: string; slug: string; excerpt: string | null; published_at: Date | null; updated_at: Date }>)
      .map(post => `
        <entry>
          <title>${post.title}</title>
          <link href="https://example.com/posts/${post.slug}" />
          <id>urn:uuid:${post.id}</id>
          <updated>${post.updated_at.toISOString()}</updated>
          <summary>${post.excerpt ?? ''}</summary>
        </entry>
      `)
      .join('');

    return `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>CMS Feed</title>
  <link href="https://example.com" />
  <updated>${new Date().toISOString()}</updated>
  <id>https://example.com</id>
  ${entries}
</feed>`;
  }

  // =========================================================================
  // Server Start
  // =========================================================================

  const serverWithStart = app as typeof app & {
    start: () => Promise<void>;
  };

  serverWithStart.start = async () => {
    try {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.info(`CMS server listening on ${fullConfig.host}:${fullConfig.port}`);
    } catch (error) {
      logger.error('Failed to start server', { error });
      throw error;
    }
  };

  return serverWithStart;
}
