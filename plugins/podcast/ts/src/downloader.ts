/**
 * Episode Audio Downloader
 * Download podcast episode audio files with progress tracking and resume support
 */

import { createWriteStream, existsSync, mkdirSync, statSync, unlinkSync, renameSync } from 'node:fs';
import { join } from 'node:path';
import { pipeline } from 'node:stream/promises';
import { Readable } from 'node:stream';
import { createLogger } from '@nself/plugin-utils';
import type { PodcastDatabase } from './database.js';
import type { EpisodeRecord, DownloadProgress } from './types.js';

const logger = createLogger('podcast:downloader');

/**
 * Download an episode audio file to the configured download path
 */
export async function downloadEpisode(
  episode: EpisodeRecord,
  downloadBasePath: string,
  db: PodcastDatabase,
  onProgress?: (progress: DownloadProgress) => void
): Promise<string> {
  if (!episode.enclosure_url) {
    throw new Error(`Episode ${episode.id} has no enclosure URL`);
  }

  // Build the download path: basePath/feedId/episodeId.ext
  const extension = getExtensionFromUrl(episode.enclosure_url) || getExtensionFromType(episode.enclosure_type);
  const sanitizedTitle = sanitizeFilename(episode.title);
  const filename = `${sanitizedTitle}-${episode.id.slice(0, 8)}${extension}`;
  const feedDir = join(downloadBasePath, episode.feed_id);
  const filePath = join(feedDir, filename);

  // Ensure directory exists
  mkdirSync(feedDir, { recursive: true });

  // Check for existing partial download
  let existingSize = 0;
  const partialPath = `${filePath}.part`;
  if (existsSync(partialPath)) {
    existingSize = statSync(partialPath).size;
    logger.info('Resuming partial download', { existingSize, episode: episode.id });
  }

  const progress: DownloadProgress = {
    episodeId: episode.id,
    bytesDownloaded: existingSize,
    totalBytes: episode.enclosure_length ?? null,
    status: 'downloading',
  };

  if (onProgress) onProgress(progress);

  try {
    const headers: Record<string, string> = {
      'User-Agent': 'nself-podcast/1.0.0',
    };

    // Resume from where we left off
    if (existingSize > 0) {
      headers['Range'] = `bytes=${existingSize}-`;
    }

    const response = await fetch(episode.enclosure_url, {
      headers,
      signal: AbortSignal.timeout(600000), // 10 minute timeout for large files
    });

    if (!response.ok && response.status !== 206) {
      // If range request fails, restart from scratch
      if (existingSize > 0 && response.status === 416) {
        // File already complete or server doesn't support ranges
        if (existsSync(partialPath)) {
          const finalSize = statSync(partialPath).size;
          if (finalSize > 0) {
            renameFile(partialPath, filePath);
            await db.markEpisodeDownloaded(episode.id, filePath);
            progress.status = 'complete';
            progress.bytesDownloaded = finalSize;
            if (onProgress) onProgress(progress);
            return filePath;
          }
        }
        existingSize = 0;
      } else {
        throw new Error(`Download failed: HTTP ${response.status} ${response.statusText}`);
      }
    }

    // Get total content length
    const contentLength = response.headers.get('content-length');
    if (contentLength) {
      progress.totalBytes = existingSize + parseInt(contentLength, 10);
    }

    if (!response.body) {
      throw new Error('Response body is null');
    }

    // Stream to file
    const writeStream = createWriteStream(partialPath, {
      flags: existingSize > 0 && response.status === 206 ? 'a' : 'w',
    });

    const reader = response.body.getReader();
    const nodeStream = new Readable({
      async read() {
        try {
          const { done, value } = await reader.read();
          if (done) {
            this.push(null);
            return;
          }
          progress.bytesDownloaded += value.byteLength;
          if (onProgress) onProgress(progress);
          this.push(Buffer.from(value));
        } catch (err) {
          this.destroy(err instanceof Error ? err : new Error(String(err)));
        }
      },
    });

    await pipeline(nodeStream, writeStream);

    // Rename .part to final filename
    renameFile(partialPath, filePath);

    // Update database
    await db.markEpisodeDownloaded(episode.id, filePath);

    progress.status = 'complete';
    if (onProgress) onProgress(progress);

    logger.info('Episode downloaded', {
      episode: episode.id,
      path: filePath,
      bytes: progress.bytesDownloaded,
    });

    return filePath;
  } catch (error) {
    progress.status = 'failed';
    progress.error = error instanceof Error ? error.message : 'Unknown error';
    if (onProgress) onProgress(progress);

    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Download failed', { episode: episode.id, error: message });
    throw error;
  }
}

/**
 * Clean up a failed download
 */
export function cleanupPartialDownload(
  episode: EpisodeRecord,
  downloadBasePath: string
): void {
  const extension = getExtensionFromUrl(episode.enclosure_url ?? '') || getExtensionFromType(episode.enclosure_type);
  const sanitizedTitle = sanitizeFilename(episode.title);
  const filename = `${sanitizedTitle}-${episode.id.slice(0, 8)}${extension}`;
  const feedDir = join(downloadBasePath, episode.feed_id);
  const partialPath = join(feedDir, `${filename}.part`);

  if (existsSync(partialPath)) {
    unlinkSync(partialPath);
    logger.info('Cleaned up partial download', { path: partialPath });
  }
}

// =========================================================================
// Helper Functions
// =========================================================================

function getExtensionFromUrl(url: string): string {
  try {
    const pathname = new URL(url).pathname;
    const lastDot = pathname.lastIndexOf('.');
    if (lastDot >= 0) {
      const ext = pathname.slice(lastDot).toLowerCase();
      // Only return known audio extensions
      const validExtensions = ['.mp3', '.m4a', '.mp4', '.ogg', '.opus', '.wav', '.aac', '.flac', '.wma'];
      if (validExtensions.includes(ext)) return ext;
    }
  } catch {
    // Invalid URL, fall through
  }
  return '';
}

function getExtensionFromType(mimeType: string | null): string {
  if (!mimeType) return '.mp3';
  const typeMap: Record<string, string> = {
    'audio/mpeg': '.mp3',
    'audio/mp3': '.mp3',
    'audio/mp4': '.m4a',
    'audio/x-m4a': '.m4a',
    'audio/aac': '.aac',
    'audio/ogg': '.ogg',
    'audio/opus': '.opus',
    'audio/wav': '.wav',
    'audio/flac': '.flac',
    'audio/x-ms-wma': '.wma',
    'video/mp4': '.mp4',
  };
  return typeMap[mimeType.toLowerCase()] ?? '.mp3';
}

function sanitizeFilename(name: string): string {
  return name
    .replace(/[^\w\s.-]/g, '')
    .replace(/\s+/g, '_')
    .slice(0, 100)
    .replace(/^[._]+|[._]+$/g, '')
    || 'episode';
}

function renameFile(from: string, to: string): void {
  renameSync(from, to);
}
