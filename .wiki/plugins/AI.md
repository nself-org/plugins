# AI Plugin

Unified AI gateway with multi-provider LLM support, embeddings, semantic search, prompt templates, and usage tracking

---

## Table of Contents
- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Features](#features)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The AI plugin provides a unified gateway to multiple LLM providers (OpenAI, Anthropic, Google, Cohere, local models), enabling:

- **Multi-Provider Support**: Switch between AI providers without changing your code
- **Conversation Management**: Track multi-turn conversations with full history
- **Embeddings**: Generate and store vector embeddings for semantic search
- **Prompt Templates**: Reusable prompt templates with variable substitution
- **Usage Tracking**: Monitor requests, tokens, and costs across all providers
- **Quotas**: Set daily limits on requests, tokens, or spending
- **Function Calling**: Support for tool use and function calling patterns
- **Streaming**: Real-time streaming responses (when supported by provider)

### Key Features

- Multi-provider LLM gateway (OpenAI, Anthropic, Google, Cohere, local)
- Conversation history and context management
- Vector embeddings and semantic search
- Prompt template library
- Usage tracking and cost monitoring
- Per-user and per-model quotas
- Function/tool calling support
- Model configuration and priority management

### Use Cases

- Chatbots and virtual assistants
- Content generation and summarization
- Translation and multilingual support
- Sentiment analysis
- Semantic search across documents
- RAG (Retrieval Augmented Generation) pipelines
- Multi-turn conversational AI

---

## Quick Start

```bash
# Install
nself plugin install ai

# Configure (.env)
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
AI_OPENAI_ENABLED=true
AI_OPENAI_API_KEY=sk-...
AI_ANTHROPIC_ENABLED=true
AI_ANTHROPIC_API_KEY=sk-ant-...
AI_DEFAULT_PROVIDER=openai

# Initialize
nself plugin ai init

# Start server
nself plugin ai server
```

Test the API:
```bash
# Check status
curl http://localhost:3705/health

# Send a chat completion
curl -X POST http://localhost:3705/api/ai/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [{"role": "user", "content": "Hello!"}],
    "temperature": 0.7
  }'
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | **Yes** | - | PostgreSQL connection string |
| `AI_PLUGIN_PORT` | No | `3705` | HTTP server port |
| `AI_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| **OpenAI** |||
| `AI_OPENAI_ENABLED` | No | `false` | Enable OpenAI provider |
| `AI_OPENAI_API_KEY` | No | - | OpenAI API key |
| `AI_OPENAI_ORG_ID` | No | - | OpenAI organization ID |
| `AI_OPENAI_DEFAULT_MODEL` | No | `gpt-4-turbo` | Default OpenAI model |
| **Anthropic** |||
| `AI_ANTHROPIC_ENABLED` | No | `false` | Enable Anthropic provider |
| `AI_ANTHROPIC_API_KEY` | No | - | Anthropic API key |
| `AI_ANTHROPIC_DEFAULT_MODEL` | No | `claude-3-opus-20240229` | Default Anthropic model |
| **Google** |||
| `AI_GOOGLE_ENABLED` | No | `false` | Enable Google (Gemini) provider |
| `AI_GOOGLE_API_KEY` | No | - | Google API key |
| `AI_GOOGLE_DEFAULT_MODEL` | No | `gemini-pro` | Default Google model |
| **Cohere** |||
| `AI_COHERE_ENABLED` | No | `false` | Enable Cohere provider |
| `AI_COHERE_API_KEY` | No | - | Cohere API key |
| **Local Models** |||
| `AI_LOCAL_ENABLED` | No | `false` | Enable local models (Ollama, etc.) |
| `AI_LOCAL_BASE_URL` | No | `http://localhost:11434` | Local model server URL |
| `AI_LOCAL_DEFAULT_MODEL` | No | `llama2` | Default local model |
| **Defaults** |||
| `AI_DEFAULT_PROVIDER` | No | `openai` | Default AI provider (openai, anthropic, google, cohere, local) |
| `AI_DEFAULT_TEMPERATURE` | No | `0.7` | Default temperature (0.0-2.0) |
| `AI_DEFAULT_MAX_TOKENS` | No | `1000` | Default max tokens per request |
| `AI_ENABLE_STREAMING` | No | `true` | Enable streaming responses |
| `AI_ENABLE_FUNCTION_CALLING` | No | `true` | Enable function/tool calling |
| **Embeddings** |||
| `AI_EMBEDDINGS_ENABLED` | No | `true` | Enable embeddings API |
| `AI_EMBEDDINGS_MODEL` | No | `text-embedding-3-large` | Embeddings model |
| `AI_EMBEDDINGS_DIMENSIONS` | No | `1536` | Embedding vector dimensions |
| **Rate Limiting** |||
| `AI_RATE_LIMIT_ENABLED` | No | `true` | Enable rate limiting |
| `AI_RATE_LIMIT_REQUESTS_PER_MINUTE` | No | `60` | Max requests per minute |
| `AI_RATE_LIMIT_TOKENS_PER_MINUTE` | No | `90000` | Max tokens per minute |
| **Quotas** |||
| `AI_DEFAULT_DAILY_REQUESTS` | No | `1000` | Default daily request quota |
| `AI_DEFAULT_DAILY_TOKENS` | No | `100000` | Default daily token quota |
| `AI_DEFAULT_DAILY_COST` | No | `10.00` | Default daily cost quota ($) |
| **Features** |||
| `AI_CHAT_ASSISTANT_ENABLED` | No | `true` | Enable chat assistant feature |
| `AI_SUMMARIZATION_ENABLED` | No | `true` | Enable summarization feature |
| `AI_TRANSLATION_ENABLED` | No | `true` | Enable translation feature |
| `AI_SENTIMENT_ANALYSIS_ENABLED` | No | `false` | Enable sentiment analysis |
| `AI_SMART_SEARCH_ENABLED` | No | `true` | Enable semantic search |
| **Caching** |||
| `AI_CACHE_ENABLED` | No | `true` | Enable response caching |
| `AI_CACHE_TTL_SECONDS` | No | `3600` | Cache TTL in seconds |
| **Monitoring** |||
| `AI_LOG_REQUESTS` | No | `true` | Log all requests |
| `AI_LOG_RESPONSES` | No | `false` | Log all responses (verbose) |
| `AI_TRACK_COSTS` | No | `true` | Track and calculate costs |
| **Security** |||
| `AI_API_KEY` | No | - | API key for securing the HTTP server |
| `AI_RATE_LIMIT_MAX` | No | `100` | Max requests per window |
| `AI_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (ms) |

### Example .env File
```bash
# Database
DATABASE_URL=postgresql://user:pass@localhost:5432/nself

# Server
AI_PLUGIN_PORT=3705

# OpenAI
AI_OPENAI_ENABLED=true
AI_OPENAI_API_KEY=sk-proj-...
AI_OPENAI_DEFAULT_MODEL=gpt-4-turbo

# Anthropic
AI_ANTHROPIC_ENABLED=true
AI_ANTHROPIC_API_KEY=sk-ant-api03-...
AI_ANTHROPIC_DEFAULT_MODEL=claude-3-opus-20240229

# Defaults
AI_DEFAULT_PROVIDER=openai
AI_DEFAULT_TEMPERATURE=0.7
AI_DEFAULT_MAX_TOKENS=2000

# Embeddings
AI_EMBEDDINGS_ENABLED=true
AI_EMBEDDINGS_MODEL=text-embedding-3-large

# Quotas
AI_DEFAULT_DAILY_REQUESTS=1000
AI_DEFAULT_DAILY_TOKENS=100000
AI_DEFAULT_DAILY_COST=10.00

# Security
AI_API_KEY=your-secure-api-key
```

---

## CLI Commands

### `init`
Initialize the AI plugin database schema.

```bash
nself plugin ai init
```

### `server`
Start the AI plugin HTTP server.

```bash
nself plugin ai server
nself plugin ai server --port 3705
```

### `status`
Show AI plugin status, enabled providers, and model counts.

```bash
nself plugin ai status
```

Output:
```
AI Plugin Status
=================
Port:           3705
Default:        openai
Models:         8 total, 6 enabled
Features:       5 enabled
Embeddings:     enabled
Streaming:      enabled

Providers:
  OpenAI:       enabled
  Anthropic:    enabled
  Google:       disabled
  Local:        disabled

Enabled Models:
  - gpt-4-turbo (openai) [default]
  - gpt-3.5-turbo (openai)
  - claude-3-opus-20240229 (anthropic)
```

### `models`
List all AI models.

```bash
nself plugin ai models
nself plugin ai models --enabled  # Only show enabled models
```

### `chat`
Send a chat completion request.

```bash
nself plugin ai chat --prompt "Explain quantum computing"
nself plugin ai chat --prompt "Hello" --model gpt-4-turbo --temperature 0.9
```

### `usage`
View AI usage statistics.

```bash
nself plugin ai usage
nself plugin ai usage --start-date 2026-01-01 --end-date 2026-01-31
nself plugin ai usage --group-by model  # or 'provider', 'day'
```

Output:
```
AI Usage Statistics
====================
Total Requests: 1,247
Total Tokens:   523,891
Total Cost:     $12.45

Breakdown by model:
  gpt-4-turbo: 452 requests, 234,123 tokens, $8.23
  claude-3-opus: 395 requests, 189,768 tokens, $4.22
```

### `prompts`
List prompt templates.

```bash
nself plugin ai prompts
nself plugin ai prompts --category translation
```

---

## REST API

### Base URL
```
http://localhost:3705
```

All endpoints support multi-app isolation via `X-Source-Account-Id` header.

---

### Health Checks

#### `GET /health`
Basic health check.

**Response:**
```json
{
  "status": "ok",
  "plugin": "ai",
  "timestamp": "2026-02-11T10:30:00.000Z"
}
```

#### `GET /ready`
Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "ai",
  "timestamp": "2026-02-11T10:30:00.000Z"
}
```

---

### Chat Completions

#### `POST /api/ai/chat/completions`
Send a chat completion request.

**Request Body:**
```json
{
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "What is the capital of France?"}
  ],
  "model": "gpt-4-turbo",
  "temperature": 0.7,
  "max_tokens": 1000,
  "stream": false,
  "user_id": "user_123",
  "conversation_id": "conv_abc123"
}
```

**Response:**
```json
{
  "id": "req_xyz789",
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "The capital of France is Paris."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 25,
    "completion_tokens": 10,
    "total_tokens": 35
  },
  "cost": 0.00035,
  "model": "gpt-4-turbo",
  "latency_ms": 1240
}
```

---

### Conversations

#### `POST /api/ai/conversations`
Create a new conversation.

**Request Body:**
```json
{
  "user_id": "user_123",
  "model_id": "model_uuid",
  "system_prompt": "You are a helpful assistant.",
  "title": "My Conversation"
}
```

**Response:**
```json
{
  "conversation_id": "conv_abc123"
}
```

#### `GET /api/ai/conversations/:id`
Get a conversation with message history.

**Response:**
```json
{
  "id": "conv_abc123",
  "user_id": "user_123",
  "model_id": "model_uuid",
  "title": "My Conversation",
  "message_count": 4,
  "total_tokens": 1250,
  "total_cost": 0.0125,
  "created_at": "2026-02-11T10:00:00.000Z",
  "messages": [
    {
      "role": "user",
      "content": "Hello!",
      "created_at": "2026-02-11T10:00:01.000Z"
    },
    {
      "role": "assistant",
      "content": "Hi! How can I help you?",
      "created_at": "2026-02-11T10:00:03.000Z"
    }
  ]
}
```

#### `GET /api/ai/conversations`
List conversations.

**Query Parameters:**
- `user_id` (optional): Filter by user
- `limit` (optional): Limit results (default: 50)

**Response:**
```json
{
  "conversations": [
    {
      "id": "conv_abc123",
      "user_id": "user_123",
      "title": "My Conversation",
      "message_count": 4,
      "last_message_at": "2026-02-11T10:05:00.000Z"
    }
  ]
}
```

#### `POST /api/ai/conversations/:id/messages`
Add a message to a conversation (and get AI response).

**Request Body:**
```json
{
  "content": "What's the weather like?",
  "role": "user"
}
```

**Response:**
```json
{
  "message_id": "msg_xyz789",
  "response": {
    "content": "I don't have access to real-time weather data...",
    "np_tokens_used": 45,
    "cost": 0.00045
  }
}
```

#### `DELETE /api/ai/conversations/:id`
Delete a conversation.

**Response:**
```json
{
  "deleted": true
}
```

---

### Embeddings

#### `POST /api/ai/embeddings/create`
Generate and store an embedding.

**Request Body:**
```json
{
  "content_type": "document",
  "content_id": "doc_123",
  "content_text": "This is the text to embed..."
}
```

**Response:**
```json
{
  "embedding_id": "emb_xyz789"
}
```

#### `POST /api/ai/embeddings/search`
Perform semantic search using embeddings.

**Request Body:**
```json
{
  "query": "quantum computing concepts",
  "content_type": "document",
  "limit": 10,
  "similarity_threshold": 0.7
}
```

**Response:**
```json
{
  "results": [
    {
      "id": "emb_xyz789",
      "content_type": "document",
      "content_id": "doc_123",
      "content_text": "Quantum computing uses qubits...",
      "similarity": 0.92,
      "metadata": {}
    }
  ]
}
```

---

### Models

#### `GET /api/ai/models`
List all AI models.

**Response:**
```json
{
  "models": [
    {
      "id": "model_uuid",
      "provider": "openai",
      "model_id": "gpt-4-turbo",
      "model_name": "GPT-4 Turbo",
      "model_type": "chat",
      "is_enabled": true,
      "is_default": true,
      "supports_streaming": true,
      "supports_functions": true,
      "max_tokens": 128000,
      "context_window": 128000
    }
  ]
}
```

#### `POST /api/ai/models`
Register a new AI model.

**Request Body:**
```json
{
  "provider": "openai",
  "model_id": "gpt-4",
  "model_name": "GPT-4",
  "model_type": "chat",
  "max_tokens": 8192,
  "context_window": 8192,
  "supports_streaming": true,
  "supports_functions": true,
  "input_price_per_million": 30.0,
  "output_price_per_million": 60.0,
  "is_default": false
}
```

**Response:**
```json
{
  "model": {
    "id": "model_uuid",
    "provider": "openai",
    "model_id": "gpt-4",
    "model_name": "GPT-4"
  }
}
```

#### `PATCH /api/ai/models/:id`
Update a model.

**Request Body:**
```json
{
  "is_enabled": false,
  "priority": 10
}
```

#### `DELETE /api/ai/models/:id`
Delete a model.

**Response:**
```json
{
  "deleted": true
}
```

---

### Prompt Templates

#### `GET /api/ai/prompts`
List prompt templates.

**Query Parameters:**
- `category` (optional): Filter by category

**Response:**
```json
{
  "templates": [
    {
      "id": "tmpl_abc123",
      "name": "summarize_article",
      "category": "summarization",
      "description": "Summarize an article",
      "usage_count": 42
    }
  ]
}
```

#### `POST /api/ai/prompts`
Create a prompt template.

**Request Body:**
```json
{
  "name": "translate_text",
  "description": "Translate text to another language",
  "category": "translation",
  "system_prompt": "You are a professional translator.",
  "user_prompt_template": "Translate the following text to {{language}}: {{text}}",
  "variables": [
    {"name": "language", "type": "string", "required": true},
    {"name": "text", "type": "string", "required": true}
  ],
  "default_temperature": 0.3,
  "is_public": true
}
```

**Response:**
```json
{
  "template_id": "tmpl_xyz789"
}
```

#### `POST /api/ai/prompts/:id/render`
Render a prompt template with variables.

**Request Body:**
```json
{
  "variables": {
    "language": "Spanish",
    "text": "Hello, how are you?"
  }
}
```

**Response:**
```json
{
  "rendered_prompt": "Translate the following text to Spanish: Hello, how are you?"
}
```

---

### Usage & Quotas

#### `GET /api/ai/usage`
Get usage statistics.

**Query Parameters:**
- `start_date` (optional): Start date (YYYY-MM-DD)
- `end_date` (optional): End date (YYYY-MM-DD)
- `group_by` (optional): `day`, `model`, or `provider`
- `user_id` (optional): Filter by user

**Response:**
```json
{
  "total_requests": 1247,
  "total_tokens": 523891,
  "total_cost": 12.45,
  "breakdown": [
    {
      "model": "gpt-4-turbo",
      "requests": 452,
      "tokens": 234123,
      "cost": 8.23
    },
    {
      "model": "claude-3-opus",
      "requests": 395,
      "tokens": 189768,
      "cost": 4.22
    }
  ]
}
```

#### `GET /api/ai/quota`
Check quota status for a user.

**Query Parameters:**
- `user_id` (required)

**Response:**
```json
{
  "max_requests_per_day": 1000,
  "max_tokens_per_day": 100000,
  "max_cost_per_day": 10.0,
  "current_requests": 245,
  "current_tokens": 34567,
  "current_cost": 1.23,
  "remaining_requests": 755,
  "remaining_tokens": 65433,
  "remaining_cost": 8.77,
  "reset_at": "2026-02-12T00:00:00.000Z"
}
```

#### `POST /api/ai/quota`
Set quota for a user or model.

**Request Body:**
```json
{
  "quota_type": "user",
  "scope_id": "user_123",
  "max_requests_per_day": 500,
  "max_tokens_per_day": 50000,
  "max_cost_per_day": 5.0
}
```

**Response:**
```json
{
  "quota": {
    "id": "quota_uuid",
    "quota_type": "user",
    "scope_id": "user_123",
    "max_requests_per_day": 500,
    "max_tokens_per_day": 50000,
    "max_cost_per_day": 5.0
  }
}
```

---

### AI Features

#### `GET /api/ai/features`
List enabled AI features.

**Response:**
```json
{
  "features": [
    {
      "id": "feat_uuid",
      "feature_name": "np_chat_assistant",
      "feature_type": "chat",
      "is_enabled": true,
      "usage_count": 1234
    }
  ]
}
```

#### `POST /api/ai/features/summarize`
Summarize multiple messages.

**Request Body:**
```json
{
  "messages": [
    "Message 1 content...",
    "Message 2 content...",
    "Message 3 content..."
  ],
  "language": "English"
}
```

**Response:**
```json
{
  "summary": "This is a summary of the messages...",
  "np_tokens_used": 145
}
```

#### `POST /api/ai/features/translate`
Translate text.

**Request Body:**
```json
{
  "text": "Hello, how are you?",
  "target_language": "Spanish",
  "source_language": "English"
}
```

**Response:**
```json
{
  "translated_text": "Hola, ¿cómo estás?"
}
```

#### `POST /api/ai/features/sentiment`
Analyze sentiment.

**Request Body:**
```json
{
  "text": "I love this product! It's amazing!"
}
```

**Response:**
```json
{
  "sentiment": "positive",
  "confidence": 0.95
}
```

---

## Database Schema

### `np_ai_models`
Stores AI model configurations.

```sql
CREATE TABLE np_ai_models (
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

CREATE INDEX idx_ai_models_account ON np_ai_models(source_account_id);
CREATE INDEX idx_ai_models_enabled ON np_ai_models(source_account_id, is_enabled, priority);
CREATE INDEX idx_ai_models_type ON np_ai_models(source_account_id, model_type, is_enabled);
CREATE INDEX idx_ai_models_default ON np_ai_models(source_account_id, is_default) WHERE is_default = true;
```

**Columns:**
| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | Generated | Primary key |
| `source_account_id` | VARCHAR(128) | No | `'primary'` | Multi-app isolation |
| `provider` | VARCHAR(50) | No | - | Provider name (openai, anthropic, google, etc.) |
| `model_id` | VARCHAR(255) | No | - | Provider's model identifier |
| `model_name` | VARCHAR(255) | No | - | Human-readable model name |
| `model_type` | VARCHAR(50) | No | - | Type (chat, completion, embedding, etc.) |
| `supports_streaming` | BOOLEAN | No | `false` | Whether model supports streaming |
| `supports_functions` | BOOLEAN | No | `false` | Whether model supports function calling |
| `supports_vision` | BOOLEAN | No | `false` | Whether model supports vision/images |
| `max_tokens` | INTEGER | No | - | Maximum tokens per request |
| `context_window` | INTEGER | No | - | Total context window size |
| `input_price_per_million` | DECIMAL(10,4) | Yes | - | Cost per million input tokens |
| `output_price_per_million` | DECIMAL(10,4) | Yes | - | Cost per million output tokens |
| `default_temperature` | DECIMAL(3,2) | Yes | `0.7` | Default temperature |
| `default_top_p` | DECIMAL(3,2) | Yes | `1.0` | Default top_p |
| `default_max_tokens` | INTEGER | Yes | `1000` | Default max tokens |
| `is_enabled` | BOOLEAN | No | `true` | Whether model is enabled |
| `is_default` | BOOLEAN | No | `false` | Whether this is the default model |
| `priority` | INTEGER | No | `100` | Priority for model selection |
| `description` | TEXT | Yes | - | Model description |
| `metadata` | JSONB | Yes | `{}` | Additional metadata |
| `created_at` | TIMESTAMPTZ | No | NOW() | Record creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | NOW() | Record update timestamp |

---

### `np_ai_conversations`
Stores multi-turn conversation contexts.

```sql
CREATE TABLE np_ai_conversations (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255),
  context_type VARCHAR(50),
  context_id VARCHAR(255),
  model_id UUID REFERENCES np_ai_models(id) ON DELETE SET NULL,
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

CREATE INDEX idx_ai_conversations_account ON np_ai_conversations(source_account_id);
CREATE INDEX idx_ai_conversations_user ON np_ai_conversations(source_account_id, user_id, created_at DESC);
CREATE INDEX idx_ai_conversations_context ON np_ai_conversations(source_account_id, context_type, context_id);
```

**Columns:**
| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `source_account_id` | VARCHAR(128) | No | Multi-app isolation |
| `user_id` | VARCHAR(255) | Yes | User identifier |
| `context_type` | VARCHAR(50) | Yes | Context type (channel, ticket, etc.) |
| `context_id` | VARCHAR(255) | Yes | Context identifier |
| `model_id` | UUID | Yes | Reference to np_ai_models |
| `system_prompt` | TEXT | Yes | System prompt for conversation |
| `temperature` | DECIMAL(3,2) | Yes | Conversation temperature |
| `max_tokens` | INTEGER | Yes | Max tokens per message |
| `parameters` | JSONB | Yes | Additional parameters |
| `message_count` | INTEGER | No | Number of messages |
| `total_tokens` | INTEGER | No | Total tokens used |
| `total_cost` | DECIMAL(10,6) | Yes | Total cost |
| `title` | VARCHAR(255) | Yes | Conversation title |
| `metadata` | JSONB | Yes | Additional metadata |
| `created_at` | TIMESTAMPTZ | No | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | Update timestamp |
| `last_message_at` | TIMESTAMPTZ | Yes | Last message timestamp |

---

### `np_ai_messages`
Stores individual messages within conversations.

```sql
CREATE TABLE np_ai_messages (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  conversation_id UUID NOT NULL REFERENCES np_ai_conversations(id) ON DELETE CASCADE,
  role VARCHAR(50) NOT NULL,
  content TEXT NOT NULL,
  function_name VARCHAR(255),
  function_arguments JSONB,
  function_response JSONB,
  np_tokens_used INTEGER,
  model_used VARCHAR(255),
  finish_reason VARCHAR(50),
  cost DECIMAL(10,6),
  latency_ms INTEGER,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_ai_messages_account ON np_ai_messages(source_account_id);
CREATE INDEX idx_ai_messages_conversation ON np_ai_messages(conversation_id, created_at);
CREATE INDEX idx_ai_messages_role ON np_ai_messages(role);
```

---

### `np_ai_requests`
Tracks all AI requests for monitoring and debugging.

```sql
CREATE TABLE np_ai_requests (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255),
  model_id UUID REFERENCES np_ai_models(id) ON DELETE SET NULL,
  provider VARCHAR(50) NOT NULL,
  model_name VARCHAR(255) NOT NULL,
  request_type VARCHAR(50) NOT NULL,
  input_data JSONB NOT NULL,
  output_data JSONB,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  error_message TEXT,
  np_tokens_input INTEGER,
  np_tokens_output INTEGER,
  np_tokens_total INTEGER,
  cost DECIMAL(10,6),
  started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  completed_at TIMESTAMP WITH TIME ZONE,
  latency_ms INTEGER,
  metadata JSONB DEFAULT '{}'
);

CREATE INDEX idx_ai_requests_account ON np_ai_requests(source_account_id);
CREATE INDEX idx_ai_requests_user ON np_ai_requests(source_account_id, user_id, started_at DESC);
CREATE INDEX idx_ai_requests_status ON np_ai_requests(source_account_id, status, started_at);
CREATE INDEX idx_ai_requests_provider ON np_ai_requests(source_account_id, provider, started_at DESC);
```

---

### `np_ai_embeddings`
Stores vector embeddings for semantic search.

```sql
CREATE TABLE np_ai_embeddings (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  content_type VARCHAR(50) NOT NULL,
  content_id VARCHAR(255) NOT NULL,
  content_text TEXT NOT NULL,
  content_hash VARCHAR(64),
  model_id UUID REFERENCES np_ai_models(id) ON DELETE SET NULL,
  embedding_dimensions INTEGER NOT NULL DEFAULT 1536,
  embedding_data JSONB,
  np_tokens_used INTEGER,
  cost DECIMAL(10,6),
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, content_type, content_id, model_id)
);

CREATE INDEX idx_ai_embeddings_account ON np_ai_embeddings(source_account_id);
CREATE INDEX idx_ai_embeddings_content ON np_ai_embeddings(source_account_id, content_type, content_id);
CREATE INDEX idx_ai_embeddings_hash ON np_ai_embeddings(content_hash) WHERE content_hash IS NOT NULL;
```

---

### `np_ai_prompt_templates`
Reusable prompt templates with variable substitution.

```sql
CREATE TABLE np_ai_prompt_templates (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  category VARCHAR(100),
  system_prompt TEXT,
  user_prompt_template TEXT NOT NULL,
  variables JSONB DEFAULT '[]',
  recommended_model_id UUID REFERENCES np_ai_models(id) ON DELETE SET NULL,
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

CREATE INDEX idx_prompt_templates_account ON np_ai_prompt_templates(source_account_id);
CREATE INDEX idx_prompt_templates_category ON np_ai_prompt_templates(source_account_id, category, is_enabled);
```

---

### `np_ai_functions`
Defines callable functions for AI agents.

```sql
CREATE TABLE np_ai_functions (
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

CREATE INDEX idx_ai_functions_account ON np_ai_functions(source_account_id);
CREATE INDEX idx_ai_functions_enabled ON np_ai_functions(source_account_id, is_enabled);
```

---

### `np_ai_function_calls`
Logs all function calls made by AI models.

```sql
CREATE TABLE np_ai_function_calls (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  function_id UUID NOT NULL REFERENCES np_ai_functions(id) ON DELETE CASCADE,
  message_id UUID REFERENCES np_ai_messages(id) ON DELETE SET NULL,
  conversation_id UUID REFERENCES np_ai_conversations(id) ON DELETE SET NULL,
  user_id VARCHAR(255),
  arguments JSONB NOT NULL,
  response JSONB,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  error_message TEXT,
  started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  completed_at TIMESTAMP WITH TIME ZONE,
  latency_ms INTEGER
);

CREATE INDEX idx_function_calls_account ON np_ai_function_calls(source_account_id);
CREATE INDEX idx_function_calls_function ON np_ai_function_calls(function_id, started_at DESC);
CREATE INDEX idx_function_calls_conversation ON np_ai_function_calls(conversation_id);
```

---

### `np_ai_usage_quotas`
Defines and tracks usage quotas.

```sql
CREATE TABLE np_ai_usage_quotas (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  quota_type VARCHAR(50) NOT NULL,
  scope_id VARCHAR(255),
  model_id UUID REFERENCES np_ai_models(id) ON DELETE CASCADE,
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

CREATE INDEX idx_usage_quotas_account ON np_ai_usage_quotas(source_account_id);
CREATE INDEX idx_usage_quotas_scope ON np_ai_usage_quotas(source_account_id, quota_type, scope_id);
```

---

### `np_ai_features`
Tracks enabled AI features (summarization, translation, etc.).

```sql
CREATE TABLE np_ai_features (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  feature_name VARCHAR(255) NOT NULL,
  feature_type VARCHAR(100) NOT NULL,
  description TEXT,
  prompt_template_id UUID REFERENCES np_ai_prompt_templates(id) ON DELETE SET NULL,
  default_model_id UUID REFERENCES np_ai_models(id) ON DELETE SET NULL,
  is_enabled BOOLEAN NOT NULL DEFAULT true,
  requires_permission BOOLEAN NOT NULL DEFAULT false,
  allowed_roles TEXT[] DEFAULT ARRAY['admin', 'owner'],
  usage_count INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, feature_name)
);

CREATE INDEX idx_ai_features_account ON np_ai_features(source_account_id);
CREATE INDEX idx_ai_features_enabled ON np_ai_features(source_account_id, is_enabled);
```

---

## Features

### Multi-Provider Gateway
Switch between AI providers without code changes. Configure multiple providers and the system automatically routes requests based on availability, quotas, and priorities.

### Conversation History
Maintain full context across multi-turn conversations. Messages are automatically stored with token usage and cost tracking.

### Embeddings & Semantic Search
Generate vector embeddings for any content and perform semantic similarity searches. Perfect for RAG pipelines, knowledge bases, and document search.

### Prompt Template Library
Create reusable prompt templates with variable substitution. Share templates across your organization and track usage.

### Cost & Usage Tracking
Every AI request is logged with full token usage and cost breakdown. Set daily quotas per user, per model, or globally.

### Function Calling
Define custom functions that AI models can call. Implement tool use patterns for complex agent workflows.

---

## Examples

### Example 1: Basic Chat Completion

```bash
# Using CLI
nself plugin ai chat --prompt "What is 2+2?" --model gpt-4-turbo

# Using API
curl -X POST http://localhost:3705/api/ai/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [{"role": "user", "content": "What is 2+2?"}],
    "model": "gpt-4-turbo"
  }'
```

---

### Example 2: Multi-Turn Conversation

```sql
-- Create a conversation
SELECT id FROM np_ai_conversations WHERE user_id = 'user_123' ORDER BY created_at DESC LIMIT 1;

-- Get conversation messages
SELECT role, content, np_tokens_used, created_at
FROM np_ai_messages
WHERE source_account_id = 'primary'
  AND conversation_id = 'conv_abc123'
ORDER BY created_at ASC;
```

```http
# Via API
POST /api/ai/conversations
{
  "user_id": "user_123",
  "title": "Math Help"
}

POST /api/ai/conversations/conv_abc123/messages
{
  "content": "What is 2+2?"
}
# Response includes AI reply

POST /api/ai/conversations/conv_abc123/messages
{
  "content": "And what about 3+3?"
}
# Context is maintained
```

---

### Example 3: Generate Embeddings

```http
POST /api/ai/embeddings/create
{
  "content_type": "article",
  "content_id": "article_123",
  "content_text": "Quantum computing uses qubits instead of classical bits..."
}

# Search for similar content
POST /api/ai/embeddings/search
{
  "query": "quantum computers",
  "content_type": "article",
  "limit": 5,
  "similarity_threshold": 0.7
}
```

---

### Example 4: Track Usage and Costs

```sql
-- Daily usage by provider
SELECT
  provider,
  COUNT(*) as requests,
  SUM(np_tokens_total) as total_tokens,
  SUM(cost) as total_cost
FROM np_ai_requests
WHERE source_account_id = 'primary'
  AND started_at >= CURRENT_DATE
GROUP BY provider;

-- User usage for the month
SELECT
  user_id,
  COUNT(*) as requests,
  SUM(np_tokens_total) as tokens,
  SUM(cost) as cost
FROM np_ai_requests
WHERE source_account_id = 'primary'
  AND started_at >= DATE_TRUNC('month', CURRENT_DATE)
GROUP BY user_id
ORDER BY cost DESC
LIMIT 10;
```

---

### Example 5: Set Usage Quotas

```bash
# Set user quota via CLI
curl -X POST http://localhost:3705/api/ai/quota \
  -H "Content-Type: application/json" \
  -d '{
    "quota_type": "user",
    "scope_id": "user_123",
    "max_requests_per_day": 100,
    "max_tokens_per_day": 50000,
    "max_cost_per_day": 2.50
  }'

# Check quota status
curl "http://localhost:3705/api/ai/quota?user_id=user_123"
```

---

### Example 6: Create and Use Prompt Template

```http
# Create template
POST /api/ai/prompts
{
  "name": "code_review",
  "category": "development",
  "system_prompt": "You are an expert code reviewer.",
  "user_prompt_template": "Review this {{language}} code for best practices:\n\n{{code}}",
  "variables": [
    {"name": "language", "type": "string", "required": true},
    {"name": "code", "type": "string", "required": true}
  ],
  "default_temperature": 0.3
}

# Render template
POST /api/ai/prompts/tmpl_xyz789/render
{
  "variables": {
    "language": "Python",
    "code": "def add(a, b):\n    return a + b"
  }
}
```

---

## Troubleshooting

### Common Issues

#### Issue: "No AI model available"
**Cause**: No models are configured or all models are disabled.

**Solution**:
1. Register at least one model via API or database
2. Ensure at least one model has `is_enabled = true`
3. Set a default model with `is_default = true`

```sql
-- Check enabled models
SELECT * FROM np_ai_models WHERE source_account_id = 'primary' AND is_enabled = true;

-- Enable a model
UPDATE np_ai_models SET is_enabled = true WHERE id = 'model_uuid';
```

---

#### Issue: "Usage quota exceeded"
**Cause**: User has reached their daily quota for requests, tokens, or cost.

**Solution**:
1. Check current quota usage
2. Increase quota limits or wait for daily reset
3. Review quota configuration

```bash
# Check quota
curl "http://localhost:3705/api/ai/quota?user_id=user_123"

# Increase quota
curl -X POST http://localhost:3705/api/ai/quota \
  -H "Content-Type: application/json" \
  -d '{
    "quota_type": "user",
    "scope_id": "user_123",
    "max_requests_per_day": 2000
  }'
```

---

#### Issue: Provider API errors
**Cause**: Invalid API keys or provider service issues.

**Solution**:
1. Verify API keys are correct in `.env`
2. Check provider status pages
3. Review error logs

```bash
# Check recent failed requests
SELECT id, provider, model_name, error_message, started_at
FROM np_ai_requests
WHERE source_account_id = 'primary'
  AND status = 'failed'
ORDER BY started_at DESC
LIMIT 10;
```

---

#### Issue: High latency or timeouts
**Cause**: Large context windows, complex requests, or provider throttling.

**Solution**:
1. Reduce max_tokens in requests
2. Use streaming for long responses
3. Consider switching to faster models (e.g., gpt-3.5-turbo instead of gpt-4)
4. Check network connectivity to provider APIs

```sql
-- Find slow requests
SELECT id, provider, model_name, latency_ms, started_at
FROM np_ai_requests
WHERE source_account_id = 'primary'
  AND latency_ms > 5000
ORDER BY latency_ms DESC
LIMIT 20;
```

---

#### Issue: Embeddings not returning similar results
**Cause**: Wrong embedding model, insufficient data, or threshold too high.

**Solution**:
1. Ensure you're using the same embedding model for query and stored embeddings
2. Lower similarity_threshold parameter
3. Verify embedding_data is being stored correctly

```sql
-- Check embeddings
SELECT content_type, COUNT(*) as count, model_id
FROM np_ai_embeddings
WHERE source_account_id = 'primary'
GROUP BY content_type, model_id;
```

---

#### Issue: Prompt templates not rendering variables
**Cause**: Variable names don't match or missing required variables.

**Solution**:
1. Check template variable definitions
2. Ensure all required variables are provided
3. Variable names are case-sensitive

```sql
-- Check template variables
SELECT name, variables FROM np_ai_prompt_templates WHERE id = 'template_id';
```

---

## Support

For issues, questions, or contributions:
- GitHub: https://github.com/acamarata/nself-plugins
- Documentation: https://github.com/acamarata/nself-plugins/wiki

---
