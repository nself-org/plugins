/**
 * Knowledge Base Plugin Server
 * HTTP server for documentation, FAQ, search, and analytics API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { KBDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateDocumentRequest,
  UpdateDocumentRequest,
  CreateCollectionRequest,
  UpdateCollectionRequest,
  CreateFaqRequest,
  UpdateFaqRequest,
  CreateCommentRequest,
  UpdateCommentRequest,
  TrackAnalyticsEventRequest,
  CreateTranslationRequest,
  UpdateTranslationRequest,
  CreateReviewRequestRequest,
  DocumentStatus,
  DocumentType,
  Visibility,
  ReviewStatus,
} from './types.js';

const logger = createLogger('knowledge-base:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  const db = new KBDatabase();
  await db.connect();
  await db.initializeSchema();

  const app = Fastify({
    logger: false,
    bodyLimit: fullConfig.maxDocumentSize,
  });

  await app.register(cors, { origin: true, credentials: true });

  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 500,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );

  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): KBDatabase {
    return (request as Record<string, unknown>).scopedDb as KBDatabase;
  }

  // =========================================================================
  // Health Checks
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'knowledge-base', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'knowledge-base', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({ ready: false, plugin: 'knowledge-base', error: 'Database unavailable', timestamp: new Date().toISOString() });
    }
  });

  app.get('/live', async () => {

    return {
      alive: true,
      plugin: 'knowledge-base',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Documents
  // =========================================================================

  app.get('/api/kb/workspaces/:workspaceId/documents', async (request) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const { limit = 100, offset = 0, collection_id, status, document_type, language, visibility, category } =
      request.query as Record<string, string | undefined>;
    const sdb = scopedDb(request);
    const docs = await sdb.listDocuments(workspaceId, Number(limit), Number(offset), {
      collection_id, status: status as DocumentStatus, document_type: document_type as DocumentType,
      language, visibility: visibility as Visibility, category,
    });
    return { data: docs, limit: Number(limit), offset: Number(offset) };
  });

  app.post('/api/kb/workspaces/:workspaceId/documents', async (request, reply) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const body = request.body as Omit<CreateDocumentRequest, 'workspace_id'>;
    if (!body.title || !body.slug || !body.content || !body.created_by) {
      return reply.status(400).send({ error: 'title, slug, content, and created_by are required' });
    }
    const sdb = scopedDb(request);
    const id = await sdb.createDocument({ ...body, workspace_id: workspaceId });
    return { success: true, id };
  });

  app.get('/api/kb/workspaces/:workspaceId/documents/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const doc = await scopedDb(request).getDocument(id);
    if (!doc) return reply.status(404).send({ error: 'Document not found' });
    return doc;
  });

  app.get('/api/kb/workspaces/:workspaceId/documents/slug/:slug', async (request, reply) => {
    const { workspaceId, slug } = request.params as { workspaceId: string; slug: string };
    const { version } = request.query as { version?: string };
    const doc = await scopedDb(request).getDocumentBySlug(workspaceId, slug, version ? Number(version) : undefined);
    if (!doc) return reply.status(404).send({ error: 'Document not found' });
    return doc;
  });

  app.put('/api/kb/workspaces/:workspaceId/documents/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const body = request.body as UpdateDocumentRequest;
    const updated = await scopedDb(request).updateDocument(id, body);
    if (!updated) return reply.status(404).send({ error: 'Document not found' });
    return { success: true };
  });

  app.delete('/api/kb/workspaces/:workspaceId/documents/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteDocument(id);
    if (!deleted) return reply.status(404).send({ error: 'Document not found' });
    return { success: true };
  });

  app.post('/api/kb/workspaces/:workspaceId/documents/:id/publish', async (request, reply) => {
    const { id } = request.params as { id: string };
    const published = await scopedDb(request).publishDocument(id);
    if (!published) return reply.status(404).send({ error: 'Document not found' });
    return { success: true };
  });

  app.post('/api/kb/workspaces/:workspaceId/documents/:id/archive', async (request, reply) => {
    const { id } = request.params as { id: string };
    const archived = await scopedDb(request).archiveDocument(id);
    if (!archived) return reply.status(404).send({ error: 'Document not found' });
    return { success: true };
  });

  app.post('/api/kb/workspaces/:workspaceId/documents/:id/version', async (request, reply) => {
    const { id } = request.params as { id: string };
    const { user_id } = request.body as { user_id: string };
    if (!user_id) return reply.status(400).send({ error: 'user_id is required' });
    const newId = await scopedDb(request).createDocumentVersion(id, user_id);
    if (!newId) return reply.status(404).send({ error: 'Document not found' });
    return { success: true, id: newId };
  });

  app.get('/api/kb/workspaces/:workspaceId/documents/:id/versions', async (request) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const doc = await scopedDb(request).getDocument((request.params as { id: string }).id);
    if (!doc) return { data: [] };
    const versions = await scopedDb(request).getDocumentVersions(workspaceId, doc.slug);
    return { data: versions };
  });

  app.post('/api/kb/workspaces/:workspaceId/documents/:id/rate', async (request, reply) => {
    const { id } = request.params as { id: string };
    const { helpful } = request.body as { helpful: boolean };
    if (helpful === undefined) return reply.status(400).send({ error: 'helpful is required' });
    await scopedDb(request).rateDocument(id, helpful);
    return { success: true };
  });

  // =========================================================================
  // Search
  // =========================================================================

  app.get('/api/kb/workspaces/:workspaceId/search', async (request, reply) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const { q, limit = '20', offset = '0' } = request.query as { q?: string; limit?: string; offset?: string };
    if (!q) return reply.status(400).send({ error: 'q query parameter is required' });
    const sdb = scopedDb(request);
    const results = await sdb.searchDocuments(workspaceId, q, Number(limit), Number(offset));
    // Track search event
    await sdb.trackAnalyticsEvent({ workspace_id: workspaceId, event_type: 'search', search_query: q });
    return { data: results, query: q };
  });

  // =========================================================================
  // Collections
  // =========================================================================

  app.get('/api/kb/workspaces/:workspaceId/collections', async (request) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const { parent_id, visibility } = request.query as { parent_id?: string; visibility?: string };
    const colls = await scopedDb(request).listCollections(workspaceId, parent_id, visibility as Visibility);
    return { data: colls };
  });

  app.post('/api/kb/workspaces/:workspaceId/collections', async (request, reply) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const body = request.body as Omit<CreateCollectionRequest, 'workspace_id'>;
    if (!body.name || !body.slug || !body.created_by) {
      return reply.status(400).send({ error: 'name, slug, and created_by are required' });
    }
    const id = await scopedDb(request).createCollection({ ...body, workspace_id: workspaceId });
    return { success: true, id };
  });

  app.get('/api/kb/workspaces/:workspaceId/collections/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const coll = await scopedDb(request).getCollection(id);
    if (!coll) return reply.status(404).send({ error: 'Collection not found' });
    return coll;
  });

  app.put('/api/kb/workspaces/:workspaceId/collections/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const body = request.body as UpdateCollectionRequest;
    const updated = await scopedDb(request).updateCollection(id, body);
    if (!updated) return reply.status(404).send({ error: 'Collection not found' });
    return { success: true };
  });

  app.delete('/api/kb/workspaces/:workspaceId/collections/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteCollection(id);
    if (!deleted) return reply.status(404).send({ error: 'Collection not found' });
    return { success: true };
  });

  app.get('/api/kb/workspaces/:workspaceId/collections/:id/documents', async (request) => {
    const { workspaceId, id } = request.params as { workspaceId: string; id: string };
    const { limit = '100', offset = '0' } = request.query as { limit?: string; offset?: string };
    const docs = await scopedDb(request).listDocuments(workspaceId, Number(limit), Number(offset), { collection_id: id });
    return { data: docs };
  });

  // =========================================================================
  // FAQs
  // =========================================================================

  app.get('/api/kb/workspaces/:workspaceId/faqs', async (request) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const { limit = '100', offset = '0', collection_id, category, status, language } =
      request.query as Record<string, string | undefined>;
    const faqs = await scopedDb(request).listFaqs(workspaceId, Number(limit), Number(offset), {
      collection_id, category, status: status as DocumentStatus, language,
    });
    return { data: faqs, limit: Number(limit), offset: Number(offset) };
  });

  app.post('/api/kb/workspaces/:workspaceId/faqs', async (request, reply) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const body = request.body as Omit<CreateFaqRequest, 'workspace_id'>;
    if (!body.question || !body.answer || !body.created_by) {
      return reply.status(400).send({ error: 'question, answer, and created_by are required' });
    }
    const id = await scopedDb(request).createFaq({ ...body, workspace_id: workspaceId });
    return { success: true, id };
  });

  app.get('/api/kb/workspaces/:workspaceId/faqs/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const faq = await scopedDb(request).getFaq(id);
    if (!faq) return reply.status(404).send({ error: 'FAQ not found' });
    return faq;
  });

  app.put('/api/kb/workspaces/:workspaceId/faqs/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const body = request.body as UpdateFaqRequest;
    const updated = await scopedDb(request).updateFaq(id, body);
    if (!updated) return reply.status(404).send({ error: 'FAQ not found' });
    return { success: true };
  });

  app.delete('/api/kb/workspaces/:workspaceId/faqs/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteFaq(id);
    if (!deleted) return reply.status(404).send({ error: 'FAQ not found' });
    return { success: true };
  });

  // =========================================================================
  // Comments
  // =========================================================================

  app.get('/api/kb/workspaces/:workspaceId/documents/:documentId/comments', async (request) => {
    const { documentId } = request.params as { documentId: string };
    const { limit = '100', offset = '0', status } = request.query as { limit?: string; offset?: string; status?: string };
    const comments = await scopedDb(request).listComments(documentId, Number(limit), Number(offset), status);
    return { data: comments };
  });

  app.post('/api/kb/workspaces/:workspaceId/documents/:documentId/comments', async (request, reply) => {
    const { workspaceId, documentId } = request.params as { workspaceId: string; documentId: string };
    const body = request.body as Omit<CreateCommentRequest, 'workspace_id' | 'document_id'>;
    if (!body.user_id || !body.content) {
      return reply.status(400).send({ error: 'user_id and content are required' });
    }
    const id = await scopedDb(request).createComment({ ...body, workspace_id: workspaceId, document_id: documentId });
    return { success: true, id };
  });

  app.put('/api/kb/workspaces/:workspaceId/comments/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const body = request.body as UpdateCommentRequest;
    const updated = await scopedDb(request).updateComment(id, body);
    if (!updated) return reply.status(404).send({ error: 'Comment not found' });
    return { success: true };
  });

  app.delete('/api/kb/workspaces/:workspaceId/comments/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteComment(id);
    if (!deleted) return reply.status(404).send({ error: 'Comment not found' });
    return { success: true };
  });

  app.post('/api/kb/workspaces/:workspaceId/comments/:id/helpful', async (request, reply) => {
    const { id } = request.params as { id: string };
    const marked = await scopedDb(request).markCommentHelpful(id);
    if (!marked) return reply.status(404).send({ error: 'Comment not found' });
    return { success: true };
  });

  // =========================================================================
  // Analytics
  // =========================================================================

  app.get('/api/kb/workspaces/:workspaceId/stats', async (request) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const stats = await scopedDb(request).getStats(workspaceId);
    return stats;
  });

  app.get('/api/kb/workspaces/:workspaceId/documents/:id/analytics', async (request) => {
    const { id } = request.params as { id: string };
    const { start_date, end_date } = request.query as { start_date?: string; end_date?: string };
    const analytics = await scopedDb(request).getDocumentAnalytics(
      id,
      start_date ? new Date(start_date) : undefined,
      end_date ? new Date(end_date) : undefined
    );
    return { data: analytics };
  });

  app.get('/api/kb/workspaces/:workspaceId/popular-searches', async (request) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const { limit = '20' } = request.query as { limit?: string };
    const searches = await scopedDb(request).getPopularSearches(workspaceId, Number(limit));
    return { data: searches };
  });

  app.post('/api/kb/workspaces/:workspaceId/events', async (request, reply) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const body = request.body as Omit<TrackAnalyticsEventRequest, 'workspace_id'>;
    if (!body.event_type) return reply.status(400).send({ error: 'event_type is required' });
    const id = await scopedDb(request).trackAnalyticsEvent({ ...body, workspace_id: workspaceId });
    return { success: true, id };
  });

  // =========================================================================
  // Translations
  // =========================================================================

  app.get('/api/kb/workspaces/:workspaceId/translations', async (request) => {
    const { document_id, faq_id, language } = request.query as { document_id?: string; faq_id?: string; language?: string };
    const translations = await scopedDb(request).listTranslations({ document_id, faq_id, language });
    return { data: translations };
  });

  app.post('/api/kb/workspaces/:workspaceId/translations', async (request, reply) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const body = request.body as Omit<CreateTranslationRequest, 'workspace_id'>;
    if (!body.language) return reply.status(400).send({ error: 'language is required' });
    const id = await scopedDb(request).createTranslation({ ...body, workspace_id: workspaceId });
    return { success: true, id };
  });

  app.put('/api/kb/workspaces/:workspaceId/translations/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const body = request.body as UpdateTranslationRequest;
    const updated = await scopedDb(request).updateTranslation(id, body);
    if (!updated) return reply.status(404).send({ error: 'Translation not found' });
    return { success: true };
  });

  // =========================================================================
  // Review Requests
  // =========================================================================

  app.get('/api/kb/workspaces/:workspaceId/reviews', async (request) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const { status } = request.query as { status?: string };
    const reviews = await scopedDb(request).listReviewRequests(workspaceId, status as ReviewStatus);
    return { data: reviews };
  });

  app.post('/api/kb/workspaces/:workspaceId/reviews', async (request, reply) => {
    const { workspaceId } = request.params as { workspaceId: string };
    const body = request.body as Omit<CreateReviewRequestRequest, 'workspace_id'>;
    if (!body.document_id || !body.requested_by) {
      return reply.status(400).send({ error: 'document_id and requested_by are required' });
    }
    const id = await scopedDb(request).createReviewRequest({ ...body, workspace_id: workspaceId });
    return { success: true, id };
  });

  app.post('/api/kb/workspaces/:workspaceId/reviews/:id/complete', async (request, reply) => {
    const { id } = request.params as { id: string };
    const { approved, notes } = request.body as { approved: boolean; notes?: string };
    if (approved === undefined) return reply.status(400).send({ error: 'approved is required' });
    const completed = await scopedDb(request).completeReview(id, approved, notes);
    if (!completed) return reply.status(404).send({ error: 'Review request not found' });
    return { success: true };
  });

  // =========================================================================
  // Status
  // =========================================================================

  app.get('/v1/status', async () => {
    return {
      plugin: 'knowledge-base',
      version: '1.0.0',
      status: 'running',
      timestamp: new Date().toISOString(),
    };
  });

  // Graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down server...');
    await app.close();
    await db.disconnect();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return { app, db, config: fullConfig, shutdown };
}

export async function startServer(config?: Partial<Config>): Promise<void> {
  const { app, config: fullConfig } = await createServer(config);
  await app.listen({ port: fullConfig.port, host: fullConfig.host });
  logger.success(`Knowledge Base plugin listening on ${fullConfig.host}:${fullConfig.port}`);
}
