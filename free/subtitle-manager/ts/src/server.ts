import fs from 'fs/promises';
import path from 'path';
import Fastify, { FastifyRequest, FastifyReply } from 'fastify';
import cors from '@fastify/cors';
import {
  createLogger,
  ApiRateLimiter,
  createAuthHook,
  createRateLimitHook,
  getAppContext,
  loadSecurityConfig,
} from '@nself/plugin-utils';
import { SubtitleManagerDatabase } from './database.js';
import { OpenSubtitlesClient, type OpenSubtitlesSearchResult } from './opensubtitles-client.js';
import { SubtitleSynchronizer } from './sync.js';
import { SubtitleQC } from './qc.js';
import { SubtitleNormalizer } from './normalize.js';
import type { SubtitleManagerConfig } from './types.js';

const logger = createLogger('subtitle-manager:server');

export class SubtitleManagerServer {
  private fastify: ReturnType<typeof Fastify>;
  private database: SubtitleManagerDatabase;
  private opensubtitles: OpenSubtitlesClient;
  private synchronizer: SubtitleSynchronizer;
  private qc: SubtitleQC;
  private normalizer: SubtitleNormalizer;
  private config: SubtitleManagerConfig;

  constructor(config: SubtitleManagerConfig, database: SubtitleManagerDatabase) {
    this.config = config;
    this.database = database;
    this.opensubtitles = new OpenSubtitlesClient(config.opensubtitles_api_key);
    this.synchronizer = new SubtitleSynchronizer(config);
    this.qc = new SubtitleQC();
    this.normalizer = new SubtitleNormalizer();
    this.fastify = Fastify({ logger: false });
  }

  async initialize(): Promise<void> {
    await this.fastify.register(cors);

    // Auth and rate-limit hooks from plugin-utils
    const securityConfig = loadSecurityConfig('SUBTITLE_MANAGER');
    const rateLimiter = new ApiRateLimiter(
      securityConfig.rateLimitMax ?? 100,
      securityConfig.rateLimitWindowMs ?? 60000,
    );

    this.fastify.addHook('preHandler', createAuthHook(securityConfig.apiKey));
    this.fastify.addHook('preHandler', createRateLimitHook(rateLimiter));

    this.registerRoutes();
    logger.info('Server initialized');
  }

  private registerRoutes(): void {
    // -------------------------------------------------------------------------
    // Health check
    // -------------------------------------------------------------------------
    this.fastify.get('/health', async () => ({
      status: 'ok',
      plugin: 'subtitle-manager',
      version: '1.0.0',
    }));

    // -------------------------------------------------------------------------
    // GET /v1/subtitles - Search locally stored subtitles
    // -------------------------------------------------------------------------
    this.fastify.get('/v1/subtitles', async (request: FastifyRequest<{ Querystring: { media_id?: string; language?: string } }>) => {
      const { media_id, language } = request.query;
      if (!media_id) {
        return { error: 'media_id query parameter is required' };
      }
      const { sourceAccountId } = getAppContext(request);
      const subtitles = await this.database.searchSubtitles(
        media_id,
        language || 'en',
        sourceAccountId,
      );
      return { subtitles };
    });

    // -------------------------------------------------------------------------
    // GET /v1/downloads - List downloaded subtitles
    // -------------------------------------------------------------------------
    this.fastify.get('/v1/downloads', async (request: FastifyRequest<{ Querystring: { limit?: string; offset?: string } }>) => {
      const { limit, offset } = request.query;
      const { sourceAccountId } = getAppContext(request);
      const result = await this.database.listDownloads(
        sourceAccountId,
        limit ? parseInt(limit, 10) : 50,
        offset ? parseInt(offset, 10) : 0,
      );
      return result;
    });

    // -------------------------------------------------------------------------
    // GET /v1/stats - Get subtitle stats
    // -------------------------------------------------------------------------
    this.fastify.get('/v1/stats', async (request: FastifyRequest) => {
      const { sourceAccountId } = getAppContext(request);
      const stats = await this.database.getStats(sourceAccountId);
      return { stats };
    });

    // -------------------------------------------------------------------------
    // POST /v1/search - Search OpenSubtitles by text query
    // -------------------------------------------------------------------------
    this.fastify.post('/v1/search', {
      schema: {
        body: {
          type: 'object',
          required: ['query'],
          properties: {
            query: { type: 'string', minLength: 1 },
            languages: {
              type: 'array',
              items: { type: 'string', minLength: 2, maxLength: 5 },
              default: ['en'],
            },
          },
          additionalProperties: false,
        },
      },
    }, async (request: FastifyRequest<{ Body: { query: string; languages?: string[] } }>) => {
      const { query, languages } = request.body;
      const results = await this.opensubtitles.searchByQuery(query, languages || ['en']);
      return { results, count: results.length };
    });

    // -------------------------------------------------------------------------
    // POST /v1/search/hash - Search OpenSubtitles by file hash
    // -------------------------------------------------------------------------
    this.fastify.post('/v1/search/hash', {
      schema: {
        body: {
          type: 'object',
          required: ['moviehash', 'moviebytesize'],
          properties: {
            moviehash: { type: 'string', minLength: 1 },
            moviebytesize: { type: 'number', minimum: 1 },
            languages: {
              type: 'array',
              items: { type: 'string', minLength: 2, maxLength: 5 },
              default: ['en'],
            },
          },
          additionalProperties: false,
        },
      },
    }, async (request: FastifyRequest<{ Body: { moviehash: string; moviebytesize: number; languages?: string[] } }>) => {
      const { moviehash, moviebytesize, languages } = request.body;
      const results = await this.opensubtitles.searchByHash(
        moviehash,
        moviebytesize,
        languages || ['en'],
      );
      return { results, count: results.length };
    });

    // -------------------------------------------------------------------------
    // POST /v1/download - Download a subtitle file and save to disk
    // -------------------------------------------------------------------------
    this.fastify.post('/v1/download', {
      schema: {
        body: {
          type: 'object',
          required: ['file_id', 'media_id'],
          properties: {
            file_id: { type: 'number', minimum: 1 },
            media_id: { type: 'string', minLength: 1 },
            media_type: { type: 'string', enum: ['movie', 'tv_episode'], default: 'movie' },
            media_title: { type: 'string' },
            language: { type: 'string', minLength: 2, maxLength: 5, default: 'en' },
            run_qc: { type: 'boolean', default: false },
          },
          additionalProperties: false,
        },
      },
    }, async (request: FastifyRequest<{ Body: { file_id: number; media_id: string; media_type?: string; media_title?: string; language?: string; run_qc?: boolean } }>, reply: FastifyReply) => {
      const { file_id, media_id, media_type, media_title, language, run_qc } = request.body;
      const { sourceAccountId } = getAppContext(request);
      const lang = language || 'en';

      // Check if already downloaded
      const existing = await this.database.getDownloadByMediaId(media_id, lang, sourceAccountId);
      if (existing) {
        return { success: true, download: existing, source: 'cache' };
      }

      // Download from OpenSubtitles
      const subtitleBuffer = await this.opensubtitles.downloadSubtitle(file_id);
      if (!subtitleBuffer) {
        reply.code(404).send({ error: 'Subtitle not found or download failed' });
        return;
      }

      // Save to disk
      const dir = path.join(this.config.subtitle_storage_path, sourceAccountId, media_id);
      await fs.mkdir(dir, { recursive: true });
      const filePath = path.join(dir, `${lang}.srt`);
      await fs.writeFile(filePath, subtitleBuffer);

      logger.info('Subtitle saved to disk', { filePath, bytes: subtitleBuffer.length });

      // Track in database
      const download = await this.database.insertDownload({
        source_account_id: sourceAccountId,
        media_id,
        media_type: media_type || 'movie',
        media_title: media_title || '',
        language: lang,
        file_path: filePath,
        file_size_bytes: subtitleBuffer.length,
        opensubtitles_file_id: file_id,
        source: 'opensubtitles',
      });

      // Optionally run QC after download
      let qcResult = undefined;
      if (run_qc) {
        try {
          const result = await this.qc.validateSubtitle(filePath);
          await this.database.insertQCResult({
            source_account_id: sourceAccountId,
            download_id: download.id,
            status: result.status,
            checks: result.checks,
            issues: result.issues,
            cue_count: result.cueCount,
            total_duration_ms: result.totalDurationMs,
          });
          await this.database.updateDownloadQC(download.id, result.status, {
            cueCount: result.cueCount,
            issueCount: result.issues.length,
          });
          qcResult = result;
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          logger.warn('QC validation after download failed', { error: message });
        }
      }

      return { success: true, download, source: 'opensubtitles', qc: qcResult };
    });

    // -------------------------------------------------------------------------
    // POST /v1/sync - Synchronize subtitle timing with video
    // -------------------------------------------------------------------------
    this.fastify.post('/v1/sync', {
      schema: {
        body: {
          type: 'object',
          required: ['video_path', 'subtitle_path'],
          properties: {
            video_path: { type: 'string', minLength: 1 },
            subtitle_path: { type: 'string', minLength: 1 },
            language: { type: 'string', minLength: 2, maxLength: 5, default: 'en' },
          },
          additionalProperties: false,
        },
      },
    }, async (request: FastifyRequest<{ Body: { video_path: string; subtitle_path: string; language?: string } }>, reply: FastifyReply) => {
      const { video_path, subtitle_path, language } = request.body;
      const lang = language || 'en';
      const { sourceAccountId } = getAppContext(request);

      try {
        const outputDir = path.join(this.config.subtitle_storage_path, sourceAccountId, 'synced');
        const baseName = path.basename(subtitle_path, path.extname(subtitle_path));
        const outputPath = path.join(outputDir, `${baseName}.synced.${lang}.srt`);

        const result = await this.synchronizer.syncSubtitle(video_path, subtitle_path, outputPath);
        return { success: true, result };
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        logger.error('Sync failed', { error: message });
        reply.code(500).send({ error: `Sync failed: ${message}` });
      }
    });

    // -------------------------------------------------------------------------
    // POST /v1/qc - Validate a subtitle file with QC checks
    // -------------------------------------------------------------------------
    this.fastify.post('/v1/qc', {
      schema: {
        body: {
          type: 'object',
          required: ['subtitle_path'],
          properties: {
            subtitle_path: { type: 'string', minLength: 1 },
            video_duration_ms: { type: 'number', minimum: 0 },
            download_id: { type: 'string' },
          },
          additionalProperties: false,
        },
      },
    }, async (request: FastifyRequest<{ Body: { subtitle_path: string; video_duration_ms?: number; download_id?: string } }>, reply: FastifyReply) => {
      const { subtitle_path, video_duration_ms, download_id } = request.body;
      const { sourceAccountId } = getAppContext(request);

      try {
        const result = await this.qc.validateSubtitle(subtitle_path, video_duration_ms);

        // If download_id is provided, store result and update download record
        if (download_id) {
          await this.database.insertQCResult({
            source_account_id: sourceAccountId,
            download_id,
            status: result.status,
            checks: result.checks,
            issues: result.issues,
            cue_count: result.cueCount,
            total_duration_ms: result.totalDurationMs,
          });
          await this.database.updateDownloadQC(download_id, result.status, {
            cueCount: result.cueCount,
            issueCount: result.issues.length,
          });
        }

        return { success: true, result };
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        logger.error('QC validation failed', { error: message });
        reply.code(500).send({ error: `QC validation failed: ${message}` });
      }
    });

    // -------------------------------------------------------------------------
    // POST /v1/normalize - Convert subtitle to WebVTT
    // -------------------------------------------------------------------------
    this.fastify.post('/v1/normalize', {
      schema: {
        body: {
          type: 'object',
          required: ['input_path'],
          properties: {
            input_path: { type: 'string', minLength: 1 },
            output_format: { type: 'string', enum: ['vtt'], default: 'vtt' },
          },
          additionalProperties: false,
        },
      },
    }, async (request: FastifyRequest<{ Body: { input_path: string; output_format?: string } }>, reply: FastifyReply) => {
      const { input_path } = request.body;

      try {
        const outputPath = await this.normalizer.normalizeToWebVTT(input_path);
        return { success: true, output_path: outputPath };
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        logger.error('Normalization failed', { error: message });
        reply.code(500).send({ error: `Normalization failed: ${message}` });
      }
    });

    // -------------------------------------------------------------------------
    // POST /v1/fetch-best - Full cascade: search + download + sync + convert
    // This is the primary endpoint nself-tv uses. It implements the full
    // subtitle cascade and NEVER blocks the content pipeline.
    // -------------------------------------------------------------------------
    this.fastify.post('/v1/fetch-best', {
      schema: {
        body: {
          type: 'object',
          required: ['video_path', 'languages'],
          properties: {
            video_path: { type: 'string', minLength: 1 },
            languages: {
              type: 'array',
              items: { type: 'string', minLength: 2, maxLength: 5 },
              minItems: 1,
            },
            max_alternatives: { type: 'number', minimum: 1, maximum: 10, default: 3 },
            media_id: { type: 'string' },
            media_type: { type: 'string', enum: ['movie', 'tv_episode'], default: 'movie' },
            media_title: { type: 'string' },
          },
          additionalProperties: false,
        },
      },
    }, async (request: FastifyRequest<{ Body: {
      video_path: string;
      languages: string[];
      max_alternatives?: number;
      media_id?: string;
      media_type?: string;
      media_title?: string;
    } }>) => {
      const { video_path, languages, max_alternatives, media_id, media_type, media_title } = request.body;
      const { sourceAccountId } = getAppContext(request);
      const maxAlts = max_alternatives || 3;

      const results: Array<{
        language: string;
        path: string | null;
        format: string;
        sync_quality: 'good' | 'warning' | 'failed';
        sync_warning: boolean;
        offset_ms: number;
        tool_used: string;
      }> = [];

      // Process each language in parallel
      const languagePromises = languages.map(async (lang) => {
        try {
          return await this.fetchBestForLanguage({
            videoPath: video_path,
            language: lang,
            maxAlternatives: maxAlts,
            sourceAccountId,
            mediaId: media_id,
            mediaType: media_type || 'movie',
            mediaTitle: media_title,
          });
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          logger.warn(`Fetch-best failed for language ${lang}`, { error: message });
          return {
            language: lang,
            path: null,
            format: 'none',
            sync_quality: 'failed' as const,
            sync_warning: true,
            offset_ms: 0,
            tool_used: 'none',
          };
        }
      });

      const langResults = await Promise.all(languagePromises);
      results.push(...langResults);

      return {
        success: true,
        subtitles: results,
        languages_requested: languages.length,
        languages_found: results.filter(r => r.path !== null).length,
      };
    });

    // -------------------------------------------------------------------------
    // DELETE /v1/downloads/:id - Delete a download record
    // -------------------------------------------------------------------------
    this.fastify.delete('/v1/downloads/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const { id } = request.params;
      const deleted = await this.database.deleteDownload(id);
      if (!deleted) {
        reply.code(404).send({ error: 'Download not found' });
        return;
      }
      return { success: true };
    });
  }

  // ---------------------------------------------------------------------------
  // Fetch-best cascade logic (private)
  // ---------------------------------------------------------------------------

  private async fetchBestForLanguage(params: {
    videoPath: string;
    language: string;
    maxAlternatives: number;
    sourceAccountId: string;
    mediaId?: string;
    mediaType: string;
    mediaTitle?: string;
  }): Promise<{
    language: string;
    path: string | null;
    format: string;
    sync_quality: 'good' | 'warning' | 'failed';
    sync_warning: boolean;
    offset_ms: number;
    tool_used: string;
  }> {
    const { videoPath, language, maxAlternatives, sourceAccountId, mediaId, mediaType, mediaTitle } = params;
    const SYNC_THRESHOLD_MS = 500;

    // Step 1: Search for subtitles (by query since hash search requires moviehash)
    const searchQuery = mediaTitle || path.basename(videoPath, path.extname(videoPath));
    let searchResults = await this.opensubtitles.searchByQuery(searchQuery, [language]);

    if (!searchResults || searchResults.length === 0) {
      logger.info(`No subtitles found for language ${language}`, { query: searchQuery });
      return {
        language,
        path: null,
        format: 'none',
        sync_quality: 'failed',
        sync_warning: true,
        offset_ms: 0,
        tool_used: 'none',
      };
    }

    // Rank results by downloads and rating
    searchResults = searchResults
      .filter((r: OpenSubtitlesSearchResult) => r.attributes?.files?.length > 0)
      .sort((a: OpenSubtitlesSearchResult, b: OpenSubtitlesSearchResult) => {
        const aScore = (a.attributes?.download_count || 0) * 0.4 + (a.attributes?.ratings || 0) * 0.3;
        const bScore = (b.attributes?.download_count || 0) * 0.4 + (b.attributes?.ratings || 0) * 0.3;
        return bScore - aScore;
      })
      .slice(0, maxAlternatives);

    let bestResult: {
      path: string;
      offset_ms: number;
      tool_used: string;
      sync_quality: 'good' | 'warning';
    } | null = null;
    let bestOffsetMs = Infinity;

    // Step 2-5: Try each alternative subtitle
    for (const result of searchResults) {
      const fileId = result.attributes?.files?.[0]?.file_id;
      if (!fileId) continue;

      try {
        // Download subtitle
        const subtitleBuffer = await this.opensubtitles.downloadSubtitle(fileId);
        if (!subtitleBuffer) continue;

        // Save raw subtitle to temp location
        const dir = path.join(this.config.subtitle_storage_path, sourceAccountId, mediaId || 'temp', language);
        await fs.mkdir(dir, { recursive: true });
        const rawPath = path.join(dir, `raw_${fileId}.srt`);
        await fs.writeFile(rawPath, subtitleBuffer);

        // Try sync with alass first
        const syncedPath = path.join(dir, `synced_${fileId}.srt`);
        let offsetMs = 0;
        let toolUsed = 'none';

        try {
          const syncResult = await this.synchronizer.syncSubtitle(
            videoPath, rawPath, syncedPath, { alassOnly: true }
          );
          offsetMs = Math.abs(syncResult.offsetMs);
          toolUsed = 'alass';

          // If offset > threshold, try ffsubsync
          if (offsetMs > SYNC_THRESHOLD_MS) {
            try {
              const ffResult = await this.synchronizer.syncSubtitle(
                videoPath, rawPath, syncedPath, { ffsubsyncOnly: true }
              );
              const ffOffset = Math.abs(ffResult.offsetMs);
              if (ffOffset < offsetMs) {
                offsetMs = ffOffset;
                toolUsed = 'ffsubsync';
              }
            } catch {
              // ffsubsync failed, keep alass result
            }
          }
        } catch {
          // Both sync tools failed, use raw file
          await fs.copyFile(rawPath, syncedPath).catch(() => {});
          toolUsed = 'raw';
        }

        // Track best result
        if (offsetMs < bestOffsetMs) {
          bestOffsetMs = offsetMs;
          bestResult = {
            path: syncedPath,
            offset_ms: offsetMs,
            tool_used: toolUsed,
            sync_quality: offsetMs <= SYNC_THRESHOLD_MS ? 'good' : 'warning',
          };
        }

        // If good sync achieved, stop trying alternatives
        if (offsetMs <= SYNC_THRESHOLD_MS) break;
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        logger.warn(`Alternative subtitle ${fileId} failed`, { error: message });
        continue;
      }
    }

    if (!bestResult) {
      return {
        language,
        path: null,
        format: 'none',
        sync_quality: 'failed',
        sync_warning: true,
        offset_ms: 0,
        tool_used: 'none',
      };
    }

    // Step 6: Normalize to WebVTT
    let vttPath: string | null = null;
    try {
      vttPath = await this.normalizer.normalizeToWebVTT(bestResult.path);
    } catch {
      // Normalization failed, use SRT
      vttPath = bestResult.path;
    }

    // Track download in database
    if (mediaId) {
      try {
        await this.database.insertDownload({
          source_account_id: sourceAccountId,
          media_id: mediaId,
          media_type: mediaType,
          media_title: mediaTitle || '',
          language,
          file_path: vttPath || bestResult.path,
          file_size_bytes: 0,
          source: 'opensubtitles',
          sync_score: bestResult.sync_quality === 'good' ? 1.0 : 0.5,
        });
      } catch {
        // DB tracking failure is non-critical
      }
    }

    return {
      language,
      path: vttPath,
      format: vttPath?.endsWith('.vtt') ? 'webvtt' : 'srt',
      sync_quality: bestResult.sync_quality,
      sync_warning: bestResult.sync_quality !== 'good',
      offset_ms: bestResult.offset_ms,
      tool_used: bestResult.tool_used,
    };
  }

  async start(): Promise<void> {
    await this.fastify.listen({ port: this.config.port, host: '0.0.0.0' });
    logger.info(`Server listening on port ${this.config.port}`);
  }

  async stop(): Promise<void> {
    await this.fastify.close();
  }
}
