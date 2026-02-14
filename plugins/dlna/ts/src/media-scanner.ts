/**
 * Media Scanner
 * Scans configured directories for media files and indexes them in the database
 */

import fs from 'node:fs/promises';
import path from 'node:path';
import { createLogger } from '@nself/plugin-utils';
import type { DlnaDatabase } from './database.js';
import { MIME_TYPES, UPnPClass, getUpnpClassForMime, getMediaCategory } from './types.js';
import type { ScanResult, MediaFileInfo } from './types.js';
import { incrementSystemUpdateId } from './content-directory.js';

const logger = createLogger('dlna:scanner');

/**
 * Root container IDs (deterministic UUIDs for top-level categories)
 * These use namespace-style UUIDs so they are stable across scans.
 */
const ROOT_CONTAINERS: Record<string, { id: string; title: string; upnpClass: string; sortOrder: number }> = {
  Video: {
    id: '10000000-0000-0000-0000-000000000001',
    title: 'Video',
    upnpClass: UPnPClass.CONTAINER_STORAGE,
    sortOrder: 1,
  },
  Audio: {
    id: '10000000-0000-0000-0000-000000000002',
    title: 'Audio',
    upnpClass: UPnPClass.CONTAINER_STORAGE,
    sortOrder: 2,
  },
  Image: {
    id: '10000000-0000-0000-0000-000000000003',
    title: 'Image',
    upnpClass: UPnPClass.CONTAINER_STORAGE,
    sortOrder: 3,
  },
};

export class MediaScanner {
  private db: DlnaDatabase;
  private sourceAccountId: string;

  constructor(db: DlnaDatabase, sourceAccountId: string) {
    this.db = db;
    this.sourceAccountId = sourceAccountId;
  }

  /**
   * Perform a full scan of all configured media paths
   */
  async scan(mediaPaths: string[]): Promise<ScanResult> {
    const startTime = Date.now();
    const result: ScanResult = {
      totalFiles: 0,
      newFiles: 0,
      updatedFiles: 0,
      removedFiles: 0,
      errors: [],
      duration: 0,
    };

    logger.info('Starting media scan', { paths: mediaPaths });

    // First, ensure root category containers exist
    await this.ensureRootContainers();

    // Track all valid file paths seen during this scan
    const validPaths = new Set<string>();
    // Track folder containers we create (key: absolute folder path, value: container UUID)
    const folderContainers = new Map<string, string>();

    for (const mediaPath of mediaPaths) {
      try {
        await this.scanDirectory(mediaPath, mediaPaths, validPaths, folderContainers, result);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to scan directory', { path: mediaPath, error: message });
        result.errors.push(`Failed to scan ${mediaPath}: ${message}`);
      }
    }

    // Remove stale items (files that were previously indexed but no longer exist)
    try {
      const removedCount = await this.db.removeStaleItems(validPaths);
      result.removedFiles = removedCount;
      if (removedCount > 0) {
        logger.info('Removed stale media items', { count: removedCount });
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to remove stale items', { error: message });
      result.errors.push(`Failed to remove stale items: ${message}`);
    }

    result.duration = Date.now() - startTime;

    // Increment system update ID so DLNA clients know content changed
    incrementSystemUpdateId();

    logger.success('Media scan complete', {
      total: result.totalFiles,
      new: result.newFiles,
      updated: result.updatedFiles,
      removed: result.removedFiles,
      duration: `${result.duration}ms`,
    });

    return result;
  }

  /**
   * Ensure root category containers (Video, Audio, Image) exist in the database
   */
  private async ensureRootContainers(): Promise<void> {
    // Remove existing containers to rebuild cleanly
    await this.db.removeContainers();

    for (const [_category, container] of Object.entries(ROOT_CONTAINERS)) {
      await this.db.upsertMediaItem({
        id: container.id,
        source_account_id: this.sourceAccountId,
        parent_id: null,
        object_type: 'container',
        upnp_class: container.upnpClass,
        title: container.title,
        file_path: null,
        file_size: null,
        mime_type: null,
        duration_seconds: null,
        resolution: null,
        bitrate: null,
        album: null,
        artist: null,
        genre: null,
        thumbnail_path: null,
        sort_order: container.sortOrder,
      });
    }
  }

  /**
   * Recursively scan a directory for media files
   */
  private async scanDirectory(
    dirPath: string,
    rootPaths: string[],
    validPaths: Set<string>,
    folderContainers: Map<string, string>,
    result: ScanResult
  ): Promise<void> {
    let entries;
    try {
      entries = await fs.readdir(dirPath, { withFileTypes: true });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.warn('Cannot read directory', { path: dirPath, error: message });
      result.errors.push(`Cannot read ${dirPath}: ${message}`);
      return;
    }

    for (const entry of entries) {
      const fullPath = path.join(dirPath, entry.name);

      // Skip hidden files and directories
      if (entry.name.startsWith('.')) continue;

      if (entry.isDirectory()) {
        await this.scanDirectory(fullPath, rootPaths, validPaths, folderContainers, result);
      } else if (entry.isFile()) {
        const fileInfo = this.getMediaFileInfo(fullPath);
        if (!fileInfo) continue; // Not a recognized media file

        result.totalFiles++;
        validPaths.add(fullPath);

        try {
          // Determine which root container this file belongs to
          const category = getMediaCategory(fileInfo.mimeType);
          const rootContainer = ROOT_CONTAINERS[category];
          if (!rootContainer) continue;

          // Determine parent container (folder-based hierarchy)
          const parentContainerId = await this.ensureFolderContainer(
            dirPath,
            rootPaths,
            rootContainer.id,
            category,
            folderContainers
          );

          // Get file stats
          const stats = await fs.stat(fullPath);

          // Check if file already exists in database
          const existing = await this.db.getMediaItemByPath(fullPath);

          await this.db.upsertMediaItem({
            source_account_id: this.sourceAccountId,
            parent_id: parentContainerId,
            object_type: 'item',
            upnp_class: fileInfo.upnpClass,
            title: this.cleanTitle(fileInfo.fileName),
            file_path: fullPath,
            file_size: stats.size,
            mime_type: fileInfo.mimeType,
            duration_seconds: null,
            resolution: null,
            bitrate: null,
            album: this.extractAlbum(fullPath, rootPaths),
            artist: this.extractArtist(fullPath, rootPaths),
            genre: null,
            thumbnail_path: null,
            sort_order: 0,
          });

          if (existing) {
            result.updatedFiles++;
          } else {
            result.newFiles++;
          }
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          logger.warn('Failed to index file', { path: fullPath, error: message });
          result.errors.push(`Failed to index ${fullPath}: ${message}`);
        }
      }
    }
  }

  /**
   * Ensure a folder container exists in the database.
   * Creates intermediate containers as needed.
   * Returns the container ID for the given directory.
   */
  private async ensureFolderContainer(
    dirPath: string,
    rootPaths: string[],
    rootContainerId: string,
    category: string,
    folderContainers: Map<string, string>
  ): Promise<string> {
    // Check if already created this session
    const cacheKey = `${category}:${dirPath}`;
    const cached = folderContainers.get(cacheKey);
    if (cached) return cached;

    // If dirPath is one of the root media paths, return the root container directly
    const isRootPath = rootPaths.some(rp => {
      const normalizedDir = path.resolve(dirPath);
      const normalizedRoot = path.resolve(rp);
      return normalizedDir === normalizedRoot;
    });

    if (isRootPath) {
      folderContainers.set(cacheKey, rootContainerId);
      return rootContainerId;
    }

    // Recursively ensure parent folder container exists
    const parentDir = path.dirname(dirPath);
    const parentContainerId = await this.ensureFolderContainer(
      parentDir,
      rootPaths,
      rootContainerId,
      category,
      folderContainers
    );

    // Create this folder's container
    const folderName = path.basename(dirPath);
    const containerId = await this.db.upsertMediaItem({
      source_account_id: this.sourceAccountId,
      parent_id: parentContainerId,
      object_type: 'container',
      upnp_class: UPnPClass.CONTAINER_STORAGE,
      title: folderName,
      file_path: null,
      file_size: null,
      mime_type: null,
      duration_seconds: null,
      resolution: null,
      bitrate: null,
      album: null,
      artist: null,
      genre: null,
      thumbnail_path: null,
      sort_order: 0,
    });

    folderContainers.set(cacheKey, containerId);
    return containerId;
  }

  /**
   * Get media file information from a file path.
   * Returns null if the file is not a recognized media type.
   */
  private getMediaFileInfo(filePath: string): MediaFileInfo | null {
    const ext = path.extname(filePath).toLowerCase();
    const mimeType = MIME_TYPES[ext];

    if (!mimeType) return null;

    return {
      filePath,
      fileName: path.basename(filePath, ext),
      fileSize: 0,
      mimeType,
      upnpClass: getUpnpClassForMime(mimeType),
      modifiedAt: new Date(),
    };
  }

  /**
   * Clean a file name for display as a title.
   * Removes common patterns like year tags, quality markers.
   */
  private cleanTitle(fileName: string): string {
    return fileName
      .replace(/\./g, ' ')
      .replace(/_/g, ' ')
      .replace(/\s+/g, ' ')
      .trim();
  }

  /**
   * Extract album name from directory structure.
   * Uses the parent directory name for audio files.
   */
  private extractAlbum(filePath: string, rootPaths: string[]): string | null {
    const ext = path.extname(filePath).toLowerCase();
    const mimeType = MIME_TYPES[ext];
    if (!mimeType || !mimeType.startsWith('audio/')) return null;

    const dir = path.dirname(filePath);
    // Don't use the root media path as album name
    if (rootPaths.some(rp => path.resolve(dir) === path.resolve(rp))) {
      return null;
    }

    return path.basename(dir);
  }

  /**
   * Extract artist name from directory structure.
   * Uses the grandparent directory name for audio files (Artist/Album/Track pattern).
   */
  private extractArtist(filePath: string, rootPaths: string[]): string | null {
    const ext = path.extname(filePath).toLowerCase();
    const mimeType = MIME_TYPES[ext];
    if (!mimeType || !mimeType.startsWith('audio/')) return null;

    const dir = path.dirname(filePath);
    const grandparent = path.dirname(dir);

    // Don't use the root media path as artist name
    if (rootPaths.some(rp => path.resolve(grandparent) === path.resolve(rp))) {
      return null;
    }

    // Check that grandparent is still within a root path
    const isWithinRootPath = rootPaths.some(rp => {
      const normalizedGrandparent = path.resolve(grandparent);
      const normalizedRoot = path.resolve(rp);
      return normalizedGrandparent.startsWith(normalizedRoot);
    });

    if (!isWithinRootPath) return null;

    return path.basename(grandparent);
  }
}
