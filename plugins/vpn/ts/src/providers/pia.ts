/**
 * Private Internet Access (PIA) Provider Implementation
 * Full support for PIA with port forwarding
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

const logger = createLogger('vpn:pia');

interface PIAServer {
  id: string;
  name: string;
  country: string;
  dns: string;
  port_forward: boolean;
  geo: boolean;
  servers: {
    wg?: Array<{ ip: string; cn: string }>;
    ovpnudp?: Array<{ ip: string; cn: string }>;
    ovpntcp?: Array<{ ip: string; cn: string }>;
  };
}

interface PIAServerList {
  regions: Record<string, PIAServer>;
}

export class PIAProvider extends BaseVPNProvider {
  readonly name = 'pia' as const;
  readonly displayName = 'Private Internet Access';
  readonly cliAvailable = true;
  readonly apiAvailable = true;
  readonly portForwardingSupported = true;
  readonly p2pAllServers = true;

  private readonly serverListUrl = 'https://serverlist.piaservers.net/vpninfo/servers/v6';
  private readonly cliCommand = 'piactl';

  private forwardedPort: number | null = null;

  // ============================================================================
  // Initialization
  // ============================================================================

  protected async checkCLIInstalled(): Promise<void> {
    try {
      await this.executeCommand(`${this.cliCommand} --version`);
      logger.info('PIA CLI (piactl) is installed');
    } catch (error) {
      throw new Error(
        'PIA CLI (piactl) is not installed. Install PIA desktop app from: https://www.privateinternetaccess.com/download'
      );
    }
  }

  protected async performAuthentication(credentials: VPNCredentialRecord): Promise<boolean> {
    try {
      if (!credentials.username || !credentials.password_encrypted) {
        throw new Error('PIA requires username and password');
      }

      // Create credentials file
      const credsFile = '/tmp/pia-creds.txt';
      const fs = await import('fs/promises');
      await fs.writeFile(credsFile, `${credentials.username}\n${credentials.password_encrypted}`, {
        mode: 0o600,
      });

      // Login with piactl
      await this.executeCommand(`${this.cliCommand} login ${credsFile}`);

      // Clean up credentials file
      await fs.unlink(credsFile);

      logger.info('Successfully authenticated with PIA');
      return true;
    } catch (error) {
      logger.error('PIA authentication failed', { error: error instanceof Error ? error.message : String(error) });
      return false;
    }
  }

  // ============================================================================
  // Server Management
  // ============================================================================

  async fetchServers(): Promise<VPNServerRecord[]> {
    logger.info('Fetching PIA server list from API');

    try {
      const response = await fetch(this.serverListUrl);
      if (!response.ok) {
        throw new Error(`API request failed: ${response.statusText}`);
      }

      const serverList: PIAServerList = await response.json();
      logger.info(`Fetched ${Object.keys(serverList.regions).length} regions from PIA`);

      const servers: VPNServerRecord[] = [];

      for (const [regionId, region] of Object.entries(serverList.regions)) {
        // PIA uses region IDs like "au-melbourne", "us-east", etc.
        const countryCode = regionId.split('-')[0].toUpperCase();
        const protocols: VPNProtocol[] = [];

        if (region.servers.wg) protocols.push('wireguard');
        if (region.servers.ovpnudp) protocols.push('openvpn_udp');
        if (region.servers.ovpntcp) protocols.push('openvpn_tcp');

        // Use first available server IP
        const ip =
          region.servers.wg?.[0]?.ip || region.servers.ovpnudp?.[0]?.ip || region.servers.ovpntcp?.[0]?.ip || '';

        servers.push({
          id: `pia-${regionId}`,
          provider_id: 'pia',
          hostname: region.dns,
          ip_address: ip,
          country_code: countryCode,
          country_name: region.country,
          p2p_supported: true, // All PIA servers support P2P
          port_forwarding_supported: region.port_forward,
          protocols,
          status: 'online',
          features: region.port_forward ? ['p2p', 'port_forwarding'] : ['p2p'],
          owned: false,
          metadata: { region_id: regionId, geo: region.geo },
          last_seen: new Date(),
          created_at: new Date(),
          updated_at: new Date(),
        } as VPNServerRecord);
      }

      return servers;
    } catch (error) {
      logger.error('Failed to fetch PIA servers', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  // ============================================================================
  // Connection Management
  // ============================================================================

  async connect(request: ConnectVPNRequest, credentials: VPNCredentialRecord): Promise<VPNConnectionRecord> {
    this.ensureAuthenticated();

    logger.info('Connecting to PIA', {
      region: request.region,
      protocol: request.protocol,
      port_forwarding: request.port_forwarding,
    });

    // Set protocol
    if (request.protocol) {
      const protocol = request.protocol === 'wireguard' ? 'wireguard' : 'openvpn';
      await this.executeCommand(`${this.cliCommand} set protocol ${protocol}`);
    }

    // Set region
    if (request.region) {
      await this.executeCommand(`${this.cliCommand} set region ${request.region}`);
    }

    // Enable port forwarding
    if (request.port_forwarding) {
      await this.executeCommand(`${this.cliCommand} set requestportforward true`);
    }

    // Enable background mode (keeps VPN active)
    await this.executeCommand(`${this.cliCommand} background enable`);

    // Connect
    try {
      await this.executeCommand(`${this.cliCommand} connect`, 60000); // 60 second timeout

      // Wait for connection to establish
      await new Promise((resolve) => setTimeout(resolve, 5000));

      // Get connection details
      const status = await this.getStatus();

      // Get forwarded port if enabled
      if (request.port_forwarding) {
        this.forwardedPort = await this.getForwardedPort();
      }

      logger.info('PIA connection established', { port_forwarded: this.forwardedPort });

      const connection: VPNConnectionRecord = {
        id: `pia-${Date.now()}`,
        provider_id: 'pia',
        server_id: undefined,
        protocol: (request.protocol || 'wireguard') as VPNProtocol,
        status: 'connected',
        local_ip: undefined,
        vpn_ip: status.vpn_ip,
        interface_name: status.interface,
        dns_servers: [],
        connected_at: new Date(),
        kill_switch_enabled: true, // PIA has automatic kill switch
        port_forwarded: this.forwardedPort || undefined,
        requested_by: request.requested_by,
        metadata: {},
        created_at: new Date(),
      };

      return connection;
    } catch (error) {
      logger.error('PIA connection failed', { error: error instanceof Error ? error.message : String(error) });
      throw new Error(`Failed to connect to PIA: ${error}`);
    }
  }

  async disconnect(connectionId: string): Promise<void> {
    logger.info('Disconnecting from PIA', { connectionId });

    try {
      await this.executeCommand(`${this.cliCommand} disconnect`);
      this.forwardedPort = null;
      logger.info('Successfully disconnected from PIA');
    } catch (error) {
      logger.error('Failed to disconnect from PIA', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  async getStatus(): Promise<VPNStatus> {
    try {
      const result = await this.executeCommand(`${this.cliCommand} get connectionstate`);
      const connected = result.stdout.trim() === 'Connected';

      if (!connected) {
        return { connected: false };
      }

      // Get VPN IP
      const vpnIpResult = await this.executeCommand(`${this.cliCommand} get vpnip`);
      const vpnIp = vpnIpResult.stdout.trim();

      // Get region
      const regionResult = await this.executeCommand(`${this.cliCommand} get region`);
      const region = regionResult.stdout.trim();

      // Get protocol
      const protocolResult = await this.executeCommand(`${this.cliCommand} get protocol`);
      const protocol = protocolResult.stdout.trim().toLowerCase();

      return {
        connected: true,
        vpn_ip: vpnIp,
        server: region,
        protocol,
        interface: protocol === 'wireguard' ? 'wg0' : 'tun0',
        kill_switch_enabled: true,
        port_forwarded: this.forwardedPort || undefined,
      };
    } catch (error) {
      logger.error('Failed to get PIA status', { error: error instanceof Error ? error.message : String(error) });
      return { connected: false };
    }
  }

  // ============================================================================
  // Port Forwarding
  // ============================================================================

  async getForwardedPort(): Promise<number | null> {
    try {
      const result = await this.executeCommand(`${this.cliCommand} get portforward`);
      const output = result.stdout.trim();

      // Parse port number
      if (output === 'Inactive' || output === 'Unavailable' || output === 'Failed') {
        logger.warn(`Port forwarding status: ${output}`);
        return null;
      }

      const port = parseInt(output, 10);
      if (isNaN(port)) {
        logger.warn(`Could not parse port from: ${output}`);
        return null;
      }

      logger.info(`Port forwarded: ${port}`);
      return port;
    } catch (error) {
      logger.error('Failed to get forwarded port', { error: error instanceof Error ? error.message : String(error) });
      return null;
    }
  }

  // ============================================================================
  // Kill Switch
  // ============================================================================

  async enableKillSwitch(): Promise<void> {
    logger.info('PIA has automatic kill switch (cannot be disabled)');
    // PIA's kill switch is always active and cannot be disabled
  }

  async disableKillSwitch(): Promise<void> {
    logger.warn('PIA kill switch cannot be disabled (always active for security)');
    // PIA's kill switch is always active
  }

  // ============================================================================
  // Additional Methods
  // ============================================================================

  /**
   * Get list of available regions
   */
  async getRegions(): Promise<string[]> {
    const result = await this.executeCommand(`${this.cliCommand} get regions`);
    return result.stdout
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line.length > 0);
  }

  /**
   * Set debug logging
   */
  async setDebugLogging(enabled: boolean): Promise<void> {
    await this.executeCommand(`${this.cliCommand} set debuglogging ${enabled ? 'true' : 'false'}`);
  }
}
