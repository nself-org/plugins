/**
 * EPG Plugin Server
 * HTTP server for electronic program guide API endpoints
 */

import crypto from 'node:crypto';
import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { EpgDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import {
  scheduleRecording,
  checkConflicts,
  resolveConflicts,
  matchSeriesRules,
} from './recording-trigger.js';
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
  CreateRecordingRuleRequest,
  ListScheduledRecordingsQuery,
  ConflictCheckQuery,
  ResolveConflictRequest,
  RecordingTriggerEvent,
  ProgramRecord,
} from './types.js';

const logger = createLogger('epg:server');

// =========================================================================
// XMLTV Parsing
// =========================================================================

interface XmltvChannel {
  id: string;
  name: string;
  displayName: string | null;
  number: string | null;
  icon: string | null;
  lang: string | null;
}

interface XmltvProgramme {
  channel: string;
  title: string;
  subTitle: string | null;
  desc: string | null;
  start: string;
  startTime: Date | null;
  endTime: Date | null;
  externalId: string | null;
  categories: string[];
  date: string | null;
  seasonNumber: number | null;
  episodeNumber: number | null;
  rating: string | null;
  starRating: number | null;
  icon: string | null;
  directors: string[];
  actors: string[];
  isNew: boolean;
  isLive: boolean;
  isPremiere: boolean;
  isRerun: boolean;
  lang: string | null;
}

/**
 * Parse XMLTV timestamp format: "20260214180000 +0000" or "20260214180000"
 */
function parseXmltvTime(timeStr: string | null): Date | null {
  if (!timeStr) return null;
  const cleaned = timeStr.trim();
  // Format: YYYYMMDDHHmmss [+/-offset]
  const match = cleaned.match(/^(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})\s*([+-]\d{4})?$/);
  if (!match) return null;

  const [, year, month, day, hour, minute, second, offset] = match;
  const isoStr = `${year}-${month}-${day}T${hour}:${minute}:${second}${offset ? formatOffset(offset) : 'Z'}`;

  const date = new Date(isoStr);
  return isNaN(date.getTime()) ? null : date;
}

/**
 * Parse XMLTV date format: "2026", "20260214", or "20260214180000"
 * Returns a Date or null.
 */
function parseXmltvDate(dateStr: string): Date | null {
  const cleaned = dateStr.trim();
  if (cleaned.length === 4) {
    // Year only: "2026" -> Jan 1 of that year
    const d = new Date(`${cleaned}-01-01T00:00:00Z`);
    return isNaN(d.getTime()) ? null : d;
  }
  if (cleaned.length >= 8) {
    const year = cleaned.substring(0, 4);
    const month = cleaned.substring(4, 6);
    const day = cleaned.substring(6, 8);
    const d = new Date(`${year}-${month}-${day}T00:00:00Z`);
    return isNaN(d.getTime()) ? null : d;
  }
  // Fallback: try native parsing
  const d = new Date(cleaned);
  return isNaN(d.getTime()) ? null : d;
}

function formatOffset(offset: string): string {
  // Convert "+0500" to "+05:00"
  return `${offset.slice(0, 3)}:${offset.slice(3)}`;
}

/**
 * Extract text content from a simple XML element.
 * Handles both <tag>text</tag> and <tag attr="val">text</tag>.
 */
function extractText(xml: string, tag: string): string | null {
  const regex = new RegExp(`<${tag}[^>]*>([^<]*)</${tag}>`, 'i');
  const match = xml.match(regex);
  return match ? decodeXmlEntities(match[1].trim()) : null;
}

/**
 * Extract all text values for a given tag (returns array).
 */
function extractAllText(xml: string, tag: string): string[] {
  const regex = new RegExp(`<${tag}[^>]*>([^<]*)</${tag}>`, 'gi');
  const results: string[] = [];
  let match;
  while ((match = regex.exec(xml)) !== null) {
    const text = decodeXmlEntities(match[1].trim());
    if (text) results.push(text);
  }
  return results;
}

/**
 * Extract an attribute value from an XML element string.
 */
function extractAttr(elementStr: string, attr: string): string | null {
  const regex = new RegExp(`${attr}\\s*=\\s*"([^"]*)"`, 'i');
  const match = elementStr.match(regex);
  return match ? decodeXmlEntities(match[1]) : null;
}

/**
 * Decode basic XML entities.
 */
function decodeXmlEntities(text: string): string {
  return text
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&#(\d+);/g, (_, code) => String.fromCharCode(parseInt(code, 10)))
    .replace(/&#x([0-9a-fA-F]+);/g, (_, code) => String.fromCharCode(parseInt(code, 16)));
}

/**
 * Parse XMLTV episode-num format (xmltv_ns): "season.episode.part"
 * Season and episode are 0-indexed in this format.
 */
function parseEpisodeNum(epNumBlock: string): { season: number | null; episode: number | null } {
  // Find xmltv_ns system
  const nsMatch = epNumBlock.match(/<episode-num[^>]*system\s*=\s*"xmltv_ns"[^>]*>([^<]*)<\/episode-num>/i);
  if (nsMatch) {
    const parts = nsMatch[1].trim().split('.');
    const season = parts[0] ? parseInt(parts[0], 10) : null;
    const episode = parts[1] ? parseInt(parts[1], 10) : null;
    return {
      season: season !== null && !isNaN(season) ? season + 1 : null,  // Convert 0-indexed to 1-indexed
      episode: episode !== null && !isNaN(episode) ? episode + 1 : null,
    };
  }

  // Try onscreen format
  const onscreenMatch = epNumBlock.match(/<episode-num[^>]*system\s*=\s*"onscreen"[^>]*>([^<]*)<\/episode-num>/i);
  if (onscreenMatch) {
    const text = onscreenMatch[1].trim();
    const seMatch = text.match(/S(\d+)\s*E(\d+)/i);
    if (seMatch) {
      return {
        season: parseInt(seMatch[1], 10),
        episode: parseInt(seMatch[2], 10),
      };
    }
  }

  return { season: null, episode: null };
}

/**
 * Parse XMLTV XML data into structured channels and programmes.
 * Uses simple regex-based parsing for the well-defined XMLTV format.
 */
function parseXmltvData(xml: string, errors: string[]): { channels: XmltvChannel[]; programmes: XmltvProgramme[] } {
  const channels: XmltvChannel[] = [];
  const programmes: XmltvProgramme[] = [];

  // Parse channels: <channel id="...">...</channel>
  const channelRegex = /<channel\s+([^>]*)>([\s\S]*?)<\/channel>/gi;
  let channelMatch;
  while ((channelMatch = channelRegex.exec(xml)) !== null) {
    try {
      const attrs = channelMatch[1];
      const body = channelMatch[2];
      const id = extractAttr(`<channel ${attrs}>`, 'id');
      if (!id) {
        errors.push('Channel element missing id attribute');
        continue;
      }

      const displayName = extractText(body, 'display-name');
      const icon = body.match(/<icon\s+[^>]*src\s*=\s*"([^"]*)"[^>]*\/?>/i)?.[1] ?? null;
      const lcn = extractText(body, 'lcn');

      channels.push({
        id,
        name: displayName ?? id,
        displayName,
        number: lcn,
        icon: icon ? decodeXmlEntities(icon) : null,
        lang: extractAttr(body.match(/<display-name[^>]*>/)?.[0] ?? '', 'lang'),
      });
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unknown error';
      errors.push(`Failed to parse channel element: ${msg}`);
    }
  }

  // Parse programmes: <programme start="..." stop="..." channel="...">...</programme>
  const progRegex = /<programme\s+([^>]*)>([\s\S]*?)<\/programme>/gi;
  let progMatch;
  while ((progMatch = progRegex.exec(xml)) !== null) {
    try {
      const attrs = progMatch[1];
      const body = progMatch[2];
      const attrStr = `<programme ${attrs}>`;
      const startStr = extractAttr(attrStr, 'start');
      const stopStr = extractAttr(attrStr, 'stop');
      const channel = extractAttr(attrStr, 'channel');

      if (!channel || !startStr) {
        errors.push('Programme element missing required attributes (channel, start)');
        continue;
      }

      const title = extractText(body, 'title');
      if (!title) {
        errors.push(`Programme on channel "${channel}" at ${startStr} missing title`);
        continue;
      }

      const startTime = parseXmltvTime(startStr);
      const endTime = parseXmltvTime(stopStr);

      if (!startTime) {
        errors.push(`Programme "${title}": invalid start time "${startStr}"`);
        continue;
      }

      const categories = extractAllText(body, 'category');
      const { season, episode } = parseEpisodeNum(body);

      // Extract credits
      const directors: string[] = [];
      const actors: string[] = [];
      const creditsMatch = body.match(/<credits>([\s\S]*?)<\/credits>/i);
      if (creditsMatch) {
        directors.push(...extractAllText(creditsMatch[1], 'director'));
        actors.push(...extractAllText(creditsMatch[1], 'actor'));
      }

      // Extract rating
      const ratingMatch = body.match(/<rating[^>]*>[\s\S]*?<value>([^<]*)<\/value>[\s\S]*?<\/rating>/i);
      const rating = ratingMatch ? decodeXmlEntities(ratingMatch[1].trim()) : null;

      // Extract star-rating
      const starMatch = body.match(/<star-rating[^>]*>[\s\S]*?<value>([^<]*)<\/value>[\s\S]*?<\/star-rating>/i);
      let starRating: number | null = null;
      if (starMatch) {
        const starParts = starMatch[1].trim().split('/');
        const num = parseFloat(starParts[0]);
        const denom = starParts[1] ? parseFloat(starParts[1]) : 10;
        starRating = !isNaN(num) && !isNaN(denom) && denom > 0 ? (num / denom) * 10 : null;
      }

      // Extract icon/poster
      const progIcon = body.match(/<icon\s+[^>]*src\s*=\s*"([^"]*)"[^>]*\/?>/i)?.[1] ?? null;

      // Flags
      const isNew = /<new\s*\/?>/i.test(body);
      const isPremiere = /<premiere[^>]*\/?>/i.test(body) || /<premiere[^>]*>[\s\S]*?<\/premiere>/i.test(body);
      const isLive = categories.some(c => c.toLowerCase() === 'live');
      const previouslyShown = /<previously-shown[^>]*\/?>/i.test(body);

      programmes.push({
        channel,
        title,
        subTitle: extractText(body, 'sub-title'),
        desc: extractText(body, 'desc'),
        start: startStr,
        startTime,
        endTime,
        externalId: extractAttr(attrStr, 'clumpidx')
          ?? `xmltv-${channel}-${startStr}`,
        categories,
        date: extractText(body, 'date'),
        seasonNumber: season,
        episodeNumber: episode,
        rating,
        starRating,
        icon: progIcon ? decodeXmlEntities(progIcon) : null,
        directors,
        actors,
        isNew,
        isLive,
        isPremiere,
        isRerun: previouslyShown,
        lang: extractAttr(body.match(/<title[^>]*>/)?.[0] ?? '', 'lang'),
      });
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unknown error';
      errors.push(`Failed to parse programme element: ${msg}`);
    }
  }

  return { channels, programmes };
}

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
      const { url, xml_data: xmlData } = request.body;
      const errors: string[] = [];

      if (!url && !xmlData) {
        return reply.status(400).send({ error: 'Either url or xml_data is required' });
      }

      let xml = xmlData ?? '';

      // Fetch XML from URL if provided
      if (url) {
        try {
          const response = await fetch(url, {
            headers: { 'Accept': 'application/xml, text/xml, */*' },
            signal: AbortSignal.timeout(60000),
          });

          if (!response.ok) {
            return reply.status(502).send({
              error: `Failed to fetch XMLTV from URL: HTTP ${response.status}`,
            });
          }

          xml = await response.text();
        } catch (fetchError) {
          const msg = fetchError instanceof Error ? fetchError.message : 'Unknown fetch error';
          return reply.status(502).send({
            error: `Failed to fetch XMLTV from URL: ${msg}`,
          });
        }
      }

      if (!xml.trim()) {
        return reply.status(400).send({ error: 'Empty XMLTV data' });
      }

      // Parse XMLTV XML
      const parsed = parseXmltvData(xml, errors);

      let channelsImported = 0;
      let programsImported = 0;
      let schedulesImported = 0;
      const importedPrograms: ProgramRecord[] = [];

      // Import channels
      const channelIdMap = new Map<string, string>();
      for (const ch of parsed.channels) {
        try {
          const channel = await scopedDb(request).upsertChannelByCallSign({
            source_account_id: scopedDb(request).getCurrentSourceAccountId(),
            channel_number: ch.number ?? null,
            call_sign: ch.id,
            name: ch.name,
            display_name: ch.displayName ?? null,
            logo_url: ch.icon ?? null,
            category: null,
            language: ch.lang ?? 'en',
            country: 'US',
            stream_url: null,
            stream_type: null,
            is_hd: false,
            is_4k: false,
            is_active: true,
            sort_order: 0,
            metadata: {},
          });
          channelIdMap.set(ch.id, channel.id);
          channelsImported++;
        } catch (err) {
          const msg = err instanceof Error ? err.message : 'Unknown error';
          errors.push(`Channel "${ch.id}": ${msg}`);
        }
      }

      // Import programs and schedules
      for (const prog of parsed.programmes) {
        const dbChannelId = channelIdMap.get(prog.channel);
        if (!dbChannelId) {
          errors.push(`Programme "${prog.title}": unknown channel "${prog.channel}"`);
          continue;
        }

        try {
          const externalId = prog.externalId ?? `xmltv-${prog.channel}-${prog.start}`;
          const program = await scopedDb(request).upsertProgramByExternalId({
            source_account_id: scopedDb(request).getCurrentSourceAccountId(),
            external_id: externalId,
            title: prog.title,
            episode_title: prog.subTitle ?? null,
            description: prog.desc ?? null,
            long_description: null,
            categories: prog.categories ?? [],
            genre: prog.categories?.[0] ?? null,
            season_number: prog.seasonNumber ?? null,
            episode_number: prog.episodeNumber ?? null,
            original_air_date: prog.date ? parseXmltvDate(prog.date) : null,
            year: prog.date ? parseInt(prog.date.substring(0, 4), 10) || null : null,
            duration_minutes: prog.startTime && prog.endTime
              ? Math.round((prog.endTime.getTime() - prog.startTime.getTime()) / 60000)
              : null,
            content_rating: prog.rating ?? null,
            star_rating: prog.starRating ?? null,
            poster_url: prog.icon ?? null,
            thumbnail_url: null,
            directors: prog.directors ?? [],
            actors: prog.actors ?? [],
            is_new: prog.isNew ?? false,
            is_live: prog.isLive ?? false,
            is_premiere: prog.isPremiere ?? false,
            is_finale: false,
            is_movie: prog.categories?.some(c => c.toLowerCase() === 'movie') ?? false,
            language: prog.lang ?? 'en',
            subtitles: [],
            audio_format: null,
            video_format: null,
            production_code: null,
            metadata: {},
          });

          importedPrograms.push(program);
          programsImported++;

          if (prog.startTime && prog.endTime) {
            await scopedDb(request).createSchedule({
              source_account_id: scopedDb(request).getCurrentSourceAccountId(),
              channel_id: dbChannelId,
              program_id: program.id,
              start_time: prog.startTime,
              end_time: prog.endTime,
              is_rerun: prog.isRerun ?? false,
              is_live: prog.isLive ?? false,
              metadata: {},
            });
            schedulesImported++;
          }
        } catch (err) {
          const msg = err instanceof Error ? err.message : 'Unknown error';
          errors.push(`Programme "${prog.title}": ${msg}`);
        }
      }

      // Match imported programs against existing recording rules
      let recordingsScheduled = 0;
      if (importedPrograms.length > 0) {
        try {
          recordingsScheduled = await matchSeriesRules(
            scopedDb(request),
            importedPrograms,
            fullConfig.antserverUrl || undefined
          );
        } catch (err) {
          const errMsg = err instanceof Error ? err.message : 'Unknown error';
          logger.warn('Recording rule matching failed during XMLTV import', { error: errMsg });
          errors.push(`Recording rule matching failed: ${errMsg}`);
        }
      }

      logger.info('XMLTV import completed', {
        channelsImported,
        programsImported,
        schedulesImported,
        recordingsScheduled,
        errorCount: errors.length,
      });

      return {
        channels_imported: channelsImported,
        programs_imported: programsImported,
        schedules_imported: schedulesImported,
        recordings_scheduled: recordingsScheduled,
        errors,
      };
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
      const importedPrograms: ProgramRecord[] = [];

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
          importedPrograms.push(program);

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

      // Match imported programs against existing recording rules
      let recordingsScheduled = 0;
      if (importedPrograms.length > 0) {
        try {
          recordingsScheduled = await matchSeriesRules(
            scopedDb(request),
            importedPrograms,
            fullConfig.antserverUrl || undefined
          );
        } catch (err) {
          const errMsg = err instanceof Error ? err.message : 'Unknown error';
          logger.warn('Recording rule matching failed during import', { error: errMsg });
          errors.push(`Recording rule matching failed: ${errMsg}`);
        }
      }

      return {
        channels_imported: 0,
        programs_imported: programsImported,
        schedules_imported: schedulesImported,
        recordings_scheduled: recordingsScheduled,
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
  // Recording Rules Endpoints
  // =========================================================================

  app.post<{ Body: CreateRecordingRuleRequest }>('/api/recordings/rules', async (request, reply) => {
    try {
      const body = request.body;
      const rule = await scopedDb(request).createRecordingRule({
        user_id: body.user_id,
        rule_type: body.rule_type,
        program_id: body.program_id ?? null,
        channel_id: body.channel_id ?? null,
        series_title: body.series_title ?? null,
        keyword: body.keyword ?? null,
        priority: body.priority,
        keep_count: body.keep_count ?? null,
        start_padding_minutes: body.start_padding_minutes,
        end_padding_minutes: body.end_padding_minutes,
      });

      // If it is a single rule with a program_id and channel_id, schedule it immediately
      if (rule.rule_type === 'single' && rule.program_id && rule.channel_id) {
        try {
          await scheduleRecording(
            scopedDb(request),
            rule.program_id,
            rule.id,
            rule.channel_id,
            fullConfig.antserverUrl || undefined
          );
        } catch (err) {
          const errMsg = err instanceof Error ? err.message : 'Unknown error';
          logger.warn('Auto-schedule failed for single rule', { ruleId: rule.id, error: errMsg });
        }
      }

      return reply.status(201).send(rule);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create recording rule', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Querystring: { user_id?: string } }>('/api/recordings/rules', async (request) => {
    const rules = await scopedDb(request).listRecordingRules(request.query.user_id);
    return { rules, count: rules.length };
  });

  app.put<{ Params: { id: string }; Body: Partial<CreateRecordingRuleRequest> }>(
    '/api/recordings/rules/:id',
    async (request, reply) => {
      try {
        const rule = await scopedDb(request).updateRecordingRule(
          request.params.id,
          request.body
        );
        if (!rule) {
          return reply.status(404).send({ error: 'Recording rule not found' });
        }
        return rule;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to update recording rule', { error: message });
        return reply.status(500).send({ error: message });
      }
    }
  );

  app.delete<{ Params: { id: string } }>('/api/recordings/rules/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteRecordingRule(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Recording rule not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Scheduled Recordings Endpoints
  // =========================================================================

  app.get<{ Querystring: ListScheduledRecordingsQuery }>('/api/recordings/scheduled', async (request) => {
    const recordings = await scopedDb(request).listScheduledRecordings({
      status: request.query.status,
      from: request.query.from ? new Date(request.query.from) : undefined,
      to: request.query.to ? new Date(request.query.to) : undefined,
    });
    return { recordings, count: recordings.length };
  });

  app.get<{ Querystring: ConflictCheckQuery }>('/api/recordings/conflicts', async (request, reply) => {
    try {
      if (!request.query.start || !request.query.end) {
        return reply.status(400).send({ error: 'start and end query parameters are required' });
      }

      const conflicts = await checkConflicts(
        scopedDb(request),
        new Date(request.query.start),
        new Date(request.query.end),
        request.query.channel_id
      );

      return { conflicts, count: conflicts.length };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to check conflicts', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Body: ResolveConflictRequest }>('/api/recordings/resolve-conflict', async (request, reply) => {
    try {
      const { recording_ids, strategy, keep_id } = request.body;

      if (!recording_ids || recording_ids.length === 0) {
        return reply.status(400).send({ error: 'recording_ids array is required' });
      }

      // Fetch all the recordings
      const recordings = [];
      for (const id of recording_ids) {
        const recording = await scopedDb(request).getScheduledRecording(id);
        if (recording) {
          recordings.push(recording);
        }
      }

      if (recordings.length === 0) {
        return reply.status(404).send({ error: 'No valid recordings found' });
      }

      if (strategy === 'keep' && keep_id) {
        // User explicitly chose which recording to keep
        for (const recording of recordings) {
          if (recording.id === keep_id) {
            await scopedDb(request).updateScheduledRecording(recording.id, { status: 'scheduled' });
          } else {
            await scopedDb(request).updateScheduledRecording(recording.id, { status: 'cancelled' });
          }
        }
      } else {
        // Priority-based resolution
        await resolveConflicts(scopedDb(request), recordings);
      }

      // Re-fetch to return updated state
      const updated = [];
      for (const id of recording_ids) {
        const recording = await scopedDb(request).getScheduledRecording(id);
        if (recording) {
          updated.push(recording);
        }
      }

      return { recordings: updated, count: updated.length };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to resolve conflict', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Body: RecordingTriggerEvent }>('/api/recordings/trigger', async (request, reply) => {
    try {
      // Verify HMAC signature from AntServer when secret is configured
      const webhookSecret = fullConfig.antserverWebhookSecret;
      if (webhookSecret) {
        const signature = request.headers['x-antserver-signature'] as string | undefined;
        const expectedSig = crypto
          .createHmac('sha256', webhookSecret)
          .update(JSON.stringify(request.body))
          .digest('hex');

        if (!signature || !crypto.timingSafeEqual(
          Buffer.from(signature),
          Buffer.from(expectedSig)
        )) {
          return reply.status(401).send({ error: 'Invalid webhook signature' });
        }
      }

      const { recording_id, status, antserver_job_id, error_message } = request.body;

      if (!recording_id || !status) {
        return reply.status(400).send({ error: 'recording_id and status are required' });
      }

      const recording = await scopedDb(request).getScheduledRecording(recording_id);
      if (!recording) {
        return reply.status(404).send({ error: 'Recording not found' });
      }

      // Map trigger event status to recording status
      let recordingStatus: 'recording' | 'completed' | 'failed';
      switch (status) {
        case 'started':
          recordingStatus = 'recording';
          break;
        case 'completed':
          recordingStatus = 'completed';
          break;
        case 'failed':
          recordingStatus = 'failed';
          break;
        default:
          return reply.status(400).send({ error: `Invalid status: ${status}` });
      }

      const updated = await scopedDb(request).updateScheduledRecording(recording_id, {
        status: recordingStatus,
        antserver_job_id: antserver_job_id ?? recording.antserver_job_id,
        error_message: error_message ?? null,
      });

      logger.info('Recording trigger event processed', {
        recordingId: recording_id,
        status: recordingStatus,
        antserverJobId: antserver_job_id,
      });

      return updated;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to process recording trigger', { error: message });
      return reply.status(500).send({ error: message });
    }
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
  // nTV v1 API Endpoints
  // =========================================================================

  /**
   * POST /api/v1/import - Import XMLTV data from URL or inline XML
   * Fetches, parses, and imports XMLTV XML into channels, programs, and schedules.
   */
  app.post<{ Body: { url?: string; xml_data?: string; appId?: string } }>(
    '/api/v1/import',
    async (request, reply) => {
      try {
        const { url, xml_data: xmlData } = request.body;
        const errors: string[] = [];

        if (!url && !xmlData) {
          return reply.status(400).send({ error: 'Either url or xml_data is required' });
        }

        let xml = xmlData ?? '';

        // Fetch XML from URL if provided
        if (url) {
          try {
            const response = await fetch(url, {
              headers: { 'Accept': 'application/xml, text/xml, */*' },
              signal: AbortSignal.timeout(60000),
            });

            if (!response.ok) {
              return reply.status(502).send({
                error: `Failed to fetch XMLTV from URL: HTTP ${response.status}`,
              });
            }

            xml = await response.text();
          } catch (fetchError) {
            const msg = fetchError instanceof Error ? fetchError.message : 'Unknown fetch error';
            return reply.status(502).send({
              error: `Failed to fetch XMLTV from URL: ${msg}`,
            });
          }
        }

        if (!xml.trim()) {
          return reply.status(400).send({ error: 'Empty XMLTV data' });
        }

        // Parse XMLTV XML
        const parsed = parseXmltvData(xml, errors);

        let channelsImported = 0;
        let programsImported = 0;
        const importedPrograms: ProgramRecord[] = [];

        // Import channels
        const channelIdMap = new Map<string, string>(); // xmltv_id -> db UUID
        for (const ch of parsed.channels) {
          try {
            const channel = await scopedDb(request).upsertChannelByCallSign({
              source_account_id: scopedDb(request).getCurrentSourceAccountId(),
              channel_number: ch.number ?? null,
              call_sign: ch.id,
              name: ch.name,
              display_name: ch.displayName ?? null,
              logo_url: ch.icon ?? null,
              category: null,
              language: ch.lang ?? 'en',
              country: 'US',
              stream_url: null,
              stream_type: null,
              is_hd: false,
              is_4k: false,
              is_active: true,
              sort_order: 0,
              metadata: {},
            });
            channelIdMap.set(ch.id, channel.id);
            channelsImported++;
          } catch (err) {
            const msg = err instanceof Error ? err.message : 'Unknown error';
            errors.push(`Channel "${ch.id}": ${msg}`);
          }
        }

        // Import programs and schedules
        for (const prog of parsed.programmes) {
          const dbChannelId = channelIdMap.get(prog.channel);
          if (!dbChannelId) {
            errors.push(`Programme "${prog.title}": unknown channel "${prog.channel}"`);
            continue;
          }

          try {
            // Create or upsert the program
            const externalId = prog.externalId ?? `xmltv-${prog.channel}-${prog.start}`;
            const program = await scopedDb(request).upsertProgramByExternalId({
              source_account_id: scopedDb(request).getCurrentSourceAccountId(),
              external_id: externalId,
              title: prog.title,
              episode_title: prog.subTitle ?? null,
              description: prog.desc ?? null,
              long_description: null,
              categories: prog.categories ?? [],
              genre: prog.categories?.[0] ?? null,
              season_number: prog.seasonNumber ?? null,
              episode_number: prog.episodeNumber ?? null,
              original_air_date: prog.date ? parseXmltvDate(prog.date) : null,
              year: prog.date ? parseInt(prog.date.substring(0, 4), 10) || null : null,
              duration_minutes: prog.startTime && prog.endTime
                ? Math.round((prog.endTime.getTime() - prog.startTime.getTime()) / 60000)
                : null,
              content_rating: prog.rating ?? null,
              star_rating: prog.starRating ?? null,
              poster_url: prog.icon ?? null,
              thumbnail_url: null,
              directors: prog.directors ?? [],
              actors: prog.actors ?? [],
              is_new: prog.isNew ?? false,
              is_live: prog.isLive ?? false,
              is_premiere: prog.isPremiere ?? false,
              is_finale: false,
              is_movie: prog.categories?.some(c => c.toLowerCase() === 'movie') ?? false,
              language: prog.lang ?? 'en',
              subtitles: [],
              audio_format: null,
              video_format: null,
              production_code: null,
              metadata: {},
            });

            importedPrograms.push(program);
            programsImported++;

            // Create schedule entry linking program to channel at the given time
            if (prog.startTime && prog.endTime) {
              await scopedDb(request).createSchedule({
                source_account_id: scopedDb(request).getCurrentSourceAccountId(),
                channel_id: dbChannelId,
                program_id: program.id,
                start_time: prog.startTime,
                end_time: prog.endTime,
                is_rerun: prog.isRerun ?? false,
                is_live: prog.isLive ?? false,
                metadata: {},
              });
            }
          } catch (err) {
            const msg = err instanceof Error ? err.message : 'Unknown error';
            errors.push(`Programme "${prog.title}": ${msg}`);
          }
        }

        // Match imported programs against existing recording rules
        let recordingsScheduled = 0;
        if (importedPrograms.length > 0) {
          try {
            recordingsScheduled = await matchSeriesRules(
              scopedDb(request),
              importedPrograms,
              fullConfig.antserverUrl || undefined
            );
          } catch (err) {
            const errMsg = err instanceof Error ? err.message : 'Unknown error';
            logger.warn('Recording rule matching failed during v1 import', { error: errMsg });
            errors.push(`Recording rule matching failed: ${errMsg}`);
          }
        }

        logger.info('XMLTV v1 import completed', {
          channelsImported,
          programsImported,
          recordingsScheduled,
          errorCount: errors.length,
        });

        return {
          channels_imported: channelsImported,
          programs_imported: programsImported,
          recordings_scheduled: recordingsScheduled,
          errors,
        };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('XMLTV v1 import failed', { error: message });
        return reply.status(500).send({ error: message });
      }
    }
  );

  /**
   * GET /api/v1/channels - List channels with optional groupId filter
   * Returns array of Channel objects with id, name, number, logo_url, group, hd fields.
   */
  app.get<{ Querystring: { groupId?: string; limit?: string; offset?: string } }>(
    '/api/v1/channels',
    async (request) => {
      const channels = await scopedDb(request).listChannels({
        groupId: request.query.groupId,
        isActive: true,
        limit: request.query.limit ? parseInt(request.query.limit, 10) : 200,
        offset: request.query.offset ? parseInt(request.query.offset, 10) : undefined,
      });

      return channels.map(ch => ({
        id: ch.id,
        name: ch.name,
        number: ch.channel_number,
        logo_url: ch.logo_url,
        group: ch.category,
        hd: ch.is_hd,
        call_sign: ch.call_sign,
        display_name: ch.display_name,
        is_active: ch.is_active,
      }));
    }
  );

  /**
   * GET /api/v1/schedule/:channelId - Get schedule for a channel
   * Optional ?date query param (ISO date string). Returns programs sorted by start_time.
   */
  app.get<{ Params: { channelId: string }; Querystring: { date?: string; days?: string } }>(
    '/api/v1/schedule/:channelId',
    async (request, reply) => {
      const channel = await scopedDb(request).getChannel(request.params.channelId);
      if (!channel) {
        return reply.status(404).send({ error: 'Channel not found' });
      }

      const startDate = request.query.date ? new Date(request.query.date) : new Date();
      // If a specific date is given, start from beginning of that day
      if (request.query.date) {
        startDate.setHours(0, 0, 0, 0);
      }
      const days = request.query.days ? parseInt(request.query.days, 10) : 1;

      const schedule = await scopedDb(request).getScheduleForChannel(
        request.params.channelId,
        startDate,
        days
      );

      return {
        channel: {
          id: channel.id,
          name: channel.name,
          number: channel.channel_number,
          logo_url: channel.logo_url,
        },
        programs: schedule,
        count: schedule.length,
      };
    }
  );

  /**
   * GET /api/v1/search - Search programs across all channels
   * Query params: ?query, ?limit (default 50). Full-text search on title and description.
   */
  app.get<{ Querystring: { query?: string; limit?: string } }>(
    '/api/v1/search',
    async (request, reply) => {
      const query = request.query.query;
      if (!query) {
        return reply.status(400).send({ error: 'query parameter is required' });
      }

      const limit = request.query.limit ? parseInt(request.query.limit, 10) : 50;

      const programs = await scopedDb(request).searchPrograms({
        query,
        limit,
      });

      return {
        programs,
        count: programs.length,
      };
    }
  );

  /**
   * GET /api/v1/now-playing - Get currently airing programs
   * Optional ?channelIds query param (comma-separated).
   * Returns programs where start_time <= NOW <= end_time, joined with channel info.
   */
  app.get<{ Querystring: { channelIds?: string } }>(
    '/api/v1/now-playing',
    async (request) => {
      const channelIds = request.query.channelIds
        ? request.query.channelIds.split(',').map(s => s.trim()).filter(Boolean)
        : undefined;

      const nowPlaying = await scopedDb(request).getNowPlaying(channelIds);

      return {
        now_playing: nowPlaying,
        count: nowPlaying.length,
        timestamp: new Date().toISOString(),
      };
    }
  );

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
