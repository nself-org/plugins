/**
 * Download State Machine
 *
 * Manages legal state transitions for downloads and records full history.
 *
 * States: created -> vpn_connecting -> searching -> downloading -> encoding
 *         -> subtitles -> uploading -> finalizing -> completed
 *
 * Any active state can transition to `failed` or `cancelled`.
 * `paused` can be entered from downloading/encoding/searching and resumed.
 */

import { createLogger } from '@nself/plugin-utils';
import type { Pool } from 'pg';
import type { DownloadState, DownloadStateTransition } from './types.js';

const logger = createLogger('content-acquisition:state-machine');

/**
 * Defines the set of valid transitions from each state.
 * `failed` and `cancelled` are reachable from any non-terminal state.
 */
const VALID_TRANSITIONS: Record<DownloadState, DownloadState[]> = {
  created:        ['vpn_connecting', 'failed', 'cancelled'],
  vpn_connecting: ['searching', 'failed', 'cancelled'],
  searching:      ['downloading', 'paused', 'failed', 'cancelled'],
  downloading:    ['encoding', 'paused', 'failed', 'cancelled'],
  encoding:       ['subtitles', 'paused', 'failed', 'cancelled'],
  subtitles:      ['uploading', 'failed', 'cancelled'],
  uploading:      ['finalizing', 'failed', 'cancelled'],
  finalizing:     ['completed', 'failed', 'cancelled'],
  completed:      [],
  failed:         ['created'],   // retry -> back to created
  cancelled:      [],
  paused:         ['searching', 'downloading', 'encoding', 'failed', 'cancelled'],
};

export class DownloadStateMachine {
  private pool: Pool;

  constructor(pool: Pool) {
    this.pool = pool;
  }

  /**
   * Attempt a state transition for a download.
   *
   * Validates that the transition is legal, updates the download record,
   * and inserts a history row.
   *
   * @returns The updated state on success.
   * @throws  Error if the transition is not allowed.
   */
  async transition(
    downloadId: string,
    toState: DownloadState,
    metadata?: Record<string, unknown>,
  ): Promise<DownloadState> {
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');

      // Lock the download row to prevent concurrent transitions
      const downloadResult = await client.query(
        `SELECT state FROM np_contacq_downloads WHERE id = $1 FOR UPDATE`,
        [downloadId],
      );

      if (downloadResult.rows.length === 0) {
        throw new Error(`Download ${downloadId} not found`);
      }

      const fromState = downloadResult.rows[0].state as DownloadState;

      // Validate the transition
      const allowed = VALID_TRANSITIONS[fromState];
      if (!allowed || !allowed.includes(toState)) {
        throw new Error(
          `Invalid state transition: ${fromState} -> ${toState}. ` +
          `Allowed transitions from ${fromState}: ${(allowed ?? []).join(', ') || 'none'}`,
        );
      }

      // Update the download record
      await client.query(
        `UPDATE np_contacq_downloads
         SET state = $2, updated_at = NOW()
         WHERE id = $1`,
        [downloadId, toState],
      );

      // Record the transition in history
      await client.query(
        `INSERT INTO np_contacq_download_state_history
           (download_id, from_state, to_state, metadata)
         VALUES ($1, $2, $3, $4)`,
        [downloadId, fromState, toState, metadata ? JSON.stringify(metadata) : null],
      );

      await client.query('COMMIT');

      logger.info(`Download ${downloadId}: ${fromState} -> ${toState}`);
      return toState;
    } catch (error) {
      await client.query('ROLLBACK');
      throw error;
    } finally {
      client.release();
    }
  }

  /**
   * Get the current state of a download.
   */
  async getCurrentState(downloadId: string): Promise<DownloadState | null> {
    const result = await this.pool.query(
      `SELECT state FROM np_contacq_downloads WHERE id = $1`,
      [downloadId],
    );
    return (result.rows[0]?.state as DownloadState) ?? null;
  }

  /**
   * Get the full transition history for a download, ordered chronologically.
   */
  async getHistory(downloadId: string): Promise<DownloadStateTransition[]> {
    const result = await this.pool.query(
      `SELECT * FROM np_contacq_download_state_history
       WHERE download_id = $1
       ORDER BY created_at ASC`,
      [downloadId],
    );
    return result.rows;
  }

  /**
   * Check whether a transition from the current state to `toState` is legal
   * without executing it.
   */
  isValidTransition(fromState: DownloadState, toState: DownloadState): boolean {
    const allowed = VALID_TRANSITIONS[fromState];
    return !!allowed && allowed.includes(toState);
  }
}
