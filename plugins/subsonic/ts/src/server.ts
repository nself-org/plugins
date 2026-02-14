/**
 * Subsonic Plugin Server
 * Fastify HTTP server implementing the Subsonic REST API for music client compatibility.
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createRateLimitHook } from '@nself/plugin-utils';
import { SubsonicDatabase } from './database.js';
import { loadConfig, type SubsonicConfig } from './config.js';
import { authenticate, validateApiVersion } from './auth.js';
import {
  okResponse,
  errorResponse,
  artistToSubsonic,
  albumToSubsonic,
  songToSubsonic,
  playlistToSubsonic,
  musicFolderToSubsonic,
  buildArtistIndex,
} from './response.js';
import { SUBSONIC_ERROR_CODES } from './types.js';
import type { SubsonicQueryParams, AlbumListParams } from './types.js';
import { streamAudio, resolveCoverArtPath, serveCoverArt } from './streaming.js';
import { scanLibrary } from './library.js';

const logger = createLogger('subsonic:server');

type SubsonicRequest = {
  query: SubsonicQueryParams & Record<string, string | undefined>;
  headers: Record<string, string | string[] | undefined>;
  ip: string;
};

export async function createServer(config?: Partial<SubsonicConfig>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new SubsonicDatabase(fullConfig.sourceAccountId);
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 1024 * 1024,
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Rate limiting
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 200,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  // ─── Authentication Hook for /rest/* ───────────────────────────────────

  app.addHook('preHandler', async (request, reply) => {
    const url = (request as unknown as { url: string }).url;

    // Skip auth for non-Subsonic endpoints
    if (!url.startsWith('/rest/')) return;

    const params = (request as unknown as SubsonicRequest).query;

    // Validate API version
    const versionError = validateApiVersion(params.v);
    if (versionError) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.SERVER_VERSION_MISMATCH, versionError));
    }

    // Authenticate
    const authResult = authenticate(params, fullConfig.adminPassword);
    if (!authResult.authenticated) {
      return reply.send(
        errorResponse(
          authResult.errorCode ?? SUBSONIC_ERROR_CODES.WRONG_CREDENTIALS,
          authResult.error ?? 'Authentication failed'
        )
      );
    }

    // Store username for later use
    (request as unknown as Record<string, unknown>).subsonicUser = authResult.username;
  });

  // ─── Helper to extract authenticated username ──────────────────────────

  function getUser(request: unknown): string {
    return ((request as Record<string, unknown>).subsonicUser as string) ?? 'admin';
  }

  // ─── Health Check (non-Subsonic) ───────────────────────────────────────

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'subsonic', timestamp: new Date().toISOString() };
  });

  // ─── System Endpoints ──────────────────────────────────────────────────

  app.get('/rest/ping.view', async () => {
    return okResponse();
  });

  app.get('/rest/getLicense.view', async () => {
    return okResponse({
      license: {
        valid: true,
        email: 'admin@nself.org',
        licenseExpires: '2099-12-31T23:59:59.000Z',
      },
    });
  });

  // ─── Browsing Endpoints ────────────────────────────────────────────────

  app.get('/rest/getMusicFolders.view', async () => {
    const folders = await db.getMusicFolders();
    return okResponse({
      musicFolders: {
        musicFolder: folders.map(musicFolderToSubsonic),
      },
    });
  });

  app.get('/rest/getIndexes.view', async (request) => {
    // musicFolderId parameter accepted but currently all folders are returned
    const artists = await db.listArtists();
    const indices = buildArtistIndex(artists);

    return okResponse({
      indexes: {
        lastModified: Date.now(),
        ignoredArticles: 'The An A',
        index: indices,
      },
    });
  });

  app.get('/rest/getArtists.view', async () => {
    const artists = await db.listArtists();
    const indices = buildArtistIndex(artists);

    return okResponse({
      artists: {
        ignoredArticles: 'The An A',
        index: indices,
      },
    });
  });

  app.get('/rest/getArtist.view', async (request, reply) => {
    const params = (request as unknown as SubsonicRequest).query;
    const id = params.id;

    if (!id) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.MISSING_PARAMETER, 'Required parameter is missing: id'));
    }

    const artist = await db.getArtistById(id);
    if (!artist) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.NOT_FOUND, 'Artist not found'));
    }

    const albums = await db.getAlbumsByArtist(id);
    const subsonicArtist = artistToSubsonic(artist);

    return okResponse({
      artist: {
        ...subsonicArtist,
        album: albums.map(a => albumToSubsonic(a, artist.name)),
      },
    });
  });

  app.get('/rest/getMusicDirectory.view', async (request, reply) => {
    const params = (request as unknown as SubsonicRequest).query;
    const id = params.id;

    if (!id) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.MISSING_PARAMETER, 'Required parameter is missing: id'));
    }

    try {
      const dirContents = await db.getMusicDirectoryContents(id);

      if (dirContents.type === 'artist') {
        const artist = dirContents.record;
        const albums = dirContents.songs as unknown as import('./types.js').AlbumRecord[];
        const children = albums.map(album => ({
          id: album.id,
          parent: artist.id,
          isDir: true,
          title: album.title,
          artist: artist.name,
          coverArt: album.cover_art_path ? `al-${album.id}` : undefined,
          year: album.year ?? undefined,
          genre: album.genre ?? undefined,
          type: 'music' as const,
          isVideo: false,
        }));

        return okResponse({
          directory: {
            id: artist.id,
            name: artist.name,
            child: children,
          },
        });
      }

      // Album directory
      const album = dirContents.record as import('./types.js').AlbumRecord;
      const songs = dirContents.songs;

      // Get artist name for songs
      let artistName: string | undefined;
      if (album.artist_id) {
        const artist = await db.getArtistById(album.artist_id);
        artistName = artist?.name;
      }

      return okResponse({
        directory: {
          id: album.id,
          parent: album.artist_id ?? undefined,
          name: album.title,
          child: songs.map(s => songToSubsonic(s, artistName, album.title)),
        },
      });
    } catch {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.NOT_FOUND, 'Directory not found'));
    }
  });

  // ─── Album Endpoints ──────────────────────────────────────────────────

  app.get('/rest/getAlbumList2.view', async (request) => {
    const params = (request as unknown as SubsonicRequest).query as unknown as AlbumListParams & SubsonicQueryParams;
    const type = params.type ?? 'alphabeticalByName';
    const size = Math.min(Math.max(parseInt(String(params.size ?? '20'), 10), 1), 500);
    const offset = Math.max(parseInt(String(params.offset ?? '0'), 10), 0);

    const albums = await db.listAlbums(
      type, size, offset,
      params.fromYear ? parseInt(String(params.fromYear), 10) : undefined,
      params.toYear ? parseInt(String(params.toYear), 10) : undefined,
      params.genre ? String(params.genre) : undefined
    );

    // Resolve artist names
    const albumsWithArtist = await Promise.all(
      albums.map(async (album) => {
        let artistName: string | undefined;
        if (album.artist_id) {
          const artist = await db.getArtistById(album.artist_id);
          artistName = artist?.name;
        }
        return albumToSubsonic(album, artistName);
      })
    );

    return okResponse({
      albumList2: {
        album: albumsWithArtist,
      },
    });
  });

  app.get('/rest/getAlbum.view', async (request, reply) => {
    const params = (request as unknown as SubsonicRequest).query;
    const id = params.id;

    if (!id) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.MISSING_PARAMETER, 'Required parameter is missing: id'));
    }

    const album = await db.getAlbumById(id);
    if (!album) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.NOT_FOUND, 'Album not found'));
    }

    let artistName: string | undefined;
    if (album.artist_id) {
      const artist = await db.getArtistById(album.artist_id);
      artistName = artist?.name;
    }

    const songs = await db.getSongsByAlbum(id);
    const subsonicAlbum = albumToSubsonic(album, artistName);

    return okResponse({
      album: {
        ...subsonicAlbum,
        song: songs.map(s => songToSubsonic(s, artistName, album.title)),
      },
    });
  });

  // ─── Song Endpoints ───────────────────────────────────────────────────

  app.get('/rest/getSong.view', async (request, reply) => {
    const params = (request as unknown as SubsonicRequest).query;
    const id = params.id;

    if (!id) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.MISSING_PARAMETER, 'Required parameter is missing: id'));
    }

    const song = await db.getSongById(id);
    if (!song) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.NOT_FOUND, 'Song not found'));
    }

    let artistName: string | undefined;
    let albumTitle: string | undefined;
    if (song.artist_id) {
      const artist = await db.getArtistById(song.artist_id);
      artistName = artist?.name;
    }
    if (song.album_id) {
      const album = await db.getAlbumById(song.album_id);
      albumTitle = album?.title;
    }

    return okResponse({
      song: songToSubsonic(song, artistName, albumTitle),
    });
  });

  app.get('/rest/getRandomSongs.view', async (request) => {
    const params = (request as unknown as SubsonicRequest).query;
    const size = Math.min(Math.max(parseInt(params.size ?? '10', 10), 1), 500);
    const genre = params.genre;
    const fromYear = params.fromYear ? parseInt(params.fromYear, 10) : undefined;
    const toYear = params.toYear ? parseInt(params.toYear, 10) : undefined;

    const songs = await db.getRandomSongs(size, genre, fromYear, toYear);

    const songsWithMeta = await Promise.all(
      songs.map(async (song) => {
        let artistName: string | undefined;
        let albumTitle: string | undefined;
        if (song.artist_id) {
          const artist = await db.getArtistById(song.artist_id);
          artistName = artist?.name;
        }
        if (song.album_id) {
          const album = await db.getAlbumById(song.album_id);
          albumTitle = album?.title;
        }
        return songToSubsonic(song, artistName, albumTitle);
      })
    );

    return okResponse({
      randomSongs: {
        song: songsWithMeta,
      },
    });
  });

  // ─── Search ───────────────────────────────────────────────────────────

  app.get('/rest/search3.view', async (request) => {
    const params = (request as unknown as SubsonicRequest).query;
    const query = params.query ?? '';
    const artistCount = Math.min(parseInt(params.artistCount ?? '20', 10), 500);
    const artistOffset = parseInt(params.artistOffset ?? '0', 10);
    const albumCount = Math.min(parseInt(params.albumCount ?? '20', 10), 500);
    const albumOffset = parseInt(params.albumOffset ?? '0', 10);
    const songCount = Math.min(parseInt(params.songCount ?? '20', 10), 500);
    const songOffset = parseInt(params.songOffset ?? '0', 10);

    if (!query) {
      return okResponse({
        searchResult3: { artist: [], album: [], song: [] },
      });
    }

    const [artists, albums, songs] = await Promise.all([
      db.searchArtists(query, artistCount, artistOffset),
      db.searchAlbums(query, albumCount, albumOffset),
      db.searchSongs(query, songCount, songOffset),
    ]);

    const subsonicArtists = artists.map(artistToSubsonic);
    const subsonicAlbums = await Promise.all(
      albums.map(async (album) => {
        let artistName: string | undefined;
        if (album.artist_id) {
          const artist = await db.getArtistById(album.artist_id);
          artistName = artist?.name;
        }
        return albumToSubsonic(album, artistName);
      })
    );
    const subsonicSongs = await Promise.all(
      songs.map(async (song) => {
        let artistName: string | undefined;
        let albumTitle: string | undefined;
        if (song.artist_id) {
          const artist = await db.getArtistById(song.artist_id);
          artistName = artist?.name;
        }
        if (song.album_id) {
          const album = await db.getAlbumById(song.album_id);
          albumTitle = album?.title;
        }
        return songToSubsonic(song, artistName, albumTitle);
      })
    );

    return okResponse({
      searchResult3: {
        artist: subsonicArtists,
        album: subsonicAlbums,
        song: subsonicSongs,
      },
    });
  });

  // ─── Streaming ────────────────────────────────────────────────────────

  app.get('/rest/stream.view', async (request, reply) => {
    const params = (request as unknown as SubsonicRequest).query;
    const id = params.id;

    if (!id) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.MISSING_PARAMETER, 'Required parameter is missing: id'));
    }

    const song = await db.getSongById(id);
    if (!song) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.NOT_FOUND, 'Song not found'));
    }

    const maxBitRate = params.maxBitRate ? parseInt(params.maxBitRate, 10) : fullConfig.maxBitrate;
    const rangeHeader = (request as unknown as { headers: Record<string, string> }).headers['range'];

    await streamAudio(song, reply as never, {
      maxBitRate,
      transcodeEnabled: fullConfig.transcodeEnabled,
      rangeHeader,
    });
  });

  // ─── Cover Art ────────────────────────────────────────────────────────

  app.get('/rest/getCoverArt.view', async (request, reply) => {
    const params = (request as unknown as SubsonicRequest).query;
    const id = params.id;

    if (!id) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.MISSING_PARAMETER, 'Required parameter is missing: id'));
    }

    const size = params.size ? parseInt(params.size, 10) : undefined;

    const coverPath = await resolveCoverArtPath(
      id,
      (albumId) => db.getAlbumById(albumId),
      (artistId) => db.getArtistById(artistId),
      (songId) => db.getSongById(songId),
      fullConfig.coverArtPath
    );

    if (!coverPath) {
      return reply.status(404).send({ error: 'Cover art not found' });
    }

    await serveCoverArt(coverPath, reply as never, size);
  });

  // ─── Starring ─────────────────────────────────────────────────────────

  app.get('/rest/star.view', async (request) => {
    const params = (request as unknown as SubsonicRequest).query;
    const songId = params.id;
    const albumId = params.albumId;
    const artistId = params.artistId;

    if (songId) await db.starSong(songId);
    if (albumId) await db.starAlbum(albumId);
    if (artistId) await db.starArtist(artistId);

    return okResponse();
  });

  app.get('/rest/unstar.view', async (request) => {
    const params = (request as unknown as SubsonicRequest).query;
    const songId = params.id;
    const albumId = params.albumId;
    const artistId = params.artistId;

    if (songId) await db.unstarSong(songId);
    if (albumId) await db.unstarAlbum(albumId);
    if (artistId) await db.unstarArtist(artistId);

    return okResponse();
  });

  app.get('/rest/getStarred2.view', async () => {
    const [artists, albums, songs] = await Promise.all([
      db.getStarredArtists(),
      db.getStarredAlbums(),
      db.getStarredSongs(),
    ]);

    const subsonicArtists = artists.map(artistToSubsonic);
    const subsonicAlbums = await Promise.all(
      albums.map(async (album) => {
        let artistName: string | undefined;
        if (album.artist_id) {
          const artist = await db.getArtistById(album.artist_id);
          artistName = artist?.name;
        }
        return albumToSubsonic(album, artistName);
      })
    );
    const subsonicSongs = await Promise.all(
      songs.map(async (song) => {
        let artistName: string | undefined;
        let albumTitle: string | undefined;
        if (song.artist_id) {
          const artist = await db.getArtistById(song.artist_id);
          artistName = artist?.name;
        }
        if (song.album_id) {
          const album = await db.getAlbumById(song.album_id);
          albumTitle = album?.title;
        }
        return songToSubsonic(song, artistName, albumTitle);
      })
    );

    return okResponse({
      starred2: {
        artist: subsonicArtists,
        album: subsonicAlbums,
        song: subsonicSongs,
      },
    });
  });

  // ─── Scrobbling ───────────────────────────────────────────────────────

  app.get('/rest/scrobble.view', async (request, reply) => {
    const params = (request as unknown as SubsonicRequest).query;
    const id = params.id;
    const submission = params.submission !== 'false';
    const time = params.time ? new Date(parseInt(params.time, 10)) : undefined;
    const username = getUser(request);

    if (!id) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.MISSING_PARAMETER, 'Required parameter is missing: id'));
    }

    const song = await db.getSongById(id);
    if (!song) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.NOT_FOUND, 'Song not found'));
    }

    await db.addScrobble(id, username, submission, time);

    if (submission) {
      await db.incrementPlayCount(id);
    }

    return okResponse();
  });

  // ─── Playlists ────────────────────────────────────────────────────────

  app.get('/rest/getPlaylists.view', async () => {
    const playlists = await db.getPlaylists();
    return okResponse({
      playlists: {
        playlist: playlists.map(playlistToSubsonic),
      },
    });
  });

  app.get('/rest/getPlaylist.view', async (request, reply) => {
    const params = (request as unknown as SubsonicRequest).query;
    const id = params.id;

    if (!id) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.MISSING_PARAMETER, 'Required parameter is missing: id'));
    }

    const playlist = await db.getPlaylistById(id);
    if (!playlist) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.NOT_FOUND, 'Playlist not found'));
    }

    const songs = await db.getPlaylistSongs(id);
    const entries = await Promise.all(
      songs.map(async (song) => {
        let artistName: string | undefined;
        let albumTitle: string | undefined;
        if (song.artist_id) {
          const artist = await db.getArtistById(song.artist_id);
          artistName = artist?.name;
        }
        if (song.album_id) {
          const album = await db.getAlbumById(song.album_id);
          albumTitle = album?.title;
        }
        return songToSubsonic(song, artistName, albumTitle);
      })
    );

    return okResponse({
      playlist: {
        ...playlistToSubsonic(playlist),
        entry: entries,
      },
    });
  });

  app.get('/rest/createPlaylist.view', async (request) => {
    const params = (request as unknown as SubsonicRequest).query;
    const name = params.name ?? 'New Playlist';
    const username = getUser(request);

    // Collect songId params (can be repeated: songId=a&songId=b)
    const url = new URL(
      (request as unknown as { url: string }).url,
      `http://${fullConfig.host}:${fullConfig.port}`
    );
    const songIds = url.searchParams.getAll('songId');

    const playlist = await db.createPlaylist(name, songIds, username);
    const songs = await db.getPlaylistSongs(playlist.id);
    const entries = await Promise.all(
      songs.map(async (song) => {
        let artistName: string | undefined;
        let albumTitle: string | undefined;
        if (song.artist_id) {
          const artist = await db.getArtistById(song.artist_id);
          artistName = artist?.name;
        }
        if (song.album_id) {
          const album = await db.getAlbumById(song.album_id);
          albumTitle = album?.title;
        }
        return songToSubsonic(song, artistName, albumTitle);
      })
    );

    return okResponse({
      playlist: {
        ...playlistToSubsonic(playlist),
        entry: entries,
      },
    });
  });

  app.get('/rest/updatePlaylist.view', async (request, reply) => {
    const params = (request as unknown as SubsonicRequest).query;
    const playlistId = params.playlistId;

    if (!playlistId) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.MISSING_PARAMETER, 'Required parameter is missing: playlistId'));
    }

    const playlist = await db.getPlaylistById(playlistId);
    if (!playlist) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.NOT_FOUND, 'Playlist not found'));
    }

    // Parse repeated params from URL
    const url = new URL(
      (request as unknown as { url: string }).url,
      `http://${fullConfig.host}:${fullConfig.port}`
    );
    const songIdsToAdd = url.searchParams.getAll('songIdToAdd');
    const songIndicesToRemove = url.searchParams.getAll('songIndexToRemove').map(Number);

    await db.updatePlaylist(playlistId, {
      name: params.name,
      comment: params.comment,
      isPublic: params.public !== undefined ? params.public === 'true' : undefined,
      songIdsToAdd: songIdsToAdd.length > 0 ? songIdsToAdd : undefined,
      songIndicesToRemove: songIndicesToRemove.length > 0 ? songIndicesToRemove : undefined,
    });

    return okResponse();
  });

  app.get('/rest/deletePlaylist.view', async (request, reply) => {
    const params = (request as unknown as SubsonicRequest).query;
    const id = params.id;

    if (!id) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.MISSING_PARAMETER, 'Required parameter is missing: id'));
    }

    const playlist = await db.getPlaylistById(id);
    if (!playlist) {
      return reply.send(errorResponse(SUBSONIC_ERROR_CODES.NOT_FOUND, 'Playlist not found'));
    }

    await db.deletePlaylist(id);
    return okResponse();
  });

  // ─── Genres ───────────────────────────────────────────────────────────

  app.get('/rest/getGenres.view', async () => {
    const genres = await db.getGenres();
    return okResponse({
      genres: {
        genre: genres.map(g => ({
          value: g.value,
          songCount: g.songCount,
          albumCount: g.albumCount,
        })),
      },
    });
  });

  // ─── Library Scan (non-Subsonic, admin endpoint) ──────────────────────

  app.post('/api/scan', async (_request, reply) => {
    try {
      const result = await scanLibrary(db, fullConfig.musicPaths, fullConfig.coverArtPath);
      return {
        success: true,
        ...result,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Library scan failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // ─── Status (non-Subsonic, admin endpoint) ────────────────────────────

  app.get('/status', async () => {
    const stats = await db.getStats();
    return {
      plugin: 'subsonic',
      version: '1.0.0',
      status: 'running',
      stats,
      musicPaths: fullConfig.musicPaths,
      timestamp: new Date().toISOString(),
    };
  });

  // ─── Stats API ────────────────────────────────────────────────────────

  app.get('/api/stats', async () => {
    return await db.getStats();
  });

  // ─── Graceful Shutdown ────────────────────────────────────────────────

  const shutdown = async () => {
    logger.info('Shutting down...');
    await app.close();
    await db.disconnect();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return {
    app,
    db,
    start: async () => {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.success(`Subsonic server running on http://${fullConfig.host}:${fullConfig.port}`);
      logger.info(`Subsonic API: http://${fullConfig.host}:${fullConfig.port}/rest/ping.view`);
      logger.info(`Music paths: ${fullConfig.musicPaths.join(', ')}`);
    },
    stop: shutdown,
  };
}

// Start server if run directly
const isMainModule = import.meta.url === `file://${process.argv[1]}`;
if (isMainModule) {
  createServer()
    .then(server => server.start())
    .catch(error => {
      logger.error('Failed to start server', { error: error.message });
      process.exit(1);
    });
}
