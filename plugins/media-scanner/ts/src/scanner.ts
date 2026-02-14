/**
 * Media Scanner - Directory Scanning
 * Recursive directory scanning for media files with batch processing
 */

import { readdir, stat } from 'node:fs/promises';
import { join, extname, basename } from 'node:path';
import { createLogger } from '@nself/plugin-utils';
import { MEDIA_EXTENSIONS } from './types.js';
import type { DiscoveredFile, ScanError } from './types.js';

const logger = createLogger('media-scanner:scanner');

export interface ScanProgress {
  filesFound: number;
  errors: ScanError[];
}

/**
 * Scan directories for media files.
 * Yields batches of discovered files for streaming processing.
 */
export async function* scanDirectories(
  paths: string[],
  recursive: boolean,
  batchSize = 50
): AsyncGenerator<DiscoveredFile[], ScanProgress, undefined> {
  const batch: DiscoveredFile[] = [];
  let totalFound = 0;
  const errors: ScanError[] = [];

  for (const rootPath of paths) {
    try {
      for await (const file of walkDirectory(rootPath, recursive)) {
        batch.push(file);
        totalFound++;

        if (batch.length >= batchSize) {
          yield [...batch];
          batch.length = 0;
        }
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to scan directory', { path: rootPath, error: message });
      errors.push({
        path: rootPath,
        error: message,
        timestamp: new Date().toISOString(),
      });
    }
  }

  // Yield remaining files
  if (batch.length > 0) {
    yield [...batch];
  }

  return { filesFound: totalFound, errors };
}

/**
 * Walk a directory recursively and yield media files.
 */
async function* walkDirectory(
  dirPath: string,
  recursive: boolean
): AsyncGenerator<DiscoveredFile> {
  let entries;
  try {
    entries = await readdir(dirPath, { withFileTypes: true });
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.warn('Cannot read directory', { path: dirPath, error: message });
    return;
  }

  for (const entry of entries) {
    const fullPath = join(dirPath, entry.name);

    if (entry.isDirectory() && recursive) {
      yield* walkDirectory(fullPath, recursive);
      continue;
    }

    if (!entry.isFile()) {
      continue;
    }

    const ext = extname(entry.name).toLowerCase();
    if (!MEDIA_EXTENSIONS.has(ext)) {
      continue;
    }

    try {
      const fileStat = await stat(fullPath);
      yield {
        path: fullPath,
        filename: basename(entry.name),
        size: fileStat.size,
        modified_at: fileStat.mtime,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.warn('Cannot stat file', { path: fullPath, error: message });
    }
  }
}

/**
 * Quick count of media files in given paths without yielding batches.
 */
export async function countMediaFiles(
  paths: string[],
  recursive: boolean
): Promise<number> {
  let count = 0;
  for (const rootPath of paths) {
    try {
      count += await countInDirectory(rootPath, recursive);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.warn('Cannot count files in directory', { path: rootPath, error: message });
    }
  }
  return count;
}

async function countInDirectory(dirPath: string, recursive: boolean): Promise<number> {
  let count = 0;
  let entries;
  try {
    entries = await readdir(dirPath, { withFileTypes: true });
  } catch {
    return 0;
  }

  for (const entry of entries) {
    if (entry.isDirectory() && recursive) {
      count += await countInDirectory(join(dirPath, entry.name), recursive);
    } else if (entry.isFile()) {
      const ext = extname(entry.name).toLowerCase();
      if (MEDIA_EXTENSIONS.has(ext)) {
        count++;
      }
    }
  }

  return count;
}
