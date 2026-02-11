#!/usr/bin/env node
/**
 * HTTP server for meetings API
 * Multi-app aware: each request is scoped to a source_account_id
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, getAppContext } from '@nself/plugin-utils';
import { config } from './config.js';
import { db, DatabaseClient } from './database.js';
import {
  CreateEventInput, UpdateEventInput, ListEventsQuery,
  CreateRoomInput, UpdateRoomInput, ListRoomsQuery,
  CreateTemplateInput, UpdateTemplateInput,
  RsvpInput,
} from './types.js';

const logger = createLogger('meetings:server');

const fastify = Fastify({ logger: { level: 'info' } });

// CORS
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
  status: 'ok', timestamp: new Date().toISOString(), service: 'meetings',
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
// Events
// =============================================================================

fastify.post<{ Body: CreateEventInput }>('/api/v1/events', async (request, reply) => {
  try {
    const event = await scopedDb(request).createEvent(request.body);
    if (request.body.attendees) {
      for (const attendee of request.body.attendees) {
        await scopedDb(request).addAttendee(event.id, attendee);
      }
    }
    return reply.code(201).send({ event });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to create event', { error: msg });
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Querystring: ListEventsQuery }>('/api/v1/events', async (request, reply) => {
  try {
    const result = await scopedDb(request).listEvents(request.query);
    return { data: result.events, total: result.total };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to list events', { error: msg });
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/events/:id', async (request, reply) => {
  try {
    const event = await scopedDb(request).getEvent(request.params.id);
    if (!event) return reply.code(404).send({ error: 'Event not found' });
    return { event };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.put<{ Params: { id: string }; Body: UpdateEventInput }>('/api/v1/events/:id', async (request, reply) => {
  try {
    const event = await scopedDb(request).updateEvent(request.params.id, request.body);
    if (!event) return reply.code(404).send({ error: 'Event not found' });
    return { event };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.delete<{ Params: { id: string } }>('/api/v1/events/:id', async (request, reply) => {
  try {
    const deleted = await scopedDb(request).deleteEvent(request.params.id);
    if (!deleted) return reply.code(404).send({ error: 'Event not found' });
    return { success: true };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string }; Body: RsvpInput }>('/api/v1/events/:id/rsvp', async (request, reply) => {
  try {
    const attendee = await scopedDb(request).updateAttendeeRsvp(request.params.id, request.body);
    if (!attendee) return reply.code(404).send({ error: 'Attendee not found' });
    return { attendee };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/events/:id/attendees', async (request, reply) => {
  try {
    const attendees = await scopedDb(request).getEventAttendees(request.params.id);
    return { data: attendees, total: attendees.length };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string } }>('/api/v1/events/:id/remind', async (_request, _reply) => {
  // Placeholder for sending reminders
  return { success: true, message: 'Reminder sent' };
});

// =============================================================================
// Rooms
// =============================================================================

fastify.post<{ Body: CreateRoomInput }>('/api/v1/rooms', async (request, reply) => {
  try {
    const room = await scopedDb(request).createRoom(request.body);
    return reply.code(201).send({ room });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Querystring: ListRoomsQuery }>('/api/v1/rooms', async (request, reply) => {
  try {
    const result = await scopedDb(request).listRooms(request.query);
    return { data: result.rooms, total: result.total };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/rooms/:id', async (request, reply) => {
  try {
    const room = await scopedDb(request).getRoom(request.params.id);
    if (!room) return reply.code(404).send({ error: 'Room not found' });
    return { room };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.put<{ Params: { id: string }; Body: UpdateRoomInput }>('/api/v1/rooms/:id', async (request, reply) => {
  try {
    const room = await scopedDb(request).updateRoom(request.params.id, request.body);
    if (!room) return reply.code(404).send({ error: 'Room not found' });
    return { room };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.delete<{ Params: { id: string } }>('/api/v1/rooms/:id', async (request, reply) => {
  try {
    const deleted = await scopedDb(request).deleteRoom(request.params.id);
    if (!deleted) return reply.code(404).send({ error: 'Room not found' });
    return { success: true };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string }; Querystring: { start_time: string; end_time: string } }>(
  '/api/v1/rooms/:id/availability',
  async (request, reply) => {
    try {
      const available = await scopedDb(request).checkRoomAvailability(
        request.params.id, request.query.start_time, request.query.end_time
      );
      return { available };
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      return reply.code(500).send({ error: msg });
    }
  }
);

fastify.post<{ Params: { id: string }; Body: { event_id: string; start_time: string; end_time: string; organizer_id: string } }>(
  '/api/v1/rooms/:id/book',
  async (request, reply) => {
    try {
      const available = await scopedDb(request).checkRoomAvailability(
        request.params.id, request.body.start_time, request.body.end_time
      );
      if (!available) {
        return reply.code(409).send({ error: 'Room is not available for the requested time' });
      }
      const event = await scopedDb(request).updateEvent(request.body.event_id, {
        room_id: request.params.id,
      });
      return { success: true, event };
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      return reply.code(500).send({ error: msg });
    }
  }
);

fastify.get<{ Querystring: { attendee_count: string; start_time: string; end_time: string } }>(
  '/api/v1/rooms/suggest',
  async (request, reply) => {
    try {
      const rooms = await scopedDb(request).suggestRooms(
        parseInt(request.query.attendee_count, 10),
        request.query.start_time,
        request.query.end_time
      );
      return { data: rooms, total: rooms.length };
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      return reply.code(500).send({ error: msg });
    }
  }
);

// =============================================================================
// Calendar Sync (placeholders)
// =============================================================================

fastify.post('/api/v1/sync/google/connect', async (_request, reply) => {
  return reply.code(501).send({ error: 'Google Calendar sync not yet implemented' });
});

fastify.post('/api/v1/sync/google/disconnect', async (_request, reply) => {
  return reply.code(501).send({ error: 'Google Calendar sync not yet implemented' });
});

fastify.post('/api/v1/sync/google/sync', async (_request, reply) => {
  return reply.code(501).send({ error: 'Google Calendar sync not yet implemented' });
});

fastify.get('/api/v1/sync/google/status', async (_request, _reply) => {
  return { status: 'not_connected' };
});

fastify.post('/api/v1/sync/outlook/connect', async (_request, reply) => {
  return reply.code(501).send({ error: 'Outlook Calendar sync not yet implemented' });
});

fastify.post('/api/v1/sync/outlook/disconnect', async (_request, reply) => {
  return reply.code(501).send({ error: 'Outlook Calendar sync not yet implemented' });
});

fastify.post('/api/v1/sync/outlook/sync', async (_request, reply) => {
  return reply.code(501).send({ error: 'Outlook Calendar sync not yet implemented' });
});

fastify.get('/api/v1/sync/outlook/status', async (_request, _reply) => {
  return { status: 'not_connected' };
});

// =============================================================================
// Availability
// =============================================================================

fastify.get<{ Params: { userId: string }; Querystring: { start_time: string; end_time: string } }>(
  '/api/v1/availability/:userId',
  async (request, reply) => {
    try {
      const result = await scopedDb(request).getUserAvailability(
        request.params.userId, request.query.start_time, request.query.end_time
      );
      return result;
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      return reply.code(500).send({ error: msg });
    }
  }
);

fastify.post<{ Body: { user_ids: string[]; start_time: string; end_time: string } }>(
  '/api/v1/availability/check',
  async (request, reply) => {
    try {
      const results: Record<string, { is_available: boolean; conflicting_events: string[] }> = {};
      for (const userId of request.body.user_ids) {
        results[userId] = await scopedDb(request).getUserAvailability(
          userId, request.body.start_time, request.body.end_time
        );
      }
      return { results };
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      return reply.code(500).send({ error: msg });
    }
  }
);

fastify.get<{ Querystring: { user_ids: string; start_time: string; end_time: string } }>(
  '/api/v1/availability/free-busy',
  async (request, reply) => {
    try {
      const userIds = request.query.user_ids.split(',');
      const results: Record<string, { is_available: boolean; conflicting_events: string[] }> = {};
      for (const userId of userIds) {
        results[userId] = await scopedDb(request).getUserAvailability(
          userId.trim(), request.query.start_time, request.query.end_time
        );
      }
      return { results };
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      return reply.code(500).send({ error: msg });
    }
  }
);

fastify.post<{ Body: { attendee_ids: string[]; duration_minutes: number; date: string } }>(
  '/api/v1/availability/suggest',
  async (_request, reply) => {
    // Simplified suggestion: return business hour slots
    return reply.code(501).send({ error: 'Time suggestion requires database function support' });
  }
);

// =============================================================================
// Templates
// =============================================================================

fastify.post<{ Body: CreateTemplateInput }>('/api/v1/templates', async (request, reply) => {
  try {
    const template = await scopedDb(request).createTemplate(request.body);
    return reply.code(201).send({ template });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get('/api/v1/templates', async (request, reply) => {
  try {
    const templates = await scopedDb(request).listTemplates();
    return { data: templates, total: templates.length };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/templates/:id', async (request, reply) => {
  try {
    const template = await scopedDb(request).getTemplate(request.params.id);
    if (!template) return reply.code(404).send({ error: 'Template not found' });
    return { template };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.put<{ Params: { id: string }; Body: UpdateTemplateInput }>('/api/v1/templates/:id', async (request, reply) => {
  try {
    const template = await scopedDb(request).updateTemplate(request.params.id, request.body);
    if (!template) return reply.code(404).send({ error: 'Template not found' });
    return { template };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.delete<{ Params: { id: string } }>('/api/v1/templates/:id', async (request, reply) => {
  try {
    const deleted = await scopedDb(request).deleteTemplate(request.params.id);
    if (!deleted) return reply.code(404).send({ error: 'Template not found' });
    return { success: true };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string }; Body: { start_time: string; organizer_id: string } }>(
  '/api/v1/templates/:id/apply',
  async (request, reply) => {
    try {
      const template = await scopedDb(request).getTemplate(request.params.id);
      if (!template) return reply.code(404).send({ error: 'Template not found' });

      const startTime = new Date(request.body.start_time);
      const endTime = new Date(startTime.getTime() + template.default_duration_minutes * 60000);

      const event = await scopedDb(request).createEvent({
        title: template.default_title ?? template.name,
        description: template.default_description ?? undefined,
        location: template.default_location ?? undefined,
        video_link: template.default_video_link ?? undefined,
        start_time: startTime.toISOString(),
        end_time: endTime.toISOString(),
        organizer_id: request.body.organizer_id,
      });
      return reply.code(201).send({ event });
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      return reply.code(500).send({ error: msg });
    }
  }
);

// =============================================================================
// Server Lifecycle
// =============================================================================

const start = async () => {
  try {
    await db.initializeSchema();
    logger.info('Database schema initialized');

    await fastify.listen({ port: config.server.port, host: config.server.host });
    logger.info(`Meetings server running on http://${config.server.host}:${config.server.port}`);
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
