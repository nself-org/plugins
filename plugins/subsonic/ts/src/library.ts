/**
 * Music Library Scanner
 * Recursively scans configured music directories, reads audio metadata,
 * and indexes all tracks into the database.
 */

import fs from 'node:fs';
import fsp from 'node:fs/promises';
import path from 'node:path';
import { createLogger } from '@nself/plugin-utils';
import { SubsonicDatabase } from './database.js';
import type { AudioMetadata, ScanResult } from './types.js';
import { SUPPORTED_AUDIO_EXTENSIONS, EXTENSION_CONTENT_TYPES } from './types.js';

const logger = createLogger('subsonic:library');

/**
 * Parse audio metadata from a file using music-metadata.
 * Dynamically imports music-metadata to avoid issues with ESM/CJS interop at load time.
 */
async function parseAudioMetadata(filePath: string): Promise<AudioMetadata> {
  const mm = await import('music-metadata');
  const ext = path.extname(filePath).toLowerCase();
  const contentType = EXTENSION_CONTENT_TYPES[ext] ?? 'application/octet-stream';

  try {
    const metadata = await mm.parseFile(filePath, { duration: true });
    const common = metadata.common;
    const format = metadata.format;

    let coverArt: Buffer | null = null;
    let coverArtMime: string | null = null;
    if (common.picture && common.picture.length > 0) {
      coverArt = Buffer.from(common.picture[0].data);
      coverArtMime = common.picture[0].format ?? null;
    }

    return {
      title: common.title ?? path.basename(filePath, ext),
      artist: common.artist ?? common.albumartist ?? null,
      album: common.album ?? null,
      year: common.year ?? null,
      trackNumber: common.track?.no ?? null,
      discNumber: common.disk?.no ?? null,
      genre: common.genre?.[0] ?? null,
      duration: format.duration ? Math.round(format.duration) : null,
      bitrate: format.bitrate ? Math.round(format.bitrate / 1000) : null,
      contentType,
      coverArt,
      coverArtMime,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.warn(`Failed to parse metadata for ${filePath}`, { error: message });

    return {
      title: path.basename(filePath, ext),
      artist: null,
      album: null,
      year: null,
      trackNumber: null,
      discNumber: null,
      genre: null,
      duration: null,
      bitrate: null,
      contentType,
      coverArt: null,
      coverArtMime: null,
    };
  }
}

/**
 * Recursively collect all audio file paths under a directory.
 */
async function collectAudioFiles(dirPath: string): Promise<string[]> {
  const files: string[] = [];

  async function walk(dir: string): Promise<void> {
    let entries: fs.Dirent[];
    try {
      entries = await fsp.readdir(dir, { withFileTypes: true });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.warn(`Cannot read directory: ${dir}`, { error: message });
      return;
    }

    for (const entry of entries) {
      const fullPath = path.join(dir, entry.name);

      // Skip hidden files and directories
      if (entry.name.startsWith('.')) continue;

      if (entry.isDirectory()) {
        await walk(fullPath);
      } else if (entry.isFile()) {
        const ext = path.extname(entry.name).toLowerCase();
        if (SUPPORTED_AUDIO_EXTENSIONS.has(ext)) {
          files.push(fullPath);
        }
      }
    }
  }

  await walk(dirPath);
  return files;
}

/**
 * Save embedded cover art to disk.
 */
async function saveCoverArt(
  coverArtPath: string,
  albumId: string,
  coverArt: Buffer,
  mime: string
): Promise<string> {
  await fsp.mkdir(coverArtPath, { recursive: true });

  let ext = '.jpg';
  if (mime.includes('png')) ext = '.png';
  else if (mime.includes('webp')) ext = '.webp';
  else if (mime.includes('gif')) ext = '.gif';

  const filePath = path.join(coverArtPath, `${albumId}${ext}`);
  await fsp.writeFile(filePath, coverArt);
  return filePath;
}

/**
 * Find cover art images in the same directory as the audio file.
 */
async function findExternalCoverArt(dirPath: string): Promise<string | null> {
  const coverNames = ['cover', 'folder', 'album', 'front', 'artwork', 'art'];

  try {
    const entries = await fsp.readdir(dirPath);
    for (const entry of entries) {
      const name = path.parse(entry).name.toLowerCase();
      const ext = path.extname(entry).toLowerCase();
      if (coverNames.includes(name) && ['.jpg', '.jpeg', '.png', '.webp'].includes(ext)) {
        return path.join(dirPath, entry);
      }
    }
  } catch {
    // Directory not readable
  }

  return null;
}

/**
 * Scan music directories and index all files into the database.
 */
export async function scanLibrary(
  db: SubsonicDatabase,
  musicPaths: string[],
  coverArtPath: string
): Promise<ScanResult> {
  const startTime = Date.now();
  const result: ScanResult = {
    songsAdded: 0,
    songsUpdated: 0,
    albumsCreated: 0,
    artistsCreated: 0,
    errors: [],
    duration: 0,
  };

  const validFilePaths = new Set<string>();
  const artistCache = new Map<string, string>(); // name -> id
  const albumCache = new Map<string, string>(); // "artist|album" -> id
  const albumCoverStatus = new Set<string>(); // album ids that already have cover art

  // Register music folders
  for (const musicPath of musicPaths) {
    const folderName = path.basename(musicPath) || musicPath;
    try {
      await fsp.access(musicPath, fs.constants.R_OK);
      await db.upsertMusicFolder(folderName, musicPath);
      logger.info(`Registered music folder: ${musicPath}`);
    } catch {
      logger.warn(`Music path not accessible: ${musicPath}`);
      result.errors.push(`Music path not accessible: ${musicPath}`);
    }
  }

  // Collect all audio files
  const allFiles: string[] = [];
  for (const musicPath of musicPaths) {
    try {
      const files = await collectAudioFiles(musicPath);
      allFiles.push(...files);
      logger.info(`Found ${files.length} audio files in ${musicPath}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`Failed to scan ${musicPath}`, { error: message });
      result.errors.push(`Failed to scan ${musicPath}: ${message}`);
    }
  }

  logger.info(`Total audio files found: ${allFiles.length}`);

  // Process each file
  for (let i = 0; i < allFiles.length; i++) {
    const filePath = allFiles[i];
    validFilePaths.add(filePath);

    if ((i + 1) % 100 === 0) {
      logger.info(`Processing file ${i + 1} / ${allFiles.length}`);
    }

    try {
      const metadata = await parseAudioMetadata(filePath);
      const stat = await fsp.stat(filePath);

      // Resolve or create artist
      let artistId: string | null = null;
      if (metadata.artist) {
        const cachedArtistId = artistCache.get(metadata.artist.toLowerCase());
        if (cachedArtistId) {
          artistId = cachedArtistId;
        } else {
          const artist = await db.getOrCreateArtist(metadata.artist);
          artistId = artist.id;
          artistCache.set(metadata.artist.toLowerCase(), artist.id);
          if (!artist.synced_at || (Date.now() - artist.synced_at.getTime()) > 1000) {
            result.artistsCreated++;
          }
        }
      }

      // Resolve or create album
      let albumId: string | null = null;
      if (metadata.album) {
        const albumKey = `${(metadata.artist ?? '').toLowerCase()}|${metadata.album.toLowerCase()}`;
        const cachedAlbumId = albumCache.get(albumKey);
        if (cachedAlbumId) {
          albumId = cachedAlbumId;
        } else {
          const album = await db.getOrCreateAlbum(
            metadata.album,
            artistId,
            metadata.year,
            metadata.genre
          );
          albumId = album.id;
          albumCache.set(albumKey, album.id);
          if (!album.synced_at || (Date.now() - album.synced_at.getTime()) > 1000) {
            result.albumsCreated++;
          }
        }

        // Handle cover art for this album
        if (albumId && !albumCoverStatus.has(albumId)) {
          let coverPath: string | null = null;

          // First try embedded cover art
          if (metadata.coverArt && metadata.coverArtMime) {
            try {
              coverPath = await saveCoverArt(
                coverArtPath,
                albumId,
                metadata.coverArt,
                metadata.coverArtMime
              );
            } catch (error) {
              const msg = error instanceof Error ? error.message : 'Unknown error';
              logger.debug(`Failed to save embedded cover art for album ${albumId}`, { error: msg });
            }
          }

          // Fall back to external cover art in the directory
          if (!coverPath) {
            coverPath = await findExternalCoverArt(path.dirname(filePath));
          }

          if (coverPath) {
            await db.updateAlbumCoverArt(albumId, coverPath);
            albumCoverStatus.add(albumId);
          }
        }
      }

      // Upsert song
      const existing = await db.getSongByFilePath(filePath);
      await db.upsertSong({
        source_account_id: 'primary', // Managed by database class
        album_id: albumId,
        artist_id: artistId,
        title: metadata.title ?? path.basename(filePath),
        track_number: metadata.trackNumber,
        disc_number: metadata.discNumber ?? 1,
        year: metadata.year,
        genre: metadata.genre,
        duration_seconds: metadata.duration,
        file_path: filePath,
        file_size: stat.size,
        bitrate: metadata.bitrate,
        content_type: metadata.contentType,
        cover_art_path: null, // Songs use album cover art reference
      });

      if (existing) {
        result.songsUpdated++;
      } else {
        result.songsAdded++;
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`Failed to process ${filePath}`, { error: message });
      result.errors.push(`${filePath}: ${message}`);
    }
  }

  // Update album stats
  const processedAlbumIds = new Set(albumCache.values());
  for (const albumId of processedAlbumIds) {
    await db.updateAlbumStats(albumId);
  }

  // Update artist album counts
  const processedArtistIds = new Set(artistCache.values());
  for (const artistId of processedArtistIds) {
    await db.updateArtistAlbumCount(artistId);
  }

  // Remove orphaned songs (files that no longer exist)
  const removedSongs = await db.removeOrphanedSongs(validFilePaths);
  if (removedSongs > 0) {
    logger.info(`Removed ${removedSongs} orphaned songs`);
  }

  // Remove orphaned albums and artists
  const removedAlbums = await db.removeOrphanedAlbums();
  if (removedAlbums > 0) {
    logger.info(`Removed ${removedAlbums} orphaned albums`);
  }

  const removedArtists = await db.removeOrphanedArtists();
  if (removedArtists > 0) {
    logger.info(`Removed ${removedArtists} orphaned artists`);
  }

  result.duration = Date.now() - startTime;
  logger.success(`Library scan complete in ${(result.duration / 1000).toFixed(1)}s`, {
    songsAdded: result.songsAdded,
    songsUpdated: result.songsUpdated,
    albumsCreated: result.albumsCreated,
    artistsCreated: result.artistsCreated,
    errors: result.errors.length,
  });

  return result;
}
