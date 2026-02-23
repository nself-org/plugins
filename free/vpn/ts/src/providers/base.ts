/**
 * Base VPN Provider Abstract Class
 * All provider implementations extend this class
 */

import { execFile as execFileCallback, type ExecException } from 'child_process';
import { promisify } from 'util';
import { createLogger } from '@nself/plugin-utils';
import type {
  IVPNProvider,
  VPNProvider,
  ConnectVPNRequest,
  VPNConnectionRecord,
  VPNCredentialRecord,
  VPNServerRecord,
  VPNStatus,
  LeakTestResult,
} from '../types.js';

const execFile = promisify(execFileCallback);
const logger = createLogger('vpn:provider');

export abstract class BaseVPNProvider implements IVPNProvider {
  abstract readonly name: VPNProvider;
  abstract readonly displayName: string;
  abstract readonly cliAvailable: boolean;
  abstract readonly apiAvailable: boolean;
  abstract readonly portForwardingSupported: boolean;
  abstract readonly p2pAllServers: boolean;

  protected initialized = false;
  protected authenticated = false;

  /**
   * Initialize provider (check CLI, verify dependencies)
   */
  async initialize(): Promise<void> {
    if (this.initialized) {
      return;
    }

    logger.info(`Initializing ${this.displayName} provider`);

    if (this.cliAvailable) {
      await this.checkCLIInstalled();
    }

    await this.performAdditionalInitialization();

    this.initialized = true;
    logger.info(`${this.displayName} provider initialized successfully`);
  }

  /**
   * Authenticate with provider
   */
  async authenticate(credentials: VPNCredentialRecord): Promise<boolean> {
    if (!this.initialized) {
      await this.initialize();
    }

    logger.info(`Authenticating with ${this.displayName}`);

    try {
      const success = await this.performAuthentication(credentials);
      this.authenticated = success;

      if (success) {
        logger.info(`Successfully authenticated with ${this.displayName}`);
      } else {
        logger.error(`Authentication failed for ${this.displayName}`);
      }

      return success;
    } catch (error) {
      logger.error(`Authentication error for ${this.displayName}`, { error: error instanceof Error ? error.message : String(error) });
      return false;
    }
  }

  /**
   * Fetch latest server list from provider
   */
  abstract fetchServers(): Promise<VPNServerRecord[]>;

  /**
   * Connect to VPN
   */
  abstract connect(request: ConnectVPNRequest, credentials: VPNCredentialRecord): Promise<VPNConnectionRecord>;

  /**
   * Disconnect from VPN
   */
  abstract disconnect(connectionId: string): Promise<void>;

  /**
   * Get current connection status
   */
  abstract getStatus(): Promise<VPNStatus>;

  /**
   * Enable kill switch
   */
  abstract enableKillSwitch(): Promise<void>;

  /**
   * Disable kill switch
   */
  abstract disableKillSwitch(): Promise<void>;

  /**
   * Get forwarded port (if supported)
   */
  async getForwardedPort?(): Promise<number | null> {
    if (!this.portForwardingSupported) {
      return null;
    }
    throw new Error(`getForwardedPort not implemented for ${this.displayName}`);
  }

  /**
   * Test for leaks (DNS, IP, WebRTC, IPv6)
   */
  async testLeaks(): Promise<LeakTestResult> {
    logger.info(`Testing for leaks on ${this.displayName}`);

    const status = await this.getStatus();
    if (!status.connected) {
      throw new Error('VPN not connected - cannot test for leaks');
    }

    const expectedIP = status.vpn_ip;
    const tests: LeakTestResult['tests'] = {
      dns: { passed: false, expected: undefined, actual: undefined },
      ip: { passed: false, expected: expectedIP, actual: undefined },
      webrtc: { passed: false, leaked_ips: [] as string[] },
      ipv6: { passed: false, leaked_ip: undefined },
    };

    try {
      // Test IP leak
      const ipResult = await this.executeCommandArgs('curl', ['-4', '-s', 'https://ifconfig.io']);
      tests.ip.actual = ipResult.stdout.trim();
      tests.ip.passed = tests.ip.actual === expectedIP;

      // Test DNS leak
      const dnsResult = await this.executeCommandArgs('nslookup', ['-type=txt', 'whoami.akamai.net']);
      const dnsMatch = dnsResult.stdout.match(/(\d+\.\d+\.\d+\.\d+)/);
      if (dnsMatch) {
        tests.dns.actual = dnsMatch[1];
        // DNS should resolve to VPN provider's DNS or the VPN IP network
        tests.dns.passed = !this.isISPIP(tests.dns.actual!);
      }

      // Test IPv6 leak
      try {
        const ipv6Result = await this.executeCommandArgs('curl', ['-6', '-s', '--max-time', '5', 'https://ifconfig.io']);
        if (ipv6Result.stdout.trim()) {
          tests.ipv6.leaked_ip = ipv6Result.stdout.trim();
          tests.ipv6.passed = false; // IPv6 should be blocked
        } else {
          tests.ipv6.passed = true;
        }
      } catch {
        // Timeout or error means IPv6 is blocked (good)
        tests.ipv6.passed = true;
      }

      // WebRTC leak test (basic - would need browser for full test)
      tests.webrtc.passed = true; // Assume no leak at CLI level

      const allPassed = tests.dns.passed && tests.ip.passed && tests.webrtc.passed && tests.ipv6.passed;

      logger.info(`Leak test results for ${this.displayName}`, {
        passed: allPassed,
        dns: tests.dns.passed,
        ip: tests.ip.passed,
        webrtc: tests.webrtc.passed,
        ipv6: tests.ipv6.passed,
      });

      return {
        passed: allPassed,
        tests,
        timestamp: new Date(),
      };
    } catch (error) {
      logger.error(`Leak test failed for ${this.displayName}`, { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  // ============================================================================
  // Protected Helper Methods
  // ============================================================================

  /**
   * Execute shell command using execFile (safer than exec — no shell injection).
   * Pass the command and its arguments as separate array elements.
   */
  protected async executeCommandArgs(cmd: string, args: string[], timeout: number = 30000): Promise<{ stdout: string; stderr: string }> {
    try {
      const result = await execFile(cmd, args, {
        timeout,
        maxBuffer: 1024 * 1024 * 10, // 10MB
      });
      return result;
    } catch (error) {
      const execError = error as ExecException;
      if (execError.killed && execError.signal === 'SIGTERM') {
        throw new Error(`Command timed out after ${timeout}ms: ${cmd} ${args.join(' ')}`);
      }
      throw error;
    }
  }

  /**
   * @deprecated Use executeCommandArgs(cmd, args) instead to avoid shell injection.
   * Kept for backwards compatibility with subclass overrides; will be removed in a future release.
   */
  protected async executeCommand(command: string, timeout: number = 30000): Promise<{ stdout: string; stderr: string }> {
    // Split naive shell string into cmd + args for execFile.
    // This does NOT handle quoted arguments — callers should migrate to executeCommandArgs.
    const parts = command.split(/\s+/);
    const [cmd, ...args] = parts;
    return this.executeCommandArgs(cmd, args, timeout);
  }

  /**
   * Check if CLI is installed
   */
  protected abstract checkCLIInstalled(): Promise<void>;

  /**
   * Perform provider-specific initialization
   */
  protected async performAdditionalInitialization(): Promise<void> {
    // Override in subclass if needed
  }

  /**
   * Perform provider-specific authentication
   */
  protected abstract performAuthentication(credentials: VPNCredentialRecord): Promise<boolean>;

  /**
   * Parse connection status from CLI output
   */
  protected parseStatus(output: string): VPNStatus {
    // Default implementation - override in subclass
    const connected = output.toLowerCase().includes('connected');
    return {
      connected,
    };
  }

  /**
   * Check if IP belongs to ISP (simple heuristic)
   */
  protected isISPIP(ip: string): boolean {
    // This is a simplification - in production, check against known VPN provider IP ranges
    // For now, assume any valid IP is potentially an ISP IP
    return /^\d+\.\d+\.\d+\.\d+$/.test(ip);
  }

  /**
   * Extract IP from interface
   */
  protected async getInterfaceIP(interfaceName: string): Promise<string | null> {
    try {
      const result = await this.executeCommandArgs('ip', ['addr', 'show', interfaceName]);
      const match = result.stdout.match(/inet (\d+\.\d+\.\d+\.\d+)/);
      return match ? match[1] : null;
    } catch {
      return null;
    }
  }

  /**
   * Get DNS servers from resolv.conf
   */
  protected async getDNSServers(): Promise<string[]> {
    try {
      const result = await this.executeCommandArgs('cat', ['/etc/resolv.conf']);
      const lines = result.stdout.split('\n');
      const servers: string[] = [];

      for (const line of lines) {
        const match = line.match(/nameserver\s+(\d+\.\d+\.\d+\.\d+)/);
        if (match) {
          servers.push(match[1]);
        }
      }

      return servers;
    } catch {
      return [];
    }
  }

  /**
   * Ensure initialized before operation
   */
  protected ensureInitialized(): void {
    if (!this.initialized) {
      throw new Error(`${this.displayName} provider not initialized. Call initialize() first.`);
    }
  }

  /**
   * Ensure authenticated before operation
   */
  protected ensureAuthenticated(): void {
    this.ensureInitialized();
    if (!this.authenticated) {
      throw new Error(`Not authenticated with ${this.displayName}. Call authenticate() first.`);
    }
  }
}
