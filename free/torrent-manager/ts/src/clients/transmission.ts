/**
 * Transmission Torrent Client Adapter
 */

import { Transmission } from '@ctrl/transmission';
import type { Torrent } from '@ctrl/transmission';
import { createLogger } from '@nself/plugin-utils';
import { BaseTorrentClient } from './base.js';
import type {
  TorrentDownload,
  AddTorrentOptions,
  TorrentFilter,
  TorrentClientStats,
  TorrentCategory,
} from '../types.js';

const logger = createLogger('torrent-manager:transmission');

export class TransmissionClient extends BaseTorrentClient {
  readonly type = 'transmission' as const;
  private client: Transmission;

  constructor(host: string, port: number, username?: string, password?: string) {
    super(host, port, username, password);
    this.client = new Transmission({
      baseUrl: `http://${host}:${port}/transmission/rpc`,
      username,
      password,
    });
  }

  async connect(): Promise<boolean> {
    try {
      // Test connection by getting session info
      await this.client.getSession();
      this.connected = true;
      logger.info('Connected to Transmission', { host: this.host, port: this.port });
      return true;
    } catch (error) {
      logger.error('Failed to connect to Transmission', { error: error instanceof Error ? error.message : String(error) });
      this.connected = false;
      return false;
    }
  }

  async disconnect(): Promise<void> {
    // Transmission doesn't need explicit disconnect
    this.connected = false;
    logger.info('Disconnected from Transmission');
  }

  async isConnected(): Promise<boolean> {
    if (!this.connected) {
      return false;
    }

    try {
      await this.client.getSession();
      return true;
    } catch {
      this.connected = false;
      return false;
    }
  }

  async addTorrent(magnetUri: string, options: AddTorrentOptions): Promise<TorrentDownload> {
    logger.info('Adding torrent to Transmission', { magnet: magnetUri.slice(0, 50) + '...' });

    try {
      const result = await this.client.addUrl(magnetUri, {
        'download-dir': options.download_path || this.getDownloadPath(),
        paused: options.paused ?? false,
      });

      const torrentAdded = result.arguments['torrent-added'];
      if (!torrentAdded) {
        throw new Error('Failed to add torrent to Transmission');
      }

      // Map Transmission torrent to TorrentDownload
      const download: TorrentDownload = {
        id: '', // Will be set by database
        source_account_id: 'primary',
        client_id: '', // Will be set by caller
        client_torrent_id: String(torrentAdded.id),

        name: torrentAdded.name,
        info_hash: torrentAdded.hashString,
        magnet_uri: magnetUri,

        status: options.paused ? 'paused' : 'queued',
        category: options.category || 'other',

        size_bytes: 0,
        downloaded_bytes: 0,
        uploaded_bytes: 0,
        progress_percent: 0,
        ratio: 0,

        download_speed_bytes: 0,
        upload_speed_bytes: 0,

        seeders: 0,
        leechers: 0,
        peers_connected: 0,

        download_path: options.download_path || this.getDownloadPath(),
        files_count: 0,

        requested_by: 'transmission',

        added_at: new Date(),
        created_at: new Date(),
        updated_at: new Date(),
      };

      logger.info('Torrent added successfully', { id: torrentAdded.id, name: torrentAdded.name });
      return download;
    } catch (error) {
      logger.error('Failed to add torrent', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  async getTorrent(id: string): Promise<TorrentDownload | null> {
    try {
      const result = await this.client.listTorrents([parseInt(id, 10)]);
      if (!result.arguments.torrents || result.arguments.torrents.length === 0) {
        return null;
      }

      const torrent = result.arguments.torrents[0];
      return this.mapTorrentToDownload(torrent);
    } catch (error) {
      logger.error('Failed to get torrent', { error: error instanceof Error ? error.message : String(error) });
      return null;
    }
  }

  async listTorrents(filter?: TorrentFilter): Promise<TorrentDownload[]> {
    try {
      const result = await this.client.listTorrents();
      const torrents = result.arguments.torrents || [];

      let filtered = torrents;

      // Apply status filter
      if (filter?.status) {
        filtered = filtered.filter((t: Torrent) => {
          const status = this.mapTransmissionStatus(t.status);
          return status === filter.status;
        });
      }

      return filtered.map((t: Torrent) => this.mapTorrentToDownload(t));
    } catch (error) {
      logger.error('Failed to list torrents', { error: error instanceof Error ? error.message : String(error) });
      return [];
    }
  }

  async pauseTorrent(id: string): Promise<void> {
    logger.info('Pausing torrent', { id });
    await this.client.pauseTorrent([parseInt(id, 10)]);
  }

  async resumeTorrent(id: string): Promise<void> {
    logger.info('Resuming torrent', { id });
    await this.client.resumeTorrent([parseInt(id, 10)]);
  }

  async removeTorrent(id: string, deleteFiles: boolean): Promise<void> {
    logger.info('Removing torrent', { id, deleteFiles });
    await this.client.removeTorrent([parseInt(id, 10)], deleteFiles);
  }

  async getStats(): Promise<TorrentClientStats> {
    try {
      const session = await this.client.getSession();
      const result = await this.client.listTorrents();
      const torrents = result.arguments.torrents || [];

      return {
        total_torrents: torrents.length,
        active_torrents: torrents.filter((t: Torrent) => t.status === 4).length,
        paused_torrents: torrents.filter((t: Torrent) => t.status === 0).length,
        seeding_torrents: torrents.filter((t: Torrent) => t.status === 6).length,
        download_speed_bytes: torrents.reduce((sum: number, t: Torrent) => sum + (t.rateDownload || 0), 0),
        upload_speed_bytes: torrents.reduce((sum: number, t: Torrent) => sum + (t.rateUpload || 0), 0),
        downloaded_bytes: torrents.reduce((sum: number, t: Torrent) => sum + (t.downloadedEver || 0), 0),
        uploaded_bytes: torrents.reduce((sum: number, t: Torrent) => sum + (t.uploadedEver || 0), 0),
      };
    } catch (error) {
      logger.error('Failed to get stats', { error: error instanceof Error ? error.message : String(error) });
      return {
        total_torrents: 0,
        active_torrents: 0,
        paused_torrents: 0,
        seeding_torrents: 0,
        download_speed_bytes: 0,
        upload_speed_bytes: 0,
        downloaded_bytes: 0,
        uploaded_bytes: 0,
      };
    }
  }

  // ============================================================================
  // Helper Methods
  // ============================================================================

  private mapTorrentToDownload(torrent: Torrent): TorrentDownload {
    const status = this.mapTransmissionStatus(torrent.status);

    return {
      id: '', // Database ID
      source_account_id: 'primary',
      client_id: '',
      client_torrent_id: String(torrent.id),

      name: torrent.name,
      info_hash: torrent.hashString,
      magnet_uri: torrent.magnetLink || '',

      status,
      category: 'other',

      size_bytes: torrent.totalSize || 0,
      downloaded_bytes: torrent.downloadedEver || 0,
      uploaded_bytes: torrent.uploadedEver || 0,
      progress_percent: (torrent.percentDone || 0) * 100,
      ratio: torrent.uploadRatio || 0,

      download_speed_bytes: torrent.rateDownload || 0,
      upload_speed_bytes: torrent.rateUpload || 0,

      seeders: torrent.peersSendingToUs || 0,
      leechers: torrent.peersGettingFromUs || 0,
      peers_connected: torrent.peersConnected || 0,

      download_path: torrent.downloadDir || '',
      files_count: torrent.files?.length || 0,

      requested_by: 'transmission',

      added_at: new Date(torrent.addedDate * 1000),
      started_at: torrent.activityDate ? new Date(torrent.activityDate * 1000) : undefined,
      completed_at: torrent.doneDate ? new Date(torrent.doneDate * 1000) : undefined,

      created_at: new Date(),
      updated_at: new Date(),
    };
  }

  private mapTransmissionStatus(status: number): TorrentDownload['status'] {
    /*
     * Transmission status codes:
     * 0: stopped
     * 1: check pending
     * 2: checking
     * 3: download pending
     * 4: downloading
     * 5: seed pending
     * 6: seeding
     */
    switch (status) {
      case 0:
        return 'paused';
      case 4:
        return 'downloading';
      case 6:
        return 'seeding';
      default:
        return 'queued';
    }
  }

  private getDownloadPath(): string {
    return '/downloads';
  }
}
