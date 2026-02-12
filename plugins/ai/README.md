# AI Plugin - nTV Integration Assessment

**Plugin**: `ai` (v1.0.0)
**Port**: 3705
**Category**: integrations
**Tables**: 10 (`np_ai_*` prefix)
**Status**: Fully implemented, provider API calls use placeholder responses pending API key configuration

---

## 1. Plugin Overview

The AI plugin is a unified gateway for multi-provider LLM operations. It provides a single API surface over multiple AI providers, with built-in conversation management, embeddings storage, semantic search, prompt templates with variable substitution, function calling orchestration, usage quotas, and cost tracking.

All data is stored in PostgreSQL across 10 tables. The plugin runs as a Fastify HTTP server on port 3705, supports multi-app isolation via `source_account_id`, and includes rate limiting, API key authentication, and CORS.

**Architecture**: Fastify server (`server.ts`) -> Database layer (`database.ts`) -> PostgreSQL. No direct provider SDK dependencies yet; the server scaffolding routes requests through model resolution, quota checking, request tracking, and response storage. Actual provider API calls are stubbed with placeholder responses -- the integration points are clearly marked in the code for OpenAI, Anthropic, Google, Cohere, HuggingFace, and Local/Ollama.

---

## 2. Current Capabilities

### Multi-Provider Gateway

Supported providers (from `types.ts` `AiProvider` union and `config.ts`):

| Provider | Config Prefix | Default Model | Status |
|----------|---------------|---------------|--------|
| OpenAI | `AI_OPENAI_*` | gpt-4-turbo | Configurable |
| Anthropic | `AI_ANTHROPIC_*` | claude-3-opus-20240229 | Configurable |
| Google | `AI_GOOGLE_*` | gemini-pro | Configurable |
| Cohere | `AI_COHERE_*` | (none set) | Configurable |
| HuggingFace | (type only) | -- | Type defined |
| Local/Ollama | `AI_LOCAL_*` | llama2 | Configurable, default URL `localhost:11434` |
| Custom | (type only) | -- | Type defined |

Models are registered in the `ai_models` table with per-model pricing (`input_price_per_million`, `output_price_per_million`), capability flags (`supports_streaming`, `supports_functions`, `supports_vision`), context window sizes, and priority ordering. The system resolves the default model automatically by priority when no explicit model is specified.

**Endpoints**: `POST /api/ai/chat/completions`, `GET/POST/PATCH/DELETE /api/ai/models`

### Conversation Management

Full conversation lifecycle with message history, per-conversation token and cost accumulation, context typing (`context_type` + `context_id` for linking conversations to external entities), and user association.

**Tables**: `ai_conversations`, `ai_messages`
**Endpoints**: `GET/POST/DELETE /api/ai/conversations`, `POST /api/ai/conversations/:id/messages`

### Embeddings and Semantic Search

Stores embeddings with content type/ID addressing (e.g., `content_type: "movie"`, `content_id: "tt0111161"`). Supports configurable dimensions (default 1536 for `text-embedding-3-large`). Content deduplication via hash. Upserts on `(source_account_id, content_type, content_id, model_id)`.

Semantic search endpoint accepts a query string, optional content type filter, limit, and similarity threshold. Current implementation returns stored embeddings sorted by placeholder similarity -- actual cosine similarity computation would be added when provider embeddings are wired up.

**Table**: `ai_embeddings`
**Endpoints**: `POST /api/ai/embeddings/create`, `POST /api/ai/embeddings/search`

### Prompt Templates

Named templates with `{{variable}}` substitution, categorization, recommended model association, temperature/token defaults, usage tracking, and public/private visibility. The `renderPromptTemplate` method performs substitution and increments the usage counter.

**Table**: `ai_prompt_templates`
**Endpoints**: `GET/POST /api/ai/prompts`, `POST /api/ai/prompts/:id/render`

### Function Calling

Function definitions with JSON Schema parameters, implementation type/config, enable/disable control, and per-function metrics (call count, success/failure counts, average latency). Function calls are tracked per-message and per-conversation.

**Tables**: `ai_functions`, `ai_function_calls`

### Usage Quotas and Cost Tracking

Per-user quotas with daily limits on requests, tokens, and cost. Automatic daily reset. Quota checking is integrated into the chat completion flow -- requests are rejected with 429 when quotas are exceeded. All requests are tracked in `ai_requests` with full input/output data, token counts, cost, and latency.

**Tables**: `ai_usage_quotas`, `ai_requests`
**Endpoints**: `GET/POST /api/ai/quota`, `GET /api/ai/usage`

### Built-In Features

- **Summarization**: Accepts an array of messages, returns a summary. Supports language parameter. Endpoint: `POST /api/ai/features/summarize`
- **Translation**: Text translation with source/target language. Endpoint: `POST /api/ai/features/translate`
- **Sentiment Analysis**: Returns positive/negative/neutral with confidence score. Endpoint: `POST /api/ai/features/sentiment`
- **Smart Search**: Configurable via `AI_SMART_SEARCH_ENABLED` (enabled by default)
- **Chat Assistant**: Configurable via `AI_CHAT_ASSISTANT_ENABLED` (enabled by default)

Feature registry in `ai_features` table allows enabling/disabling features with role-based access control.

---

## 3. nTV Integration Opportunities (Ranked by Value)

### HIGH VALUE

#### 1. Content Recommendations via Embeddings

**What**: Use the embeddings storage and semantic search to power "Because you watched X, you might like Y" recommendations.

**How it works with existing code**: The `ai_embeddings` table already supports typed content storage with `content_type` and `content_id` fields. nTV would embed content metadata (title, description, genre, cast, director, keywords) for each media item and store it via `POST /api/ai/embeddings/create`. Recommendation queries would use `POST /api/ai/embeddings/search` with the last-watched item's metadata as the query, filtered by `content_type`.

**What exists**: Embedding storage with upsert (`storeEmbedding`), content-type filtering (`listEmbeddings`), semantic search endpoint structure (`/api/ai/embeddings/search`), content deduplication via hash.

**What needs to be added**: (a) Actual provider embedding generation (replace placeholder random vectors in `server.ts` line 278), (b) Cosine similarity computation in the search endpoint (replace placeholder `Math.random()` in `server.ts` line 308), (c) nTV-specific prompt templates for metadata-to-embedding-text conversion, (d) A batch embedding job for the existing library.

**Effort**: 2-3 days. The scaffolding is complete; the work is wiring up a real embedding provider and adding cosine similarity math (or using pgvector).

#### 2. Smart Content Matching

**What**: Replace regex-based release name matching in the content-acquisition pipeline with AI-powered fuzzy title matching. Example: `"The.Office.S03E05.720p.BluRay.x264-DEMAND"` -> `The Office, Season 3, Episode 5`.

**How it works with existing code**: Register a function definition in `ai_functions` with a JSON Schema describing the expected input (raw filename) and output (structured title, season, episode, quality). Use the chat completion endpoint with a system prompt like "You are a media filename parser. Extract the title, season, episode, year, and quality from the following release name." The function calling infrastructure (`ai_function_calls` table) tracks success/failure rates, which is useful for monitoring matching accuracy.

**What exists**: Function definition storage, function call tracking with metrics, chat completion endpoint with model resolution and quota checking, prompt template system for reusable parsing prompts.

**What needs to be added**: (a) Wire up actual provider API calls in the chat completion flow, (b) Create a prompt template for release name parsing, (c) Add an integration endpoint or function that content-acquisition calls, (d) Consider using a local model (Ollama) for this to avoid per-request cost on high-volume parsing.

**Effort**: 1-2 days. The critical path is short: one prompt template + one function definition + provider API wiring.

#### 3. Metadata Enrichment

**What**: AI-generated summaries, tags, genre classifications, and descriptions for content that has sparse or missing metadata. Particularly valuable for foreign films, indie content, and community ROMs where TMDB/IGDB coverage is incomplete.

**How it works with existing code**: The summarization feature (`POST /api/ai/features/summarize`) and prompt template system are directly applicable. Create templates like "Given the following movie title and partial metadata, generate a 2-sentence plot summary, 5 relevant tags, and a genre classification." The `ai_prompt_templates` table supports variable substitution (`{{title}}`, `{{year}}`, `{{existing_description}}`).

**What exists**: Summarization endpoint, prompt templates with `{{variable}}` substitution, per-template usage tracking, category-based template organization.

**What needs to be added**: (a) nTV-specific prompt templates for different content types (movies, TV shows, ROMs), (b) A batch enrichment job that identifies items with sparse metadata, (c) Provider API wiring.

**Effort**: 1-2 days. Mostly template creation and a simple batch script.

### MEDIUM VALUE

#### 4. ROM Identification

**What**: Identify unlabeled or ambiguously named ROM files by analyzing filename patterns, file sizes, and known ROM database signatures. Example: `"Super Mario Bros. 3 (U) [!].nes"` is straightforward, but `"SMB3_hack_v2.nes"` or `"game.nes"` are not.

**How it works with existing code**: Create a function definition for ROM identification with parameters for filename, file size, file extension, and optional header bytes. Use the chat completion endpoint with a specialized system prompt that includes knowledge of common ROM naming conventions, No-Intro naming standards, and GoodTools naming. Store successful identifications as embeddings for future similarity matching.

**What exists**: Function calling infrastructure, prompt templates, embeddings for building a knowledge base of identified ROMs over time.

**What needs to be added**: (a) ROM-specific prompt templates, (b) Integration with ROM scanner in nTV, (c) Optional: a local model fine-tuned or prompted with ROM naming convention knowledge.

**Effort**: 2-3 days. Requires ROM domain knowledge in the prompts and testing against diverse ROM collections.

#### 5. Natural Language Search

**What**: Users search their library with natural language queries: "action movies from the 90s with Bruce Willis" or "platformer games for SNES" instead of structured genre/year/actor filters.

**How it works with existing code**: Combine the prompt template system (to convert natural language queries into structured search parameters) with the embeddings system (to find semantically similar content). The chat completion endpoint can parse the query into structured filters, and the semantic search endpoint can find matches by similarity.

**What exists**: Chat completion for query parsing, semantic search for similarity, prompt templates for query-to-filter conversion.

**What needs to be added**: (a) A dedicated search endpoint in nTV that chains AI query parsing with database filtering and semantic similarity, (b) Embeddings for all library content, (c) Query parsing prompt templates.

**Effort**: 2-3 days. Depends on content embeddings being in place (see item 1).

#### 6. Content Moderation

**What**: Auto-flag inappropriate or inaccurate content descriptions, user-generated metadata, or community submissions using sentiment analysis and classification.

**How it works with existing code**: The sentiment analysis endpoint (`POST /api/ai/features/sentiment`) provides a starting point. For moderation, a more targeted prompt template that classifies content as safe/unsafe with specific category flags (violence, adult content, misinformation) would be more appropriate.

**What exists**: Sentiment analysis endpoint, prompt templates, feature registry with role-based access control.

**What needs to be added**: (a) Moderation-specific prompt templates, (b) Integration hook in nTV's metadata ingestion pipeline.

**Effort**: 1 day. The sentiment endpoint is already functional; moderation is a prompt template variation.

### LOW VALUE (Post-V1)

#### 7. Subtitle Quality Scoring

**What**: AI-based assessment of subtitle quality (timing accuracy, translation quality, OCR error detection) and translation suggestions for missing languages.

**How it works with existing code**: Use the translation endpoint as a base, add quality scoring via prompt templates. Computationally expensive for full subtitle files.

**Effort**: 3-5 days. Subtitle parsing, quality metrics definition, and per-line analysis are significant scope.

#### 8. Watch Pattern Analysis

**What**: Analyze viewing habits to predict what users want to watch, optimal download scheduling, and personalized content priorities.

**How it works with existing code**: The usage analytics system (`getUsage` with groupBy support) provides a framework. Watch history data would be fed through prompt templates or embeddings for pattern extraction.

**Effort**: 3-5 days. Requires watch history data collection, pattern analysis prompts, and recommendation logic.

---

## 4. Recommended Approach for V1

**Use existing AI plugin capabilities rather than building new AI features.** The plugin is production-ready in terms of architecture -- the database schema, API endpoints, conversation management, embeddings storage, prompt templates, function calling, and usage tracking are all implemented. The primary gap is wiring up actual provider API calls (replacing placeholder responses with real OpenAI/Anthropic/Ollama calls), which is a well-defined integration task.

### V1 Priority

1. **Content recommendations via embeddings** -- Highest user-visible impact. Users immediately see value in "related content" suggestions. Requires: provider embedding wiring + cosine similarity (consider pgvector extension) + batch embedding job for existing library.

2. **Smart content matching** -- Highest operational impact. Reduces failed matches in the content-acquisition pipeline, directly improving the core content ingestion flow. Requires: provider chat completion wiring + one prompt template + integration endpoint.

3. **Everything else is post-V1.** Metadata enrichment is a natural follow-on once provider APIs are wired up. Natural language search depends on embeddings being populated. ROM identification, moderation, subtitles, and watch patterns are all viable but not critical path.

### Implementation Sequence

```
Step 1: Wire up at least one provider (OpenAI or Ollama) in server.ts
        - Replace placeholder in chat completion handler (~line 126)
        - Replace placeholder in embedding creation handler (~line 278)
        - Replace placeholder in semantic search handler (~line 308)

Step 2: Add pgvector or implement cosine similarity for embedding search

Step 3: Create nTV prompt templates:
        - content-match: release filename -> structured metadata
        - content-embed: media metadata -> embedding-ready text
        - content-recommend: "find similar to this" query construction

Step 4: Batch embed existing nTV library content

Step 5: Integrate content-acquisition with smart matching function
```

---

## 5. Estimated Effort Summary

| Integration | Value | Effort | V1 Scope? | Dependencies |
|---|---|---|---|---|
| Content recommendations | HIGH | 2-3 days | Yes | Provider wiring, pgvector/cosine similarity |
| Smart content matching | HIGH | 1-2 days | Yes | Provider wiring |
| Metadata enrichment | HIGH | 1-2 days | Yes | Provider wiring |
| ROM identification | MEDIUM | 2-3 days | No | Provider wiring, ROM domain prompts |
| Natural language search | MEDIUM | 2-3 days | No | Content embeddings populated |
| Content moderation | MEDIUM | 1 day | No | Provider wiring |
| Subtitle quality | LOW | 3-5 days | No | Subtitle parsing, provider wiring |
| Watch pattern analysis | LOW | 3-5 days | No | Watch history collection |

**Total V1 effort**: 4-7 days (content recommendations + smart matching + metadata enrichment, with shared provider wiring work).

---

## 6. Database Tables Reference

| Table | Purpose | Key Columns |
|---|---|---|
| `ai_models` | Registered AI models across providers | provider, model_id, model_type, pricing, capability flags |
| `ai_conversations` | Conversation sessions | user_id, context_type/context_id, model_id, token/cost totals |
| `ai_messages` | Message history per conversation | role, content, function_call data, tokens, cost, latency |
| `ai_requests` | All AI request tracking | provider, model, status, input/output, tokens, cost, latency |
| `ai_embeddings` | Vector embeddings storage | content_type, content_id, content_text, embedding_data, dimensions |
| `ai_prompt_templates` | Reusable prompt templates | name, category, system/user prompts, variables, usage_count |
| `ai_functions` | Function definitions for function calling | name, parameters_schema, implementation_type/config, metrics |
| `ai_function_calls` | Function call execution log | function_id, arguments, response, status, latency |
| `ai_usage_quotas` | Per-user/scope daily usage limits | quota_type, scope_id, max/current requests/tokens/cost |
| `ai_features` | Feature registry with access control | feature_name, feature_type, prompt_template_id, allowed_roles |

---

## 7. API Endpoints Reference

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Health check |
| GET | `/ready` | Readiness check (verifies DB) |
| POST | `/api/ai/chat/completions` | Chat completion with model resolution and quota checking |
| GET | `/api/ai/conversations` | List conversations (optional user_id filter) |
| POST | `/api/ai/conversations` | Create conversation |
| GET | `/api/ai/conversations/:id` | Get conversation with messages |
| POST | `/api/ai/conversations/:id/messages` | Add message and get AI response |
| DELETE | `/api/ai/conversations/:id` | Delete conversation |
| POST | `/api/ai/embeddings/create` | Store embedding for content |
| POST | `/api/ai/embeddings/search` | Semantic search across embeddings |
| GET | `/api/ai/models` | List all models |
| POST | `/api/ai/models` | Register a model |
| PATCH | `/api/ai/models/:id` | Update model settings |
| DELETE | `/api/ai/models/:id` | Delete a model |
| GET | `/api/ai/prompts` | List prompt templates (optional category filter) |
| POST | `/api/ai/prompts` | Create prompt template |
| POST | `/api/ai/prompts/:id/render` | Render template with variables |
| GET | `/api/ai/usage` | Usage statistics (groupable by day/model/provider) |
| GET | `/api/ai/quota` | Check user quota status |
| POST | `/api/ai/quota` | Set user quota |
| GET | `/api/ai/features` | List enabled features |
| POST | `/api/ai/features/summarize` | Summarize messages |
| POST | `/api/ai/features/translate` | Translate text |
| POST | `/api/ai/features/sentiment` | Sentiment analysis |
