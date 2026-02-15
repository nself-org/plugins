/**
 * Game Metadata Plugin Server
 * HTTP server for game metadata API endpoints
 */

import express from 'express';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { GameMetadataDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateGameRequest,
  UpdateGameRequest,
  SearchGamesRequest,
  ListGamesQuery,
  LookupGameRequest,
  EnrichGameRequest,
  CreatePlatformRequest,
  CreateGenreRequest,
  CreateArtworkRequest,
  TierRequirement,
  GameCatalogRecord,
} from './types.js';

const logger = createLogger('game-metadata:server');

// =========================================================================
// Tier Definitions
// =========================================================================

const TIER_REQUIREMENTS: TierRequirement[] = [
  {
    tier: 'S',
    label: 'S-Tier (Masterpiece)',
    description: 'All-time greats. Must have IGDB rating 90+, verified ROM hash, complete artwork set, and full metadata enrichment.',
    min_rating: 90,
    max_games: null,
    features: ['igdb_verified', 'rom_hash', 'full_artwork', 'full_metadata'],
  },
  {
    tier: 'A',
    label: 'A-Tier (Excellent)',
    description: 'Highly recommended. IGDB rating 80+, verified ROM hash, cover artwork required.',
    min_rating: 80,
    max_games: null,
    features: ['igdb_verified', 'rom_hash', 'cover_artwork'],
  },
  {
    tier: 'B',
    label: 'B-Tier (Good)',
    description: 'Worth playing. IGDB rating 70+, at least one ROM hash.',
    min_rating: 70,
    max_games: null,
    features: ['rom_hash'],
  },
  {
    tier: 'C',
    label: 'C-Tier (Average)',
    description: 'Playable but unremarkable. IGDB rating 50+.',
    min_rating: 50,
    max_games: null,
    features: [],
  },
  {
    tier: 'D',
    label: 'D-Tier (Below Average)',
    description: 'For completionists only. Any rating or unrated.',
    min_rating: null,
    max_games: null,
    features: [],
  },
];

// =========================================================================
// Slug Generation
// =========================================================================

function generateSlug(title: string): string {
  return title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

// =========================================================================
// Express Middleware Adapters
// =========================================================================

function expressRateLimitMiddleware(rateLimiter: ApiRateLimiter) {
  const hook = createRateLimitHook(rateLimiter);
  return (req: express.Request, res: express.Response, next: express.NextFunction) => {
    // Adapt express request to fastify-like request for the hook
    const fakeRequest = { ip: req.ip, headers: req.headers } as never;
    const fakeReply = {
      status: (code: number) => ({
        send: (body: unknown) => { res.status(code).json(body); },
      }),
    } as never;
    const result = hook(fakeRequest, fakeReply);
    if (result instanceof Promise) {
      result.then(() => {
        if (!res.headersSent) next();
      }).catch(next);
    } else if (!res.headersSent) {
      next();
    }
  };
}

function expressAuthMiddleware(apiKey: string) {
  const hook = createAuthHook(apiKey);
  return (req: express.Request, res: express.Response, next: express.NextFunction) => {
    const fakeRequest = { headers: req.headers } as never;
    const fakeReply = {
      status: (code: number) => ({
        send: (body: unknown) => { res.status(code).json(body); },
      }),
    } as never;
    const result = hook(fakeRequest, fakeReply);
    if (result instanceof Promise) {
      result.then(() => {
        if (!res.headersSent) next();
      }).catch(next);
    } else if (!res.headersSent) {
      next();
    }
  };
}

// =========================================================================
// Server Factory
// =========================================================================

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new GameMetadataDatabase();
  await db.connect();
  await db.initializeSchema();

  // Create Express app
  const app = express();

  // Middleware
  app.use(express.json({ limit: '10mb' }));
  app.use((_req, res, next) => {
    res.header('Access-Control-Allow-Origin', '*');
    res.header('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
    res.header('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-API-Key, X-Source-Account-Id');
    if (_req.method === 'OPTIONS') {
      return res.sendStatus(204);
    }
    next();
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 200,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );

  app.use(expressRateLimitMiddleware(rateLimiter));

  if (fullConfig.security.apiKey) {
    app.use(expressAuthMiddleware(fullConfig.security.apiKey));
    logger.info('API key authentication enabled');
  }

  // Multi-app context middleware
  app.use((req: express.Request, _res: express.Response, next: express.NextFunction) => {
    const ctx = getAppContext(req as never);
    (req as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
    next();
  });

  function scopedDb(req: express.Request): GameMetadataDatabase {
    return (req as unknown as Record<string, unknown>).scopedDb as GameMetadataDatabase;
  }

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', (_req, res) => {
    res.json({ status: 'ok', plugin: 'game-metadata', timestamp: new Date().toISOString() });
  });

  app.get('/ready', async (_req, res) => {
    try {
      await db.query('SELECT 1');
      res.json({ ready: true, plugin: 'game-metadata', timestamp: new Date().toISOString() });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      res.status(503).json({
        ready: false,
        plugin: 'game-metadata',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (req, res) => {
    const stats = await scopedDb(req).getStats();
    res.json({
      alive: true,
      plugin: 'game-metadata',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        totalGames: stats.total_games,
        verifiedGames: stats.verified_games,
        totalPlatforms: stats.total_platforms,
      },
      timestamp: new Date().toISOString(),
    });
  });

  // =========================================================================
  // Game Catalog Endpoints
  // =========================================================================

  app.post('/api/games', async (req, res) => {
    try {
      const body: CreateGameRequest = req.body;
      const game = await scopedDb(req).createGame({
        source_account_id: scopedDb(req).getCurrentSourceAccountId(),
        title: body.title,
        slug: generateSlug(body.title),
        platform_id: body.platform_id ?? null,
        genre_id: body.genre_id ?? null,
        release_date: body.release_date ? new Date(body.release_date) : null,
        developer: body.developer ?? null,
        publisher: body.publisher ?? null,
        description: body.description ?? null,
        igdb_id: body.igdb_id ?? null,
        rom_hash_md5: body.rom_hash_md5 ?? null,
        rom_hash_sha1: body.rom_hash_sha1 ?? null,
        rom_hash_sha256: body.rom_hash_sha256 ?? null,
        rom_hash_crc32: body.rom_hash_crc32 ?? null,
        rom_filename: body.rom_filename ?? null,
        rom_size_bytes: body.rom_size_bytes ?? null,
        tier: body.tier ?? null,
        rating: body.rating ?? null,
        players_min: body.players_min ?? 1,
        players_max: body.players_max ?? 1,
        is_verified: false,
        metadata: {},
      });

      res.status(201).json(game);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create game', { error: message });
      res.status(500).json({ error: message });
    }
  });

  app.get('/api/games', async (req, res) => {
    const query = req.query as ListGamesQuery;
    const games = await scopedDb(req).listGames({
      platformId: query.platform_id,
      genreId: query.genre_id,
      tier: query.tier,
      isVerified: query.is_verified === 'true' ? true : query.is_verified === 'false' ? false : undefined,
      limit: query.limit ? parseInt(String(query.limit), 10) : 200,
      offset: query.offset ? parseInt(String(query.offset), 10) : undefined,
    });

    res.json({ games, count: games.length });
  });

  app.get('/api/games/:id', async (req, res) => {
    const game = await scopedDb(req).getGame(req.params.id);
    if (!game) {
      return res.status(404).json({ error: 'Game not found' });
    }

    // Include metadata and artwork
    const metadata = await scopedDb(req).getGameMetadata(game.id);
    const artwork = await scopedDb(req).listArtwork(game.id);
    const platform = game.platform_id ? await scopedDb(req).getPlatform(game.platform_id) : null;
    const genre = game.genre_id ? await scopedDb(req).getGenre(game.genre_id) : null;

    res.json({ game, metadata, artwork, platform, genre });
  });

  app.put('/api/games/:id', async (req, res) => {
    const body: UpdateGameRequest = req.body;
    const game = await scopedDb(req).updateGame(
      req.params.id,
      body as Partial<GameCatalogRecord>
    );
    if (!game) {
      return res.status(404).json({ error: 'Game not found' });
    }
    res.json(game);
  });

  app.delete('/api/games/:id', async (req, res) => {
    const deleted = await scopedDb(req).deleteGame(req.params.id);
    if (!deleted) {
      return res.status(404).json({ error: 'Game not found' });
    }
    res.json({ success: true });
  });

  // =========================================================================
  // Lookup Endpoint
  // =========================================================================

  app.post('/api/lookup', async (req, res) => {
    try {
      const body: LookupGameRequest = req.body;

      // Lookup by hash first (most precise)
      if (body.hash && body.hash_type) {
        const game = await scopedDb(req).lookupByHash(body.hash, body.hash_type);
        if (game) {
          const metadata = await scopedDb(req).getGameMetadata(game.id);
          const artwork = await scopedDb(req).listArtwork(game.id);
          return res.json({ found: true, match_type: 'hash', game, metadata, artwork });
        }
      }

      // Lookup by title
      if (body.title) {
        const games = await scopedDb(req).searchGames({
          query: body.title,
          platformId: body.platform,
          limit: 10,
        });

        if (games.length > 0) {
          return res.json({ found: true, match_type: 'title', games, count: games.length });
        }
      }

      res.json({ found: false, match_type: null, games: [], count: 0 });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Lookup failed', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Search Endpoint
  // =========================================================================

  app.post('/api/games/search', async (req, res) => {
    try {
      const body: SearchGamesRequest = req.body;
      const games = await scopedDb(req).searchGames({
        query: body.query,
        platformId: body.platform_id,
        genreId: body.genre_id,
        tier: body.tier,
        isVerified: body.is_verified,
        limit: body.limit,
      });

      res.json({ games, count: games.length });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Search failed', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Enrich Endpoint (IGDB)
  // =========================================================================

  app.post('/api/enrich', async (req, res) => {
    try {
      const body: EnrichGameRequest = req.body;
      const errors: string[] = [];

      const game = await scopedDb(req).getGame(body.game_id);
      if (!game) {
        return res.status(404).json({ error: 'Game not found' });
      }

      // Check if already enriched and not forced
      if (!body.force) {
        const existing = await scopedDb(req).getGameMetadata(game.id);
        if (existing) {
          return res.json({
            game_id: game.id,
            igdb_id: existing.igdb_id,
            metadata_updated: false,
            artwork_count: 0,
            errors: ['Already enriched. Use force=true to re-enrich.'],
          });
        }
      }

      // Check IGDB credentials
      if (!fullConfig.igdbClientId || !fullConfig.igdbClientSecret) {
        return res.status(400).json({
          error: 'IGDB credentials not configured. Set IGDB_CLIENT_ID and IGDB_CLIENT_SECRET.',
        });
      }

      // Get IGDB access token
      let accessToken: string;
      try {
        const tokenResponse = await fetch(
          `https://id.twitch.tv/oauth2/token?client_id=${fullConfig.igdbClientId}&client_secret=${fullConfig.igdbClientSecret}&grant_type=client_credentials`,
          { method: 'POST' }
        );
        const tokenData = await tokenResponse.json() as { access_token: string };
        accessToken = tokenData.access_token;
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'Unknown error';
        return res.status(502).json({ error: `Failed to get IGDB token: ${msg}` });
      }

      // Search IGDB for the game
      let igdbGame: Record<string, unknown> | null = null;
      try {
        const searchBody = game.igdb_id
          ? `fields *; where id = ${game.igdb_id};`
          : `fields *; search "${game.title.replace(/"/g, '\\"')}"; limit 1;`;

        const igdbResponse = await fetch('https://api.igdb.com/v4/games', {
          method: 'POST',
          headers: {
            'Client-ID': fullConfig.igdbClientId,
            'Authorization': `Bearer ${accessToken}`,
            'Content-Type': 'text/plain',
          },
          body: searchBody,
        });

        const igdbResults = await igdbResponse.json() as Record<string, unknown>[];
        if (igdbResults.length > 0) {
          igdbGame = igdbResults[0];
        }
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'Unknown error';
        errors.push(`IGDB search failed: ${msg}`);
      }

      if (!igdbGame) {
        return res.json({
          game_id: game.id,
          igdb_id: null,
          metadata_updated: false,
          artwork_count: 0,
          errors: ['Game not found on IGDB'],
        });
      }

      const igdbId = igdbGame.id as number;

      // Save metadata
      await scopedDb(req).upsertGameMetadata({
        source_account_id: scopedDb(req).getCurrentSourceAccountId(),
        game_id: game.id,
        source: 'igdb',
        igdb_id: igdbId,
        igdb_url: igdbGame.url as string ?? null,
        summary: igdbGame.summary as string ?? null,
        storyline: igdbGame.storyline as string ?? null,
        total_rating: igdbGame.total_rating as number ?? null,
        total_rating_count: igdbGame.total_rating_count as number ?? null,
        aggregated_rating: igdbGame.aggregated_rating as number ?? null,
        aggregated_rating_count: igdbGame.aggregated_rating_count as number ?? null,
        first_release_date: igdbGame.first_release_date
          ? new Date((igdbGame.first_release_date as number) * 1000)
          : null,
        genres: [],
        themes: [],
        keywords: [],
        game_modes: [],
        franchises: [],
        alternative_names: [],
        websites: {},
        age_ratings: {},
        involved_companies: [],
        raw_data: igdbGame,
        fetched_at: new Date(),
      });

      // Update game's igdb_id if not set
      if (!game.igdb_id) {
        await scopedDb(req).updateGame(game.id, { igdb_id: igdbId } as Partial<GameCatalogRecord>);
      }

      // Fetch artwork from IGDB
      let artworkCount = 0;
      try {
        const coverResponse = await fetch('https://api.igdb.com/v4/covers', {
          method: 'POST',
          headers: {
            'Client-ID': fullConfig.igdbClientId,
            'Authorization': `Bearer ${accessToken}`,
            'Content-Type': 'text/plain',
          },
          body: `fields *; where game = ${igdbId};`,
        });

        const covers = await coverResponse.json() as Record<string, unknown>[];
        for (const cover of covers) {
          const imageId = cover.image_id as string;
          if (imageId) {
            await scopedDb(req).createArtwork({
              source_account_id: scopedDb(req).getCurrentSourceAccountId(),
              game_id: game.id,
              artwork_type: 'cover',
              url: `https://images.igdb.com/igdb/image/upload/t_cover_big/${imageId}.jpg`,
              local_path: null,
              width: cover.width as number ?? null,
              height: cover.height as number ?? null,
              mime_type: 'image/jpeg',
              file_size_bytes: null,
              source: 'igdb',
              igdb_image_id: imageId,
              is_primary: true,
              sort_order: 0,
              metadata: {},
            });
            artworkCount++;
          }
        }

        // Fetch screenshots
        const screenshotResponse = await fetch('https://api.igdb.com/v4/screenshots', {
          method: 'POST',
          headers: {
            'Client-ID': fullConfig.igdbClientId,
            'Authorization': `Bearer ${accessToken}`,
            'Content-Type': 'text/plain',
          },
          body: `fields *; where game = ${igdbId}; limit 10;`,
        });

        const screenshots = await screenshotResponse.json() as Record<string, unknown>[];
        for (let i = 0; i < screenshots.length; i++) {
          const imageId = screenshots[i].image_id as string;
          if (imageId) {
            await scopedDb(req).createArtwork({
              source_account_id: scopedDb(req).getCurrentSourceAccountId(),
              game_id: game.id,
              artwork_type: 'screenshot',
              url: `https://images.igdb.com/igdb/image/upload/t_screenshot_big/${imageId}.jpg`,
              local_path: null,
              width: screenshots[i].width as number ?? null,
              height: screenshots[i].height as number ?? null,
              mime_type: 'image/jpeg',
              file_size_bytes: null,
              source: 'igdb',
              igdb_image_id: imageId,
              is_primary: false,
              sort_order: i + 1,
              metadata: {},
            });
            artworkCount++;
          }
        }
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'Unknown error';
        errors.push(`Artwork fetch failed: ${msg}`);
      }

      logger.info('Game enrichment completed', {
        gameId: game.id,
        igdbId,
        artworkCount,
        errorCount: errors.length,
      });

      res.json({
        game_id: game.id,
        igdb_id: igdbId,
        metadata_updated: true,
        artwork_count: artworkCount,
        errors,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Enrichment failed', { error: message });
      res.status(500).json({ error: message });
    }
  });

  // =========================================================================
  // Tier Endpoints
  // =========================================================================

  app.get('/api/tiers', (_req, res) => {
    res.json({ tiers: TIER_REQUIREMENTS });
  });

  app.get('/api/tiers/:tier', (req, res) => {
    const tier = TIER_REQUIREMENTS.find(t => t.tier.toLowerCase() === req.params.tier.toLowerCase());
    if (!tier) {
      return res.status(404).json({ error: 'Tier not found' });
    }
    res.json(tier);
  });

  // =========================================================================
  // Platform Endpoints
  // =========================================================================

  app.post('/api/platforms', async (req, res) => {
    try {
      const body: CreatePlatformRequest = req.body;
      const platform = await scopedDb(req).createPlatform({
        source_account_id: scopedDb(req).getCurrentSourceAccountId(),
        name: body.name,
        abbreviation: body.abbreviation ?? null,
        slug: generateSlug(body.name),
        igdb_id: body.igdb_id ?? null,
        generation: body.generation ?? null,
        manufacturer: body.manufacturer ?? null,
        platform_family: body.platform_family ?? null,
        category: body.category ?? null,
        release_date: body.release_date ? new Date(body.release_date) : null,
        summary: body.summary ?? null,
        is_active: true,
        sort_order: 0,
        metadata: {},
      });

      res.status(201).json(platform);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create platform', { error: message });
      res.status(500).json({ error: message });
    }
  });

  app.get('/api/platforms', async (req, res) => {
    const platforms = await scopedDb(req).listPlatforms({
      isActive: true,
      limit: req.query.limit ? parseInt(String(req.query.limit), 10) : 200,
      offset: req.query.offset ? parseInt(String(req.query.offset), 10) : undefined,
    });

    res.json({ platforms, count: platforms.length });
  });

  app.get('/api/platforms/:id', async (req, res) => {
    const platform = await scopedDb(req).getPlatform(req.params.id);
    if (!platform) {
      return res.status(404).json({ error: 'Platform not found' });
    }
    res.json(platform);
  });

  app.delete('/api/platforms/:id', async (req, res) => {
    const deleted = await scopedDb(req).deletePlatform(req.params.id);
    if (!deleted) {
      return res.status(404).json({ error: 'Platform not found' });
    }
    res.json({ success: true });
  });

  // =========================================================================
  // Genre Endpoints
  // =========================================================================

  app.post('/api/genres', async (req, res) => {
    try {
      const body: CreateGenreRequest = req.body;
      const genre = await scopedDb(req).createGenre({
        source_account_id: scopedDb(req).getCurrentSourceAccountId(),
        name: body.name,
        slug: generateSlug(body.name),
        igdb_id: body.igdb_id ?? null,
        description: body.description ?? null,
        parent_id: body.parent_id ?? null,
        is_active: true,
        sort_order: 0,
        metadata: {},
      });

      res.status(201).json(genre);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create genre', { error: message });
      res.status(500).json({ error: message });
    }
  });

  app.get('/api/genres', async (req, res) => {
    const genres = await scopedDb(req).listGenres({
      isActive: true,
      limit: req.query.limit ? parseInt(String(req.query.limit), 10) : 200,
      offset: req.query.offset ? parseInt(String(req.query.offset), 10) : undefined,
    });

    res.json({ genres, count: genres.length });
  });

  app.get('/api/genres/:id', async (req, res) => {
    const genre = await scopedDb(req).getGenre(req.params.id);
    if (!genre) {
      return res.status(404).json({ error: 'Genre not found' });
    }
    res.json(genre);
  });

  app.delete('/api/genres/:id', async (req, res) => {
    const deleted = await scopedDb(req).deleteGenre(req.params.id);
    if (!deleted) {
      return res.status(404).json({ error: 'Genre not found' });
    }
    res.json({ success: true });
  });

  // =========================================================================
  // Artwork Endpoints
  // =========================================================================

  app.post('/api/artwork', async (req, res) => {
    try {
      const body: CreateArtworkRequest = req.body;
      const artwork = await scopedDb(req).createArtwork({
        source_account_id: scopedDb(req).getCurrentSourceAccountId(),
        game_id: body.game_id,
        artwork_type: body.artwork_type,
        url: body.url ?? null,
        local_path: body.local_path ?? null,
        width: body.width ?? null,
        height: body.height ?? null,
        mime_type: body.mime_type ?? null,
        file_size_bytes: body.file_size_bytes ?? null,
        source: body.source ?? 'manual',
        igdb_image_id: body.igdb_image_id ?? null,
        is_primary: body.is_primary ?? false,
        sort_order: 0,
        metadata: {},
      });

      res.status(201).json(artwork);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create artwork', { error: message });
      res.status(500).json({ error: message });
    }
  });

  app.get('/api/artwork/:gameId', async (req, res) => {
    const artworkType = req.query.type as string | undefined;
    const artwork = await scopedDb(req).listArtwork(req.params.gameId, artworkType);
    res.json({ artwork, count: artwork.length });
  });

  app.delete('/api/artwork/:id', async (req, res) => {
    const deleted = await scopedDb(req).deleteArtwork(req.params.id);
    if (!deleted) {
      return res.status(404).json({ error: 'Artwork not found' });
    }
    res.json({ success: true });
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', async (req, res) => {
    const stats = await scopedDb(req).getStats();
    res.json({
      plugin: 'game-metadata',
      version: '1.0.0',
      stats,
      timestamp: new Date().toISOString(),
    });
  });

  // =========================================================================
  // Server Lifecycle
  // =========================================================================

  let httpServer: ReturnType<typeof app.listen> | null = null;

  const server = {
    async start() {
      try {
        httpServer = app.listen(fullConfig.port, fullConfig.host, () => {
          logger.info(`Game Metadata server listening on ${fullConfig.host}:${fullConfig.port}`);
        });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Server failed to start', { error: message });
        throw error;
      }
    },

    async stop() {
      if (httpServer) {
        await new Promise<void>((resolve, reject) => {
          httpServer!.close((err) => {
            if (err) reject(err);
            else resolve();
          });
        });
      }
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
