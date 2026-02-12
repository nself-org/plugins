/**
 * Torrent Client Integration
 * Transmission RPC-based torrent downloads bound to VPN interface
 */

import { Transmission } from '@ctrl/transmission';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('vpn:torrent');

export interface TorrentInfo {
  id: string;
  name: string;
  progress: number;
  downloadSpeed: number;
  uploadSpeed: number;
  size: number;
  downloaded: number;
  uploaded: number;
  status: string;
  peers: number;
  seeds: number;
}

export class TorrentClient {
  private client: Transmission | null = null;
  private config: { host: string; port: number; username?: string; password?: string };

  constructor(config?: { host?: string; port?: number; username?: string; password?: string }) {
    this.config = {
      host: config?.host || process.env.TRANSMISSION_HOST || 'localhost',
      port: config?.port || parseInt(process.env.TRANSMISSION_PORT || '9091', 10),
      username: config?.username || process.env.TRANSMISSION_USERNAME,
      password: config?.password || process.env.TRANSMISSION_PASSWORD,
    };
  }

  async connect(): Promise<void> {
    try {
      this.client = new Transmission({
        baseUrl: `http://${this.config.host}:${this.config.port}/transmission/rpc`,
        username: this.config.username,
        password: this.config.password,
      });
      // Test connection
      await this.client.getAllData();
      logger.info('Connected to Transmission');
    } catch (error) {
      logger.error('Failed to connect to Transmission', { error: error instanceof Error ? error.message : String(error) });
      throw new Error(`Transmission connection failed: ${error}`);
    }
  }

  async download(magnetLink: string, downloadDir?: string): Promise<TorrentInfo> {
    if (!this.client) throw new Error('Not connected to Transmission');

    try {
      const result = await this.client.addMagnet(magnetLink, { 'download-dir': downloadDir });
      const args = result.arguments as Record<string, { id: number; hashString: string; name: string }>;
      const torrent = args['torrent-added'] || args['torrent-duplicate'];

      return {
        id: String(torrent.id),
        name: torrent.name || 'Unknown',
        progress: 0,
        downloadSpeed: 0,
        uploadSpeed: 0,
        size: 0,
        downloaded: 0,
        uploaded: 0,
        status: 'downloading',
        peers: 0,
        seeds: 0,
      };
    } catch (error) {
      logger.error('Failed to add torrent', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  async getProgress(torrentId: string): Promise<TorrentInfo> {
    if (!this.client) throw new Error('Not connected to Transmission');

    try {
      const t = await this.client.getTorrent(parseInt(torrentId));

      return {
        id: String(t.id),
        name: t.name,
        progress: Math.round(t.progress),
        downloadSpeed: t.downloadSpeed || 0,
        uploadSpeed: t.uploadSpeed || 0,
        size: t.totalSize || 0,
        downloaded: t.totalDownloaded || 0,
        uploaded: t.totalUploaded || 0,
        status: t.isCompleted ? 'completed' : 'downloading',
        peers: t.connectedPeers || 0,
        seeds: t.connectedSeeds || 0,
      };
    } catch (error) {
      logger.error(`Failed to get torrent progress for ${torrentId}`, { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  async pause(torrentId: string): Promise<void> {
    if (!this.client) throw new Error('Not connected to Transmission');
    await this.client.pauseTorrent(parseInt(torrentId));
    logger.info(`Paused torrent ${torrentId}`);
  }

  async resume(torrentId: string): Promise<void> {
    if (!this.client) throw new Error('Not connected to Transmission');
    await this.client.resumeTorrent(parseInt(torrentId));
    logger.info(`Resumed torrent ${torrentId}`);
  }

  async cancel(torrentId: string, deleteFiles = false): Promise<void> {
    if (!this.client) throw new Error('Not connected to Transmission');
    await this.client.removeTorrent(parseInt(torrentId), deleteFiles);
    logger.info(`Cancelled torrent ${torrentId}, deleteFiles=${deleteFiles}`);
  }

  async listAll(): Promise<TorrentInfo[]> {
    if (!this.client) throw new Error('Not connected to Transmission');
    const result = await this.client.getAllData();
    return (result.torrents || []).map((t) => ({
      id: String(t.id),
      name: t.name,
      progress: Math.round(t.progress),
      downloadSpeed: t.downloadSpeed || 0,
      uploadSpeed: t.uploadSpeed || 0,
      size: t.totalSize || 0,
      downloaded: t.totalDownloaded || 0,
      uploaded: t.totalUploaded || 0,
      status: t.isCompleted ? 'completed' : t.state === 'paused' ? 'paused' : 'downloading',
      peers: t.connectedPeers || 0,
      seeds: t.connectedSeeds || 0,
    }));
  }
}
