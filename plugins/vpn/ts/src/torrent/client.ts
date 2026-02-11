/**
 * Torrent Client Integration
 * WebTorrent-based torrent downloads bound to VPN interface
 */

import { createLogger } from '@nself/plugin-utils';
import type { TorrentInfo, TorrentProgress } from '../types.js';

const logger = createLogger('vpn:torrent');

export class TorrentClient {
  private interfaceName?: string;
  private downloadPath: string;

  constructor(downloadPath: string) {
    this.downloadPath = downloadPath;
  }

  /**
   * Bind to VPN interface
   */
  bindToInterface(interfaceName: string): void {
    this.interfaceName = interfaceName;
    logger.info(`Torrent client bound to interface: ${interfaceName}`);
  }

  /**
   * Start download from magnet link
   */
  async download(
    magnetLink: string,
    destination?: string
  ): Promise<{ id: string; info: TorrentInfo }> {
    if (!this.interfaceName) {
      logger.warn('Torrent client not bound to VPN interface - downloads may leak IP');
    }

    logger.info('Starting torrent download', {
      magnet: magnetLink.slice(0, 50) + '...',
      interface: this.interfaceName,
    });

    // Extract info hash
    const infoHashMatch = magnetLink.match(/urn:btih:([a-fA-F0-9]{40})/);
    const infoHash = infoHashMatch ? infoHashMatch[1] : 'unknown';

    // TODO: Actual WebTorrent implementation
    // This is a placeholder that would be replaced with real WebTorrent code:
    //
    // const WebTorrent = await import('webtorrent');
    // const client = new WebTorrent();
    //
    // // Bind to VPN interface
    // if (this.interfaceName) {
    //   client.on('torrent', (torrent) => {
    //     torrent.wires.forEach(wire => {
    //       // Force connections through VPN interface
    //       wire.socket.bind(this.interfaceName);
    //     });
    //   });
    // }
    //
    // return new Promise((resolve, reject) => {
    //   client.add(magnetLink, { path: destination || this.downloadPath }, (torrent) => {
    //     resolve({
    //       id: infoHash,
    //       info: {
    //         infoHash: torrent.infoHash,
    //         name: torrent.name,
    //         length: torrent.length,
    //         files: torrent.files.map(f => ({
    //           name: f.name,
    //           length: f.length,
    //           path: f.path,
    //         })),
    //       },
    //     });
    //   });
    // });

    return {
      id: infoHash,
      info: {
        infoHash,
        name: undefined,
        length: undefined,
        files: [],
      },
    };
  }

  /**
   * Get download progress
   */
  async getProgress(downloadId: string): Promise<TorrentProgress> {
    // TODO: Actual WebTorrent implementation
    // This would query the active torrent by infoHash

    return {
      progress: 0,
      downloaded: 0,
      total: undefined,
      downloadSpeed: 0,
      uploadSpeed: 0,
      numPeers: 0,
      ratio: 0,
    };
  }

  /**
   * Pause download
   */
  async pause(downloadId: string): Promise<void> {
    logger.info(`Pausing download: ${downloadId}`);
    // TODO: Implement WebTorrent pause
  }

  /**
   * Resume download
   */
  async resume(downloadId: string): Promise<void> {
    logger.info(`Resuming download: ${downloadId}`);
    // TODO: Implement WebTorrent resume
  }

  /**
   * Cancel download
   */
  async cancel(downloadId: string): Promise<void> {
    logger.info(`Cancelling download: ${downloadId}`);
    // TODO: Implement WebTorrent destroy
  }

  /**
   * Verify VPN interface is active
   */
  async verifyInterface(): Promise<boolean> {
    if (!this.interfaceName) {
      return false;
    }

    try {
      // Check if interface exists
      const { exec } = await import('child_process');
      const { promisify } = await import('util');
      const execAsync = promisify(exec);

      const { stdout } = await execAsync(`ip link show ${this.interfaceName}`);
      return stdout.includes(this.interfaceName);
    } catch {
      return false;
    }
  }
}

/**
 * Create and configure torrent client for VPN
 */
export function createTorrentClient(interfaceName: string, downloadPath: string): TorrentClient {
  const client = new TorrentClient(downloadPath);
  client.bindToInterface(interfaceName);
  return client;
}
