/**
 * Calendar Plugin Server
 * HTTP server for calendar API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { CalendarDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import { expandRecurrence, validateRRule, generateRRule, describeRRule } from './rrule.js';
import { generateICalendar } from './ical.js';
import type {
  CreateCalendarRequest,
  UpdateCalendarRequest,
  CreateEventRequest,
  UpdateEventRequest,
  CreateAttendeeRequest,
  RSVPRequest,
  CheckInRequest,
  CreateICalFeedRequest,
  ListEventsQuery,
  CalendarRecord,
  CalendarEventRecord,
  EventOccurrence,
} from './types.js';
import crypto from 'crypto';

const logger = createLogger('calendar:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new CalendarDatabase();
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

  function scopedDb(request: unknown): CalendarDatabase {
    return (request as Record<string, unknown>).scopedDb as CalendarDatabase;
  }

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'calendar', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'calendar', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'calendar',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'calendar',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        calendars: stats.calendars,
        events: stats.events,
        upcomingEvents: stats.upcomingEvents,
      },
      timestamp: new Date().toISOString(),
    };
  });

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'calendar',
      version: '1.0.0',
      status: 'running',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Calendar Endpoints
  // =========================================================================

  app.post<{ Body: CreateCalendarRequest }>('/v1/calendars', async (request, reply) => {
    try {
      const calendar = await scopedDb(request).createCalendar({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        name: request.body.name,
        description: request.body.description ?? null,
        color: request.body.color ?? '#3B82F6',
        owner_id: request.body.owner_id,
        owner_type: request.body.owner_type ?? 'user',
        is_default: request.body.is_default ?? false,
        timezone: request.body.timezone ?? fullConfig.defaultTimezone,
        visibility: request.body.visibility ?? 'private',
        metadata: request.body.metadata ?? {},
      });

      return reply.status(201).send(calendar);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create calendar', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: { owner_id?: string } }>('/v1/calendars', async (request) => {
    try {
          const calendars = await scopedDb(request).listCalendars(request.query.owner_id);
    return { calendars, count: calendars.length };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('GET /v1/calendars error', { error: message });
      throw error;
    }
  });

  app.get<{ Params: { id: string } }>('/v1/calendars/:id', async (request, reply) => {
    try {
          const calendar = await scopedDb(request).getCalendar(request.params.id);
    if (!calendar) {
      return reply.status(404).send({ error: 'Calendar not found' });
    }
    return calendar;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('GET /v1/calendars/:id error', { error: message });
      return reply.code(500).send({ error: message });
    }
  });

  app.put<{ Params: { id: string }; Body: UpdateCalendarRequest }>('/v1/calendars/:id', async (request, reply) => {
    const calendar = await scopedDb(request).updateCalendar(request.params.id, request.body as Partial<CalendarRecord>);
    if (!calendar) {
      return reply.status(404).send({ error: 'Calendar not found' });
    }
    return calendar;
  });

  app.delete<{ Params: { id: string } }>('/v1/calendars/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteCalendar(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Calendar not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Event Endpoints
  // =========================================================================

  app.post<{ Body: CreateEventRequest }>('/v1/events', async (request, reply) => {
    try {
      const body = request.body;

      // Validate RRULE if provided
      if (body.recurrence_rule) {
        const validation = validateRRule(body.recurrence_rule);
        if (!validation.valid) {
          return reply.status(400).send({ error: `Invalid recurrence rule: ${validation.error}` });
        }
      }

      // Generate series_id for recurring events
      const seriesId = body.recurrence_rule ? crypto.randomUUID() : null;

      const event = await scopedDb(request).createEvent({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        calendar_id: body.calendar_id,
        title: body.title,
        description: body.description ?? null,
        event_type: body.event_type ?? 'event',
        start_at: new Date(body.start_at),
        end_at: body.end_at ? new Date(body.end_at) : null,
        all_day: body.all_day ?? false,
        timezone: body.timezone ?? fullConfig.defaultTimezone,
        location_name: body.location_name ?? null,
        location_address: body.location_address ?? null,
        location_lat: body.location_lat ?? null,
        location_lon: body.location_lon ?? null,
        recurrence_rule: body.recurrence_rule ?? null,
        recurrence_end_at: body.recurrence_end_at ? new Date(body.recurrence_end_at) : null,
        series_id: seriesId,
        is_exception: false,
        original_start_at: null,
        color: body.color ?? null,
        url: body.url ?? null,
        organizer_id: body.organizer_id ?? null,
        status: body.status ?? 'confirmed',
        visibility: body.visibility ?? 'default',
        reminder_minutes: body.reminder_minutes ?? [15],
        attendee_count: 0,
        metadata: body.metadata ?? {},
      });

      // Add attendees if provided
      if (body.attendees && body.attendees.length > 0) {
        for (const attendeeReq of body.attendees) {
          await scopedDb(request).addAttendee({
            source_account_id: scopedDb(request).getCurrentSourceAccountId(),
            event_id: event.id,
            user_id: attendeeReq.user_id ?? null,
            email: attendeeReq.email ?? null,
            name: attendeeReq.name ?? null,
            rsvp_status: 'pending',
            rsvp_at: null,
            role: attendeeReq.role ?? 'attendee',
            checked_in: false,
            checked_in_at: null,
          });
        }
      }

      return reply.status(201).send(event);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create event', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: ListEventsQuery }>('/v1/events', async (request) => {
    const query = request.query;
    const events = await scopedDb(request).listEvents({
      calendarId: query.calendar_id,
      start: query.start ? new Date(query.start) : undefined,
      end: query.end ? new Date(query.end) : undefined,
      type: query.type,
      status: query.status,
      limit: query.limit ? parseInt(String(query.limit), 10) : undefined,
      offset: query.offset ? parseInt(String(query.offset), 10) : undefined,
    });

    return { events, count: events.length };
  });

  app.get<{ Params: { id: string } }>('/v1/events/:id', async (request, reply) => {
    const event = await scopedDb(request).getEvent(request.params.id);
    if (!event) {
      return reply.status(404).send({ error: 'Event not found' });
    }
    return event;
  });

  app.put<{ Params: { id: string }; Body: UpdateEventRequest }>('/v1/events/:id', async (request, reply) => {
    // Validate RRULE if provided
    if (request.body.recurrence_rule) {
      const validation = validateRRule(request.body.recurrence_rule);
      if (!validation.valid) {
        return reply.status(400).send({ error: `Invalid recurrence rule: ${validation.error}` });
      }
    }

    const updates: Partial<CalendarEventRecord> = {};
    for (const [key, value] of Object.entries(request.body)) {
      if (['start_at', 'end_at', 'recurrence_end_at'].includes(key) && value) {
        updates[key as keyof CalendarEventRecord] = new Date(value as string) as never;
      } else if (value !== undefined) {
        updates[key as keyof CalendarEventRecord] = value as never;
      }
    }

    const event = await scopedDb(request).updateEvent(request.params.id, updates);
    if (!event) {
      return reply.status(404).send({ error: 'Event not found' });
    }
    return event;
  });

  app.delete<{ Params: { id: string }; Querystring: { scope?: 'this' | 'this_and_future' | 'all' } }>(
    '/v1/events/:id',
    async (request, reply) => {
      const event = await scopedDb(request).getEvent(request.params.id);
      if (!event) {
        return reply.status(404).send({ error: 'Event not found' });
      }

      const scope = request.query.scope ?? 'this';

      if (!event.recurrence_rule || scope === 'all') {
        // Delete single event or entire series
        const deleted = await scopedDb(request).deleteEvent(request.params.id);
        return { success: deleted };
      }

      if (scope === 'this') {
        // Create exception for this occurrence
        const { id, created_at, updated_at, ...eventData } = event;
        await scopedDb(request).createEvent({
          ...eventData,
          is_exception: true,
          original_start_at: event.start_at,
          status: 'cancelled',
          recurrence_rule: null,
        });
        return { success: true, action: 'exception_created' };
      }

      if (scope === 'this_and_future') {
        // Set UNTIL on the recurrence rule
        const untilDate = new Date(event.start_at.getTime() - 1000);
        await scopedDb(request).updateEvent(request.params.id, {
          recurrence_end_at: untilDate,
        });
        return { success: true, action: 'recurrence_ended' };
      }

      return { success: false, error: 'Invalid scope' };
    }
  );

  app.post<{ Params: { id: string } }>('/v1/events/:id/duplicate', async (request, reply) => {
    const original = await scopedDb(request).getEvent(request.params.id);
    if (!original) {
      return reply.status(404).send({ error: 'Event not found' });
    }

    const { id, created_at, updated_at, ...originalData } = original;
    const duplicate = await scopedDb(request).createEvent({
      ...originalData,
      title: `${original.title} (Copy)`,
      series_id: null,
      is_exception: false,
      original_start_at: null,
    });

    return reply.status(201).send(duplicate);
  });

  app.get('/v1/events/upcoming', async (request) => {
    const limit = parseInt(String((request.query as { limit?: string }).limit ?? '10'), 10);
    const events = await scopedDb(request).getUpcomingEvents(limit);
    return { events, count: events.length };
  });

  app.get('/v1/events/today', async (request) => {
    const events = await scopedDb(request).getTodayEvents();
    return { events, count: events.length };
  });

  app.get<{ Querystring: { start: string; end: string; calendar_id?: string } }>(
    '/v1/events/range',
    async (request, reply) => {
      const { start, end, calendar_id } = request.query;

      if (!start || !end) {
        return reply.status(400).send({ error: 'start and end parameters required' });
      }

      const startDate = new Date(start);
      const endDate = new Date(end);

      // Get base events
      const events = await scopedDb(request).listEvents({
        calendarId: calendar_id,
        start: startDate,
        end: endDate,
      });

      // Expand recurring events
      const allOccurrences: EventOccurrence[] = [];

      for (const event of events) {
        const occurrences = expandRecurrence(event, startDate, endDate, fullConfig.maxRecurrenceExpand);
        allOccurrences.push(...occurrences);
      }

      // Sort by occurrence start
      allOccurrences.sort((a, b) => a.occurrence_start.getTime() - b.occurrence_start.getTime());

      return { events: allOccurrences, count: allOccurrences.length };
    }
  );

  // =========================================================================
  // Attendee Endpoints
  // =========================================================================

  app.post<{ Params: { id: string }; Body: CreateAttendeeRequest[] }>(
    '/v1/events/:id/attendees',
    async (request, reply) => {
      const event = await scopedDb(request).getEvent(request.params.id);
      if (!event) {
        return reply.status(404).send({ error: 'Event not found' });
      }

      const attendees = [];
      for (const attendeeReq of request.body) {
        const attendee = await scopedDb(request).addAttendee({
          source_account_id: scopedDb(request).getCurrentSourceAccountId(),
          event_id: request.params.id,
          user_id: attendeeReq.user_id ?? null,
          email: attendeeReq.email ?? null,
          name: attendeeReq.name ?? null,
          rsvp_status: 'pending',
          rsvp_at: null,
          role: attendeeReq.role ?? 'attendee',
          checked_in: false,
          checked_in_at: null,
        });
        attendees.push(attendee);
      }

      return reply.status(201).send({ attendees, count: attendees.length });
    }
  );

  app.get<{ Params: { id: string } }>('/v1/events/:id/attendees', async (request, reply) => {
    const event = await scopedDb(request).getEvent(request.params.id);
    if (!event) {
      return reply.status(404).send({ error: 'Event not found' });
    }

    const attendees = await scopedDb(request).listAttendees(request.params.id);
    return { attendees, count: attendees.length };
  });

  app.delete<{ Params: { id: string; userId: string } }>('/v1/events/:id/attendees/:userId', async (request, reply) => {
    const deleted = await scopedDb(request).removeAttendee(request.params.id, request.params.userId);
    if (!deleted) {
      return reply.status(404).send({ error: 'Attendee not found' });
    }
    return { success: true };
  });

  app.post<{ Params: { id: string }; Body: RSVPRequest }>('/v1/events/:id/rsvp', async (request, reply) => {
    const event = await scopedDb(request).getEvent(request.params.id);
    if (!event) {
      return reply.status(404).send({ error: 'Event not found' });
    }

    const userId = request.body.user_id ?? 'current_user';
    const attendee = await scopedDb(request).updateRSVP(request.params.id, userId, request.body.status);

    if (!attendee) {
      return reply.status(404).send({ error: 'Attendee not found' });
    }

    return attendee;
  });

  app.post<{ Params: { id: string }; Body: CheckInRequest }>('/v1/events/:id/checkin', async (request, reply) => {
    const event = await scopedDb(request).getEvent(request.params.id);
    if (!event) {
      return reply.status(404).send({ error: 'Event not found' });
    }

    const attendee = await scopedDb(request).checkInAttendee(request.params.id, request.body.user_id);
    if (!attendee) {
      return reply.status(404).send({ error: 'Attendee not found' });
    }

    return attendee;
  });

  // =========================================================================
  // Birthday & Special Events
  // =========================================================================

  app.get<{ Querystring: { days?: string } }>('/v1/birthdays', async (request) => {
    const days = parseInt(String(request.query.days ?? '30'), 10);
    const birthdays = await scopedDb(request).getUpcomingBirthdays(days);
    return { birthdays, count: birthdays.length };
  });

  // =========================================================================
  // iCal Feed Endpoints
  // =========================================================================

  app.post<{ Body: CreateICalFeedRequest }>('/v1/ical-feeds', async (request, reply) => {
    try {
      const token = crypto.randomBytes(fullConfig.icalTokenLength / 2).toString('hex');

      const feed = await scopedDb(request).createICalFeed({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        calendar_id: request.body.calendar_id,
        token,
        name: request.body.name ?? null,
        enabled: true,
        last_accessed_at: null,
      });

      return reply.status(201).send(feed);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create iCal feed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: { calendar_id?: string } }>('/v1/ical-feeds', async (request) => {
    const feeds = await scopedDb(request).listICalFeeds(request.query.calendar_id);
    return { feeds, count: feeds.length };
  });

  app.delete<{ Params: { id: string } }>('/v1/ical-feeds/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteICalFeed(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'iCal feed not found' });
    }
    return { success: true };
  });

  // Public iCal endpoint (no auth required)
  app.get<{ Params: { token: string } }>('/v1/ical/:token', async (request, reply) => {
    const feed = await db.getICalFeedByToken(request.params.token);
    if (!feed) {
      return reply.status(404).send({ error: 'Feed not found' });
    }

    const calendar = await db.forSourceAccount(feed.source_account_id).getCalendar(feed.calendar_id);
    if (!calendar) {
      return reply.status(404).send({ error: 'Calendar not found' });
    }

    const events = await db.forSourceAccount(feed.source_account_id).listEvents({
      calendarId: feed.calendar_id,
    });

    // Get attendees for all events
    const attendeesMap = new Map();
    for (const event of events) {
      const attendees = await db.forSourceAccount(feed.source_account_id).listAttendees(event.id);
      attendeesMap.set(event.id, attendees);
    }

    const icalContent = generateICalendar(calendar, events, attendeesMap);

    return reply
      .header('Content-Type', 'text/calendar; charset=utf-8')
      .header('Content-Disposition', `attachment; filename="${calendar.name}.ics"`)
      .send(icalContent);
  });

  // =========================================================================
  // Availability Endpoint
  // =========================================================================

  app.get<{ Querystring: { calendar_id: string; start: string; end: string; duration?: string } }>(
    '/v1/availability',
    async (request, reply) => {
      const { calendar_id, start, end, duration = '60' } = request.query;

      if (!calendar_id || !start || !end) {
        return reply.status(400).send({ error: 'calendar_id, start, and end parameters required' });
      }

      const startDate = new Date(start);
      const endDate = new Date(end);
      const durationMinutes = parseInt(duration, 10);

      // Get existing events in range
      const events = await scopedDb(request).listEvents({
        calendarId: calendar_id,
        start: startDate,
        end: endDate,
        status: 'confirmed',
      });

      // Simple availability calculation (can be enhanced)
      const busySlots = events.map(e => ({
        start: e.start_at,
        end: e.end_at ?? new Date(e.start_at.getTime() + 60 * 60 * 1000),
      }));

      return {
        calendar_id,
        start: startDate,
        end: endDate,
        duration_minutes: durationMinutes,
        busy_slots: busySlots,
        busy_count: busySlots.length,
      };
    }
  );

  // =========================================================================
  // RRULE Helper Endpoints
  // =========================================================================

  app.post<{ Body: { rrule: string } }>('/v1/rrule/validate', async (request) => {
    const validation = validateRRule(request.body.rrule);
    return validation;
  });

  app.post<{ Body: { rrule: string } }>('/v1/rrule/describe', async (request) => {
    const description = describeRRule(request.body.rrule);
    return { description };
  });

  app.post<{
    Body: {
      frequency: 'DAILY' | 'WEEKLY' | 'MONTHLY' | 'YEARLY';
      interval?: number;
      count?: number;
      until?: string;
      byDay?: string[];
      byMonthDay?: number[];
      byMonth?: number[];
    };
  }>('/v1/rrule/generate', async (request) => {
    const pattern = {
      ...request.body,
      until: request.body.until ? new Date(request.body.until) : undefined,
    };
    const rrule = generateRRule(pattern);
    const description = describeRRule(rrule);
    return { rrule, description };
  });

  // =========================================================================
  // Server Lifecycle
  // =========================================================================

  const server = {
    async start() {
      try {
        await app.listen({ port: fullConfig.port, host: fullConfig.host });
        logger.info(`Calendar server listening on ${fullConfig.host}:${fullConfig.port}`);
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

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  const server = await createServer();
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
