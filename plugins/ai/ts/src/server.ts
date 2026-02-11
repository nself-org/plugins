/**
 * AI Plugin Server
 * HTTP server for AI gateway API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { AiDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  ChatCompletionRequest,
  CreateConversationRequest,
  CreateModelRequest,
  UpdateModelRequest,
  CreatePromptTemplateRequest,
  RenderPromptRequest,
  StoreEmbeddingRequest,
  SemanticSearchRequest,
  SetQuotaRequest,
  SummarizeRequest,
  TranslateRequest,
  SentimentRequest,
  ChatCompletionResponse,
} from './types.js';

const logger = createLogger('ai:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  const db = new AiDatabase();
  await db.connect();
  await db.initializeSchema();

  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024,
  });

  await app.register(cors, { origin: true, credentials: true });

  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 100,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );

  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): AiDatabase {
    return (request as Record<string, unknown>).scopedDb as AiDatabase;
  }

  // =========================================================================
  // Health Checks
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'ai', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'ai', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false, plugin: 'ai', error: 'Database unavailable', timestamp: new Date().toISOString(),
      });
    }
  });

  // =========================================================================
  // Chat Completions
  // =========================================================================

  app.post<{ Body: ChatCompletionRequest }>('/api/ai/chat/completions', async (request, reply) => {
    try {
      const { messages, temperature, max_tokens, user_id, conversation_id } = request.body;
      if (!messages || messages.length === 0) {
        return reply.status(400).send({ error: 'messages array is required' });
      }

      // Check quota
      if (user_id) {
        const hasQuota = await scopedDb(request).checkQuota(user_id);
        if (!hasQuota) {
          return reply.status(429).send({ error: 'Usage quota exceeded' });
        }
      }

      // Resolve model
      let model = await scopedDb(request).getDefaultModel();
      if (request.body.model) {
        const models = await scopedDb(request).listModels(true);
        model = models.find(m => m.model_id === request.body.model || m.model_name === request.body.model) ?? model;
      }

      if (!model) {
        return reply.status(400).send({ error: 'No AI model available. Please configure at least one model.' });
      }

      const startTime = Date.now();

      // Track request
      const aiRequest = await scopedDb(request).createRequest(
        model.provider, model.model_name, 'chat',
        { messages, temperature, max_tokens }, user_id, model.id
      );

      // Simulate completion response (actual provider integration would go here)
      const simulatedContent = `[AI Response from ${model.model_name}] This is a placeholder response. Configure provider API keys for actual completions.`;
      const promptTokens = messages.reduce((sum, m) => sum + Math.ceil(m.content.length / 4), 0);
      const completionTokens = Math.ceil(simulatedContent.length / 4);
      const totalTokens = promptTokens + completionTokens;
      const inputCost = model.input_price_per_million ? (promptTokens / 1000000) * Number(model.input_price_per_million) : 0;
      const outputCost = model.output_price_per_million ? (completionTokens / 1000000) * Number(model.output_price_per_million) : 0;
      const totalCost = inputCost + outputCost;
      const latencyMs = Date.now() - startTime;

      await scopedDb(request).completeRequest(aiRequest.id, { content: simulatedContent }, promptTokens, completionTokens, totalCost, latencyMs);

      // Store in conversation if provided
      if (conversation_id) {
        const lastUserMsg = messages[messages.length - 1];
        if (lastUserMsg) {
          await scopedDb(request).addMessage(conversation_id, lastUserMsg.role, lastUserMsg.content);
        }
        await scopedDb(request).addMessage(conversation_id, 'assistant', simulatedContent, {
          tokensUsed: totalTokens, modelUsed: model.model_name,
          finishReason: 'stop', cost: totalCost, latencyMs,
        });
      }

      // Track usage
      if (user_id) {
        await scopedDb(request).trackUsage(user_id, totalTokens, totalCost);
      }

      const response: ChatCompletionResponse = {
        id: aiRequest.id,
        choices: [{
          message: { role: 'assistant', content: simulatedContent },
          finish_reason: 'stop',
        }],
        usage: { prompt_tokens: promptTokens, completion_tokens: completionTokens, total_tokens: totalTokens },
        cost: totalCost,
        model: model.model_name,
        latency_ms: latencyMs,
      };

      return response;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Chat completion failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Conversations
  // =========================================================================

  app.post<{ Body: CreateConversationRequest }>('/api/ai/conversations', async (request, reply) => {
    try {
      const conversation = await scopedDb(request).createConversation(request.body);
      return reply.status(201).send({ conversation_id: conversation.id });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create conversation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Params: { id: string } }>('/api/ai/conversations/:id', async (request, reply) => {
    const conversation = await scopedDb(request).getConversation(request.params.id);
    if (!conversation) {
      return reply.status(404).send({ error: 'Conversation not found' });
    }
    const messages = await scopedDb(request).getMessages(conversation.id);
    return {
      ...conversation,
      messages: messages.map(m => ({ role: m.role, content: m.content, created_at: m.created_at })),
    };
  });

  app.get<{ Querystring: { user_id?: string; limit?: string } }>('/api/ai/conversations', async (request) => {
    const conversations = await scopedDb(request).listConversations(
      request.query.user_id,
      request.query.limit ? parseInt(request.query.limit, 10) : 50
    );
    return { conversations };
  });

  app.post<{ Params: { id: string }; Body: { content: string; role?: string } }>(
    '/api/ai/conversations/:id/messages',
    async (request, reply) => {
      try {
        const { content, role } = request.body;
        if (!content) {
          return reply.status(400).send({ error: 'content is required' });
        }

        const conversation = await scopedDb(request).getConversation(request.params.id);
        if (!conversation) {
          return reply.status(404).send({ error: 'Conversation not found' });
        }

        // Add user message
        const userMsg = await scopedDb(request).addMessage(request.params.id, role ?? 'user', content);

        // Generate response (placeholder)
        const model = conversation.model_id ? await scopedDb(request).getModel(conversation.model_id) : await scopedDb(request).getDefaultModel();
        const responseContent = `[AI Response] Placeholder response to: "${content.substring(0, 50)}..."`;
        const tokensUsed = Math.ceil((content.length + responseContent.length) / 4);
        const cost = 0.001;

        await scopedDb(request).addMessage(request.params.id, 'assistant', responseContent, {
          tokensUsed, modelUsed: model?.model_name ?? 'unknown', finishReason: 'stop', cost,
        });

        return {
          message_id: userMsg.id,
          response: {
            content: responseContent,
            tokens_used: tokensUsed,
            cost,
          },
        };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Add message failed', { error: message });
        return reply.status(500).send({ error: message });
      }
    }
  );

  app.delete<{ Params: { id: string } }>('/api/ai/conversations/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteConversation(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Conversation not found' });
    }
    return { deleted: true };
  });

  // =========================================================================
  // Embeddings
  // =========================================================================

  app.post<{ Body: StoreEmbeddingRequest }>('/api/ai/embeddings/create', async (request, reply) => {
    try {
      const { content_type, content_id, content_text } = request.body;
      if (!content_type || !content_id || !content_text) {
        return reply.status(400).send({ error: 'content_type, content_id, and content_text are required' });
      }

      const model = await scopedDb(request).getDefaultModel();
      if (!model) {
        return reply.status(400).send({ error: 'No embedding model available' });
      }

      // Placeholder embedding (actual integration would call provider API)
      const dimensions = fullConfig.embeddingsDimensions;
      const placeholderEmbedding = Array.from({ length: dimensions }, () => Math.random() * 2 - 1);

      const embedding = await scopedDb(request).storeEmbedding(
        content_type, content_id, content_text,
        model.id, dimensions, placeholderEmbedding
      );

      return reply.status(201).send({ embedding_id: embedding.id });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create embedding failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Body: SemanticSearchRequest }>('/api/ai/embeddings/search', async (request, reply) => {
    try {
      const { query, content_type, limit } = request.body;
      if (!query) {
        return reply.status(400).send({ error: 'query is required' });
      }

      // Placeholder semantic search (cosine similarity on stored embeddings)
      const embeddings = await scopedDb(request).listEmbeddings(content_type, limit ?? 10);

      const results = embeddings.map(e => ({
        id: e.id,
        content_type: e.content_type,
        content_id: e.content_id,
        content_text: e.content_text,
        similarity: Math.random() * 0.5 + 0.5, // placeholder
        metadata: e.metadata,
      }));

      results.sort((a, b) => b.similarity - a.similarity);

      return { results };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Semantic search failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Models
  // =========================================================================

  app.get('/api/ai/models', async (request) => {
    const models = await scopedDb(request).listModels();
    return {
      models: models.map(m => ({
        id: m.id, provider: m.provider, model_id: m.model_id,
        model_name: m.model_name, model_type: m.model_type,
        is_enabled: m.is_enabled, is_default: m.is_default,
        supports_streaming: m.supports_streaming,
        supports_functions: m.supports_functions,
        max_tokens: m.max_tokens, context_window: m.context_window,
      })),
    };
  });

  app.post<{ Body: CreateModelRequest }>('/api/ai/models', async (request, reply) => {
    try {
      const { provider, model_id, model_name, model_type, max_tokens, context_window } = request.body;
      if (!provider || !model_id || !model_name || !model_type || !max_tokens || !context_window) {
        return reply.status(400).send({ error: 'provider, model_id, model_name, model_type, max_tokens, and context_window are required' });
      }
      const model = await scopedDb(request).createModel(request.body);
      return reply.status(201).send({ model: model });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create model failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.patch<{ Params: { id: string }; Body: UpdateModelRequest }>('/api/ai/models/:id', async (request, reply) => {
    try {
      const model = await scopedDb(request).updateModel(request.params.id, request.body);
      if (!model) {
        return reply.status(404).send({ error: 'Model not found' });
      }
      return { success: true, model };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Update model failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete<{ Params: { id: string } }>('/api/ai/models/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteModel(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Model not found' });
    }
    return { deleted: true };
  });

  // =========================================================================
  // Prompt Templates
  // =========================================================================

  app.get<{ Querystring: { category?: string } }>('/api/ai/prompts', async (request) => {
    const templates = await scopedDb(request).listPromptTemplates(request.query.category);
    return {
      templates: templates.map(t => ({
        id: t.id, name: t.name, category: t.category,
        description: t.description, usage_count: t.usage_count,
      })),
    };
  });

  app.post<{ Body: CreatePromptTemplateRequest }>('/api/ai/prompts', async (request, reply) => {
    try {
      const { name, user_prompt_template } = request.body;
      if (!name || !user_prompt_template) {
        return reply.status(400).send({ error: 'name and user_prompt_template are required' });
      }
      const template = await scopedDb(request).createPromptTemplate(request.body);
      return reply.status(201).send({ template_id: template.id });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create prompt template failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Params: { id: string }; Body: RenderPromptRequest }>('/api/ai/prompts/:id/render', async (request, reply) => {
    try {
      const { variables } = request.body;
      if (!variables) {
        return reply.status(400).send({ error: 'variables object is required' });
      }
      const rendered = await scopedDb(request).renderPromptTemplate(request.params.id, variables);
      return { rendered_prompt: rendered };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Render prompt failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Usage & Quotas
  // =========================================================================

  app.get<{ Querystring: { start_date?: string; end_date?: string; group_by?: string; user_id?: string } }>(
    '/api/ai/usage',
    async (request) => {
      const usage = await scopedDb(request).getUsage({
        startDate: request.query.start_date,
        endDate: request.query.end_date,
        groupBy: request.query.group_by as 'day' | 'model' | 'provider' | undefined,
        userId: request.query.user_id,
      });
      return usage;
    }
  );

  app.get<{ Querystring: { user_id: string } }>('/api/ai/quota', async (request, reply) => {
    const userId = request.query.user_id;
    if (!userId) {
      return reply.status(400).send({ error: 'user_id query parameter is required' });
    }
    const quota = await scopedDb(request).getQuota(userId);
    if (!quota) {
      return reply.status(404).send({ error: 'No quota found for user' });
    }
    return quota;
  });

  app.post<{ Body: SetQuotaRequest }>('/api/ai/quota', async (request, reply) => {
    try {
      const { quota_type } = request.body;
      if (!quota_type) {
        return reply.status(400).send({ error: 'quota_type is required' });
      }
      const quota = await scopedDb(request).setQuota(request.body);
      return reply.status(201).send({ quota });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Set quota failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // AI Features
  // =========================================================================

  app.get('/api/ai/features', async (request) => {
    const features = await scopedDb(request).listFeatures();
    return { features };
  });

  app.post<{ Body: SummarizeRequest }>('/api/ai/features/summarize', async (request, reply) => {
    try {
      const { messages, language } = request.body;
      if (!messages || messages.length === 0) {
        return reply.status(400).send({ error: 'messages array is required' });
      }

      const combined = messages.join('\n');
      const summary = `[Summary in ${language ?? 'English'}] This is a placeholder summary of ${messages.length} messages (${combined.length} characters).`;
      const tokensUsed = Math.ceil(combined.length / 4) + Math.ceil(summary.length / 4);

      return { summary, tokens_used: tokensUsed };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Summarization failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Body: TranslateRequest }>('/api/ai/features/translate', async (request, reply) => {
    try {
      const { text, target_language } = request.body;
      if (!text || !target_language) {
        return reply.status(400).send({ error: 'text and target_language are required' });
      }

      const translated = `[Translated to ${target_language}] ${text}`;
      return { translated_text: translated };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Translation failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Body: SentimentRequest }>('/api/ai/features/sentiment', async (request, reply) => {
    try {
      const { text } = request.body;
      if (!text) {
        return reply.status(400).send({ error: 'text is required' });
      }

      // Placeholder sentiment analysis
      const sentiments: Array<'positive' | 'negative' | 'neutral'> = ['positive', 'negative', 'neutral'];
      const sentiment = sentiments[Math.floor(Math.random() * sentiments.length)] ?? 'neutral';
      const confidence = 0.5 + Math.random() * 0.5;

      return { sentiment, confidence: Math.round(confidence * 100) / 100 };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sentiment analysis failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  return { app, db, config: fullConfig };
}

export async function startServer(config?: Partial<Config>) {
  const { app, config: fullConfig } = await createServer(config);

  await app.listen({ port: fullConfig.port, host: fullConfig.host });
  logger.info(`AI server listening on ${fullConfig.host}:${fullConfig.port}`);

  return app;
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
