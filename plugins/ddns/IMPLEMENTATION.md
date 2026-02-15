# DDNS Plugin - Full Implementation Guide

## Overview

The DDNS plugin provides dynamic DNS updates for **Cloudflare** and **AWS Route53**, automatically keeping DNS records in sync with changing IP addresses. Essential for self-hosted services on residential connections.

## Current Status

**Infrastructure Status**: ✅ Complete (database, API endpoints, IP monitoring)
**Provider Integration Status**: ⚠️ Placeholder (requires API implementation)

## What's Already Built

- ✅ Database schema for records, IP history, update logs
- ✅ REST API endpoints
- ✅ IP change detection logic
- ✅ Multi-provider support architecture
- ✅ Retry logic for failed updates

## What Needs Implementation

**Provider API Integration**:
- `updateRecord()` - Update DNS A/AAAA record
- `getRecord()` - Fetch current DNS record
- `listRecords()` - List all DNS records in zone
- `verifyUpdate()` - Confirm DNS propagation

---

## Required Packages

```bash
# Cloudflare API client
pnpm add cloudflare

# AWS SDK for Route53
pnpm add @aws-sdk/client-route-53

# IP detection
pnpm add axios public-ip
```

---

## Complete Implementation Code

### 1. Provider Integration Module

Create `ts/src/providers.ts`:

```typescript
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

  /**
   * List DNS records in a zone
   */
  async listRecords(zoneId: string, type?: 'A' | 'AAAA'): Promise<DNSRecord[]> {
    try {
      const params: Record<string, unknown> = {};
      if (type) params.type = type;

      const response = await this.client.dns.records.list({
        zone_id: zoneId,
        ...params,
      });

      return response.result.map(record => ({
        record_id: record.id,
        zone_id: record.zone_id,
        name: record.name,
        type: record.type as 'A' | 'AAAA',
        content: record.content,
        ttl: record.ttl,
        proxied: record.proxied,
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
      const response = await this.client.dns.records.get({
        zone_id: zoneId,
        dns_record_id: recordId,
      });

      return {
        record_id: response.id,
        zone_id: response.zone_id,
        name: response.name,
        type: response.type as 'A' | 'AAAA',
        content: response.content,
        ttl: response.ttl,
        proxied: response.proxied,
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
      const response = await this.client.dns.records.update({
        zone_id: request.zone_id,
        dns_record_id: request.record_id,
        type: request.type,
        name: request.name,
        content: request.content,
        ttl: request.ttl ?? 1, // 1 = auto
        proxied: request.proxied ?? false,
      });

      return {
        record_id: response.id,
        zone_id: response.zone_id,
        name: response.name,
        type: response.type as 'A' | 'AAAA',
        content: response.content,
        ttl: response.ttl,
        proxied: response.proxied,
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
```

### 2. Update Server to Use Providers

Modify `ts/src/server.ts`:

```typescript
import { createDDNSProvider, IPDetector } from './providers.js';

// In createServer():
const ddnsProvider = createDDNSProvider(
  fullConfig.provider,
  process.env as Record<string, string>
);

const ipDetector = new IPDetector();

// Add IP check endpoint:
app.get('/api/ip', async (_request, reply) => {
  try {
    const ipv4 = await ipDetector.getIPv4();
    const ipv6 = await ipDetector.getIPv6();

    return {
      ipv4,
      ipv6,
      timestamp: new Date().toISOString(),
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    return reply.status(500).send({ error: message });
  }
});

// Add update endpoint:
app.post('/api/update', async (request, reply) => {
  try {
    const body = request.body as { record_id: string };

    // Get record from database
    const record = await scopedDb(request).getRecord(body.record_id);
    if (!record) {
      return reply.status(404).send({ error: 'Record not found' });
    }

    // Detect current IP
    const currentIp = record.record_type === 'A'
      ? await ipDetector.getIPv4()
      : await ipDetector.getIPv6();

    if (!currentIp) {
      return reply.status(500).send({ error: 'Could not detect IP address' });
    }

    // Check if IP changed
    if (record.current_ip === currentIp) {
      return { updated: false, message: 'IP unchanged', current_ip: currentIp };
    }

    // Update DNS record via provider
    const updatedRecord = await ddnsProvider.updateRecord({
      zone_id: record.zone_id,
      record_id: record.provider_record_id,
      name: record.hostname,
      type: record.record_type as 'A' | 'AAAA',
      content: currentIp,
      ttl: record.ttl,
    });

    // Log update in database
    await scopedDb(request).logUpdate(
      record.id,
      record.current_ip ?? '',
      currentIp,
      'success'
    );

    // Update record in database
    await scopedDb(request).updateRecordIP(record.id, currentIp);

    // Verify propagation
    const verified = await ddnsProvider.verifyRecord(record.hostname, currentIp);

    return {
      updated: true,
      record_id: record.id,
      hostname: record.hostname,
      old_ip: record.current_ip,
      new_ip: currentIp,
      verified,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Update failed', { error: message });
    return reply.status(500).send({ error: message });
  }
});

// Add batch update endpoint:
app.post('/api/update-all', async (request, reply) => {
  try {
    const records = await scopedDb(request).listRecords(true); // Active only

    const results: Array<{ record_id: string; updated: boolean; error?: string }> = [];

    for (const record of records) {
      try {
        const currentIp = record.record_type === 'A'
          ? await ipDetector.getIPv4()
          : await ipDetector.getIPv6();

        if (!currentIp) {
          results.push({ record_id: record.id, updated: false, error: 'IP detection failed' });
          continue;
        }

        if (record.current_ip === currentIp) {
          results.push({ record_id: record.id, updated: false });
          continue;
        }

        await ddnsProvider.updateRecord({
          zone_id: record.zone_id,
          record_id: record.provider_record_id,
          name: record.hostname,
          type: record.record_type as 'A' | 'AAAA',
          content: currentIp,
          ttl: record.ttl,
        });

        await scopedDb(request).logUpdate(record.id, record.current_ip ?? '', currentIp, 'success');
        await scopedDb(request).updateRecordIP(record.id, currentIp);

        results.push({ record_id: record.id, updated: true });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        results.push({ record_id: record.id, updated: false, error: message });
        await scopedDb(request).logUpdate(record.id, '', '', 'failed', [message]);
      }
    }

    return {
      total: results.length,
      updated: results.filter(r => r.updated).length,
      failed: results.filter(r => r.error).length,
      results,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Batch update failed', { error: message });
    return reply.status(500).send({ error: message });
  }
});
```

---

## Configuration

### Environment Variables

**Cloudflare**:
```bash
DDNS_PROVIDER=cloudflare
DDNS_CLOUDFLARE_API_TOKEN=your_cloudflare_api_token
DDNS_CLOUDFLARE_ZONE_IDS=zone_id_1,zone_id_2
DDNS_UPDATE_INTERVAL=300000  # 5 minutes
```

**AWS Route53**:
```bash
DDNS_PROVIDER=route53
DDNS_AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
DDNS_AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
DDNS_AWS_REGION=us-east-1
DDNS_ROUTE53_HOSTED_ZONE_IDS=Z1234567890ABC
```

### Get API Credentials

**Cloudflare**:
1. Log in to Cloudflare Dashboard
2. Go to **My Profile** → **API Tokens**
3. Create token with **Zone.DNS.Edit** permission
4. Get Zone ID from domain overview page

**AWS Route53**:
1. Go to [AWS IAM Console](https://console.aws.amazon.com/iam/)
2. Create user with **Route53FullAccess** policy
3. Create access key
4. Get Hosted Zone ID from Route53 dashboard

---

## Testing

```bash
cd plugins/ddns/ts
pnpm install
pnpm add cloudflare @aws-sdk/client-route-53 axios public-ip
pnpm build
pnpm start
```

**Check Current IP**:
```bash
curl http://localhost:3000/api/ip \
  -H "X-API-Key: test-key"
```

**Update Single Record**:
```bash
curl -X POST http://localhost:3000/api/update \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -d '{"record_id": "record_uuid"}'
```

**Update All Records**:
```bash
curl -X POST http://localhost:3000/api/update-all \
  -H "X-API-Key: test-key"
```

---

## Activation Checklist

- [ ] Install dependencies: `pnpm add cloudflare @aws-sdk/client-route-53 axios public-ip`
- [ ] Create `providers.ts`
- [ ] Update `server.ts` with update endpoints
- [ ] Add API credentials to `.env`
- [ ] Build: `pnpm build`
- [ ] Start: `pnpm start`
- [ ] Test IP detection
- [ ] Test DNS update
- [ ] Set up cron job for periodic updates

---

## Cron Job Setup

**Update DNS every 5 minutes**:

```bash
# Add to crontab
*/5 * * * * curl -X POST http://localhost:3000/api/update-all -H "X-API-Key: your-api-key"
```

Or use systemd timer for more robust scheduling.

---

## Use Cases

1. **Home Server**: Keep DNS pointing to residential IP
2. **Self-Hosted Services**: Automatic DNS updates when ISP changes IP
3. **Failover**: Update DNS to backup server IP on failure
4. **Multi-WAN**: Update based on active internet connection

---

## Security Notes

- **Never expose API publicly** without authentication
- Use Cloudflare proxy (orange cloud) to hide origin IP
- Set short TTL (60-120s) for faster failover
- Monitor update logs for suspicious activity

---

## Support

- **Cloudflare API**: https://developers.cloudflare.com/api/operations/dns-records-for-a-zone-update-dns-record
- **Route53 API**: https://docs.aws.amazon.com/route53/latest/APIReference/
