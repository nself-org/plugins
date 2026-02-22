/**
 * ProtonVPN Provider Implementation
 * Full support for ProtonVPN CLI and API
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

const logger = createLogger('vpn:protonvpn');

interface ProtonVPNServer {
  ID: string;
  Name: string;
  EntryCountry: string;
  ExitCountry: string;
  Domain: string;
  Features: number; // Bitmask: 1=SecureCore, 2=Tor, 4=P2P, 8=XOR, 16=IPv6
  Load: number;
  Score: number;
  Status: number; // 1=Online, 0=Offline
  Tier: number; // 0=Free, 1=Plus, 2=Visionary
  Servers: Array<{
    EntryIP: string;
    ExitIP: string;
    Domain: string;
    Status: number;
    X25519PublicKey?: string;
  }>;
  City?: string;
  Latitude?: number;
  Longitude?: number;
}

interface ProtonVPNServerListResponse {
  Code: number;
  LogicalServers: ProtonVPNServer[];
}

// ProtonVPN server feature bitmask values
const PROTON_FEATURE_SECURE_CORE = 1;
const PROTON_FEATURE_TOR = 2;
const PROTON_FEATURE_P2P = 4;
const PROTON_FEATURE_STREAMING = 8;

export class ProtonVPNProvider extends BaseVPNProvider {
  readonly name = 'protonvpn' as const;
  readonly displayName = 'ProtonVPN';
  readonly cliAvailable = true;
  readonly apiAvailable = true;
  readonly portForwardingSupported = false;
  readonly p2pAllServers = false;

  private readonly apiEndpoint = 'https://api.protonvpn.ch/vpn';
  private readonly cliCommand = 'protonvpn-cli';

  // ============================================================================
  // Initialization
  // ============================================================================

  protected async checkCLIInstalled(): Promise<void> {
    try {
      await this.executeCommand(`${this.cliCommand} --version`);
      logger.info('ProtonVPN CLI is installed');
    } catch (error) {
      throw new Error(
        'ProtonVPN CLI is not installed. Install from: https://protonvpn.com/support/linux-vpn-tool/'
      );
    }
  }

  protected async performAuthentication(credentials: VPNCredentialRecord): Promise<boolean> {
    try {
      // Check if already logged in
      try {
        const statusResult = await this.executeCommand(`${this.cliCommand} status`);
        if (!statusResult.stdout.toLowerCase().includes('not logged in')) {
          logger.info('ProtonVPN is already authenticated');
          return true;
        }
      } catch {
        // Not authenticated yet — proceed
      }

      if (!credentials.username || !credentials.password_encrypted) {
        throw new Error('ProtonVPN requires username and password');
      }

      // Login using CLI
      const fs = await import('fs/promises');
      const credsFile = '/tmp/protonvpn-creds.txt';
      await fs.writeFile(
        credsFile,
        `${credentials.username}\n${credentials.password_encrypted}`,
        { mode: 0o600 }
      );

      await this.executeCommand(
        `${this.cliCommand} login --username ${credentials.username}`,
        30000
      );

      await fs.unlink(credsFile).catch(() => {});

      logger.info('Successfully authenticated with ProtonVPN');
      return true;
    } catch (error) {
      logger.error('ProtonVPN authentication failed', {
        error: error instanceof Error ? error.message : String(error),
      });
      return false;
    }
  }

  // ============================================================================
  // Server Management
  // ============================================================================

  async fetchServers(): Promise<VPNServerRecord[]> {
    logger.info('Fetching ProtonVPN server list from API');

    try {
      const response = await fetch(`${this.apiEndpoint}/logicals`, {
        headers: {
          'x-pm-appversion': 'LinuxVpnCli_1.0.0',
          'User-Agent': 'LinuxVpnCli/1.0.0',
        },
      });

      if (!response.ok) {
        throw new Error(`API request failed: ${response.statusText}`);
      }

      const data: ProtonVPNServerListResponse = await response.json();

      if (data.Code !== 1000) {
        throw new Error(`ProtonVPN API returned error code: ${data.Code}`);
      }

      logger.info(`Fetched ${data.LogicalServers.length} logical servers from ProtonVPN`);

      return data.LogicalServers
        .filter((server) => server.Status === 1)
        .map((server) => this.mapServer(server));
    } catch (error) {
      logger.error('Failed to fetch ProtonVPN servers', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  private mapServer(server: ProtonVPNServer): VPNServerRecord {
    const isP2P = (server.Features & PROTON_FEATURE_P2P) !== 0;
    const isSecureCore = (server.Features & PROTON_FEATURE_SECURE_CORE) !== 0;
    const isTor = (server.Features & PROTON_FEATURE_TOR) !== 0;
    const isStreaming = (server.Features & PROTON_FEATURE_STREAMING) !== 0;

    const features: string[] = [];
    if (isP2P) features.push('p2p');
    if (isSecureCore) features.push('secure_core');
    if (isTor) features.push('tor');
    if (isStreaming) features.push('streaming');

    const protocols: VPNProtocol[] = ['wireguard', 'openvpn_udp', 'openvpn_tcp'];

    // Use the first server's IPs
    const firstServer = server.Servers[0];
    const entryIP = firstServer?.EntryIP ?? '';
    const publicKey = firstServer?.X25519PublicKey;

    return {
      id: `protonvpn-${server.ID}`,
      provider_id: 'protonvpn',
      hostname: server.Domain,
      ip_address: entryIP,
      country_code: (server.ExitCountry ?? '').toUpperCase(),
      country_name: server.ExitCountry,
      city: server.City,
      latitude: server.Latitude,
      longitude: server.Longitude,
      p2p_supported: isP2P,
      port_forwarding_supported: false,
      protocols,
      load: server.Load,
      status: server.Status === 1 ? 'online' : 'offline',
      features,
      public_key: publicKey,
      owned: true,
      metadata: {
        tier: server.Tier,
        score: server.Score,
        entry_country: server.EntryCountry,
        server_name: server.Name,
        feature_bitmask: server.Features,
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

    logger.info('Connecting to ProtonVPN', {
      region: request.region,
      server: request.server,
      protocol: request.protocol,
    });

    // Set protocol
    if (request.protocol) {
      const proto = this.mapProtocol(request.protocol);
      await this.executeCommand(`${this.cliCommand} config --protocol ${proto}`);
    }

    // Build connect command
    let connectCmd = `${this.cliCommand} connect`;

    if (request.server) {
      // Specific server name (e.g., "US-NY#1")
      connectCmd += ` --server ${request.server}`;
    } else if (request.region) {
      // Country code (e.g., "US")
      connectCmd += ` --cc ${request.region.toUpperCase()}`;
    } else if (request.kill_switch !== false) {
      // Fastest server with P2P
      connectCmd += ' --p2p';
    } else {
      // Fastest available
      connectCmd += ' --fastest';
    }

    try {
      const result = await this.executeCommand(connectCmd, 60000);
      logger.info('ProtonVPN connection established', { output: result.stdout });

      const status = await this.getStatus();

      const connection: VPNConnectionRecord = {
        id: `protonvpn-${Date.now()}`,
        provider_id: 'protonvpn',
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
      logger.error('ProtonVPN connection failed', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw new Error(`Failed to connect to ProtonVPN: ${error}`);
    }
  }

  async disconnect(connectionId: string): Promise<void> {
    logger.info('Disconnecting from ProtonVPN', { connectionId });

    try {
      await this.executeCommand(`${this.cliCommand} disconnect`);
      logger.info('Successfully disconnected from ProtonVPN');
    } catch (error) {
      logger.error('Failed to disconnect from ProtonVPN', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  async getStatus(): Promise<VPNStatus> {
    try {
      const result = await this.executeCommand(`${this.cliCommand} status`);
      return this.parseProtonVPNStatus(result.stdout);
    } catch (error) {
      logger.error('Failed to get ProtonVPN status', {
        error: error instanceof Error ? error.message : String(error),
      });
      return { connected: false };
    }
  }

  private parseProtonVPNStatus(output: string): VPNStatus {
    const lines = output.split('\n');
    const status: VPNStatus = { connected: false };

    for (const line of lines) {
      const trimmed = line.trim();

      if (trimmed.startsWith('Status:')) {
        status.connected = trimmed.toLowerCase().includes('connected');
      } else if (trimmed.startsWith('Server:') || trimmed.startsWith('Server IP:')) {
        const val = trimmed.split(':').slice(1).join(':').trim();
        if (trimmed.startsWith('Server IP:')) {
          status.vpn_ip = val;
        } else {
          status.server = val;
        }
      } else if (trimmed.startsWith('Protocol:')) {
        status.protocol = trimmed.split(':')[1]?.trim().toLowerCase();
      } else if (trimmed.startsWith('IP:')) {
        status.vpn_ip = trimmed.split(':').slice(1).join(':').trim();
      }
    }

    if (status.connected) {
      const proto = status.protocol ?? '';
      status.interface = proto.includes('wireguard') ? 'proton0' : 'tun0';
    }

    return status;
  }

  // ============================================================================
  // Kill Switch
  // ============================================================================

  async enableKillSwitch(): Promise<void> {
    logger.info('Enabling ProtonVPN kill switch');

    try {
      await this.executeCommand(`${this.cliCommand} ks --on`);
      logger.info('Kill switch enabled');
    } catch (error) {
      logger.error('Failed to enable kill switch', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  async disableKillSwitch(): Promise<void> {
    logger.info('Disabling ProtonVPN kill switch');

    try {
      await this.executeCommand(`${this.cliCommand} ks --off`);
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
   * Map generic protocol name to ProtonVPN CLI protocol string
   */
  private mapProtocol(protocol: VPNProtocol): string {
    const map: Record<string, string> = {
      wireguard: 'wireguard',
      openvpn_udp: 'udp',
      openvpn_tcp: 'tcp',
    };
    return map[protocol] ?? 'wireguard';
  }

  /**
   * Connect to a Secure Core server (double-hop via privacy-friendly country)
   */
  async connectSecureCore(exitCountry: string): Promise<void> {
    await this.executeCommand(
      `${this.cliCommand} connect --sc --cc ${exitCountry.toUpperCase()}`,
      60000
    );
  }

  /**
   * Connect via Tor over VPN
   */
  async connectTor(): Promise<void> {
    await this.executeCommand(`${this.cliCommand} connect --tor`, 90000);
  }

  /**
   * Enable NetShield (DNS-level malware/ad blocking)
   */
  async enableNetShield(level: 1 | 2 = 1): Promise<void> {
    // level 1 = block malware only, level 2 = block malware + ads
    await this.executeCommand(`${this.cliCommand} netshield --f${level}`);
  }

  /**
   * List servers for a specific country
   */
  async listServersForCountry(countryCode: string): Promise<string[]> {
    const result = await this.executeCommand(
      `${this.cliCommand} list --cc ${countryCode.toUpperCase()}`
    );
    return result.stdout
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line.length > 0);
  }
}
