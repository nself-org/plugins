/**
 * HTTP Media Streaming
 * Handles streaming media files to DLNA renderers with HTTP Range support
 */

import fs from 'node:fs';
import { stat } from 'node:fs/promises';
import path from 'node:path';
import { createLogger } from '@nself/plugin-utils';
import type { FastifyRequest, FastifyReply } from 'fastify';
import type { DlnaDatabase } from './database.js';

const logger = createLogger('dlna:streaming');

interface RangeInfo {
  start: number;
  end: number;
  total: number;
}

/**
 * Parse an HTTP Range header.
 * Returns null if the header is invalid or missing.
 */
function parseRangeHeader(rangeHeader: string | undefined, fileSize: number): RangeInfo | null {
  if (!rangeHeader) return null;

  const match = rangeHeader.match(/bytes=(\d*)-(\d*)/);
  if (!match) return null;

  const startStr = match[1];
  const endStr = match[2];

  let start: number;
  let end: number;

  if (startStr === '' && endStr !== '') {
    // Suffix range: -500 means last 500 bytes
    const suffix = parseInt(endStr, 10);
    start = Math.max(0, fileSize - suffix);
    end = fileSize - 1;
  } else if (startStr !== '' && endStr === '') {
    // Open-ended range: 500- means from byte 500 to end
    start = parseInt(startStr, 10);
    end = fileSize - 1;
  } else if (startStr !== '' && endStr !== '') {
    // Explicit range: 500-999
    start = parseInt(startStr, 10);
    end = parseInt(endStr, 10);
  } else {
    return null;
  }

  // Validate range
  if (start < 0 || start >= fileSize || end < start || end >= fileSize) {
    return null;
  }

  return { start, end, total: fileSize };
}

/**
 * Handle a media streaming request.
 * Supports HTTP Range requests for seeking.
 */
export async function handleMediaStream(
  request: FastifyRequest<{ Params: { id: string } }>,
  reply: FastifyReply,
  db: DlnaDatabase
): Promise<void> {
  const { id } = request.params;

  // Look up the media item in the database
  const item = await db.getMediaItem(id);
  if (!item || !item.file_path) {
    reply.status(404).send({ error: 'Media item not found' });
    return;
  }

  // Verify the file exists
  let fileStats;
  try {
    fileStats = await stat(item.file_path);
  } catch {
    logger.error('Media file not found on disk', { id, path: item.file_path });
    reply.status(404).send({ error: 'Media file not found on disk' });
    return;
  }

  const fileSize = fileStats.size;
  const mimeType = item.mime_type ?? 'application/octet-stream';
  const fileName = path.basename(item.file_path);

  // Set common headers
  reply.header('Accept-Ranges', 'bytes');
  reply.header('Content-Type', mimeType);
  reply.header('Content-Disposition', `inline; filename="${encodeURIComponent(fileName)}"`);

  // DLNA-specific headers
  reply.header('transferMode.dlna.org', 'Streaming');
  reply.header('contentFeatures.dlna.org', buildDlnaContentFeatures(mimeType));

  // Check for Range header
  const rangeHeader = request.headers.range;
  const range = parseRangeHeader(rangeHeader, fileSize);

  if (range) {
    // Partial content response
    const contentLength = range.end - range.start + 1;

    reply.status(206);
    reply.header('Content-Length', String(contentLength));
    reply.header('Content-Range', `bytes ${range.start}-${range.end}/${range.total}`);

    logger.debug('Streaming partial content', {
      id,
      range: `${range.start}-${range.end}/${range.total}`,
      contentLength,
    });

    const stream = fs.createReadStream(item.file_path, {
      start: range.start,
      end: range.end,
    });

    reply.send(stream);
  } else if (rangeHeader) {
    // Invalid range requested
    reply.status(416);
    reply.header('Content-Range', `bytes */${fileSize}`);
    reply.send({ error: 'Range not satisfiable' });
  } else {
    // Full content response
    reply.status(200);
    reply.header('Content-Length', String(fileSize));

    logger.debug('Streaming full content', { id, fileSize });

    const stream = fs.createReadStream(item.file_path);
    reply.send(stream);
  }
}

/**
 * Handle a thumbnail request for a media item
 */
export async function handleThumbnailStream(
  request: FastifyRequest<{ Params: { id: string } }>,
  reply: FastifyReply,
  db: DlnaDatabase
): Promise<void> {
  const { id } = request.params;

  const item = await db.getMediaItem(id);
  if (!item || !item.thumbnail_path) {
    reply.status(404).send({ error: 'Thumbnail not found' });
    return;
  }

  try {
    await stat(item.thumbnail_path);
  } catch {
    reply.status(404).send({ error: 'Thumbnail file not found on disk' });
    return;
  }

  const ext = path.extname(item.thumbnail_path).toLowerCase();
  const mimeType = ext === '.png' ? 'image/png' : 'image/jpeg';

  reply.header('Content-Type', mimeType);
  reply.header('Cache-Control', 'public, max-age=86400');

  const stream = fs.createReadStream(item.thumbnail_path);
  reply.send(stream);
}

/**
 * Build DLNA content features header value for a given MIME type
 */
function buildDlnaContentFeatures(mimeType: string): string {
  // DLNA.ORG_OP=01 means byte-based seeking supported
  // DLNA.ORG_CI=0 means content is not transcoded
  // DLNA.ORG_FLAGS first 8 hex chars control streaming behavior
  const flags = '01500000000000000000000000000000';

  if (mimeType.startsWith('video/')) {
    return `DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=${flags}`;
  }

  if (mimeType.startsWith('audio/')) {
    return `DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=${flags}`;
  }

  if (mimeType.startsWith('image/')) {
    return `DLNA.ORG_OP=00;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=${flags}`;
  }

  return `DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=${flags}`;
}
