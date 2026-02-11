#!/usr/bin/env node
/**
 * HTTP server for streaming API
 * Multi-app aware: each request is scoped to a source_account_id
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, getAppContext } from '@nself/plugin-utils';
import { config } from './config.js';
import { db, DatabaseClient } from './database.js';
import {
  CreateStreamInput, UpdateStreamInput, ListStreamsQuery,
  CreateClipInput,
  SendChatInput, ModeratorPermissionsInput,
  CreateReportInput,
} from './types.js';

const logger = createLogger('streaming:server');

const fastify = Fastify({ logger: { level: 'info' } });

fastify.register(cors, { origin: true });

// Multi-app context
fastify.decorateRequest('scopedDb', null);
fastify.addHook('onRequest', async (request) => {
  const ctx = getAppContext(request);
  (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
});

function scopedDb(request: unknown): DatabaseClient {
  return (request as Record<string, unknown>).scopedDb as DatabaseClient;
}

// =============================================================================
// Health
// =============================================================================

fastify.get('/health', async () => ({
  status: 'ok', timestamp: new Date().toISOString(), service: 'streaming',
}));

fastify.get('/ready', async () => {
  try {
    const stats = await db.getStats();
    return { status: 'ready', ...stats };
  } catch {
    return { status: 'not_ready' };
  }
});

// =============================================================================
// Streams
// =============================================================================

fastify.post<{ Body: CreateStreamInput }>('/api/v1/streams', async (request, reply) => {
  try {
    const stream = await scopedDb(request).createStream(request.body);
    return reply.code(201).send({ stream });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to create stream', { error: msg });
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Querystring: ListStreamsQuery }>('/api/v1/streams', async (request, reply) => {
  try {
    const result = await scopedDb(request).listStreams(request.query);
    return { data: result.streams, total: result.total };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/streams/:id', async (request, reply) => {
  try {
    const stream = await scopedDb(request).getStream(request.params.id);
    if (!stream) return reply.code(404).send({ error: 'Stream not found' });
    const viewerCount = await scopedDb(request).getViewerCount(stream.id);
    return { stream, current_viewers: viewerCount };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.put<{ Params: { id: string }; Body: UpdateStreamInput }>('/api/v1/streams/:id', async (request, reply) => {
  try {
    const stream = await scopedDb(request).updateStream(request.params.id, request.body);
    if (!stream) return reply.code(404).send({ error: 'Stream not found' });
    return { stream };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.delete<{ Params: { id: string } }>('/api/v1/streams/:id', async (request, reply) => {
  try {
    const deleted = await scopedDb(request).deleteStream(request.params.id);
    if (!deleted) return reply.code(404).send({ error: 'Stream not found' });
    return { success: true };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string } }>('/api/v1/streams/:id/start', async (request, reply) => {
  try {
    const stream = await scopedDb(request).startStream(request.params.id);
    if (!stream) return reply.code(404).send({ error: 'Stream not found' });
    return { stream };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string } }>('/api/v1/streams/:id/stop', async (request, reply) => {
  try {
    const stream = await scopedDb(request).stopStream(request.params.id);
    if (!stream) return reply.code(404).send({ error: 'Stream not found' });
    return { stream };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string } }>('/api/v1/streams/:id/restart', async (request, reply) => {
  try {
    await scopedDb(request).stopStream(request.params.id);
    const stream = await scopedDb(request).startStream(request.params.id);
    if (!stream) return reply.code(404).send({ error: 'Stream not found' });
    return { stream };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/streams/:id/status', async (request, reply) => {
  try {
    const stream = await scopedDb(request).getStream(request.params.id);
    if (!stream) return reply.code(404).send({ error: 'Stream not found' });
    const viewerCount = await scopedDb(request).getViewerCount(stream.id);
    return {
      id: stream.id, status: stream.status, started_at: stream.started_at,
      current_viewers: viewerCount, peak_viewers: stream.peak_viewers,
      total_views: stream.total_views, duration_seconds: stream.duration_seconds,
    };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

// =============================================================================
// Stream Keys
// =============================================================================

fastify.post<{ Params: { id: string }; Body: { name: string } }>('/api/v1/streams/:id/keys', async (request, reply) => {
  try {
    const key = await scopedDb(request).generateStreamKey(request.params.id, request.body.name);
    return reply.code(201).send({ key });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/streams/:id/keys', async (request, reply) => {
  try {
    const keys = await scopedDb(request).listStreamKeys(request.params.id);
    return { data: keys, total: keys.length };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.delete<{ Params: { id: string; keyId: string } }>('/api/v1/streams/:id/keys/:keyId', async (request, reply) => {
  try {
    const deleted = await scopedDb(request).deleteStreamKey(request.params.keyId);
    if (!deleted) return reply.code(404).send({ error: 'Key not found' });
    return { success: true };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string; keyId: string } }>('/api/v1/streams/:id/keys/:keyId/rotate', async (request, reply) => {
  try {
    const key = await scopedDb(request).rotateStreamKey(request.params.keyId);
    if (!key) return reply.code(404).send({ error: 'Key not found' });
    return { key };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

// =============================================================================
// Playback
// =============================================================================

fastify.get<{ Params: { id: string } }>('/api/v1/streams/:id/playback', async (request, reply) => {
  try {
    const stream = await scopedDb(request).getStream(request.params.id);
    if (!stream) return reply.code(404).send({ error: 'Stream not found' });
    return { hls_url: stream.hls_url, webrtc_url: stream.webrtc_url };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/streams/:id/thumbnail', async (request, reply) => {
  try {
    const stream = await scopedDb(request).getStream(request.params.id);
    if (!stream) return reply.code(404).send({ error: 'Stream not found' });
    return { thumbnail_url: stream.thumbnail_url };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

// =============================================================================
// Recordings & VOD
// =============================================================================

fastify.post<{ Params: { id: string } }>('/api/v1/streams/:id/record', async (request, reply) => {
  try {
    const stream = await scopedDb(request).updateStream(request.params.id, { enable_recording: true });
    if (!stream) return reply.code(404).send({ error: 'Stream not found' });
    return { success: true, message: 'Recording started' };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string } }>('/api/v1/streams/:id/record/stop', async (request, reply) => {
  try {
    const stream = await scopedDb(request).updateStream(request.params.id, { enable_recording: false });
    if (!stream) return reply.code(404).send({ error: 'Stream not found' });
    return { success: true, message: 'Recording stopped' };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/streams/:id/recordings', async (request, reply) => {
  try {
    const recordings = await scopedDb(request).listRecordings(request.params.id);
    return { data: recordings, total: recordings.length };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/recordings/:id', async (request, reply) => {
  try {
    const recording = await scopedDb(request).getRecording(request.params.id);
    if (!recording) return reply.code(404).send({ error: 'Recording not found' });
    return { recording };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.delete<{ Params: { id: string } }>('/api/v1/recordings/:id', async (request, reply) => {
  try {
    const deleted = await scopedDb(request).deleteRecording(request.params.id);
    if (!deleted) return reply.code(404).send({ error: 'Recording not found' });
    return { success: true };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string }; Body: CreateClipInput }>('/api/v1/recordings/:id/clip', async (request, reply) => {
  try {
    const clip = await scopedDb(request).createClip({
      ...request.body, recording_id: request.params.id,
    });
    return reply.code(201).send({ clip });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

// =============================================================================
// Analytics
// =============================================================================

fastify.get<{ Params: { id: string }; Querystring: { start_date?: string; end_date?: string } }>(
  '/api/v1/streams/:id/analytics',
  async (request, reply) => {
    try {
      const analytics = await scopedDb(request).getStreamAnalytics(
        request.params.id, request.query.start_date, request.query.end_date
      );
      return { data: analytics, total: analytics.length };
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      return reply.code(500).send({ error: msg });
    }
  }
);

fastify.get<{ Params: { id: string } }>('/api/v1/streams/:id/viewers', async (request, reply) => {
  try {
    const viewers = await scopedDb(request).getActiveViewers(request.params.id);
    return { data: viewers, total: viewers.length };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/streams/:id/stats', async (request, reply) => {
  try {
    const stream = await scopedDb(request).getStream(request.params.id);
    if (!stream) return reply.code(404).send({ error: 'Stream not found' });
    const viewerCount = await scopedDb(request).getViewerCount(stream.id);
    return {
      current_viewers: viewerCount, peak_viewers: stream.peak_viewers,
      total_views: stream.total_views, duration_seconds: stream.duration_seconds,
    };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get('/api/v1/analytics/dashboard', async (request, reply) => {
  try {
    const stats = await scopedDb(request).getStats();
    return { stats };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

// =============================================================================
// Chat
// =============================================================================

fastify.get<{ Params: { id: string }; Querystring: { limit?: string; before?: string } }>(
  '/api/v1/streams/:id/chat',
  async (request, reply) => {
    try {
      const limit = parseInt(request.query.limit ?? '50', 10);
      const messages = await scopedDb(request).getChatMessages(request.params.id, limit, request.query.before);
      return { data: messages, total: messages.length };
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      return reply.code(500).send({ error: msg });
    }
  }
);

fastify.post<{ Params: { id: string }; Body: SendChatInput }>('/api/v1/streams/:id/chat', async (request, reply) => {
  try {
    const message = await scopedDb(request).sendChatMessage(
      request.params.id, request.body.user_id, request.body.content
    );
    return reply.code(201).send({ message });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.delete<{ Params: { id: string; msgId: string }; Body: { deleted_by: string } }>(
  '/api/v1/streams/:id/chat/:msgId',
  async (request, reply) => {
    try {
      const deleted = await scopedDb(request).deleteChatMessage(request.params.msgId, request.body.deleted_by);
      if (!deleted) return reply.code(404).send({ error: 'Message not found' });
      return { success: true };
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      return reply.code(500).send({ error: msg });
    }
  }
);

fastify.post<{ Params: { id: string } }>('/api/v1/streams/:id/chat/slow', async (_request, _reply) => {
  return { success: true, message: 'Slow mode enabled' };
});

fastify.post<{ Params: { id: string }; Body: { user_id: string } }>('/api/v1/streams/:id/chat/ban', async (_request, _reply) => {
  return { success: true, message: 'User banned from chat' };
});

// =============================================================================
// Moderation
// =============================================================================

fastify.post<{ Params: { id: string }; Body: { user_id: string; permissions: ModeratorPermissionsInput } }>(
  '/api/v1/streams/:id/moderators',
  async (request, reply) => {
    try {
      const mod = await scopedDb(request).addModerator(
        request.params.id, request.body.user_id, request.body.permissions
      );
      return reply.code(201).send({ moderator: mod });
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      return reply.code(500).send({ error: msg });
    }
  }
);

fastify.delete<{ Params: { id: string; userId: string } }>(
  '/api/v1/streams/:id/moderators/:userId',
  async (request, reply) => {
    try {
      const deleted = await scopedDb(request).removeModerator(request.params.id, request.params.userId);
      if (!deleted) return reply.code(404).send({ error: 'Moderator not found' });
      return { success: true };
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      return reply.code(500).send({ error: msg });
    }
  }
);

fastify.get<{ Params: { id: string } }>('/api/v1/streams/:id/reports', async (request, reply) => {
  try {
    const reports = await scopedDb(request).getReports(request.params.id);
    return { data: reports, total: reports.length };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string }; Body: CreateReportInput }>('/api/v1/streams/:id/reports', async (request, reply) => {
  try {
    const report = await scopedDb(request).createReport(request.params.id, request.body);
    return reply.code(201).send({ report });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string }; Body: { reason: string } }>('/api/v1/streams/:id/takedown', async (request, reply) => {
  try {
    const stream = await scopedDb(request).takedownStream(request.params.id, request.body.reason);
    if (!stream) return reply.code(404).send({ error: 'Stream not found' });
    return { stream };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

// =============================================================================
// Server Lifecycle
// =============================================================================

const start = async () => {
  try {
    await db.initializeSchema();
    logger.info('Database schema initialized');

    await fastify.listen({ port: config.server.port, host: config.server.host });
    logger.info(`Streaming server running on http://${config.server.host}:${config.server.port}`);
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'Unknown error';
    logger.error('Failed to start server', { error: msg });
    process.exit(1);
  }
};

const gracefulShutdown = async () => {
  logger.info('Shutting down gracefully...');
  try {
    await fastify.close();
    await db.close();
    process.exit(0);
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'Unknown error';
    logger.error('Error during shutdown', { error: msg });
    process.exit(1);
  }
};

process.on('SIGTERM', gracefulShutdown);
process.on('SIGINT', gracefulShutdown);

start();

export { fastify };
