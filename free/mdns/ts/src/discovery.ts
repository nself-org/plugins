/**
 * mDNS Discovery Module
 * Real multicast DNS service discovery using multicast-dns package
 */

import mdns from 'multicast-dns';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('mdns:discovery');

export interface DiscoveredService {
  service_type: string;
  service_name: string;
  host: string;
  port: number;
  addresses: string[];
  txt_records: Record<string, string>;
}

export interface DiscoveryOptions {
  serviceType?: string;
  timeout?: number; // milliseconds
  domain?: string;
}

/**
 * mDNS Service Browser
 * Discovers services on the local network using multicast DNS
 */
export class MDNSBrowser {
  private mdnsInstance: ReturnType<typeof mdns> | null = null;

  /**
   * Discover services on the network
   */
  async discover(options: DiscoveryOptions = {}): Promise<DiscoveredService[]> {
    const {
      serviceType = '_http._tcp',
      timeout = 5000,
      domain = 'local',
    } = options;

    logger.info('Starting mDNS discovery', { serviceType, timeout, domain });

    return new Promise((resolve) => {
      const discovered = new Map<string, DiscoveredService>();
      const mdnsClient = mdns();

      // Handle responses
      mdnsClient.on('response', (response: {
        answers?: Array<{
          name: string;
          type: string;
          data?: unknown;
          ttl?: number;
        }>;
        additionals?: Array<{
          name: string;
          type: string;
          data?: unknown;
          ttl?: number;
        }>;
      }) => {
        try {
          // Process PTR records (service discovery)
          const ptrRecords = response.answers?.filter(a => a.type === 'PTR') ?? [];

          for (const ptr of ptrRecords) {
            if (typeof ptr.data !== 'string') continue;

            const serviceName = ptr.data;

            // Find SRV record for port and host
            const srvRecord = response.additionals?.find(
              a => a.name === serviceName && a.type === 'SRV'
            );

            if (!srvRecord || typeof srvRecord.data !== 'object' || !srvRecord.data) continue;

            const srvData = srvRecord.data as {
              target?: string;
              port?: number;
              priority?: number;
              weight?: number;
            };

            const host = srvData.target ?? '';
            const port = srvData.port ?? 0;

            // Find A/AAAA records for IP addresses
            const addresses: string[] = [];
            const aRecords = response.additionals?.filter(
              a => (a.type === 'A' || a.type === 'AAAA') && a.name === host
            ) ?? [];

            for (const aRecord of aRecords) {
              if (typeof aRecord.data === 'string') {
                addresses.push(aRecord.data);
              }
            }

            // Find TXT records for additional info
            const txtRecords: Record<string, string> = {};
            const txtRecord = response.additionals?.find(
              a => a.name === serviceName && a.type === 'TXT'
            );

            if (txtRecord && Array.isArray(txtRecord.data)) {
              for (const entry of txtRecord.data) {
                if (typeof entry === 'string' || (entry instanceof Buffer)) {
                  const text = typeof entry === 'string' ? entry : entry.toString();
                  const [key, ...valueParts] = text.split('=');
                  if (key) {
                    txtRecords[key] = valueParts.join('=') || '';
                  }
                }
              }
            }

            // Extract service type from service name
            const serviceTypeMatch = serviceName.match(/(_[^.]+\._[^.]+)/);
            const extractedServiceType = serviceTypeMatch ? serviceTypeMatch[1] : serviceType;

            // Only add if service type matches filter
            if (serviceType === '_services._dns-sd._udp' || serviceName.includes(serviceType)) {
              const key = `${serviceName}@${host}:${port}`;

              discovered.set(key, {
                service_type: extractedServiceType,
                service_name: serviceName,
                host,
                port,
                addresses,
                txt_records: txtRecords,
              });

              logger.debug('Service discovered', {
                service_name: serviceName,
                host,
                port,
                addresses,
              });
            }
          }
        } catch (error) {
          logger.error('Error processing mDNS response', {
            error: error instanceof Error ? error.message : 'Unknown error',
          });
        }
      });

      // Query for services
      const query = {
        questions: [
          {
            name: serviceType + '.' + domain,
            type: 'PTR' as const,
          },
        ],
      };

      mdnsClient.query(query as never);

      // Set timeout to stop discovery
      setTimeout(() => {
        mdnsClient.destroy();
        const services = Array.from(discovered.values());
        logger.info('Discovery completed', { count: services.length });
        resolve(services);
      }, timeout);
    });
  }

  /**
   * Cleanup resources
   */
  destroy(): void {
    if (this.mdnsInstance) {
      this.mdnsInstance.destroy();
      this.mdnsInstance = null;
    }
  }
}

/**
 * Discover all services on the network
 */
export async function discoverServices(options: DiscoveryOptions = {}): Promise<DiscoveredService[]> {
  const browser = new MDNSBrowser();
  try {
    return await browser.discover(options);
  } finally {
    browser.destroy();
  }
}
