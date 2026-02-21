/**
 * Mullvad VPN Provider Implementation
 * Account-number based authentication, WireGuard focus
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

const logger = createLogger('vpn:mullvad');

interface MullvadRelay {
  hostname: string;
  ipv4_addr_in: string;
  ipv6_addr_in?: string;
  public_key: string;
  multihop_port: number;
  socks_port: number;
  socks_name: string;
  active: boolean;
  owned: boolean;
  provider: string;
  type: string;
  daita: boolean;
}

interface MullvadCity {
  name: string;
  code: string;
  latitude: number;
  longitude: number;
  relays: MullvadRelay[];
}

interface MullvadCountry {
  name: string;
  code: string;
  cities: MullvadCity[];
}

interface MullvadServerList {
  countries: MullvadCountry[];
}

export class MullvadProvider extends BaseVPNProvider {
  readonly name = 'mullvad' as const;
  readonly displayName = 'Mullvad VPN';
  readonly cliAvailable = true;
  readonly apiAvailable = true;
  readonly portForwardingSupported = false; // Removed July 2023
  readonly p2pAllServers = true;

  private readonly serverListUrl = 'https://api.mullvad.net/www/relays/all/';
  private readonly cliCommand = 'mullvad';

  // ============================================================================
  // Initialization
  // ============================================================================

  protected async checkCLIInstalled(): Promise<void> {
    try {
      await this.executeCommand(`${this.cliCommand} version`);
      logger.info('Mullvad CLI is installed');
    } catch (error) {
      throw new Error(
        'Mullvad CLI is not installed. Install from: https://mullvad.net/en/download'
      );
    }
  }

  protected async performAuthentication(credentials: VPNCredentialRecord): Promise<boolean> {
    try {
      if (!credentials.account_number) {
        throw new Error('Mullvad requires account number (16 digits)');
      }

      // Login with account number
      await this.executeCommand(`${this.cliCommand} account login ${credentials.account_number}`);

      // Verify login
      const accountResult = await this.executeCommand(`${this.cliCommand} account get`);
      if (!accountResult.stdout.includes('Account:')) {
        throw new Error('Failed to verify account');
      }

      logger.info('Successfully authenticated with Mullvad');
      return true;
    } catch (error) {
      logger.error('Mullvad authentication failed', { error: error instanceof Error ? error.message : String(error) });
      return false;
    }
  }

  // ============================================================================
  // Server Management
  // ============================================================================

  async fetchServers(): Promise<VPNServerRecord[]> {
    logger.info('Fetching Mullvad server list from API');

    try {
      const response = await fetch(this.serverListUrl);
      if (!response.ok) {
        throw new Error(`API request failed: ${response.statusText}`);
      }

      const serverList: MullvadServerList = await response.json();
      logger.info(`Fetched ${serverList.countries.length} countries from Mullvad`);

      const servers: VPNServerRecord[] = [];

      for (const country of serverList.countries) {
        for (const city of country.cities) {
          for (const relay of city.relays) {
            if (!relay.active || relay.type !== 'wireguard') {
              continue; // Skip inactive or non-WireGuard relays
            }

            const protocols: VPNProtocol[] = ['wireguard'];

            servers.push({
              id: `mullvad-${relay.hostname}`,
              provider_id: 'mullvad',
              hostname: relay.hostname,
              ip_address: relay.ipv4_addr_in,
              ipv6_address: relay.ipv6_addr_in,
              country_code: country.code.toUpperCase(),
              country_name: country.name,
              city: city.name,
              latitude: city.latitude,
              longitude: city.longitude,
              p2p_supported: true, // All Mullvad servers support P2P
              port_forwarding_supported: false, // Removed July 2023
              protocols,
              status: relay.active ? 'online' : 'offline',
              features: relay.daita ? ['p2p', 'daita'] : ['p2p'],
              public_key: relay.public_key,
              endpoint_port: relay.multihop_port,
              owned: relay.owned,
              metadata: {
                provider: relay.provider,
                socks_port: relay.socks_port,
                socks_name: relay.socks_name,
              },
              last_seen: new Date(),
              created_at: new Date(),
              updated_at: new Date(),
            } as VPNServerRecord);
          }
        }
      }

      return servers;
    } catch (error) {
      logger.error('Failed to fetch Mullvad servers', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  // ============================================================================
  // Connection Management
  // ============================================================================

  async connect(request: ConnectVPNRequest, credentials: VPNCredentialRecord): Promise<VPNConnectionRecord> {
    this.ensureAuthenticated();

    logger.info('Connecting to Mullvad', {
      region: request.region,
      city: request.city,
      server: request.server,
    });

    // Set protocol to WireGuard
    await this.executeCommand(`${this.cliCommand} relay set tunnel-protocol wireguard`);

    // Set location
    if (request.server) {
      // Specific server (e.g., "us-nyc-wg-001")
      await this.executeCommand(`${this.cliCommand} relay set location ${request.server}`);
    } else if (request.city && request.region) {
      // City + country (e.g., "au syd")
      await this.executeCommand(`${this.cliCommand} relay set location ${request.region} ${request.city}`);
    } else if (request.region) {
      // Country only (e.g., "au")
      await this.executeCommand(`${this.cliCommand} relay set location ${request.region}`);
    }

    // Enable auto-connect
    await this.executeCommand(`${this.cliCommand} auto-connect set on`);

    // Enable lockdown mode (kill switch)
    if (request.kill_switch !== false) {
      await this.enableKillSwitch();
    }

    // Set DNS
    await this.executeCommand(`${this.cliCommand} dns set default --block-ads --block-trackers`);

    // Connect
    try {
      await this.executeCommand(`${this.cliCommand} connect`, 60000); // 60 second timeout

      // Wait for connection to establish
      await new Promise((resolve) => setTimeout(resolve, 3000));

      // Get connection details
      const status = await this.getStatus();

      logger.info('Mullvad connection established');

      const connection: VPNConnectionRecord = {
        id: `mullvad-${Date.now()}`,
        provider_id: 'mullvad',
        server_id: undefined,
        protocol: 'wireguard',
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
      logger.error('Mullvad connection failed', { error: error instanceof Error ? error.message : String(error) });
      throw new Error(`Failed to connect to Mullvad: ${error}`);
    }
  }

  async disconnect(connectionId: string): Promise<void> {
    logger.info('Disconnecting from Mullvad', { connectionId });

    try {
      await this.executeCommand(`${this.cliCommand} disconnect`);
      logger.info('Successfully disconnected from Mullvad');
    } catch (error) {
      logger.error('Failed to disconnect from Mullvad', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  async getStatus(): Promise<VPNStatus> {
    try {
      const result = await this.executeCommand(`${this.cliCommand} status`);
      const output = result.stdout;

      const lines = output.split('\n');
      const status: VPNStatus = {
        connected: false,
      };

      for (const line of lines) {
        const trimmed = line.trim();

        if (trimmed.startsWith('Connected:') || trimmed === 'Connected') {
          status.connected = true;
        } else if (trimmed.startsWith('Location:')) {
          const location = trimmed.split(':')[1]?.trim();
          if (location) {
            status.server = location;
          }
        } else if (trimmed.startsWith('Tunnel protocol:')) {
          const protocol = trimmed.split(':')[1]?.trim().toLowerCase();
          if (protocol) {
            status.protocol = protocol;
          }
        }
      }

      if (status.connected) {
        // Get interface IP
        status.interface = 'wg0-mullvad';
        status.vpn_ip = (await this.getInterfaceIP(status.interface)) ?? undefined;
      }

      return status;
    } catch (error) {
      logger.error('Failed to get Mullvad status', { error: error instanceof Error ? error.message : String(error) });
      return { connected: false };
    }
  }

  // ============================================================================
  // Kill Switch
  // ============================================================================

  async enableKillSwitch(): Promise<void> {
    logger.info('Enabling Mullvad lockdown mode (kill switch)');

    try {
      await this.executeCommand(`${this.cliCommand} lockdown-mode set on`);
      logger.info('Lockdown mode enabled');
    } catch (error) {
      logger.error('Failed to enable lockdown mode', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  async disableKillSwitch(): Promise<void> {
    logger.info('Disabling Mullvad lockdown mode');

    try {
      await this.executeCommand(`${this.cliCommand} lockdown-mode set off`);
      logger.info('Lockdown mode disabled');
    } catch (error) {
      logger.error('Failed to disable lockdown mode', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  // ============================================================================
  // Additional Methods
  // ============================================================================

  /**
   * Enable DAITA (Defense Against AI-Guided Traffic Analysis)
   */
  async enableDAITA(): Promise<void> {
    logger.info('Enabling DAITA');
    await this.executeCommand(`${this.cliCommand} tunnel set daita on`);
  }

  /**
   * Set automatic WireGuard key rotation interval
   */
  async setKeyRotation(hours: number): Promise<void> {
    logger.info(`Setting key rotation to ${hours} hours`);
    await this.executeCommand(`${this.cliCommand} tunnel set rotation-interval ${hours}`);
  }

  /**
   * Manually rotate WireGuard keys
   */
  async rotateKeys(): Promise<void> {
    logger.info('Rotating WireGuard keys');
    await this.executeCommand(`${this.cliCommand} tunnel set rotate-key`);
  }

  /**
   * List available relay locations
   */
  async getRelayList(): Promise<string[]> {
    const result = await this.executeCommand(`${this.cliCommand} relay list`);
    return result.stdout
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line.length > 0);
  }

  /**
   * Allow LAN access
   */
  async allowLAN(): Promise<void> {
    await this.executeCommand(`${this.cliCommand} lan set allow`);
  }

  /**
   * Block LAN access
   */
  async blockLAN(): Promise<void> {
    await this.executeCommand(`${this.cliCommand} lan set block`);
  }
}
