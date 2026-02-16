/**
 * DDNS Provider Integration
 * Supports Cloudflare and AWS Route53
 */

import Cloudflare from 'cloudflare';
import { Route53Client, ChangeResourceRecordSetsCommand, ListResourceRecordSetsCommand } from '@aws-sdk/client-route-53';
import axios from 'axios';
import { publicIpv4, publicIpv6 } from 'public-ip';

export interface DNSRecord {
  record_id: string;
  zone_id: string;
  name: string;
  type: 'A' | 'AAAA';
  content: string;
  ttl: number;
  proxied?: boolean;
}

export interface UpdateRecordRequest {
  zone_id: string;
  record_id: string;
  name: string;
  type: 'A' | 'AAAA';
  content: string;
  ttl?: number;
  proxied?: boolean;
}

/**
 * Cloudflare DDNS Provider
 */
export class CloudflareProvider {
  private client: Cloudflare;

  constructor(apiToken: string) {
    this.client = new Cloudflare({ apiToken });
  }

  // Helper to access DNS records API with proper typing
  private get dnsRecords() {
    return (this.client as { dns: { records: unknown } }).dns.records as {
      list: (params: unknown) => Promise<{ result: unknown[] }>;
      get: (zoneId: string, recordId: string) => Promise<unknown>;
      update: (zoneId: string, recordId: string, data: unknown) => Promise<unknown>;
    };
  }

  /**
   * List DNS records in a zone
   */
  async listRecords(zoneId: string, type?: 'A' | 'AAAA'): Promise<DNSRecord[]> {
    try {
      const params: { zone_id: string; type?: string } = { zone_id: zoneId };
      if (type) params.type = type;

      const response = await this.dnsRecords.list(params);

      const records = response.result as Array<{
        id: string;
        name: string;
        type: string;
        content?: string;
        ttl: number;
        proxied?: boolean;
      }>;

      return records.map(record => ({
        record_id: record.id,
        zone_id: zoneId,
        name: record.name,
        type: record.type as 'A' | 'AAAA',
        content: record.content ?? '',
        ttl: record.ttl,
        proxied: record.proxied ?? false,
      }));
    } catch (error) {
      throw new Error(`Cloudflare list records failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Get a specific DNS record
   */
  async getRecord(zoneId: string, recordId: string): Promise<DNSRecord> {
    try {
      const response = await this.dnsRecords.get(zoneId, recordId) as {
        id: string;
        name: string;
        type: string;
        content?: string;
        ttl: number;
        proxied?: boolean;
      };

      return {
        record_id: response.id,
        zone_id: zoneId,
        name: response.name,
        type: response.type as 'A' | 'AAAA',
        content: response.content ?? '',
        ttl: response.ttl,
        proxied: response.proxied ?? false,
      };
    } catch (error) {
      throw new Error(`Cloudflare get record failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Update DNS record with new IP
   */
  async updateRecord(request: UpdateRecordRequest): Promise<DNSRecord> {
    try {
      const response = await this.dnsRecords.update(request.zone_id, request.record_id, {
        type: request.type,
        name: request.name,
        content: request.content,
        ttl: request.ttl ?? 1, // 1 = auto
        proxied: request.proxied ?? false,
      }) as {
        id: string;
        name: string;
        type: string;
        content?: string;
        ttl: number;
        proxied?: boolean;
      };

      return {
        record_id: response.id,
        zone_id: request.zone_id,
        name: response.name,
        type: response.type as 'A' | 'AAAA',
        content: response.content ?? request.content,
        ttl: response.ttl,
        proxied: response.proxied ?? false,
      };
    } catch (error) {
      throw new Error(`Cloudflare update record failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Verify DNS record has propagated
   */
  async verifyRecord(name: string, expectedIp: string): Promise<boolean> {
    try {
      // Use Cloudflare DNS resolver 1.1.1.1
      const response = await axios.get(`https://1.1.1.1/dns-query`, {
        params: {
          name,
          type: 'A',
        },
        headers: {
          'Accept': 'application/dns-json',
        },
      });

      const answers = response.data.Answer ?? [];
      return answers.some((answer: { data: string }) => answer.data === expectedIp);
    } catch (error) {
      return false;
    }
  }
}

/**
 * AWS Route53 DDNS Provider
 */
export class Route53Provider {
  private client: Route53Client;

  constructor(region = 'us-east-1', credentials?: { accessKeyId: string; secretAccessKey: string }) {
    this.client = new Route53Client({
      region,
      credentials,
    });
  }

  /**
   * List DNS records in a hosted zone
   */
  async listRecords(hostedZoneId: string, type?: 'A' | 'AAAA'): Promise<DNSRecord[]> {
    try {
      const command = new ListResourceRecordSetsCommand({
        HostedZoneId: hostedZoneId,
      });

      const response = await this.client.send(command);

      const records = response.ResourceRecordSets ?? [];

      return records
        .filter(record => !type || record.Type === type)
        .filter(record => record.Type === 'A' || record.Type === 'AAAA')
        .map(record => ({
          record_id: record.Name!, // Route53 uses name as ID
          zone_id: hostedZoneId,
          name: record.Name!,
          type: record.Type as 'A' | 'AAAA',
          content: record.ResourceRecords?.[0]?.Value ?? '',
          ttl: record.TTL ?? 300,
        }));
    } catch (error) {
      throw new Error(`Route53 list records failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Update DNS record
   */
  async updateRecord(request: UpdateRecordRequest): Promise<DNSRecord> {
    try {
      const command = new ChangeResourceRecordSetsCommand({
        HostedZoneId: request.zone_id,
        ChangeBatch: {
          Changes: [
            {
              Action: 'UPSERT',
              ResourceRecordSet: {
                Name: request.name,
                Type: request.type,
                TTL: request.ttl ?? 300,
                ResourceRecords: [{ Value: request.content }],
              },
            },
          ],
        },
      });

      await this.client.send(command);

      return {
        record_id: request.name,
        zone_id: request.zone_id,
        name: request.name,
        type: request.type,
        content: request.content,
        ttl: request.ttl ?? 300,
      };
    } catch (error) {
      throw new Error(`Route53 update record failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Verify DNS record
   */
  async verifyRecord(name: string, expectedIp: string): Promise<boolean> {
    try {
      const response = await axios.get(`https://dns.google/resolve`, {
        params: {
          name,
          type: 'A',
        },
      });

      const answers = response.data.Answer ?? [];
      return answers.some((answer: { data: string }) => answer.data === expectedIp);
    } catch (error) {
      return false;
    }
  }
}

/**
 * IP Detection Utilities
 */
export class IPDetector {
  /**
   * Get current public IPv4 address
   */
  async getIPv4(): Promise<string> {
    try {
      return await publicIpv4();
    } catch (error) {
      throw new Error(`IPv4 detection failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Get current public IPv6 address
   */
  async getIPv6(): Promise<string | null> {
    try {
      return await publicIpv6();
    } catch (error) {
      // IPv6 may not be available
      return null;
    }
  }

  /**
   * Get IP from external service (fallback)
   */
  async getIPFromService(): Promise<string> {
    try {
      const response = await axios.get('https://api.ipify.org?format=json');
      return response.data.ip;
    } catch (error) {
      throw new Error(`IP detection via service failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }
}

/**
 * Provider factory
 */
export function createDDNSProvider(
  provider: string,
  config: Record<string, string>
): CloudflareProvider | Route53Provider {
  switch (provider.toLowerCase()) {
    case 'cloudflare':
      if (!config.DDNS_CLOUDFLARE_API_TOKEN) {
        throw new Error('DDNS_CLOUDFLARE_API_TOKEN is required for Cloudflare provider');
      }
      return new CloudflareProvider(config.DDNS_CLOUDFLARE_API_TOKEN);

    case 'route53':
      if (!config.DDNS_AWS_ACCESS_KEY_ID || !config.DDNS_AWS_SECRET_ACCESS_KEY) {
        throw new Error('AWS credentials required for Route53 provider');
      }
      return new Route53Provider(config.DDNS_AWS_REGION ?? 'us-east-1', {
        accessKeyId: config.DDNS_AWS_ACCESS_KEY_ID,
        secretAccessKey: config.DDNS_AWS_SECRET_ACCESS_KEY,
      });

    default:
      throw new Error(`Unsupported DDNS provider: ${provider}`);
  }
}
