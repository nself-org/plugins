/**
 * Pipeline Orchestrator
 *
 * Connects the 7 media plugins in sequence:
 *   detect -> VPN check -> torrent download -> metadata enrich -> subtitle fetch
 *          -> encoding -> publishing
 *
 * Each stage updates the pipeline run record in the database and handles failures
 * gracefully -- optional plugins (metadata, subtitles, encoding, publishing) are
 * skipped rather than failing the entire pipeline.
 */

import axios from 'axios';
import { createLogger } from '@nself/plugin-utils';
import type { ContentAcquisitionDatabase } from './database.js';
import type { ContentAcquisitionConfig } from './types.js';

const logger = createLogger('content-acquisition:pipeline');

/** Default timeout for HTTP calls to sibling plugins (30 seconds). */
const HTTP_TIMEOUT = 30_000;

/** Poll interval when waiting for a torrent download to complete (30 seconds). */
const POLL_INTERVAL_MS = 30_000;

/** Maximum number of polls before giving up (720 = 6 hours at 30 s intervals). */
const MAX_POLLS = 720;

/** Maximum number of polls for encoding jobs (2880 = 24 hours at 30 s intervals). */
const MAX_ENCODING_POLLS = 2880;

export class PipelineOrchestrator {
  private db: ContentAcquisitionDatabase;
  private config: ContentAcquisitionConfig;

  constructor(db: ContentAcquisitionDatabase, config: ContentAcquisitionConfig) {
    this.db = db;
    this.config = config;
  }

  // ==========================================================================
  // Main pipeline execution
  // ==========================================================================

  /**
   * Run the full pipeline for a given pipeline run ID.
   *
   * The stages execute in order:
   *   1. VPN check
   *   2. Torrent submit
   *   3. Poll for download completion
   *   4. Metadata enrichment
   *   5. Subtitle fetch
   *   6. Encoding (media processing)
   *   7. Publishing (nTV backend)
   *   8. Mark pipeline complete
   *
   * If a stage fails the pipeline is halted and the error is recorded.
   */
  async executePipeline(pipelineId: number): Promise<void> {
    const run = await this.db.getPipelineRun(pipelineId);
    if (!run) {
      logger.error(`Pipeline run ${pipelineId} not found`);
      return;
    }

    logger.info(`Starting pipeline ${pipelineId} for "${run.content_title}"`);

    try {
      // Stage 1 -- VPN check
      await this.db.updatePipelineRun(pipelineId, { status: 'vpn_checking' });
      const vpnOk = await this.checkVpn(pipelineId);
      if (!vpnOk) {
        // VPN is down -- pause the pipeline; operator must retry later
        await this.db.updatePipelineRun(pipelineId, { status: 'vpn_waiting' });
        logger.warn(`Pipeline ${pipelineId} paused: VPN is not active`);
        return;
      }

      // Stage 2 -- Torrent submit
      await this.db.updatePipelineRun(pipelineId, { status: 'torrent_submitting' });
      const magnetUrl = (run.metadata as Record<string, unknown>)?.magnet_url as string | undefined;
      const torrentUrl = (run.metadata as Record<string, unknown>)?.torrent_url as string | undefined;
      const downloadUrl = magnetUrl || torrentUrl;

      if (!downloadUrl) {
        await this.db.updatePipelineRun(pipelineId, {
          status: 'failed',
          torrent_status: 'failed',
          error_message: 'No magnet or torrent URL available',
        });
        logger.error(`Pipeline ${pipelineId} failed: no download URL`);
        return;
      }

      const downloadId = await this.submitTorrent(pipelineId, downloadUrl);
      if (!downloadId) {
        await this.db.updatePipelineRun(pipelineId, { status: 'failed' });
        return;
      }

      // Stage 3 -- Poll for download completion
      await this.db.updatePipelineRun(pipelineId, { status: 'downloading' });
      const downloadComplete = await this.pollDownloadStatus(pipelineId, downloadId);
      if (!downloadComplete) {
        await this.db.updatePipelineRun(pipelineId, { status: 'failed' });
        return;
      }

      // Stage 4 -- Metadata enrichment (optional -- graceful degradation)
      await this.db.updatePipelineRun(pipelineId, { status: 'enriching_metadata' });
      await this.enrichMetadata(pipelineId, run.content_title, run.content_type ?? undefined);

      // Stage 5 -- Subtitle fetch (optional -- graceful degradation)
      await this.db.updatePipelineRun(pipelineId, { status: 'fetching_subtitles' });
      await this.fetchSubtitles(pipelineId, run.content_title);

      // Stage 6 -- Encoding (optional -- graceful degradation)
      await this.db.updatePipelineRun(pipelineId, { status: 'encoding' });
      // Retrieve download path from the torrent download record
      const refreshedRun = await this.db.getPipelineRun(pipelineId);
      const downloadPath = (refreshedRun?.metadata as Record<string, unknown>)?.download_path as string | undefined;
      await this.encodeContent(pipelineId, downloadPath, refreshedRun?.metadata ?? {});

      // Stage 7 -- Publishing (optional -- graceful degradation)
      await this.db.updatePipelineRun(pipelineId, { status: 'publishing' });
      const runAfterEncoding = await this.db.getPipelineRun(pipelineId);
      await this.publishContent(
        pipelineId,
        runAfterEncoding?.encoding_job_id ?? null,
        runAfterEncoding?.metadata ?? {},
      );

      // All stages done
      await this.db.updatePipelineRun(pipelineId, {
        status: 'completed',
        pipeline_completed_at: new Date(),
      });
      logger.info(`Pipeline ${pipelineId} completed successfully for "${run.content_title}"`);

    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      await this.db.updatePipelineRun(pipelineId, {
        status: 'failed',
        error_message: message,
      });
      logger.error(`Pipeline ${pipelineId} failed: ${message}`);
    }
  }

  // ==========================================================================
  // Individual stage methods
  // ==========================================================================

  /**
   * Stage 1 -- Check that the VPN is active before proceeding.
   *
   * Returns `true` only when the VPN plugin confirms the VPN is connected.
   *
   * Returns `false` when the VPN plugin is unreachable or reports VPN is down.
   * This ensures torrent downloads NEVER proceed without a verified VPN connection.
   */
  async checkVpn(pipelineId: number): Promise<boolean> {
    try {
      const response = await axios.get(`${this.config.vpn_manager_url}/api/status`, {
        timeout: HTTP_TIMEOUT,
      });

      const isActive = response.data?.active === true || response.data?.status === 'connected';

      if (isActive) {
        await this.db.updatePipelineRun(pipelineId, {
          vpn_check_status: 'passed',
          vpn_checked_at: new Date(),
        });
        logger.info(`Pipeline ${pipelineId}: VPN check passed`);
        return true;
      }

      // VPN is explicitly not active — block the download
      await this.db.updatePipelineRun(pipelineId, {
        vpn_check_status: 'failed',
        vpn_checked_at: new Date(),
        error_message: 'VPN is not active',
      });
      logger.error(`Pipeline ${pipelineId}: VPN is not active — blocking download`);
      return false;

    } catch (error) {
      // VPN plugin unreachable — BLOCK the download (never download without verified VPN)
      const message = error instanceof Error ? error.message : String(error);
      await this.db.updatePipelineRun(pipelineId, {
        vpn_check_status: 'failed',
        vpn_checked_at: new Date(),
        error_message: `VPN plugin unreachable: ${message}`,
      });
      logger.error(`Pipeline ${pipelineId}: VPN plugin unreachable — blocking download`, { error: message });
      return false;
    }
  }

  /**
   * Stage 2 -- Submit a magnet/torrent URL to the torrent manager.
   *
   * Returns the download ID on success, or `null` on failure.
   */
  async submitTorrent(pipelineId: number, magnetUrl: string): Promise<string | null> {
    try {
      const response = await axios.post(
        `${this.config.torrent_manager_url}/api/downloads`,
        { url: magnetUrl },
        { timeout: HTTP_TIMEOUT },
      );

      const downloadId: string = response.data?.id ?? response.data?.download_id;
      if (!downloadId) {
        await this.db.updatePipelineRun(pipelineId, {
          torrent_status: 'failed',
          error_message: 'Torrent manager returned no download ID',
        });
        logger.error(`Pipeline ${pipelineId}: torrent submit returned no download ID`);
        return null;
      }

      await this.db.updatePipelineRun(pipelineId, {
        torrent_status: 'downloading',
        torrent_download_id: downloadId,
        torrent_submitted_at: new Date(),
      });
      logger.info(`Pipeline ${pipelineId}: torrent submitted, download ID = ${downloadId}`);
      return downloadId;

    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      await this.db.updatePipelineRun(pipelineId, {
        torrent_status: 'failed',
        error_message: `Torrent submit failed: ${message}`,
      });
      logger.error(`Pipeline ${pipelineId}: torrent submit failed: ${message}`);
      return null;
    }
  }

  /**
   * Stage 3 -- Poll the torrent manager until the download completes or times out.
   *
   * Uses 30-second intervals with a maximum of 720 polls (~6 hours).
   * Returns `true` when the download completes, `false` on timeout or failure.
   */
  async pollDownloadStatus(pipelineId: number, downloadId: string): Promise<boolean> {
    for (let attempt = 0; attempt < MAX_POLLS; attempt++) {
      try {
        const response = await axios.get(
          `${this.config.torrent_manager_url}/api/downloads/${downloadId}`,
          { timeout: HTTP_TIMEOUT },
        );

        const status: string = response.data?.status ?? '';

        if (status === 'completed' || status === 'seeding') {
          await this.db.updatePipelineRun(pipelineId, {
            torrent_status: 'completed',
            download_completed_at: new Date(),
          });
          logger.info(`Pipeline ${pipelineId}: download completed (${downloadId})`);
          return true;
        }

        if (status === 'error' || status === 'failed') {
          await this.db.updatePipelineRun(pipelineId, {
            torrent_status: 'failed',
            error_message: `Download failed with status: ${status}`,
          });
          logger.error(`Pipeline ${pipelineId}: download ${downloadId} failed`);
          return false;
        }

        // Still in progress -- wait before polling again
        logger.debug(`Pipeline ${pipelineId}: download ${downloadId} status = ${status}, poll ${attempt + 1}/${MAX_POLLS}`);
        await this.sleep(POLL_INTERVAL_MS);

      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        logger.warn(`Pipeline ${pipelineId}: poll error (attempt ${attempt + 1}): ${message}`);
        // Transient errors: keep trying
        await this.sleep(POLL_INTERVAL_MS);
      }
    }

    // Timed out
    await this.db.updatePipelineRun(pipelineId, {
      torrent_status: 'failed',
      error_message: 'Download timed out after maximum poll attempts',
    });
    logger.error(`Pipeline ${pipelineId}: download ${downloadId} timed out`);
    return false;
  }

  /**
   * Stage 4 -- Enrich metadata via the metadata enrichment plugin.
   *
   * Graceful degradation: if the plugin is unreachable the stage is marked as
   * "skipped" and the pipeline continues.
   *
   * Returns `true` if enrichment succeeded, `false` otherwise.
   */
  async enrichMetadata(pipelineId: number, contentTitle: string, contentType?: string): Promise<boolean> {
    try {
      await axios.post(
        `${this.config.metadata_enrichment_url}/api/enrich`,
        { title: contentTitle, type: contentType },
        { timeout: HTTP_TIMEOUT },
      );

      await this.db.updatePipelineRun(pipelineId, {
        metadata_status: 'completed',
        metadata_enriched_at: new Date(),
      });
      logger.info(`Pipeline ${pipelineId}: metadata enrichment completed`);
      return true;

    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);

      if (axios.isAxiosError(error) && !error.response) {
        // Network error -- plugin unreachable
        await this.db.updatePipelineRun(pipelineId, {
          metadata_status: 'skipped',
          metadata_enriched_at: new Date(),
        });
        logger.warn(`Pipeline ${pipelineId}: metadata plugin unreachable (${message}), skipping`);
      } else {
        await this.db.updatePipelineRun(pipelineId, {
          metadata_status: 'failed',
          metadata_enriched_at: new Date(),
          error_message: `Metadata enrichment failed: ${message}`,
        });
        logger.error(`Pipeline ${pipelineId}: metadata enrichment failed: ${message}`);
      }
      return false;
    }
  }

  /**
   * Stage 5 -- Fetch subtitles via the subtitle manager plugin.
   *
   * Graceful degradation: if the plugin is unreachable the stage is marked as
   * "skipped" and the pipeline continues.
   *
   * Returns `true` if subtitle fetch succeeded, `false` otherwise.
   */
  async fetchSubtitles(pipelineId: number, contentTitle: string): Promise<boolean> {
    try {
      await axios.post(
        `${this.config.subtitle_manager_url}/api/search`,
        { title: contentTitle },
        { timeout: HTTP_TIMEOUT },
      );

      await this.db.updatePipelineRun(pipelineId, {
        subtitle_status: 'completed',
        subtitles_fetched_at: new Date(),
      });
      logger.info(`Pipeline ${pipelineId}: subtitles fetched`);
      return true;

    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);

      if (axios.isAxiosError(error) && !error.response) {
        // Network error -- plugin unreachable
        await this.db.updatePipelineRun(pipelineId, {
          subtitle_status: 'skipped',
          subtitles_fetched_at: new Date(),
        });
        logger.warn(`Pipeline ${pipelineId}: subtitle plugin unreachable (${message}), skipping`);
      } else {
        await this.db.updatePipelineRun(pipelineId, {
          subtitle_status: 'failed',
          subtitles_fetched_at: new Date(),
          error_message: `Subtitle fetch failed: ${message}`,
        });
        logger.error(`Pipeline ${pipelineId}: subtitle fetch failed: ${message}`);
      }
      return false;
    }
  }

  /**
   * Stage 6 -- Encode content via the media-processing plugin.
   *
   * Submits an encoding job and polls for completion (up to 24 hours).
   * Graceful degradation: if the media-processing URL is not configured or
   * the service is unreachable, the stage is marked as "skipped" and the
   * pipeline continues.
   *
   * Returns `true` if encoding succeeded or was skipped, `false` on failure.
   */
  async encodeContent(
    pipelineId: number,
    downloadPath: string | undefined,
    metadata: Record<string, unknown>,
  ): Promise<boolean> {
    // Graceful degradation: if media-processing URL is not configured, skip
    if (!this.config.media_processing_url) {
      await this.db.updatePipelineRun(pipelineId, {
        encoding_status: 'skipped',
        encoding_completed_at: new Date(),
      });
      logger.warn(`Pipeline ${pipelineId}: media-processing URL not configured, skipping encoding`);
      return true;
    }

    try {
      // Submit encoding job
      const profileId = (metadata.encoding_profile_id as string) ?? 'default';
      const response = await axios.post(
        `${this.config.media_processing_url}/v1/jobs`,
        {
          input_url: downloadPath ?? '',
          input_type: 'file',
          profile_id: profileId,
          priority: 5,
        },
        { timeout: HTTP_TIMEOUT },
      );

      const jobId: string = response.data?.id ?? response.data?.job_id;
      if (!jobId) {
        await this.db.updatePipelineRun(pipelineId, {
          encoding_status: 'failed',
          encoding_completed_at: new Date(),
          error_message: 'Media-processing returned no job ID',
        });
        logger.error(`Pipeline ${pipelineId}: encoding submit returned no job ID`);
        return false;
      }

      // Store the encoding job ID for later reference
      await this.db.updatePipelineRun(pipelineId, {
        encoding_status: 'encoding',
        encoding_job_id: jobId,
      });
      logger.info(`Pipeline ${pipelineId}: encoding job submitted, job ID = ${jobId}`);

      // Poll for encoding completion
      for (let attempt = 0; attempt < MAX_ENCODING_POLLS; attempt++) {
        try {
          const pollResponse = await axios.get(
            `${this.config.media_processing_url}/v1/jobs/${jobId}`,
            { timeout: HTTP_TIMEOUT },
          );

          const status: string = pollResponse.data?.status ?? '';

          if (status === 'completed') {
            await this.db.updatePipelineRun(pipelineId, {
              encoding_status: 'completed',
              encoding_completed_at: new Date(),
            });
            logger.info(`Pipeline ${pipelineId}: encoding completed (${jobId})`);
            return true;
          }

          if (status === 'failed' || status === 'error') {
            const errorDetail = pollResponse.data?.error ?? 'Unknown encoding error';
            await this.db.updatePipelineRun(pipelineId, {
              encoding_status: 'failed',
              encoding_completed_at: new Date(),
              error_message: `Encoding failed: ${errorDetail}`,
            });
            logger.error(`Pipeline ${pipelineId}: encoding job ${jobId} failed: ${errorDetail}`);
            return false;
          }

          // Still in progress -- wait before polling again
          logger.debug(`Pipeline ${pipelineId}: encoding job ${jobId} status = ${status}, poll ${attempt + 1}/${MAX_ENCODING_POLLS}`);
          await this.sleep(POLL_INTERVAL_MS);

        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          logger.warn(`Pipeline ${pipelineId}: encoding poll error (attempt ${attempt + 1}): ${message}`);
          // Transient errors: keep trying
          await this.sleep(POLL_INTERVAL_MS);
        }
      }

      // Timed out
      await this.db.updatePipelineRun(pipelineId, {
        encoding_status: 'failed',
        encoding_completed_at: new Date(),
        error_message: 'Encoding timed out after maximum poll attempts',
      });
      logger.error(`Pipeline ${pipelineId}: encoding job ${jobId} timed out`);
      return false;

    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);

      if (axios.isAxiosError(error) && !error.response) {
        // Network error -- plugin unreachable
        await this.db.updatePipelineRun(pipelineId, {
          encoding_status: 'skipped',
          encoding_completed_at: new Date(),
        });
        logger.warn(`Pipeline ${pipelineId}: media-processing plugin unreachable (${message}), skipping encoding`);
        return true;
      }

      await this.db.updatePipelineRun(pipelineId, {
        encoding_status: 'failed',
        encoding_completed_at: new Date(),
        error_message: `Encoding failed: ${message}`,
      });
      logger.error(`Pipeline ${pipelineId}: encoding failed: ${message}`);
      return false;
    }
  }

  /**
   * Stage 7 -- Publish content to the nTV backend.
   *
   * Fetches the completed encoding job outputs (HLS/DASH URLs, subtitle tracks)
   * from the media-processing service, then publishes the content to the nTV
   * backend library endpoint.
   *
   * Graceful degradation: if the nTV backend URL is not configured or the
   * service is unreachable, the stage is marked as "skipped" and the pipeline
   * continues.
   *
   * Returns `true` if publishing succeeded or was skipped, `false` on failure.
   */
  async publishContent(
    pipelineId: number,
    encodingJobId: string | null,
    metadata: Record<string, unknown>,
  ): Promise<boolean> {
    // Graceful degradation: if nTV backend URL is not configured, skip
    if (!this.config.ntv_backend_url) {
      await this.db.updatePipelineRun(pipelineId, {
        publishing_status: 'skipped',
        published_at: new Date(),
      });
      logger.warn(`Pipeline ${pipelineId}: nTV backend URL not configured, skipping publishing`);
      return true;
    }

    try {
      // Retrieve streaming outputs from the completed encoding job
      let hlsManifestUrl: string | undefined;
      let dashManifestUrl: string | undefined;
      let subtitleTracks: Array<{ language: string; url: string }> = [];

      if (encodingJobId && this.config.media_processing_url) {
        try {
          const jobResponse = await axios.get(
            `${this.config.media_processing_url}/v1/jobs/${encodingJobId}`,
            { timeout: HTTP_TIMEOUT },
          );

          const outputs = jobResponse.data?.outputs ?? jobResponse.data?.output ?? {};
          hlsManifestUrl = outputs.hls_manifest_url ?? outputs.hls_url;
          dashManifestUrl = outputs.dash_manifest_url ?? outputs.dash_mpd_url;
          subtitleTracks = outputs.subtitle_tracks ?? [];
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          logger.warn(`Pipeline ${pipelineId}: failed to fetch encoding job outputs (${message}), publishing with available metadata`);
        }
      }

      // Fetch the pipeline run to get content details
      const run = await this.db.getPipelineRun(pipelineId);
      if (!run) {
        await this.db.updatePipelineRun(pipelineId, {
          publishing_status: 'failed',
          published_at: new Date(),
          error_message: 'Pipeline run not found during publishing',
        });
        logger.error(`Pipeline ${pipelineId}: run not found during publishing`);
        return false;
      }

      // Publish to nTV backend
      await axios.post(
        `${this.config.ntv_backend_url}/api/library/publish`,
        {
          tmdb_id: metadata.tmdb_id ?? null,
          title: run.content_title,
          type: run.content_type ?? 'movie',
          hls_manifest_url: hlsManifestUrl ?? null,
          dash_manifest_url: dashManifestUrl ?? null,
          subtitle_tracks: subtitleTracks,
          metadata: metadata,
        },
        { timeout: HTTP_TIMEOUT },
      );

      await this.db.updatePipelineRun(pipelineId, {
        publishing_status: 'completed',
        published_at: new Date(),
      });
      logger.info(`Pipeline ${pipelineId}: content published to nTV backend`);
      return true;

    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);

      if (axios.isAxiosError(error) && !error.response) {
        // Network error -- nTV backend unreachable
        await this.db.updatePipelineRun(pipelineId, {
          publishing_status: 'skipped',
          published_at: new Date(),
        });
        logger.warn(`Pipeline ${pipelineId}: nTV backend unreachable (${message}), skipping publishing`);
        return true;
      }

      await this.db.updatePipelineRun(pipelineId, {
        publishing_status: 'failed',
        published_at: new Date(),
        error_message: `Publishing failed: ${message}`,
      });
      logger.error(`Pipeline ${pipelineId}: publishing failed: ${message}`);
      return false;
    }
  }

  // ==========================================================================
  // Retry support
  // ==========================================================================

  /**
   * Retry a failed pipeline from the stage that failed.
   *
   * Inspects the current run status and individual stage statuses to determine
   * the resume point, then re-executes from that stage onward.
   */
  async retryPipeline(pipelineId: number): Promise<void> {
    const run = await this.db.getPipelineRun(pipelineId);
    if (!run) {
      logger.error(`Pipeline run ${pipelineId} not found for retry`);
      return;
    }

    if (run.status === 'completed') {
      logger.info(`Pipeline ${pipelineId} already completed, nothing to retry`);
      return;
    }

    logger.info(`Retrying pipeline ${pipelineId} from failed stage`);

    // Clear the error so we can re-attempt
    await this.db.updatePipelineRun(pipelineId, { error_message: null, status: 'retrying' });

    try {
      // Re-run VPN check if it failed
      if (run.vpn_check_status === 'failed') {
        await this.db.updatePipelineRun(pipelineId, { status: 'vpn_checking' });
        const vpnOk = await this.checkVpn(pipelineId);
        if (!vpnOk) {
          await this.db.updatePipelineRun(pipelineId, { status: 'vpn_waiting' });
          return;
        }
      }

      // Re-submit torrent if it failed or was never submitted
      if (run.torrent_status === 'failed' || run.torrent_status === 'pending') {
        await this.db.updatePipelineRun(pipelineId, { status: 'torrent_submitting' });
        const magnetUrl = (run.metadata as Record<string, unknown>)?.magnet_url as string | undefined;
        const torrentUrl = (run.metadata as Record<string, unknown>)?.torrent_url as string | undefined;
        const downloadUrl = magnetUrl || torrentUrl;

        if (!downloadUrl) {
          await this.db.updatePipelineRun(pipelineId, {
            status: 'failed',
            torrent_status: 'failed',
            error_message: 'No magnet or torrent URL available',
          });
          return;
        }

        const downloadId = await this.submitTorrent(pipelineId, downloadUrl);
        if (!downloadId) {
          await this.db.updatePipelineRun(pipelineId, { status: 'failed' });
          return;
        }

        // Refresh the run data to use the new download ID
        const refreshedRun = await this.db.getPipelineRun(pipelineId);
        if (refreshedRun) {
          run.torrent_download_id = refreshedRun.torrent_download_id;
          run.torrent_status = refreshedRun.torrent_status;
        }
      }

      // Poll if downloading but not yet completed
      if (run.torrent_status === 'downloading' && run.torrent_download_id) {
        await this.db.updatePipelineRun(pipelineId, { status: 'downloading' });
        const downloadComplete = await this.pollDownloadStatus(pipelineId, run.torrent_download_id);
        if (!downloadComplete) {
          await this.db.updatePipelineRun(pipelineId, { status: 'failed' });
          return;
        }
      }

      // Re-run metadata enrichment if failed (not if skipped)
      if (run.metadata_status === 'failed' || run.metadata_status === 'pending') {
        await this.db.updatePipelineRun(pipelineId, { status: 'enriching_metadata' });
        await this.enrichMetadata(pipelineId, run.content_title, run.content_type ?? undefined);
      }

      // Re-run subtitle fetch if failed (not if skipped)
      if (run.subtitle_status === 'failed' || run.subtitle_status === 'pending') {
        await this.db.updatePipelineRun(pipelineId, { status: 'fetching_subtitles' });
        await this.fetchSubtitles(pipelineId, run.content_title);
      }

      // Re-run encoding if failed (not if skipped or completed)
      if (run.encoding_status === 'failed' || run.encoding_status === 'pending') {
        await this.db.updatePipelineRun(pipelineId, { status: 'encoding' });
        const downloadPath = (run.metadata as Record<string, unknown>)?.download_path as string | undefined;
        await this.encodeContent(pipelineId, downloadPath, run.metadata);
      }

      // Re-run publishing if failed (not if skipped or completed)
      if (run.publishing_status === 'failed' || run.publishing_status === 'pending') {
        await this.db.updatePipelineRun(pipelineId, { status: 'publishing' });
        const retryRun = await this.db.getPipelineRun(pipelineId);
        await this.publishContent(
          pipelineId,
          retryRun?.encoding_job_id ?? run.encoding_job_id ?? null,
          retryRun?.metadata ?? run.metadata,
        );
      }

      // All stages done
      await this.db.updatePipelineRun(pipelineId, {
        status: 'completed',
        pipeline_completed_at: new Date(),
      });
      logger.info(`Pipeline ${pipelineId} retry completed successfully`);

    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      await this.db.updatePipelineRun(pipelineId, {
        status: 'failed',
        error_message: message,
      });
      logger.error(`Pipeline ${pipelineId} retry failed: ${message}`);
    }
  }

  // ==========================================================================
  // Helpers
  // ==========================================================================

  private sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}
