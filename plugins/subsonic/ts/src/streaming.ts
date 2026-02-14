/**
 * Audio Streaming and Cover Art Serving
 * Handles streaming audio files and serving cover art images for Subsonic clients.
 */

import fs from 'node:fs';
import fsp from 'node:fs/promises';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { pipeline } from 'node:stream/promises';
import { createLogger } from '@nself/plugin-utils';
import type { SongRecord, AlbumRecord, ArtistRecord } from './types.js';
import { EXTENSION_CONTENT_TYPES } from './types.js';

const logger = createLogger('subsonic:streaming');

/**
 * Check if ffmpeg is available for transcoding.
 */
let ffmpegAvailable: boolean | null = null;

async function checkFfmpeg(): Promise<boolean> {
  if (ffmpegAvailable !== null) return ffmpegAvailable;

  return new Promise((resolve) => {
    const proc = spawn('ffmpeg', ['-version'], { stdio: 'ignore' });
    proc.on('error', () => {
      ffmpegAvailable = false;
      resolve(false);
    });
    proc.on('close', (code) => {
      ffmpegAvailable = code === 0;
      resolve(ffmpegAvailable);
    });
  });
}

/**
 * Stream an audio file directly to the client.
 * Supports range requests for seeking.
 */
export async function streamAudio(
  song: SongRecord,
  reply: {
    header: (name: string, value: string) => void;
    status: (code: number) => { send: (body: unknown) => void };
    raw: NodeJS.WritableStream;
  },
  options: {
    maxBitRate?: number;
    transcodeEnabled?: boolean;
    rangeHeader?: string;
  } = {}
): Promise<void> {
  const filePath = song.file_path;

  // Verify file exists
  try {
    await fsp.access(filePath, fs.constants.R_OK);
  } catch {
    logger.error('Audio file not found', { filePath });
    reply.status(404).send({ error: 'Audio file not found' });
    return;
  }

  const stat = await fsp.stat(filePath);
  const ext = path.extname(filePath).toLowerCase();
  const contentType = song.content_type ?? EXTENSION_CONTENT_TYPES[ext] ?? 'application/octet-stream';
  const fileBitrate = song.bitrate ?? 320;

  // Determine if transcoding is needed
  const shouldTranscode =
    options.transcodeEnabled !== false &&
    options.maxBitRate &&
    options.maxBitRate > 0 &&
    fileBitrate > options.maxBitRate &&
    await checkFfmpeg();

  if (shouldTranscode && options.maxBitRate) {
    // Transcode with ffmpeg to mp3 at requested bitrate
    logger.debug('Transcoding audio', { filePath, targetBitrate: options.maxBitRate });

    reply.header('Content-Type', 'audio/mpeg');
    reply.header('Accept-Ranges', 'none');
    reply.header('Transfer-Encoding', 'chunked');

    const ffmpeg = spawn('ffmpeg', [
      '-i', filePath,
      '-map', '0:a:0',
      '-b:a', `${options.maxBitRate}k`,
      '-f', 'mp3',
      '-v', 'quiet',
      'pipe:1',
    ]);

    ffmpeg.stderr.on('data', (data: Buffer) => {
      logger.debug(`ffmpeg: ${data.toString()}`);
    });

    ffmpeg.on('error', (error) => {
      logger.error('ffmpeg error', { error: error.message });
    });

    try {
      await pipeline(ffmpeg.stdout, reply.raw);
    } catch (error) {
      // Client may disconnect mid-stream, which is normal
      const message = error instanceof Error ? error.message : 'Unknown error';
      if (!message.includes('EPIPE') && !message.includes('ERR_STREAM_PREMATURE_CLOSE')) {
        logger.error('Stream pipeline error', { error: message });
      }
    }
    return;
  }

  // Direct file streaming (no transcode)
  reply.header('Content-Type', contentType);
  reply.header('Accept-Ranges', 'bytes');

  // Handle range requests for seeking
  if (options.rangeHeader) {
    const match = options.rangeHeader.match(/bytes=(\d+)-(\d*)/);
    if (match) {
      const start = parseInt(match[1], 10);
      const end = match[2] ? parseInt(match[2], 10) : stat.size - 1;
      const chunkSize = end - start + 1;

      reply.status(206);
      reply.header('Content-Range', `bytes ${start}-${end}/${stat.size}`);
      reply.header('Content-Length', chunkSize.toString());

      const stream = fs.createReadStream(filePath, { start, end });
      try {
        await pipeline(stream, reply.raw);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        if (!message.includes('EPIPE') && !message.includes('ERR_STREAM_PREMATURE_CLOSE')) {
          logger.error('Range stream error', { error: message });
        }
      }
      return;
    }
  }

  // Full file streaming
  reply.header('Content-Length', stat.size.toString());

  const stream = fs.createReadStream(filePath);
  try {
    await pipeline(stream, reply.raw);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    if (!message.includes('EPIPE') && !message.includes('ERR_STREAM_PREMATURE_CLOSE')) {
      logger.error('Stream error', { error: message });
    }
  }
}

/**
 * Resolve cover art path from an ID.
 * IDs follow the pattern: al-{albumId}, ar-{artistId}, so-{songId}, or a raw path.
 */
export async function resolveCoverArtPath(
  id: string,
  getAlbum: (id: string) => Promise<AlbumRecord | null>,
  getArtist: (id: string) => Promise<ArtistRecord | null>,
  getSong: (id: string) => Promise<SongRecord | null>,
  coverArtBasePath: string
): Promise<string | null> {
  if (id.startsWith('al-')) {
    const albumId = id.slice(3);
    const album = await getAlbum(albumId);
    return album?.cover_art_path ?? null;
  }

  if (id.startsWith('ar-')) {
    const artistId = id.slice(3);
    const artist = await getArtist(artistId);
    if (artist?.image_url) {
      // image_url could be a local path or external URL
      if (artist.image_url.startsWith('/') || artist.image_url.startsWith('.')) {
        return artist.image_url;
      }
      return null; // External URLs not served as local files
    }
    return null;
  }

  if (id.startsWith('so-')) {
    const songId = id.slice(3);
    const song = await getSong(songId);
    if (song?.cover_art_path) return song.cover_art_path;
    if (song?.album_id) {
      const album = await getAlbum(song.album_id);
      return album?.cover_art_path ?? null;
    }
    return null;
  }

  // Try as a direct album ID (some clients may send just the album ID)
  const album = await getAlbum(id);
  if (album?.cover_art_path) return album.cover_art_path;

  // Try as a file path under coverArtBasePath
  const directPath = path.join(coverArtBasePath, id);
  try {
    await fsp.access(directPath, fs.constants.R_OK);
    return directPath;
  } catch {
    return null;
  }
}

/**
 * Serve a cover art image.
 */
export async function serveCoverArt(
  coverPath: string,
  reply: {
    header: (name: string, value: string) => void;
    status: (code: number) => { send: (body: unknown) => void };
    raw: NodeJS.WritableStream;
  },
  requestedSize?: number
): Promise<void> {
  try {
    await fsp.access(coverPath, fs.constants.R_OK);
  } catch {
    logger.debug('Cover art not found', { coverPath });
    reply.status(404).send({ error: 'Cover art not found' });
    return;
  }

  const ext = path.extname(coverPath).toLowerCase();
  const imageContentTypes: Record<string, string> = {
    '.jpg': 'image/jpeg',
    '.jpeg': 'image/jpeg',
    '.png': 'image/png',
    '.webp': 'image/webp',
    '.gif': 'image/gif',
    '.bmp': 'image/bmp',
  };
  const contentType = imageContentTypes[ext] ?? 'image/jpeg';

  // If size is requested and ffmpeg is available, resize
  if (requestedSize && requestedSize < 1000 && await checkFfmpeg()) {
    reply.header('Content-Type', 'image/jpeg');

    const ffmpeg = spawn('ffmpeg', [
      '-i', coverPath,
      '-vf', `scale=${requestedSize}:${requestedSize}:force_original_aspect_ratio=decrease`,
      '-f', 'image2',
      '-vcodec', 'mjpeg',
      '-q:v', '3',
      '-v', 'quiet',
      'pipe:1',
    ]);

    try {
      await pipeline(ffmpeg.stdout, reply.raw);
    } catch {
      // Fall back to serving original
      const stream = fs.createReadStream(coverPath);
      reply.header('Content-Type', contentType);
      await pipeline(stream, reply.raw).catch(() => { /* client disconnect */ });
    }
    return;
  }

  // Serve original file
  const stat = await fsp.stat(coverPath);
  reply.header('Content-Type', contentType);
  reply.header('Content-Length', stat.size.toString());
  reply.header('Cache-Control', 'public, max-age=86400');

  const stream = fs.createReadStream(coverPath);
  try {
    await pipeline(stream, reply.raw);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    if (!message.includes('EPIPE') && !message.includes('ERR_STREAM_PREMATURE_CLOSE')) {
      logger.error('Cover art stream error', { error: message });
    }
  }
}
