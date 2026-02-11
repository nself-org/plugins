/**
 * AI Database Operations
 * Complete CRUD operations for AI models, conversations, embeddings, and usage tracking
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  AiModelRecord,
  CreateModelRequest,
  UpdateModelRequest,
  AiConversationRecord,
  CreateConversationRequest,
  AiMessageRecord,
  AiRequestRecord,
  AiEmbeddingRecord,
  AiPromptTemplateRecord,
  CreatePromptTemplateRequest,
  AiFunctionRecord,
  AiUsageQuotaRecord,
  QuotaStatus,
  SetQuotaRequest,
  AiFeatureRecord,
  UsageBreakdown,
  AiProvider,
} from './types.js';

const logger = createLogger('ai:db');

export class AiDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): AiDatabase {
    return new AiDatabase(this.db, sourceAccountId);
  }

  getCurrentSourceAccountId(): string {
    return this.sourceAccountId;
  }

  private normalizeSourceAccountId(value: string): string {
    const normalized = value.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '');
    return normalized.length > 0 ? normalized : 'primary';
  }

  async connect(): Promise<void> { await this.db.connect(); }
  async disconnect(): Promise<void> { await this.db.disconnect(); }
  async query<T extends Record<string, unknown>>(sql: string, params?: unknown[]): Promise<{ rows: T[]; rowCount: number | null }> { return this.db.query<T>(sql, params); }
  async execute(sql: string, params?: unknown[]): Promise<number> { return this.db.execute(sql, params); }

  // =========================================================================
  // Schema Management
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing AI schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- AI Models
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ai_models (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        provider VARCHAR(50) NOT NULL,
        model_id VARCHAR(255) NOT NULL,
        model_name VARCHAR(255) NOT NULL,
        model_type VARCHAR(50) NOT NULL,
        supports_streaming BOOLEAN NOT NULL DEFAULT false,
        supports_functions BOOLEAN NOT NULL DEFAULT false,
        supports_vision BOOLEAN NOT NULL DEFAULT false,
        max_tokens INTEGER NOT NULL,
        context_window INTEGER NOT NULL,
        input_price_per_million DECIMAL(10,4),
        output_price_per_million DECIMAL(10,4),
        default_temperature DECIMAL(3,2) DEFAULT 0.7,
        default_top_p DECIMAL(3,2) DEFAULT 1.0,
        default_max_tokens INTEGER DEFAULT 1000,
        is_enabled BOOLEAN NOT NULL DEFAULT true,
        is_default BOOLEAN NOT NULL DEFAULT false,
        priority INTEGER NOT NULL DEFAULT 100,
        description TEXT,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, provider, model_id)
      );
      CREATE INDEX IF NOT EXISTS idx_ai_models_account ON ai_models(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ai_models_enabled ON ai_models(source_account_id, is_enabled, priority);
      CREATE INDEX IF NOT EXISTS idx_ai_models_type ON ai_models(source_account_id, model_type, is_enabled);
      CREATE INDEX IF NOT EXISTS idx_ai_models_default ON ai_models(source_account_id, is_default) WHERE is_default = true;

      -- =====================================================================
      -- AI Conversations
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ai_conversations (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255),
        context_type VARCHAR(50),
        context_id VARCHAR(255),
        model_id UUID REFERENCES ai_models(id) ON DELETE SET NULL,
        system_prompt TEXT,
        temperature DECIMAL(3,2),
        max_tokens INTEGER,
        parameters JSONB DEFAULT '{}',
        message_count INTEGER NOT NULL DEFAULT 0,
        total_tokens INTEGER NOT NULL DEFAULT 0,
        total_cost DECIMAL(10,6) DEFAULT 0,
        title VARCHAR(255),
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        last_message_at TIMESTAMP WITH TIME ZONE
      );
      CREATE INDEX IF NOT EXISTS idx_ai_conversations_account ON ai_conversations(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ai_conversations_user ON ai_conversations(source_account_id, user_id, created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_ai_conversations_context ON ai_conversations(source_account_id, context_type, context_id);

      -- =====================================================================
      -- AI Messages
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ai_messages (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        conversation_id UUID NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
        role VARCHAR(50) NOT NULL,
        content TEXT NOT NULL,
        function_name VARCHAR(255),
        function_arguments JSONB,
        function_response JSONB,
        tokens_used INTEGER,
        model_used VARCHAR(255),
        finish_reason VARCHAR(50),
        cost DECIMAL(10,6),
        latency_ms INTEGER,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_ai_messages_account ON ai_messages(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ai_messages_conversation ON ai_messages(conversation_id, created_at);
      CREATE INDEX IF NOT EXISTS idx_ai_messages_role ON ai_messages(role);

      -- =====================================================================
      -- AI Requests (tracking)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ai_requests (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255),
        model_id UUID REFERENCES ai_models(id) ON DELETE SET NULL,
        provider VARCHAR(50) NOT NULL,
        model_name VARCHAR(255) NOT NULL,
        request_type VARCHAR(50) NOT NULL,
        input_data JSONB NOT NULL,
        output_data JSONB,
        status VARCHAR(20) NOT NULL DEFAULT 'pending',
        error_message TEXT,
        tokens_input INTEGER,
        tokens_output INTEGER,
        tokens_total INTEGER,
        cost DECIMAL(10,6),
        started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
        completed_at TIMESTAMP WITH TIME ZONE,
        latency_ms INTEGER,
        metadata JSONB DEFAULT '{}'
      );
      CREATE INDEX IF NOT EXISTS idx_ai_requests_account ON ai_requests(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ai_requests_user ON ai_requests(source_account_id, user_id, started_at DESC);
      CREATE INDEX IF NOT EXISTS idx_ai_requests_status ON ai_requests(source_account_id, status, started_at);
      CREATE INDEX IF NOT EXISTS idx_ai_requests_provider ON ai_requests(source_account_id, provider, started_at DESC);

      -- =====================================================================
      -- AI Embeddings
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ai_embeddings (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        content_type VARCHAR(50) NOT NULL,
        content_id VARCHAR(255) NOT NULL,
        content_text TEXT NOT NULL,
        content_hash VARCHAR(64),
        model_id UUID REFERENCES ai_models(id) ON DELETE SET NULL,
        embedding_dimensions INTEGER NOT NULL DEFAULT 1536,
        embedding_data JSONB,
        tokens_used INTEGER,
        cost DECIMAL(10,6),
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, content_type, content_id, model_id)
      );
      CREATE INDEX IF NOT EXISTS idx_ai_embeddings_account ON ai_embeddings(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ai_embeddings_content ON ai_embeddings(source_account_id, content_type, content_id);
      CREATE INDEX IF NOT EXISTS idx_ai_embeddings_hash ON ai_embeddings(content_hash) WHERE content_hash IS NOT NULL;

      -- =====================================================================
      -- AI Prompt Templates
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ai_prompt_templates (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        category VARCHAR(100),
        system_prompt TEXT,
        user_prompt_template TEXT NOT NULL,
        variables JSONB DEFAULT '[]',
        recommended_model_id UUID REFERENCES ai_models(id) ON DELETE SET NULL,
        default_temperature DECIMAL(3,2) DEFAULT 0.7,
        default_max_tokens INTEGER DEFAULT 1000,
        usage_count INTEGER NOT NULL DEFAULT 0,
        is_enabled BOOLEAN NOT NULL DEFAULT true,
        is_public BOOLEAN NOT NULL DEFAULT false,
        created_by VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );
      CREATE INDEX IF NOT EXISTS idx_prompt_templates_account ON ai_prompt_templates(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_prompt_templates_category ON ai_prompt_templates(source_account_id, category, is_enabled);

      -- =====================================================================
      -- AI Functions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ai_functions (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT NOT NULL,
        parameters_schema JSONB NOT NULL,
        implementation_type VARCHAR(50) NOT NULL,
        implementation_config JSONB NOT NULL,
        is_enabled BOOLEAN NOT NULL DEFAULT true,
        timeout_seconds INTEGER DEFAULT 30,
        call_count INTEGER NOT NULL DEFAULT 0,
        success_count INTEGER NOT NULL DEFAULT 0,
        failure_count INTEGER NOT NULL DEFAULT 0,
        avg_latency_ms INTEGER,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );
      CREATE INDEX IF NOT EXISTS idx_ai_functions_account ON ai_functions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ai_functions_enabled ON ai_functions(source_account_id, is_enabled);

      -- =====================================================================
      -- AI Function Calls
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ai_function_calls (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        function_id UUID NOT NULL REFERENCES ai_functions(id) ON DELETE CASCADE,
        message_id UUID REFERENCES ai_messages(id) ON DELETE SET NULL,
        conversation_id UUID REFERENCES ai_conversations(id) ON DELETE SET NULL,
        user_id VARCHAR(255),
        arguments JSONB NOT NULL,
        response JSONB,
        status VARCHAR(20) NOT NULL DEFAULT 'pending',
        error_message TEXT,
        started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
        completed_at TIMESTAMP WITH TIME ZONE,
        latency_ms INTEGER
      );
      CREATE INDEX IF NOT EXISTS idx_function_calls_account ON ai_function_calls(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_function_calls_function ON ai_function_calls(function_id, started_at DESC);
      CREATE INDEX IF NOT EXISTS idx_function_calls_conversation ON ai_function_calls(conversation_id);

      -- =====================================================================
      -- AI Usage Quotas
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ai_usage_quotas (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        quota_type VARCHAR(50) NOT NULL,
        scope_id VARCHAR(255),
        model_id UUID REFERENCES ai_models(id) ON DELETE CASCADE,
        max_requests_per_day INTEGER,
        max_tokens_per_day INTEGER,
        max_cost_per_day DECIMAL(10,2),
        current_requests INTEGER NOT NULL DEFAULT 0,
        current_tokens INTEGER NOT NULL DEFAULT 0,
        current_cost DECIMAL(10,6) NOT NULL DEFAULT 0,
        last_reset_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
        is_enabled BOOLEAN NOT NULL DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_usage_quotas_account ON ai_usage_quotas(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_usage_quotas_scope ON ai_usage_quotas(source_account_id, quota_type, scope_id);

      -- =====================================================================
      -- AI Features
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ai_features (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        feature_name VARCHAR(255) NOT NULL,
        feature_type VARCHAR(100) NOT NULL,
        description TEXT,
        prompt_template_id UUID REFERENCES ai_prompt_templates(id) ON DELETE SET NULL,
        default_model_id UUID REFERENCES ai_models(id) ON DELETE SET NULL,
        is_enabled BOOLEAN NOT NULL DEFAULT true,
        requires_permission BOOLEAN NOT NULL DEFAULT false,
        allowed_roles TEXT[] DEFAULT ARRAY['admin', 'owner'],
        usage_count INTEGER NOT NULL DEFAULT 0,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, feature_name)
      );
      CREATE INDEX IF NOT EXISTS idx_ai_features_account ON ai_features(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ai_features_enabled ON ai_features(source_account_id, is_enabled);
    `;

    await this.db.execute(schema);
    logger.info('AI schema initialized successfully');
  }

  // =========================================================================
  // Models CRUD
  // =========================================================================

  async createModel(request: CreateModelRequest): Promise<AiModelRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO ai_models (
        source_account_id, provider, model_id, model_name, model_type,
        supports_streaming, supports_functions, supports_vision,
        max_tokens, context_window,
        input_price_per_million, output_price_per_million,
        default_temperature, default_top_p, default_max_tokens,
        is_default, priority, description
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
      RETURNING *`,
      [
        this.sourceAccountId, request.provider, request.model_id, request.model_name, request.model_type,
        request.supports_streaming ?? false, request.supports_functions ?? false, request.supports_vision ?? false,
        request.max_tokens, request.context_window,
        request.input_price_per_million ?? null, request.output_price_per_million ?? null,
        request.default_temperature ?? 0.7, request.default_top_p ?? 1.0, request.default_max_tokens ?? 1000,
        request.is_default ?? false, request.priority ?? 100, request.description ?? null,
      ]
    );
    return result.rows[0] as unknown as AiModelRecord;
  }

  async getModel(id: string): Promise<AiModelRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM ai_models WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return (result.rows[0] ?? null) as unknown as AiModelRecord | null;
  }

  async getModelByProviderId(provider: string, modelId: string): Promise<AiModelRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM ai_models WHERE source_account_id = $1 AND provider = $2 AND model_id = $3',
      [this.sourceAccountId, provider, modelId]
    );
    return (result.rows[0] ?? null) as unknown as AiModelRecord | null;
  }

  async getDefaultModel(_modelType: AiProvider | string = 'chat'): Promise<AiModelRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM ai_models WHERE source_account_id = $1 AND is_enabled = true AND is_default = true ORDER BY priority ASC LIMIT 1',
      [this.sourceAccountId]
    );
    if (result.rows[0]) return result.rows[0] as unknown as AiModelRecord;

    // Fallback to highest priority enabled model
    const fallback = await this.query<Record<string, unknown>>(
      'SELECT * FROM ai_models WHERE source_account_id = $1 AND is_enabled = true ORDER BY priority ASC LIMIT 1',
      [this.sourceAccountId]
    );
    return (fallback.rows[0] ?? null) as unknown as AiModelRecord | null;
  }

  async listModels(enabledOnly = false): Promise<AiModelRecord[]> {
    const sql = enabledOnly
      ? 'SELECT * FROM ai_models WHERE source_account_id = $1 AND is_enabled = true ORDER BY priority ASC'
      : 'SELECT * FROM ai_models WHERE source_account_id = $1 ORDER BY priority ASC';
    const result = await this.query<Record<string, unknown>>(sql, [this.sourceAccountId]);
    return result.rows as unknown as AiModelRecord[];
  }

  async updateModel(id: string, updates: UpdateModelRequest): Promise<AiModelRecord | null> {
    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, id];
    let paramIndex = 3;

    if (updates.model_name !== undefined) { sets.push(`model_name = $${paramIndex++}`); params.push(updates.model_name); }
    if (updates.is_enabled !== undefined) { sets.push(`is_enabled = $${paramIndex++}`); params.push(updates.is_enabled); }
    if (updates.is_default !== undefined) { sets.push(`is_default = $${paramIndex++}`); params.push(updates.is_default); }
    if (updates.priority !== undefined) { sets.push(`priority = $${paramIndex++}`); params.push(updates.priority); }
    if (updates.default_temperature !== undefined) { sets.push(`default_temperature = $${paramIndex++}`); params.push(updates.default_temperature); }
    if (updates.default_top_p !== undefined) { sets.push(`default_top_p = $${paramIndex++}`); params.push(updates.default_top_p); }
    if (updates.default_max_tokens !== undefined) { sets.push(`default_max_tokens = $${paramIndex++}`); params.push(updates.default_max_tokens); }
    if (updates.input_price_per_million !== undefined) { sets.push(`input_price_per_million = $${paramIndex++}`); params.push(updates.input_price_per_million); }
    if (updates.output_price_per_million !== undefined) { sets.push(`output_price_per_million = $${paramIndex++}`); params.push(updates.output_price_per_million); }
    if (updates.description !== undefined) { sets.push(`description = $${paramIndex++}`); params.push(updates.description); }

    if (sets.length === 0) return this.getModel(id);
    sets.push('updated_at = NOW()');

    const result = await this.query<Record<string, unknown>>(
      `UPDATE ai_models SET ${sets.join(', ')} WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      params
    );
    return (result.rows[0] ?? null) as unknown as AiModelRecord | null;
  }

  async deleteModel(id: string): Promise<boolean> {
    const count = await this.execute('DELETE FROM ai_models WHERE source_account_id = $1 AND id = $2', [this.sourceAccountId, id]);
    return count > 0;
  }

  // =========================================================================
  // Conversations CRUD
  // =========================================================================

  async createConversation(request: CreateConversationRequest): Promise<AiConversationRecord> {
    let modelId = request.model_id;
    if (!modelId) {
      const defaultModel = await this.getDefaultModel();
      modelId = defaultModel?.id ?? null as unknown as string;
    }

    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO ai_conversations (
        source_account_id, user_id, context_type, context_id,
        model_id, system_prompt, temperature, max_tokens, title
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
      RETURNING *`,
      [
        this.sourceAccountId, request.user_id ?? null,
        request.context_type ?? null, request.context_id ?? null,
        modelId, request.system_prompt ?? null,
        request.temperature ?? null, request.max_tokens ?? null,
        request.title ?? null,
      ]
    );
    return result.rows[0] as unknown as AiConversationRecord;
  }

  async getConversation(id: string): Promise<AiConversationRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM ai_conversations WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return (result.rows[0] ?? null) as unknown as AiConversationRecord | null;
  }

  async listConversations(userId?: string, limit = 50): Promise<AiConversationRecord[]> {
    const sql = userId
      ? 'SELECT * FROM ai_conversations WHERE source_account_id = $1 AND user_id = $2 ORDER BY last_message_at DESC NULLS LAST LIMIT $3'
      : 'SELECT * FROM ai_conversations WHERE source_account_id = $1 ORDER BY last_message_at DESC NULLS LAST LIMIT $2';
    const params = userId ? [this.sourceAccountId, userId, limit] : [this.sourceAccountId, limit];
    const result = await this.query<Record<string, unknown>>(sql, params);
    return result.rows as unknown as AiConversationRecord[];
  }

  async deleteConversation(id: string): Promise<boolean> {
    const count = await this.execute('DELETE FROM ai_conversations WHERE source_account_id = $1 AND id = $2', [this.sourceAccountId, id]);
    return count > 0;
  }

  // =========================================================================
  // Messages CRUD
  // =========================================================================

  async addMessage(
    conversationId: string,
    role: string,
    content: string,
    options: {
      tokensUsed?: number;
      modelUsed?: string;
      finishReason?: string;
      cost?: number;
      latencyMs?: number;
      functionName?: string;
      functionArguments?: Record<string, unknown>;
      functionResponse?: Record<string, unknown>;
    } = {}
  ): Promise<AiMessageRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO ai_messages (
        source_account_id, conversation_id, role, content,
        function_name, function_arguments, function_response,
        tokens_used, model_used, finish_reason, cost, latency_ms
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
      RETURNING *`,
      [
        this.sourceAccountId, conversationId, role, content,
        options.functionName ?? null,
        options.functionArguments ? JSON.stringify(options.functionArguments) : null,
        options.functionResponse ? JSON.stringify(options.functionResponse) : null,
        options.tokensUsed ?? null, options.modelUsed ?? null,
        options.finishReason ?? null, options.cost ?? null, options.latencyMs ?? null,
      ]
    );

    // Update conversation stats
    await this.execute(
      `UPDATE ai_conversations SET
        message_count = message_count + 1,
        total_tokens = total_tokens + COALESCE($3, 0),
        total_cost = total_cost + COALESCE($4, 0),
        last_message_at = NOW(),
        updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, conversationId, options.tokensUsed ?? 0, options.cost ?? 0]
    );

    return result.rows[0] as unknown as AiMessageRecord;
  }

  async getMessages(conversationId: string, limit = 100): Promise<AiMessageRecord[]> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM ai_messages WHERE source_account_id = $1 AND conversation_id = $2 ORDER BY created_at ASC LIMIT $3',
      [this.sourceAccountId, conversationId, limit]
    );
    return result.rows as unknown as AiMessageRecord[];
  }

  // =========================================================================
  // Request Tracking
  // =========================================================================

  async createRequest(
    provider: AiProvider,
    modelName: string,
    requestType: string,
    inputData: Record<string, unknown>,
    userId?: string,
    modelId?: string
  ): Promise<AiRequestRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO ai_requests (
        source_account_id, user_id, model_id, provider, model_name,
        request_type, input_data, status
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, 'processing')
      RETURNING *`,
      [this.sourceAccountId, userId ?? null, modelId ?? null, provider, modelName, requestType, JSON.stringify(inputData)]
    );
    return result.rows[0] as unknown as AiRequestRecord;
  }

  async completeRequest(
    id: string,
    outputData: Record<string, unknown>,
    tokensInput: number,
    tokensOutput: number,
    cost: number,
    latencyMs: number
  ): Promise<void> {
    await this.execute(
      `UPDATE ai_requests SET
        output_data = $2, status = 'completed', tokens_input = $3,
        tokens_output = $4, tokens_total = $5, cost = $6,
        completed_at = NOW(), latency_ms = $7
       WHERE id = $1`,
      [id, JSON.stringify(outputData), tokensInput, tokensOutput, tokensInput + tokensOutput, cost, latencyMs]
    );
  }

  async failRequest(id: string, errorMessage: string): Promise<void> {
    await this.execute(
      `UPDATE ai_requests SET status = 'failed', error_message = $2, completed_at = NOW()
       WHERE id = $1`,
      [id, errorMessage]
    );
  }

  // =========================================================================
  // Embeddings
  // =========================================================================

  async storeEmbedding(
    contentType: string,
    contentId: string,
    contentText: string,
    modelId: string,
    dimensions: number,
    embeddingData: number[],
    tokensUsed?: number,
    cost?: number
  ): Promise<AiEmbeddingRecord> {
    const contentHash = await this.hashContent(contentText);

    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO ai_embeddings (
        source_account_id, content_type, content_id, content_text,
        content_hash, model_id, embedding_dimensions, embedding_data,
        tokens_used, cost
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
      ON CONFLICT (source_account_id, content_type, content_id, model_id) DO UPDATE SET
        content_text = EXCLUDED.content_text,
        content_hash = EXCLUDED.content_hash,
        embedding_data = EXCLUDED.embedding_data,
        embedding_dimensions = EXCLUDED.embedding_dimensions,
        tokens_used = EXCLUDED.tokens_used,
        cost = EXCLUDED.cost,
        created_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, contentType, contentId, contentText,
        contentHash, modelId, dimensions, JSON.stringify(embeddingData),
        tokensUsed ?? null, cost ?? null,
      ]
    );
    return result.rows[0] as unknown as AiEmbeddingRecord;
  }

  async getEmbedding(contentType: string, contentId: string): Promise<AiEmbeddingRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM ai_embeddings WHERE source_account_id = $1 AND content_type = $2 AND content_id = $3 LIMIT 1',
      [this.sourceAccountId, contentType, contentId]
    );
    return (result.rows[0] ?? null) as unknown as AiEmbeddingRecord | null;
  }

  async listEmbeddings(contentType?: string, limit = 100): Promise<AiEmbeddingRecord[]> {
    const sql = contentType
      ? 'SELECT * FROM ai_embeddings WHERE source_account_id = $1 AND content_type = $2 ORDER BY created_at DESC LIMIT $3'
      : 'SELECT * FROM ai_embeddings WHERE source_account_id = $1 ORDER BY created_at DESC LIMIT $2';
    const params = contentType ? [this.sourceAccountId, contentType, limit] : [this.sourceAccountId, limit];
    const result = await this.query<Record<string, unknown>>(sql, params);
    return result.rows as unknown as AiEmbeddingRecord[];
  }

  private async hashContent(text: string): Promise<string> {
    // Simple hash for deduplication
    let hash = 0;
    for (let i = 0; i < text.length; i++) {
      const char = text.charCodeAt(i);
      hash = ((hash << 5) - hash) + char;
      hash = hash & hash;
    }
    return Math.abs(hash).toString(16).padStart(16, '0');
  }

  // =========================================================================
  // Prompt Templates
  // =========================================================================

  async createPromptTemplate(request: CreatePromptTemplateRequest): Promise<AiPromptTemplateRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO ai_prompt_templates (
        source_account_id, name, description, category, system_prompt,
        user_prompt_template, variables, recommended_model_id,
        default_temperature, default_max_tokens, is_public
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
      RETURNING *`,
      [
        this.sourceAccountId, request.name, request.description ?? null,
        request.category ?? null, request.system_prompt ?? null,
        request.user_prompt_template, JSON.stringify(request.variables ?? []),
        request.recommended_model_id ?? null,
        request.default_temperature ?? 0.7, request.default_max_tokens ?? 1000,
        request.is_public ?? false,
      ]
    );
    return result.rows[0] as unknown as AiPromptTemplateRecord;
  }

  async getPromptTemplate(id: string): Promise<AiPromptTemplateRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM ai_prompt_templates WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return (result.rows[0] ?? null) as unknown as AiPromptTemplateRecord | null;
  }

  async listPromptTemplates(category?: string): Promise<AiPromptTemplateRecord[]> {
    const sql = category
      ? 'SELECT * FROM ai_prompt_templates WHERE source_account_id = $1 AND category = $2 AND is_enabled = true ORDER BY name'
      : 'SELECT * FROM ai_prompt_templates WHERE source_account_id = $1 AND is_enabled = true ORDER BY name';
    const params = category ? [this.sourceAccountId, category] : [this.sourceAccountId];
    const result = await this.query<Record<string, unknown>>(sql, params);
    return result.rows as unknown as AiPromptTemplateRecord[];
  }

  async renderPromptTemplate(id: string, variables: Record<string, string>): Promise<string> {
    const template = await this.getPromptTemplate(id);
    if (!template) throw new Error('Template not found');

    let rendered = template.user_prompt_template;
    for (const [key, value] of Object.entries(variables)) {
      rendered = rendered.replace(new RegExp(`\\{\\{${key}\\}\\}`, 'g'), value);
    }

    await this.execute(
      `UPDATE ai_prompt_templates SET usage_count = usage_count + 1 WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, id]
    );

    return rendered;
  }

  // =========================================================================
  // Functions
  // =========================================================================

  async listFunctions(enabledOnly = true): Promise<AiFunctionRecord[]> {
    const sql = enabledOnly
      ? 'SELECT * FROM ai_functions WHERE source_account_id = $1 AND is_enabled = true ORDER BY name'
      : 'SELECT * FROM ai_functions WHERE source_account_id = $1 ORDER BY name';
    const result = await this.query<Record<string, unknown>>(sql, [this.sourceAccountId]);
    return result.rows as unknown as AiFunctionRecord[];
  }

  // =========================================================================
  // Quotas
  // =========================================================================

  async getQuota(userId: string): Promise<QuotaStatus | null> {
    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM ai_usage_quotas
       WHERE source_account_id = $1 AND quota_type = 'user' AND scope_id = $2 AND is_enabled = true
       ORDER BY created_at DESC LIMIT 1`,
      [this.sourceAccountId, userId]
    );

    if (!result.rows[0]) return null;
    const q = result.rows[0] as unknown as AiUsageQuotaRecord;

    return {
      max_requests_per_day: q.max_requests_per_day,
      max_tokens_per_day: q.max_tokens_per_day,
      max_cost_per_day: q.max_cost_per_day,
      current_requests: q.current_requests,
      current_tokens: q.current_tokens,
      current_cost: q.current_cost,
      remaining_requests: q.max_requests_per_day ? q.max_requests_per_day - q.current_requests : null,
      remaining_tokens: q.max_tokens_per_day ? q.max_tokens_per_day - q.current_tokens : null,
      remaining_cost: q.max_cost_per_day ? q.max_cost_per_day - q.current_cost : null,
      reset_at: new Date(new Date().setHours(24, 0, 0, 0)).toISOString(),
    };
  }

  async setQuota(request: SetQuotaRequest): Promise<AiUsageQuotaRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO ai_usage_quotas (
        source_account_id, quota_type, scope_id, model_id,
        max_requests_per_day, max_tokens_per_day, max_cost_per_day
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      ON CONFLICT ON CONSTRAINT ai_usage_quotas_pkey DO NOTHING
      RETURNING *`,
      [
        this.sourceAccountId, request.quota_type,
        request.scope_id ?? null, request.model_id ?? null,
        request.max_requests_per_day ?? null,
        request.max_tokens_per_day ?? null,
        request.max_cost_per_day ?? null,
      ]
    );

    if (!result.rows[0]) {
      // Update existing
      await this.execute(
        `UPDATE ai_usage_quotas SET
          max_requests_per_day = COALESCE($3, max_requests_per_day),
          max_tokens_per_day = COALESCE($4, max_tokens_per_day),
          max_cost_per_day = COALESCE($5, max_cost_per_day),
          updated_at = NOW()
         WHERE source_account_id = $1 AND quota_type = $2 AND scope_id = $6`,
        [this.sourceAccountId, request.quota_type, request.max_requests_per_day ?? null,
         request.max_tokens_per_day ?? null, request.max_cost_per_day ?? null, request.scope_id ?? null]
      );
    }

    const updated = await this.query<Record<string, unknown>>(
      `SELECT * FROM ai_usage_quotas WHERE source_account_id = $1 AND quota_type = $2 AND scope_id = $3 LIMIT 1`,
      [this.sourceAccountId, request.quota_type, request.scope_id ?? null]
    );
    return updated.rows[0] as unknown as AiUsageQuotaRecord;
  }

  async trackUsage(userId: string, tokens: number, cost: number): Promise<void> {
    // Reset if new day
    await this.execute(
      `UPDATE ai_usage_quotas SET
        current_requests = 0, current_tokens = 0, current_cost = 0, last_reset_at = NOW()
       WHERE source_account_id = $1 AND last_reset_at < DATE_TRUNC('day', NOW()) AND is_enabled = true`,
      [this.sourceAccountId]
    );

    await this.execute(
      `UPDATE ai_usage_quotas SET
        current_requests = current_requests + 1,
        current_tokens = current_tokens + $3,
        current_cost = current_cost + $4
       WHERE source_account_id = $1 AND quota_type = 'user' AND scope_id = $2 AND is_enabled = true`,
      [this.sourceAccountId, userId, tokens, cost]
    );
  }

  async checkQuota(userId: string, estimatedTokens = 0): Promise<boolean> {
    const result = await this.query<{ exceeded: boolean }>(
      `SELECT EXISTS (
        SELECT 1 FROM ai_usage_quotas
        WHERE source_account_id = $1 AND quota_type = 'user' AND scope_id = $2 AND is_enabled = true
        AND (
          (max_requests_per_day IS NOT NULL AND current_requests >= max_requests_per_day)
          OR (max_tokens_per_day IS NOT NULL AND (current_tokens + $3) > max_tokens_per_day)
        )
      ) as exceeded`,
      [this.sourceAccountId, userId, estimatedTokens]
    );
    return !(result.rows[0]?.exceeded ?? false);
  }

  // =========================================================================
  // Features
  // =========================================================================

  async listFeatures(enabledOnly = true): Promise<AiFeatureRecord[]> {
    const sql = enabledOnly
      ? 'SELECT * FROM ai_features WHERE source_account_id = $1 AND is_enabled = true ORDER BY feature_name'
      : 'SELECT * FROM ai_features WHERE source_account_id = $1 ORDER BY feature_name';
    const result = await this.query<Record<string, unknown>>(sql, [this.sourceAccountId]);
    return result.rows as unknown as AiFeatureRecord[];
  }

  // =========================================================================
  // Usage Analytics
  // =========================================================================

  async getUsage(options: {
    startDate?: string;
    endDate?: string;
    groupBy?: 'day' | 'model' | 'provider';
    userId?: string;
  } = {}): Promise<{ total_requests: number; total_tokens: number; total_cost: number; breakdown: UsageBreakdown[] }> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.startDate) { conditions.push(`started_at >= $${paramIndex++}::timestamptz`); params.push(options.startDate); }
    if (options.endDate) { conditions.push(`started_at <= $${paramIndex++}::timestamptz`); params.push(options.endDate); }
    if (options.userId) { conditions.push(`user_id = $${paramIndex++}`); params.push(options.userId); }

    const whereClause = conditions.join(' AND ');

    // Totals
    const totalsResult = await this.query<{ total_requests: string; total_tokens: string; total_cost: string }>(
      `SELECT COUNT(*) as total_requests, COALESCE(SUM(tokens_total), 0) as total_tokens, COALESCE(SUM(cost), 0) as total_cost
       FROM ai_requests WHERE ${whereClause}`,
      params
    );

    const totals = totalsResult.rows[0];

    // Breakdown
    let groupColumn: string;
    let selectColumn: string;
    switch (options.groupBy) {
      case 'model': groupColumn = 'model_name'; selectColumn = 'model_name as label'; break;
      case 'provider': groupColumn = 'provider'; selectColumn = 'provider as label'; break;
      default: groupColumn = "DATE_TRUNC('day', started_at)"; selectColumn = "DATE_TRUNC('day', started_at)::date as label"; break;
    }

    const breakdownResult = await this.query<{ label: string; requests: string; tokens: string; cost: string }>(
      `SELECT ${selectColumn}, COUNT(*) as requests, COALESCE(SUM(tokens_total), 0) as tokens, COALESCE(SUM(cost), 0) as cost
       FROM ai_requests WHERE ${whereClause}
       GROUP BY ${groupColumn} ORDER BY ${groupColumn}`,
      params
    );

    const breakdown: UsageBreakdown[] = breakdownResult.rows.map(row => {
      const entry: UsageBreakdown = {
        requests: parseInt(row.requests, 10),
        tokens: parseInt(row.tokens, 10),
        cost: parseFloat(row.cost),
      };
      if (options.groupBy === 'model') entry.model = row.label;
      else if (options.groupBy === 'provider') entry.provider = row.label;
      else entry.date = row.label;
      return entry;
    });

    return {
      total_requests: parseInt(totals?.total_requests ?? '0', 10),
      total_tokens: parseInt(totals?.total_tokens ?? '0', 10),
      total_cost: parseFloat(totals?.total_cost ?? '0'),
      breakdown,
    };
  }
}
