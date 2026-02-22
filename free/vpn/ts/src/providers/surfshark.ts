/**
 * Surfshark VPN Provider Implementation
 * Full support for Surfshark CLI and API
 */

import { BaseVPNProvider } from './base.js';
import { createLogger } from '@nself/plugin-utils';
import type {
  ConnectVPNRequest,
  VPNConnectionRecord,
  VPNCredentialRecord,
  VPNServerRecord,
  VPNStatus,
  VPNProtocol,
} from '../types.js';

const logger = createLogger('vpn:surfshark');

interface SurfsharkServer {
  id: string;
  connectionName: string;
  country: string;
  countryCode: string;
  location: string;
  load: number;
  pubKey?: string;
  tags: string[];
  type: string;
  host: string;
}

interface SurfsharkServerListResponse {
  servers: SurfsharkServer[];
}

export class SurfsharkProvider extends BaseVPNProvider {
  readonly name = 'surfshark' as const;
  readonly displayName = 'Surfshark';
  readonly cliAvailable = true;
  readonly apiAvailable = true;
  readonly portForwardingSupported = false;
  readonly p2pAllServers = false;

  private readonly apiEndpoint = 'https://api.surfshark.com/v1';
  private readonly cliCommand = 'surfshark-vpn';

  // ============================================================================
  // Initialization
  // ============================================================================

  protected async checkCLIInstalled(): Promise<void> {
    try {
      await this.executeCommand(`${this.cliCommand} --version`);
      logger.info('Surfshark CLI is installed');
    } catch (error) {
      throw new Error(
        'Surfshark CLI is not installed. Install from: https://surfshark.com/download/linux'
      );
    }
  }

  protected async performAuthentication(credentials: VPNCredentialRecord): Promise<boolean> {
    try {
      if (!credentials.username || !credentials.password_encrypted) {
        throw new Error('Surfshark requires username and password');
      }

      // Check if already authenticated
      try {
        const statusResult = await this.executeCommand(`${this.cliCommand} status`);
        if (statusResult.stdout.includes('Logged in') || statusResult.stdout.includes('Connected')) {
          logger.info('Already authenticated with Surfshark');
          return true;
        }
      } catch {
        // Not authenticated yet — proceed with login
      }

      // Write credentials to temp file for login
      const fs = await import('fs/promises');
      const credsFile = '/tmp/surfshark-creds.txt';
      await fs.writeFile(credsFile, `${credentials.username}\n${credentials.password_encrypted}`, {
        mode: 0o600,
      });

      await this.executeCommand(
        `echo "${credentials.password_encrypted}" | ${this.cliCommand} set credentials ${credentials.username}`
      );

      await fs.unlink(credsFile).catch(() => {});

      logger.info('Successfully authenticated with Surfshark');
      return true;
    } catch (error) {
      logger.error('Surfshark authentication failed', {
        error: error instanceof Error ? error.message : String(error),
      });
      return false;
    }
  }

  // ============================================================================
  // Server Management
  // ============================================================================

  async fetchServers(): Promise<VPNServerRecord[]> {
    logger.info('Fetching Surfshark server list from API');

    try {
      const response = await fetch(`${this.apiEndpoint}/server/clusters`, {
        headers: { 'Content-Type': 'application/json' },
      });

      if (!response.ok) {
        throw new Error(`API request failed: ${response.statusText}`);
      }

      const data: SurfsharkServerListResponse = await response.json();
      const serverList = data.servers ?? (data as unknown as SurfsharkServer[]);
      const servers: SurfsharkServer[] = Array.isArray(serverList) ? serverList : (data as unknown as SurfsharkServer[]);

      logger.info(`Fetched ${servers.length} servers from Surfshark`);

      return servers.map((server) => this.mapServer(server));
    } catch (error) {
      logger.error('Failed to fetch Surfshark servers', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  private mapServer(server: SurfsharkServer): VPNServerRecord {
    const protocols: VPNProtocol[] = ['wireguard', 'openvpn_udp', 'openvpn_tcp', 'ikev2'];
    const isP2P = server.tags?.includes('p2p') || server.type === 'p2p';

    return {
      id: `surfshark-${server.id}`,
      provider_id: 'surfshark',
      hostname: server.host,
      ip_address: server.host, // Surfshark uses hostname-based routing
      country_code: (server.countryCode ?? '').toUpperCase(),
      country_name: server.country,
      city: server.location,
      p2p_supported: isP2P,
      port_forwarding_supported: false,
      protocols,
      load: server.load,
      status: 'online',
      features: server.tags ?? [],
      public_key: server.pubKey,
      owned: true,
      metadata: {
        connection_name: server.connectionName,
        server_type: server.type,
      },
      last_seen: new Date(),
      created_at: new Date(),
      updated_at: new Date(),
    } as VPNServerRecord;
  }

  // ============================================================================
  // Connection Management
  // ============================================================================

  async connect(request: ConnectVPNRequest, credentials: VPNCredentialRecord): Promise<VPNConnectionRecord> {
    this.ensureAuthenticated();

    logger.info('Connecting to Surfshark', {
      region: request.region,
      server: request.server,
      protocol: request.protocol,
    });

    // Set protocol
    if (request.protocol) {
      const proto = this.mapProtocol(request.protocol);
      await this.executeCommand(`${this.cliCommand} set protocol ${proto}`);
    }

    // Build connect command
    let connectCmd = `${this.cliCommand} connect`;

    if (request.server) {
      connectCmd += ` --server ${request.server}`;
    } else if (request.region) {
      connectCmd += ` --country ${request.region}`;
    } else {
      connectCmd += ' --fastest';
    }

    try {
      const result = await this.executeCommand(connectCmd, 60000);
      logger.info('Surfshark connection established', { output: result.stdout });

      const status = await this.getStatus();

      const connection: VPNConnectionRecord = {
        id: `surfshark-${Date.now()}`,
        provider_id: 'surfshark',
        server_id: undefined,
        protocol: (request.protocol || 'wireguard') as VPNProtocol,
        status: 'connected',
        local_ip: undefined,
        vpn_ip: status.vpn_ip,
        interface_name: status.interface,
        dns_servers: [],
        connected_at: new Date(),
        kill_switch_enabled: request.kill_switch !== false,
        requested_by: request.requested_by,
        metadata: {},
        created_at: new Date(),
      };

      return connection;
    } catch (error) {
      logger.error('Surfshark connection failed', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw new Error(`Failed to connect to Surfshark: ${error}`);
    }
  }

  async disconnect(connectionId: string): Promise<void> {
    logger.info('Disconnecting from Surfshark', { connectionId });

    try {
      await this.executeCommand(`${this.cliCommand} disconnect`);
      logger.info('Successfully disconnected from Surfshark');
    } catch (error) {
      logger.error('Failed to disconnect from Surfshark', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  async getStatus(): Promise<VPNStatus> {
    try {
      const result = await this.executeCommand(`${this.cliCommand} status`);
      return this.parseSurfsharkStatus(result.stdout);
    } catch (error) {
      logger.error('Failed to get Surfshark status', {
        error: error instanceof Error ? error.message : String(error),
      });
      return { connected: false };
    }
  }

  private parseSurfsharkStatus(output: string): VPNStatus {
    const lines = output.split('\n');
    const status: VPNStatus = { connected: false };

    for (const line of lines) {
      const trimmed = line.trim();

      if (trimmed.toLowerCase().includes('connected')) {
        status.connected = true;
      } else if (trimmed.startsWith('Server:') || trimmed.startsWith('Location:')) {
        status.server = trimmed.split(':').slice(1).join(':').trim();
      } else if (trimmed.startsWith('IP:') || trimmed.startsWith('VPN IP:')) {
        status.vpn_ip = trimmed.split(':').slice(1).join(':').trim();
      } else if (trimmed.startsWith('Protocol:')) {
        status.protocol = trimmed.split(':')[1]?.trim().toLowerCase();
      }
    }

    if (status.connected) {
      const proto = status.protocol ?? '';
      status.interface = proto.includes('wireguard') ? 'wg0' : 'tun0';
    }

    return status;
  }

  // ============================================================================
  // Kill Switch
  // ============================================================================

  async enableKillSwitch(): Promise<void> {
    logger.info('Enabling Surfshark kill switch');

    try {
      await this.executeCommand(`${this.cliCommand} set killswitch on`);
      logger.info('Kill switch enabled');
    } catch (error) {
      logger.error('Failed to enable kill switch', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  async disableKillSwitch(): Promise<void> {
    logger.info('Disabling Surfshark kill switch');

    try {
      await this.executeCommand(`${this.cliCommand} set killswitch off`);
      logger.info('Kill switch disabled');
    } catch (error) {
      logger.error('Failed to disable kill switch', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  // ============================================================================
  // Additional Methods
  // ============================================================================

  /**
   * Map generic protocol name to Surfshark CLI protocol string
   */
  private mapProtocol(protocol: VPNProtocol): string {
    const map: Record<string, string> = {
      wireguard: 'wireguard',
      openvpn_udp: 'openvpn-udp',
      openvpn_tcp: 'openvpn-tcp',
      ikev2: 'ikev2',
    };
    return map[protocol] ?? 'wireguard';
  }

  /**
   * Get list of available countries
   */
  async getCountries(): Promise<string[]> {
    const result = await this.executeCommand(`${this.cliCommand} list countries`);
    return result.stdout
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line.length > 0);
  }

  /**
   * Enable Surfshark CleanWeb (ad/tracker blocking)
   */
  async enableCleanWeb(): Promise<void> {
    await this.executeCommand(`${this.cliCommand} set cleanweb on`);
  }

  /**
   * Enable NoBorders mode (obfuscation for restricted regions)
   */
  async enableNoBorders(): Promise<void> {
    await this.executeCommand(`${this.cliCommand} set noborders on`);
  }
}
