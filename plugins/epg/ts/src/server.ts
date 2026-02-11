/**
 * EPG Plugin Server
 * HTTP server for electronic program guide API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { EpgDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateChannelRequest,
  UpdateChannelRequest,
  ListChannelsQuery,
  CreateProgramRequest,
  SearchProgramsRequest,
  GetScheduleQuery,
  GetScheduleChannelQuery,
  GetScheduleProgramQuery,
  GetTonightQuery,
  CreateChannelGroupRequest,
  UpdateChannelGroupRequest,
  ImportXmltvRequest,
  ImportManualRequest,
  ChannelRecord,
  ChannelGroupRecord,
} from './types.js';

const logger = createLogger('epg:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new EpgDatabase();
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 50 * 1024 * 1024, // 50MB for XMLTV imports
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

  function scopedDb(request: unknown): EpgDatabase {
    return (request as Record<string, unknown>).scopedDb as EpgDatabase;
  }

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'epg', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'epg', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'epg',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'epg',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        totalChannels: stats.total_channels,
        activeChannels: stats.active_channels,
        totalPrograms: stats.total_programs,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Channel Endpoints
  // =========================================================================

  app.post<{ Body: CreateChannelRequest }>('/api/channels', async (request, reply) => {
    try {
      const channel = await scopedDb(request).createChannel({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        channel_number: request.body.channel_number ?? null,
        call_sign: request.body.call_sign ?? null,
        name: request.body.name,
        display_name: request.body.display_name ?? null,
        logo_url: request.body.logo_url ?? null,
        category: request.body.category ?? null,
        language: request.body.language ?? 'en',
        country: request.body.country ?? 'US',
        stream_url: request.body.stream_url ?? null,
        stream_type: request.body.stream_type ?? null,
        is_hd: request.body.is_hd ?? false,
        is_4k: request.body.is_4k ?? false,
        is_active: true,
        sort_order: 0,
        metadata: {},
      });

      return reply.status(201).send(channel);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create channel', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: ListChannelsQuery }>('/api/channels', async (request) => {
    const channels = await scopedDb(request).listChannels({
      category: request.query.category,
      isActive: request.query.is_active === 'true' ? true : request.query.is_active === 'false' ? false : undefined,
      groupId: request.query.group_id,
      limit: request.query.limit ? parseInt(String(request.query.limit), 10) : 200,
      offset: request.query.offset ? parseInt(String(request.query.offset), 10) : undefined,
    });

    return { channels, count: channels.length };
  });

  app.get<{ Params: { id: string } }>('/api/channels/:id', async (request, reply) => {
    const channel = await scopedDb(request).getChannel(request.params.id);
    if (!channel) {
      return reply.status(404).send({ error: 'Channel not found' });
    }
    return channel;
  });

  app.put<{ Params: { id: string }; Body: UpdateChannelRequest }>('/api/channels/:id', async (request, reply) => {
    const channel = await scopedDb(request).updateChannel(
      request.params.id,
      request.body as Partial<ChannelRecord>
    );
    if (!channel) {
      return reply.status(404).send({ error: 'Channel not found' });
    }
    return channel;
  });

  app.delete<{ Params: { id: string } }>('/api/channels/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteChannel(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Channel not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Channel Group Endpoints
  // =========================================================================

  app.post<{ Body: CreateChannelGroupRequest }>('/api/channel-groups', async (request, reply) => {
    try {
      const group = await scopedDb(request).createChannelGroup({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        name: request.body.name,
        description: request.body.description ?? null,
        sort_order: 0,
        metadata: {},
      });

      // Add channels if specified
      if (request.body.channel_ids && request.body.channel_ids.length > 0) {
        for (let i = 0; i < request.body.channel_ids.length; i++) {
          await scopedDb(request).addChannelToGroup(group.id, request.body.channel_ids[i], i);
        }
      }

      return reply.status(201).send(group);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create channel group', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/channel-groups', async (request) => {
    const groups = await scopedDb(request).listChannelGroups();
    return { groups, count: groups.length };
  });

  app.put<{ Params: { id: string }; Body: UpdateChannelGroupRequest }>('/api/channel-groups/:id', async (request, reply) => {
    const group = await scopedDb(request).updateChannelGroup(
      request.params.id,
      request.body as Partial<ChannelGroupRecord>
    );
    if (!group) {
      return reply.status(404).send({ error: 'Channel group not found' });
    }
    return group;
  });

  app.delete<{ Params: { id: string } }>('/api/channel-groups/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteChannelGroup(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Channel group not found' });
    }
    return { success: true };
  });

  app.post<{ Params: { id: string }; Body: { channel_id: string; sort_order?: number } }>(
    '/api/channel-groups/:id/channels',
    async (request, reply) => {
      try {
        const member = await scopedDb(request).addChannelToGroup(
          request.params.id,
          request.body.channel_id,
          request.body.sort_order ?? 0
        );
        return reply.status(201).send(member);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to add channel to group', { error: message });
        return reply.status(500).send({ error: message });
      }
    }
  );

  app.delete<{ Params: { id: string; channelId: string } }>(
    '/api/channel-groups/:id/channels/:channelId',
    async (request, reply) => {
      const removed = await scopedDb(request).removeChannelFromGroup(
        request.params.id,
        request.params.channelId
      );
      if (!removed) {
        return reply.status(404).send({ error: 'Channel not in group' });
      }
      return { success: true };
    }
  );

  // =========================================================================
  // Program Endpoints
  // =========================================================================

  app.post<{ Body: CreateProgramRequest }>('/api/programs', async (request, reply) => {
    try {
      const program = await scopedDb(request).createProgram({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        external_id: null,
        title: request.body.title,
        episode_title: request.body.episode_title ?? null,
        description: request.body.description ?? null,
        long_description: null,
        categories: request.body.categories ?? [],
        genre: request.body.genre ?? null,
        season_number: request.body.season_number ?? null,
        episode_number: request.body.episode_number ?? null,
        original_air_date: request.body.original_air_date ? new Date(request.body.original_air_date) : null,
        year: request.body.year ?? null,
        duration_minutes: request.body.duration_minutes ?? null,
        content_rating: request.body.content_rating ?? null,
        star_rating: request.body.star_rating ?? null,
        poster_url: request.body.poster_url ?? null,
        thumbnail_url: request.body.thumbnail_url ?? null,
        directors: request.body.directors ?? [],
        actors: request.body.actors ?? [],
        is_new: request.body.is_new ?? false,
        is_live: request.body.is_live ?? false,
        is_premiere: request.body.is_premiere ?? false,
        is_finale: request.body.is_finale ?? false,
        is_movie: request.body.is_movie ?? false,
        language: 'en',
        subtitles: [],
        audio_format: null,
        video_format: null,
        production_code: null,
        metadata: {},
      });

      return reply.status(201).send(program);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create program', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Params: { id: string } }>('/api/programs/:id', async (request, reply) => {
    const program = await scopedDb(request).getProgram(request.params.id);
    if (!program) {
      return reply.status(404).send({ error: 'Program not found' });
    }
    return program;
  });

  app.post<{ Body: SearchProgramsRequest }>('/api/programs/search', async (request, reply) => {
    try {
      const programs = await scopedDb(request).searchPrograms({
        query: request.body.query,
        genre: request.body.genre,
        contentRating: request.body.content_rating,
        isMovie: request.body.is_movie,
        language: request.body.language,
        limit: request.body.limit,
      });

      return { programs, count: programs.length };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Search failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Schedule Endpoints
  // =========================================================================

  app.get<{ Querystring: GetScheduleQuery }>('/api/schedule', async (request) => {
    const hours = request.query.hours ? parseInt(String(request.query.hours), 10) : 6;
    const startTime = request.query.date ? new Date(request.query.date) : new Date();
    const endTime = new Date(startTime.getTime() + hours * 60 * 60 * 1000);

    const channelIds = request.query.channel_ids
      ? request.query.channel_ids.split(',').map(s => s.trim())
      : undefined;

    const schedule = await scopedDb(request).getScheduleGrid({
      channelIds,
      startTime,
      endTime,
    });

    return { channels: schedule };
  });

  app.get('/api/schedule/now', async (request) => {
    const queryObj = request.query as { channel_ids?: string };
    const channelIds = queryObj.channel_ids
      ? queryObj.channel_ids.split(',').map(s => s.trim())
      : undefined;

    const now = await scopedDb(request).getWhatsOnNow(channelIds);
    return { now, count: now.length };
  });

  app.get<{ Querystring: GetTonightQuery }>('/api/schedule/tonight', async (request) => {
    const date = request.query.date ? new Date(request.query.date) : new Date();

    // Parse primetime hours
    const [startHour, startMin] = fullConfig.primetimeStart.split(':').map(Number);
    const [endHour, endMin] = fullConfig.primetimeEnd.split(':').map(Number);

    const startTime = new Date(date);
    startTime.setHours(startHour, startMin, 0, 0);

    const endTime = new Date(date);
    endTime.setHours(endHour, endMin, 0, 0);

    // If end is before start (crosses midnight), add a day
    if (endTime <= startTime) {
      endTime.setDate(endTime.getDate() + 1);
    }

    const schedule = await scopedDb(request).getScheduleGrid({
      startTime,
      endTime,
    });

    return { channels: schedule };
  });

  app.get<{ Params: { id: string }; Querystring: GetScheduleChannelQuery }>(
    '/api/schedule/channel/:id',
    async (request, reply) => {
      const channel = await scopedDb(request).getChannel(request.params.id);
      if (!channel) {
        return reply.status(404).send({ error: 'Channel not found' });
      }

      const startDate = request.query.date ? new Date(request.query.date) : new Date();
      startDate.setHours(0, 0, 0, 0);
      const days = request.query.days ? parseInt(String(request.query.days), 10) : 7;

      const schedule = await scopedDb(request).getScheduleForChannel(
        request.params.id,
        startDate,
        days
      );

      return {
        channel,
        schedule,
        count: schedule.length,
      };
    }
  );

  app.get<{ Params: { id: string }; Querystring: GetScheduleProgramQuery }>(
    '/api/schedule/program/:id',
    async (request, reply) => {
      const program = await scopedDb(request).getProgram(request.params.id);
      if (!program) {
        return reply.status(404).send({ error: 'Program not found' });
      }

      const days = request.query.days ? parseInt(String(request.query.days), 10) : 14;
      const airings = await scopedDb(request).getUpcomingAirings(request.params.id, days);

      return {
        program,
        airings,
        count: airings.length,
      };
    }
  );

  // =========================================================================
  // Import Endpoints
  // =========================================================================

  app.post<{ Body: ImportXmltvRequest }>('/api/import/xmltv', async (request, reply) => {
    try {
      // XMLTV import is a placeholder - in production, this would parse actual XMLTV XML
      logger.info('XMLTV import requested', {
        hasUrl: !!request.body.url,
        hasData: !!request.body.xml_data,
      });

      return reply.status(202).send({
        message: 'XMLTV import initiated',
        channels_imported: 0,
        programs_imported: 0,
        schedules_imported: 0,
        errors: [],
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('XMLTV import failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/import/schedules-direct', async (request, reply) => {
    try {
      const body = request.body as { lineup?: string };
      logger.info('Schedules Direct import requested', { lineup: body.lineup });

      return reply.status(202).send({
        message: 'Schedules Direct import initiated',
        lineup: body.lineup ?? fullConfig.schedulesDirectLineup,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Schedules Direct import failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Body: ImportManualRequest }>('/api/import/manual', async (request, reply) => {
    try {
      let schedulesImported = 0;
      let programsImported = 0;
      const errors: string[] = [];

      for (const entry of request.body.schedules) {
        try {
          // Create program
          const program = await scopedDb(request).createProgram({
            source_account_id: scopedDb(request).getCurrentSourceAccountId(),
            external_id: null,
            title: entry.program_title,
            episode_title: null,
            description: entry.description ?? null,
            long_description: null,
            categories: entry.categories ?? [],
            genre: null,
            season_number: null,
            episode_number: null,
            original_air_date: null,
            year: null,
            duration_minutes: null,
            content_rating: null,
            star_rating: null,
            poster_url: null,
            thumbnail_url: null,
            directors: [],
            actors: [],
            is_new: false,
            is_live: entry.is_live ?? false,
            is_premiere: false,
            is_finale: false,
            is_movie: false,
            language: 'en',
            subtitles: [],
            audio_format: null,
            video_format: null,
            production_code: null,
            metadata: {},
          });
          programsImported++;

          // Create schedule
          await scopedDb(request).createSchedule({
            source_account_id: scopedDb(request).getCurrentSourceAccountId(),
            channel_id: entry.channel_id,
            program_id: program.id,
            start_time: new Date(entry.start_time),
            end_time: new Date(entry.end_time),
            is_rerun: false,
            is_live: entry.is_live ?? false,
            metadata: {},
          });
          schedulesImported++;
        } catch (err) {
          const errMessage = err instanceof Error ? err.message : 'Unknown error';
          errors.push(`Failed to import "${entry.program_title}": ${errMessage}`);
        }
      }

      return {
        channels_imported: 0,
        programs_imported: programsImported,
        schedules_imported: schedulesImported,
        errors,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Manual import failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/import/status', async () => {
    return {
      last_import: null,
      status: 'idle',
      sources: fullConfig.xmltvUrls,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Sync Endpoints
  // =========================================================================

  app.post('/api/sync', async (_request, reply) => {
    try {
      logger.info('EPG sync triggered');

      return reply.status(202).send({
        message: 'EPG sync initiated',
        timestamp: new Date().toISOString(),
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync trigger failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/sync/status', async () => {
    return {
      status: 'idle',
      last_sync: null,
      sources: {
        xmltv: fullConfig.xmltvUrls,
        schedulesDirect: fullConfig.schedulesDirectLineup || null,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'epg',
      version: '1.0.0',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Server Lifecycle
  // =========================================================================

  const server = {
    async start() {
      try {
        await app.listen({ port: fullConfig.port, host: fullConfig.host });
        logger.info(`EPG server listening on ${fullConfig.host}:${fullConfig.port}`);
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

export async function startServer(config?: Partial<Config>): Promise<void> {
  const server = await createServer(config);
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

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
