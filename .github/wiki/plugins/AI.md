# AI Plugin

Unified AI gateway with multi-provider LLM support, conversation management, embeddings, semantic search, prompt templates, function calling, usage quotas, and built-in features (summarize, translate, sentiment analysis).

| Property | Value |
|----------|-------|
| **Port** | `3705` |
| **Category** | `integrations` |
| **Multi-App** | `source_account_id` (UUID) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
nself plugin run ai init
nself plugin run ai server
```

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `AI_PLUGIN_PORT` | `3705` | Server port |
| `AI_PLUGIN_HOST` | `0.0.0.0` | Server host |
| `AI_DEFAULT_PROVIDER` | `openai` | Default AI provider |
| `AI_DEFAULT_MODEL` | `gpt-4o` | Default model ID |
| `AI_DEFAULT_TEMPERATURE` | `0.7` | Default temperature |
| `AI_DEFAULT_MAX_TOKENS` | `2048` | Default max tokens |
| `AI_OPENAI_API_KEY` | - | OpenAI API key |
| `AI_OPENAI_ORG_ID` | - | OpenAI organization ID |
| `AI_OPENAI_BASE_URL` | - | Custom OpenAI-compatible endpoint |
| `AI_ANTHROPIC_API_KEY` | - | Anthropic API key |
| `AI_ANTHROPIC_BASE_URL` | - | Custom Anthropic endpoint |
| `AI_GOOGLE_API_KEY` | - | Google AI (Gemini) API key |
| `AI_COHERE_API_KEY` | - | Cohere API key |
| `AI_LOCAL_BASE_URL` | - | Local model endpoint (e.g., Ollama) |
| `AI_EMBEDDINGS_PROVIDER` | `openai` | Embeddings provider |
| `AI_EMBEDDINGS_MODEL` | `text-embedding-3-small` | Embeddings model |
| `AI_EMBEDDINGS_DIMENSIONS` | `1536` | Embedding vector dimensions |
| `AI_RATE_LIMIT_REQUESTS_PER_MINUTE` | `60` | Provider rate limit (RPM) |
| `AI_RATE_LIMIT_TOKENS_PER_MINUTE` | `100000` | Provider rate limit (TPM) |
| `AI_QUOTA_ENABLED` | `false` | Enable usage quotas |
| `AI_QUOTA_DEFAULT_DAILY_REQUESTS` | `1000` | Default daily request quota |
| `AI_QUOTA_DEFAULT_DAILY_TOKENS` | `100000` | Default daily token quota |
| `AI_FEATURE_SUMMARIZE_ENABLED` | `true` | Enable summarize feature |
| `AI_FEATURE_TRANSLATE_ENABLED` | `true` | Enable translate feature |
| `AI_FEATURE_SENTIMENT_ENABLED` | `true` | Enable sentiment feature |
| `AI_CACHE_ENABLED` | `false` | Enable response caching |
| `AI_CACHE_TTL_SECONDS` | `3600` | Cache TTL |
| `AI_MONITORING_ENABLED` | `false` | Enable request monitoring |
| `AI_API_KEY` | - | API key for plugin authentication |
| `AI_RATE_LIMIT_MAX` | `200` | Plugin rate limit (requests/window) |
| `AI_RATE_LIMIT_WINDOW_MS` | `60000` | Plugin rate limit window (ms) |

### Supported Providers

| Provider | Type | Models |
|----------|------|--------|
| `openai` | Cloud | GPT-4o, GPT-4, GPT-3.5-turbo, text-embedding-3-small/large |
| `anthropic` | Cloud | Claude 3.5 Sonnet, Claude 3 Opus/Haiku |
| `google` | Cloud | Gemini Pro, Gemini Ultra |
| `cohere` | Cloud | Command R+, Embed v3 |
| `huggingface` | Cloud | Any HuggingFace Inference API model |
| `local` | Self-hosted | Ollama, llama.cpp, vLLM, any OpenAI-compatible API |
| `custom` | Custom | Any provider with a compatible adapter |

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (10 tables) |
| `server` | Start the HTTP API server (`-p`/`--port`) |
| `status` | Show model counts, request totals, quota status |
| `models` | List registered models (`--provider`, `--type`) |
| `chat` | Interactive chat session (`--model`, `--system`) |
| `usage` | View usage statistics (`--days`, `--provider`, `--model`) |
| `prompts` | List prompt templates (`--category`) |

---

## REST API

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (DB) |

### Chat Completions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/ai/chat/completions` | Create chat completion (body: `model?`, `messages[]`, `temperature?`, `max_tokens?`, `functions?`, `stream?`) |

### Conversations

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/ai/conversations` | Create conversation (body: `title?`, `model?`, `system_prompt?`, `metadata?`) |
| `GET` | `/api/ai/conversations` | List conversations (query: `limit?`, `offset?`) |
| `GET` | `/api/ai/conversations/:id` | Get conversation with messages |
| `DELETE` | `/api/ai/conversations/:id` | Delete conversation |
| `GET` | `/api/ai/conversations/:id/messages` | List messages in conversation |

### Embeddings

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/ai/embeddings/create` | Create embedding (body: `text`, `model?`, `collection?`, `document_id?`, `metadata?`) |
| `POST` | `/api/ai/embeddings/search` | Semantic search (body: `query`, `collection?`, `limit?`, `threshold?`) |

### Models

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/ai/models` | Register model (body: `provider`, `model_id`, `display_name`, `model_type`, `config?`) |
| `GET` | `/api/ai/models` | List models (query: `provider?`, `type?`) |
| `GET` | `/api/ai/models/:id` | Get model details |
| `DELETE` | `/api/ai/models/:id` | Delete model |

### Prompt Templates

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/ai/prompts` | Create template (body: `name`, `template`, `category?`, `variables?`, `metadata?`) |
| `GET` | `/api/ai/prompts` | List templates (query: `category?`) |
| `GET` | `/api/ai/prompts/:id` | Get template |
| `POST` | `/api/ai/prompts/:id/render` | Render template with variables (body: `variables: {}`) |
| `DELETE` | `/api/ai/prompts/:id` | Delete template |

### Usage & Quotas

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/ai/usage` | Get usage breakdown (query: `days?`, `group_by?`) |
| `GET` | `/api/ai/quota` | Check quota status for current account |

### Built-in Features

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/ai/features` | List enabled features |
| `POST` | `/api/ai/features/summarize` | Summarize text (body: `text`, `max_length?`, `style?`) |
| `POST` | `/api/ai/features/translate` | Translate text (body: `text`, `target_language`, `source_language?`) |
| `POST` | `/api/ai/features/sentiment` | Analyze sentiment (body: `text`) |

---

## Webhook Events

| Event | Description |
|-------|-------------|
| `ai.request.completed` | AI request completed successfully |
| `ai.request.failed` | AI request failed |
| `ai.conversation.created` | New conversation created |
| `ai.conversation.deleted` | Conversation deleted |
| `ai.quota.warning` | Approaching quota limit (80%) |
| `ai.quota.exceeded` | Quota exceeded |
| `ai.model.registered` | New model registered |

---

## Prompt Templates

Templates use `{{variable}}` syntax for variable interpolation:

```
Summarize the following {{document_type}} in {{language}}:

{{content}}

Focus on: {{focus_areas}}
```

Render with `POST /api/ai/prompts/:id/render`:

```json
{
  "variables": {
    "document_type": "research paper",
    "language": "English",
    "content": "...",
    "focus_areas": "methodology and results"
  }
}
```

---

## Database Schema

### `np_ai_models`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Model record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `provider` | `VARCHAR(50)` | Provider name |
| `model_id` | `VARCHAR(255)` | Provider model identifier |
| `display_name` | `VARCHAR(255)` | Human-readable name |
| `model_type` | `VARCHAR(50)` | `chat`, `completion`, `embedding`, `image`, `audio`, `code` |
| `is_enabled` | `BOOLEAN` | Whether model is available |
| `config` | `JSONB` | Model-specific configuration |
| `max_tokens` | `INTEGER` | Maximum context window |
| `cost_per_input_token` | `DECIMAL` | Cost per input token |
| `cost_per_output_token` | `DECIMAL` | Cost per output token |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `np_ai_conversations`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Conversation ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `title` | `VARCHAR(255)` | Conversation title |
| `model` | `VARCHAR(255)` | Model used |
| `system_prompt` | `TEXT` | System prompt |
| `message_count` | `INTEGER` | Number of messages |
| `total_tokens` | `INTEGER` | Total tokens consumed |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last activity |

### `np_ai_messages`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Message ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `conversation_id` | `UUID` (FK) | References `np_ai_conversations` |
| `role` | `VARCHAR(50)` | `system`, `user`, `assistant`, `function` |
| `content` | `TEXT` | Message content |
| `function_call` | `JSONB` | Function call data |
| `token_count` | `INTEGER` | Tokens in this message |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Timestamp |

### `np_ai_requests`

Tracks every API request for usage analytics.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Request ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `provider` | `VARCHAR(50)` | Provider used |
| `model` | `VARCHAR(255)` | Model used |
| `request_type` | `VARCHAR(50)` | `chat`, `embedding`, `feature` |
| `input_tokens` | `INTEGER` | Input token count |
| `output_tokens` | `INTEGER` | Output token count |
| `total_tokens` | `INTEGER` | Total tokens |
| `latency_ms` | `INTEGER` | Response latency |
| `status` | `VARCHAR(20)` | `success` or `error` |
| `error_message` | `TEXT` | Error details |
| `cost` | `DECIMAL` | Estimated cost |
| `metadata` | `JSONB` | Request metadata |
| `created_at` | `TIMESTAMPTZ` | Timestamp |

### `np_ai_embeddings`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Embedding ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `collection` | `VARCHAR(255)` | Logical grouping |
| `document_id` | `VARCHAR(255)` | Source document ID |
| `content` | `TEXT` | Original text |
| `embedding` | `VECTOR` | Vector embedding (requires pgvector) |
| `model` | `VARCHAR(255)` | Model used |
| `dimensions` | `INTEGER` | Vector dimensions |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Timestamp |

### `np_ai_prompt_templates`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Template ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `name` | `VARCHAR(255)` | Template name |
| `template` | `TEXT` | Template content with `{{variables}}` |
| `category` | `VARCHAR(100)` | Category grouping |
| `variables` | `TEXT[]` | Declared variable names |
| `is_active` | `BOOLEAN` | Whether template is active |
| `version` | `INTEGER` | Template version |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `np_ai_functions`, `np_ai_function_calls`

Function calling support. Register functions that AI models can invoke, and track all function call invocations.

### `np_ai_usage_quotas`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Quota ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `scope` | `VARCHAR(50)` | `account`, `user`, `model` |
| `scope_id` | `VARCHAR(255)` | Scope identifier |
| `daily_request_limit` | `INTEGER` | Max requests per day |
| `daily_token_limit` | `INTEGER` | Max tokens per day |
| `daily_requests_used` | `INTEGER` | Requests used today |
| `daily_tokens_used` | `INTEGER` | Tokens used today |
| `last_reset_at` | `TIMESTAMPTZ` | Last daily reset |

### `np_ai_features`

Tracks enabled/disabled status and configuration for built-in AI features (summarize, translate, sentiment).

---

## Troubleshooting

**"Provider API key not configured"** -- Set the appropriate `AI_<PROVIDER>_API_KEY` environment variable.

**Embeddings search returns no results** -- Ensure pgvector extension is installed. Check that embeddings exist in the specified collection with `GET /api/ai/embeddings`.

**Quota exceeded** -- Check current usage with `GET /api/ai/quota`. Quotas reset daily at the time specified by `last_reset_at`. Increase limits by updating the quota record.

**High latency** -- Enable caching with `AI_CACHE_ENABLED=true`. Use smaller/faster models. Check provider rate limits.
