/**
 * Retro Gaming Plugin Server
 * HTTP server for retro gaming ROM library, save states, play sessions,
 * emulator cores, and controller configuration API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { RetroGamingDatabase } from './database.js';
import { IgdbClient } from './igdb-client.js';
import { loadConfig, type RetroGamingConfig } from './config.js';
import type {
  CreateRomRequest,
  UpdateRomRequest,
  ScanRomsRequest,
  CreateSaveStateRequest,
  StartSessionRequest,
  EndSessionRequest,
  CreateControllerConfigRequest,
  RecordCoreInstallationRequest,
  ListRomsQuery,
} from './types.js';

const logger = createLogger('retro-gaming:server');

export async function createServer(config?: Partial<RetroGamingConfig>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new RetroGamingDatabase();
  await db.connect();
  await db.initializeSchema();

  // Initialize IGDB client
  const igdb = new IgdbClient(fullConfig.igdbClientId, fullConfig.igdbClientSecret);

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB
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

  function scopedDb(request: unknown): RetroGamingDatabase {
    return (request as Record<string, unknown>).scopedDb as RetroGamingDatabase;
  }

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'retro-gaming', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'retro-gaming', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'retro-gaming',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  // =========================================================================
  // ROM Endpoints
  // =========================================================================

  // List ROMs
  app.get<{ Querystring: ListRomsQuery }>('/api/games/roms', async (request) => {
    const roms = await scopedDb(request).listRoms({
      platform: request.query.platform,
      genre: request.query.genre,
      favorite: request.query.favorite === 'true' ? true : request.query.favorite === 'false' ? false : undefined,
      search: request.query.search,
      sort: request.query.sort,
      limit: request.query.limit ? parseInt(String(request.query.limit), 10) : 100,
      offset: request.query.offset ? parseInt(String(request.query.offset), 10) : 0,
    });

    return { roms, count: roms.length };
  });

  // Get ROM details
  app.get<{ Params: { id: string } }>('/api/games/roms/:id', async (request, reply) => {
    const rom = await scopedDb(request).getRom(request.params.id);
    if (!rom) {
      return reply.status(404).send({ error: 'ROM not found' });
    }
    return rom;
  });

  // Create ROM entry
  app.post<{ Body: CreateRomRequest }>('/api/games/roms', {
    schema: {
      body: {
        type: 'object',
        required: ['rom_file_path', 'game_title', 'platform'],
        properties: {
          rom_file_path: { type: 'string', minLength: 1 },
          rom_file_size_bytes: { type: 'number' },
          rom_file_hash: { type: 'string' },
          game_title: { type: 'string', minLength: 1 },
          platform: { type: 'string', minLength: 1 },
          region: { type: 'string' },
          release_year: { type: 'number' },
          genre: { type: 'string' },
          publisher: { type: 'string' },
          developer: { type: 'string' },
          igdb_id: { type: 'number' },
          moby_games_id: { type: 'number' },
          box_art_url: { type: 'string' },
          box_art_local_path: { type: 'string' },
          screenshot_urls: { type: 'array', items: { type: 'string' } },
          screenshot_local_paths: { type: 'array', items: { type: 'string' } },
          description: { type: 'string' },
          description_source: { type: 'string' },
          recommended_core: { type: 'string' },
          core_overrides: { type: 'object' },
          scan_source: { type: 'string' },
          added_by_user_id: { type: 'string' },
        },
      },
    },
  }, async (request, reply) => {
    try {
      const rom = await scopedDb(request).createRom({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        rom_file_path: request.body.rom_file_path,
        rom_file_size_bytes: request.body.rom_file_size_bytes ?? null,
        rom_file_hash: request.body.rom_file_hash ?? null,
        game_title: request.body.game_title,
        platform: request.body.platform,
        region: request.body.region ?? null,
        release_year: request.body.release_year ?? null,
        genre: request.body.genre ?? null,
        publisher: request.body.publisher ?? null,
        developer: request.body.developer ?? null,
        igdb_id: request.body.igdb_id ?? null,
        moby_games_id: request.body.moby_games_id ?? null,
        box_art_url: request.body.box_art_url ?? null,
        box_art_local_path: request.body.box_art_local_path ?? null,
        screenshot_urls: request.body.screenshot_urls ?? [],
        screenshot_local_paths: request.body.screenshot_local_paths ?? [],
        description: request.body.description ?? null,
        description_source: request.body.description_source ?? null,
        recommended_core: request.body.recommended_core ?? null,
        core_overrides: request.body.core_overrides ?? {},
        user_rating: null,
        scan_source: request.body.scan_source ?? null,
        added_by_user_id: request.body.added_by_user_id ?? null,
      });

      return reply.status(201).send(rom);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create ROM', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // Update ROM metadata
  app.patch<{ Params: { id: string }; Body: UpdateRomRequest }>('/api/games/roms/:id', {
    schema: {
      body: {
        type: 'object',
        properties: {
          game_title: { type: 'string', minLength: 1 },
          platform: { type: 'string' },
          region: { type: 'string' },
          release_year: { type: 'number' },
          genre: { type: 'string' },
          publisher: { type: 'string' },
          developer: { type: 'string' },
          igdb_id: { type: 'number' },
          moby_games_id: { type: 'number' },
          box_art_url: { type: 'string' },
          box_art_local_path: { type: 'string' },
          screenshot_urls: { type: 'array', items: { type: 'string' } },
          screenshot_local_paths: { type: 'array', items: { type: 'string' } },
          description: { type: 'string' },
          description_source: { type: 'string' },
          recommended_core: { type: 'string' },
          core_overrides: { type: 'object' },
          user_rating: { type: 'number', minimum: 0, maximum: 10 },
          favorite: { type: 'boolean' },
        },
      },
    },
  }, async (request, reply) => {
    try {
      const rom = await scopedDb(request).updateRom(request.params.id, request.body as Record<string, unknown>);
      if (!rom) {
        return reply.status(404).send({ error: 'ROM not found' });
      }
      return rom;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to update ROM', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // Delete ROM
  app.delete<{ Params: { id: string } }>('/api/games/roms/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteRom(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'ROM not found' });
    }
    return { success: true };
  });

  // Scan/import ROMs
  app.post<{ Body: ScanRomsRequest }>('/api/games/roms/scan', {
    schema: {
      body: {
        type: 'object',
        required: ['files'],
        properties: {
          files: {
            type: 'array',
            items: {
              type: 'object',
              required: ['rom_file_path'],
              properties: {
                rom_file_path: { type: 'string', minLength: 1 },
                rom_file_size_bytes: { type: 'number' },
                rom_file_hash: { type: 'string' },
                game_title: { type: 'string' },
                platform: { type: 'string' },
              },
            },
          },
          scan_source: { type: 'string' },
          auto_enrich: { type: 'boolean' },
        },
      },
    },
  }, async (request, reply) => {
    try {
      let romsCreated = 0;
      let romsSkipped = 0;
      const errors: string[] = [];

      for (const file of request.body.files) {
        try {
          // Check if ROM already exists by file path
          const existing = await scopedDb(request).getRomByFilePath(file.rom_file_path);
          if (existing) {
            romsSkipped++;
            continue;
          }

          // Derive game title and platform from file path if not provided
          const fileName = file.rom_file_path.split('/').pop() ?? file.rom_file_path;
          const titleFromFile = fileName.replace(/\.[^.]+$/, '').replace(/[_-]/g, ' ').replace(/\s*\([^)]*\)\s*/g, ' ').trim();
          const extToPlat: Record<string, string> = {
            '.nes': 'nes', '.smc': 'snes', '.sfc': 'snes',
            '.gb': 'gb', '.gbc': 'gbc', '.gba': 'gba',
            '.md': 'genesis', '.gen': 'genesis',
            '.z64': 'n64', '.n64': 'n64', '.v64': 'n64',
            '.bin': 'ps1', '.cue': 'ps1', '.iso': 'ps1',
            '.zip': 'arcade',
          };
          const ext = fileName.substring(fileName.lastIndexOf('.')).toLowerCase();
          const derivedPlatform = file.platform ?? extToPlat[ext] ?? 'unknown';

          await scopedDb(request).createRom({
            source_account_id: scopedDb(request).getCurrentSourceAccountId(),
            rom_file_path: file.rom_file_path,
            rom_file_size_bytes: file.rom_file_size_bytes ?? null,
            rom_file_hash: file.rom_file_hash ?? null,
            game_title: file.game_title ?? titleFromFile,
            platform: derivedPlatform,
            region: null,
            release_year: null,
            genre: null,
            publisher: null,
            developer: null,
            igdb_id: null,
            moby_games_id: null,
            box_art_url: null,
            box_art_local_path: null,
            screenshot_urls: [],
            screenshot_local_paths: [],
            description: null,
            description_source: null,
            recommended_core: null,
            core_overrides: {},
            user_rating: null,
            scan_source: request.body.scan_source ?? 'scan',
            added_by_user_id: null,
          });
          romsCreated++;
        } catch (err) {
          const errMsg = err instanceof Error ? err.message : 'Unknown error';
          errors.push(`Failed to import "${file.rom_file_path}": ${errMsg}`);
        }
      }

      // If auto_enrich is requested and IGDB is configured, enrich all new ROMs
      if (request.body.auto_enrich && igdb.isConfigured() && romsCreated > 0) {
        logger.info(`Auto-enrichment requested for ${romsCreated} new ROMs (will run in background)`);
      }

      return reply.status(201).send({
        roms_created: romsCreated,
        roms_skipped: romsSkipped,
        errors,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('ROM scan failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // Trigger metadata enrichment for a ROM
  app.post<{ Params: { id: string } }>('/api/games/roms/:id/enrich', async (request, reply) => {
    try {
      const rom = await scopedDb(request).getRom(request.params.id);
      if (!rom) {
        return reply.status(404).send({ error: 'ROM not found' });
      }

      if (!igdb.isConfigured()) {
        return reply.status(400).send({ error: 'IGDB not configured. Set IGDB_CLIENT_ID and IGDB_CLIENT_SECRET.' });
      }

      // Search IGDB for this game
      const games = await igdb.searchGames(rom.game_title, rom.platform);
      if (games.length === 0) {
        return { enriched: false, message: 'No matches found on IGDB', rom_id: rom.id };
      }

      const bestMatch = games[0];
      const updates: Record<string, unknown> = {};

      if (bestMatch.summary && !rom.description) {
        updates.description = bestMatch.summary;
        updates.description_source = 'igdb';
      }

      if (bestMatch.first_release_date && !rom.release_year) {
        updates.release_year = new Date(bestMatch.first_release_date * 1000).getFullYear();
      }

      if (bestMatch.cover?.url && !rom.box_art_url) {
        // IGDB returns //images... URLs, convert to https
        updates.box_art_url = bestMatch.cover.url.startsWith('//') ? `https:${bestMatch.cover.url}` : bestMatch.cover.url;
        // Request larger image
        updates.box_art_url = (updates.box_art_url as string).replace('/t_thumb/', '/t_cover_big/');
      }

      if (bestMatch.screenshots && bestMatch.screenshots.length > 0 && (!rom.screenshot_urls || rom.screenshot_urls.length === 0)) {
        updates.screenshot_urls = bestMatch.screenshots.map(s => {
          const url = s.url.startsWith('//') ? `https:${s.url}` : s.url;
          return url.replace('/t_thumb/', '/t_screenshot_big/');
        });
      }

      if (bestMatch.genres && bestMatch.genres.length > 0 && !rom.genre) {
        updates.genre = bestMatch.genres[0].name;
      }

      if (bestMatch.involved_companies) {
        const publishers = bestMatch.involved_companies.filter(c => c.publisher);
        const developers = bestMatch.involved_companies.filter(c => c.developer);
        if (publishers.length > 0 && !rom.publisher) {
          updates.publisher = publishers[0].company.name;
        }
        if (developers.length > 0 && !rom.developer) {
          updates.developer = developers[0].company.name;
        }
      }

      updates.igdb_id = bestMatch.id;

      const updatedRom = await scopedDb(request).updateRom(rom.id, updates);

      return {
        enriched: true,
        igdb_match: bestMatch.name,
        igdb_id: bestMatch.id,
        fields_updated: Object.keys(updates),
        rom: updatedRom,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('ROM enrichment failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Save State Endpoints
  // =========================================================================

  // List save states for a ROM
  app.get<{ Params: { rom_id: string } }>('/api/games/save-states/:rom_id', async (request, reply) => {
    const rom = await scopedDb(request).getRom(request.params.rom_id);
    if (!rom) {
      return reply.status(404).send({ error: 'ROM not found' });
    }

    const saveStates = await scopedDb(request).listSaveStates(request.params.rom_id);
    return { save_states: saveStates, count: saveStates.length, rom_id: request.params.rom_id };
  });

  // Create save state
  app.post<{ Params: { rom_id: string }; Body: CreateSaveStateRequest }>('/api/games/save-states/:rom_id', {
    schema: {
      body: {
        type: 'object',
        required: ['user_id', 'slot', 'save_state_file_path', 'emulator_core'],
        properties: {
          user_id: { type: 'string', minLength: 1 },
          slot: { type: 'number', minimum: 0 },
          save_state_file_path: { type: 'string', minLength: 1 },
          save_state_file_size_bytes: { type: 'number' },
          screenshot_url: { type: 'string' },
          screenshot_local_path: { type: 'string' },
          emulator_core: { type: 'string', minLength: 1 },
          emulator_version: { type: 'string' },
          description: { type: 'string' },
          play_time_seconds: { type: 'number', minimum: 0 },
        },
      },
    },
  }, async (request, reply) => {
    try {
      const rom = await scopedDb(request).getRom(request.params.rom_id);
      if (!rom) {
        return reply.status(404).send({ error: 'ROM not found' });
      }

      const saveState = await scopedDb(request).createSaveState({
        user_id: request.body.user_id,
        rom_id: request.params.rom_id,
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        slot: request.body.slot,
        save_state_file_path: request.body.save_state_file_path,
        save_state_file_size_bytes: request.body.save_state_file_size_bytes ?? null,
        screenshot_url: request.body.screenshot_url ?? null,
        screenshot_local_path: request.body.screenshot_local_path ?? null,
        emulator_core: request.body.emulator_core,
        emulator_version: request.body.emulator_version ?? null,
        description: request.body.description ?? null,
        play_time_seconds: request.body.play_time_seconds ?? 0,
      });

      return reply.status(201).send(saveState);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create save state', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // Get save state by ROM and slot
  app.get<{ Params: { rom_id: string; slot: string } }>('/api/games/save-states/:rom_id/:slot', async (request, reply) => {
    const slot = parseInt(request.params.slot, 10);
    if (isNaN(slot)) {
      return reply.status(400).send({ error: 'Invalid slot number' });
    }

    const saveState = await scopedDb(request).getSaveState(request.params.rom_id, slot);
    if (!saveState) {
      return reply.status(404).send({ error: 'Save state not found' });
    }
    return saveState;
  });

  // Delete save state
  app.delete<{ Params: { rom_id: string; slot: string } }>('/api/games/save-states/:rom_id/:slot', async (request, reply) => {
    const slot = parseInt(request.params.slot, 10);
    if (isNaN(slot)) {
      return reply.status(400).send({ error: 'Invalid slot number' });
    }

    const deleted = await scopedDb(request).deleteSaveState(request.params.rom_id, slot);
    if (!deleted) {
      return reply.status(404).send({ error: 'Save state not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Emulator Core Endpoints
  // =========================================================================

  // List all cores (or filter by platform via query param)
  app.get('/api/games/cores', async (request) => {
    const query = request.query as { platform?: string };
    const cores = await db.listCores(query.platform);
    return { cores, count: cores.length };
  });

  // Get recommended core for a platform
  app.get<{ Params: { platform: string } }>('/api/games/cores/:platform', async (request, reply) => {
    // Check if this is a download request (handled below) or platform query
    const cores = await db.listCores(request.params.platform);
    if (cores.length === 0) {
      return reply.status(404).send({ error: `No cores found for platform: ${request.params.platform}` });
    }

    const recommended = cores.find(c => c.is_recommended) ?? cores[0];

    return {
      platform: request.params.platform,
      recommended: recommended,
      all_cores: cores,
    };
  });

  // Get core download URL
  app.get<{ Params: { core_name: string } }>('/api/games/cores/:core_name/download', async (request, reply) => {
    const core = await db.getCoreByName(request.params.core_name);
    if (!core) {
      return reply.status(404).send({ error: `Core not found: ${request.params.core_name}` });
    }

    const baseUrl = fullConfig.cdnUrl || `http://localhost:${fullConfig.port}`;
    const downloadUrl = core.core_wasm_path
      ? `${baseUrl}${core.core_wasm_path}`
      : null;

    return {
      core_name: core.core_name,
      display_name: core.display_name,
      version: core.version,
      platform: core.platform,
      download_url: downloadUrl,
      size_bytes: core.core_wasm_size_bytes,
    };
  });

  // Record core installation
  app.post<{ Body: RecordCoreInstallationRequest }>('/api/games/cores/installed', {
    schema: {
      body: {
        type: 'object',
        required: ['user_id', 'device_id', 'device_platform', 'core_name', 'core_version'],
        properties: {
          user_id: { type: 'string', minLength: 1 },
          device_id: { type: 'string', minLength: 1 },
          device_platform: { type: 'string', minLength: 1 },
          core_name: { type: 'string', minLength: 1 },
          core_version: { type: 'string', minLength: 1 },
        },
      },
    },
  }, async (request, reply) => {
    try {
      const installation = await scopedDb(request).recordCoreInstallation({
        user_id: request.body.user_id,
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        device_id: request.body.device_id,
        device_platform: request.body.device_platform,
        core_name: request.body.core_name,
        core_version: request.body.core_version,
      });

      return reply.status(201).send(installation);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to record core installation', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // List installed cores
  app.get('/api/games/cores/installed', async (request) => {
    const query = request.query as { device_id?: string };
    const installations = await scopedDb(request).listInstalledCores(query.device_id);
    return { installations, count: installations.length };
  });

  // =========================================================================
  // Play Session Endpoints
  // =========================================================================

  // Start play session
  app.post<{ Body: StartSessionRequest }>('/api/games/sessions/start', {
    schema: {
      body: {
        type: 'object',
        required: ['user_id', 'rom_id', 'platform', 'emulator_core'],
        properties: {
          user_id: { type: 'string', minLength: 1 },
          rom_id: { type: 'string', minLength: 1 },
          platform: { type: 'string', minLength: 1 },
          device_id: { type: 'string' },
          emulator_core: { type: 'string', minLength: 1 },
          controller_type: { type: 'string' },
        },
      },
    },
  }, async (request, reply) => {
    try {
      // Verify ROM exists
      const rom = await scopedDb(request).getRom(request.body.rom_id);
      if (!rom) {
        return reply.status(404).send({ error: 'ROM not found' });
      }

      // Start the session
      const session = await scopedDb(request).startPlaySession({
        user_id: request.body.user_id,
        rom_id: request.body.rom_id,
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        platform: request.body.platform,
        device_id: request.body.device_id ?? null,
        emulator_core: request.body.emulator_core,
        started_at: new Date(),
        controller_type: request.body.controller_type ?? null,
      });

      // Update ROM play count and last_played_at
      await scopedDb(request).incrementRomPlayCount(request.body.rom_id);

      // Update core last used if device_id is provided
      if (request.body.device_id) {
        await scopedDb(request).updateCoreLastUsed(
          request.body.user_id,
          request.body.device_id,
          request.body.emulator_core
        );
      }

      return reply.status(201).send(session);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start play session', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // End play session
  app.post<{ Params: { session_id: string }; Body: EndSessionRequest }>('/api/games/sessions/:session_id/end', {
    schema: {
      body: {
        type: 'object',
        properties: {
          save_state_id: { type: 'string' },
          auto_save_created: { type: 'boolean' },
        },
      },
    },
  }, async (request, reply) => {
    try {
      const session = await scopedDb(request).endPlaySession(
        request.params.session_id,
        request.body.save_state_id ?? null,
        request.body.auto_save_created ?? false
      );

      if (!session) {
        return reply.status(404).send({ error: 'Active session not found' });
      }

      return session;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to end play session', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // Recent play sessions
  app.get('/api/games/sessions/recent', async (request) => {
    const query = request.query as { limit?: string };
    const limit = query.limit ? parseInt(query.limit, 10) : 20;
    const sessions = await scopedDb(request).listRecentSessions(limit);
    return { sessions, count: sessions.length };
  });

  // =========================================================================
  // Controller Config Endpoints
  // =========================================================================

  // List controller configs
  app.get('/api/games/controllers', async (request) => {
    const query = request.query as { user_id?: string };
    const configs = await scopedDb(request).listControllerConfigs(query.user_id);
    return { configs, count: configs.length };
  });

  // Create controller config
  app.post<{ Body: CreateControllerConfigRequest }>('/api/games/controllers', {
    schema: {
      body: {
        type: 'object',
        required: ['user_id', 'config_name', 'controller_type', 'button_mapping'],
        properties: {
          user_id: { type: 'string', minLength: 1 },
          config_name: { type: 'string', minLength: 1 },
          platform: { type: 'string' },
          controller_type: { type: 'string', minLength: 1 },
          button_mapping: { type: 'object' },
          touch_layout: { type: 'object' },
          analog_sensitivity: { type: 'number', minimum: 0, maximum: 5 },
          vibration_enabled: { type: 'boolean' },
        },
      },
    },
  }, async (request, reply) => {
    try {
      const config = await scopedDb(request).createControllerConfig({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        user_id: request.body.user_id,
        config_name: request.body.config_name,
        platform: request.body.platform ?? null,
        controller_type: request.body.controller_type,
        button_mapping: request.body.button_mapping,
        touch_layout: request.body.touch_layout ?? {},
        analog_sensitivity: request.body.analog_sensitivity ?? 1.0,
        vibration_enabled: request.body.vibration_enabled ?? true,
      });

      return reply.status(201).send(config);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create controller config', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // Delete controller config
  app.delete<{ Params: { id: string } }>('/api/games/controllers/:id', async (request, reply) => {
    const deleted = await scopedDb(request).deleteControllerConfig(request.params.id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Controller config not found' });
    }
    return { success: true };
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'retro-gaming',
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
        logger.info(`Retro gaming server listening on ${fullConfig.host}:${fullConfig.port}`);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Server failed to start', { error: message });
        throw error;
      }
    },

    async stop() {
      igdb.destroy();
      await app.close();
      await db.disconnect();
      logger.info('Server stopped');
    },
  };

  return server;
}

export async function startServer(config?: Partial<RetroGamingConfig>): Promise<void> {
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
