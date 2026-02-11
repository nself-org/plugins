/**
 * Documents Plugin Server
 * HTTP server for document management API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { DocumentsDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateDocumentRequest,
  UpdateDocumentRequest,
  GenerateDocumentRequest,
  GeneratePreviewRequest,
  CreateTemplateRequest,
  UpdateTemplateRequest,
  CreateShareRequest,
  SearchDocumentsRequest,
  ListDocumentsQuery,
  ListTemplatesQuery,
  DocumentRecord,
  TemplateRecord,
} from './types.js';
import crypto from 'crypto';

const logger = createLogger('documents:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new DocumentsDatabase();
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024,
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

  // Multi-app context: resolve source_account_id per request
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): DocumentsDatabase {
    return (request as Record<string, unknown>).scopedDb as DocumentsDatabase;
  }

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'documents', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'documents', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'documents',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'documents',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        totalDocuments: stats.total_documents,
        totalTemplates: stats.total_templates,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Document Endpoints
  // =========================================================================

  app.post<{ Body: CreateDocumentRequest }>('/api/documents', async (request, reply) => {
    try {
      const doc = await scopedDb(request).createDocument({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        owner_id: request.body.owner_id,
        title: request.body.title,
        description: request.body.description ?? null,
        doc_type: request.body.doc_type,
        category: request.body.category ?? null,
        tags: request.body.tags ?? [],
        template_id: null,
        file_url: request.body.file_url ?? null,
        file_size_bytes: request.body.file_size_bytes ?? null,
        mime_type: request.body.mime_type ?? null,
        version: 1,
        status: 'draft',
        generated_from: null,
        metadata: request.body.metadata ?? {},
      });

      return reply.status(201).send(doc);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create document', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: ListDocumentsQuery }>('/api/documents', async (request) => {
    const documents = await scopedDb(request).listDocuments({
      ownerId: request.query.owner_id,
      docType: request.query.doc_type,
      category: request.query.category,
      status: request.query.status,
      limit: request.query.limit ? parseInt(String(request.query.limit), 10) : undefined,
      offset: request.query.offset ? parseInt(String(request.query.offset), 10) : undefined,
    });

    return { documents, count: documents.length };
  });

  app.get<{ Params: { id: string } }>('/api/documents/:id', async (request, reply) => {
    const doc = await scopedDb(request).getDocument(request.params.id);
    if (!doc) {
      return reply.status(404).send({ error: 'Document not found' });
    }

    // Also fetch versions
    const versions = await scopedDb(request).listVersions(request.params.id);
    return { ...doc, versions };
  });

  app.put<{ Params: { id: string }; Body: UpdateDocumentRequest }>('/api/documents/:id', async (request, reply) => {
    const doc = await scopedDb(request).updateDocument(
      request.params.id,
      request.body as Partial<DocumentRecord>
    );
    if (!doc) {
      return reply.status(404).send({ error: 'Document not found' });
    }
    return doc;
  });

  app.delete<{ Params: { id: string } }>('/api/documents/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteDocument(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Document not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Document Generation Endpoints
  // =========================================================================

  app.post<{ Body: GenerateDocumentRequest }>('/api/generate', async (request, reply) => {
    try {
      const body = request.body;

      // Find template
      let template: TemplateRecord | null = null;
      if (body.template_id) {
        template = await scopedDb(request).getTemplate(body.template_id);
      } else if (body.template_name) {
        template = await scopedDb(request).getTemplateByName(body.template_name);
      }

      if (!template) {
        return reply.status(404).send({ error: 'Template not found' });
      }

      // Generate HTML from template (simple Handlebars-style replacement)
      let html = template.template_content;
      for (const [key, value] of Object.entries(body.data)) {
        const regex = new RegExp(`\\{\\{\\s*${key}\\s*\\}\\}`, 'g');
        html = html.replace(regex, String(value ?? ''));
      }

      // Wrap with CSS if available
      if (template.css_content) {
        html = `<style>${template.css_content}</style>${html}`;
      }

      // Add header/footer
      if (template.header_content) {
        html = `${template.header_content}${html}`;
      }
      if (template.footer_content) {
        html = `${html}${template.footer_content}`;
      }

      const outputFormat = body.output_format ?? template.output_format;
      const mimeType = outputFormat === 'pdf' ? 'application/pdf' : 'text/html';

      // Create document record
      const doc = await scopedDb(request).createDocument({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        owner_id: body.owner_id,
        title: body.title ?? `Generated from ${template.name}`,
        description: null,
        doc_type: template.doc_type,
        category: body.category ?? null,
        tags: [],
        template_id: template.id,
        file_url: null, // Would be set after actual PDF generation
        file_size_bytes: Buffer.byteLength(html, 'utf8'),
        mime_type: mimeType,
        version: 1,
        status: 'final',
        generated_from: { template_id: template.id, data: body.data },
        metadata: {},
      });

      return reply.status(201).send({
        document_id: doc.id,
        file_url: doc.file_url,
        mime_type: mimeType,
        file_size_bytes: doc.file_size_bytes,
        html_preview: html,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to generate document', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Body: GeneratePreviewRequest }>('/api/generate/preview', async (request, reply) => {
    try {
      const template = await scopedDb(request).getTemplate(request.body.template_id);
      if (!template) {
        return reply.status(404).send({ error: 'Template not found' });
      }

      // Generate HTML from template
      let html = template.template_content;
      for (const [key, value] of Object.entries(request.body.data)) {
        const regex = new RegExp(`\\{\\{\\s*${key}\\s*\\}\\}`, 'g');
        html = html.replace(regex, String(value ?? ''));
      }

      if (template.css_content) {
        html = `<style>${template.css_content}</style>${html}`;
      }

      return { html };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to generate preview', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Template Endpoints
  // =========================================================================

  app.post<{ Body: CreateTemplateRequest }>('/api/templates', async (request, reply) => {
    try {
      const template = await scopedDb(request).createTemplate({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        name: request.body.name,
        description: request.body.description ?? null,
        doc_type: request.body.doc_type,
        output_format: request.body.output_format ?? 'pdf',
        template_engine: request.body.template_engine ?? 'handlebars',
        template_content: request.body.template_content,
        css_content: request.body.css_content ?? null,
        header_content: request.body.header_content ?? null,
        footer_content: request.body.footer_content ?? null,
        variables: request.body.variables ?? {},
        sample_data: request.body.sample_data ?? {},
        is_default: false,
        version: 1,
      });

      return reply.status(201).send(template);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create template', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: ListTemplatesQuery }>('/api/templates', async (request) => {
    const templates = await scopedDb(request).listTemplates({
      docType: request.query.doc_type,
      limit: request.query.limit ? parseInt(String(request.query.limit), 10) : undefined,
      offset: request.query.offset ? parseInt(String(request.query.offset), 10) : undefined,
    });

    return { templates, count: templates.length };
  });

  app.get<{ Params: { id: string } }>('/api/templates/:id', async (request, reply) => {
    const template = await scopedDb(request).getTemplate(request.params.id);
    if (!template) {
      return reply.status(404).send({ error: 'Template not found' });
    }
    return template;
  });

  app.put<{ Params: { id: string }; Body: UpdateTemplateRequest }>('/api/templates/:id', async (request, reply) => {
    const template = await scopedDb(request).updateTemplate(
      request.params.id,
      request.body as Partial<TemplateRecord>
    );
    if (!template) {
      return reply.status(404).send({ error: 'Template not found' });
    }
    return template;
  });

  app.delete<{ Params: { id: string } }>('/api/templates/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteTemplate(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Template not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Version Endpoints
  // =========================================================================

  app.get<{ Params: { id: string } }>('/api/documents/:id/versions', async (request, reply) => {
    const doc = await scopedDb(request).getDocument(request.params.id);
    if (!doc) {
      return reply.status(404).send({ error: 'Document not found' });
    }

    const versions = await scopedDb(request).listVersions(request.params.id);
    return { versions, count: versions.length };
  });

  app.get<{ Params: { id: string; version: string } }>('/api/documents/:id/versions/:version', async (request, reply) => {
    const version = await scopedDb(request).getVersion(
      request.params.id,
      parseInt(request.params.version, 10)
    );
    if (!version) {
      return reply.status(404).send({ error: 'Version not found' });
    }
    return version;
  });

  // =========================================================================
  // Share Endpoints
  // =========================================================================

  app.post<{ Params: { id: string }; Body: CreateShareRequest }>('/api/documents/:id/share', async (request, reply) => {
    try {
      const doc = await scopedDb(request).getDocument(request.params.id);
      if (!doc) {
        return reply.status(404).send({ error: 'Document not found' });
      }

      const shareToken = crypto.randomBytes(fullConfig.shareTokenLength / 2).toString('hex');

      const expiresAt = request.body.expires_at
        ? new Date(request.body.expires_at)
        : new Date(Date.now() + fullConfig.shareDefaultExpiryDays * 24 * 60 * 60 * 1000);

      const share = await scopedDb(request).createShare({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        document_id: request.params.id,
        shared_with_user_id: request.body.shared_with_user_id ?? null,
        shared_with_email: request.body.shared_with_email ?? null,
        share_token: shareToken,
        permission: request.body.permission ?? 'view',
        expires_at: expiresAt,
      });

      return reply.status(201).send({
        share_id: share.id,
        share_token: shareToken,
        share_url: `/api/shared/${shareToken}`,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to share document', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Params: { id: string } }>('/api/documents/:id/shares', async (request, reply) => {
    const doc = await scopedDb(request).getDocument(request.params.id);
    if (!doc) {
      return reply.status(404).send({ error: 'Document not found' });
    }

    const shares = await scopedDb(request).listShares(request.params.id);
    return { shares, count: shares.length };
  });

  app.delete<{ Params: { id: string } }>('/api/shares/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteShare(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Share not found' });
    }
    return { success: true };
  });

  // Public shared document endpoint (no auth required)
  app.get<{ Params: { token: string } }>('/api/shared/:token', async (request, reply) => {
    const share = await db.getShareByToken(request.params.token);
    if (!share) {
      return reply.status(404).send({ error: 'Shared document not found or link expired' });
    }

    return {
      document_id: share.document_id,
      title: share.doc_title,
      file_url: share.doc_file_url,
      permission: share.permission,
    };
  });

  // =========================================================================
  // Search Endpoint
  // =========================================================================

  app.post<{ Body: SearchDocumentsRequest }>('/api/documents/search', async (request, reply) => {
    try {
      const documents = await scopedDb(request).searchDocuments({
        query: request.body.query,
        docType: request.body.doc_type,
        category: request.body.category,
        ownerId: request.body.owner_id,
        dateFrom: request.body.date_from ? new Date(request.body.date_from) : undefined,
        dateTo: request.body.date_to ? new Date(request.body.date_to) : undefined,
        limit: request.body.limit,
      });

      return { documents, count: documents.length };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Search failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'documents',
      version: '1.0.0',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Server Lifecycle
  // =========================================================================

  const server = {
    async start() {
      try {
        await app.listen({ port: fullConfig.port, host: fullConfig.host });
        logger.info(`Documents server listening on ${fullConfig.host}:${fullConfig.port}`);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Server failed to start', { error: message });
        throw error;
      }
    },

    async stop() {
      await app.close();
      await db.disconnect();
      logger.info('Server stopped');
    },
  };

  return server;
}

export async function startServer(config?: Partial<Config>): Promise<void> {
  const server = await createServer(config);
  await server.start();

  process.on('SIGTERM', async () => {
    logger.info('SIGTERM received, shutting down gracefully');
    await server.stop();
    process.exit(0);
  });

  process.on('SIGINT', async () => {
    logger.info('SIGINT received, shutting down gracefully');
    await server.stop();
    process.exit(0);
  });
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
