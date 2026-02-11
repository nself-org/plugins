/**
 * AI Plugin Types
 * Complete type definitions for AI/LLM gateway operations
 */

// =============================================================================
// Enum Types
// =============================================================================

export type AiProvider = 'openai' | 'anthropic' | 'google' | 'cohere' | 'huggingface' | 'local' | 'custom';

export type AiModelType = 'chat' | 'completion' | 'embedding' | 'image' | 'audio' | 'multimodal';

export type AiRequestStatus = 'pending' | 'processing' | 'completed' | 'failed' | 'cancelled';

// =============================================================================
// Model Types
// =============================================================================

export interface AiModelRecord {
  id: string;
  source_account_id: string;
  provider: AiProvider;
  model_id: string;
  model_name: string;
  model_type: AiModelType;
  supports_streaming: boolean;
  supports_functions: boolean;
  supports_vision: boolean;
  max_tokens: number;
  context_window: number;
  input_price_per_million: number | null;
  output_price_per_million: number | null;
  default_temperature: number;
  default_top_p: number;
  default_max_tokens: number;
  is_enabled: boolean;
  is_default: boolean;
  priority: number;
  description: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateModelRequest {
  provider: AiProvider;
  model_id: string;
  model_name: string;
  model_type: AiModelType;
  supports_streaming?: boolean;
  supports_functions?: boolean;
  supports_vision?: boolean;
  max_tokens: number;
  context_window: number;
  input_price_per_million?: number;
  output_price_per_million?: number;
  default_temperature?: number;
  default_top_p?: number;
  default_max_tokens?: number;
  is_default?: boolean;
  priority?: number;
  description?: string;
}

export interface UpdateModelRequest {
  model_name?: string;
  is_enabled?: boolean;
  is_default?: boolean;
  priority?: number;
  default_temperature?: number;
  default_top_p?: number;
  default_max_tokens?: number;
  input_price_per_million?: number;
  output_price_per_million?: number;
  description?: string;
}

// =============================================================================
// Conversation Types
// =============================================================================

export interface AiConversationRecord {
  id: string;
  source_account_id: string;
  user_id: string | null;
  context_type: string | null;
  context_id: string | null;
  model_id: string;
  system_prompt: string | null;
  temperature: number | null;
  max_tokens: number | null;
  parameters: Record<string, unknown>;
  message_count: number;
  total_tokens: number;
  total_cost: number;
  title: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
  last_message_at: Date | null;
}

export interface CreateConversationRequest {
  user_id?: string;
  model_id?: string;
  context_type?: string;
  context_id?: string;
  system_prompt?: string;
  temperature?: number;
  max_tokens?: number;
  title?: string;
}

// =============================================================================
// Message Types
// =============================================================================

export interface AiMessageRecord {
  id: string;
  source_account_id: string;
  conversation_id: string;
  role: string;
  content: string;
  function_name: string | null;
  function_arguments: Record<string, unknown> | null;
  function_response: Record<string, unknown> | null;
  tokens_used: number | null;
  model_used: string | null;
  finish_reason: string | null;
  cost: number | null;
  latency_ms: number | null;
  created_at: Date;
}

export interface ChatMessage {
  role: 'system' | 'user' | 'assistant' | 'function';
  content: string;
  name?: string;
}

// =============================================================================
// Chat Completion Types
// =============================================================================

export interface ChatCompletionRequest {
  model?: string;
  messages: ChatMessage[];
  temperature?: number;
  max_tokens?: number;
  stream?: boolean;
  functions?: FunctionDefinition[];
  user_id?: string;
  conversation_id?: string;
}

export interface FunctionDefinition {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
}

export interface ChatCompletionChoice {
  message: {
    role: string;
    content: string;
    function_call?: {
      name: string;
      arguments: string;
    };
  };
  finish_reason: string;
}

export interface ChatCompletionResponse {
  id: string;
  choices: ChatCompletionChoice[];
  usage: {
    prompt_tokens: number;
    completion_tokens: number;
    total_tokens: number;
  };
  cost: number;
  model: string;
  latency_ms: number;
}

// =============================================================================
// Request Tracking Types
// =============================================================================

export interface AiRequestRecord {
  id: string;
  source_account_id: string;
  user_id: string | null;
  model_id: string | null;
  provider: AiProvider;
  model_name: string;
  request_type: string;
  input_data: Record<string, unknown>;
  output_data: Record<string, unknown> | null;
  status: AiRequestStatus;
  error_message: string | null;
  tokens_input: number | null;
  tokens_output: number | null;
  tokens_total: number | null;
  cost: number | null;
  started_at: Date;
  completed_at: Date | null;
  latency_ms: number | null;
  metadata: Record<string, unknown>;
}

// =============================================================================
// Embedding Types
// =============================================================================

export interface AiEmbeddingRecord {
  id: string;
  source_account_id: string;
  content_type: string;
  content_id: string;
  content_text: string;
  content_hash: string | null;
  model_id: string;
  embedding_dimensions: number;
  tokens_used: number | null;
  cost: number | null;
  metadata: Record<string, unknown>;
  created_at: Date;
}

export interface CreateEmbeddingRequest {
  input: string | string[];
  model?: string;
}

export interface CreateEmbeddingResponse {
  embeddings: number[][];
  tokens_used: number;
  cost: number;
}

export interface StoreEmbeddingRequest {
  content_type: string;
  content_id: string;
  content_text: string;
  model_id?: string;
}

export interface SemanticSearchRequest {
  query: string;
  content_type?: string;
  limit?: number;
  similarity_threshold?: number;
}

export interface SemanticSearchResult {
  id: string;
  content_type: string;
  content_id: string;
  content_text: string;
  similarity: number;
  metadata: Record<string, unknown>;
}

// =============================================================================
// Prompt Template Types
// =============================================================================

export interface AiPromptTemplateRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  category: string | null;
  system_prompt: string | null;
  user_prompt_template: string;
  variables: PromptVariable[];
  recommended_model_id: string | null;
  default_temperature: number;
  default_max_tokens: number;
  usage_count: number;
  is_enabled: boolean;
  is_public: boolean;
  created_by: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface PromptVariable {
  name: string;
  type: string;
  required?: boolean;
  default?: string;
}

export interface CreatePromptTemplateRequest {
  name: string;
  description?: string;
  category?: string;
  system_prompt?: string;
  user_prompt_template: string;
  variables?: PromptVariable[];
  recommended_model_id?: string;
  default_temperature?: number;
  default_max_tokens?: number;
  is_public?: boolean;
}

export interface RenderPromptRequest {
  variables: Record<string, string>;
}

// =============================================================================
// Function Types
// =============================================================================

export interface AiFunctionRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string;
  parameters_schema: Record<string, unknown>;
  implementation_type: string;
  implementation_config: Record<string, unknown>;
  is_enabled: boolean;
  timeout_seconds: number;
  call_count: number;
  success_count: number;
  failure_count: number;
  avg_latency_ms: number | null;
  created_at: Date;
  updated_at: Date;
}

export interface AiFunctionCallRecord {
  id: string;
  source_account_id: string;
  function_id: string;
  message_id: string | null;
  conversation_id: string | null;
  user_id: string | null;
  arguments: Record<string, unknown>;
  response: Record<string, unknown> | null;
  status: AiRequestStatus;
  error_message: string | null;
  started_at: Date;
  completed_at: Date | null;
  latency_ms: number | null;
}

// =============================================================================
// Quota Types
// =============================================================================

export interface AiUsageQuotaRecord {
  id: string;
  source_account_id: string;
  quota_type: string;
  scope_id: string | null;
  model_id: string | null;
  max_requests_per_day: number | null;
  max_tokens_per_day: number | null;
  max_cost_per_day: number | null;
  current_requests: number;
  current_tokens: number;
  current_cost: number;
  last_reset_at: Date;
  is_enabled: boolean;
  created_at: Date;
  updated_at: Date;
}

export interface QuotaStatus {
  max_requests_per_day: number | null;
  max_tokens_per_day: number | null;
  max_cost_per_day: number | null;
  current_requests: number;
  current_tokens: number;
  current_cost: number;
  remaining_requests: number | null;
  remaining_tokens: number | null;
  remaining_cost: number | null;
  reset_at: string;
}

export interface SetQuotaRequest {
  quota_type: string;
  scope_id?: string;
  model_id?: string;
  max_requests_per_day?: number;
  max_tokens_per_day?: number;
  max_cost_per_day?: number;
}

// =============================================================================
// Feature Types
// =============================================================================

export interface AiFeatureRecord {
  id: string;
  source_account_id: string;
  feature_name: string;
  feature_type: string;
  description: string | null;
  prompt_template_id: string | null;
  default_model_id: string | null;
  is_enabled: boolean;
  requires_permission: boolean;
  allowed_roles: string[];
  usage_count: number;
  created_at: Date;
  updated_at: Date;
}

export interface SummarizeRequest {
  messages: string[];
  language?: string;
  model?: string;
}

export interface TranslateRequest {
  text: string;
  target_language: string;
  source_language?: string;
  model?: string;
}

export interface SentimentRequest {
  text: string;
  model?: string;
}

export interface SentimentResponse {
  sentiment: 'positive' | 'negative' | 'neutral';
  confidence: number;
}

// =============================================================================
// Usage / Analytics Types
// =============================================================================

export interface UsageBreakdown {
  date?: string;
  model?: string;
  provider?: string;
  requests: number;
  tokens: number;
  cost: number;
}

export interface UsageResponse {
  total_requests: number;
  total_tokens: number;
  total_cost: number;
  breakdown: UsageBreakdown[];
}
