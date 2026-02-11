/**
 * Social Plugin Server
 * HTTP server for webhooks and API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { SocialDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import { SocialWebhookHandler } from './webhooks.js';

const logger = createLogger('social:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new SocialDatabase();
  const webhookHandler = new SocialWebhookHandler(db);

  // Connect to database
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB for large payloads
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

  /** Extract scoped SocialDatabase from request */
  function scopedDb(request: unknown): SocialDatabase {
    return (request as Record<string, unknown>).scopedDb as SocialDatabase;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'social', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'social', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'social',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'social',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        posts: stats.posts,
        comments: stats.comments,
        reactions: stats.reactions,
        lastUpdated: stats.lastUpdatedAt,
      },
      timestamp: new Date().toISOString(),
    };
  });

  app.get('/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'social',
      version: '1.0.0',
      status: 'running',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Webhook Endpoint
  // =========================================================================

  app.post('/webhooks/social', async (request, reply) => {
    try {
      const event = request.body as { type: string; data: Record<string, unknown>; timestamp: string };

      if (!event.type || !event.data) {
        return reply.status(400).send({ error: 'Invalid webhook payload' });
      }

      await webhookHandler.handle(event);
      return { received: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Webhook processing failed', { error: message });
      return reply.status(500).send({ error: 'Processing failed' });
    }
  });

  // =========================================================================
  // Post Endpoints
  // =========================================================================

  app.post('/v1/posts', async (request, reply) => {
    try {
      const input = request.body as Record<string, unknown>;

      if (!input.author_id) {
        return reply.status(400).send({ error: 'author_id is required' });
      }

      if (input.content && typeof input.content === 'string' && input.content.length > fullConfig.maxPostLength) {
        return reply.status(400).send({
          error: `Content exceeds maximum length of ${fullConfig.maxPostLength} characters`,
        });
      }

      const post = await scopedDb(request).createPost(input as never);
      return reply.status(201).send(post);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create post', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/posts', async (request) => {
    const { author, hashtag, visibility, limit = 100, offset = 0 } = request.query as {
      author?: string;
      hashtag?: string;
      visibility?: string;
      limit?: number;
      offset?: number;
    };

    const posts = await scopedDb(request).listPosts({
      author_id: author,
      hashtag,
      visibility: visibility as never,
      limit,
      offset,
    });

    const total = await scopedDb(request).countPosts();

    return { data: posts, total, limit, offset };
  });

  app.get('/v1/posts/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const post = await scopedDb(request).getPost(id);

    if (!post) {
      return reply.status(404).send({ error: 'Post not found' });
    }

    return post;
  });

  app.put('/v1/posts/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const input = request.body as Record<string, unknown>;

    // Check edit window
    const existingPost = await scopedDb(request).getPost(id);
    if (!existingPost) {
      return reply.status(404).send({ error: 'Post not found' });
    }

    const now = new Date();
    const createdAt = new Date(existingPost.created_at);
    const editWindowMs = fullConfig.editWindowMinutes * 60 * 1000;
    const timeSinceCreation = now.getTime() - createdAt.getTime();

    if (timeSinceCreation > editWindowMs) {
      return reply.status(403).send({
        error: `Edit window of ${fullConfig.editWindowMinutes} minutes has expired`,
      });
    }

    if (input.content && typeof input.content === 'string' && input.content.length > fullConfig.maxPostLength) {
      return reply.status(400).send({
        error: `Content exceeds maximum length of ${fullConfig.maxPostLength} characters`,
      });
    }

    const post = await scopedDb(request).updatePost(id, input as never);

    if (!post) {
      return reply.status(404).send({ error: 'Post not found' });
    }

    return post;
  });

  app.delete('/v1/posts/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deletePost(id);

    if (!deleted) {
      return reply.status(404).send({ error: 'Post not found' });
    }

    return { success: true };
  });

  app.get('/v1/posts/:id/comments', async (request) => {
    const { id } = request.params as { id: string };
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };

    const comments = await scopedDb(request).listComments({
      target_type: 'post',
      target_id: id,
      limit,
      offset,
    });

    return { data: comments, limit, offset };
  });

  // =========================================================================
  // Comment Endpoints
  // =========================================================================

  app.post('/v1/comments', async (request, reply) => {
    try {
      const input = request.body as Record<string, unknown>;

      if (!input.target_type || !input.target_id || !input.author_id || !input.content) {
        return reply.status(400).send({
          error: 'target_type, target_id, author_id, and content are required',
        });
      }

      if (typeof input.content === 'string' && input.content.length > fullConfig.maxCommentLength) {
        return reply.status(400).send({
          error: `Content exceeds maximum length of ${fullConfig.maxCommentLength} characters`,
        });
      }

      // Check parent comment depth if parent_id provided
      if (input.parent_id) {
        const parent = await scopedDb(request).getComment(input.parent_id as string);
        if (!parent) {
          return reply.status(404).send({ error: 'Parent comment not found' });
        }

        if (parent.depth >= fullConfig.maxCommentDepth - 1) {
          return reply.status(400).send({
            error: `Maximum comment depth of ${fullConfig.maxCommentDepth} reached`,
          });
        }
      }

      const comment = await scopedDb(request).createComment(input as never);
      return reply.status(201).send(comment);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create comment', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/comments/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const comment = await scopedDb(request).getComment(id);

    if (!comment) {
      return reply.status(404).send({ error: 'Comment not found' });
    }

    // Get replies
    const replies = await scopedDb(request).listComments({
      parent_id: id,
    });

    return { ...comment, replies };
  });

  app.put('/v1/comments/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const input = request.body as Record<string, unknown>;

    // Check edit window
    const existingComment = await scopedDb(request).getComment(id);
    if (!existingComment) {
      return reply.status(404).send({ error: 'Comment not found' });
    }

    const now = new Date();
    const createdAt = new Date(existingComment.created_at);
    const editWindowMs = fullConfig.editWindowMinutes * 60 * 1000;
    const timeSinceCreation = now.getTime() - createdAt.getTime();

    if (timeSinceCreation > editWindowMs) {
      return reply.status(403).send({
        error: `Edit window of ${fullConfig.editWindowMinutes} minutes has expired`,
      });
    }

    if (input.content && typeof input.content === 'string' && input.content.length > fullConfig.maxCommentLength) {
      return reply.status(400).send({
        error: `Content exceeds maximum length of ${fullConfig.maxCommentLength} characters`,
      });
    }

    const comment = await scopedDb(request).updateComment(id, input as never);

    if (!comment) {
      return reply.status(404).send({ error: 'Comment not found' });
    }

    return comment;
  });

  app.delete('/v1/comments/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteComment(id);

    if (!deleted) {
      return reply.status(404).send({ error: 'Comment not found' });
    }

    return { success: true };
  });

  // =========================================================================
  // Reaction Endpoints
  // =========================================================================

  app.post('/v1/reactions', async (request, reply) => {
    try {
      const input = request.body as Record<string, unknown>;

      if (!input.target_type || !input.target_id || !input.user_id || !input.reaction_type) {
        return reply.status(400).send({
          error: 'target_type, target_id, user_id, and reaction_type are required',
        });
      }

      if (!fullConfig.reactionsAllowed.includes(input.reaction_type as string)) {
        return reply.status(400).send({
          error: `Invalid reaction_type. Allowed: ${fullConfig.reactionsAllowed.join(', ')}`,
        });
      }

      const reaction = await scopedDb(request).addReaction(input as never);
      return reply.status(201).send(reaction);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to add reaction', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete('/v1/reactions', async (request, reply) => {
    const { target_type, target_id, user_id, reaction_type } = request.query as {
      target_type?: string;
      target_id?: string;
      user_id?: string;
      reaction_type?: string;
    };

    if (!target_type || !target_id || !user_id) {
      return reply.status(400).send({
        error: 'target_type, target_id, and user_id are required',
      });
    }

    const deleted = await scopedDb(request).removeReaction(target_type, target_id, user_id, reaction_type);

    if (!deleted) {
      return reply.status(404).send({ error: 'Reaction not found' });
    }

    return { success: true };
  });

  app.get('/v1/reactions', async (request) => {
    const { target_type, target_id, user_id, reaction_type } = request.query as {
      target_type?: string;
      target_id?: string;
      user_id?: string;
      reaction_type?: string;
    };

    const reactions = await scopedDb(request).getReactions({
      target_type,
      target_id,
      user_id,
      reaction_type,
    });

    return { data: reactions };
  });

  // =========================================================================
  // Follow Endpoints
  // =========================================================================

  app.post('/v1/follows', async (request, reply) => {
    try {
      const input = request.body as Record<string, unknown>;

      if (!input.follower_id || !input.following_type || !input.following_id) {
        return reply.status(400).send({
          error: 'follower_id, following_type, and following_id are required',
        });
      }

      if (!['user', 'tag', 'category'].includes(input.following_type as string)) {
        return reply.status(400).send({
          error: 'following_type must be one of: user, tag, category',
        });
      }

      const follow = await scopedDb(request).createFollow(input as never);
      return reply.status(201).send(follow);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create follow', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete('/v1/follows', async (request, reply) => {
    const { follower_id, following_type, following_id } = request.query as {
      follower_id?: string;
      following_type?: string;
      following_id?: string;
    };

    if (!follower_id || !following_type || !following_id) {
      return reply.status(400).send({
        error: 'follower_id, following_type, and following_id are required',
      });
    }

    const deleted = await scopedDb(request).deleteFollow(follower_id, following_type, following_id);

    if (!deleted) {
      return reply.status(404).send({ error: 'Follow relationship not found' });
    }

    return { success: true };
  });

  app.get('/v1/follows/followers/:userId', async (request) => {
    const { userId } = request.params as { userId: string };

    const follows = await scopedDb(request).listFollows({
      following_type: 'user',
      following_id: userId,
    });

    return { data: follows, total: follows.length };
  });

  app.get('/v1/follows/following/:userId', async (request) => {
    const { userId } = request.params as { userId: string };

    const follows = await scopedDb(request).listFollows({
      follower_id: userId,
    });

    return { data: follows, total: follows.length };
  });

  app.get('/v1/follows/check', async (request) => {
    const { follower_id, following_type, following_id } = request.query as {
      follower_id?: string;
      following_type?: string;
      following_id?: string;
    };

    if (!follower_id || !following_type || !following_id) {
      return { following: false };
    }

    const following = await scopedDb(request).checkFollow(follower_id, following_type, following_id);

    return { following };
  });

  // =========================================================================
  // Bookmark Endpoints
  // =========================================================================

  app.post('/v1/bookmarks', async (request, reply) => {
    try {
      const input = request.body as Record<string, unknown>;

      if (!input.user_id || !input.target_type || !input.target_id) {
        return reply.status(400).send({
          error: 'user_id, target_type, and target_id are required',
        });
      }

      const bookmark = await scopedDb(request).createBookmark(input as never);
      return reply.status(201).send(bookmark);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create bookmark', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete('/v1/bookmarks', async (request, reply) => {
    const { user_id, target_type, target_id } = request.query as {
      user_id?: string;
      target_type?: string;
      target_id?: string;
    };

    if (!user_id || !target_type || !target_id) {
      return reply.status(400).send({
        error: 'user_id, target_type, and target_id are required',
      });
    }

    const deleted = await scopedDb(request).deleteBookmark(user_id, target_type, target_id);

    if (!deleted) {
      return reply.status(404).send({ error: 'Bookmark not found' });
    }

    return { success: true };
  });

  app.get('/v1/bookmarks', async (request) => {
    const { user_id, target_type, collection, limit = 100, offset = 0 } = request.query as {
      user_id?: string;
      target_type?: string;
      collection?: string;
      limit?: number;
      offset?: number;
    };

    const bookmarks = await scopedDb(request).listBookmarks({
      user_id,
      target_type,
      collection,
      limit,
      offset,
    });

    return { data: bookmarks, limit, offset };
  });

  // =========================================================================
  // Share Endpoints
  // =========================================================================

  app.post('/v1/shares', async (request, reply) => {
    try {
      const input = request.body as Record<string, unknown>;

      if (!input.user_id || !input.target_type || !input.target_id || !input.share_type) {
        return reply.status(400).send({
          error: 'user_id, target_type, target_id, and share_type are required',
        });
      }

      if (!['repost', 'quote'].includes(input.share_type as string)) {
        return reply.status(400).send({
          error: 'share_type must be one of: repost, quote',
        });
      }

      const share = await scopedDb(request).createShare(input as never);
      return reply.status(201).send(share);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create share', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // User Profile Endpoints
  // =========================================================================

  app.get('/v1/users/:userId/profile', async (request) => {
    const { userId } = request.params as { userId: string };

    const profile = await scopedDb(request).getUserProfile(userId);

    return profile;
  });

  // =========================================================================
  // Trending Endpoints
  // =========================================================================

  app.get('/v1/trending', async (request) => {
    const { limit = 10 } = request.query as { limit?: number };

    const trending = await scopedDb(request).getTrendingHashtags(limit);

    return { data: trending };
  });

  // Start server
  const start = async () => {
    try {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.info(`Social plugin server listening on ${fullConfig.host}:${fullConfig.port}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start server', { error: message });
      process.exit(1);
    }
  };

  return { app, start, db, webhookHandler };
}
