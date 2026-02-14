/**
 * SSDP (Simple Service Discovery Protocol) Implementation
 * Handles UPnP device discovery via UDP multicast on 239.255.255.250:1900
 */

import dgram from 'node:dgram';
import { createLogger } from '@nself/plugin-utils';
import type { SSDPConfig, DiscoveredRenderer } from './types.js';
import { getLocalIpAddress } from './config.js';

const logger = createLogger('dlna:ssdp');

const SSDP_ADDRESS = '239.255.255.250';
const SSDP_PORT = 1900;

const DEVICE_TYPES = [
  'upnp:rootdevice',
  'urn:schemas-upnp-org:device:MediaServer:1',
  'urn:schemas-upnp-org:service:ContentDirectory:1',
  'urn:schemas-upnp-org:service:ConnectionManager:1',
];

export class SSDPServer {
  private config: SSDPConfig;
  private socket: dgram.Socket | null = null;
  private advertiseTimer: ReturnType<typeof setInterval> | null = null;
  private discoveredRenderers: Map<string, DiscoveredRenderer> = new Map();
  private running = false;
  private localIp: string;
  private onRendererDiscovered: ((renderer: DiscoveredRenderer) => void) | null = null;

  constructor(config: SSDPConfig) {
    this.config = config;
    this.localIp = getLocalIpAddress();
  }

  /**
   * Set callback for when a renderer is discovered
   */
  setRendererCallback(callback: (renderer: DiscoveredRenderer) => void): void {
    this.onRendererDiscovered = callback;
  }

  /**
   * Start the SSDP server: bind to multicast, listen for M-SEARCH,
   * and begin periodic NOTIFY advertisements
   */
  async start(): Promise<void> {
    if (this.running) return;

    return new Promise<void>((resolve, reject) => {
      this.socket = dgram.createSocket({ type: 'udp4', reuseAddr: true });

      this.socket.on('error', (err) => {
        logger.error('SSDP socket error', { error: err.message });
        if (!this.running) {
          reject(err);
        }
      });

      this.socket.on('message', (msg, rinfo) => {
        this.handleMessage(msg.toString(), rinfo);
      });

      this.socket.bind(this.config.port, () => {
        try {
          this.socket!.addMembership(SSDP_ADDRESS);
          this.socket!.setMulticastTTL(4);
          this.socket!.setBroadcast(true);
        } catch (err) {
          const message = err instanceof Error ? err.message : 'Unknown error';
          logger.warn('Failed to join multicast group (may need elevated permissions)', { error: message });
        }

        this.running = true;

        // Send initial alive notifications
        this.sendAliveNotifications();

        // Set up periodic advertisements
        this.advertiseTimer = setInterval(() => {
          this.sendAliveNotifications();
        }, this.config.advertiseInterval * 1000);

        logger.info('SSDP server started', {
          address: SSDP_ADDRESS,
          port: this.config.port,
          uuid: this.config.uuid,
          interval: this.config.advertiseInterval,
        });

        resolve();
      });
    });
  }

  /**
   * Stop the SSDP server and send bye-bye notifications
   */
  async stop(): Promise<void> {
    if (!this.running) return;

    this.running = false;

    // Stop periodic advertisements
    if (this.advertiseTimer) {
      clearInterval(this.advertiseTimer);
      this.advertiseTimer = null;
    }

    // Send bye-bye notifications
    await this.sendByeByeNotifications();

    // Close socket
    return new Promise<void>((resolve) => {
      if (this.socket) {
        try {
          this.socket.dropMembership(SSDP_ADDRESS);
        } catch {
          // Ignore errors when leaving multicast group
        }

        this.socket.close(() => {
          this.socket = null;
          logger.info('SSDP server stopped');
          resolve();
        });
      } else {
        resolve();
      }
    });
  }

  /**
   * Get all discovered renderers
   */
  getDiscoveredRenderers(): DiscoveredRenderer[] {
    // Prune expired renderers
    const now = Date.now();
    for (const [usn, renderer] of this.discoveredRenderers) {
      if (now - renderer.lastSeen.getTime() > renderer.maxAge * 1000) {
        this.discoveredRenderers.delete(usn);
      }
    }
    return Array.from(this.discoveredRenderers.values());
  }

  /**
   * Actively search for renderers on the network
   */
  searchForRenderers(): void {
    if (!this.socket || !this.running) return;

    const searchMessage = [
      'M-SEARCH * HTTP/1.1',
      `HOST: ${SSDP_ADDRESS}:${SSDP_PORT}`,
      'MAN: "ssdp:discover"',
      'MX: 3',
      'ST: urn:schemas-upnp-org:device:MediaRenderer:1',
      '',
      '',
    ].join('\r\n');

    const buffer = Buffer.from(searchMessage);
    this.socket.send(buffer, 0, buffer.length, SSDP_PORT, SSDP_ADDRESS, (err) => {
      if (err) {
        logger.error('Failed to send M-SEARCH', { error: err.message });
      } else {
        logger.debug('Sent M-SEARCH for renderers');
      }
    });
  }

  // ---------------------------------------------------------------------------
  // Private Methods
  // ---------------------------------------------------------------------------

  private handleMessage(message: string, rinfo: dgram.RemoteInfo): void {
    const lines = message.split('\r\n');
    const firstLine = lines[0];

    if (firstLine.startsWith('M-SEARCH')) {
      this.handleMSearch(message, rinfo);
    } else if (firstLine.startsWith('NOTIFY')) {
      this.handleNotify(message, rinfo);
    } else if (firstLine.startsWith('HTTP/1.1 200')) {
      // Response to our M-SEARCH
      this.handleSearchResponse(message, rinfo);
    }
  }

  /**
   * Handle incoming M-SEARCH requests from DLNA clients
   */
  private handleMSearch(message: string, rinfo: dgram.RemoteInfo): void {
    const headers = this.parseHeaders(message);
    const st = headers['st'] ?? headers['ST'];

    if (!st) return;

    logger.debug('Received M-SEARCH', { st, from: `${rinfo.address}:${rinfo.port}` });

    // Check if the search target matches our device types
    const shouldRespond =
      st === 'ssdp:all' ||
      st === 'upnp:rootdevice' ||
      st === `uuid:${this.config.uuid}` ||
      DEVICE_TYPES.includes(st);

    if (!shouldRespond) return;

    // Respond to the M-SEARCH with our device information
    const mx = parseInt(headers['mx'] ?? headers['MX'] ?? '3', 10);
    const delay = Math.floor(Math.random() * Math.min(mx, 5) * 1000);

    setTimeout(() => {
      const responseTargets = st === 'ssdp:all' ? DEVICE_TYPES : [st];
      for (const target of responseTargets) {
        this.sendSearchResponse(target, rinfo);
      }
    }, delay);
  }

  /**
   * Handle incoming NOTIFY messages from other devices
   */
  private handleNotify(message: string, rinfo: dgram.RemoteInfo): void {
    const headers = this.parseHeaders(message);
    const nts = headers['nts'] ?? headers['NTS'];
    const nt = headers['nt'] ?? headers['NT'];
    const usn = headers['usn'] ?? headers['USN'];
    const location = headers['location'] ?? headers['LOCATION'];

    if (!usn) return;

    // Skip our own notifications
    if (usn.includes(this.config.uuid)) return;

    // Check if this is a renderer
    const isRenderer =
      (nt && nt.includes('MediaRenderer')) ||
      (usn && usn.includes('MediaRenderer'));

    if (!isRenderer) return;

    if (nts === 'ssdp:alive') {
      const cacheControl = headers['cache-control'] ?? headers['CACHE-CONTROL'] ?? 'max-age=1800';
      const maxAgeMatch = cacheControl.match(/max-age\s*=\s*(\d+)/i);
      const maxAge = maxAgeMatch ? parseInt(maxAgeMatch[1], 10) : 1800;

      const renderer: DiscoveredRenderer = {
        usn,
        location: location ?? '',
        ipAddress: rinfo.address,
        deviceType: nt ?? '',
        server: headers['server'] ?? headers['SERVER'] ?? '',
        maxAge,
        lastSeen: new Date(),
      };

      this.discoveredRenderers.set(usn, renderer);
      logger.debug('Renderer discovered via NOTIFY', { usn, ip: rinfo.address });

      if (this.onRendererDiscovered) {
        this.onRendererDiscovered(renderer);
      }
    } else if (nts === 'ssdp:byebye') {
      this.discoveredRenderers.delete(usn);
      logger.debug('Renderer left', { usn });
    }
  }

  /**
   * Handle responses to our M-SEARCH queries
   */
  private handleSearchResponse(message: string, rinfo: dgram.RemoteInfo): void {
    const headers = this.parseHeaders(message);
    const usn = headers['usn'] ?? headers['USN'];
    const location = headers['location'] ?? headers['LOCATION'];
    const st = headers['st'] ?? headers['ST'];

    if (!usn || !location) return;
    if (usn.includes(this.config.uuid)) return;

    const cacheControl = headers['cache-control'] ?? headers['CACHE-CONTROL'] ?? 'max-age=1800';
    const maxAgeMatch = cacheControl.match(/max-age\s*=\s*(\d+)/i);
    const maxAge = maxAgeMatch ? parseInt(maxAgeMatch[1], 10) : 1800;

    const renderer: DiscoveredRenderer = {
      usn,
      location,
      ipAddress: rinfo.address,
      deviceType: st ?? '',
      server: headers['server'] ?? headers['SERVER'] ?? '',
      maxAge,
      lastSeen: new Date(),
    };

    this.discoveredRenderers.set(usn, renderer);
    logger.debug('Renderer discovered via search response', { usn, ip: rinfo.address });

    if (this.onRendererDiscovered) {
      this.onRendererDiscovered(renderer);
    }
  }

  /**
   * Send M-SEARCH response to a specific client
   */
  private sendSearchResponse(st: string, rinfo: dgram.RemoteInfo): void {
    if (!this.socket || !this.running) return;

    const usn = st === 'upnp:rootdevice'
      ? `uuid:${this.config.uuid}::upnp:rootdevice`
      : st === `uuid:${this.config.uuid}`
        ? `uuid:${this.config.uuid}`
        : `uuid:${this.config.uuid}::${st}`;

    const response = [
      'HTTP/1.1 200 OK',
      `CACHE-CONTROL: max-age=1800`,
      `DATE: ${new Date().toUTCString()}`,
      `EXT:`,
      `LOCATION: http://${this.localIp}:${this.config.httpPort}/description.xml`,
      `SERVER: ${this.getServerString()}`,
      `ST: ${st}`,
      `USN: ${usn}`,
      `CONTENT-LENGTH: 0`,
      '',
      '',
    ].join('\r\n');

    const buffer = Buffer.from(response);
    this.socket.send(buffer, 0, buffer.length, rinfo.port, rinfo.address, (err) => {
      if (err) {
        logger.error('Failed to send M-SEARCH response', { error: err.message });
      }
    });
  }

  /**
   * Send NOTIFY ssdp:alive for all device types
   */
  private sendAliveNotifications(): void {
    if (!this.socket || !this.running) return;

    // Send for root device
    this.sendNotify('upnp:rootdevice', `uuid:${this.config.uuid}::upnp:rootdevice`, 'ssdp:alive');

    // Send for UUID
    this.sendNotify(`uuid:${this.config.uuid}`, `uuid:${this.config.uuid}`, 'ssdp:alive');

    // Send for each device/service type
    for (const dt of DEVICE_TYPES) {
      if (dt === 'upnp:rootdevice') continue; // Already sent
      this.sendNotify(dt, `uuid:${this.config.uuid}::${dt}`, 'ssdp:alive');
    }

    logger.debug('Sent alive notifications');
  }

  /**
   * Send NOTIFY ssdp:byebye for all device types
   */
  private async sendByeByeNotifications(): Promise<void> {
    if (!this.socket) return;

    const sends: Promise<void>[] = [];

    // Bye-bye for root device
    sends.push(this.sendNotifyAsync('upnp:rootdevice', `uuid:${this.config.uuid}::upnp:rootdevice`, 'ssdp:byebye'));

    // Bye-bye for UUID
    sends.push(this.sendNotifyAsync(`uuid:${this.config.uuid}`, `uuid:${this.config.uuid}`, 'ssdp:byebye'));

    // Bye-bye for each type
    for (const dt of DEVICE_TYPES) {
      if (dt === 'upnp:rootdevice') continue;
      sends.push(this.sendNotifyAsync(dt, `uuid:${this.config.uuid}::${dt}`, 'ssdp:byebye'));
    }

    await Promise.allSettled(sends);
    logger.debug('Sent bye-bye notifications');
  }

  /**
   * Send a single NOTIFY message
   */
  private sendNotify(nt: string, usn: string, nts: string): void {
    if (!this.socket || !this.running) return;

    const message = this.buildNotifyMessage(nt, usn, nts);
    const buffer = Buffer.from(message);

    this.socket.send(buffer, 0, buffer.length, SSDP_PORT, SSDP_ADDRESS, (err) => {
      if (err) {
        logger.error('Failed to send NOTIFY', { error: err.message, nt });
      }
    });
  }

  /**
   * Send a single NOTIFY message (async version)
   */
  private sendNotifyAsync(nt: string, usn: string, nts: string): Promise<void> {
    return new Promise<void>((resolve) => {
      if (!this.socket) {
        resolve();
        return;
      }

      const message = this.buildNotifyMessage(nt, usn, nts);
      const buffer = Buffer.from(message);

      this.socket.send(buffer, 0, buffer.length, SSDP_PORT, SSDP_ADDRESS, (err) => {
        if (err) {
          logger.error('Failed to send NOTIFY', { error: err.message, nt });
        }
        resolve();
      });
    });
  }

  /**
   * Build a NOTIFY message
   */
  private buildNotifyMessage(nt: string, usn: string, nts: string): string {
    const headers = [
      'NOTIFY * HTTP/1.1',
      `HOST: ${SSDP_ADDRESS}:${SSDP_PORT}`,
      `CACHE-CONTROL: max-age=1800`,
      `LOCATION: http://${this.localIp}:${this.config.httpPort}/description.xml`,
      `NT: ${nt}`,
      `NTS: ${nts}`,
      `SERVER: ${this.getServerString()}`,
      `USN: ${usn}`,
    ];

    // Only include LOCATION for alive notifications
    if (nts === 'ssdp:byebye') {
      const locationIndex = headers.findIndex(h => h.startsWith('LOCATION:'));
      if (locationIndex !== -1) {
        headers.splice(locationIndex, 1);
      }
      const cacheIndex = headers.findIndex(h => h.startsWith('CACHE-CONTROL:'));
      if (cacheIndex !== -1) {
        headers.splice(cacheIndex, 1);
      }
    }

    return headers.join('\r\n') + '\r\n\r\n';
  }

  /**
   * Parse SSDP/HTTP headers from a message
   */
  private parseHeaders(message: string): Record<string, string> {
    const headers: Record<string, string> = {};
    const lines = message.split('\r\n');

    for (let i = 1; i < lines.length; i++) {
      const line = lines[i];
      if (!line) continue;

      const colonIndex = line.indexOf(':');
      if (colonIndex === -1) continue;

      const key = line.substring(0, colonIndex).trim().toLowerCase();
      const value = line.substring(colonIndex + 1).trim();
      headers[key] = value;
    }

    return headers;
  }

  /**
   * Get the SERVER header string
   */
  private getServerString(): string {
    return `${process.platform}/${process.version} UPnP/1.1 nself-dlna/1.0`;
  }
}
