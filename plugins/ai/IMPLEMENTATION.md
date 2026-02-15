# AI Plugin - Full Implementation Guide

## Overview

The AI plugin provides a unified gateway for **OpenAI, Anthropic (Claude), Google (Gemini), and Cohere** with chat completions, embeddings, semantic search, prompt templates, conversation management, and usage tracking.

## Current Status

**Infrastructure Status**: ✅ Complete (database, API, quotas, tracking)
**Provider Integration Status**: ⚠️ Placeholder (requires SDK integration)

## What's Already Built

- ✅ Complete database schema for models, conversations, messages, embeddings, quotas, usage
- ✅ Full REST API with all endpoints
- ✅ Multi-provider model management
- ✅ Conversation history tracking
- ✅ Usage quotas and cost tracking
- ✅ Prompt template system with variable substitution
- ✅ Semantic search infrastructure
- ✅ Rate limiting per user

## What Needs Implementation

**Provider SDK Integration** - The actual AI provider calls in:
- `chatCompletion()` - Generate text from prompts
- `generateEmbedding()` - Create vector embeddings
- `streamCompletion()` - Streaming responses
- `functionCalling()` - Tool/agent execution

---

## Required Packages

Base dependencies **already installed**:

```json
{
  "@nself/plugin-utils": "file:../../../shared",
  "fastify": "^4.24.0",
  "@fastify/cors": "^8.4.0",
  "pg": "^8.11.3"
}
```

### Additional Packages for Provider Integration

```bash
# OpenAI SDK (official)
pnpm add openai

# Anthropic SDK (official)
pnpm add @anthropic-ai/sdk

# Google Generative AI SDK
pnpm add @google/generative-ai

# Cohere SDK
pnpm add cohere-ai

# For vector operations
pnpm add @xenova/transformers  # Optional: local embeddings
```

---

## Complete Implementation Code

### 1. Provider Integration Module

Create `ts/src/providers.ts`:

```typescript
/**
 * AI Provider Integration
 * Supports OpenAI, Anthropic, Google (Gemini), and Cohere
 */

import OpenAI from 'openai';
import Anthropic from '@anthropic-ai/sdk';
import { GoogleGenerativeAI } from '@google/generative-ai';
import { CohereClient } from 'cohere-ai';

export interface ChatMessage {
  role: 'system' | 'user' | 'assistant';
  content: string;
}

export interface CompletionOptions {
  temperature?: number;
  maxTokens?: number;
  topP?: number;
  stream?: boolean;
}

export interface CompletionResult {
  content: string;
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  finishReason: string;
  cost: number;
}

export interface EmbeddingResult {
  embedding: number[];
  dimensions: number;
  tokens: number;
}

/**
 * OpenAI Provider
 */
export class OpenAIProvider {
  private client: OpenAI;
  private model: string;
  private embeddingModel: string;

  // Pricing (per million tokens)
  private readonly PRICING = {
    'gpt-4': { input: 30.00, output: 60.00 },
    'gpt-4-turbo': { input: 10.00, output: 30.00 },
    'gpt-3.5-turbo': { input: 0.50, output: 1.50 },
    'text-embedding-3-small': { input: 0.02, output: 0 },
    'text-embedding-3-large': { input: 0.13, output: 0 },
  };

  constructor(apiKey: string, model = 'gpt-4-turbo', embeddingModel = 'text-embedding-3-small') {
    this.client = new OpenAI({ apiKey });
    this.model = model;
    this.embeddingModel = embeddingModel;
  }

  /**
   * Chat completion
   */
  async chatCompletion(messages: ChatMessage[], options: CompletionOptions = {}): Promise<CompletionResult> {
    try {
      const response = await this.client.chat.completions.create({
        model: this.model,
        messages: messages.map(m => ({ role: m.role, content: m.content })),
        temperature: options.temperature ?? 0.7,
        max_tokens: options.maxTokens,
        top_p: options.topP,
      });

      const choice = response.choices[0];
      const usage = response.usage!;

      const pricing = this.PRICING[this.model as keyof typeof this.PRICING] ?? { input: 10, output: 30 };
      const cost = (usage.prompt_tokens / 1_000_000) * pricing.input +
                   (usage.completion_tokens / 1_000_000) * pricing.output;

      return {
        content: choice.message.content ?? '',
        promptTokens: usage.prompt_tokens,
        completionTokens: usage.completion_tokens,
        totalTokens: usage.total_tokens,
        finishReason: choice.finish_reason,
        cost,
      };
    } catch (error) {
      throw new Error(`OpenAI completion failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Generate embedding
   */
  async generateEmbedding(text: string): Promise<EmbeddingResult> {
    try {
      const response = await this.client.embeddings.create({
        model: this.embeddingModel,
        input: text,
      });

      const embedding = response.data[0];

      return {
        embedding: embedding.embedding,
        dimensions: embedding.embedding.length,
        tokens: response.usage.total_tokens,
      };
    } catch (error) {
      throw new Error(`OpenAI embedding failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Stream completion
   */
  async *streamCompletion(messages: ChatMessage[], options: CompletionOptions = {}): AsyncGenerator<string> {
    try {
      const stream = await this.client.chat.completions.create({
        model: this.model,
        messages: messages.map(m => ({ role: m.role, content: m.content })),
        temperature: options.temperature ?? 0.7,
        max_tokens: options.maxTokens,
        stream: true,
      });

      for await (const chunk of stream) {
        const content = chunk.choices[0]?.delta?.content ?? '';
        if (content) {
          yield content;
        }
      }
    } catch (error) {
      throw new Error(`OpenAI stream failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }
}

/**
 * Anthropic (Claude) Provider
 */
export class AnthropicProvider {
  private client: Anthropic;
  private model: string;

  private readonly PRICING = {
    'claude-3-opus-20240229': { input: 15.00, output: 75.00 },
    'claude-3-sonnet-20240229': { input: 3.00, output: 15.00 },
    'claude-3-haiku-20240307': { input: 0.25, output: 1.25 },
  };

  constructor(apiKey: string, model = 'claude-3-sonnet-20240229') {
    this.client = new Anthropic({ apiKey });
    this.model = model;
  }

  async chatCompletion(messages: ChatMessage[], options: CompletionOptions = {}): Promise<CompletionResult> {
    try {
      // Separate system message from conversation
      const systemMessage = messages.find(m => m.role === 'system')?.content;
      const conversationMessages = messages.filter(m => m.role !== 'system');

      const response = await this.client.messages.create({
        model: this.model,
        max_tokens: options.maxTokens ?? 4096,
        temperature: options.temperature ?? 0.7,
        system: systemMessage,
        messages: conversationMessages.map(m => ({
          role: m.role === 'user' ? 'user' : 'assistant',
          content: m.content,
        })),
      });

      const content = response.content[0];
      const textContent = content.type === 'text' ? content.text : '';

      const pricing = this.PRICING[this.model as keyof typeof this.PRICING] ?? { input: 3, output: 15 };
      const cost = (response.usage.input_tokens / 1_000_000) * pricing.input +
                   (response.usage.output_tokens / 1_000_000) * pricing.output;

      return {
        content: textContent,
        promptTokens: response.usage.input_tokens,
        completionTokens: response.usage.output_tokens,
        totalTokens: response.usage.input_tokens + response.usage.output_tokens,
        finishReason: response.stop_reason ?? 'end_turn',
        cost,
      };
    } catch (error) {
      throw new Error(`Anthropic completion failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  async *streamCompletion(messages: ChatMessage[], options: CompletionOptions = {}): AsyncGenerator<string> {
    try {
      const systemMessage = messages.find(m => m.role === 'system')?.content;
      const conversationMessages = messages.filter(m => m.role !== 'system');

      const stream = await this.client.messages.create({
        model: this.model,
        max_tokens: options.maxTokens ?? 4096,
        temperature: options.temperature ?? 0.7,
        system: systemMessage,
        messages: conversationMessages.map(m => ({
          role: m.role === 'user' ? 'user' : 'assistant',
          content: m.content,
        })),
        stream: true,
      });

      for await (const event of stream) {
        if (event.type === 'content_block_delta' && event.delta.type === 'text_delta') {
          yield event.delta.text;
        }
      }
    } catch (error) {
      throw new Error(`Anthropic stream failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }
}

/**
 * Google Gemini Provider
 */
export class GoogleProvider {
  private client: GoogleGenerativeAI;
  private model: string;

  private readonly PRICING = {
    'gemini-pro': { input: 0.50, output: 1.50 },
    'gemini-pro-vision': { input: 0.50, output: 1.50 },
  };

  constructor(apiKey: string, model = 'gemini-pro') {
    this.client = new GoogleGenerativeAI(apiKey);
    this.model = model;
  }

  async chatCompletion(messages: ChatMessage[], options: CompletionOptions = {}): Promise<CompletionResult> {
    try {
      const model = this.client.getGenerativeModel({ model: this.model });

      // Convert messages to Gemini format
      const history = messages.slice(0, -1).map(m => ({
        role: m.role === 'user' ? 'user' : 'model',
        parts: [{ text: m.content }],
      }));

      const lastMessage = messages[messages.length - 1]!;

      const chat = model.startChat({
        history,
        generationConfig: {
          temperature: options.temperature ?? 0.7,
          maxOutputTokens: options.maxTokens,
          topP: options.topP,
        },
      });

      const result = await chat.sendMessage(lastMessage.content);
      const response = result.response;
      const text = response.text();

      // Gemini doesn't return token counts in the same way, estimate
      const estimatedPromptTokens = messages.reduce((sum, m) => sum + Math.ceil(m.content.length / 4), 0);
      const estimatedCompletionTokens = Math.ceil(text.length / 4);

      const pricing = this.PRICING[this.model as keyof typeof this.PRICING] ?? { input: 0.5, output: 1.5 };
      const cost = (estimatedPromptTokens / 1_000_000) * pricing.input +
                   (estimatedCompletionTokens / 1_000_000) * pricing.output;

      return {
        content: text,
        promptTokens: estimatedPromptTokens,
        completionTokens: estimatedCompletionTokens,
        totalTokens: estimatedPromptTokens + estimatedCompletionTokens,
        finishReason: response.candidates?.[0]?.finishReason ?? 'STOP',
        cost,
      };
    } catch (error) {
      throw new Error(`Google completion failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  async generateEmbedding(text: string): Promise<EmbeddingResult> {
    try {
      const model = this.client.getGenerativeModel({ model: 'embedding-001' });
      const result = await model.embedContent(text);

      return {
        embedding: result.embedding.values,
        dimensions: result.embedding.values.length,
        tokens: Math.ceil(text.length / 4), // Estimate
      };
    } catch (error) {
      throw new Error(`Google embedding failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }
}

/**
 * Cohere Provider
 */
export class CohereProvider {
  private client: CohereClient;
  private model: string;

  private readonly PRICING = {
    'command': { input: 1.00, output: 2.00 },
    'command-light': { input: 0.30, output: 0.60 },
  };

  constructor(apiKey: string, model = 'command') {
    this.client = new CohereClient({ token: apiKey });
    this.model = model;
  }

  async chatCompletion(messages: ChatMessage[], options: CompletionOptions = {}): Promise<CompletionResult> {
    try {
      // Cohere chat endpoint uses conversation history
      const chatHistory = messages.slice(0, -1).map(m => ({
        role: m.role === 'user' ? 'USER' : 'CHATBOT',
        message: m.content,
      }));

      const lastMessage = messages[messages.length - 1]!;

      const response = await this.client.chat({
        model: this.model,
        message: lastMessage.content,
        chatHistory: chatHistory as never,
        temperature: options.temperature ?? 0.7,
        maxTokens: options.maxTokens,
      });

      const estimatedPromptTokens = messages.reduce((sum, m) => sum + Math.ceil(m.content.length / 4), 0);
      const estimatedCompletionTokens = Math.ceil(response.text.length / 4);

      const pricing = this.PRICING[this.model as keyof typeof this.PRICING] ?? { input: 1, output: 2 };
      const cost = (estimatedPromptTokens / 1_000_000) * pricing.input +
                   (estimatedCompletionTokens / 1_000_000) * pricing.output;

      return {
        content: response.text,
        promptTokens: estimatedPromptTokens,
        completionTokens: estimatedCompletionTokens,
        totalTokens: estimatedPromptTokens + estimatedCompletionTokens,
        finishReason: response.finishReason ?? 'COMPLETE',
        cost,
      };
    } catch (error) {
      throw new Error(`Cohere completion failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  async generateEmbedding(text: string): Promise<EmbeddingResult> {
    try {
      const response = await this.client.embed({
        texts: [text],
        model: 'embed-english-v3.0',
        inputType: 'search_document',
      });

      const embedding = response.embeddings[0];

      return {
        embedding: embedding!,
        dimensions: embedding!.length,
        tokens: Math.ceil(text.length / 4),
      };
    } catch (error) {
      throw new Error(`Cohere embedding failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }
}

/**
 * Provider factory
 */
export function createAIProvider(
  provider: string,
  config: Record<string, string>
): OpenAIProvider | AnthropicProvider | GoogleProvider | CohereProvider {
  switch (provider.toLowerCase()) {
    case 'openai':
      if (!config.AI_OPENAI_API_KEY) {
        throw new Error('AI_OPENAI_API_KEY is required for OpenAI provider');
      }
      return new OpenAIProvider(
        config.AI_OPENAI_API_KEY,
        config.AI_OPENAI_MODEL,
        config.AI_OPENAI_EMBEDDING_MODEL
      );

    case 'anthropic':
      if (!config.AI_ANTHROPIC_API_KEY) {
        throw new Error('AI_ANTHROPIC_API_KEY is required for Anthropic provider');
      }
      return new AnthropicProvider(
        config.AI_ANTHROPIC_API_KEY,
        config.AI_ANTHROPIC_MODEL
      );

    case 'google':
      if (!config.AI_GOOGLE_API_KEY) {
        throw new Error('AI_GOOGLE_API_KEY is required for Google provider');
      }
      return new GoogleProvider(
        config.AI_GOOGLE_API_KEY,
        config.AI_GOOGLE_MODEL
      );

    case 'cohere':
      if (!config.AI_COHERE_API_KEY) {
        throw new Error('AI_COHERE_API_KEY is required for Cohere provider');
      }
      return new CohereProvider(
        config.AI_COHERE_API_KEY,
        config.AI_COHERE_MODEL
      );

    default:
      throw new Error(`Unsupported AI provider: ${provider}`);
  }
}
```

### 2. Update Server to Use Providers

Modify `ts/src/server.ts`:

```typescript
import { createAIProvider } from './providers.js';

// In createServer() after config:
const aiProvider = createAIProvider(
  fullConfig.defaultProvider,
  process.env as Record<string, string>
);

// Update chat completion endpoint (around line 91):
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

    // Call AI provider
    const result = await aiProvider.chatCompletion(
      messages,
      { temperature, maxTokens: max_tokens }
    );

    const latencyMs = Date.now() - startTime;

    await scopedDb(request).completeRequest(
      aiRequest.id,
      { content: result.content },
      result.promptTokens,
      result.completionTokens,
      result.cost,
      latencyMs
    );

    // Store in conversation if provided
    if (conversation_id) {
      const lastUserMsg = messages[messages.length - 1];
      if (lastUserMsg) {
        await scopedDb(request).addMessage(conversation_id, lastUserMsg.role, lastUserMsg.content);
      }
      await scopedDb(request).addMessage(conversation_id, 'assistant', result.content, {
        tokensUsed: result.totalTokens,
        modelUsed: model.model_name,
        finishReason: result.finishReason,
        cost: result.cost,
        latencyMs,
      });
    }

    // Track usage
    if (user_id) {
      await scopedDb(request).trackUsage(user_id, result.totalTokens, result.cost);
    }

    const response: ChatCompletionResponse = {
      id: aiRequest.id,
      choices: [{
        message: { role: 'assistant', content: result.content },
        finish_reason: result.finishReason,
      }],
      usage: {
        prompt_tokens: result.promptTokens,
        completion_tokens: result.completionTokens,
        total_tokens: result.totalTokens,
      },
      cost: result.cost,
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

// Update embeddings endpoint (around line 264):
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

    // Generate embedding using provider
    const result = await aiProvider.generateEmbedding(content_text);

    const embedding = await scopedDb(request).storeEmbedding(
      content_type, content_id, content_text,
      model.id, result.dimensions, result.embedding
    );

    return reply.status(201).send({ embedding_id: embedding.id });
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Create embedding failed', { error: message });
    return reply.status(500).send({ error: message });
  }
});
```

---

## Configuration

### Environment Variables

**OpenAI**:
```bash
AI_DEFAULT_PROVIDER=openai
AI_OPENAI_API_KEY=sk-...
AI_OPENAI_MODEL=gpt-4-turbo
AI_OPENAI_EMBEDDING_MODEL=text-embedding-3-small
AI_EMBEDDINGS_ENABLED=true
```

**Anthropic (Claude)**:
```bash
AI_DEFAULT_PROVIDER=anthropic
AI_ANTHROPIC_API_KEY=sk-ant-...
AI_ANTHROPIC_MODEL=claude-3-sonnet-20240229
```

**Google (Gemini)**:
```bash
AI_DEFAULT_PROVIDER=google
AI_GOOGLE_API_KEY=...
AI_GOOGLE_MODEL=gemini-pro
```

**Multi-Provider**:
```bash
AI_DEFAULT_PROVIDER=openai
AI_OPENAI_API_KEY=...
AI_ANTHROPIC_API_KEY=...
AI_GOOGLE_API_KEY=...
```

### Get API Credentials

**OpenAI**: https://platform.openai.com/api-keys
**Anthropic**: https://console.anthropic.com/
**Google**: https://makersuite.google.com/app/apikey
**Cohere**: https://dashboard.cohere.com/api-keys

---

## Testing

```bash
cd plugins/ai/ts
pnpm install
pnpm add openai @anthropic-ai/sdk @google/generative-ai cohere-ai
pnpm build
pnpm start
```

**Test Chat**:
```bash
curl -X POST http://localhost:3101/api/ai/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -d '{
    "messages": [
      {"role": "user", "content": "Hello, how are you?"}
    ],
    "temperature": 0.7,
    "max_tokens": 150
  }'
```

---

## Activation Checklist

- [ ] Install SDKs: `pnpm add openai @anthropic-ai/sdk @google/generative-ai cohere-ai`
- [ ] Create `providers.ts`
- [ ] Update `server.ts`
- [ ] Add API keys to `.env`
- [ ] Build & start
- [ ] Test completions
- [ ] Test embeddings
- [ ] Verify cost tracking

---

## Cost Management

**OpenAI Pricing** (GPT-4 Turbo):
- Input: $10/M tokens
- Output: $30/M tokens

**Anthropic Pricing** (Claude 3 Sonnet):
- Input: $3/M tokens
- Output: $15/M tokens

**Quotas**: Configure per-user limits in database to control costs.
